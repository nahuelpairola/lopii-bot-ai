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

// mode discriminates the flow's write path: a fresh CREATE vs a resolved
// UPDATE reusing the create flow. Stored under conversation.KeyMode.
const (
	ModeCreate = "create"
	ModeUpdate = "update"
)

// InsufficientFunds NO es del dominio: es cómo este paquete se avisa a sí
// mismo que tiene que desviar al gate de confirmación en vez de insertar. El
// dominio solo reporta los shortfalls (movement.CheckBalances); qué hacer con
// ellos es una decisión de UI que toma el borde.
type InsufficientFunds struct {
	Shortfalls []movement.AccountShortfall
}

func (e *InsufficientFunds) Error() string { return "insufficient funds" }

// ResolveAndInsertMovements convierte las filas resueltas del flujo en
// movimientos reales y los persiste. Cada paso está abajo, en su propia
// función; acá queda solo el orden, que es lo que importa: primero se
// materializan las cuentas que faltan (porque los movimientos necesitan
// apuntar a algo), después se arman los movimientos, después se aplican las
// reglas sobre el set completo, y recién al final se escribe.
func ResolveAndInsertMovements(r runner, data conversation.Data) ([]movement.Movement, error) {
	userID, rows := data.UserID(), movement.DecodeMovementRows(data)

	idx, err := LoadAccountIndex(r, userID)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}

	// skipBalanceCheck tiene DOS orígenes y hay que respetar los dos. El gate de
	// saldo negativo lo deja en data cuando el usuario ya dijo "sí, dale igual"
	// (ver movement_negative_confirm_flow.go), y createFirstAccount lo devuelve
	// cuando creó la primera cuenta sin saldo de apertura. Mirar solo uno haría
	// que confirmar el gate vuelva a disparar el gate.
	skipBalanceCheck := conversation.Flag(data, conversation.KeySkipBalanceCheck)

	openedWithoutBalance, err := CreateFirstAccount(r, data, rows, idx)
	if err != nil {
		return nil, fmt.Errorf("create first account: %w", err)
	}
	skipBalanceCheck = skipBalanceCheck || openedWithoutBalance

	if err := createCounterpartyAccounts(r, userID, rows, idx); err != nil {
		return nil, fmt.Errorf("create counterparty account: %w", err)
	}

	// Las cuentas ya están materializadas y las filas volvieron con su
	// account_id. Se escriben de vuelta en data porque esta función se puede
	// REINTENTAR sobre el mismo data: el gate de saldo insuficiente parkea y
	// vuelve a entrar acá al confirmar. Sin esto el reintento ve las filas
	// originales, sin cuenta, y crea la cuenta y su apertura por segunda vez.
	data[conversation.KeyMovements] = movement.EncodeMovementRows(rows)

	movements, groups, err := buildMovements(r, userID, rows)
	if err != nil {
		return nil, fmt.Errorf("build movements: %w", err)
	}

	// Group by the LLM tag, compute the FCI gain (inherits the leg's
	// transaction_id), then enforce the invariants on the complete set.
	movement.AssignTransactionIDs(movements, groups)
	gain, ok, err := FciRedemptionGain(r, movements)
	if err != nil {
		return nil, fmt.Errorf("fci redemption gain: %w", err)
	}
	if ok {
		movements = append(movements, gain)
	}
	// Sin envolver: los sentinels del guard se matchean con errors.Is arriba
	// (createErrorCopy/guardReason) y no ganan nada con más contexto.
	movements, err = movement.Normalize(movements, idx.byID, idx.defaultByCurrency)
	if err != nil {
		return nil, err
	}
	idx.attachAccounts(movements)

	return persistMovements(r, data, movements, idx, skipBalanceCheck)
}

// AccountIndex es el estado de cuentas compartido por toda la resolución: las
// del usuario indexadas por id, y cuál es la default de cada moneda.
//
// Los dos mapas van juntos en un struct, y no como dos parámetros sueltos, justo
// porque se mutan de a pares cada vez que se crea una cuenta a mitad del
// proceso: pasarlos por separado es cómo se desincronizan.
type AccountIndex struct {
	byID              map[uint64]account.Account
	defaultByCurrency map[string]uint64
}

