package flow

import (
	"fmt"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

const (
	ModeCreate = "create"
	ModeUpdate = "update"
)

type InsufficientFunds struct {
	Shortfalls []movement.AccountShortfall
}

func (e *InsufficientFunds) Error() string { return "insufficient funds" }

func ResolveAndInsertMovements(r runner, data conversation.Data) ([]movement.Movement, error) {
	userID, rows := data.UserID(), movement.DecodeMovementRows(data)

	idx, err := LoadAccountIndex(r, userID)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}

	skipBalanceCheck := conversation.Flag(data, conversation.KeySkipBalanceCheck)

	openedWithoutBalance, err := CreateFirstAccount(r, data, rows, idx)
	if err != nil {
		return nil, fmt.Errorf("create first account: %w", err)
	}
	skipBalanceCheck = skipBalanceCheck || openedWithoutBalance

	if err := createCounterpartyAccounts(r, userID, rows, idx); err != nil {
		return nil, fmt.Errorf("create counterparty account: %w", err)
	}

	if err := createPendingSubcategories(r, userID, rows, data); err != nil {
		return nil, fmt.Errorf("create pending subcategory: %w", err)
	}

	data[conversation.KeyMovements] = movement.EncodeMovementRows(rows)

	movements, groups, err := buildMovements(r, userID, rows)
	if err != nil {
		return nil, fmt.Errorf("build movements: %w", err)
	}

	movement.AssignTransactionIDs(movements, groups)
	gain, ok, err := FciRedemptionGain(r, movements)
	if err != nil {
		return nil, fmt.Errorf("fci redemption gain: %w", err)
	}
	if ok {
		movements = append(movements, gain)
	}
	movements, err = movement.Normalize(movements, idx.byID, idx.defaultByCurrency)
	if err != nil {
		return nil, err
	}
	idx.attachAccounts(movements)

	return persistMovements(r, data, movements, idx, skipBalanceCheck)
}

type AccountIndex struct {
	byID              map[uint64]account.Account
	defaultByCurrency map[string]uint64
}

func (idx *AccountIndex) hasDefault(cur currency.Currency) bool {
	_, ok := idx.defaultByCurrency[cur.String()]
	return ok
}

func (idx *AccountIndex) add(a account.Account) {
	id := uint64(a.ID)
	idx.byID[id] = a
	if a.IsDefault {
		idx.defaultByCurrency[a.Currency.String()] = id
	}
}

func (idx *AccountIndex) attachAccounts(movs []movement.Movement) {
	for i := range movs {
		if movs[i].AccountID == nil {
			continue
		}
		if acc, ok := idx.byID[*movs[i].AccountID]; ok {
			a := acc
			movs[i].Account = &a
		}
	}
}

func LoadAccountIndex(r runner, userID uint64) (*AccountIndex, error) {
	accs, err := r.FindUserAccounts(userID)
	if err != nil {
		return nil, err
	}
	idx := &AccountIndex{
		byID:              make(map[uint64]account.Account, len(accs)),
		defaultByCurrency: make(map[string]uint64),
	}
	for _, a := range accs {
		idx.add(a)
	}
	return idx, nil
}

func CreateFirstAccount(r runner, data conversation.Data, rows []movement.MovementRow, idx *AccountIndex) (skipBalanceCheck bool, err error) {
	name := conversation.StringOrEmpty(data[conversation.KeyFirstAccountName])
	if name == "" {
		return false, nil
	}
	userID := data.UserID()
	asked := FirstAccountCurrency(data, idx.hasDefault)
	netDelta := FirstAccountNetDelta(rows, asked)
	var created []string
	byCurrency := map[string]*account.Account{}

	for i, row := range rows {
		if row.AccountID != "" || movement.TypeFromString(row.Type) == movement.Transfer {
			continue
		}
		acc, ok := byCurrency[row.Currency]
		if !ok {
			cur := currency.Currency(row.Currency)
			if idx.hasDefault(cur) {
				continue
			}
			acc = &account.Account{UserID: userID, Name: name, Currency: cur, IsDefault: true}
			if err := r.InsertAccount(acc); err != nil {
				return false, err
			}
			idx.add(*acc)
			byCurrency[row.Currency] = acc
			created = append(created, row.Currency)
			data[conversation.KeyFirstAccountCurrencies] = conversation.EncodeStringSlice(created)
		}
		rows[i].AccountID = strconv.FormatUint(uint64(acc.ID), 10)
	}

	opened, err := openFirstAccountBalance(r, data, byCurrency[asked], netDelta)
	if err != nil {
		return false, err
	}
	return len(created) > 0 && (!opened || len(created) > 1), nil
}

