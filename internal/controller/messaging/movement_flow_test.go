package messaging

import (
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
)

func TestEncodeDecodeMovementRows_RoundTrip(t *testing.T) {
	rows := []movementRow{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"},
		{Type: "transfer", Amount: "100", Currency: "USD", AccountID: "5"},
	}

	data := conversation.Data{"movements": encodeMovementRows(rows)}
	got := decodeMovementRows(data)

	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Amount != "3000" || got[0].Category != "Alimentación" {
		t.Errorf("row 0 = %+v", got[0])
	}
	if got[1].AccountID != "5" {
		t.Errorf("row 1 account id = %q, want 5", got[1].AccountID)
	}
}

func TestEncodeDecodeStringSlice_RoundTrip(t *testing.T) {
	data := conversation.Data{"gaps": encodeStringSlice([]string{"0", "2"})}
	got := decodeStringSlice(data, "gaps")

	if len(got) != 2 || got[0] != "0" || got[1] != "2" {
		t.Errorf("got %v, want [0 2]", got)
	}
}

func TestDecodeStringSlice_MissingKey_ReturnsEmpty(t *testing.T) {
	got := decodeStringSlice(conversation.Data{}, "missing")
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestBuildCreateSeed_QueuesCategoryAndAccountGaps(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "100000", Currency: "ARS", Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW"},
			{Type: "transfer", Amount: "100", Currency: "USD", AccountNameGuess: "FCI", Category: "Inversiones", Subcategory: "FCI"},
		},
	}

	data := buildCreateSeed(result, nil)

	categoryGaps := decodeStringSlice(data, "pending_category_gaps")
	if len(categoryGaps) != 1 || categoryGaps[0] != "0" {
		t.Errorf("category gaps = %v, want [0]", categoryGaps)
	}
	accountGaps := decodeStringSlice(data, "pending_account_gaps")
	if len(accountGaps) != 1 || accountGaps[0] != "1" {
		t.Errorf("account gaps = %v, want [1]", accountGaps)
	}
	if data["mode"] != "create" {
		t.Errorf("mode = %v, want create", data["mode"])
	}
}

func TestBuildCreateSeed_NoGaps_EmptyQueues(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"},
		},
	}

	data := buildCreateSeed(result, nil)

	if len(decodeStringSlice(data, "pending_category_gaps")) != 0 {
		t.Error("expected no category gaps")
	}
	if len(decodeStringSlice(data, "pending_account_gaps")) != 0 {
		t.Error("expected no account gaps")
	}
}

// TestBuildCreateSeed_UnknownCategoryPair_QueuesGap cubre la causa raíz de los
// create_failed observados en intent_events ("Carne 10 mil" falló dos veces): el
// modelo devuelve un par que NO existe en la taxonomía del usuario en vez de
// PENDING_REVIEW. Sin gap, el flujo va derecho a insertar y
// FindByCategoryAndSubcategory falla — el movimiento se pierde y el usuario ve un
// error genérico. Un par desconocido tiene que caer al gap-fill, igual que
// PENDING_REVIEW.
func TestBuildCreateSeed_UnknownCategoryPair_QueuesGap(t *testing.T) {
	taxonomy := []orchestrator.TaxonomyEntry{
		{Category: "Alimentación", Subcategory: "Café"},
		{Category: "Supermercado", Subcategory: "Almacén"},
	}
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "10000", Currency: "ARS", Category: "Comida", Subcategory: "Carnicería"},
		},
	}

	data := buildCreateSeed(result, taxonomy)

	gaps := decodeStringSlice(data, "pending_category_gaps")
	if len(gaps) != 1 || gaps[0] != "0" {
		t.Errorf("category gaps = %v, want [0]: un par inexistente debe caer al gap-fill, no al insert", gaps)
	}
}

// TestBuildCreateSeed_KnownCategoryPair_NoGap es la guarda del riesgo inverso: la
// validación no debe abrir gaps donde hoy se inserta bien. El delta del fix tiene
// que ser estrictamente error → pregunta, nunca inserto-OK → pregunta.
func TestBuildCreateSeed_KnownCategoryPair_NoGap(t *testing.T) {
	taxonomy := []orchestrator.TaxonomyEntry{
		{Category: "Alimentación", Subcategory: "Café"},
	}
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"},
		},
	}

	data := buildCreateSeed(result, taxonomy)

	if gaps := decodeStringSlice(data, "pending_category_gaps"); len(gaps) != 0 {
		t.Errorf("category gaps = %v, want []: un par que existe no debe preguntar nada", gaps)
	}
}

