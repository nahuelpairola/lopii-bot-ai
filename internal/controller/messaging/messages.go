package messaging

import (
	"fmt"

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
	currencies := ""
	for _, c := range as {
		currencies = fmt.Sprintf("%s , ", currencies, c.Currency.String())
	}
	return fmt.Sprintf("%s, disponés de %d cuentas en %v que van a vincular tus movimientos.", u.Username, len(as), currencies)
}
