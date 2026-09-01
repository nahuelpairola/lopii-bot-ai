package messaging

import (
	"context"
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
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
		s, e, err := flow.ParseWindow(c.in)
		if err != nil || s != c.wantStart || e != c.wantEnd {
			t.Errorf("flow.ParseWindow(%q) = (%d,%d,%v), want (%d,%d,nil)", c.in, s, e, err, c.wantStart, c.wantEnd)
		}
	}
	bad := []string{"", "21", "abc", "25 a 26", "21 a 20", "20 a 20", "-1 a 5"}
	for _, in := range bad {
		if _, _, err := flow.ParseWindow(in); err == nil {
			t.Errorf("flow.ParseWindow(%q) expected error, got nil", in)
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
		flow.ReminderActionKey: flow.ReminderActionSet,
		flow.ReminderStartKey:  "1200",
		flow.ReminderEndKey:    "1320",
	}
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, data)
	if repo.upserted == nil || repo.upserted.WindowStartMin != 1200 || repo.upserted.WindowEndMin != 1320 || !repo.upserted.Enabled || repo.upserted.UserID != 42 {
		t.Fatalf("unexpected upsert: %+v", repo.upserted)
	}
}

func TestFinishReminderSetup_Custom(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	data := conversation.Data{
		conversation.UserIDKey: uint64(7),
		flow.ReminderActionKey: flow.ReminderActionSet,
		flow.ReminderCustomKey: "9 a 13",
	}
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, data)
	if repo.upserted == nil || repo.upserted.WindowStartMin != 540 || repo.upserted.WindowEndMin != 780 {
		t.Fatalf("unexpected upsert: %+v", repo.upserted)
	}
}

func TestFinishReminderSetup_Disable(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	data := conversation.Data{
		conversation.UserIDKey: uint64(99),
		flow.ReminderActionKey: flow.ReminderActionOff,
	}
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, data)
	if repo.disabled != 99 {
		t.Fatalf("expected Disable(99), got %d", repo.disabled)
	}
}

func TestFinishReminderSetup_Cancelled(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, conversation.Data{
		conversation.UserIDKey: uint64(1),
		"cancelled":            "true",
	})
	if repo.upserted != nil || repo.disabled != 0 {
		t.Fatal("cancelled path must not touch the repo")
	}
}

func TestNewReminderSetupFlow_Valid(t *testing.T) {
	_ = flow.NewReminderSetupFlow()
}

func TestReminderSetup_PresetThenWeeklyYes(t *testing.T) {
	data := flow.OnReminderPickWindow("1200-1320", conversation.Data{conversation.UserIDKey: uint64(42)})
	data = flow.OnReminderWeekly(flow.OptionWeeklyOn, data)

	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, data)

	if repo.upserted == nil || !repo.upserted.WeeklySummaryEnabled {
		t.Fatalf("expected upsert with WeeklySummaryEnabled=true, got %+v", repo.upserted)
	}
	if repo.upserted.WindowStartMin != 1200 || repo.upserted.WindowEndMin != 1320 {
		t.Fatalf("unexpected window: %+v", repo.upserted)
	}
}

