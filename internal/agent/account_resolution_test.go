package agent

import (
	"strings"
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
		Description: desc, Date: "2026-08-22",
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
		Description: "algo", Date: "2026-08-22",
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
			Description: "sueldo", Date: "2026-08-22",
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

func accountsUser2() []account.Account {
	return []account.Account{
		{Model: gorm.Model{ID: 43}, Name: "Mercado Pago", Currency: currency.ARS, IsDefault: true},
		{Model: gorm.Model{ID: 44}, Name: "Mercado Pago", Currency: currency.USD, IsDefault: true},
		{Model: gorm.Model{ID: 45}, Name: "FCI", Currency: currency.ARS},
		{Model: gorm.Model{ID: 46}, Name: "FCI", Currency: currency.USD},
		{Model: gorm.Model{ID: 47}, Name: "Inversión", Currency: currency.ARS},
		{Model: gorm.Model{ID: 48}, Name: "Efectivo", Currency: currency.USD},
	}
}

func transferDrafts(outID, inID *uint64, outCur, inCur, inGuess string) orchestrator.CreateResult {
	return orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "transfer", Amount: "-1000", Currency: outCur, AccountID: outID,
			Description: "transferencia", Date: "2026-10-29", Group: "t1"},
		{Type: "transfer", Amount: "1000", Currency: inCur, AccountID: inID, AccountNameGuess: inGuess,
			Description: "transferencia", Date: "2026-10-29", Group: "t1"},
	}}
}

