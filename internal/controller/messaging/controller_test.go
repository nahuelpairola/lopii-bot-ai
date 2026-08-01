package messaging

import (
	"testing"

	"lopiibot.com/internal/conversation"
)

func TestChunkButtons_SplitsIntoRowsOfTwo(t *testing.T) {
	buttons := []conversation.Button{
		{Label: "A", Data: "a"}, {Label: "B", Data: "b"}, {Label: "C", Data: "c"},
	}
	rows := chunkButtons(buttons)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 1 {
		t.Errorf("row sizes = %d/%d, want 2/1", len(rows[0]), len(rows[1]))
	}
	if rows[0][0].CallbackData != "a" || rows[0][1].CallbackData != "b" || rows[1][0].CallbackData != "c" {
		t.Error("button order not preserved across rows")
	}
}

func TestChunkButtons_Empty_ReturnsNil(t *testing.T) {
	if rows := chunkButtons(nil); rows != nil {
		t.Errorf("rows = %v, want nil", rows)
	}
}

// TestChunkButtons_LongLabelsGetTheirOwnRow: los candidatos de una corrección
// llevan monto Y fecha, y de a dos por fila Telegram corta el final — que es
// justo la fecha, lo único que los distingue entre sí. Con tres candidatos de
// "Cafe" el picker quedaba con tres botones que se leían igual.
func TestChunkButtons_LongLabelsGetTheirOwnRow(t *testing.T) {
	buttons := []conversation.Button{
		{Label: "🔴 Cafe · $1.800 · hoy", Data: "0"},
		{Label: "🔴 Cafe · $1.200 · hoy", Data: "1"},
		{Label: "🔴 Cafe · $2.000 · 27/07", Data: "2"},
		{Label: "🚫 Cancelar", Data: "cancel"},
	}
	rows := chunkButtons(buttons)
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4 — una por botón", len(rows))
	}
	for i, row := range rows {
		if len(row) != 1 {
			t.Errorf("fila %d tiene %d botones, want 1", i, len(row))
		}
	}
}

// Y el contrapeso: las categorías son cortas y son ~16. Estirarlas a una por
// fila duplica el alto del teclado sin ganar legibilidad.
func TestChunkButtons_ShortLabelsStayTwoPerRow(t *testing.T) {
	buttons := []conversation.Button{
		{Label: "🍔 Alimentación", Data: "a"},
		{Label: "🚗 Transporte", Data: "b"},
		{Label: "🏠 Hogar", Data: "c"},
		{Label: "💊 Salud", Data: "d"},
	}
	rows := chunkButtons(buttons)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
}

func TestChunkButtons_ExactMultiple_NoTrailingShortRow(t *testing.T) {
	buttons := []conversation.Button{
		{Label: "A", Data: "a"}, {Label: "B", Data: "b"}, {Label: "C", Data: "c"}, {Label: "D", Data: "d"},
	}
	rows := chunkButtons(buttons)
	if len(rows) != 2 || len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("rows = %+v, want two rows of 2", rows)
	}
}
