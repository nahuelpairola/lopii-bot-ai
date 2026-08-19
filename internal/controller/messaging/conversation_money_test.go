//go:build conv_test

package messaging

import (
	"strconv"
	"testing"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

// El modelo de plata, de punta a punta. Es el único lugar del código donde un
// error chico es un bug contable y no una molestia: no hay columna de saldo, el
// saldo ES la suma de los movimientos, así que un signo invertido no se nota
// hasta que alguien suma.
//
// Todo lo de acá corre sin gastar cupo de Groq: lo que se prueba es lo que pasa
// DESPUÉS de la tool call, que es donde vive el riesgo.

// Un ingreso se guarda POSITIVO — y lo decide LA APP, no el modelo.
//
// Por eso el modelo emite acá un monto NEGATIVO a propósito: si el test le
// pasara uno ya positivo, sólo probaría que la app no lo rompe, que es medir el
// fixture. Lo que se exige es que Normalize lo re-firme.
func TestMoney_IncomeIsStoredPositive(t *testing.T) {
	h := newConversationHarness(t)
	h.ScriptCategory(orchestrator.Pair{Category: "Ingresos", Subcategory: "Sueldo"})
	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"income","amount":"-500000","currency":"ARS",
		 "description":"Sueldo","date":"2026-08-12"}]}`))

	antes := h.Balance("Banco Test")
	h.SendText("Cobré 500000 de sueldo")

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1. Copia: %v", len(movs), h.Messages())
	}
	if !movs[0].Amount.Equal(mustDec(t, "500000")) {
		t.Errorf("amount = %s, want +500000: un ingreso NEGATIVO le resta al saldo", movs[0].Amount)
	}
	if movs[0].Type != movement.Income {
		t.Errorf("type = %s, want income", movs[0].Type)
	}
	if got := h.Balance("Banco Test").Sub(antes); !got.Equal(mustDec(t, "500000")) {
		t.Errorf("el saldo se movió %s, want +500000", got)
	}
}

// Una transferencia son DOS patas que se sostienen entre sí: mismo
// transaction_id y suma exactamente cero. Si una entra y la otra no, los dos
// saldos quedan mal y nada lo avisa.
func TestMoney_TransferHasTwoLegsSummingZero(t *testing.T) {
	h := newConversationHarness(t)
	destino := h.SeedAccount("Galicia Test", currency.ARS, "0")
	origen := h.accounts["Banco Test"]

	h.ScriptCategory(orchestrator.Pair{Category: "Sistema", Subcategory: "Transferencia"})
	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"transfer","amount":"-50000","currency":"ARS","account_id":` + itoa(origen) + `,
		 "description":"Transferencia","date":"2026-08-12","group":"t1"},
		{"type":"transfer","amount":"50000","currency":"ARS","account_id":` + itoa(destino) + `,
		 "description":"Transferencia","date":"2026-08-12","group":"t1"}]}`))

	h.SendText("Pasé 50000 del Banco Test a Galicia Test")

	movs := h.Movements()
	if len(movs) != 2 {
		t.Fatalf("patas = %d, want 2. Copia: %v", len(movs), h.Messages())
	}
	if movs[0].TransactionID == nil || movs[1].TransactionID == nil ||
		*movs[0].TransactionID != *movs[1].TransactionID {
		t.Fatalf("las dos patas tienen que compartir transaction_id: %v / %v",
			movs[0].TransactionID, movs[1].TransactionID)
	}
	if suma := movs[0].Amount.Add(movs[1].Amount); !suma.IsZero() {
		t.Errorf("las dos patas suman %s, want 0 — el invariante de transferencia", suma)
	}
	// Y los saldos se movieron en direcciones opuestas, por el mismo monto.
	if got := h.Balance("Galicia Test"); !got.Equal(mustDec(t, "50000")) {
		t.Errorf("destino = %s, want 50000", got)
	}
}

