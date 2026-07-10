package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/reminder"
)

func TestParseWindow(t *testing.T) {
	ok := []struct {
		in                 string
		wantStart, wantEnd int
	}{
		{"20 a 21", 1200, 1260},
		{"entre las 8 y las 10", 480, 600},
		{"de 9 a 13", 540, 780},
		{"20-22", 1200, 1320},
	}
	for _, c := range ok {
		s, e, err := parseWindow(c.in)
		if err != nil || s != c.wantStart || e != c.wantEnd {
			t.Errorf("parseWindow(%q) = (%d,%d,%v), want (%d,%d,nil)", c.in, s, e, err, c.wantStart, c.wantEnd)
		}
	}
	bad := []string{"", "21", "abc", "25 a 26", "21 a 20", "20 a 20", "-1 a 5"}
	for _, in := range bad {
		if _, _, err := parseWindow(in); err == nil {
			t.Errorf("parseWindow(%q) expected error, got nil", in)
		}
	}
}

type fakeReminderRepo struct {
	upserted *reminder.Reminder
	disabled uint64
}

func (f *fakeReminderRepo) Upsert(r *reminder.Reminder) error { f.upserted = r; return nil }
func (f *fakeReminderRepo) Disable(userID uint64) error       { f.disabled = userID; return nil }
func (f *fakeReminderRepo) FindByUserID(uint64) (*reminder.Reminder, error) {
	return nil, nil
}

func TestFinishReminderSetup_Preset(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	data := conversation.Data{
		conversation.UserIDKey: uint64(42),
		reminderActionKey:      reminderActionSet,
		reminderStartKey:       "1200",
		reminderEndKey:         "1320",
	}
	c.finishReminderSetup(context.Background(), nil, 0, data)
	if repo.upserted == nil || repo.upserted.WindowStartMin != 1200 || repo.upserted.WindowEndMin != 1320 || !repo.upserted.Enabled || repo.upserted.UserID != 42 {
		t.Fatalf("unexpected upsert: %+v", repo.upserted)
	}
}

func TestFinishReminderSetup_Custom(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	data := conversation.Data{
		conversation.UserIDKey: uint64(7),
		reminderActionKey:      reminderActionSet,
		reminderCustomKey:      "9 a 13",
	}
	c.finishReminderSetup(context.Background(), nil, 0, data)
	if repo.upserted == nil || repo.upserted.WindowStartMin != 540 || repo.upserted.WindowEndMin != 780 {
		t.Fatalf("unexpected upsert: %+v", repo.upserted)
	}
}

func TestFinishReminderSetup_Disable(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	data := conversation.Data{
		conversation.UserIDKey: uint64(99),
		reminderActionKey:      reminderActionOff,
	}
	c.finishReminderSetup(context.Background(), nil, 0, data)
	if repo.disabled != 99 {
		t.Fatalf("expected Disable(99), got %d", repo.disabled)
	}
}

func TestFinishReminderSetup_Cancelled(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), nil, 0, conversation.Data{
		conversation.UserIDKey: uint64(1),
		"cancelled":            "true",
	})
	if repo.upserted != nil || repo.disabled != 0 {
		t.Fatal("cancelled path must not touch the repo")
	}
}

func TestNewReminderSetupFlow_Valid(t *testing.T) {
	// panics at construction if the step graph is invalid
	_ = NewReminderSetupFlow()
}
