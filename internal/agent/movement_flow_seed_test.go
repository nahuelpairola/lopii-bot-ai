package agent

import (
	"testing"

	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

func TestBuildCreateSeed_QueuesCategoryAndAccountGaps(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "100000", Currency: "ARS", Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW"},
			{Type: "transfer", Amount: "100", Currency: "USD", AccountNameGuess: "FCI", Category: "Inversiones", Subcategory: "FCI"},
		},
	}

	data := buildCreateSeed(result, nil, nil)

	categoryGaps := conversation.DecodeStringSlice(data, "pending_category_gaps")
	if len(categoryGaps) != 1 || categoryGaps[0] != "0" {
		t.Errorf("category gaps = %v, want [0]", categoryGaps)
	}
	accountGaps := conversation.DecodeStringSlice(data, "pending_account_gaps")
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

	data := buildCreateSeed(result, nil, nil)

	if len(conversation.DecodeStringSlice(data, "pending_category_gaps")) != 0 {
		t.Error("expected no category gaps")
	}
	if len(conversation.DecodeStringSlice(data, "pending_account_gaps")) != 0 {
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

	data := buildCreateSeed(result, taxonomy, nil)

	gaps := conversation.DecodeStringSlice(data, "pending_category_gaps")
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

	data := buildCreateSeed(result, taxonomy, nil)

	if gaps := conversation.DecodeStringSlice(data, "pending_category_gaps"); len(gaps) != 0 {
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

	data := buildCreateSeed(result, nil, nil)

	accountGaps := conversation.DecodeStringSlice(data, "pending_account_gaps")
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

	data := buildCreateSeed(result, nil, nil)

	if gaps := conversation.DecodeStringSlice(data, "pending_account_gaps"); len(gaps) != 0 {
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

	data := buildCreateSeed(result, nil, nil)

	if gaps := conversation.DecodeStringSlice(data, "pending_account_gaps"); len(gaps) != 0 {
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

	data := buildCreateSeed(result, nil, nil)

	if gaps := conversation.DecodeStringSlice(data, "pending_account_gaps"); len(gaps) != 1 {
		t.Errorf("account gaps = %v, want 1: 'pagué el curso con Brubank' tiene que preguntar", gaps)
	}
}

// categoryGapsFor es la paridad que a UPDATE le faltaba: hasta el 2026-08-12
// seedAndStartUpdateConfirm escribía conversation.KeyPendingCategoryGaps en nil hardcodeado,
// así que una corrección que nombraba una categoría inexistente no marcaba gap,
// insertaba derecho y FindByCategoryAndSubcategory fallaba — el movimiento se
// perdía con un error genérico.
func TestCategoryGapsFor(t *testing.T) {
	taxonomy := []orchestrator.TaxonomyEntry{
		{Category: "Alimentación", Subcategory: "Supermercado"},
		{Category: "Ocio y salidas", Subcategory: "Salir a comer"},
	}

	cases := []struct {
		name     string
		rows     []movement.MovementRow
		taxonomy []orchestrator.TaxonomyEntry
		want     []string
	}{
		{"par conocido no abre gap",
			[]movement.MovementRow{{Category: "Alimentación", Subcategory: "Supermercado"}}, taxonomy, nil},
		{"par inventado abre gap",
			[]movement.MovementRow{{Category: "proyecto hogar", Subcategory: "agua"}}, taxonomy, []string{"0"}},
		{"categoría real con subcategoría inventada abre gap",
			[]movement.MovementRow{{Category: "Alimentación", Subcategory: "no existe"}}, taxonomy, []string{"0"}},
		{"PENDING_REVIEW abre gap",
			[]movement.MovementRow{{Category: constants.PendingReview, Subcategory: ""}}, taxonomy, []string{"0"}},
		{"sólo las filas malas, con su índice",
			[]movement.MovementRow{
				{Category: "Alimentación", Subcategory: "Supermercado"},
				{Category: "inventada", Subcategory: "x"},
				{Category: "Ocio y salidas", Subcategory: "Salir a comer"},
				{Category: "otra inventada", Subcategory: "y"},
			}, taxonomy, []string{"1", "3"}},
		// Sin con qué comparar no se inventan gaps: preguntar por TODO sería peor
		// que no validar.
		{"taxonomía vacía no valida",
			[]movement.MovementRow{{Category: "cualquier cosa", Subcategory: "x"}}, nil, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := categoryGapsFor(tc.rows, tc.taxonomy)
			if len(got) != len(tc.want) {
				t.Fatalf("gaps = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("gaps = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// Nombrar una cuenta que EXISTE no puede abrir un picker: el usuario ya dijo
// cuál. Medido en vivo el 2026-08-12 con "Lote cemento 45000", donde el modelo
// mandó account_name_guess="Mercado Pago" —la cuenta default, que existe— y el
// bot preguntó igual a cuál iba.
func TestBuildCreateSeed_NamedAccountThatExistsResolvesWithoutAsking(t *testing.T) {
	accounts := []account.Account{
		{Model: gorm.Model{ID: 46}, Name: "Mercado Pago", Currency: currency.ARS},
		{Model: gorm.Model{ID: 51}, Name: "Balala", Currency: currency.USD},
	}
	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{{
		Type: "expense", Amount: "45000", Currency: "ARS",
		AccountNameGuess: "mercado pago", // sin mayúsculas, como lo escribiría el usuario
		Category:         "Alimentación", Subcategory: "Supermercado",
		Description: "Lote cemento", Date: "2026-08-12",
	}}}

	data := buildCreateSeed(result, nil, accounts)

	if gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps); len(gaps) != 0 {
		t.Fatalf("se abrió un gap de cuenta pese a que la nombró: %v", gaps)
	}
	rows := movement.DecodeMovementRows(data)
	if rows[0].AccountID != "46" {
		t.Errorf("account_id = %q, want 46", rows[0].AccountID)
	}
}

// Y la dirección contraria, que es la que protege la plata: un nombre que NO es
// exactamente una cuenta del usuario NO se adivina. "Galicia" contra "banco
// galicia" cae al gap y pregunta.
func TestBuildCreateSeed_PartialAccountNameStillAsks(t *testing.T) {
	accounts := []account.Account{{Model: gorm.Model{ID: 72}, Name: "banco galicia", Currency: currency.ARS}}
	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{{
		Type: "expense", Amount: "45000", Currency: "ARS",
		AccountNameGuess: "Galicia",
		Description:      "Lote cemento", Date: "2026-08-12",
	}}}

	data := buildCreateSeed(result, nil, accounts)

	if gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps); len(gaps) != 1 {
		t.Errorf("un nombre parcial tiene que preguntar, gaps = %v", gaps)
	}
}

// Dos cuentas con el mismo nombre en monedas distintas: la moneda desempata. Sin
// eso, un gasto en pesos podría caer en la cuenta en dólares.
func TestBuildCreateSeed_SameNameDifferentCurrencyPicksByCurrency(t *testing.T) {
	accounts := []account.Account{
		{Model: gorm.Model{ID: 10}, Name: "Brubank", Currency: currency.ARS},
		{Model: gorm.Model{ID: 11}, Name: "Brubank", Currency: currency.USD},
	}
	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{{
		Type: "expense", Amount: "100", Currency: "USD",
		AccountNameGuess: "Brubank",
		Description:      "algo", Date: "2026-08-12",
	}}}

	rows := movement.DecodeMovementRows(buildCreateSeed(result, nil, accounts))
	if rows[0].AccountID != "11" {
		t.Errorf("account_id = %q, want 11 (la de USD)", rows[0].AccountID)
	}
}
