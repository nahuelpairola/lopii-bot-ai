package account

import (
	"strconv"
	"strings"

	"lopiibot.com/internal/conversation"
)

// accountCreator es lo que este Step necesita para efectivamente crear
// la cuenta en DB y, si corresponde, liberar el flag default de la
// cuenta que lo tenía antes.
type accountCreator interface {
	Insert(a *Account) error
	UnsetDefault(userID uint64, currency Currency) error
	HasDefaultForCurrency(userID uint64, currency Currency) bool
}

// initialBalanceStep pide el saldo inicial, crea la cuenta en DB con los
// datos acumulados del flujo, y decide si vuelve al selector de moneda
// (el usuario puede seguir agregando cuentas) o termina el flujo.
//
// No guarda userID por closure: lo lee de Data en cada llamada, así una
// sola instancia sirve para todos los usuarios.
type initialBalanceStep struct {
	creator accountCreator
}

func (s initialBalanceStep) Prompt(data conversation.Data) conversation.Prompt {
	name := data[dataKeyAccountName].(string)
	currency := data[dataKeyCurrency].(string)
	return conversation.Prompt{Text: msgAskInitialBalance(name, currency)}
}

func (s initialBalanceStep) Process(input conversation.Input, data conversation.Data) conversation.Transition {
	raw := strings.ReplaceAll(strings.TrimSpace(input.Text), ",", ".")
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return conversation.Retry(msgInvalidAmount)
	}

	name := data[dataKeyAccountName].(string)
	currency := Currency(data[dataKeyCurrency].(string))
	isDefault, _ := data[dataKeyIsDefault].(bool)

	// Si la nueva cuenta queda como default y ya había otra default en
	// esa moneda, hay que liberar el flag de la anterior antes de
	// insertar — el índice único de DB no permite dos default a la vez.
	if isDefault && s.creator.HasDefaultForCurrency(data.UserID(), currency) {
		if err := s.creator.UnsetDefault(data.UserID(), currency); err != nil {
			return conversation.Retry(msgAccountCreationError)
		}
	}

	newAccount := &Account{
		UserID:    data.UserID(),
		Name:      name,
		Type:      StandardType,
		Currency:  currency,
		IsDefault: isDefault,
	}

	if err := s.creator.Insert(newAccount); err != nil {
		if err == ErrAccountAlreadyExists {
			return conversation.Retry(msgInvalidAccountName)
		}
		return conversation.Retry(msgAccountCreationError)
	}

	// TODO: registrar el movimiento de "Saldo inicial" por `amount` una
	// vez que exista el repository de movement (subcategoría reservada
	// "Sistema | Saldo inicial").
	_ = amount

	next := conversation.Data{
		conversation.UserIDKey: data.UserID(),
	}
	return conversation.Advance(StepChooseCurrency, next)
}

func (s initialBalanceStep) PossibleNextSteps() []string {
	return []string{StepChooseCurrency}
}
