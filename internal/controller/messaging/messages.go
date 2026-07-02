package messaging

import (
	"fmt"
	"strings"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/user"
)

// Mensajes estáticos, sin variables.
const (
	msgAlreadyHasAccount           = "Ya tenés una cuenta activa. Mandame un gasto para registrarlo."
	msgPrivateBot                  = "Este bot es privado. Si tenés una invitación, abrí el link que te compartieron."
	msgInvalidInvitation           = "Esa invitación no es válida."
	msgInvitationError             = "Hubo un error procesando tu invitación, probá de nuevo en un momento."
	msgInvitationUsed              = "Esa invitación ya fue utilizada."
	msgInvitationExpired           = "Esa invitación expiró, pedí una nueva."
	msgUserCreationError           = "No pude crear tu cuenta, probá de nuevo."
	msgDefaultAccountCreationError = "No se pudieron crear las cuentas por defecto"
	msgUserCreatedSuccessfully     = "Bienvenid@ a LopiiBot, tus clasificador de gastos personales "

	msgGenericFlowError = "Algo salió mal, probá de nuevo en un momento."

	msgAccountSetupFinished = "Listo, ya podés empezar a registrar tus gastos. " +
		"Mandame algo como \"café 500\" o usá /cuentas si querés agregar otra cuenta más adelante."

	msgSubcategorySetupFinished = "Listo, tus subcategorías están guardadas. " +
		"Usá /subcategorias cuando quieras agregar más."
)

func createMsgUserDefaultAccountsCreatedSuccessfully(u *user.User, as []account.Account) string {
	accounts := make([]string, 0, len(as))

	for _, a := range as {
		accounts = append(accounts, fmt.Sprintf("%s (%s)", a.Name, a.Currency))
	}

	return fmt.Sprintf(
		"%s, disponés de %d cuentas: %s, donde se van a vincular todos tus movimientos por defecto.\n\n"+
			"Para arrancar con el saldo correcto, contame cuánta plata tenés hoy en cada una.",
		u.Username,
		len(as),
		strings.Join(accounts, ", "),
	)
}

func msgConfirmInitialBalances(arsAmount, usdAmount string) string {
	return fmt.Sprintf(
		"Así quedarían tus saldos iniciales:\n• Wallet ARS: $%s\n• Wallet USD: US$%s\n\n¿Confirmás o querés corregir?",
		arsAmount, usdAmount,
	)
}
