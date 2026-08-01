package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

const (
	movementCreateFlowName = "movement_create"

	stepCreateFirstAccount  = "create_first_account"
	stepFirstAccountBalance = "first_account_balance"
	stepResolveCategory     = "resolve_category"
	stepResolveSubcategory  = "resolve_subcategory"
	stepResolveAccount      = "resolve_account"

	// optionCancel is the escape hatch every gap-fill ChoiceStep offers:
	// the user realizing mid-flow that the original message was a
	// mistake, with nowhere else to bail out (see finishMovementCreateFlow).
	optionCancel = "cancel"

	// optionConfirm is the shared confirm-button value across movement/account flows.
	optionConfirm = "confirm"

	// optionBalanceLater lets the user skip the opening-balance question for
	// a freshly lazy-created account.
	optionBalanceLater = "balance_later"

	// accountChoiceExistingPrefix marca el botón de una cuenta que YA existe,
	// contra optionAccountCreate. El valor se arma con este prefijo y se
	// desarma con TrimPrefix — el par clásico que se desincroniza si cada lado
	// escribe el literal por su cuenta.
	accountChoiceExistingPrefix = "existing:"

	// optionAccountCreate es el botón "crear la cuenta que adivinó el LLM".
	optionAccountCreate = "create"
)

// cancelOption is the "🚫 Cancelar" button appended to every gap-fill
// step's options — same escape hatch the movement_confirm_intent gate
// offers before the flow even starts, but for the case where the user
// only realizes mid-flow that the message was wrong.
var cancelOption = conversation.ChoiceOption{Label: "🚫 Cancelar", Value: optionCancel, Finish: true}

