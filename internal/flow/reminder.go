package flow

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"lopiibot.com/internal/conversation"
)

var errBadWindow = errors.New("reminder: bad window")

// twoHours grabs the first two 1–2 digit numbers in the text ("de 9 a 13",
// "20-22", "entre las 8 y las 10"). The leading (?:^|[^-\d]) requires each
// number to start clean (string start or a non-digit, non-minus char) so a
// negative like "-1" isn't misread as "1". ponytail: 24h whole-hours only, no
// am/pm NLP — the preset bands cover the common intent; upgrade only if users ask.
var twoHours = regexp.MustCompile(`(?:^|[^-\d])(\d{1,2})\D+(\d{1,2})`)

// ParseWindow turns free text into a [start,end) window in minutes since
// midnight. Whole hours, 0–23, start strictly before end.
func ParseWindow(text string) (startMin, endMin int, err error) {
	m := twoHours.FindStringSubmatch(text)
	if m == nil {
		return 0, 0, errBadWindow
	}
	h1, _ := strconv.Atoi(m[1])
	h2, _ := strconv.Atoi(m[2])
	if h1 < 0 || h1 > 23 || h2 < 0 || h2 > 23 || h1 >= h2 {
		return 0, 0, errBadWindow
	}
	return h1 * 60, h2 * 60, nil
}

// Keys y valores de estado del flow. Exportados porque el borde los seeda y
// los lee: startReminderSetup arma el panel (keyHub*), finishReminderSetup
// aplica la acción (ReminderActionKey/StartKey/EndKey/CustomKey).
const (
	StepReminderHub          = "reminder_hub"
	StepReminderPickWindow   = "reminder_pick_window"
	StepReminderCustomWindow = "reminder_custom_window"
	StepReminderWeekly       = "reminder_weekly"

	ReminderActionKey        = "reminder_action" // "set" | "disable" | "weekly_only"
	ReminderStartKey         = "reminder_start_min"
	ReminderEndKey           = "reminder_end_min"
	ReminderCustomKey        = "reminder_custom_raw"
	ReminderActionSet        = "set"
	ReminderActionOff        = "disable"
	ReminderActionWeeklyOnly = "weekly_only"
	ReminderActionOffAll     = "off_all"
	ReminderActionSoftExit   = "soft_exit"
	OptionReminderOff        = "reminder_off"
	OptionReminderOther      = "reminder_other"
	OptionWeeklyOn           = "weekly_on"
	OptionWeeklyOff          = "weekly_off"
	OptionHubDaily           = "hub_daily"
	OptionOffAll             = "hub_off_all"
	OptionSoftExit           = "hub_exit"

	KeyHubHasRow  = "hub_has_row"  // seeded true when the user already has a reminders row
	KeySkipHub    = "skip_hub"     // onboarding: skip the hub, go straight to the picker
	KeyAskWeekly  = "ask_weekly"   // onboarding: show the weekly Sí/No after the band
	KeyHubDailyOn = "hub_daily_on" // seeded: daily reminder currently enabled
)

// ReminderPresets: window presets, "startMin-endMin" encoded in the option Value.
var ReminderPresets = []conversation.ChoiceOption{
	{Label: "🌅 Mañana (8 a 10)", Value: "480-600", NextStep: StepReminderWeekly},
	{Label: "☀️ Mediodía (12 a 14)", Value: "720-840", NextStep: StepReminderWeekly},
	{Label: "🌆 Tarde (16 a 18)", Value: "960-1080", NextStep: StepReminderWeekly},
	{Label: "🌙 Noche (20 a 22)", Value: "1200-1320", NextStep: StepReminderWeekly},
}

// OnReminderPickWindow records the chosen action/window into Data. Presets and
// "apagar" finish the flow; "otro horario" advances to the text step; cancelar
// flags cancellation.
func OnReminderPickWindow(value string, data conversation.Data) conversation.Data {
	next := conversation.CopyData(data)
	switch value {
	case OptionCancel:
		conversation.SetFlag(next, conversation.KeyCancelled)
	case OptionReminderOff:
		next[ReminderActionKey] = ReminderActionOff
	case OptionReminderOther:
		// no data; advances to the custom text step
	default: // a preset "start-end"
		start, end := SplitPreset(value)
		next[ReminderActionKey] = ReminderActionSet
		next[ReminderStartKey] = strconv.Itoa(start)
		next[ReminderEndKey] = strconv.Itoa(end)
	}
	return next
}

