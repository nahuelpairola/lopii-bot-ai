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

// fakeUserRepository mocks the userRepository interface for start_test.
type fakeUserRepository struct {
	byTelegramID map[string]*user.User
	inserted     []*user.User
	insertErr    error
}

func (r *fakeUserRepository) FindByTelegramID(telegramID string) (*user.User, error) {
	if u, ok := r.byTelegramID[telegramID]; ok {
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
	u.ID = uint64(len(r.inserted) + 1) // assign a fake ID
	r.inserted = append(r.inserted, u)
	return nil
}

// fakeInvitationRepository mocks the invitationRepository interface for start_test.
type fakeInvitationRepository struct {
	byCode       map[string]*invitation.Invitation
	markedAsUsed map[uint64]uint64 // invitation ID -> user ID who used it
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

// TestHandleStart_ValueFirst_NoOnboardingFlow verifies the value-first /start:
// a new user gets created and greeted with msgWelcome, with NO flow started
// (no bulk-collect barrage) and no account fabricated.
func TestHandleStart_ValueFirst_NoOnboardingFlow(t *testing.T) {
	const telegramID = "123456789"
	const validCode = "ABC123"

	userRepo := &fakeUserRepository{byTelegramID: make(map[string]*user.User)}
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

	// Construct a fake Telegram /start message with the invitation code
	update := &models.Update{
		Message: &models.Message{
			From: &models.User{
				ID:       123456789, // matches the telegramID string numerically
				Username: "testuser",
			},
			Chat: models.Chat{ID: 111111},
			Text: "/start " + validCode,
		},
	}

	// Call handleStart with nil bot (sendPrompt and reply guard against nil)
	c.handleStart(context.Background(), nil, update)

	// Verify: userRepository.Insert was called exactly once
	if len(userRepo.inserted) != 1 {
		t.Fatalf("expected 1 user inserted, got %d", len(userRepo.inserted))
	}
	if userRepo.inserted[0].TelegramID != telegramID {
		t.Errorf("inserted user telegram_id = %q, want %q", userRepo.inserted[0].TelegramID, telegramID)
	}

	// Verify: accountRepository.Insert was NOT called — value-first, no fabricated account.
	if len(accountRepo.inserted) != 0 {
		t.Errorf("accountRepository.Insert should not be called, but was called %d times", len(accountRepo.inserted))
	}

	// Verify: NO flow was started — value-first /start is a single greeting.
	if store.found {
		t.Errorf("expected no flow to be started, but got found=%v flowName=%q", store.found, store.flowName)
	}

	// Verify: invitation was marked as used
	if userID, ok := invRepo.markedAsUsed[1]; !ok {
		t.Error("invitation was not marked as used")
	} else if userID != userRepo.inserted[0].ID {
		t.Errorf("invitation marked as used by user %d, want %d", userID, userRepo.inserted[0].ID)
	}
}

// TestHandleStart_ExistingUser_DoesNotRestart verifies that if a user already
// exists, /start does not create a duplicate and does not start any flow.
func TestHandleStart_ExistingUser_DoesNotRestart(t *testing.T) {
	const telegramID = "123456789"
	const validCode = "ABC123"

	existingUser := &user.User{ID: 42, TelegramID: telegramID, Username: "testuser"}

	userRepo := &fakeUserRepository{
		byTelegramID: map[string]*user.User{telegramID: existingUser},
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
				ID:       123456789, // matches the telegramID string numerically
				Username: "testuser",
			},
			Chat: models.Chat{ID: 111111},
			Text: "/start " + validCode,
		},
	}

	c.handleStart(context.Background(), nil, update)

	// Verify: no new user was inserted
	if len(userRepo.inserted) != 0 {
		t.Errorf("no new user should be inserted for an existing user, but %d were", len(userRepo.inserted))
	}

	// Verify: no flow was started
	if store.found {
		t.Errorf("no flow should be started for an existing user, but %q was started", store.flowName)
	}

	// Verify: invitation was not marked as used
	if len(invRepo.markedAsUsed) != 0 {
		t.Error("invitation should not be marked as used for an existing user")
	}
}