// NewMovementCreateFlow builds the single registered flow used for
// CREATE's gap-fill (and, per movement_update_flow.go, for reusing the
// same graph to fill gaps in an UPDATE's corrected set). It is only
// ever started via StartWithData when Call 2 CREATE left at least one
// gap — a fully-resolved CREATE never touches the conversation engine
// at all (see free_text.go).
func NewMovementCreateFlow(subcategories subcategoryRepository, accounts accountRepository) *conversation.Flow {
	steps := map[string]conversation.Step{
		stepCreateFirstAccount: conversation.TextStep{
			PromptText: msgAskFirstAccountName,
			DataKey:    keyFirstAccountName,
			SkipIf: func(data conversation.Data) (string, bool) {
				if needsFirstAccount(data, func(cur currency.Currency) bool {
					return accounts.HasDefaultForCurrency(data.UserID(), cur)
				}) {
					return "", false // hay que preguntar
				}
				return stepResolveCategory, true
			},
			Validate: func(text string, _ conversation.Data) string {
				if strings.TrimSpace(text) == "" {
					return msgInvalidAccountCreateName
				}
				return ""
			},
			NextStep:      stepFirstAccountBalance,
			EscapeOptions: []conversation.ChoiceOption{cancelOption},
			OnEscape: func(value string, data conversation.Data) conversation.Data {
				if value != optionCancel {
					return data
				}
				next := copyData(data)
				setFlag(next, keyCancelled)
				return next
			},
		},
		stepFirstAccountBalance: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return msgAskFirstAccountBalance(stringOrEmpty(data[keyFirstAccountName]))
			},
			DataKey: keyFirstAccountBalance,
			SkipIf: func(data conversation.Data) (string, bool) {
				if stringOrEmpty(data[keyFirstAccountName]) == "" {
					return stepResolveCategory, true // no hubo first-account
				}
				return "", false
			},
			Validate: func(text string, _ conversation.Data) string {
				if _, err := parseARAmount(text); err != nil {
					return account.MsgInvalidAmount
				}
				return ""
			},
			NextStep: stepResolveCategory,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepCreateFirstAccount},
				{Label: "⏭️ Después", Value: optionBalanceLater, NextStep: stepResolveCategory},
				cancelOption,
			},
			OnEscape: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				if value == optionCancel {
					setFlag(next, keyCancelled)
				}
				return next
			},
		},
		stepResolveCategory: conversation.ChoiceStep{
			PromptText: msgAskCategory,
			SkipIf: func(data conversation.Data) (string, bool) {
				if len(decodeStringSlice(data, keyPendingCategoryGaps)) == 0 {
					return stepResolveAccount, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				cats, _ := subcategories.DistinctCategoriesForUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(cats))
				for _, cat := range cats {
					opts = append(opts, conversation.ChoiceOption{
						Label:    subcategories.IconForCategory(data.UserID(), cat) + " " + cat,
						Value:    cat,
						NextStep: stepResolveSubcategory,
					})
				}
				opts = append(opts, cancelOption)
				return opts
			},
			DeclaredNextSteps: []string{stepResolveSubcategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel {
					next := copyData(data)
					setFlag(next, keyCancelled)
					return next
				}
				gaps := decodeStringSlice(data, keyPendingCategoryGaps)
				if len(gaps) == 0 {
					return data
				}
				next := copyData(data)
				next[keyGapActiveRow] = gaps[0]
				rows := decodeMovementRows(data)
				idx, _ := strconv.Atoi(gaps[0])
				rows[idx].Category = value
				next[keyMovements] = encodeMovementRows(rows)
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
		stepResolveSubcategory: conversation.ChoiceStep{
			PromptText: msgAskSubcategory,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				rowIdx, _ := strconv.Atoi(stringOrEmpty(data[keyGapActiveRow]))
				rows := decodeMovementRows(data)
				category := rows[rowIdx].Category

				subs, _ := subcategories.FindAllForUser(data.UserID())
				var opts []conversation.ChoiceOption
				for _, s := range subs {
					if s.Category != category {
						continue
					}
					opts = append(opts, conversation.ChoiceOption{
						Label:    s.Subcategory,
						Value:    s.Subcategory,
						NextStep: stepResolveCategory,
					})
				}
				opts = append(opts, cancelOption)
				return opts
			},
			DeclaredNextSteps: []string{stepResolveCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel {
					next := copyData(data)
					setFlag(next, keyCancelled)
					return next
				}
				next := copyData(data)
				gaps := decodeStringSlice(data, keyPendingCategoryGaps)
				rowIdx, _ := strconv.Atoi(stringOrEmpty(data[keyGapActiveRow]))

				rows := decodeMovementRows(data)
				rows[rowIdx].Subcategory = value
				next[keyMovements] = encodeMovementRows(rows)
				if len(gaps) > 0 {
					next[keyPendingCategoryGaps] = encodeStringSlice(gaps[1:])
				}
				next[keyGapActiveRow] = ""
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
		stepResolveAccount: conversation.ChoiceStep{
			PromptText: msgAskAccount,
			SkipIf: func(data conversation.Data) (string, bool) {
				if len(decodeStringSlice(data, keyPendingAccountGaps)) == 0 {
					return "", true // nothing left — the flow is complete
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				gaps := decodeStringSlice(data, keyPendingAccountGaps)
				if len(gaps) == 0 {
					return nil
				}
				rowIdx, _ := strconv.Atoi(gaps[0])
				rows := decodeMovementRows(data)

				accs, _ := accounts.FindByUserID(data.UserID())
				var opts []conversation.ChoiceOption
				for _, a := range accs {
					if a.Currency.String() != rows[rowIdx].Currency {
						continue
					}
					opts = append(opts, conversation.ChoiceOption{
						Label:    a.Name,
						Value:    accountChoiceExistingPrefix + strconv.FormatUint(uint64(a.ID), 10),
						NextStep: stepResolveAccount,
					})
				}
				opts = append(opts, conversation.ChoiceOption{
					Label:    "➕ Crear cuenta \"" + rows[rowIdx].AccountNameGuess + "\"",
					Value:    optionAccountCreate,
					NextStep: stepResolveAccount,
				})
				opts = append(opts, cancelOption)
				return opts
			},
			DeclaredNextSteps: []string{stepResolveAccount},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel {
					next := copyData(data)
					setFlag(next, keyCancelled)
					return next
				}
				next := copyData(data)
				gaps := decodeStringSlice(data, keyPendingAccountGaps)
				if len(gaps) == 0 {
					return next
				}
				rowIdx, _ := strconv.Atoi(gaps[0])

				rows := decodeMovementRows(data)
				if value == optionAccountCreate {
					rows[rowIdx].AccountID = accountPendingCreate
				} else {
					rows[rowIdx].AccountID = strings.TrimPrefix(value, accountChoiceExistingPrefix)
				}
				next[keyMovements] = encodeMovementRows(rows)
				next[keyPendingAccountGaps] = encodeStringSlice(gaps[1:])
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(movementCreateFlowName, stepCreateFirstAccount, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// finishMovementCreateFlow is the Telegram-facing wrapper around
// resolveAndInsertMovements — same split for testability as
// finishInitialBalanceFlow/insertInitialBalanceMovements.
func (c *controller) finishMovementCreateFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if flag(data, keyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeCreateCancelled)
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgCreateCancelled})
		}
		return
	}

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		slog.ErrorContext(ctx, "movement insert failed", "user_id", data.UserID(), "reason", guardReason(err))
		c.resolveMetric(ctx, data.UserID(), outcomeCreateFailed)
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: createErrorCopy(err)})
		}
		return
	}
	c.resolveMetric(ctx, data.UserID(), outcomeCreateInserted, collectMovementIDs(inserted)...)
	if b != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgConfirmMovements(inserted)})
		if name := stringOrEmpty(data[keyFirstAccountName]); name != "" {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgFirstAccountDefault(name)})
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgInviteMoreAccounts})
			// R1/R2 just fired — mark correct_tip sent (not delivered) so the
			// post-message nudge hook doesn't stack a 3rd tip on this same turn.
			if c.nudges != nil {
				_ = c.nudges.MarkSent(data.UserID(), nudgeCorrectTip)
			}
		}
	}
}

