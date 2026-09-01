package agent

func changeFieldOptions() []string {
	return []string{labelChangeCategory, labelChangeDate, labelChangeAccount}
}

const (
	labelChangeCategory = "La categoría"
	labelChangeDate     = "La fecha"
	labelChangeAccount  = "La cuenta"
)

func changeFieldForLabel(label string) changeField {
	switch label {
	case labelChangeCategory:
		return fieldCategory
	case labelChangeDate:
		return fieldDate
	case labelChangeAccount:
		return fieldAccount
	default:
		return ""
	}
}