// HubOptions builds the root "Notificaciones" panel buttons from the seeded
// current state: dynamic daily label, weekly toggle, and "Apagar todo" only
// when something is actually on.
func HubOptions(data conversation.Data) []conversation.ChoiceOption {
	opts := make([]conversation.ChoiceOption, 0, 4)

	dailyOn := conversation.Flag(data, KeyHubDailyOn)
	if dailyOn {
		s, _ := strconv.Atoi(conversation.StringOrEmpty(data[ReminderStartKey]))
		e, _ := strconv.Atoi(conversation.StringOrEmpty(data[ReminderEndKey]))
		opts = append(opts, conversation.ChoiceOption{
			Label:    fmt.Sprintf("🌙 Cambiar horario (%d-%d)", s/60, e/60),
			Value:    OptionHubDaily,
			NextStep: StepReminderPickWindow,
		})
	} else {
		opts = append(opts, conversation.ChoiceOption{
			Label:    "🌙 Activar recordatorio diario",
			Value:    OptionHubDaily,
			NextStep: StepReminderPickWindow,
		})
	}

	weeklyOn := conversation.Flag(data, conversation.KeyWeeklySummary)
	if weeklyOn {
		opts = append(opts, conversation.ChoiceOption{Label: "📊 Desactivar resumen semanal", Value: OptionWeeklyOff, Finish: true})
	} else {
		opts = append(opts, conversation.ChoiceOption{Label: "📊 Activar resumen semanal", Value: OptionWeeklyOn, Finish: true})
	}

	if dailyOn || weeklyOn {
		opts = append(opts, conversation.ChoiceOption{Label: "🔕 Apagar todo", Value: OptionOffAll, Finish: true})
	}
	opts = append(opts, conversation.ChoiceOption{Label: "🚫 Salir", Value: OptionSoftExit, Finish: true})
	return opts
}

// OnReminderHub records the hub choice. Daily routes to the picker (no action
// yet). Weekly toggles set weekly_only + the explicit on/off flag. Off-all and
// soft-exit set their terminal actions.
func OnReminderHub(value string, data conversation.Data) conversation.Data {
	next := conversation.CopyData(data)
	switch value {
	case OptionHubDaily:
		// no action; advances to the picker which sets window/action
	case OptionWeeklyOn:
		next[ReminderActionKey] = ReminderActionWeeklyOnly
		conversation.SetFlag(next, conversation.KeyWeeklySummary)
	case OptionWeeklyOff:
		next[ReminderActionKey] = ReminderActionWeeklyOnly
		next[conversation.KeyWeeklySummary] = "false"
	case OptionOffAll:
		next[ReminderActionKey] = ReminderActionOffAll
	case OptionSoftExit:
		next[ReminderActionKey] = ReminderActionSoftExit
	}
	return next
}

// SkipHubIfSeeded skips the hub screen when the onboarding entry seeded skipHub,
// landing straight on the band picker.
func SkipHubIfSeeded(data conversation.Data) (string, bool) {
	if conversation.Flag(data, KeySkipHub) {
		return StepReminderPickWindow, true
	}
	return "", false
}

// MsgReminderHub renders the compact "Notificaciones" status panel from seeded
// state. (describeReminder in query.go is intentionally NOT reused — it's the
// verbose QUERY-intent format, wrong for a two-line panel.)
func MsgReminderHub(data conversation.Data) string {
	daily := "❌ desactivado"
	if conversation.Flag(data, KeyHubDailyOn) {
		s, _ := strconv.Atoi(conversation.StringOrEmpty(data[ReminderStartKey]))
		e, _ := strconv.Atoi(conversation.StringOrEmpty(data[ReminderEndKey]))
		daily = fmt.Sprintf("✅ %d a %d hs", s/60, e/60)
	}
	weekly := "❌ desactivado"
	if conversation.Flag(data, conversation.KeyWeeklySummary) {
		weekly = "✅ los lunes"
	}
	return fmt.Sprintf("🔔 Notificaciones\n\nRecordatorio diario: %s\nResumen semanal: %s\n\n¿Qué querés hacer?", daily, weekly)
}