// resolveAndInsertMovements convierte las filas resueltas del flujo en
// movimientos reales y los persiste. Cada paso está abajo, en su propia
// función; acá queda solo el orden, que es lo que importa: primero se
// materializan las cuentas que faltan (porque los movimientos necesitan
// apuntar a algo), después se arman los movimientos, después se aplican las
// reglas sobre el set completo, y recién al final se escribe.
func (c *controller) resolveAndInsertMovements(data conversation.Data) ([]movement.Movement, error) {
	userID, rows := data.UserID(), decodeMovementRows(data)

	idx, err := c.loadAccountIndex(userID)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}

	// skipBalanceCheck tiene DOS orígenes y hay que respetar los dos. El gate de
	// saldo negativo lo deja en data cuando el usuario ya dijo "sí, dale igual"
	// (ver movement_negative_confirm_flow.go), y createFirstAccount lo devuelve
	// cuando creó la primera cuenta sin saldo de apertura. Mirar solo uno haría
	// que confirmar el gate vuelva a disparar el gate.
	skipBalanceCheck := flag(data, keySkipBalanceCheck)

	openedWithoutBalance, err := c.createFirstAccount(data, rows, idx)
	if err != nil {
		return nil, fmt.Errorf("create first account: %w", err)
	}
	skipBalanceCheck = skipBalanceCheck || openedWithoutBalance

	if err := c.createCounterpartyAccounts(userID, rows, idx); err != nil {
		return nil, fmt.Errorf("create counterparty account: %w", err)
	}

	movements, groups, err := c.buildMovements(userID, rows)
	if err != nil {
		return nil, fmt.Errorf("build movements: %w", err)
	}

	// Group by the LLM tag, compute the FCI gain (inherits the leg's
	// transaction_id), then enforce the invariants on the complete set.
	movement.AssignTransactionIDs(movements, groups)
	gain, ok, err := fciRedemptionGain(c, movements)
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

	return c.persistMovements(data, movements, idx, skipBalanceCheck)
}

// accountIndex es el estado de cuentas compartido por toda la resolución: las
// del usuario indexadas por id, y cuál es la default de cada moneda.
//
// Los dos mapas van juntos en un struct, y no como dos parámetros sueltos, justo
// porque se mutan de a pares cada vez que se crea una cuenta a mitad del
// proceso: pasarlos por separado es cómo se desincronizan.
type accountIndex struct {
	byID              map[uint64]account.Account
	defaultByCurrency map[string]uint64
}

// add registra una cuenta en los dos mapas de una sola vez.
func (idx *accountIndex) add(a account.Account) {
	id := uint64(a.ID)
	idx.byID[id] = a
	if a.IsDefault {
		idx.defaultByCurrency[a.Currency.String()] = id
	}
}

