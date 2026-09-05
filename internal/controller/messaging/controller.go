package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/nudges"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/settings"
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

func (c *controller) FindUserAccounts(userID uint64) ([]account.Account, error) {
	return c.accounts.FindByUserID(userID)
}

func (c *controller) InsertAccount(a *account.Account) error {
	return c.accounts.Insert(a)
}

func (c *controller) GetAccount(id uint64) (*account.Account, error) {
	return c.accounts.GetAccount(id)
}

func (c *controller) SumAmountForAccount(id uint64) (decimal.Decimal, error) {
	return c.movements.SumAmountForAccount(id)
}

func (c *controller) InsertMovements(movs []movement.Movement) error {
	return c.movements.InsertBatch(movs)
}

func (c *controller) ReplaceMovements(oldIDs []uint, movs []movement.Movement) error {
	return c.movements.ReplaceMovements(oldIDs, movs)
}

func (c *controller) FindSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error) {
	return c.subcategories.FindByCategoryAndSubcategory(userID, category, subcategory)
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

	if nudges.HandleCallback(ctx, c, chat, u.ID, input.CallbackData) {
		return &uid, nil
	}
	if flow.HandleNearDuplicateChoice(ctx, c, chat, u.ID, input.CallbackData) {
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
			nudges.Maybe(ctx, c, chat, u.ID)
			return &uid, err
		}
		return &uid, nil
	}
	if result.Finished {
		c.handleFlowFinished(ctx, chat, result)
		nudges.Maybe(ctx, c, chat, u.ID)
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
		agent.FinishAskUser(ctx, c, chat, result.Data)
		return
	}
	defer func() {
		if err := agent.DrainNextAction(ctx, c, chat, result.Data.UserID()); err != nil {
			slog.ErrorContext(ctx, "drain parked actions failed", "err", err)
		}
	}()

	switch result.FlowName {
	case flow.MovementCreateFlowName:
		flow.FinishMovementCreate(ctx, c, chat, result.Data)
	case flow.MovementUpdatePickFlowName:
		agent.FinishMovementUpdatePick(ctx, c, chat, result.Data)
	case flow.MovementUpdateConfirmFlowName:
		c.finishMovementUpdateConfirmFlow(ctx, chat, result.Data)
	case flow.MovementDeleteFlowName:
		flow.FinishMovementDelete(ctx, c, chat, result.Data)
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
		flow.FinishMovementNegativeConfirm(ctx, c, chat, result.Data)
	case flow.ReminderSetupFlowName:
		c.finishReminderSetup(ctx, chat, result.Data)
	default:
		c.sendText(ctx, chat, msgSomethingBroke)
	}
}

func (c *controller) finishMovementUpdateConfirmFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishMovementUpdateConfirm(ctx, c, chat, data)
}

func (c *controller) sendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt) {
	if err := chat.Send(ctx, prompt); err != nil {
		slog.ErrorContext(ctx, "controller: send prompt failed", "err", err)
	}
}

func (c *controller) startFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error {
	prompt, err := c.engine.StartWithData(userID, flowName, seed)
	if err != nil {
		c.sendText(ctx, chat, msgSomethingBroke)
		return fmt.Errorf("%s: %w", errCtx, err)
	}
	c.sendPrompt(ctx, chat, prompt)
	return nil
}

func (c *controller) ResolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint) {
	c.resolveMetric(ctx, userID, outcome, movementIDs...)
}

func (c *controller) SendText(ctx context.Context, chat messenger.Chat, text string) {
	if err := messenger.SendText(ctx, chat, text); err != nil {
		slog.ErrorContext(ctx, "controller: send text failed", "err", err)
	}
}

func (c *controller) StartFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error {
	return c.startFlow(ctx, chat, userID, flowName, seed, errCtx)
}

func (c *controller) MarkTipSent(userID uint64, tip string) error {
	if c.nudges == nil {
		return nil
	}
	return c.nudges.MarkSent(userID, tip)
}

func (c *controller) SoftDeleteByIDs(ids []uint) error {
	return c.movements.SoftDeleteByIDs(ids)
}

func (c *controller) RenameAccount(id uint64, name string) error {
	return c.accounts.Rename(id, name)
}

func (c *controller) FindDefaultAccountByCurrency(userID uint64, cur currency.Currency) (*account.Account, error) {
	return c.accounts.FindDefaultByCurrency(userID, cur)
}

func (c *controller) UnsetDefaultAccount(userID uint64, cur currency.Currency) error {
	return c.accounts.UnsetDefault(userID, cur)
}

func (c *controller) SetDefaultAccount(id uint64) error {
	return c.accounts.SetDefault(id)
}

func (c *controller) InsertMovementsBatch(movs []movement.Movement) error {
	return c.movements.InsertBatch(movs)
}

func (c *controller) ReassignAccountMovements(fromID, toID uint64) error {
	return c.movements.ReassignAccount(fromID, toID)
}

func (c *controller) StartAccountCreate(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	return settings.StartAccountCreate(ctx, c, chat, userID, text)
}

func (c *controller) SubcategoryIconForCategory(userID uint64, category string) string {
	return c.subcategories.IconForCategory(userID, category)
}

func (c *controller) InsertSubcategory(s *subcategory.Subcategory) error {
	return c.subcategories.Insert(s)
}

func (c *controller) ReloadSubcategories() error {
	return c.subcategories.Reload()
}

func (c *controller) DeleteSubcategory(userID, id uint64) error {
	return c.subcategories.Delete(userID, id)
}

func (c *controller) CountMovementsBySubcategory(userID, subcategoryID uint64) (int64, error) {
	return c.movements.CountBySubcategory(userID, subcategoryID)
}

func (c *controller) ReassignSubcategoryMovements(userID, fromID, toID uint64) error {
	return c.movements.ReassignSubcategory(userID, fromID, toID)
}

func (c *controller) SuggestMergeTarget(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory {
	return c.suggestMergeTarget(ctx, userID, sourceID, data)
}

func (c *controller) UpsertReminder(rem *reminder.Reminder) error {
	return c.reminders.Upsert(rem)
}

func (c *controller) DisableReminder(userID uint64) error {
	return c.reminders.Disable(userID)
}

func (c *controller) SetWeeklySummary(userID uint64, enabled bool) error {
	return c.reminders.SetWeeklySummary(userID, enabled)
}

func (c *controller) FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error) {
	return c.movements.FindRecentlyCreatedForUser(userID, since, limit)
}
