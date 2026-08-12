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

	// topDescriptionsForSuggestion acota cuántas descripciones se le mandan al LLM
	// como contexto: suficientes para desambiguar, pocos para no diluir.
	topDescriptionsForSuggestion = 5
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
	next := clearTargetSubcategory(data)
	delete(next, keyTargetCategory)
	return next
}

// clearTargetSubcategory borra la subcategoría destino pero conserva la
// categoría. Es lo que hace falta al volver desde el confirm al picker de
// subcategoría: ese step lista las subcategorías DE una categoría, así que si
// también se borrara la categoría el usuario aterrizaría en un picker vacío,
// sin nada que elegir y sin entender por qué.
func clearTargetSubcategory(data conversation.Data) conversation.Data {
	next := copyData(data)
	delete(next, keyTargetSubcategoryID)
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
				// Todo-o-nada: la fila se busca ANTES de tocar Data. Si esta
				// segunda consulta no matchea (la vista del cache puede cambiar
				// entre el armado de opciones y este OnChoice), no dejamos un
				// origen a medias (ID seteado sin nombres) — devolvemos data
				// intacta.
				owned, _ := subs.FindOwnedByUser(data.UserID())
				for _, s := range owned {
					if strconv.FormatUint(uint64(s.ID), 10) == value {
						next := copyData(data)
						next[keySourceSubcategoryID] = value
						next[keySourceCategory] = s.Category
						next[keySourceSubcategory] = s.Subcategory
						return next
					}
				}
				return data
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(categoryManagePickFlowName, stepPickSource, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// targetSubcategoryLister es lo que el flujo 2 necesita para armar sus dos
// pickers: las categorías (vía el helper compartido) y las subcategorías
// existentes de una categoría.
type targetSubcategoryLister interface {
	categoryLister
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
}

// NewCategoryManageTargetFlow es el flujo 2: ofrece la sugerencia del LLM, cae
// a un picker manual de dos pasos si no hay o si el usuario la rechaza, y
// termina en un confirm que muestra exactamente lo que se va a escribir.
//
// El picker es de dos pasos porque el catálogo global tiene ~90 subcategorías:
// 90 botones en Telegram es inusable.
func NewCategoryManageTargetFlow(subs targetSubcategoryLister) *conversation.Flow {
	hasSuggestion := func(data conversation.Data) bool {
		return stringOrEmpty(data[keySuggestedSubcategory]) != ""
	}
	isEmpty := func(data conversation.Data) bool {
		return stringOrEmpty(data[keyMovementCount]) == "0"
	}

	steps := map[string]conversation.Step{
		stepSuggestTarget: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return msgCategoryManageSuggest(sourceLabel(data), stringOrEmpty(data[keyMovementCount]), suggestionLabel(data))
			},
			// El orden importa: sin movimientos no hay destino que elegir, así
			// que ese chequeo va primero.
			SkipIf: func(data conversation.Data) (string, bool) {
				if isEmpty(data) {
					return stepConfirmCategoryManage, true
				}
				if !hasSuggestion(data) {
					return stepPickTargetCategory, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				return []conversation.ChoiceOption{
					{Label: "✅ Sí, a «" + suggestionLabel(data) + "»", Value: optionAcceptSuggestion, NextStep: stepConfirmCategoryManage},
					{Label: "🔍 Elegir otra", Value: optionChooseOther, NextStep: stepPickTargetCategory},
					cancelOption,
				}
			},
			DeclaredNextSteps: []string{stepConfirmCategoryManage, stepPickTargetCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				switch value {
				case optionAcceptSuggestion:
					next := copyData(data)
					next[keyTargetSubcategoryID] = stringOrEmpty(data[keySuggestedSubcategoryID])
					next[keyTargetCategory] = stringOrEmpty(data[keySuggestedCategory])
					next[keyTargetSubcategory] = stringOrEmpty(data[keySuggestedSubcategory])
					next[keyTargetOrigin] = targetOriginSuggested
					return next
				case optionChooseOther:
					return clearTarget(data)
				}
				return onCategoryManageCancel(value, data)
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},

		stepPickTargetCategory: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return msgCategoryManagePickTargetCat },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				extra := make([]conversation.ChoiceOption, 0, 2)
				// Solo se ofrece volver si hay una sugerencia a la que volver.
				if hasSuggestion(data) {
					extra = append(extra, backOptionTo(stepSuggestTarget))
				}
				extra = append(extra, cancelOption)
				return categoryOptions(subs, data, stepPickTargetSubcategory, extra...)
			},
			DeclaredNextSteps: []string{stepPickTargetSubcategory, stepSuggestTarget},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel {
					return onCategoryManageCancel(optionCancel, data)
				}
				if value == optionBack {
					return clearTarget(data)
				}
				next := clearTarget(data)
				next[keyTargetCategory] = value
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},

		stepPickTargetSubcategory: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return msgCategoryManagePickTargetSub(stringOrEmpty(data[keyTargetCategory]))
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				all, _ := subs.FindAllForUser(data.UserID())
				wantCategory := stringOrEmpty(data[keyTargetCategory])
				sourceID := stringOrEmpty(data[keySourceSubcategoryID])

				opts := make([]conversation.ChoiceOption, 0, len(all)+2)
				for _, s := range all {
					if s.Category != wantCategory {
						continue
					}
					id := strconv.FormatUint(uint64(s.ID), 10)
					if id == sourceID {
						continue // el destino nunca puede ser el origen
					}
					opts = append(opts, conversation.ChoiceOption{
						Label:    subcategoryIcon(s) + " " + s.Subcategory,
						Value:    id,
						NextStep: stepConfirmCategoryManage,
					})
				}
				return append(opts, backOptionTo(stepPickTargetCategory), cancelOption)
			},
			DeclaredNextSteps: []string{stepConfirmCategoryManage, stepPickTargetCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel || value == optionBack {
					return onCategoryManageCancel(value, data)
				}
				// Todo-o-nada: el ID y el nombre se escriben juntos o no se
				// escribe ninguno. Un destino con ID pero sin nombre haría que
				// el confirm dijera «Alimentos › » y el usuario no podría
				// verificar qué está por confirmar — en una operación que mueve
				// movimientos y borra una fila.
				all, _ := subs.FindAllForUser(data.UserID())
				for _, s := range all {
					if strconv.FormatUint(uint64(s.ID), 10) != value {
						continue
					}
					next := copyData(data)
					next[keyTargetSubcategoryID] = value
					next[keyTargetSubcategory] = s.Subcategory
					next[keyTargetOrigin] = targetOriginManual
					return next
				}
				return data
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},

		stepConfirmCategoryManage: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				if isEmpty(data) {
					return msgCategoryManageConfirmDelete(sourceLabel(data))
				}
				return msgCategoryManageConfirmMerge(stringOrEmpty(data[keyMovementCount]), sourceLabel(data), targetLabel(data))
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				opts := []conversation.ChoiceOption{
					{Label: "✅ Confirmar", Value: optionConfirm, Finish: true},
				}
				// El Atrás vuelve al step donde se eligió el destino. Con
				// conteo 0 no hubo elección, así que no se ofrece.
				switch {
				case isEmpty(data):
				case stringOrEmpty(data[keyTargetOrigin]) == targetOriginSuggested:
					opts = append(opts, backOptionTo(stepSuggestTarget))
				default:
					opts = append(opts, backOptionTo(stepPickTargetSubcategory))
				}
				return append(opts, cancelOption)
			},
			DeclaredNextSteps: []string{stepSuggestTarget, stepPickTargetSubcategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				switch value {
				case optionCancel:
					return onCategoryManageCancel(optionCancel, data)
				case optionBack:
					// Volver a la sugerencia descarta el destino entero; volver
					// al picker de subcategoría conserva la categoría, que es
					// justo lo que ese step necesita para tener qué listar.
					if stringOrEmpty(data[keyTargetOrigin]) == targetOriginSuggested {
						return clearTarget(data)
					}
					return clearTargetSubcategory(data)
				}
				next := copyData(data)
				setFlag(next, keyConfirmed)
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(categoryManageTargetFlowName, stepSuggestTarget, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