func openFirstAccountBalance(r runner, data conversation.Data, acc *account.Account, netDelta decimal.Decimal) (bool, error) {
	if acc == nil {
		return false, nil
	}
	bal := conversation.StringOrEmpty(data[conversation.KeyFirstAccountBalance])
	if bal == "" {
		return false, nil
	}
	amt, err := movement.ParseARAmount(bal)
	if err != nil || amt.IsNegative() {
		return false, nil
	}
	if err := insertOpeningMovement(r, acc, amt.Sub(netDelta)); err != nil {
		return false, err
	}
	return true, nil
}

func FirstAccountNetDelta(rows []movement.MovementRow, cur string) decimal.Decimal {
	var netDelta decimal.Decimal
	for _, row := range rows {
		if row.AccountID != "" || row.Currency != cur || movement.TypeFromString(row.Type) == movement.Transfer {
			continue
		}
		amt, err := movement.ParseARAmount(row.Amount)
		if err != nil {
			continue
		}
		switch movement.TypeFromString(row.Type) {
		case movement.Expense:
			netDelta = netDelta.Add(amt.Neg())
		case movement.Income:
			netDelta = netDelta.Add(amt)
		}
	}
	return netDelta
}

func insertOpeningMovement(r runner, acc *account.Account, amount decimal.Decimal) error {
	sub, err := r.FindSubcategory(acc.UserID, subcategory.CategorySystem, subcategory.SubOpeningBalance)
	if err != nil {
		return err
	}
	accountID := uint64(acc.ID)
	return r.InsertMovements([]movement.Movement{{
		UserID:        acc.UserID,
		AccountID:     &accountID,
		SubcategoryID: uint64(sub.ID),
		Date:          movement.TodayCivil(),
		Type:          movement.Transfer,
		Amount:        amount,
		Currency:      acc.Currency,
	}})
}

func createCounterpartyAccounts(r runner, userID uint64, rows []movement.MovementRow, idx *AccountIndex) error {
	created := make(map[string]uint64)
	for i, row := range rows {
		if row.AccountID != AccountPendingCreate {
			continue
		}
		if movement.TypeFromString(row.Type) != movement.Transfer &&
			!GuessNamesOwnAccount(row.AccountNameGuess, row.Description) {
			rows[i].AccountID = ""
			continue
		}
		key := row.AccountNameGuess + "|" + row.Currency
		if id, ok := created[key]; ok {
			rows[i].AccountID = strconv.FormatUint(id, 10)
			continue
		}
		newAccount := &account.Account{
			UserID:   userID,
			Name:     row.AccountNameGuess,
			Currency: currency.Currency(row.Currency),
		}
		if err := r.InsertAccount(newAccount); err != nil {
			return err
		}
		id := uint64(newAccount.ID)
		created[key] = id
		idx.add(*newAccount)
		rows[i].AccountID = strconv.FormatUint(id, 10)
	}
	return nil
}

func createPendingSubcategories(r runner, userID uint64, rows []movement.MovementRow, data conversation.Data) error {
	for _, raw := range conversation.DecodeStringSlice(data, conversation.KeyPendingNewSubcats) {
		i, err := strconv.Atoi(raw)
		if err != nil || i < 0 || i >= len(rows) {
			continue
		}
		row := rows[i]
		if row.Category == "" || row.Subcategory == "" {
			continue
		}
		if _, err := r.FindSubcategory(userID, row.Category, row.Subcategory); err == nil {
			continue
		}
		if err := insertSubcategory(r, userID, row.Category, row.Subcategory, "", r.SubcategoryIconForCategory(userID, row.Category)); err != nil {
			return err
		}
	}
	return nil
}

