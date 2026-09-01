package flow

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"lopiibot.com/internal/conversation"
)

var errBadWindow = errors.New("reminder: bad window")

var twoHours = regexp.MustCompile(`(?:^|[^-\d])(\d{1,2})\D+(\d{1,2})`)

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

const (
	StepReminderHub          = "reminder_hub"
	StepReminderPickWindow   = "reminder_pick_window"
	StepReminderCustomWindow = "reminder_custom_window"
	StepReminderWeekly       = "reminder_weekly"

	ReminderActionKey        = "reminder_action"
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

	KeyHubHasRow  = "hub_has_row"
	KeySkipHub    = "skip_hub"
	KeyAskWeekly  = "ask_weekly"
	KeyHubDailyOn = "hub_daily_on"
)

var ReminderPresets = []conversation.ChoiceOption{
	{Label: "🌅 Mañana (8 a 10)", Value: "480-600", NextStep: StepReminderWeekly},
	{Label: "☀️ Mediodía (12 a 14)", Value: "720-840", NextStep: StepReminderWeekly},
	{Label: "🌆 Tarde (16 a 18)", Value: "960-1080", NextStep: StepReminderWeekly},
	{Label: "🌙 Noche (20 a 22)", Value: "1200-1320", NextStep: StepReminderWeekly},
}

func OnReminderPickWindow(value string, data conversation.Data) conversation.Data {
	next := conversation.CopyData(data)
	switch value {
	case OptionCancel:
		conversation.SetFlag(next, conversation.KeyCancelled)
	case OptionReminderOff:
		next[ReminderActionKey] = ReminderActionOff
	case OptionReminderOther:
	default:
		start, end := SplitPreset(value)
		next[ReminderActionKey] = ReminderActionSet
		next[ReminderStartKey] = strconv.Itoa(start)
		next[ReminderEndKey] = strconv.Itoa(end)
	}
	return next
}

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

func OnReminderHub(value string, data conversation.Data) conversation.Data {
	next := conversation.CopyData(data)
	switch value {
	case OptionHubDaily:
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

func SkipHubIfSeeded(data conversation.Data) (string, bool) {
	if conversation.Flag(data, KeySkipHub) {
		return StepReminderPickWindow, true
	}
	return "", false
}

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

func OnReminderWeekly(value string, data conversation.Data) conversation.Data {
	next := conversation.CopyData(data)
	if value == OptionWeeklyOn {
		conversation.SetFlag(next, conversation.KeyWeeklySummary)
	}
	return next
}

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

func SkipWeeklyUnlessAsked(data conversation.Data) (string, bool) {
	if conversation.Flag(data, KeyAskWeekly) {
		return "", false
	}
	return "", true
}

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
