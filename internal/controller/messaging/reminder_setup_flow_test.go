package messaging

import (
	"context"
	"strings"
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
	upserted     *reminder.Reminder
	disabled     uint64
	weeklySet    *bool
	weeklySetFor uint64
}

func (f *fakeReminderRepo) Upsert(r *reminder.Reminder) error { f.upserted = r; return nil }
func (f *fakeReminderRepo) Disable(userID uint64) error       { f.disabled = userID; return nil }
func (f *fakeReminderRepo) FindByUserID(uint64) (*reminder.Reminder, error) {
	return nil, nil
}
func (f *fakeReminderRepo) SetWeeklySummary(userID uint64, enabled bool) error {
	f.weeklySetFor = userID
	f.weeklySet = &enabled
	return nil
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

func TestReminderSetup_PresetThenWeeklyYes(t *testing.T) {
	// onReminderPickWindow(preset) then onReminderWeekly(Sí) sets the flag; finish Upserts it.
	data := onReminderPickWindow("1200-1320", conversation.Data{conversation.UserIDKey: uint64(42)})
	data = onReminderWeekly(optionWeeklyOn, data)

	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), nil, 0, data)

	if repo.upserted == nil || !repo.upserted.WeeklySummaryEnabled {
		t.Fatalf("expected upsert with WeeklySummaryEnabled=true, got %+v", repo.upserted)
	}
	if repo.upserted.WindowStartMin != 1200 || repo.upserted.WindowEndMin != 1320 {
		t.Fatalf("unexpected window: %+v", repo.upserted)
	}
}

func TestReminderSetup_WeeklyOnlyOff(t *testing.T) {
	// weekly toggle OFF (row exists) -> SetWeeklySummary(userID,false); no Upsert/Disable.
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), nil, 0, conversation.Data{
		conversation.UserIDKey: uint64(7),
		reminderActionKey:      reminderActionWeeklyOnly,
		keyHubHasRow:           "true",
		// keyWeeklySummary absent => turning OFF
	})
	if repo.weeklySet == nil || *repo.weeklySet != false || repo.weeklySetFor != 7 {
		t.Fatalf("expected SetWeeklySummary(7,false), got for=%d val=%v", repo.weeklySetFor, repo.weeklySet)
	}
	if repo.upserted != nil || repo.disabled != 0 {
		t.Fatal("weekly-only path must not Upsert or Disable")
	}
}

func TestFinishReminderSetup_OffAll(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), nil, 0, conversation.Data{
		conversation.UserIDKey: uint64(42),
		reminderActionKey:      reminderActionOffAll,
	})
	if repo.disabled != 42 {
		t.Fatalf("expected Disable(42), got %d", repo.disabled)
	}
	if repo.weeklySet == nil || *repo.weeklySet != false || repo.weeklySetFor != 42 {
		t.Fatalf("expected SetWeeklySummary(42,false), got for=%d val=%v", repo.weeklySetFor, repo.weeklySet)
	}
}

func TestFinishReminderSetup_SoftExit(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), nil, 0, conversation.Data{
		conversation.UserIDKey: uint64(1),
		reminderActionKey:      reminderActionSoftExit,
	})
	if repo.upserted != nil || repo.disabled != 0 || repo.weeklySet != nil {
		t.Fatal("soft-exit must not touch the repo")
	}
}

func TestFinishReminderSetup_WeeklyActivateNoRow(t *testing.T) {
	// activating weekly for a user with NO reminders row must Upsert a minimal
	// row (SetWeeklySummary is UPDATE-only and would silently no-op).
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), nil, 0, conversation.Data{
		conversation.UserIDKey: uint64(8),
		reminderActionKey:      reminderActionWeeklyOnly,
		keyWeeklySummary:       "true",
		// keyHubHasRow absent => no row
	})
	if repo.upserted == nil || !repo.upserted.WeeklySummaryEnabled || repo.upserted.Enabled {
		t.Fatalf("expected minimal weekly-only upsert (weekly=true, enabled=false), got %+v", repo.upserted)
	}
	if repo.weeklySet != nil {
		t.Fatal("no-row activate must Upsert, not SetWeeklySummary")
	}
}

func TestFinishReminderSetup_WeeklyActivateHasRow(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), nil, 0, conversation.Data{
		conversation.UserIDKey: uint64(9),
		reminderActionKey:      reminderActionWeeklyOnly,
		keyWeeklySummary:       "true",
		keyHubHasRow:           "true",
	})
	if repo.weeklySet == nil || *repo.weeklySet != true || repo.weeklySetFor != 9 {
		t.Fatalf("expected SetWeeklySummary(9,true), got for=%d val=%v", repo.weeklySetFor, repo.weeklySet)
	}
	if repo.upserted != nil {
		t.Fatal("has-row activate must not Upsert")
	}
}