// OnReminderWeekly records the weekly-summary yes/no into Data.
func OnReminderWeekly(value string, data conversation.Data) conversation.Data {
	next := conversation.CopyData(data)
	if value == OptionWeeklyOn {
		conversation.SetFlag(next, conversation.KeyWeeklySummary)
	}
	return next
}

// SplitPreset parses "480-600" (our own constants, always well-formed).
func SplitPreset(v string) (start, end int) {
	for i := 0; i < len(v); i++ {
		if v[i] == '-' {
			start, _ = strconv.Atoi(v[:i])
			end, _ = strconv.Atoi(v[i+1:])
			return start, end
		}
	}
	return 0, 0
}

// ReminderPickOptions builds the band-picker buttons: presets, custom, "apagar
// recordatorio diario" (daily only), cancel. NO weekly button — the weekly
// summary is managed from the hub, never mixed into the band keyboard.
func ReminderPickOptions(conversation.Data) []conversation.ChoiceOption {
	opts := make([]conversation.ChoiceOption, 0, len(ReminderPresets)+3)
	opts = append(opts, ReminderPresets...)
	opts = append(opts,
		conversation.ChoiceOption{Label: "⌨️ Otro horario", Value: OptionReminderOther, NextStep: StepReminderCustomWindow},
		conversation.ChoiceOption{Label: "🔕 Apagar recordatorio diario", Value: OptionReminderOff, Finish: true},
		CancelOption,
	)
	return opts
}

// SkipWeeklyUnlessAsked skips the weekly Sí/No unless the onboarding entry
// seeded askWeekly. Hub entries never re-ask weekly (they toggle it from the
// hub); returning ("", true) completes the flow.
func SkipWeeklyUnlessAsked(data conversation.Data) (string, bool) {
	if conversation.Flag(data, KeyAskWeekly) {
		return "", false
	}
	return "", true
}

// NewReminderSetupFlow builds the deterministic reminder-config flow: a hub
// screen, a band picker (presets / custom / apagar / cancelar), a custom-window
// text step, and a gated weekly step. No LLM call — the flow captures
// everything by button/text.
func NewReminderSetupFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		StepReminderHub: conversation.ChoiceStep{
			PromptText:           MsgReminderHub,
			OptionsFunc:          HubOptions,
			DeclaredNextSteps:    []string{StepReminderPickWindow},
			OnChoice:             OnReminderHub,
			InvalidChoiceMessage: MsgInvalidChoice,
			SkipIf:               SkipHubIfSeeded,
		},
		StepReminderPickWindow: conversation.ChoiceStep{
			PromptText:           func(conversation.Data) string { return MsgAskReminderWindow },
			OptionsFunc:          ReminderPickOptions,
			DeclaredNextSteps:    []string{StepReminderCustomWindow, StepReminderWeekly},
			OnChoice:             OnReminderPickWindow,
			InvalidChoiceMessage: MsgInvalidChoice,
		},
		StepReminderCustomWindow: conversation.TextStep{
			PromptText: func(conversation.Data) string { return MsgAskReminderCustomWindow },
			DataKey:    ReminderCustomKey,
			Validate: func(text string, _ conversation.Data) string {
				if _, _, err := ParseWindow(text); err != nil {
					return MsgInvalidReminderWindow
				}
				return ""
			},
			NextStep:      StepReminderWeekly,
			EscapeOptions: []conversation.ChoiceOption{CancelOption},
			OnEscape: func(value string, data conversation.Data) conversation.Data {
				if value != OptionCancel {
					return data
				}
				next := conversation.CopyData(data)
				conversation.SetFlag(next, conversation.KeyCancelled)
				return next
			},
		},
		StepReminderWeekly: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return MsgAskWeeklySummary },
			Options: []conversation.ChoiceOption{
				{Label: "✅ Sí, dale", Value: OptionWeeklyOn, Finish: true},
				{Label: "No por ahora", Value: OptionWeeklyOff, Finish: true},
			},
			OnChoice:             OnReminderWeekly,
			InvalidChoiceMessage: MsgInvalidChoice,
			SkipIf:               SkipWeeklyUnlessAsked,
		},
	}

	flow, err := conversation.NewFlow(ReminderSetupFlowName, StepReminderHub, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