func TestBuildCreateSeed_TransferLegAccountComesFromTheMessageOrTheDefault(t *testing.T) {
	tests := []struct {
		name     string
		userText string
		result   orchestrator.CreateResult
		accounts []account.Account
		wantOut  string
		wantIn   string
		wantGaps []string
	}{
		{
			name:     "compra de dólares sin cuenta nombrada: el modelo inventa FCI USD y cae en la default",
			userText: "Compra de 1129,48 usd por 1586 pesos cada dólar",
			result:   transferDrafts(acctID(43), acctID(46), "ARS", "USD", "FCI (USD)"),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "",
		},
		{
			name:     "otra compra de dólares real con FCI USD inventado",
			userText: "Hoy compré 20usd por 30900.",
			result:   transferDrafts(acctID(43), acctID(46), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "",
		},
		{
			name:     "compra de dólares en el mismo banco: la pierna ARS hermana de la USD default se respeta",
			userText: "El 05/10 compré 200 USD por $1545",
			result:   transferDrafts(acctID(25), acctID(26), "ARS", "USD", ""),
			accounts: accountsUser3(),
			wantOut:  "25", wantIn: "26",
		},
		{
			name:     "compra de dólares a las dos default: se respetan",
			userText: "Compra de 1129,48 usd por 1568 pesos cada dólar",
			result:   transferDrafts(acctID(43), acctID(44), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "44",
		},
		{
			name:     "FCI nombrado como palabra corta + default",
			userText: "Transferencia de 1129,48 usd a fci",
			result:   transferDrafts(acctID(44), acctID(46), "USD", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "44", wantIn: "46",
		},
		{
			name:     "las dos nombradas, con acento plegado",
			userText: "pasé 1000 de mercado pago a inversion",
			result:   transferDrafts(acctID(43), acctID(47), "ARS", "ARS", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "47",
		},
		{
			name:     "cuenta no default nombrada: se respeta",
			userText: "compré 100 usd en efectivo a 1500",
			result:   transferDrafts(acctID(43), acctID(48), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "48",
		},
		{
			name:     "cuenta no default sin nombrar: cae en la default",
			userText: "compré 100 usd a 1500",
			result:   transferDrafts(acctID(43), acctID(48), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "",
		},
		{
			name:     "id de otra moneda en la pierna USD: cae en la default",
			userText: "compré 100 usd a 1500",
			result:   transferDrafts(acctID(43), acctID(45), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "",
		},
		{
			name:     "id que no está entre las cuentas vivas: cae en la default",
			userText: "compré 100 usd a 1500",
			result:   transferDrafts(acctID(43), acctID(999), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "",
		},
		{
			name:     "la default de la pierna sin respaldo ya es la otra pierna: pregunta",
			userText: "pasé 1000 a mercado pago",
			result:   transferDrafts(acctID(47), acctID(43), "ARS", "ARS", ""),
			accounts: accountsUser2(),
			wantOut:  "", wantIn: "43", wantGaps: []string{"0"},
		},
		{
			name:     "sin default en la moneda: pregunta",
			userText: "compré 100 usd a 1500",
			result:   transferDrafts(acctID(43), acctID(46), "ARS", "USD", ""),
			accounts: withoutDefault(accountsUser2(), currency.USD),
			wantOut:  "43", wantIn: "", wantGaps: []string{"1"},
		},
		{
			name:     "palabra que contiene el nombre corto no lo respalda",
			userText: "pasé 1000 al fcil",
			result:   transferDrafts(acctID(43), acctID(45), "ARS", "ARS", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "", wantGaps: []string{"1"},
		},
		{
			name:     "compra de dólares nombrando la cuenta USD: le gana a la default que mandó el modelo",
			userText: "compré 100 usd en efectivo a 1500",
			result:   transferDrafts(acctID(43), acctID(44), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "48",
		},
		{
			name:     "un nombre que existe en las dos monedas no decide ninguna pierna",
			userText: "compré 100 usd y los dejo en el fci",
			result:   transferDrafts(acctID(43), acctID(44), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "44",
		},
		{
			name:     "compra de dólares nombrando la cuenta ARS de origen",
			userText: "compré 100 usd con plata de inversion",
			result:   transferDrafts(acctID(43), acctID(44), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "47", wantIn: "44",
		},
		{
			name:     "la cuenta nombrada sin id del modelo también gana",
			userText: "compré 100 usd en efectivo a 1500",
			result:   transferDrafts(acctID(43), nil, "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "48",
		},
		{
			name:     "dos cuentas de la misma moneda nombradas: no se elige ninguna por el mensaje",
			userText: "compré 100 usd con fci e inversion",
			result:   transferDrafts(acctID(43), acctID(44), "ARS", "USD", ""),
			accounts: accountsUser2(),
			wantOut:  "43", wantIn: "44",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildCreateSeed(tt.result, nil, tt.accounts, tt.userText)
			rows := movement.DecodeMovementRows(data)
			if rows[0].AccountID != tt.wantOut || rows[1].AccountID != tt.wantIn {
				t.Errorf("piernas = %q / %q, want %q / %q", rows[0].AccountID, rows[1].AccountID, tt.wantOut, tt.wantIn)
			}
			gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps)
			if strings.Join(gaps, ",") != strings.Join(tt.wantGaps, ",") {
				t.Errorf("gaps = %v, want %v", gaps, tt.wantGaps)
			}
		})
	}
}

func TestBuildCreateSeed_TransferKeepsModelAccountWhenMessageNamesIt(t *testing.T) {
	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "transfer", Amount: "-90000", Currency: "ARS", AccountID: acctID(28),
			Description: "transferencia", Date: "2026-08-22", Group: "t1"},
		{Type: "transfer", Amount: "90000", Currency: "ARS", AccountID: acctID(27),
			Description: "transferencia", Date: "2026-08-22", Group: "t1"},
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

func TestAccountBackedByMessage(t *testing.T) {
	fci := account.Account{Name: "FCI", Currency: currency.ARS}
	mp := account.Account{Name: "Mercado Pago", Currency: currency.ARS}
	inv := account.Account{Name: "Inversión", Currency: currency.ARS}
	tests := []struct {
		name string
		acc  account.Account
		msg  string
		want bool
	}{
		{"nombre corto como palabra entera", fci, "Transferencia de 1129,48 usd a fci", true},
		{"nombre corto pegado a puntuación", fci, "pasé 1000 al FCI.", true},
		{"nombre corto dentro de otra palabra no cuenta", fci, "pasé 1000 al fcil", false},
		{"nombre de dos palabras escrito junto", mp, "de fci a mercadopago", true},
		{"mayúsculas se pliegan", mp, "PASÉ 1000 A MERCADO PAGO", true},
		{"pago de monotributo NO es Mercado Pago", mp, "pago de monotributo 5000", false},
		{"acentos se pliegan", inv, "pasé 1000 a inversion", true},
		{"el mensaje no la nombra", fci, "Compra de 1129,48 usd por 1586 pesos cada dólar", false},
		{"mensaje vacío", fci, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accountBackedByMessage(tt.acc, tt.msg); got != tt.want {
				t.Errorf("accountBackedByMessage(%q, %q) = %v, want %v", tt.acc.Name, tt.msg, got, tt.want)
			}
		})
	}
}

func withoutDefault(accounts []account.Account, cur currency.Currency) []account.Account {
	for i := range accounts {
		if accounts[i].Currency == cur {
			accounts[i].IsDefault = false
		}
	}
	return accounts
}

func TestBuildCreateSeed_SameInstitutionPairNeedsOneTrustedLeg(t *testing.T) {
	data := buildCreateSeed(transferDrafts(acctID(45), acctID(46), "ARS", "USD", ""), nil, accountsUser2(), "Compra de 100 usd por 1500 pesos cada dólar")
	rows := movement.DecodeMovementRows(data)
	if rows[0].AccountID != "" || rows[1].AccountID != "" {
		t.Errorf("piernas = %q / %q, want las dos a la default: un par FCI ARS → FCI USD sin ancla es justo el inventado", rows[0].AccountID, rows[1].AccountID)
	}
}
