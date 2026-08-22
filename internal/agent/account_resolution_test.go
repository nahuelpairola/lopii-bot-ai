package agent

import (
	"testing"

	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
)

// accountsUser3 son las cuentas reales del usuario 3 el 2026-08-22, incluida la
// trampa: Banco Galicia (25) es la PRIMERA ARS por id, y la default es la 27.
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

// Dos cuentas con el mismo nombre y la misma moneda existen en producción. Elegir
// una es no determinista; preguntar es lo correcto.
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
