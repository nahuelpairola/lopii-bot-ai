package user

import "testing"

// El core no puede nombrar un canal: la constante es el único lugar donde
// la palabra "telegram" existe del lado del modelo.
func TestChannelConstants(t *testing.T) {
	if ChannelTelegram != "telegram" {
		t.Errorf("ChannelTelegram = %q, want %q", ChannelTelegram, "telegram")
	}
}

func TestUserChannelTableName(t *testing.T) {
	if (UserChannel{}).TableName() != "user_channels" {
		t.Errorf("TableName = %q, want user_channels", (UserChannel{}).TableName())
	}
}
