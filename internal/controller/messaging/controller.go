package messaging

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type userRepository interface {
	FindByChannel(channel, channelUserID string) (*user.User, error)
	FindByID(id uint64) (*user.User, error)
	Insert(u *user.User) error
	InsertWithChannel(u *user.User, channel, channelUserID string) error
	FindChannelID(userID uint64, channel string) (string, error)
}

type invitationRepository interface {
	FindByCode(code string) (*invitation.Invitation, error)
	MarkAsUsed(id uint64, userID uint64) error
}

type accountRepository interface {
	Insert(*account.Account) error
	FindDefaultByCurrency(userID uint64, currency currency.Currency) (*account.Account, error)
	HasDefaultForCurrency(userID uint64, currency currency.Currency) bool
	FindByUserID(userID uint64) ([]account.Account, error)
	GetAccount(id uint64) (*account.Account, error)
	Rename(accountID uint64, name string) error
	UnsetDefault(userID uint64, currency currency.Currency) error
	SetDefault(accountID uint64) error
}

type movementRepository interface {
	InsertBatch([]movement.Movement) error
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
	ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error
	FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error)
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
	SoftDeleteByIDs(ids []uint) error
	InsertAccountsWithOpenings(items []movement.AccountOpening) error
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	ListForUser(q movement.MovementQuery, limit, offset int) ([]movement.Movement, error)
	ReassignAccount(fromID, toID uint64) error
	CountForUser(userID uint64) (int64, error)
	CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error)
	ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error
	TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error)
	CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error)
}

type subcategoryRepository interface {
	FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	IconForCategory(userID uint64, category string) string
	Insert(s *subcategory.Subcategory) error
	Reload() error
	Delete(userID uint64, id uint64) error
	FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error)
}

type movementOrchestrator interface {
	ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error)
	ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error)
	ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error)
	ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error)
	AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
	ClassifyCategories(ctx context.Context, message string, rows []orchestrator.ClassifyRow, taxonomy []orchestrator.TaxonomyEntry) []orchestrator.Pair
	Run(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
}

