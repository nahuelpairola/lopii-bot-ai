package orchestrator

import "strings"

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

	IntentQueued Intent = "QUEUED"
)

type IntentResult struct {
	Intent Intent `json:"intent"`
}

type TaxonomyEntry struct {
	Category    string
	Subcategory string
	Description string
}

type AccountOption struct {
	ID       uint64
	Name     string
	Currency string
}

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

type CreateResult struct {
	Movements []MovementDraft `json:"movements"`
}

type MovementCandidate struct {
	TransactionID string          `json:"transaction_id"`
	Movements     []MovementDraft `json:"movements"`
}

type UpdateResult struct {
	Resolved          bool            `json:"resolved"`
	MentionedDateFrom string          `json:"mentioned_date_from,omitempty"`
	MentionedDateTo   string          `json:"mentioned_date_to,omitempty"`
	Movements         []MovementDraft `json:"movements"`
}

type DeleteResult struct {
	Resolved          bool   `json:"resolved"`
	MentionedDateFrom string `json:"mentioned_date_from,omitempty"`
	MentionedDateTo   string `json:"mentioned_date_to,omitempty"`
}

type OnboardingAccountDraft struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Balance  string `json:"balance"`
}

type OnboardingResult struct {
	Accounts []OnboardingAccountDraft `json:"accounts"`
}

type CategoryMatch struct {
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
}

type CategoryProposal struct {
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Icon        string `json:"emoji"`
	Description string `json:"description"`
}

type CategoryCreateResult struct {
	Match    *CategoryMatch    `json:"match"`
	Proposal *CategoryProposal `json:"proposal"`
}

type AccountManageResult struct {
	MatchedAccountID *uint64
	WantsNewAccount  bool
}

func (r *CreateResult) Normalize() {
	for i := range r.Movements {
		r.Movements[i].normalizeSubcategory()
	}
}

func (d *MovementDraft) normalizeSubcategory() {
	if d.Category == d.Subcategory {
		if cat, sub, ok := strings.Cut(d.Category, " | "); ok {
			d.Category, d.Subcategory = strings.TrimSpace(cat), strings.TrimSpace(sub)
			return
		}
	}
	if sub := strings.TrimSpace(d.Subcategory); sub != "" && strings.HasSuffix(d.Category, " | "+sub) {
		d.Category = strings.TrimSpace(strings.TrimSuffix(d.Category, " | "+sub))
	}
	prefix := strings.TrimSpace(d.Category) + " | "
	if strings.HasPrefix(d.Subcategory, prefix) {
		d.Subcategory = strings.TrimSpace(strings.TrimPrefix(d.Subcategory, prefix))
	}
}