func TestReminderSetup_WeeklyOnlyOff(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, conversation.Data{
		conversation.UserIDKey: uint64(7),
		flow.ReminderActionKey: flow.ReminderActionWeeklyOnly,
		flow.KeyHubHasRow:      "true",
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
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, conversation.Data{
		conversation.UserIDKey: uint64(42),
		flow.ReminderActionKey: flow.ReminderActionOffAll,
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
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, conversation.Data{
		conversation.UserIDKey: uint64(1),
		flow.ReminderActionKey: flow.ReminderActionSoftExit,
	})
	if repo.upserted != nil || repo.disabled != 0 || repo.weeklySet != nil {
		t.Fatal("soft-exit must not touch the repo")
	}
}

func TestFinishReminderSetup_WeeklyActivateNoRow(t *testing.T) {
	repo := &fakeReminderRepo{}
	c := &controller{reminders: repo}
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, conversation.Data{
		conversation.UserIDKey:        uint64(8),
		flow.ReminderActionKey:        flow.ReminderActionWeeklyOnly,
		conversation.KeyWeeklySummary: "true",
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
	c.finishReminderSetup(context.Background(), &messenger.FakeChat{}, conversation.Data{
		conversation.UserIDKey:        uint64(9),
		flow.ReminderActionKey:        flow.ReminderActionWeeklyOnly,
		conversation.KeyWeeklySummary: "true",
		flow.KeyHubHasRow:             "true",
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
	opts := flow.HubOptions(conversation.Data{})
	daily := hubOpt(t, opts, flow.OptionHubDaily)
	if !strings.Contains(daily.Label, "Activar recordatorio diario") {
		t.Errorf("daily-off label should say Activar recordatorio diario, got %q", daily.Label)
	}
	if daily.NextStep != flow.StepReminderPickWindow {
		t.Errorf("daily option must route to the picker, got %q", daily.NextStep)
	}
	weekly := hubOpt(t, opts, flow.OptionWeeklyOn)
	if !strings.Contains(weekly.Label, "Activar resumen") || !weekly.Finish {
		t.Errorf("weekly-off toggle should say Activar and Finish, got %+v", weekly)
	}
	for _, o := range opts {
		if o.Value == flow.OptionOffAll {
			t.Error("Apagar todo must be hidden when nothing is on")
		}
	}
	_ = hubOpt(t, opts, flow.OptionSoftExit)
}

func TestHubOptions_AllOn(t *testing.T) {
	data := conversation.Data{
		flow.KeyHubDailyOn:            "true",
		flow.ReminderStartKey:         "1200",
		flow.ReminderEndKey:           "1320",
		conversation.KeyWeeklySummary: "true",
	}
	opts := flow.HubOptions(data)
	daily := hubOpt(t, opts, flow.OptionHubDaily)
	if !strings.Contains(daily.Label, "Cambiar horario") || !strings.Contains(daily.Label, "20-22") {
		t.Errorf("daily-on label should show Cambiar horario (20-22), got %q", daily.Label)
	}
	weekly := hubOpt(t, opts, flow.OptionWeeklyOff)
	if !strings.Contains(weekly.Label, "Desactivar resumen") || !weekly.Finish {
		t.Errorf("weekly-on toggle should say Desactivar and Finish, got %+v", weekly)
	}
	off := hubOpt(t, opts, flow.OptionOffAll)
	if !off.Finish {
		t.Errorf("Apagar todo must Finish, got %+v", off)
	}
}

func TestOnReminderHub(t *testing.T) {
	cases := []struct {
		value      string
		wantAction string
		wantWeekly string
	}{
		{flow.OptionHubDaily, "", ""},
		{flow.OptionWeeklyOn, flow.ReminderActionWeeklyOnly, "true"},
		{flow.OptionWeeklyOff, flow.ReminderActionWeeklyOnly, "false"},
		{flow.OptionOffAll, flow.ReminderActionOffAll, ""},
		{flow.OptionSoftExit, flow.ReminderActionSoftExit, ""},
	}
	for _, c := range cases {
		got := flow.OnReminderHub(c.value, conversation.Data{})
		if conversation.StringOrEmpty(got[flow.ReminderActionKey]) != c.wantAction {
			t.Errorf("%s: action = %q, want %q", c.value, got[flow.ReminderActionKey], c.wantAction)
		}
		if conversation.StringOrEmpty(got[conversation.KeyWeeklySummary]) != c.wantWeekly {
			t.Errorf("%s: weekly flag = %q, want %q", c.value, got[conversation.KeyWeeklySummary], c.wantWeekly)
		}
	}
}

func TestSkipHubIfSeeded(t *testing.T) {
	if next, ok := flow.SkipHubIfSeeded(conversation.Data{flow.KeySkipHub: "true"}); !ok || next != flow.StepReminderPickWindow {
		t.Errorf("seeded skipHub should skip to picker, got (%q,%v)", next, ok)
	}
	if _, ok := flow.SkipHubIfSeeded(conversation.Data{}); ok {
		t.Error("no seed => hub must not be skipped")
	}
}

func TestPickOptions_NoWeeklyButton(t *testing.T) {
	fl := flow.NewReminderSetupFlow()
	_ = fl
	opts := flow.ReminderPickOptions(conversation.Data{})
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
	if next, ok := flow.SkipWeeklyUnlessAsked(conversation.Data{}); !ok || next != "" {
		t.Errorf("hub entry (no askWeekly) must skip weekly and complete, got (%q,%v)", next, ok)
	}
	if _, ok := flow.SkipWeeklyUnlessAsked(conversation.Data{flow.KeyAskWeekly: "true"}); ok {
		t.Error("onboarding (askWeekly) must NOT skip the weekly step")
	}
}

func newReminderTestEngine() (*conversation.Engine, *fakeStateStore) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "los recordatorios" })
	engine.Register(flow.NewReminderSetupFlow())
	return engine, store
}

func TestReminderFlow_HubBandChange_SkipsWeekly(t *testing.T) {
	engine, _ := newReminderTestEngine()
	seed := conversation.Data{conversation.KeyWeeklySummary: "true", flow.KeyHubHasRow: "true"}
	if _, err := engine.StartWithData(1, flow.ReminderSetupFlowName, seed); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionHubDaily}); err != nil {
		t.Fatalf("hub choice: %v", err)
	}
	res, _, err := engine.Handle(1, conversation.Input{CallbackData: "1200-1320"})
	if err != nil {
		t.Fatalf("preset: %v", err)
	}
	if !res.Finished {
		t.Fatal("hub band change must finish without asking the weekly question")
	}
	if conversation.StringOrEmpty(res.Data[conversation.KeyWeeklySummary]) != "true" {
		t.Errorf("weekly flag must be preserved through a band change, got %q", res.Data[conversation.KeyWeeklySummary])
	}
	if conversation.StringOrEmpty(res.Data[flow.ReminderActionKey]) != flow.ReminderActionSet {
		t.Errorf("expected action=set, got %q", res.Data[flow.ReminderActionKey])
	}
}

func TestReminderFlow_Onboarding_SkipsHubShowsWeekly(t *testing.T) {
	engine, _ := newReminderTestEngine()
	seed := conversation.Data{}
	conversation.SetFlag(seed, flow.KeySkipHub)
	conversation.SetFlag(seed, flow.KeyAskWeekly)
	prompt, err := engine.StartWithData(1, flow.ReminderSetupFlowName, seed)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !strings.Contains(prompt.Text, "franja") {
		t.Fatalf("onboarding must land on the band picker, got prompt %q", prompt.Text)
	}
	res, _, err := engine.Handle(1, conversation.Input{CallbackData: "1200-1320"})
	if err != nil {
		t.Fatalf("preset: %v", err)
	}
	if res.Finished {
		t.Fatal("onboarding must show the weekly question after the band, not finish")
	}
	if !strings.Contains(res.Prompt.Text, "resumen") {
		t.Errorf("expected the weekly-summary question, got %q", res.Prompt.Text)
	}
}
