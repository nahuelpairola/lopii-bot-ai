package subcategory

import (
	"errors"
	"strings"

	"lopiibot.com/internal/conversation"
)

// subcategoryNameAndSaveStep pide el nombre de la subcategoría, la
// inserta en DB, y vuelve al selector de categoría para que el usuario
// pueda seguir agregando o terminar. No es un TextStep genérico porque
// necesita ejecutar el efecto de guardar en DB antes de decidir la
// transición — algo que TextStep no contempla (es agnóstico de
// persistencia a propósito, para mantenerse reutilizable en casos que no
// la necesitan, como account.accountNameStep).
type subcategoryNameAndSaveStep struct {
	creator subcategoryCreator
}

func (s subcategoryNameAndSaveStep) Prompt(data conversation.Data) conversation.Prompt {
	category := data[dataKeyCategory].(string)
	return conversation.Prompt{Text: msgAskSubcategoryName(category)}
}

func (s subcategoryNameAndSaveStep) Process(input conversation.Input, data conversation.Data) conversation.Transition {
	name := strings.TrimSpace(input.Text)
	if name == "" {
		return conversation.Retry(msgInvalidSubcategoryName)
	}

	category := data[dataKeyCategory].(string)
	userID := data.UserID()

	newSub := &Subcategory{
		UserID:      &userID,
		Category:    category,
		Subcategory: name,
		IsGlobal:    false,
	}

	if err := s.creator.Insert(newSub); err != nil {
		if errors.Is(err, ErrSubcategoryAlreadyExists) {
			return conversation.Retry(msgSubcategoryAlreadyExists(category, name))
		}
		return conversation.Retry(msgCreationError)
	}

	next := conversation.Data{
		conversation.UserIDKey:   userID,
		dataKeySubcategoriesDone: true,
	}
	return conversation.Advance(StepChooseCategory, next)
}

func (s subcategoryNameAndSaveStep) PossibleNextSteps() []string {
	return []string{StepChooseCategory}
}