// hasDefault dice si la moneda ya tiene una cuenta por defecto donde caer.
// Se pregunta al índice y no al repo porque el índice se mutó con las cuentas
// creadas en esta misma resolución, que todavía no volvieron de la base.
func (idx *AccountIndex) hasDefault(cur currency.Currency) bool {
	_, ok := idx.defaultByCurrency[cur.String()]
	return ok
}

// add registra una cuenta en los dos mapas de una sola vez.
func (idx *AccountIndex) add(a account.Account) {
	id := uint64(a.ID)
	idx.byID[id] = a
	if a.IsDefault {
		idx.defaultByCurrency[a.Currency.String()] = id
	}
}

// attachAccounts cuelga de cada movimiento la cuenta que le tocó, para que el
// recibo de confirmación pueda nombrarla. Estos movimientos se arman en memoria
// y nunca se leen de la DB, así que no hay ningún Preload en el que apoyarse.
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

// LoadAccountIndex trae las cuentas del usuario en UNA consulta y las indexa.
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

// CreateFirstAccount materializa la cuenta que stepCreateFirstAccount pidió por
// nombre. Se crea acá, al terminar el flujo, y no cuando el usuario tipeó el
// nombre: así abandonar a mitad de camino no deja una cuenta huérfana. Queda
// como default si la moneda todavía no tenía ninguna.
//
// Devuelve skipBalanceCheck=true cuando alguna cuenta se creó SIN saldo de
// apertura. En ese caso el primer gasto la deja en negativo por construcción,
// así que avisar sería ruido y no información.
//
// Itera MONEDAS, no filas: se abre una cuenta por moneda que no tenga default,
// y todas las filas de esa moneda van a esa misma cuenta. Una moneda que YA
// tiene default no se toca — sus filas caen ahí solas en movement.Normalize.
func CreateFirstAccount(r runner, data conversation.Data, rows []movement.MovementRow, idx *AccountIndex) (skipBalanceCheck bool, err error) {
	name := conversation.StringOrEmpty(data[conversation.KeyFirstAccountName])
	if name == "" {
		return false, nil
	}
	userID := data.UserID()
	// asked es la moneda por la que preguntó stepCreateFirstAccount, y la única
	// a la que se le puede aplicar el saldo declarado: el usuario contestó ese
	// número mirando "¿cuánto tenés en <nombre> (dólares)?". Se calcula ANTES de
	// crear nada, porque crear una cuenta cambia la respuesta.
	asked := FirstAccountCurrency(data, idx.hasDefault)
	// El neteo se calcula ACÁ, antes de que el loop les ponga account_id a las
	// filas: FirstAccountNetDelta saltea toda fila que ya tenga cuenta, así que
	// calcularlo después da cero siempre.
	netDelta := FirstAccountNetDelta(rows, asked)
	// Las monedas se anotan para el mensaje de confirmación: cuando éste corre,
	// las filas ya tienen account_id y no hay forma de saber qué monedas se
	// acaban de crear. Ver conversation.KeyFirstAccountCurrencies.
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
				continue // ya tiene dónde caer; Normalize la manda a la default
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
	// Toda cuenta creada que NO recibió apertura arranca en cero.
	return len(created) > 0 && (!opened || len(created) > 1), nil
}

// openFirstAccountBalance escribe la apertura de la cuenta cuya moneda el
// usuario declaró. Devuelve si llegó a escribirla: sin saldo declarado, o con
// uno ilegible o negativo, la cuenta arranca en cero.
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

// FirstAccountNetDelta suma el efecto neto de los movimientos que van a caer en
// la primera cuenta de la moneda cur.
//
// Hace falta porque la pregunta es "¿cuánto saldo tenés AHORA?", y esa respuesta
// ya incluye los movimientos que el usuario está cargando en este mismo mensaje.
// La apertura tiene que compensarlos: apertura = saldo declarado − netDelta.
//
// El filtro por moneda no es un detalle: sumar un gasto en pesos contra un saldo
// declarado en dólares abre la cuenta con un número que no existe, y como el
// balance es la suma de los movimientos, ese error no se corrige nunca solo.
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