func buildMovements(r runner, userID uint64, rows []movement.MovementRow) ([]movement.Movement, []string, error) {
	movements := make([]movement.Movement, 0, len(rows)+1)
	groups := make([]string, 0, len(rows)+1)
	for _, row := range rows {
		sub, err := r.FindSubcategory(userID, row.Category, row.Subcategory)
		if err != nil {
			return nil, nil, fmt.Errorf("subcategoría %q/%q: %w", row.Category, row.Subcategory, err)
		}
		amount, err := movement.RowAmount(row)
		if err != nil {
			return nil, nil, fmt.Errorf("monto %q: %w", row.Amount, err)
		}
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return nil, nil, fmt.Errorf("fecha %q: %w", row.Date, err)
		}

		var accountID *uint64
		if row.AccountID != "" {
			id, err := strconv.ParseUint(row.AccountID, 10, 64)
			if err != nil {
				return nil, nil, fmt.Errorf("account_id %q: %w", row.AccountID, err)
			}
			accountID = &id
		}

		movements = append(movements, movement.Movement{
			UserID:        userID,
			AccountID:     accountID,
			SubcategoryID: uint64(sub.ID),
			Subcategory:   sub,
			Date:          date,
			Type:          movement.TypeFromString(row.Type),
			Amount:        amount,
			Currency:      currency.Currency(row.Currency),
			Description:   optionalString(row.Description),
		})
		groups = append(groups, row.Group)
	}
	return movements, groups, nil
}

func persistMovements(r runner, data conversation.Data, movs []movement.Movement, idx *AccountIndex, skipBalanceCheck bool) ([]movement.Movement, error) {
	if conversation.StringOrEmpty(data[conversation.KeyMode]) == ModeUpdate {
		oldIDs, err := ParseUintSlice(conversation.DecodeStringSlice(data, conversation.KeyOldMovementIDs))
		if err != nil {
			return nil, fmt.Errorf("parse old movement ids: %w", err)
		}
		if err := r.ReplaceMovements(oldIDs, movs); err != nil {
			return nil, fmt.Errorf("replace movements: %w", err)
		}
		return movs, nil
	}

	if !skipBalanceCheck {
		balances := make(map[uint64]decimal.Decimal, len(idx.byID))
		for id := range idx.byID {
			bal, err := r.SumAmountForAccount(id)
			if err != nil {
				return nil, fmt.Errorf("sum account %d: %w", id, err)
			}
			balances[id] = bal
		}
		if short := movement.CheckBalances(movs, balances, idx.byID); len(short) > 0 {
			return nil, &InsufficientFunds{Shortfalls: short}
		}
	}

	if err := r.InsertMovements(movs); err != nil {
		return nil, fmt.Errorf("insert movements: %w", err)
	}
	return movs, nil
}

func FciRedemptionGain(r runner, movements []movement.Movement) (movement.Movement, bool, error) {
	for _, m := range movements {
		if m.Type != movement.Transfer || m.AccountID == nil {
			continue
		}
		if !m.Amount.IsNegative() {
			continue
		}

		fciSub, err := r.FindSubcategory(m.UserID, "Inversiones", "FCI")
		if err != nil {
			return movement.Movement{}, false, err
		}
		if m.SubcategoryID != uint64(fciSub.ID) {
			continue
		}

		acc, err := r.GetAccount(*m.AccountID)
		if err != nil || acc.IsDefault {
			continue
		}

		balanceBefore, err := r.SumAmountForAccount(*m.AccountID)
		if err != nil {
			return movement.Movement{}, false, err
		}
		redeemed := m.Amount.Neg()
		if redeemed.LessThan(balanceBefore) {
			continue
		}
		gain := redeemed.Sub(balanceBefore)
		if !gain.IsPositive() {
			continue
		}

		gainSub, err := r.FindSubcategory(m.UserID, subcategory.CategorySystem, "Rendimiento inversión")
		if err != nil {
			return movement.Movement{}, false, err
		}
		return movement.Movement{
			TransactionID: m.TransactionID,
			UserID:        m.UserID,
			AccountID:     m.AccountID,
			SubcategoryID: uint64(gainSub.ID),
			Subcategory:   gainSub,
			Date:          m.Date,
			Type:          movement.Income,
			Amount:        gain,
			Currency:      m.Currency,
		}, true, nil
	}
	return movement.Movement{}, false, nil
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
