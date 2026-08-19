package flow

import (
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

const (
	StepPickSource            = "category_manage_pick_source"
	StepSuggestTarget         = "category_manage_suggest_target"
	StepPickTargetCategory    = "category_manage_pick_target_category"
	StepPickTargetSubcategory = "category_manage_pick_target_subcategory"
	StepConfirmCategoryManage = "category_manage_confirm"

	OptionAcceptSuggestion = "accept_suggestion"
	OptionChooseOther      = "choose_other"

	// valores de conversation.KeyTargetOrigin
	TargetOriginSuggested = "suggested"
	TargetOriginManual    = "manual"

	// DefaultCategoryIcon es el ícono cuando una fila no tiene uno propio.
	DefaultCategoryIcon = "📂"

	// TopDescriptionsForSuggestion acota cuántas descripciones se le mandan al LLM
	// como contexto: suficientes para desambiguar, pocos para no diluir.
	TopDescriptionsForSuggestion = 5
)

// OnCategoryManageCancel marca el flujo como cancelado, para que el finish
// saltee toda escritura. Mismo contrato que OnAccountManageCancel.
func OnCategoryManageCancel(value string, data conversation.Data) conversation.Data {
	if value != OptionCancel {
		return data
	}
	next := conversation.CopyData(data)
	conversation.SetFlag(next, conversation.KeyCancelled)
	return next
}

// ClearTarget borra el destino elegido. Lo llama toda transición que reabre esa
// elección: sin esto, aceptar la sugerencia, volver con Atrás y elegir "otra"
// arrastraría el destino viejo y fusionaría contra la categoría rechazada.
func ClearTarget(data conversation.Data) conversation.Data {
	next := ClearTargetSubcategory(data)
	delete(next, conversation.KeyTargetCategory)
	return next
}

// ClearTargetSubcategory borra la subcategoría destino pero conserva la
// categoría. Es lo que hace falta al volver desde el confirm al picker de
// subcategoría: ese step lista las subcategorías DE una categoría, así que si
// también se borrara la categoría el usuario aterrizaría en un picker vacío,
// sin nada que elegir y sin entender por qué.
func ClearTargetSubcategory(data conversation.Data) conversation.Data {
	next := conversation.CopyData(data)
	delete(next, conversation.KeyTargetSubcategoryID)
	delete(next, conversation.KeyTargetSubcategory)
	delete(next, conversation.KeyTargetOrigin)
	return next
}

func SubcategoryIcon(s subcategory.Subcategory) string {
	if s.Icon == "" {
		return DefaultCategoryIcon
	}
	return s.Icon
}

func SourceLabel(data conversation.Data) string {
	return conversation.StringOrEmpty(data[conversation.KeySourceCategory]) + " › " + conversation.StringOrEmpty(data[conversation.KeySourceSubcategory])
}

func SuggestionLabel(data conversation.Data) string {
	return conversation.StringOrEmpty(data[conversation.KeySuggestedCategory]) + " › " + conversation.StringOrEmpty(data[conversation.KeySuggestedSubcategory])
}

func TargetLabel(data conversation.Data) string {
	return conversation.StringOrEmpty(data[conversation.KeyTargetCategory]) + " › " + conversation.StringOrEmpty(data[conversation.KeyTargetSubcategory])
}

// NewCategoryManagePickFlow es el flujo 1: un solo paso para elegir cuál de las
// categorías propias sacar. Termina ahí porque lo que sigue necesita una
// llamada al LLM, y eso no puede vivir dentro de un SkipIf — el controller
// hace el puente (ver proceedToCategoryTarget) y arranca el flujo 2.
func NewCategoryManagePickFlow(subs ownedSubcategoryLister) *conversation.Flow {
	steps := map[string]conversation.Step{
		StepPickSource: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return MsgCategoryManagePickSource },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				owned, _ := subs.FindOwnedByUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(owned)+1)
				for _, s := range owned {
					opts = append(opts, conversation.ChoiceOption{
						Label:  CategoryLabel(SubcategoryIcon(s), s.Category, s.Subcategory),
						Value:  strconv.FormatUint(uint64(s.ID), 10),
						Finish: true,
					})
				}
				return append(opts, CancelOption)
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					return OnCategoryManageCancel(OptionCancel, data)
				}
				// Todo-o-nada: la fila se busca ANTES de tocar Data. Si esta
				// segunda consulta no matchea (la vista del cache puede cambiar
				// entre el armado de opciones y este OnChoice), no dejamos un
				// origen a medias (ID seteado sin nombres) — devolvemos data
				// intacta.
				owned, _ := subs.FindOwnedByUser(data.UserID())
				for _, s := range owned {
					if strconv.FormatUint(uint64(s.ID), 10) == value {
						next := conversation.CopyData(data)
						next[conversation.KeySourceSubcategoryID] = value
						next[conversation.KeySourceCategory] = s.Category
						next[conversation.KeySourceSubcategory] = s.Subcategory
						return next
					}
				}
				return data
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(CategoryManagePickFlowName, StepPickSource, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// NewCategoryManageTargetFlow es el flujo 2: ofrece la sugerencia del LLM, cae
// a un picker manual de dos pasos si no hay o si el usuario la rechaza, y
// termina en un confirm que muestra exactamente lo que se va a escribir.
//
// El picker es de dos pasos porque el catálogo global tiene ~90 subcategorías:
// 90 botones en Telegram es inusable.
func NewCategoryManageTargetFlow(subs targetSubcategoryLister) *conversation.Flow {
	hasSuggestion := func(data conversation.Data) bool {
		return conversation.StringOrEmpty(data[conversation.KeySuggestedSubcategory]) != ""
	}
	isEmpty := func(data conversation.Data) bool {
		return conversation.StringOrEmpty(data[conversation.KeyMovementCount]) == "0"
	}

	steps := map[string]conversation.Step{
		StepSuggestTarget: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return MsgCategoryManageSuggest(SourceLabel(data), conversation.StringOrEmpty(data[conversation.KeyMovementCount]), SuggestionLabel(data))
			},
			// El orden importa: sin movimientos no hay destino que elegir, así
			// que ese chequeo va primero.
			SkipIf: func(data conversation.Data) (string, bool) {
				if isEmpty(data) {
					return StepConfirmCategoryManage, true
				}
				if !hasSuggestion(data) {
					return StepPickTargetCategory, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				return []conversation.ChoiceOption{
					{Label: "✅ Sí, a «" + SuggestionLabel(data) + "»", Value: OptionAcceptSuggestion, NextStep: StepConfirmCategoryManage},
					{Label: "🔍 Elegir otra", Value: OptionChooseOther, NextStep: StepPickTargetCategory},
					CancelOption,
				}
			},
			DeclaredNextSteps: []string{StepConfirmCategoryManage, StepPickTargetCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				switch value {
				case OptionAcceptSuggestion:
					next := conversation.CopyData(data)
					next[conversation.KeyTargetSubcategoryID] = conversation.StringOrEmpty(data[conversation.KeySuggestedSubcategoryID])
					next[conversation.KeyTargetCategory] = conversation.StringOrEmpty(data[conversation.KeySuggestedCategory])
					next[conversation.KeyTargetSubcategory] = conversation.StringOrEmpty(data[conversation.KeySuggestedSubcategory])
					next[conversation.KeyTargetOrigin] = TargetOriginSuggested
					return next
				case OptionChooseOther:
					return ClearTarget(data)
				}
				return OnCategoryManageCancel(value, data)
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},

		StepPickTargetCategory: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return MsgCategoryManagePickTargetCat },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				extra := make([]conversation.ChoiceOption, 0, 2)
				// Solo se ofrece volver si hay una sugerencia a la que volver.
				if hasSuggestion(data) {
					extra = append(extra, BackOptionTo(StepSuggestTarget))
				}
				extra = append(extra, CancelOption)
				return CategoryOptions(subs, data, StepPickTargetSubcategory, extra...)
			},
			DeclaredNextSteps: []string{StepPickTargetSubcategory, StepSuggestTarget},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					return OnCategoryManageCancel(OptionCancel, data)
				}
				if value == OptionBack {
					return ClearTarget(data)
				}
				next := ClearTarget(data)
				next[conversation.KeyTargetCategory] = value
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},

		StepPickTargetSubcategory: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return MsgCategoryManagePickTargetSub(conversation.StringOrEmpty(data[conversation.KeyTargetCategory]))
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				all, _ := subs.FindAllForUser(data.UserID())
				wantCategory := conversation.StringOrEmpty(data[conversation.KeyTargetCategory])
				sourceID := conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID])

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
						Label:    SubcategoryIcon(s) + " " + s.Subcategory,
						Value:    id,
						NextStep: StepConfirmCategoryManage,
					})
				}
				return append(opts, BackOptionTo(StepPickTargetCategory), CancelOption)
			},
			DeclaredNextSteps: []string{StepConfirmCategoryManage, StepPickTargetCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel || value == OptionBack {
					return OnCategoryManageCancel(value, data)
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
					next := conversation.CopyData(data)
					next[conversation.KeyTargetSubcategoryID] = value
					next[conversation.KeyTargetSubcategory] = s.Subcategory
					next[conversation.KeyTargetOrigin] = TargetOriginManual
					return next
				}
				return data
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},

		StepConfirmCategoryManage: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				if isEmpty(data) {
					return MsgCategoryManageConfirmDelete(SourceLabel(data))
				}
				return MsgCategoryManageConfirmMerge(conversation.StringOrEmpty(data[conversation.KeyMovementCount]), SourceLabel(data), TargetLabel(data))
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				opts := []conversation.ChoiceOption{
					{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				}
				// El Atrás vuelve al step donde se eligió el destino. Con
				// conteo 0 no hubo elección, así que no se ofrece.
				switch {
				case isEmpty(data):
				case conversation.StringOrEmpty(data[conversation.KeyTargetOrigin]) == TargetOriginSuggested:
					opts = append(opts, BackOptionTo(StepSuggestTarget))
				default:
					opts = append(opts, BackOptionTo(StepPickTargetSubcategory))
				}
				return append(opts, CancelOption)
			},
			DeclaredNextSteps: []string{StepSuggestTarget, StepPickTargetSubcategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				switch value {
				case OptionCancel:
					return OnCategoryManageCancel(OptionCancel, data)
				case OptionBack:
					// Volver a la sugerencia descarta el destino entero; volver
					// al picker de subcategoría conserva la categoría, que es
					// justo lo que ese step necesita para tener qué listar.
					if conversation.StringOrEmpty(data[conversation.KeyTargetOrigin]) == TargetOriginSuggested {
						return ClearTarget(data)
					}
					return ClearTargetSubcategory(data)
				}
				next := conversation.CopyData(data)
				conversation.SetFlag(next, conversation.KeyConfirmed)
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(CategoryManageTargetFlowName, StepSuggestTarget, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
