//go:build integration

package user

import (
	"fmt"
	"testing"
	"time"

	"lopiibot.com/internal/database"
)

func TestInsertWithChannel_FailedChannelInsertLeavesNoOrphanedUser(t *testing.T) {
	conn := testConnection(t)
	repo := NewRepository(conn)

	channelUserID := fmt.Sprintf("dup-%d", time.Now().UnixNano())

	userA := &User{Username: "userA-" + channelUserID}
	if err := repo.Insert(userA); err != nil {
		t.Fatalf("insert userA: %v", err)
	}
	if err := repo.LinkChannel(userA.ID, ChannelTelegram, channelUserID); err != nil {
		t.Fatalf("link userA channel: %v", err)
	}
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", userA.ID).Delete(&UserChannel{})
		conn.DB.Unscoped().Where("id = ?", userA.ID).Delete(&User{})
	})

	userB := &User{Username: "userB-" + channelUserID}
	err := repo.InsertWithChannel(userB, ChannelTelegram, channelUserID)
	if err == nil {
		t.Fatalf("InsertWithChannel: want error on duplicate (channel, channel_user_id), got nil")
	}

	var count int64
	if err := conn.DB.Unscoped().Model(&User{}).Where("username = ?", userB.Username).Count(&count).Error; err != nil {
		t.Fatalf("count userB rows: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0 users rows for the failed insert, got %d: la transacción no deshizo el Create de users", count)
	}
}

func testConnection(t *testing.T) *database.Connection {
	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	return conn
}
