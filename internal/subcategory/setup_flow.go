package subcategory

import "lopiibot.com/internal/conversation"

// Nombres de los pasos del flujo de alta de subcategorías.
const (
	FlowName = "subcategory_setup"

	StepChooseCategory  = "choose_category"
	StepNewCategoryName = "new_category_name"
	StepSubcategoryName = "subcategory_name"
)

// Claves usadas dentro de Data a lo largo del flujo.
const (
	dataKeyCategory          = "category"
	dataKeySubcategoriesDone = "subcategories_created"
)

// categoryLister es lo mínimo que el flujo necesita para ofrecer las
// categorías ya existentes del usuario como opciones.
type categoryLister interface {
	DistinctCategoriesForUser(userID uint64) ([]string, error)
}

// subcategoryCreator es lo que el último paso necesita para persistir la
// subcategoría en DB.
type subcategoryCreator interface {
	Insert(s *Subcategory) error
}

// NewSetupFlow arma el Flow de alta de subcategorías. Es estático, igual
// que account.NewSetupFlow: no depende de ningún usuario en particular,
// los Steps leen el userID de Data en runtime (ver
// conversation.Data.UserID), así se registra una sola vez al arrancar el
// server.
func NewSetupFlow(categories categoryLister, creator subcategoryCreator) (*conversation.Flow, error) {
	// chooseCategory usa OptionsFunc porque las categorías son específicas
	// de cada usuario (se cargan dinámicamente de DB). Las opciones fijas
	// siempre incluyen "Nueva categoría" y "Listo".
	chooseCategory := conversation.ChoiceStep{
		PromptText: func(data conversation.Data) string {
			if _, already := data[dataKeySubcategoriesDone]; already {
				return msgAskAddAnother
			}
			return msgChooseCategoryIntro
		},
		OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
			existing, err := categories.DistinctCategoriesForUser(data.UserID())
			if err != nil {
				existing = nil // degrada con gracia: solo se ofrece "otra" + "listo"
			}

			opts := make([]conversation.ChoiceOption, 0, len(existing)+2)
			for _, cat := range existing {
				opts = append(opts, conversation.ChoiceOption{
					Label:    cat,
					Value:    cat,
					NextStep: StepSubcategoryName,
				})
			}
			opts = append(opts,
				conversation.ChoiceOption{Label: btnNewCategory, Value: "_new", NextStep: StepNewCategoryName},
				conversation.ChoiceOption{Label: btnFinishSetup, Value: "_done", Finish: true},
			)
			return opts
		},
		OnChoice: func(value string, data conversation.Data) conversation.Data {
			next := copyData(data)
			if value != "_new" && value != "_done" {
				next[dataKeyCategory] = value
			}
			return next
		},
	}

	// newCategoryName: TextStep genérico, sin necesidad de un Step
	// custom — a diferencia de accountNameStep en account, acá no hay
	// ninguna decisión condicional de a dónde saltar.
	newCategoryName := conversation.TextStep{
		PromptText: func(data conversation.Data) string {
			return msgAskNewCategoryName()
		},
		DataKey: dataKeyCategory,
		Validate: func(text string, data conversation.Data) string {
			if text == "" {
				return msgInvalidCategoryName
			}
			return ""
		},
		NextStep: StepSubcategoryName,
	}

	subcategoryNameStep := subcategoryNameAndSaveStep{creator: creator}

	steps := map[string]conversation.Step{
		StepChooseCategory:  chooseCategory,
		StepNewCategoryName: newCategoryName,
		StepSubcategoryName: subcategoryNameStep,
	}

	return conversation.NewFlow(FlowName, StepChooseCategory, steps)
}

func copyData(data conversation.Data) conversation.Data {
	next := conversation.Data{}
	for k, v := range data {
		next[k] = v
	}
	return next
}
