package flow

import (
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

func gapData(rows []movement.MovementRow, gaps []string) conversation.Data {
	return conversation.Data{
		conversation.KeyMovements:           movement.EncodeMovementRows(rows),
		conversation.KeyPendingCategoryGaps: conversation.EncodeStringSlice(gaps),
		conversation.UserIDKey:              uint64(1),
	}
}

func TestActiveGapRow_FallsBackToFirstPendingGap(t *testing.T) {
	rows := []movement.MovementRow{{Category: "Salud"}, {Category: "Transporte"}}
	if got := ActiveGapRow(gapData(rows, []string{"1"})); got != 1 {
		t.Errorf("ActiveGapRow = %d, want 1 (el gap pendiente)", got)
	}
}

func TestActiveGapRow_PrefersTheExplicitActiveRow(t *testing.T) {
	rows := []movement.MovementRow{{Category: "Salud"}, {Category: "Transporte"}}
	data := gapData(rows, []string{"1"})
	data[conversation.KeyGapActiveRow] = "0"
	if got := ActiveGapRow(data); got != 0 {
		t.Errorf("ActiveGapRow = %d, want 0 (el que escribió el paso)", got)
	}
}

func TestRowCategoryExists_UserNamedCategory(t *testing.T) {
	repo := &fakeSubcatSetupRepo{categories: []string{"Salud", "Transporte"}}
	rows := []movement.MovementRow{{Category: "Transporte", Subcategory: ""}}
	if !rowCategoryExists(repo, gapData(rows, []string{"0"})) {
		t.Error("rowCategoryExists = false: volvería a preguntar la categoría que el usuario acaba de decir")
	}
}

func TestRowCategoryExists_UnknownCategoryStillAsks(t *testing.T) {
	repo := &fakeSubcatSetupRepo{categories: []string{"Salud"}}
	rows := []movement.MovementRow{{Category: "PENDING_REVIEW"}}
	if rowCategoryExists(repo, gapData(rows, []string{"0"})) {
		t.Error("rowCategoryExists = true sobre una categoría que el usuario no tiene")
	}
}