// ARS y USD son mundos separados: una compra de dólares mueve DOS cuentas en
// DOS monedas, y ningún total los mezcla.
func TestMoney_UsdAndArsNeverMix(t *testing.T) {
	h := newConversationHarness(t)
	usd := h.SeedAccount("Balala Test", currency.USD, "0")
	ars := h.accounts["Banco Test"]

	h.ScriptCategory(orchestrator.Pair{Category: "Inversiones", Subcategory: "Dólares"})
	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"transfer","amount":"-150000","currency":"ARS","account_id":` + itoa(ars) + `,
		 "description":"Compra USD","date":"2026-08-12","group":"u1"},
		{"type":"transfer","amount":"100","currency":"USD","account_id":` + itoa(usd) + `,
		 "description":"Compra USD","date":"2026-08-12","group":"u1"}]}`))

	h.SendText("Compré 100 dólares a 1500")

	movs := h.Movements()
	if len(movs) != 2 {
		t.Fatalf("patas = %d, want 2. Copia: %v", len(movs), h.Messages())
	}
	// Acá la suma NO da cero, y está bien: son monedas distintas. Lo que tiene
	// que dar es cada saldo por su lado.
	if got := h.Balance("Balala Test"); !got.Equal(mustDec(t, "100")) {
		t.Errorf("saldo USD = %s, want 100", got)
	}
	if got := h.Balance("Banco Test"); !got.Equal(mustDec(t, "850000")) {
		t.Errorf("saldo ARS = %s, want 850000 (1.000.000 − 150.000)", got)
	}
}

// Cancelar una corrección no puede escribir NADA. Sin este caso, un flujo que
// escribiera siempre pasaría el test de la rama que confirma.
func TestMoney_CancelledCorrectionWritesNothing(t *testing.T) {
	h := newConversationHarness(t)
	super := h.subID("Alimentación", "Supermercado")
	id := h.SeedMovementOn("Carrefour", "-12700", super, agent.StartOfTodayArgentina())
	antes := h.Balance("Banco Test")

	h.ScriptToolCalls(correctMovementCall(`{
		"change":"eran 99999",
		"changes":[{"field":"amount","op":"set","value":"99999"}]}`))
	h.SendText("el carrefour eran 99999")
	h.TapButton("cancel")

	movs := h.Movements()
	if len(movs) != 1 || movs[0].ID != id {
		t.Fatalf("cancelar cambió la base: %s", describeRows(movs))
	}
	if !movs[0].Amount.Equal(mustDec(t, "-12700")) {
		t.Errorf("amount = %s, want -12700: cancelar no puede escribir", movs[0].Amount)
	}
	if got := h.Balance("Banco Test"); !got.Equal(antes) {
		t.Errorf("el saldo se movió %s al cancelar", got.Sub(antes))
	}
}

// itoa: los account_id viajan en el JSON de la tool call, que es un string.
func itoa(id uint64) string { return strconv.FormatUint(id, 10) }

// Cambiar un movimiento de cuenta mueve DOS saldos: el de origen sube, el de
// destino baja. No hay columna de saldo — es la suma de los movimientos — así
// que si sólo se moviera uno, aparecería plata de la nada.
//
// Medido en vivo el 2026-08-12, donde falló por DOS bugs encadenados: la guarda
// de no-op no comparaba AccountNameGuess (cambiar de cuenta pone el nombre y
// VACÍA el id, y el vacío se leía como "no lo tocó"), y conversation.KeyPendingAccountGaps
// iba hardcodeado en nil — el mismo bug que ya había tenido la categoría.
func TestMoney_ChangingTheAccountMovesBothBalances(t *testing.T) {
	h := newConversationHarness(t)
	h.SeedAccount("Galicia Test", currency.ARS, "0")
	h.SeedMovementOn("Peaje", "-2500", h.subID("Transporte", "Peaje"), agent.StartOfTodayArgentina())

	origenAntes := h.Balance("Banco Test")
	destinoAntes := h.Balance("Galicia Test")

	h.ScriptToolCalls(correctMovementCall(`{
		"change":"el peaje ponelo en Galicia Test",
		"changes":[{"field":"account","value":"Galicia Test"}]}`))
	h.SendText("el peaje ponelo en Galicia Test")
	h.TapButton(flow.OptionConfirm)

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1: %s", len(movs), describeRows(movs))
	}
	if movs[0].AccountID == nil || *movs[0].AccountID != h.accounts["Galicia Test"] {
		t.Fatalf("el movimiento no se mudó de cuenta: %s", describeRows(movs))
	}
	// Y los dos saldos, que es lo que de verdad importa.
	if got := h.Balance("Banco Test").Sub(origenAntes); !got.Equal(mustDec(t, "2500")) {
		t.Errorf("el origen se movió %s, want +2500 (el gasto se fue)", got)
	}
	if got := h.Balance("Galicia Test").Sub(destinoAntes); !got.Equal(mustDec(t, "-2500")) {
		t.Errorf("el destino se movió %s, want -2500", got)
	}

}
