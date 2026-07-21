package messaging

import (
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

const (
	categoryManagePickFlowName   = "category_manage_pick"
	categoryManageTargetFlowName = "category_manage_target"

	stepPickSource            = "category_manage_pick_source"
	stepSuggestTarget         = "category_manage_suggest_target"
	stepPickTargetCategory    = "category_manage_pick_target_category"
	stepPickTargetSubcategory = "category_manage_pick_target_subcategory"
	stepConfirmCategoryManage = "category_manage_confirm"

	optionAcceptSuggestion = "accept_suggestion"
	optionChooseOther      = "choose_other"

	// valores de keyTargetOrigin
	targetOriginSuggested = "suggested"
	targetOriginManual    = "manual"

	// defaultCategoryIcon es el ícono cuando una fila no tiene uno propio.
	defaultCategoryIcon = "📂"

	// topMerchantsForSuggestion acota cuántos comercios se le mandan al LLM
	// como contexto: suficientes para desambiguar, pocos para no diluir.
	topMerchantsForSuggestion = 5
)

// ownedSubcategoryLister es lo único que el flujo 1 necesita: las filas que
// creó el usuario. Las globales no son candidatas a borrarse.
type ownedSubcategoryLister interface {
	FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error)
}

// onCategoryManageCancel marca el flujo como cancelado, para que el finish
// saltee toda escritura. Mismo contrato que onAccountManageCancel.
func onCategoryManageCancel(value string, data conversation.Data) conversation.Data {
	if value != optionCancel {
		return data
	}
	next := copyData(data)
	setFlag(next, keyCancelled)
	return next
}

// clearTarget borra el destino elegido. Lo llama toda transición que reabre esa
// elección: sin esto, aceptar la sugerencia, volver con Atrás y elegir "otra"
// arrastraría el destino viejo y fusionaría contra la categoría rechazada.
func clearTarget(data conversation.Data) conversation.Data {
	next := copyData(data)
	delete(next, keyTargetSubcategoryID)
	delete(next, keyTargetCategory)
	delete(next, keyTargetSubcategory)
	delete(next, keyTargetOrigin)
	return next
}

func subcategoryIcon(s subcategory.Subcategory) string {
	if s.Icon == "" {
		return defaultCategoryIcon
	}
	return s.Icon
}

func sourceLabel(data conversation.Data) string {
	return stringOrEmpty(data[keySourceCategory]) + " › " + stringOrEmpty(data[keySourceSubcategory])
}

func suggestionLabel(data conversation.Data) string {
	return stringOrEmpty(data[keySuggestedCategory]) + " › " + stringOrEmpty(data[keySuggestedSubcategory])
}

func targetLabel(data conversation.Data) string {
	return stringOrEmpty(data[keyTargetCategory]) + " › " + stringOrEmpty(data[keyTargetSubcategory])
}

// NewCategoryManagePickFlow es el flujo 1: un solo paso para elegir cuál de las
// categorías propias sacar. Termina ahí porque lo que sigue necesita una
// llamada al LLM, y eso no puede vivir dentro de un SkipIf — el controller
// hace el puente (ver proceedToCategoryTarget) y arranca el flujo 2.
func NewCategoryManagePickFlow(subs ownedSubcategoryLister) *conversation.Flow {
	steps := map[string]conversation.Step{
		stepPickSource: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return msgCategoryManagePickSource },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				owned, _ := subs.FindOwnedByUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(owned)+1)
				for _, s := range owned {
					opts = append(opts, conversation.ChoiceOption{
						Label:  categoryLabel(subcategoryIcon(s), s.Category, s.Subcategory),
						Value:  strconv.FormatUint(uint64(s.ID), 10),
						Finish: true,
					})
				}
				return append(opts, cancelOption)
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel {
					return onCategoryManageCancel(optionCancel, data)
				}
				next := copyData(data)
				next[keySourceSubcategoryID] = value
				owned, _ := subs.FindOwnedByUser(data.UserID())
				for _, s := range owned {
					if strconv.FormatUint(uint64(s.ID), 10) == value {
						next[keySourceCategory] = s.Category
						next[keySourceSubcategory] = s.Subcategory
						break
					}
				}
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(categoryManagePickFlowName, stepPickSource, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
