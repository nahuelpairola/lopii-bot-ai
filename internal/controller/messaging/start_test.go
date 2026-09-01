package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"gorm.io/gorm"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/user"
)

type fakeUserRepository struct {
	byChannelUserID map[string]*user.User
	linked          map[string]uint64
	inserted        []*user.User
	insertErr       error
}

func (r *fakeUserRepository) FindByChannel(channel, channelUserID string) (*user.User, error) {
	if u, ok := r.byChannelUserID[channelUserID]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeUserRepository) FindByID(id uint64) (*user.User, error) {
	for _, u := range r.inserted {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeUserRepository) Insert(u *user.User) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	u.ID = uint64(len(r.inserted) + 1)
	r.inserted = append(r.inserted, u)
	return nil
}

func (r *fakeUserRepository) LinkChannel(userID uint64, channel, channelUserID string) error {
	if r.linked == nil {
		r.linked = make(map[string]uint64)
	}
	r.linked[channelUserID] = userID
	return nil
}

func (r *fakeUserRepository) InsertWithChannel(u *user.User, channel, channelUserID string) error {
	if err := r.Insert(u); err != nil {
		return err
	}
	return r.LinkChannel(u.ID, channel, channelUserID)
}

func (r *fakeUserRepository) FindChannelID(userID uint64, channel string) (string, error) {
	for channelUserID, uid := range r.linked {
		if uid == userID {
			return channelUserID, nil
		}
	}
	return "", gorm.ErrRecordNotFound
}

type fakeInvitationRepository struct {
	byCode       map[string]*invitation.Invitation
	markedAsUsed map[uint64]uint64
	markErr      error
}

func (r *fakeInvitationRepository) FindByCode(code string) (*invitation.Invitation, error) {
	if inv, ok := r.byCode[code]; ok {
		return inv, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeInvitationRepository) MarkAsUsed(id uint64, userID uint64) error {
	if r.markErr != nil {
		return r.markErr
	}
	if r.markedAsUsed == nil {
		r.markedAsUsed = make(map[uint64]uint64)
	}
	r.markedAsUsed[id] = userID
	return nil
}

func TestHandleStart_ValueFirst_NoOnboardingFlow(t *testing.T) {
	const telegramID = "123456789"
	const validCode = "ABC123"

	userRepo := &fakeUserRepository{byChannelUserID: make(map[string]*user.User)}
	invRepo := &fakeInvitationRepository{
		byCode: map[string]*invitation.Invitation{
			validCode: {
				ID:        1,
				Code:      validCode,
				CreatedBy: 999,
				UsedAt:    nil,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
		},
	}
	accountRepo := &fakeAccountRepoFull{}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })

	c := &controller{
		users:       userRepo,
		invitations: invRepo,
		accounts:    accountRepo,
		engine:      engine,
	}

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{
				ID:       123456789,
				Username: "testuser",
			},
			Chat: models.Chat{ID: 111111},
			Text: "/start " + validCode,
		},
	}

	c.HandleStart(context.Background(), nil, update)

	if len(userRepo.inserted) != 1 {
		t.Fatalf("expected 1 user inserted, got %d", len(userRepo.inserted))
	}
	if got, err := userRepo.FindChannelID(userRepo.inserted[0].ID, user.ChannelTelegram); err != nil || got != telegramID {
		t.Errorf("linked channel_user_id = %q (err=%v), want %q", got, err, telegramID)
	}

	if len(accountRepo.inserted) != 0 {
		t.Errorf("accountRepository.Insert should not be called, but was called %d times", len(accountRepo.inserted))
	}

	if store.found {
		t.Errorf("expected no flow to be started, but got found=%v flowName=%q", store.found, store.flowName)
	}

	if userID, ok := invRepo.markedAsUsed[1]; !ok {
		t.Error("invitation was not marked as used")
	} else if userID != userRepo.inserted[0].ID {
		t.Errorf("invitation marked as used by user %d, want %d", userID, userRepo.inserted[0].ID)
	}
}

func TestHandleStart_ExistingUser_DoesNotRestart(t *testing.T) {
	const telegramID = "123456789"
	const validCode = "ABC123"

	existingUser := &user.User{ID: 42, Username: "testuser"}

	userRepo := &fakeUserRepository{
		byChannelUserID: map[string]*user.User{telegramID: existingUser},
	}
	invRepo := &fakeInvitationRepository{
		byCode: map[string]*invitation.Invitation{
			validCode: {
				ID:        1,
				Code:      validCode,
				CreatedBy: 999,
				UsedAt:    nil,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
		},
	}
	accountRepo := &fakeAccountRepoFull{}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })

	c := &controller{
		users:       userRepo,
		invitations: invRepo,
		accounts:    accountRepo,
		engine:      engine,
	}

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{
				ID:       123456789,
				Username: "testuser",
			},
			Chat: models.Chat{ID: 111111},
			Text: "/start " + validCode,
		},
	}

	c.HandleStart(context.Background(), nil, update)

	if len(userRepo.inserted) != 0 {
		t.Errorf("no new user should be inserted for an existing user, but %d were", len(userRepo.inserted))
	}

	if store.found {
		t.Errorf("no flow should be started for an existing user, but %q was started", store.flowName)
	}

	if len(invRepo.markedAsUsed) != 0 {
		t.Error("invitation should not be marked as used for an existing user")
	}
}
