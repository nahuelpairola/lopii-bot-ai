package orchestrator

import "strings"

// Intent is the router's classification of a free-text message.
type Intent string

const (
	IntentCreate         Intent = "CREATE"
	IntentUpdate         Intent = "UPDATE"
	IntentDelete         Intent = "DELETE"
	IntentQuery          Intent = "QUERY"
	IntentAccountManage  Intent = "ACCOUNT_MANAGE"
	IntentCreateCategory Intent = "CREATE_CATEGORY"
	IntentCategoryManage Intent = "CATEGORY_MANAGE"
	IntentReminderSet    Intent = "REMINDER_SET"
	IntentHelp           Intent = "HELP"
	IntentUnclear        Intent = "UNCLEAR"

	// IntentQueued: el turno se topó con el cupo ANTES de que el modelo eligiera
	// ninguna herramienta, así que el intent todavía no se sabe. No es UNCLEAR —
	// eso significa "no te entendí", y acá no llegamos ni a intentarlo.
	//
	// Es transitorio: el drenaje lo pisa con el intent real cuando el replay
	// funciona. Una fila que quede en QUEUED es un mensaje que nunca se pudo
	// procesar, y ESO es justamente lo que hay que poder contar.
	IntentQueued Intent = "QUEUED"
)

// IntentResult is Call 1 router's output: the classified intent.
type IntentResult struct {
	Intent Intent `json:"intent"`
}

// TaxonomyEntry is one category/subcategory row, fed to Call 2 CREATE
// as a classification hint — the description field carries the
// disambiguation notes already written into the seeded taxonomy (e.g.
// "NO incluye compras específicas como carnicería").
type TaxonomyEntry struct {
	Category    string
	Subcategory string
	Description string
}

// AccountOption is one of the user's existing accounts, fed to Call 2
// CREATE so it can match a transfer's account by name instead of
// guessing.
type AccountOption struct {
	ID       uint64
	Name     string
	Currency string
}

// MovementDraft is what Call 2 CREATE/UPDATE returns per movement row.
// Amount and Date are strings — LLM JSON output is never trusted as a
// native numeric/date type; the caller parses these into
// decimal.Decimal/time.Time.
//
// AccountID is what the LLM CLAIMS the account is — it is not a match
// against anything, and on non-transfer rows the app ignores it outright
// (see buildCreateSeed). AccountNameGuess is likewise a claim, honored
// only when the user's message backs it up.
type MovementDraft struct {
	Type             string  `json:"type"`
	Amount           string  `json:"amount"`
	Currency         string  `json:"currency"`
	AccountID        *uint64 `json:"account_id,omitempty"`
	AccountNameGuess string  `json:"account_name_guess,omitempty"`
	Category         string  `json:"category"`
	Subcategory      string  `json:"subcategory"`
	PaymentMethod    string  `json:"payment_method"`
	Description      string  `json:"description"`
	Date             string  `json:"date"`
	Group            string  `json:"group"`
}

// CreateResult is Call 2 CREATE's output: 1..N movement drafts sharing
// one transaction (the caller assigns the actual transaction_id — see
// movement_create_flow.go — only when there's more than one row).
type CreateResult struct {
	Movements []MovementDraft `json:"movements"`
}

// MovementCandidate is a full transaction group (one or more rows)
// fed as context into Call 2 UPDATE/DELETE.
type MovementCandidate struct {
	TransactionID string          `json:"transaction_id"`
	Movements     []MovementDraft `json:"movements"`
}

// UpdateResult is Call 2 UPDATE's output. MentionedDateFrom/MentionedDateTo
// are populated whenever the LLM recognizes a date, relative-day, or date
// range reference in the message, regardless of Resolved — the reference-
// resolution DB search fallback (reference_resolution.go) uses them to
// anchor/bound its search window instead of the default 7-day cap. A
// single mentioned date sets only MentionedDateFrom.
type UpdateResult struct {
	Resolved          bool            `json:"resolved"`
	MentionedDateFrom string          `json:"mentioned_date_from,omitempty"`
	MentionedDateTo   string          `json:"mentioned_date_to,omitempty"`
	Movements         []MovementDraft `json:"movements"`
}

// DeleteResult is Call 2 DELETE's output — it never rebuilds movement
// rows, just confirms whether the candidate is the one meant.
type DeleteResult struct {
	Resolved          bool   `json:"resolved"`
	MentionedDateFrom string `json:"mentioned_date_from,omitempty"`
	MentionedDateTo   string `json:"mentioned_date_to,omitempty"`
}

