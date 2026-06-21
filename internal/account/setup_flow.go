package account

import "lopiibot.com/internal/conversation"

// Nombres de los pasos del flujo de alta de cuentas.
//
// StepInitialBalance no existe por ahora: el saldo inicial está
// deshabilitado (ver setup_steps.go, función createAccount). La cuenta
// se crea directo desde accountNameStep o confirmDefaultStep.
const (
	FlowName = "account_setup"

	StepChooseCurrency = "choose_currency"
	StepAccountName    = "account_name"
	StepConfirmDefault = "confirm_default"
)

// Claves usadas dentro de Data a lo largo del flujo.
const (
	dataKeyCurrency        = "currency"
	dataKeyAccountName     = "account_name"
	dataKeyIsDefault       = "is_default"
	dataKeyAccountsCreated = "accounts_created" // marca que ya se creó al menos una en este flujo
)

// flowRepository es lo mínimo que el flujo necesita del dominio de
// cuentas para decidir sus transiciones (sin incluir el Insert, que
// vive en AccountCreator).
type flowRepository interface {
	HasDefaultForCurrency(userID uint64, currency Currency) bool
}

// NewSetupFlow arma el Flow completo de alta de cuentas. Es estático: no
// depende de ningún usuario en particular (los Steps leen el userID de
// Data en runtime, ver conversation.Data.UserID), así que se construye y
// se registra en el Engine una única vez al levantar el server.
func NewSetupFlow(repo flowRepository, creator accountCreator) (*conversation.Flow, error) {
	chooseCurrency := conversation.ChoiceStep{
		PromptText: func(data conversation.Data) string {
			if _, already := data[dataKeyAccountsCreated]; already {
				return msgAskAddAnotherAccount
			}
			return msgWelcomeAccountsIntro
		},
		OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
			opts := make([]conversation.ChoiceOption, 0, len(SupportedCurrencies)+1)
			for _, cur := range SupportedCurrencies {
				opts = append(opts, conversation.ChoiceOption{
					Label:    cur.String(),
					Value:    cur.String(),
					NextStep: StepAccountName,
				})
			}

			// "Listo" solo tiene sentido si ya hay al menos una cuenta
			// creada en este flujo — antes de eso, no hay nada que
			// terminar.
			if _, already := data[dataKeyAccountsCreated]; already {
				opts = append(opts, conversation.ChoiceOption{
					Label:  btnFinishAccountSetup,
					Value:  "done",
					Finish: true,
				})
			}
			return opts
		},
		DeclaredNextSteps: []string{StepAccountName},
		OnChoice: func(value string, data conversation.Data) conversation.Data {
			next := copyData(data)
			next[dataKeyCurrency] = value
			return next
		},
	}

	steps := map[string]conversation.Step{
		StepChooseCurrency: chooseCurrency,
		StepAccountName:    accountNameStep{repo: repo, creator: creator},
		StepConfirmDefault: newConfirmDefaultStep(creator),
	}

	return conversation.NewFlow(FlowName, StepChooseCurrency, steps)
}
