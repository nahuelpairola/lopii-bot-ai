package agent

import (
	"testing"

	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

func accountsUser3() []account.Account {
	return []account.Account{
		{Model: gorm.Model{ID: 25}, Name: "Banco Galicia", Currency: currency.ARS},
		{Model: gorm.Model{ID: 26}, Name: "Banco Galicia", Currency: currency.USD, IsDefault: true},
		{Model: gorm.Model{ID: 27}, Name: "Mercado Pago", Currency: currency.ARS, IsDefault: true},
		{Model: gorm.Model{ID: 28}, Name: "FCI", Currency: currency.ARS},
		{Model: gorm.Model{ID: 29}, Name: "Cedears", Currency: currency.ARS},
	}
}

func TestAccountNamedInMessage(t *testing.T) {
	tests := []struct {
		name          string
		msg           string
		cur           string
		wantID        uint64
		wantAmbiguous bool
	}{
		{"el mensaje no nombra ninguna", "$40000 kinesiologa el 19 de agosto", "ARS", 0, false},
		{"pago de monotributo NO es Mercado Pago", "$5585,77 pago de monotributo el día 20 de agosto", "ARS", 0, false},
		{"nombre completo resuelve", "pagué 5000 de luz con mercado pago", "ARS", 27, false},
		{"acentos y mayúsculas se pliegan", "PAGUÉ 5000 CON MERCADO PAGO", "ARS", 27, false},
		{"nombre parcial no resuelve", "pagué 5000 de luz con Galicia", "ARS", 0, false},
		{"la moneda desempata el mismo nombre", "gasté 100 usd con banco galicia", "USD", 26, false},
		{"nombre de menos de 4 runas no resuelve", "saqué 5000 del fci", "ARS", 0, false},
		{"mensaje vacío", "", "ARS", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ambiguous := accountNamedInMessage(tt.msg, accountsUser3(), tt.cur)
			if id != tt.wantID || ambiguous != tt.wantAmbiguous {
				t.Errorf("accountNamedInMessage(%q) = (%d, %v), want (%d, %v)",
					tt.msg, id, ambiguous, tt.wantID, tt.wantAmbiguous)
			}
		})
	}
}

func TestAccountNamedInMessage_DuplicateNameIsAmbiguous(t *testing.T) {
	accounts := []account.Account{
		{Model: gorm.Model{ID: 27}, Name: "Mercado Pago", Currency: currency.ARS},
		{Model: gorm.Model{ID: 33}, Name: "Mercado Pago", Currency: currency.ARS},
	}
	id, ambiguous := accountNamedInMessage("pagué con mercado pago", accounts, "ARS")
	if id != 0 || !ambiguous {
		t.Errorf("= (%d, %v), want (0, true)", id, ambiguous)
	}
}

func TestGuessBackedByMessage(t *testing.T) {
	tests := []struct {
		name  string
		guess string
		msg   string
		want  bool
	}{
		{"guess vacío", "", "pagué 5000 de luz", false},
		{"el guess copiado del prompt no está en el mensaje", "Banco Galicia (ARS)", "$5585,77 pago de monotributo el día 20 de agosto", false},
		{"la default alucinada tampoco", "Mercado Pago", "3700 cerveza viernes a la noche", false},
		{"una cuenta que el usuario sí nombró", "Brubank", "pagué el curso con Brubank", true},
		{"nombre parcial que el usuario escribió", "Galicia", "pagué 5000 de luz con Galicia", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := guessBackedByMessage(tt.guess, tt.msg); got != tt.want {
				t.Errorf("guessBackedByMessage(%q, %q) = %v, want %v", tt.guess, tt.msg, got, tt.want)
			}
		})
	}
}

func draftARS(desc string, accountID *uint64, guess string) orchestrator.MovementDraft {
	return orchestrator.MovementDraft{
		Type: "expense", Amount: "5000", Currency: "ARS",
		AccountID: accountID, AccountNameGuess: guess,
		PaymentMethod: "transfer", Description: desc, Date: "2026-08-22",
	}
}

func acctID(v uint64) *uint64 { return &v }

