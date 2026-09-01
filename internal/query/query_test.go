package query

import (
	"strings"
	"testing"

	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

func TestExecListCategories_OmitsEmptyDescription(t *testing.T) {
	subs := &fakeQuerySubcats{subs: []subcategory.Subcategory{
		{Category: "Alimentación", Subcategory: "Supermercado", Description: "Compra grande. NO incluye almacén.", Icon: "🍔"},
		{Category: "Alimentación", Subcategory: "Sueldo", Icon: "💵"},
	}}
	svc := &queryTestServices{subcats: subs}

	out, err := execListCategories(svc, 1, queryToolArgs{Category: "Alimentación"})
	if err != nil {
		t.Fatalf("execListCategories: %v", err)
	}

	if !strings.Contains(out, "Alimentación | Supermercado | Compra grande. NO incluye almacén.") {
		t.Errorf("un par con nota tiene que conservarla:\n%s", out)
	}
	if strings.Contains(out, "Sueldo | ") {
		t.Errorf("separador colgante en la respuesta al usuario:\n%s", out)
	}
	if !strings.Contains(out, "Alimentación | Sueldo") {
		t.Errorf("el par sin nota igual tiene que estar:\n%s", out)
	}
}

func TestExecGetReminder(t *testing.T) {
	start := 1200
	active := &reminder.Reminder{UserID: 5, WindowStartMin: start, WindowEndMin: 1260, Enabled: true}

	got := describeReminder(active)
	if !strings.Contains(got, "20") || !strings.Contains(got, "21") || !strings.Contains(got, "activo") {
		t.Errorf("active description missing window/estado: %q", got)
	}

	off := &reminder.Reminder{UserID: 5, WindowStartMin: start, WindowEndMin: 1260, Enabled: false}
	if !strings.Contains(describeReminder(off), "apagado") {
		t.Errorf("disabled description should say apagado: %q", describeReminder(off))
	}

	if describeReminder(nil) == "" {
		t.Error("nil (no reminder) must return a non-empty description")
	}
}
