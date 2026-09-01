package user

import "testing"

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