type metricRepository interface {
	Log(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error
	Resolve(userID uint64, outcome string, movementIDs []uint) error
	SetIntentIfQueued(userID uint64, intent string) error
}

type chatHistoryRepository interface {
	Recent(userID uint64) ([]chathistory.Turn, error)
	Append(userID uint64, question, answer string) error
}

type reminderRepository interface {
	Upsert(r *reminder.Reminder) error
	Disable(userID uint64) error
	FindByUserID(userID uint64) (*reminder.Reminder, error)
	SetWeeklySummary(userID uint64, enabled bool) error
}

type traceRepository interface {
	InsertRequestTrace(traceID string, userID *uint64, updateType string, receivedAt time.Time, latencyMs int, errMsg string) error
}

type nudgeRepository interface {
	SentKeys(userID uint64) ([]string, error)
	MarkSent(userID uint64, key string) error
	MarkSentAgain(userID uint64, key string) error
	MarkTapped(userID uint64, key string) error
	LastSentAt(userID uint64) (*time.Time, error)
}

type actionsRepository interface {
	Insert(action *pendingaction.PendingAction) error
	NextForUser(userID uint64) (*pendingaction.PendingAction, error)
	Update(action *pendingaction.PendingAction) error
	Delete(id uint64) error
	CountForUser(userID uint64) (int64, error)
}

type controller struct {
	users         userRepository
	invitations   invitationRepository
	accounts      accountRepository
	movements     movementRepository
	subcategories subcategoryRepository
	engine        *conversation.Engine
	orchestrator  movementOrchestrator
	metrics       metricRepository
	chatHistory   chatHistoryRepository
	reminders     reminderRepository
	traces        traceRepository
	nudges        nudgeRepository
	jobs          pendingjob.Repository
	actions       actionsRepository
	locks         userLocks
}

func NewController(
	users userRepository,
	invitations invitationRepository,
	accounts accountRepository,
	movements movementRepository,
	subcategories subcategoryRepository,
	engine *conversation.Engine,
	orch movementOrchestrator,
	metrics metricRepository,
	chatHistory chatHistoryRepository,
	reminders reminderRepository,
	traces traceRepository,
	nudges nudgeRepository,
	jobs pendingjob.Repository,
	actions actionsRepository,
) *controller {
	return &controller{
		users:         users,
		invitations:   invitations,
		accounts:      accounts,
		movements:     movements,
		subcategories: subcategories,
		engine:        engine,
		orchestrator:  orch,
		metrics:       metrics,
		chatHistory:   chatHistory,
		reminders:     reminders,
		traces:        traces,
		nudges:        nudges,
		jobs:          jobs,
		actions:       actions,
	}
}

func (c *controller) Handle(ctx context.Context, in messenger.Incoming) {
	c.traced(ctx, kindOf(in), rawOf(in), func(ctx context.Context) (*uint64, error) {
		u, err := c.users.FindByChannel(in.Channel, in.ChannelUserID)
		if err != nil {
			return nil, err
		}
		uid := u.ID

		defer c.locks.lock(uid)()

		return c.dispatch(ctx, in, u)
	})
}

func (c *controller) dispatch(ctx context.Context, in messenger.Incoming, u *user.User) (*uint64, error) {
	uid := u.ID
	chat := in.Chat
	input := in.Input
	if input.CallbackData != "" {
		chat = messenger.ReplaceTapped(chat)
	}

	if c.tips().HandleCallback(ctx, chat, u.ID, input.CallbackData) {
		return &uid, nil
	}
	if c.finisher().HandleNearDuplicateChoice(ctx, chat, u.ID, input.CallbackData) {
		return &uid, nil
	}

	result, found, err := c.engine.Handle(u.ID, input)
	if err != nil {
		c.sendText(ctx, chat, msgSomethingBroke)
		return &uid, err
	}
	if !found {
		if input.Text != "" {
			if pendingjob.EnqueueBehindPending(ctx, c, c.jobs, chat, u.ID, input.Text) {
				return &uid, nil
			}
			err := c.handleFreeText(ctx, chat, u.ID, input.Text)
			c.tips().Maybe(ctx, chat, u.ID)
			return &uid, err
		}
		return &uid, nil
	}
	if result.Finished {
		c.handleFlowFinished(ctx, chat, result)
		c.tips().Maybe(ctx, chat, u.ID)
		return &uid, nil
	}
	c.sendPrompt(ctx, chat, result.Prompt)
	return &uid, nil
}

func (c *controller) handleFlowFinished(ctx context.Context, chat messenger.Chat, result conversation.Result) {
	slog.InfoContext(ctx, "flow finished", "flow", result.FlowName)
	if conversation.StringOrEmpty(result.Data[conversation.ResumeCancelledKey]) == "true" {
		c.sendText(ctx, chat, msgResumeCancelled)
		return
	}
	if result.FlowName == flow.AskUserFlowName {
		c.loop().FinishAskUser(ctx, chat, result.Data)
		return
	}
	defer func() {
		if err := c.loop().DrainNextAction(ctx, chat, result.Data.UserID()); err != nil {
			slog.ErrorContext(ctx, "drain parked actions failed", "err", err)
		}
	}()

	switch result.FlowName {
	case flow.MovementCreateFlowName:
		c.finisher().FinishMovementCreate(ctx, chat, result.Data)
	case flow.MovementUpdatePickFlowName:
		c.loop().FinishMovementUpdatePick(ctx, chat, result.Data)
	case flow.MovementUpdateConfirmFlowName:
		c.finishMovementUpdateConfirmFlow(ctx, chat, result.Data)
	case flow.MovementDeleteFlowName:
		c.finisher().FinishMovementDelete(ctx, chat, result.Data)
	case flow.AccountCreateFlowName:
		c.finishAccountCreateFlow(ctx, chat, result.Data)
	case flow.AccountManageFlowName:
		c.finishAccountManageFlow(ctx, chat, result.Data)
	case flow.AccountMoveOfferFlowName:
		c.finishAccountMoveOffer(ctx, chat, result.Data)
	case flow.SubcategorySetupFlowName:
		c.finishSubcategorySetupFlow(ctx, chat, result.Data)
	case flow.CategoryMatchOfferFlowName:
		c.finishCategoryMatchOffer(ctx, chat, result.Data)
	case flow.CategoryProposalConfirmFlowName:
		c.finishCategoryProposalConfirm(ctx, chat, result.Data)
	case flow.CategoryManagePickFlowName:
		c.finishCategoryManagePickFlow(ctx, chat, result.Data)
	case flow.CategoryManageTargetFlowName:
		c.finishCategoryManageTargetFlow(ctx, chat, result.Data)
	case flow.MovementNegativeConfirmFlowName:
		c.finisher().FinishMovementNegativeConfirm(ctx, chat, result.Data)
	case flow.ReminderSetupFlowName:
		c.finishReminderSetup(ctx, chat, result.Data)
	default:
		c.sendText(ctx, chat, msgSomethingBroke)
	}
}

func (c *controller) finishMovementUpdateConfirmFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishMovementUpdateConfirm(ctx, chat, data)
}

func (c *controller) sendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt) {
	flow.SendPrompt(ctx, chat, prompt)
}

func (c *controller) startFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error {
	return flow.StartFlow(ctx, c.engine, chat, userID, flowName, seed, errCtx)
}

func (c *controller) SendText(ctx context.Context, chat messenger.Chat, text string) {
	flow.SendText(ctx, chat, text)
}
