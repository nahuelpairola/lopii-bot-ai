package messaging

import "lopiibot.com/internal/flow"

// createErrorCopy/guardReason viven en flow (movement_label.go). El alias
// conserva el nombre corto para el finish de cuentas (account_manage_finish.go),
// que se migra en un commit posterior.
func createErrorCopy(err error) string {
	return flow.CreateErrorCopy(err)
}

func guardReason(err error) string {
	return flow.GuardReason(err)
}