// TestBuildCreateSeed_NonTransferWithUnmatchedAccountName_QueuesAccountGap: el
// modelo setea AccountNameGuess cuando el mensaje nombró una cuenta que no pudo
// matchear con ninguna existente. Sin gap, la fila cae en la cuenta default de la
// moneda y la cuenta nombrada nunca se crea: la intención del usuario se descarta.
func TestBuildCreateSeed_NonTransferWithUnmatchedAccountName_QueuesAccountGap(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{
				Type: "expense", Amount: "20000", Currency: "ARS",
				AccountNameGuess: "Brubank",
				Category:         "Comida", Subcategory: "Restaurante",
			},
		},
	}

	data := buildCreateSeed(result, nil)

	accountGaps := decodeStringSlice(data, "pending_account_gaps")
	if len(accountGaps) != 1 || accountGaps[0] != "0" {
		t.Errorf("account gaps = %v, want [0]", accountGaps)
	}
}

// TestBuildCreateSeed_NonTransferWithoutAccountName_NoAccountGap es la guarda del
// riesgo inverso: sin cuenta nombrada no hay pregunta. Es el camino de la mayoría
// de los CREATE que hoy funcionan y no puede ganar un prompt.
func TestBuildCreateSeed_NonTransferWithoutAccountName_NoAccountGap(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{
				Type: "expense", Amount: "20000", Currency: "ARS",
				Category: "Comida", Subcategory: "Restaurante",
			},
		},
	}

	data := buildCreateSeed(result, nil)

	if gaps := decodeStringSlice(data, "pending_account_gaps"); len(gaps) != 0 {
		t.Errorf("account gaps = %v, want []: sin cuenta nombrada no se pregunta nada", gaps)
	}
}

// TestBuildCreateSeed_CounterpartyInDescription_NoAccountGap: "pizza con Pablo"
// deja AccountNameGuess en "Pablo" y a Pablo DENTRO de la description — Pablo es
// la contraparte, no una cuenta. Ese gasto hoy se guarda solo contra la default
// y no puede ganar una pregunta, ni terminar creando una cuenta que se llama
// como una persona (la fila que ya fija
// TestResolveAndInsert_ExpenseNeverCreatesCounterpartyAccount).
func TestBuildCreateSeed_CounterpartyInDescription_NoAccountGap(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{
				Type: "expense", Amount: "100000", Currency: "ARS",
				AccountNameGuess: "Pablo", Description: "pizza con Pablo",
				Category: "Ocio y salidas", Subcategory: "Restaurante",
			},
		},
	}

	data := buildCreateSeed(result, nil)

	if gaps := decodeStringSlice(data, "pending_account_gaps"); len(gaps) != 0 {
		t.Errorf("account gaps = %v, want []: el nombre de la contraparte no es una cuenta", gaps)
	}
}

// La otra dirección: el nombre NO está en la description, así que es una cuenta
// del usuario y tiene que abrir gap. Sin este caso la regla queda medio
// verificada — y es la mitad que puede CREAR una cuenta.
func TestBuildCreateSeed_OwnAccountNotInDescription_OpensGap(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{
				Type: "expense", Amount: "35000", Currency: "ARS",
				AccountNameGuess: "Brubank", Description: "curso de ingles",
				Category: "Educación", Subcategory: "Cursos",
			},
		},
	}

	data := buildCreateSeed(result, nil)

	if gaps := decodeStringSlice(data, "pending_account_gaps"); len(gaps) != 1 {
		t.Errorf("account gaps = %v, want 1: 'pagué el curso con Brubank' tiene que preguntar", gaps)
	}
}

func TestParseUintSlice(t *testing.T) {
	got, err := parseUintSlice([]string{"1", "2", "30"})
	if err != nil {
		t.Fatalf("parseUintSlice: %v", err)
	}
	want := []uint{1, 2, 30}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestParseUintSlice_ErrorsOnGarbage(t *testing.T) {
	if _, err := parseUintSlice([]string{"not-a-number"}); err == nil {
		t.Fatal("expected an error for a non-numeric id")
	}
}

func TestMovementRow_GroupRoundTrip(t *testing.T) {
	rows := []movementRow{{Type: "transfer", Amount: "100", Currency: "ARS", Group: "g1"}}
	data := conversation.Data{"movements": encodeMovementRows(rows)}
	got := decodeMovementRows(data)
	if len(got) != 1 || got[0].Group != "g1" {
		t.Fatalf("group round trip = %+v, want Group=g1", got)
	}
}

// TestCopyDataNeverReturnsNil — mismo invariante que conversation.cloneData:
// los ~36 call sites escriben sobre la copia, así que devolver nil paniquea.
func TestCopyDataNeverReturnsNil(t *testing.T) {
	got := copyData(nil)
	if got == nil {
		t.Fatal("copyData(nil) devolvió nil: el próximo write va a paniquear")
	}
	got[keyCancelled] = "true" // no debe paniquear
}