// attachAccounts cuelga de cada movimiento la cuenta que le tocó, para que el
// recibo de confirmación pueda nombrarla. Estos movimientos se arman en memoria
// y nunca se leen de la DB, así que no hay ningún Preload en el que apoyarse.
func (idx *accountIndex) attachAccounts(movs []movement.Movement) {
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

// loadAccountIndex trae las cuentas del usuario en UNA consulta y las indexa.
func (c *controller) loadAccountIndex(userID uint64) (*accountIndex, error) {
	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		return nil, err
	}
	idx := &accountIndex{
		byID:              make(map[uint64]account.Account, len(accs)),
		defaultByCurrency: make(map[string]uint64),
	}
	for _, a := range accs {
		idx.add(a)
	}
	return idx, nil
}

// createFirstAccount materializa la cuenta que stepCreateFirstAccount pidió por
// nombre. Se crea acá, al terminar el flujo, y no cuando el usuario tipeó el
// nombre: así abandonar a mitad de camino no deja una cuenta huérfana. Queda
// como default si la moneda todavía no tenía ninguna.
//
// Devuelve skipBalanceCheck=true cuando la cuenta se creó SIN saldo de apertura.
// En ese caso el primer gasto la deja en negativo por construcción, así que
// avisar sería ruido y no información.
func (c *controller) createFirstAccount(data conversation.Data, rows []movementRow, idx *accountIndex) (skipBalanceCheck bool, err error) {
	name := stringOrEmpty(data[keyFirstAccountName])
	if name == "" {
		return false, nil
	}
	userID := data.UserID()
	netDelta := firstAccountNetDelta(rows)

	for i, row := range rows {
		if row.AccountID != "" || movement.TypeFromString(row.Type) == movement.Transfer {
			continue
		}
		cur := currency.Currency(row.Currency)
		newAcc := &account.Account{
			UserID:    userID,
			Name:      name,
			Currency:  cur,
			IsDefault: !c.accounts.HasDefaultForCurrency(userID, cur),
		}
		if err := c.accounts.Insert(newAcc); err != nil {
			return false, err
		}
		idx.add(*newAcc)
		rows[i].AccountID = strconv.FormatUint(uint64(newAcc.ID), 10)

		bal := stringOrEmpty(data[keyFirstAccountBalance])
		if bal == "" {
			skipBalanceCheck = true
			continue
		}
		amt, perr := parseARAmount(bal)
		if perr != nil || amt.IsNegative() {
			continue
		}
		if err := c.insertOpeningMovement(newAcc, amt.Sub(netDelta)); err != nil {
			return false, err
		}
	}
	return skipBalanceCheck, nil
}

