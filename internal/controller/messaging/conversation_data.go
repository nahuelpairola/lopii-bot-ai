package messaging

import "lopiibot.com/internal/account"

// accountCreationData es lo que persistimos en conversation_states.data
// mientras dura el flujo de alta de cuenta.
type accountCreationData struct {
	Currency account.Currency `json:"currency"`
}