// insertOpeningMovement escribe el movimiento de saldo inicial de una cuenta.
//
// Toma la cuenta entera por el mismo motivo que insertAccountOpeningMovement: id
// y moneda salen de la misma fila, así que el movimiento no puede terminar en una
// moneda distinta a la de su cuenta. Y por el mismo motivo tampoco pasa por
// movement.Normalize — una apertura es una pata suelta sin contraparte, y el
// guard rechaza toda transferencia que no sea un grupo de 2.
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

// createCounterpartyAccounts resuelve las filas marcadas PENDING_CREATE.
//
// El centinela lo escribe UN SOLO lugar: el tap del usuario en "➕ Crear cuenta
// X" del gap-fill. O sea que llegar acá ya significa que él pidió la cuenta.
//
// Aun así un gasto o un ingreso no puede crear una cuenta que se llama como la
// contraparte ("pizza con Juan" no es la cuenta de Juan): esas filas se dejan sin
// cuenta y el guard las resuelve a la default de su moneda. Lo que sí crea es la
// fila que nombró una cuenta propia que no aparece en la description — "pagué
// con Brubank" —, que desde que buildCreateSeed le abre gap llega hasta acá; sin
// esto el bot preguntaría, ofrecería crearla y después tiraría la respuesta.
func createCounterpartyAccounts(r runner, userID uint64, rows []movement.MovementRow, idx *AccountIndex) error {
	created := make(map[string]uint64) // "nombre|moneda" -> id, para no crear dos veces la misma
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

// buildMovements traduce cada movement.MovementRow (todo strings, por el round-trip de
// JSONB) al movement.Movement real. No aplica ninguna regla de plata: resuelve
// la subcategoría, parsea monto y fecha, y devuelve en paralelo los tags de
// grupo que después usa assignTransactionIDs.
//
// Cada error dice qué campo lo causó: antes todos estos fallos llegaban al log
// como "other" (ver guardReason), o sea que una subcategoría inexistente y una
// fecha mal parseada eran indistinguibles en producción.
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
			PaymentMethod: optionalString(row.PaymentMethod),
			Description:   optionalString(row.Description),
		})
		groups = append(groups, row.Group)
	}
	return movements, groups, nil
}

// persistMovements es el único punto donde este camino escribe movimientos.
//
// UPDATE reemplaza el set viejo ENTERO (nunca un patch parcial, ver
// ReplaceMovements). CREATE inserta, previo chequeo de que la operación no deje
// ninguna cuenta en negativo — salvo que ese chequeo ya se haya salteado
// explícitamente (ver skipBalanceCheck en ResolveAndInsertMovements).
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
		// Sin envolver: el caller lo detecta con errors.As para desviar al gate.
		if short := movement.CheckBalances(movs, balances, idx.byID); len(short) > 0 {
			return nil, &InsufficientFunds{Shortfalls: short}
		}
	}

	if err := r.InsertMovements(movs); err != nil {
		return nil, fmt.Errorf("insert movements: %w", err)
	}
	return movs, nil
}

// fciRedemptionGain detects an FCI-redemption outflow leg among the
// movements about to be inserted and, per the spec's deterministic
// rule, computes a gain movement only when the redeemed amount is at
// least the account's balance before this transaction. Never guesses a
// number the app can't actually justify.
//
// A negative-amount transfer under Inversiones|FCI is NOT enough to
// identify a redemption: an FCI *subscription* (money leaving the
// wallet to invest) has the exact same shape (same type, same negative
// sign, same subcategory — there's only one Inversiones|FCI
// subcategory, no separate buy/sell). The disambiguator is
// Account.IsDefault: an FCI account is almost never the user's default
// (everyday) wallet, so a negative leg on the default account is a
// subscription, never a redemption candidate.
//
// Repository errors on lookups that matter once a movement otherwise
// looks like a genuine redemption candidate (subcategory resolution,
// balance lookup, gain-subcategory resolution) are propagated instead
// of silently treated as "not a candidate" — a config problem (e.g. a
// missing reserved subcategory) or a transient DB failure should never
// silently drop a real gain.
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
			continue // default (everyday) account: a subscription, not a redemption
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