// OnboardingAccountDraft is one account the user described at onboarding:
// a name, a supported currency, and a fixed monetary balance. Balance is a
// string (never trusted as a native number). Onboarding models a fixed
// amount, never an asset position (units/shares) — see the prompt.
type OnboardingAccountDraft struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Balance  string `json:"balance"`
}

// OnboardingResult is ClassifyOnboarding's output: 0..N account drafts.
// Zero accounts is a valid result (nothing parseable) — the caller re-prompts.
type OnboardingResult struct {
	Accounts []OnboardingAccountDraft `json:"accounts"`
}

// CategoryMatch is ClassifyCategoryCreate's "ya existe algo parecido" answer:
// the existing taxonomy entry the user's request is already covered by.
type CategoryMatch struct {
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
}

// CategoryProposal is ClassifyCategoryCreate's "creá esto" answer: a complete
// subcategory the user only has to confirm. Description is the LLM-written
// classification hint (fed to Call 2 CREATE as TaxonomyEntry.Description).
// Whether Category already exists is decided app-side, not by the LLM.
type CategoryProposal struct {
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Icon        string `json:"emoji"` // the model reaches for "emoji"; app maps it to the row's Icon
	Description string `json:"description"`
}

// CategoryCreateResult carries exactly one of Match/Proposal on success.
type CategoryCreateResult struct {
	Match    *CategoryMatch    `json:"match"`
	Proposal *CategoryProposal `json:"proposal"`
}

// AccountManageResult is ResolveAccountManage's output: which existing
// account the message refers to (nil if none), or whether the user is
// asking for a brand-new account. It carries NO operation and NO values —
// those come from the deterministic menu flow, never from free text.
type AccountManageResult struct {
	MatchedAccountID *uint64
	WantsNewAccount  bool
}

// Normalize corrige el formato de categoría/subcategoría de todas las filas.
//
// Está exportada porque hay DOS entradas a un CreateResult: ClassifyCreate, que
// lo desarma acá adentro, y el agent loop, que desarma los argumentos de
// record_movements en el controller (messaging/agent_executor.go), fuera de
// este paquete. Sin esto la segunda se saltea la corrección que la primera hace,
// y como CREATE va por el loop, se la saltea SIEMPRE.
func (r *CreateResult) Normalize() {
	for i := range r.Movements {
		r.Movements[i].normalizeSubcategory()
	}
}

// normalizeSubcategory corrige un formato que el modelo devuelve de a ratos:
// "Categoría | Subcategoría" en el campo subcategoría, en vez de solo el nombre
// de la subcategoría.
//
// No es alucinación, es imitación: la taxonomía se le pasa al modelo como
// "categoría | subcategoría | descripción", y varias reglas del prompt dicen
// literalmente subcategoría "Sistema | Transferencia". El modelo copia ese
// formato de forma intermitente (reproducido ~1 de cada 8 llamadas).
//
// El costo de no corregirlo es concreto: el par no matchea ninguna fila, así
// que el movimiento cae al gap-fill y se le pregunta al usuario la categoría
// que YA había dicho en su mensaje.
//
// Solo se saca el prefijo cuando coincide con la categoría del mismo draft: si
// una subcategoría legítima llevara un pipe, no se la toca.
//
// El mismo error aparece ESPEJADO: el par entero en el campo categoría, con la
// subcategoría bien puesta ("Ingresos | Freelance / honorarios" + "Freelance /
// honorarios", visto en producción). Las dos mitades se arreglan acá porque son
// el mismo error del modelo —copiar el formato de la taxonomía— y tienen el
// mismo costo: el par no matchea y el gap-fill le pregunta al usuario lo que ya
// dijo.
func (d *MovementDraft) normalizeSubcategory() {
	// El par entero en LOS DOS campos. Va primero porque los otros dos casos
	// asumen que un lado está limpio, y acá no lo está ninguno.
	if d.Category == d.Subcategory {
		if cat, sub, ok := strings.Cut(d.Category, " | "); ok {
			d.Category, d.Subcategory = strings.TrimSpace(cat), strings.TrimSpace(sub)
			return
		}
	}
	// El par en la categoría, subcategoría limpia. Va antes que el de abajo
	// porque le deja d.Category limpia para armar el prefijo.
	if sub := strings.TrimSpace(d.Subcategory); sub != "" && strings.HasSuffix(d.Category, " | "+sub) {
		d.Category = strings.TrimSpace(strings.TrimSuffix(d.Category, " | "+sub))
	}
	// El par en la subcategoría, categoría limpia.
	prefix := strings.TrimSpace(d.Category) + " | "
	if strings.HasPrefix(d.Subcategory, prefix) {
		d.Subcategory = strings.TrimSpace(strings.TrimPrefix(d.Subcategory, prefix))
	}
}
