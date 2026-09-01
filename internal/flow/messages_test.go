package flow

import (
	"strings"
	"testing"
)

func TestTurningNotificationsOffSaysTheMonthlyKeepsComing(t *testing.T) {
	cases := map[string]string{
		"MsgWeeklySummaryOff": MsgWeeklySummaryOff,
		"MsgReminderAllOff":   MsgReminderAllOff,
	}
	for name, msg := range cases {
		if !strings.Contains(msg, msgMonthlyAlwaysOn) {
			t.Errorf("%s no dice que el resumen del mes sigue: %q", name, msg)
		}
	}
}

func TestNoMessagePromisesEveryNotificationIsOff(t *testing.T) {
	for name, msg := range map[string]string{
		"MsgReminderAllOff":   MsgReminderAllOff,
		"MsgWeeklySummaryOff": MsgWeeklySummaryOff,
		"MsgReminderDisabled": MsgReminderDisabled,
	} {
		if strings.Contains(strings.ToLower(msg), "todas las notificaciones") {
			t.Errorf("%s promete apagar todo, y el resumen del mes no se apaga: %q", name, msg)
		}
	}
}
