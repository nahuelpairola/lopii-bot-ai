package messaging

import (
	"context"
	"errors"
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

	stepReminderPickWindow   = "reminder_pick_window"
	stepReminderCustomWindow = "reminder_custom_window"
	stepReminderWeekly       = "reminder_weekly"
	stepReminderDone         = "reminder_done" // terminal skip step (never rendered)

	reminderActionKey        = "reminder_action" // "set" | "disable" | "weekly_only"
	reminderStartKey         = "reminder_start_min"
	reminderEndKey           = "reminder_end_min"
	reminderCustomKey        = "reminder_custom_raw"
	reminderActionSet        = "set"
	reminderActionOff        = "disable"
	reminderActionWeeklyOnly = "weekly_only"
	optionReminderOff        = "reminder_off"
	optionReminderOther      = "reminder_other"
	optionWeeklyManage       = "weekly_manage"
	optionWeeklyOn           = "weekly_on"
	optionWeeklyOff          = "weekly_off"
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
	case optionWeeklyManage:
		next[reminderActionKey] = reminderActionWeeklyOnly
	default: // a preset "start-end"
		start, end := splitPreset(value)
		next[reminderActionKey] = reminderActionSet
		next[reminderStartKey] = strconv.Itoa(start)
		next[reminderEndKey] = strconv.Itoa(end)
	}
	return next
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

// NewReminderSetupFlow builds the deterministic reminder-config flow: a band
// picker (presets / custom / apagar / cancelar), a custom-window text step, and
// a terminal skip step. No LLM call — the flow captures everything by button/text.
func NewReminderSetupFlow() *conversation.Flow {
	pickOptions := func(conversation.Data) []conversation.ChoiceOption {
		opts := make([]conversation.ChoiceOption, 0, len(reminderPresets)+3)
		opts = append(opts, reminderPresets...)
		opts = append(opts,
			conversation.ChoiceOption{Label: "⌨️ Otro horario", Value: optionReminderOther, NextStep: stepReminderCustomWindow},
			conversation.ChoiceOption{Label: "🔕 Apagar recordatorio", Value: optionReminderOff, Finish: true},
			conversation.ChoiceOption{Label: "📊 Resumen semanal", Value: optionWeeklyManage, NextStep: stepReminderWeekly},
			cancelOption,
		)
		return opts
	}

	steps := map[string]conversation.Step{
		stepReminderPickWindow: conversation.ChoiceStep{
			PromptText:           func(conversation.Data) string { return msgAskReminderWindow },
			OptionsFunc:          pickOptions,
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
		},
		stepReminderDone: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return "" }, // never rendered
			SkipIf:     func(conversation.Data) (string, bool) { return "", true },
		},
	}

	flow, err := conversation.NewFlow(reminderSetupFlowName, stepReminderPickWindow, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// startReminderSetup starts reminder_setup fresh — like startAccountCreate,
// there's no seed: the flow captures the window by button/text.
func (c *controller) startReminderSetup(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) {
	slog.InfoContext(ctx, "flow started", "flow", reminderSetupFlowName, "user_id", userID)
	prompt, err := c.engine.Start(userID, reminderSetupFlowName)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
}