// firstAccountNetDelta suma el efecto neto de los movimientos que van a caer en
// la primera cuenta.
//
// Hace falta porque la pregunta es "¿cuánto saldo tenés AHORA?", y esa respuesta
// ya incluye los movimientos que el usuario está cargando en este mismo mensaje.
// La apertura tiene que compensarlos: apertura = saldo declarado − netDelta.
func firstAccountNetDelta(rows []movementRow) decimal.Decimal {
	var netDelta decimal.Decimal
	for _, row := range rows {
		if row.AccountID != "" || movement.TypeFromString(row.Type) == movement.Transfer {
			continue
		}
		amt, err := parseARAmount(row.Amount)
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
func (c *controller) insertOpeningMovement(acc *account.Account, amount decimal.Decimal) error {
	sub, err := c.subcategories.FindByCategoryAndSubcategory(acc.UserID, subcategory.CategorySystem, subcategory.SubOpeningBalance)
	if err != nil {
		return err
	}
	accountID := uint64(acc.ID)
	return c.movements.InsertBatch([]movement.Movement{{
		UserID:        acc.UserID,
		AccountID:     &accountID,
		SubcategoryID: uint64(sub.ID),
		Date:          time.Now(),
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
// fila que nombró una cuenta propia distinta del merchant — "pagué con Brubank"
// —, que desde que buildCreateSeed le abre gap llega hasta acá; sin esto el bot
// preguntaría, ofrecería crearla y después tiraría la respuesta.
func (c *controller) createCounterpartyAccounts(userID uint64, rows []movementRow, idx *accountIndex) error {
	created := make(map[string]uint64) // "nombre|moneda" -> id, para no crear dos veces la misma
	for i, row := range rows {
		if row.AccountID != accountPendingCreate {
			continue
		}
		if movement.TypeFromString(row.Type) != movement.Transfer &&
			!guessNamesOwnAccount(row.AccountNameGuess, row.Merchant) {
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
		if err := c.accounts.Insert(newAccount); err != nil {
			return err
		}
		id := uint64(newAccount.ID)
		created[key] = id
		idx.add(*newAccount)
		rows[i].AccountID = strconv.FormatUint(id, 10)
	}
	return nil
}

// buildMovements traduce cada movementRow (todo strings, por el round-trip de
// JSONB) al movement.Movement real. No aplica ninguna regla de plata: resuelve
// la subcategoría, parsea monto y fecha, y devuelve en paralelo los tags de
// grupo que después usa assignTransactionIDs.
//
// Cada error dice qué campo lo causó: antes todos estos fallos llegaban al log
// como "other" (ver guardReason), o sea que una subcategoría inexistente y una
// fecha mal parseada eran indistinguibles en producción.
func (c *controller) buildMovements(userID uint64, rows []movementRow) ([]movement.Movement, []string, error) {
	movements := make([]movement.Movement, 0, len(rows)+1)
	groups := make([]string, 0, len(rows)+1)
	for _, row := range rows {
		sub, err := c.subcategories.FindByCategoryAndSubcategory(userID, row.Category, row.Subcategory)
		if err != nil {
			return nil, nil, fmt.Errorf("subcategoría %q/%q: %w", row.Category, row.Subcategory, err)
		}
		amount, err := parseARAmount(row.Amount)
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
			Merchant:      optionalString(row.Merchant),
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
// explícitamente (ver skipBalanceCheck en resolveAndInsertMovements).
func (c *controller) persistMovements(data conversation.Data, movs []movement.Movement, idx *accountIndex, skipBalanceCheck bool) ([]movement.Movement, error) {
	if stringOrEmpty(data[keyMode]) == modeUpdate {
		oldIDs, err := parseUintSlice(decodeStringSlice(data, keyOldMovementIDs))
		if err != nil {
			return nil, fmt.Errorf("parse old movement ids: %w", err)
		}
		if err := c.movements.ReplaceMovements(oldIDs, movs); err != nil {
			return nil, fmt.Errorf("replace movements: %w", err)
		}
		return movs, nil
	}

	if !skipBalanceCheck {
		balances := make(map[uint64]decimal.Decimal, len(idx.byID))
		for id := range idx.byID {
			bal, err := c.movements.SumAmountForAccount(id)
			if err != nil {
				return nil, fmt.Errorf("sum account %d: %w", id, err)
			}
			balances[id] = bal
		}
		// Sin envolver: el caller lo detecta con errors.As para desviar al gate.
		if short := movement.CheckBalances(movs, balances, idx.byID); len(short) > 0 {
			return nil, &insufficientFunds{shortfalls: short}
		}
	}

	if err := c.movements.InsertBatch(movs); err != nil {
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
func fciRedemptionGain(c *controller, movements []movement.Movement) (movement.Movement, bool, error) {
	for _, m := range movements {
		if m.Type != movement.Transfer || m.AccountID == nil {
			continue
		}
		if !m.Amount.IsNegative() {
			continue
		}

		fciSub, err := c.subcategories.FindByCategoryAndSubcategory(m.UserID, "Inversiones", "FCI")
		if err != nil {
			return movement.Movement{}, false, err
		}
		if m.SubcategoryID != uint64(fciSub.ID) {
			continue
		}

		acc, err := c.accounts.GetAccount(*m.AccountID)
		if err != nil || acc.IsDefault {
			continue // default (everyday) account: a subscription, not a redemption
		}

		balanceBefore, err := c.movements.SumAmountForAccount(*m.AccountID)
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

		gainSub, err := c.subcategories.FindByCategoryAndSubcategory(m.UserID, subcategory.CategorySystem, "Rendimiento inversión")
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