func TestBuildCreateSeed_AccountComesFromTheMessage(t *testing.T) {
	tests := []struct {
		name     string
		userText string
		draft    orchestrator.MovementDraft
		wantAcct string
		wantGap  bool
	}{
		{
			name:     "el modelo inventa la cuenta y el mensaje no la nombra",
			userText: "$40000 kinesiologa el 19 de agosto",
			draft:    draftARS("kinesiologa", acctID(25), ""),
			wantAcct: "", wantGap: false,
		},
		{
			name:     "guess copiado del bloque de cuentas del prompt",
			userText: "$5585,77 pago de monotributo el día 20 de agosto",
			draft:    draftARS("pago de monotributo", acctID(25), "Banco Galicia (ARS)"),
			wantAcct: "", wantGap: false,
		},
		{
			name:     "el modelo no manda nada",
			userText: "$22500 arreglo plomero",
			draft:    draftARS("arreglo plomero", nil, ""),
			wantAcct: "", wantGap: false,
		},
		{
			name:     "guess alucinado con el nombre de la default",
			userText: "3700 cerveza viernes a la noche",
			draft:    draftARS("cerveza", acctID(27), "Mercado Pago"),
			wantAcct: "", wantGap: false,
		},
		{
			name:     "el usuario nombra la cuenta",
			userText: "pagué 5000 de luz con mercado pago",
			draft:    draftARS("luz", nil, ""),
			wantAcct: "27", wantGap: false,
		},
		{
			name:     "el usuario nombra una y el modelo manda otra",
			userText: "pagué 5000 de luz con mercado pago",
			draft:    draftARS("luz", acctID(25), "Banco Galicia"),
			wantAcct: "27", wantGap: false,
		},
		{
			name:     "nombre parcial: pregunta",
			userText: "pagué 5000 de luz con Galicia",
			draft:    draftARS("luz", nil, "Galicia"),
			wantAcct: "", wantGap: true,
		},
		{
			name:     "cuenta que todavía no existe: pregunta y ofrece crearla",
			userText: "pagué el curso con Brubank",
			draft:    draftARS("curso", nil, "Brubank"),
			wantAcct: "", wantGap: true,
		},
		{
			name:     "una contraparte no es una cuenta",
			userText: "pizza con Pablo 8000",
			draft:    draftARS("pizza con Pablo", nil, "Pablo"),
			wantAcct: "", wantGap: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{tt.draft}}
			data := buildCreateSeed(result, nil, accountsUser3(), tt.userText)

			rows := movement.DecodeMovementRows(data)
			if rows[0].AccountID != tt.wantAcct {
				t.Errorf("account_id = %q, want %q", rows[0].AccountID, tt.wantAcct)
			}
			gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps)
			if got := len(gaps) > 0; got != tt.wantGap {
				t.Errorf("gap de cuenta = %v, want %v (gaps=%v)", got, tt.wantGap, gaps)
			}
		})
	}
}

func TestBuildCreateSeed_MessageNamedAccountRespectsCurrency(t *testing.T) {
	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{{
		Type: "expense", Amount: "100", Currency: "USD",
		PaymentMethod: "transfer", Description: "algo", Date: "2026-08-22",
	}}}
	rows := movement.DecodeMovementRows(
		buildCreateSeed(result, nil, accountsUser3(), "gasté 100 usd con banco galicia"))
	if rows[0].AccountID != "26" {
		t.Errorf("account_id = %q, want 26 (la de USD)", rows[0].AccountID)
	}
}

func TestBuildCreateSeed_IncomeResolvesLikeExpense(t *testing.T) {
	income := func(guess string, accountID *uint64) orchestrator.CreateResult {
		return orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{{
			Type: "income", Amount: "300000", Currency: "ARS",
			AccountID: accountID, AccountNameGuess: guess,
			PaymentMethod: "transfer", Description: "sueldo", Date: "2026-08-22",
		}}}
	}

	t.Run("sin cuenta nombrada cae en la default", func(t *testing.T) {
		rows := movement.DecodeMovementRows(
			buildCreateSeed(income("Banco Galicia", acctID(25)), nil, accountsUser3(), "me entraron 300 mil de sueldo"))
		if rows[0].AccountID != "" {
			t.Errorf("account_id = %q, want vacío: el id del modelo no vale en un income", rows[0].AccountID)
		}
	})

	t.Run("con cuenta nombrada va a esa", func(t *testing.T) {
		rows := movement.DecodeMovementRows(
			buildCreateSeed(income("", nil), nil, accountsUser3(), "me entraron 300 mil de sueldo al banco galicia"))
		if rows[0].AccountID != "25" {
			t.Errorf("account_id = %q, want 25", rows[0].AccountID)
		}
	})
}

func TestBuildCreateSeed_OneNamedAccountAppliesToEveryRowOfTheTurn(t *testing.T) {
	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		draftARS("luz", nil, ""),
		draftARS("gas", nil, ""),
	}}
	rows := movement.DecodeMovementRows(
		buildCreateSeed(result, nil, accountsUser3(), "pagué luz 5000 y gas 3000 con mercado pago"))
	if rows[0].AccountID != "27" || rows[1].AccountID != "27" {
		t.Errorf("account_id = %q y %q, want 27 las dos", rows[0].AccountID, rows[1].AccountID)
	}
}

func TestBuildCreateSeed_TransferStillUsesModelAccountID(t *testing.T) {
	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "transfer", Amount: "-90000", Currency: "ARS", AccountID: acctID(28),
			PaymentMethod: "transfer", Description: "transferencia", Date: "2026-08-22", Group: "t1"},
		{Type: "transfer", Amount: "90000", Currency: "ARS", AccountID: acctID(27),
			PaymentMethod: "transfer", Description: "transferencia", Date: "2026-08-22", Group: "t1"},
	}}
	data := buildCreateSeed(result, nil, accountsUser3(), "Tranferencia de 90 mil de fci a mercadopago")

	rows := movement.DecodeMovementRows(data)
	if rows[0].AccountID != "28" || rows[1].AccountID != "27" {
		t.Errorf("las piernas perdieron su cuenta: %q y %q, want 28 y 27",
			rows[0].AccountID, rows[1].AccountID)
	}
	if gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps); len(gaps) != 0 {
		t.Errorf("un transfer con las dos cuentas resueltas no pregunta, gaps = %v", gaps)
	}
}
