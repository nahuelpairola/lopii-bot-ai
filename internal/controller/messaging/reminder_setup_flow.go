package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

var errBadWindow = errors.New("reminder: bad window")

// twoHours grabs the first two 1–2 digit numbers in the text ("de 9 a 13",
// "20-22", "entre las 8 y las 10"). The leading (?:^|[^-\d]) requires each
// number to start clean (string start or a non-digit, non-minus char) so a
// negative like "-1" isn't misread as "1". ponytail: 24h whole-hours only, no
// am/pm NLP — the preset bands cover the common intent; upgrade only if users ask.
var twoHours = regexp.MustCompile(`(?:^|[^-\d])(\d{1,2})\D+(\d{1,2})`)

// parseWindow turns free text into a [start,end) window in minutes since
// midnight. Whole hours, 0–23, start strictly before end.
func parseWindow(text string) (startMin, endMin int, err error) {
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

const (
	reminderSetupFlowName = "reminder_setup"

	stepReminderHub          = "reminder_hub"
	stepReminderPickWindow   = "reminder_pick_window"
	stepReminderCustomWindow = "reminder_custom_window"
	stepReminderWeekly       = "reminder_weekly"

	reminderActionKey        = "reminder_action" // "set" | "disable" | "weekly_only"
	reminderStartKey         = "reminder_start_min"
	reminderEndKey           = "reminder_end_min"
	reminderCustomKey        = "reminder_custom_raw"
	reminderActionSet        = "set"
	reminderActionOff        = "disable"
	reminderActionWeeklyOnly = "weekly_only"
	reminderActionOffAll     = "off_all"
	reminderActionSoftExit   = "soft_exit"
	optionReminderOff        = "reminder_off"
	optionReminderOther      = "reminder_other"
	optionWeeklyOn           = "weekly_on"
	optionWeeklyOff          = "weekly_off"
	optionHubDaily           = "hub_daily"
	optionOffAll             = "hub_off_all"
	optionSoftExit           = "hub_exit"

	keyHubHasRow  = "hub_has_row"  // seeded true when the user already has a reminders row
	keySkipHub    = "skip_hub"     // onboarding: skip the hub, go straight to the picker
	keyAskWeekly  = "ask_weekly"   // onboarding: show the weekly Sí/No after the band
	keyHubDailyOn = "hub_daily_on" // seeded: daily reminder currently enabled
)

// window presets, "startMin-endMin" encoded in the option Value.
var reminderPresets = []conversation.ChoiceOption{
	{Label: "🌅 Mañana (8 a 10)", Value: "480-600", NextStep: stepReminderWeekly},
	{Label: "☀️ Mediodía (12 a 14)", Value: "720-840", NextStep: stepReminderWeekly},
	{Label: "🌆 Tarde (16 a 18)", Value: "960-1080", NextStep: stepReminderWeekly},
	{Label: "🌙 Noche (20 a 22)", Value: "1200-1320", NextStep: stepReminderWeekly},
}

// onReminderPickWindow records the chosen action/window into Data. Presets and
// "apagar" finish the flow; "otro horario" advances to the text step; cancelar
// flags cancellation.
func onReminderPickWindow(value string, data conversation.Data) conversation.Data {
	next := copyData(data)
	switch value {
	case optionCancel:
		setFlag(next, keyCancelled)
	case optionReminderOff:
		next[reminderActionKey] = reminderActionOff
	case optionReminderOther:
		// no data; advances to the custom text step
	default: // a preset "start-end"
		start, end := splitPreset(value)
		next[reminderActionKey] = reminderActionSet
		next[reminderStartKey] = strconv.Itoa(start)
		next[reminderEndKey] = strconv.Itoa(end)
	}
	return next
}

// hubOptions builds the root "Notificaciones" panel buttons from the seeded
// current state: dynamic daily label, weekly toggle, and "Apagar todo" only
// when something is actually on.
func hubOptions(data conversation.Data) []conversation.ChoiceOption {
	opts := make([]conversation.ChoiceOption, 0, 4)

	dailyOn := flag(data, keyHubDailyOn)
	if dailyOn {
		s, _ := strconv.Atoi(stringOrEmpty(data[reminderStartKey]))
		e, _ := strconv.Atoi(stringOrEmpty(data[reminderEndKey]))
		opts = append(opts, conversation.ChoiceOption{
			Label:    fmt.Sprintf("🌙 Cambiar horario (%d-%d)", s/60, e/60),
			Value:    optionHubDaily,
			NextStep: stepReminderPickWindow,
		})
	} else {
		opts = append(opts, conversation.ChoiceOption{
			Label:    "🌙 Activar recordatorio diario",
			Value:    optionHubDaily,
			NextStep: stepReminderPickWindow,
		})
	}

	weeklyOn := flag(data, keyWeeklySummary)
	if weeklyOn {
		opts = append(opts, conversation.ChoiceOption{Label: "📊 Desactivar resumen semanal", Value: optionWeeklyOff, Finish: true})
	} else {
		opts = append(opts, conversation.ChoiceOption{Label: "📊 Activar resumen semanal", Value: optionWeeklyOn, Finish: true})
	}

	if dailyOn || weeklyOn {
		opts = append(opts, conversation.ChoiceOption{Label: "🔕 Apagar todo", Value: optionOffAll, Finish: true})
	}
	opts = append(opts, conversation.ChoiceOption{Label: "🚫 Salir", Value: optionSoftExit, Finish: true})
	return opts
}

// onReminderHub records the hub choice. Daily routes to the picker (no action
// yet). Weekly toggles set weekly_only + the explicit on/off flag. Off-all and
// soft-exit set their terminal actions.
func onReminderHub(value string, data conversation.Data) conversation.Data {
	next := copyData(data)
	switch value {
	case optionHubDaily:
		// no action; advances to the picker which sets window/action
	case optionWeeklyOn:
		next[reminderActionKey] = reminderActionWeeklyOnly
		setFlag(next, keyWeeklySummary)
	case optionWeeklyOff:
		next[reminderActionKey] = reminderActionWeeklyOnly
		next[keyWeeklySummary] = "false"
	case optionOffAll:
		next[reminderActionKey] = reminderActionOffAll
	case optionSoftExit:
		next[reminderActionKey] = reminderActionSoftExit
	}
	return next
}

// skipHubIfSeeded skips the hub screen when the onboarding entry seeded skipHub,
// landing straight on the band picker.
func skipHubIfSeeded(data conversation.Data) (string, bool) {
	if flag(data, keySkipHub) {
		return stepReminderPickWindow, true
	}
	return "", false
}

// msgReminderHub renders the compact "Notificaciones" status panel from seeded
// state. (describeReminder in query.go is intentionally NOT reused — it's the
// verbose QUERY-intent format, wrong for a two-line panel.)
func msgReminderHub(data conversation.Data) string {
	daily := "❌ desactivado"
	if flag(data, keyHubDailyOn) {
		s, _ := strconv.Atoi(stringOrEmpty(data[reminderStartKey]))
		e, _ := strconv.Atoi(stringOrEmpty(data[reminderEndKey]))
		daily = fmt.Sprintf("✅ %d a %d hs", s/60, e/60)
	}
	weekly := "❌ desactivado"
	if flag(data, keyWeeklySummary) {
		weekly = "✅ los lunes"
	}
	return fmt.Sprintf("🔔 Notificaciones\n\nRecordatorio diario: %s\nResumen semanal: %s\n\n¿Qué querés hacer?", daily, weekly)
}

// onReminderWeekly records the weekly-summary yes/no into Data.
func onReminderWeekly(value string, data conversation.Data) conversation.Data {
	next := copyData(data)
	if value == optionWeeklyOn {
		setFlag(next, keyWeeklySummary)
	}
	return next
}

// splitPreset parses "480-600" (our own constants, always well-formed).
func splitPreset(v string) (start, end int) {
	for i := 0; i < len(v); i++ {
		if v[i] == '-' {
			start, _ = strconv.Atoi(v[:i])
			end, _ = strconv.Atoi(v[i+1:])
			return start, end
		}
	}
	return 0, 0
}

// reminderPickOptions builds the band-picker buttons: presets, custom, "apagar
// recordatorio diario" (daily only), cancel. NO weekly button — the weekly
// summary is managed from the hub, never mixed into the band keyboard.
func reminderPickOptions(conversation.Data) []conversation.ChoiceOption {
	opts := make([]conversation.ChoiceOption, 0, len(reminderPresets)+3)
	opts = append(opts, reminderPresets...)
	opts = append(opts,
		conversation.ChoiceOption{Label: "⌨️ Otro horario", Value: optionReminderOther, NextStep: stepReminderCustomWindow},
		conversation.ChoiceOption{Label: "🔕 Apagar recordatorio diario", Value: optionReminderOff, Finish: true},
		cancelOption,
	)
	return opts
}

// skipWeeklyUnlessAsked skips the weekly Sí/No unless the onboarding entry
// seeded askWeekly. Hub entries never re-ask weekly (they toggle it from the
// hub); returning ("", true) completes the flow.
func skipWeeklyUnlessAsked(data conversation.Data) (string, bool) {
	if flag(data, keyAskWeekly) {
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
		stepReminderHub: conversation.ChoiceStep{
			PromptText:           msgReminderHub,
			OptionsFunc:          hubOptions,
			DeclaredNextSteps:    []string{stepReminderPickWindow},
			OnChoice:             onReminderHub,
			InvalidChoiceMessage: msgGenericFlowError,
			SkipIf:               skipHubIfSeeded,
		},
		stepReminderPickWindow: conversation.ChoiceStep{
			PromptText:           func(conversation.Data) string { return msgAskReminderWindow },
			OptionsFunc:          reminderPickOptions,
			DeclaredNextSteps:    []string{stepReminderCustomWindow, stepReminderWeekly},
			OnChoice:             onReminderPickWindow,
			InvalidChoiceMessage: msgGenericFlowError,
		},
		stepReminderCustomWindow: conversation.TextStep{
			PromptText: func(conversation.Data) string { return msgAskReminderCustomWindow },
			DataKey:    reminderCustomKey,
			Validate: func(text string, _ conversation.Data) string {
				if _, _, err := parseWindow(text); err != nil {
					return msgInvalidReminderWindow
				}
				return ""
			},
			NextStep:      stepReminderWeekly,
			EscapeOptions: []conversation.ChoiceOption{cancelOption},
			OnEscape: func(value string, data conversation.Data) conversation.Data {
				if value != optionCancel {
					return data
				}
				next := copyData(data)
				setFlag(next, keyCancelled)
				return next
			},
		},
		stepReminderWeekly: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return msgAskWeeklySummary },
			Options: []conversation.ChoiceOption{
				{Label: "✅ Sí, dale", Value: optionWeeklyOn, Finish: true},
				{Label: "No por ahora", Value: optionWeeklyOff, Finish: true},
			},
			OnChoice:             onReminderWeekly,
			InvalidChoiceMessage: msgGenericFlowError,
			SkipIf:               skipWeeklyUnlessAsked,
		},
	}

	flow, err := conversation.NewFlow(reminderSetupFlowName, stepReminderHub, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// startReminderSetup opens the Notificaciones hub: it reads the user's current
// reminder (if any) and seeds the panel state, so the root step can render both
// features' status and preserve the weekly flag when only the window changes.
func (c *controller) startReminderSetup(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", reminderSetupFlowName, "user_id", userID)
	seed := conversation.Data{}
	if rem, err := c.reminders.FindByUserID(userID); err == nil && rem != nil {
		setFlag(seed, keyHubHasRow)
		if rem.Enabled {
			setFlag(seed, keyHubDailyOn)
			seed[reminderStartKey] = strconv.Itoa(rem.WindowStartMin)
			seed[reminderEndKey] = strconv.Itoa(rem.WindowEndMin)
		}
		if rem.WeeklySummaryEnabled {
			setFlag(seed, keyWeeklySummary)
		}
	}
	prompt, err := c.engine.StartWithData(userID, reminderSetupFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("start reminder_setup flow: %w", err)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}
