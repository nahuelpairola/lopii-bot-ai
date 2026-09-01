package movement

func IconForType(t movementType) string {
	switch t {
	case Expense:
		return "🔴"
	case Income:
		return "🟢"
	default:
		return "🏦"
	}
}

func TypeFromString(s string) movementType {
	switch s {
	case string(Income):
		return Income
	case string(Transfer):
		return Transfer
	default:
		return Expense
	}
}