func hubOpt(t *testing.T, opts []conversation.ChoiceOption, value string) conversation.ChoiceOption {
	t.Helper()
	for _, o := range opts {
		if o.Value == value {
			return o
		}
	}
	t.Fatalf("option %q not found in %+v", value, opts)
	return conversation.ChoiceOption{}
}

func TestHubOptions_AllOff(t *testing.T) {
	opts := hubOptions(conversation.Data{})
	daily := hubOpt(t, opts, optionHubDaily)
	if !strings.Contains(daily.Label, "Activar recordatorio diario") {
		t.Errorf("daily-off label should say Activar recordatorio diario, got %q", daily.Label)
	}
	if daily.NextStep != stepReminderPickWindow {
		t.Errorf("daily option must route to the picker, got %q", daily.NextStep)
	}
	weekly := hubOpt(t, opts, optionWeeklyOn)
	if !strings.Contains(weekly.Label, "Activar resumen") || !weekly.Finish {
		t.Errorf("weekly-off toggle should say Activar and Finish, got %+v", weekly)
	}
	for _, o := range opts {
		if o.Value == optionOffAll {
			t.Error("Apagar todo must be hidden when nothing is on")
		}
	}
	_ = hubOpt(t, opts, optionSoftExit) // Salir always present
}

func TestHubOptions_AllOn(t *testing.T) {
	data := conversation.Data{
		keyHubDailyOn:    "true",
		reminderStartKey: "1200",
		reminderEndKey:   "1320",
		keyWeeklySummary: "true",
	}
	opts := hubOptions(data)
	daily := hubOpt(t, opts, optionHubDaily)
	if !strings.Contains(daily.Label, "Cambiar horario") || !strings.Contains(daily.Label, "20-22") {
		t.Errorf("daily-on label should show Cambiar horario (20-22), got %q", daily.Label)
	}
	weekly := hubOpt(t, opts, optionWeeklyOff)
	if !strings.Contains(weekly.Label, "Desactivar resumen") || !weekly.Finish {
		t.Errorf("weekly-on toggle should say Desactivar and Finish, got %+v", weekly)
	}
	off := hubOpt(t, opts, optionOffAll)
	if !off.Finish {
		t.Errorf("Apagar todo must Finish, got %+v", off)
	}
}

func TestOnReminderHub(t *testing.T) {
	cases := []struct {
		value      string
		wantAction string
		wantWeekly string // "" = flag absent
	}{
		{optionHubDaily, "", ""},
		{optionWeeklyOn, reminderActionWeeklyOnly, "true"},
		{optionWeeklyOff, reminderActionWeeklyOnly, "false"},
		{optionOffAll, reminderActionOffAll, ""},
		{optionSoftExit, reminderActionSoftExit, ""},
	}
	for _, c := range cases {
		got := onReminderHub(c.value, conversation.Data{})
		if stringOrEmpty(got[reminderActionKey]) != c.wantAction {
			t.Errorf("%s: action = %q, want %q", c.value, got[reminderActionKey], c.wantAction)
		}
		if stringOrEmpty(got[keyWeeklySummary]) != c.wantWeekly {
			t.Errorf("%s: weekly flag = %q, want %q", c.value, got[keyWeeklySummary], c.wantWeekly)
		}
	}
}

func TestSkipHubIfSeeded(t *testing.T) {
	if next, ok := skipHubIfSeeded(conversation.Data{keySkipHub: "true"}); !ok || next != stepReminderPickWindow {
		t.Errorf("seeded skipHub should skip to picker, got (%q,%v)", next, ok)
	}
	if _, ok := skipHubIfSeeded(conversation.Data{}); ok {
		t.Error("no seed => hub must not be skipped")
	}
}

func TestPickOptions_NoWeeklyButton(t *testing.T) {
	// The band picker must not carry the weekly-summary button anymore.
	flow := NewReminderSetupFlow() // panics if the graph is invalid
	_ = flow
	opts := reminderPickOptions(conversation.Data{})
	for _, o := range opts {
		if strings.Contains(o.Label, "Resumen semanal") {
			t.Errorf("picker must not show a weekly button, got %q", o.Label)
		}
	}
	found := false
	for _, o := range opts {
		if strings.Contains(o.Label, "Apagar recordatorio diario") {
			found = true
		}
	}
	if !found {
		t.Error("picker should label the off button 'Apagar recordatorio diario'")
	}
}

func TestSkipWeeklyUnlessAsked(t *testing.T) {
	if next, ok := skipWeeklyUnlessAsked(conversation.Data{}); !ok || next != "" {
		t.Errorf("hub entry (no askWeekly) must skip weekly and complete, got (%q,%v)", next, ok)
	}
	if _, ok := skipWeeklyUnlessAsked(conversation.Data{keyAskWeekly: "true"}); ok {
		t.Error("onboarding (askWeekly) must NOT skip the weekly step")
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
