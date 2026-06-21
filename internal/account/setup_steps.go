package account

import (
	"errors"
	"strings"

	"lopiibot.com/internal/conversation"
)

// accountNameStep pide el nombre de la cuenta y, según si ya hay una
// default en esa moneda, o bien crea la cuenta directo (no hace falta
// preguntar nada más) o salta a confirmDefaultStep para resolver el
// conflicto antes de crearla.
//
// El saldo inicial está deshabilitado por ahora: la cuenta se crea acá
// mismo sin pedir ningún monto (ver createAccount más abajo). Cuando se
// reactive, este Step debería avanzar a un StepInitialBalance en vez de
// crear directamente — ver el comentario en createAccount.
//
// No guarda userID por closure: lo lee de Data en cada llamada (ver
// conversation.Data.UserID), así una sola instancia de este Step sirve
// para todos los usuarios sin pisarse entre sí.
type accountNameStep struct {
	repo    flowRepository
	creator accountCreator
}

func (s accountNameStep) Prompt(data conversation.Data) conversation.Prompt {
	currency := data[dataKeyCurrency].(string)
	return conversation.Prompt{Text: conversation.PrependPendingError(data, msgAskAccountName(currency))}
}

func (s accountNameStep) Process(input conversation.Input, data conversation.Data) conversation.Transition {
	name := strings.TrimSpace(input.Text)
	if name == "" {
		return conversation.Retry(msgInvalidAccountName)
	}

	next := copyData(data)
	next[dataKeyAccountName] = name
	delete(next, "_pending_error") // si veníamos de un reintento por nombre duplicado, ya se mostró

	currency := Currency(data[dataKeyCurrency].(string))
	if s.repo.HasDefaultForCurrency(data.UserID(), currency) {
		return conversation.Advance(StepConfirmDefault, next)
	}

	next[dataKeyIsDefault] = true
	return createAccount(s.creator, next, currency, name, true)
}

func (s accountNameStep) PossibleNextSteps() []string {
	return []string{StepConfirmDefault, StepChooseCurrency}
}

// newConfirmDefaultStep pregunta si la cuenta nueva reemplaza a la
// default existente en esa moneda, y crea la cuenta apenas el usuario
// responde — no es un ChoiceStep puramente genérico porque necesita
// ejecutar el efecto de creación, por eso se arma con un Step custom en
// vez de uno configurado.
type confirmDefaultStep struct {
	creator accountCreator
}

func newConfirmDefaultStep(creator accountCreator) confirmDefaultStep {
	return confirmDefaultStep{creator: creator}
}

func (s confirmDefaultStep) Prompt(data conversation.Data) conversation.Prompt {
	currency := data[dataKeyCurrency].(string)
	name := data[dataKeyAccountName].(string)
	return conversation.Prompt{
		Text: msgConfirmReplaceDefault(currency, name),
		Buttons: []conversation.Button{
			{Label: btnReplaceYes, Data: "yes"},
			{Label: btnReplaceNo, Data: "no"},
		},
	}
}

func (s confirmDefaultStep) Process(input conversation.Input, data conversation.Data) conversation.Transition {
	var isDefault bool
	switch input.CallbackData {
	case "yes":
		isDefault = true
	case "no":
		isDefault = false
	default:
		return conversation.Retry("Elegí una de las opciones.")
	}

	currency := Currency(data[dataKeyCurrency].(string))
	name := data[dataKeyAccountName].(string)

	next := copyData(data)
	next[dataKeyIsDefault] = isDefault

	return createAccount(s.creator, next, currency, name, isDefault)
}

func (s confirmDefaultStep) PossibleNextSteps() []string {
	return []string{StepChooseCurrency, StepAccountName}
}

// createAccount inserta la cuenta en DB y decide la transición. Punto
// único de creación, compartido por accountNameStep (cuando no hace
// falta preguntar por el default) y confirmDefaultStep (cuando sí).
//
// El saldo inicial está deshabilitado: en vez de avanzar a un
// StepInitialBalance que pida el monto, se crea la cuenta directo con
// saldo cero implícito. Para reactivarlo, esta función debería devolver
// conversation.Advance(StepInitialBalance, next) en el caso de éxito en
// vez de crear la cuenta ella misma — y el Step de saldo inicial (hoy
// eliminado, ver historial) sería quien la cree después de pedir el monto.
func createAccount(creator accountCreator, data conversation.Data, currency Currency, name string, isDefault bool) conversation.Transition {
	if isDefault && creator.HasDefaultForCurrency(data.UserID(), currency) {
		if err := creator.UnsetDefault(data.UserID(), currency); err != nil {
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

	if err := creator.Insert(newAccount); err != nil {
		if errors.Is(err, ErrAccountAlreadyExists) {
			next := copyData(data)
			delete(next, dataKeyAccountName)
			next = conversation.WithPendingError(next, msgAccountAlreadyExists(name, currency.String()))
			return conversation.Advance(StepAccountName, next)
		}
		return conversation.Retry(msgAccountCreationError)
	}

	next := conversation.Data{
		conversation.UserIDKey: data.UserID(),
		dataKeyAccountsCreated: true,
	}
	return conversation.Advance(StepChooseCurrency, next)
}

func copyData(data conversation.Data) conversation.Data {
	next := conversation.Data{}
	for k, v := range data {
		next[k] = v
	}
	return next
}
