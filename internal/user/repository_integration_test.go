//go:build integration

package user

import (
	"fmt"
	"testing"
	"time"

	"lopiibot.com/internal/database"
)

// Run with: go test -tags integration ./internal/user/
// Requires local Postgres (docker compose up -d) with migrations applied.

// TestInsertWithChannel_FailedChannelInsertLeavesNoOrphanedUser prueba la
// atomicidad exigida en fix round 1: si la segunda escritura (user_channels)
// falla, la primera (users) tiene que deshacerse. Antes de InsertWithChannel,
// Insert+LinkChannel eran dos escrituras independientes — una fila en users
// sin su fila en user_channels era un usuario invisible para FindByChannel, y
// el reintento de /start insertaba otra huérfana en vez de encontrarlo.
//
// Para forzar la falla SIN mockear nada, se aprovecha la restricción real:
// UNIQUE (channel, channel_user_id) en user_channels. Un usuario A ya tiene
// esa dirección linkeada; InsertWithChannel para un usuario B con la MISMA
// dirección tiene que fallar en el segundo Create, y entonces el primero
// (la fila de B en users) tiene que quedar deshecho por la transacción.
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

	// La fila de B en users no puede haber sobrevivido a la falla del segundo
	// Create: si InsertWithChannel no fuera atómico, esto encontraría un
	// usuario huérfano, sin dirección, invisible para FindByChannel.
	var count int64
	if err := conn.DB.Unscoped().Model(&User{}).Where("username = ?", userB.Username).Count(&count).Error; err != nil {
		t.Fatalf("count userB rows: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0 users rows for the failed insert, got %d: la transacción no deshizo el Create de users", count)
	}
}

// testConnection connects to the local Docker Postgres instance used by integration tests.
func testConnection(t *testing.T) *database.Connection {
	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	return conn
}
