//go:build conv_test

package messaging

import (
	"context"
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

func (h *convHarness) Answer(text string) {
	h.t.Helper()
	h.c.Handle(context.Background(), messenger.Incoming{
		Channel:       user.ChannelTelegram,
		ChannelUserID: h.telegramID,
		Chat:          h.chat,
		Input:         conversation.Input{Text: text},
	})
}

func (h *convHarness) subcategoryOf(t *testing.T, movementID uint) subcategory.Subcategory {
	t.Helper()
	for _, m := range h.AllMovements() {
		if m.ID != movementID {
			continue
		}
		var s subcategory.Subcategory
		if err := h.conn.DB.Unscoped().Where("id = ?", m.SubcategoryID).First(&s).Error; err != nil {
			t.Fatalf("subcategoría %d: %v", m.SubcategoryID, err)
		}
		return s
	}
	t.Fatalf("el movimiento %d ya no está: %+v", movementID, h.AllMovements())
	return subcategory.Subcategory{}
}

func (h *convHarness) firstSubcategoryOf(t *testing.T, category string) string {
	t.Helper()
	subs, err := h.cache.FindAllForUser(h.userID)
	if err != nil {
		t.Fatalf("FindAllForUser: %v", err)
	}
	for _, s := range subs {
		if s.Category == category {
			return s.Subcategory
		}
	}
	t.Fatalf("la taxonomía local no tiene ninguna subcategoría de %q", category)
	return ""
}

func (h *convHarness) recordTheCordobaRent(t *testing.T) uint {
	t.Helper()
	h.ScriptCategory(orchestrator.Pair{Category: "Vivienda", Subcategory: "Alquiler"})
	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"expense","amount":"80990","currency":"ARS",
		 "description":"alquiler departamento Córdoba salud","date":"2026-09-06"}]}`))
	h.SendText("$80990 alquiler departamento para Córdoba por salud")

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1. Copia: %v", len(movs), h.Messages())
	}
	if got := h.subcategoryOf(t, movs[0].ID); got.Category != "Vivienda" || got.Subcategory != "Alquiler" {
		t.Fatalf("arrancó en %q/%q, el caso real arranca en Vivienda/Alquiler", got.Category, got.Subcategory)
	}
	return movs[0].ID
}

func (h *convHarness) askToMoveItToAnotherCategory() {
	h.ScriptToolCalls(correctMovementCall(`{
		"change":"Mover el movimiento a otra categoría","changes":[],"scope":"one"}`))
	h.SendText("Mover el movimiento a otra categoría")
	h.TapButton(flow.AskOptionPrefix + "0")
}

func TestConversation_MoveToAnotherCategory_TypingItLowercaseSkipsTheCategoryQuestion(t *testing.T) {
	h := newConversationHarness(t)
	h.recordTheCordobaRent(t)
	h.askToMoveItToAnotherCategory()

	before := len(h.Messages())
	h.Answer("salud")

	asked := strings.Join(h.Messages()[before:], "\n")
	if strings.Contains(strings.ToLower(asked), "a qué categoría") {
		t.Errorf("volvió a preguntar la categoría que el usuario acaba de escribir: %q", asked)
	}
	if !strings.Contains(strings.ToLower(asked), "subcategoría") {
		t.Errorf("no saltó al paso de subcategoría: %q", asked)
	}
	if !strings.Contains(asked, "Salud") {
		t.Errorf("no reconoció «salud» como la categoría Salud: %q", asked)
	}
}

func TestConversation_MoveToAnotherCategory_CreatingTheSubcategoryRewritesTheMovement(t *testing.T) {
	h := newConversationHarness(t)
	h.recordTheCordobaRent(t)
	h.askToMoveItToAnotherCategory()
	h.Answer("salud")

	h.TapButton(flow.OptionSubcategoryCreate)
	h.Answer("Alojamiento tratamiento")

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1 (la corrección no puede perder plata). Copia: %v", len(movs), h.Messages())
	}
	if !movs[0].Amount.Equal(mustDec(t, "-80990")) {
		t.Errorf("el monto cambió: %s", movs[0].Amount)
	}

	got := h.subcategoryOf(t, movs[0].ID)
	if got.Category != "Salud" || got.Subcategory != "Alojamiento tratamiento" {
		t.Fatalf("el movimiento quedó en %q/%q. Copia: %v", got.Category, got.Subcategory, h.Messages())
	}
	if got.UserID == nil || *got.UserID != h.userID {
		t.Error("la subcategoría nueva tiene que ser del usuario, no global")
	}
	if got.Icon == "" {
		t.Error("la subcategoría nueva quedó sin ícono: hereda el de la categoría")
	}
}

func TestConversation_MoveToAnotherCategory_BackReachesADifferentCategory(t *testing.T) {
	h := newConversationHarness(t)
	h.recordTheCordobaRent(t)
	h.askToMoveItToAnotherCategory()
	h.Answer("salud")

	h.TapButton(flow.OptionBack)
	if !containsAny(h.Messages(), "a qué categoría") {
		t.Fatalf("⬅️ Atrás no volvió al listado de categorías: %v", h.Messages())
	}

	h.TapButton("Transporte")
	sub := h.firstSubcategoryOf(t, "Transporte")
	h.TapButton(sub)

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1. Copia: %v", len(movs), h.Messages())
	}
	got := h.subcategoryOf(t, movs[0].ID)
	if got.Category != "Transporte" || got.Subcategory != sub {
		t.Errorf("quedó en %q/%q, want Transporte/%s. Copia: %v", got.Category, got.Subcategory, sub, h.Messages())
	}
}
