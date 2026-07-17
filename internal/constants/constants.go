package constants

const (
	ARS = "ARS"
	USD = "USD"
)

const (
	Expense  = "expense"
	Income   = "income"
	Transfer = "transfer"
)

const PendingReview = "PENDING_REVIEW"

// WeeklySummaryOffData is the Telegram callback data for the weekly-summary
// disable button. Shared by the notifier (attaches it) and messaging (handles it).
const WeeklySummaryOffData = "weekly_summary:off"
