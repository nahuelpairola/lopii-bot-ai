package account

const (
	msgInvalidAccountName   = "Mandame un nombre válido para la cuenta."
	msgAccountCreationError = "No se pudo crear la cuenta, probá de nuevo."

	// El ejemplo usa coma decimal a propósito: es la convención argentina que
	// valida parseARAmount. "sin letras" es la parte que enseña por qué se
	// rechaza "30k". También cubre el caso de monto negativo (ver spec §3.2).
	MsgInvalidAmount = "Mandame solo el monto, sin letras (ej: 1500 o 1500,50)."

	btnReplaceYes         = "Sí, reemplazar"
	btnReplaceNo          = "No, agregar como extra"
	btnFinishAccountSetup = "✅ Listo"

	msgWelcomeAccountsIntro = "Bienvenido! Vamos a configurar tus cuentas.\n\n" +
		"Una cuenta es donde guardás tu plata: tu billetera, una cuenta bancaria, " +
		"una inversión, etc. Vas a necesitar al menos una por cada moneda que uses.\n\n" +
		"La primera cuenta que crees en cada moneda va a ser tu cuenta principal: " +
		"es la que se usa automáticamente cuando registrás un gasto sin indicar " +
		"ninguna otra. Después podés agregar más cuentas en la misma moneda " +
		"(por ejemplo, una inversión aparte) sin que afecte cuál es la principal.\n\n" +
		"Empecemos: ¿en qué moneda está tu primera cuenta?"

	msgAskAddAnotherAccount = "Cuenta creada ✅\n\n" +
		"¿Querés agregar otra? Podés sumar más cuentas en la misma moneda " +
		"(por ejemplo, una inversión) o en otra distinta. Si ya terminaste, tocá \"Listo\"."
)

func msgAskAccountName(currency string) string {
	return "¿Cómo querés llamar a esta cuenta en " + currency + "? (ej: Wallet, FCI, Ahorros)"
}

func msgConfirmReplaceDefault(currency, accountName string) string {
	return "Ya tenés una cuenta principal en " + currency + ". ¿Querés que \"" + accountName +
		"\" la reemplace como principal, o la agregamos como una cuenta más sin que cambie tu cuenta principal actual?"
}

func MsgAskInitialBalance(accountName, currency string) string {
	return "¿Cuánto tenés hoy en \"" + accountName + "\" (" + currency + ")? Tirame el número (ej: 50000)."
}

func MsgAccountAlreadyExists(name, currency string) string {
	return "Ya tenés una cuenta llamada \"" + name + "\" en " + currency + ". Probá con otro nombre."
}
