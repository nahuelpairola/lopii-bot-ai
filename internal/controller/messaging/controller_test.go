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

func TestChunkButtons_ExactMultiple_NoTrailingShortRow(t *testing.T) {
	buttons := []conversation.Button{
		{Label: "A", Data: "a"}, {Label: "B", Data: "b"}, {Label: "C", Data: "c"}, {Label: "D", Data: "d"},
	}
	rows := chunkButtons(buttons)
	if len(rows) != 2 || len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("rows = %+v, want two rows of 2", rows)
	}
}
