package orchestrator

// Intent is the router's classification of a free-text message.
type Intent string

const (
	IntentCreate         Intent = "CREATE"
	IntentUpdate         Intent = "UPDATE"
	IntentDelete         Intent = "DELETE"
	IntentQuery          Intent = "QUERY"
	IntentAccountCreate  Intent = "ACCOUNT_CREATE"
	IntentCreateCategory Intent = "CREATE_CATEGORY"
)

// IntentResult is Call 1 router's output: the classified intent, plus
// whether the message was too ambiguous to trust outright. Only
// meaningful when Intent == IntentCreate — that's the only path with a
// frictionless (no-confirmation) default to guard.
type IntentResult struct {
	Intent            Intent `json:"intent"`
	NeedsConfirmation bool   `json:"needs_confirmation"`
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
// decimal.Decimal/time.Time. AccountID is set only when the LLM matched
// an existing account; AccountNameGuess is set instead when a transfer
// names an account that doesn't exist yet (triggers the account-
// creation gap in the CREATE flow).
type MovementDraft struct {
	Type             string  `json:"type"`
	Amount           string  `json:"amount"`
	Currency         string  `json:"currency"`
	AccountID        *uint64 `json:"account_id,omitempty"`
	AccountNameGuess string  `json:"account_name_guess,omitempty"`
	Category         string  `json:"category"`
	Subcategory      string  `json:"subcategory"`
	PaymentMethod    string  `json:"payment_method"`
	Merchant         string  `json:"merchant"`
	Description      string  `json:"description"`
	Date             string  `json:"date"`
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
