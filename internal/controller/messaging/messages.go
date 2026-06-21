package messaging

// Mensajes estáticos, sin variables.
const (
	msgAlreadyHasAccount = "Ya tenés una cuenta activa. Mandame un gasto para registrarlo."
	msgPrivateBot        = "Este bot es privado. Si tenés una invitación, abrí el link que te compartieron."
	msgInvalidInvitation = "Esa invitación no es válida."
	msgInvitationError   = "Hubo un error procesando tu invitación, probá de nuevo en un momento."
	msgInvitationUsed    = "Esa invitación ya fue utilizada."
	msgInvitationExpired = "Esa invitación expiró, pedí una nueva."
	msgUserCreationError = "No pude crear tu cuenta, probá de nuevo."

	msgUserNotFound         = "No encontramos tu cuenta."
	msgDefaultUpdateError   = "No se pudo actualizar la cuenta principal."
	msgReplyProcessingError = "Error procesando la respuesta."

	msgGenericFlowError = "Algo salió mal, probá de nuevo en un momento."

	msgAccountSetupFinished = "Listo, ya podés empezar a registrar tus gastos. " +
		"Mandame algo como \"café 500\" o usá /cuentas si querés agregar otra cuenta más adelante."

	msgSubcategorySetupFinished = "Listo, tus subcategorías están guardadas. " +
		"Usá /subcategorias cuando quieras agregar más."

	msgResumeAccountSetup = "Antes de continuar necesitamos terminar de configurar tu primera cuenta. " +
		"¿En qué moneda está?"

	btnAddAnotherAccount = "➕ Agregar otra cuenta"
)

// Mensajes con variables, armados como funciones.

func msgAskAccountName(currency string) string {
	return "¿Cómo querés llamar a esta cuenta en " + currency + "? (ej: Wallet, FCI, Ahorros)"
}

func msgConfirmReplaceDefault(currency, accountName string) string {
	return "Ya tenés una cuenta principal en " + currency + ". ¿Querés que \"" + accountName +
		"\" la reemplace como principal, o la agregamos como una cuenta más sin que cambie tu cuenta principal actual?"
}

func msgAccountCreated(name, currency string) string {
	return "Cuenta \"" + name + "\" en " + currency + " creada ✅\n\n" +
		"Podés agregar más cuentas si querés (por ejemplo, una inversión o una " +
		"segunda billetera), o terminar acá y ya podés empezar a registrar gastos."
}

func msgAccountAlreadyExists(name, currency string) string {
	return "Ya tenés una cuenta llamada \"" + name + "\" en " + currency + ". Probá con otro nombre."
}
