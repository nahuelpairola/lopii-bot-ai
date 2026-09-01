package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

type fakeServices struct {
	engine        *conversation.Engine
	actions       *fakeActionsRepo
	movements     fakeMovements
	accounts      *fakeAccountRepoFull
	subcategories *fakeSubcategoryRepoFull
	chatHistory   *stubChatHistory
	orch          *fakeOrchestrator
	metrics       *fakeMetricRepo

	replaying   bool
	sendTexts   []string
	enqueued    int
	handledGroq bool
}

type fakeMovements interface {
	InsertBatch(ms []movement.Movement) error
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
	ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error
	FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error)
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
	SoftDeleteByIDs(ids []uint) error
	InsertAccountsWithOpenings(items []movement.AccountOpening) error
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error)
	ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error)
	ReassignAccount(fromID, toID uint64) error
	CountForUser(userID uint64) (int64, error)
	CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error)
	ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error
	TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error)
}

func (f *fakeServices) FindUserAccounts(userID uint64) ([]account.Account, error) {
	return f.accounts.FindByUserID(userID)
}

func (f *fakeServices) AccountsHasDefaultForCurrency(userID uint64, cur currency.Currency) bool {
	return f.accounts.HasDefaultForCurrency(userID, cur)
}

func (f *fakeServices) FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error) {
	return f.movements.FindRecentlyCreatedForUser(userID, since, limit)
}

func (f *fakeServices) MovementsFindSimilarForUser(userID uint64, message string, since time.Time, until *time.Time) ([]movement.Movement, error) {
	return f.movements.FindSimilarForUser(userID, message, since, until)
}

func (f *fakeServices) SubcategoriesFindAllForUser(userID uint64) ([]subcategory.Subcategory, error) {
	return f.subcategories.FindAllForUser(userID)
}

func (f *fakeServices) FindSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error) {
	return f.subcategories.FindByCategoryAndSubcategory(userID, category, subcategory)
}

func (f *fakeServices) ChatHistoryRecent(userID uint64) ([]chathistory.Turn, error) {
	return f.chatHistory.Recent(userID)
}

func (f *fakeServices) ChatHistoryAppend(userID uint64, question, answer string) error {
	return f.chatHistory.Append(userID, question, answer)
}

func (f *fakeServices) ActionsInsert(a *pendingaction.PendingAction) error {
	return f.actions.Insert(a)
}

func (f *fakeServices) ActionsNextForUser(userID uint64) (*pendingaction.PendingAction, error) {
	return f.actions.NextForUser(userID)
}

func (f *fakeServices) ActionsUpdate(a *pendingaction.PendingAction) error {
	return f.actions.Update(a)
}

func (f *fakeServices) ActionsDelete(id uint64) error {
	return f.actions.Delete(id)
}

func (f *fakeServices) ActionsEnabled() bool {
	return true
}

func (f *fakeServices) MetricsLog(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error {
	return f.metrics.Log(userID, traceID, rawMessage, intent, needsConfirmation, outcome)
}

func (f *fakeServices) ResolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint) {
	f.metrics.Resolve(userID, outcome, movementIDs)
}

func (f *fakeServices) MetricsSetIntentIfQueued(userID uint64, intent string) error {
	return f.metrics.SetIntentIfQueued(userID, intent)
}

func (f *fakeServices) Run(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return f.orch.Run(ctx, systemPrompt, userText, history, tools, execute)
}

func (f *fakeServices) ClassifyCategories(ctx context.Context, message string, rows []orchestrator.ClassifyRow, taxonomy []orchestrator.TaxonomyEntry) []orchestrator.Pair {
	return f.orch.ClassifyCategories(ctx, message, rows, taxonomy)
}

func (f *fakeServices) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error) {
	return f.orch.ResolveUpdate(ctx, text, candidate, accounts)
}

func (f *fakeServices) SendText(ctx context.Context, chat messenger.Chat, text string) {
	f.sendTexts = append(f.sendTexts, text)
}

func (f *fakeServices) SendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt) {
}

func (f *fakeServices) StartFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error {
	_, _ = f.engine.StartWithData(userID, flowName, seed)
	return nil
}

func (f *fakeServices) EngineStartWithData(userID uint64, flowName string, seed conversation.Data) (conversation.Prompt, error) {
	return f.engine.StartWithData(userID, flowName, seed)
}

func (f *fakeServices) IsReplaying(ctx context.Context) bool {
	return f.replaying
}

func (f *fakeServices) MaybeNearDuplicate(userID uint64, inserted []movement.Movement) []conversation.Button {
	return nil
}

func (f *fakeServices) ResolveAndInsertMovements(data conversation.Data) ([]movement.Movement, error) {
	return flow.ResolveAndInsertMovements(f, data)
}

func (f *fakeServices) HandleGroqError(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error) {
	f.handledGroq = true
	f.enqueued++
	return true, nil
}

func (f *fakeServices) EnqueueUpdatePickIfRateLimited(ctx context.Context, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool {
	return false
}

func (f *fakeServices) FinishAnswerQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	return nil
}

func (f *fakeServices) FinishManageSettings(ctx context.Context, chat messenger.Chat, userID uint64, text, area string) error {
	return nil
}

func (f *fakeServices) InsertAccount(a *account.Account) error {
	return f.accounts.Insert(a)
}

func (f *fakeServices) GetAccount(id uint64) (*account.Account, error) {
	return f.accounts.GetAccount(id)
}

func (f *fakeServices) SumAmountForAccount(id uint64) (decimal.Decimal, error) {
	return f.movements.SumAmountForAccount(id)
}

func (f *fakeServices) InsertMovements(movs []movement.Movement) error {
	return f.movements.InsertBatch(movs)
}

func (f *fakeServices) InsertMovementsBatch(movs []movement.Movement) error {
	return f.movements.InsertBatch(movs)
}

func (f *fakeServices) ReplaceMovements(oldIDs []uint, movs []movement.Movement) error {
	return f.movements.ReplaceMovements(oldIDs, movs)
}

func (f *fakeServices) SoftDeleteByIDs(ids []uint) error {
	return f.movements.SoftDeleteByIDs(ids)
}

func (f *fakeServices) RenameAccount(id uint64, name string) error {
	return f.accounts.Rename(id, name)
}

func (f *fakeServices) FindDefaultAccountByCurrency(userID uint64, cur currency.Currency) (*account.Account, error) {
	return f.accounts.FindDefaultByCurrency(userID, cur)
}

func (f *fakeServices) UnsetDefaultAccount(userID uint64, cur currency.Currency) error {
	return f.accounts.UnsetDefault(userID, cur)
}

func (f *fakeServices) SetDefaultAccount(id uint64) error {
	return f.accounts.SetDefault(id)
}

func (f *fakeServices) ReassignAccountMovements(fromID, toID uint64) error {
	return f.movements.ReassignAccount(fromID, toID)
}

func (f *fakeServices) SubcategoryIconForCategory(userID uint64, category string) string {
	return f.subcategories.IconForCategory(userID, category)
}

func (f *fakeServices) InsertSubcategory(s *subcategory.Subcategory) error {
	return f.subcategories.Insert(s)
}

func (f *fakeServices) ReloadSubcategories() error {
	return f.subcategories.Reload()
}

func (f *fakeServices) DeleteSubcategory(userID, id uint64) error {
	return f.subcategories.Delete(userID, id)
}

func (f *fakeServices) CountMovementsBySubcategory(userID, subcategoryID uint64) (int64, error) {
	return f.movements.CountBySubcategory(userID, subcategoryID)
}

func (f *fakeServices) ReassignSubcategoryMovements(userID, fromID, toID uint64) error {
	return f.movements.ReassignSubcategory(userID, fromID, toID)
}

func (f *fakeServices) UpsertReminder(rem *reminder.Reminder) error { return nil }
func (f *fakeServices) DisableReminder(userID uint64) error         { return nil }
func (f *fakeServices) SetWeeklySummary(userID uint64, enabled bool) error {
	return nil
}
func (f *fakeServices) MarkTipSent(userID uint64, tip string) error { return nil }
func (f *fakeServices) StartAccountCreate(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	return nil
}
func (f *fakeServices) SuggestMergeTarget(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory {
	return nil
}

type fakeOrchestrator struct {
	updateResult      orchestrator.UpdateResult
	updateErr         error
	gotUpdateAccounts []orchestrator.AccountOption
	runFn             func(execute func(string, json.RawMessage) (string, error)) (string, error)
	gotRunPrompt      string
	gotRunTools       []orchestrator.AgentTool
	classifyPairs     []orchestrator.Pair
	updateCalled      bool
}

func (o *fakeOrchestrator) ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error) {
	return orchestrator.CreateResult{}, nil
}
func (o *fakeOrchestrator) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error) {
	o.gotUpdateAccounts = accounts
	o.updateCalled = true
	return o.updateResult, o.updateErr
}
func (o *fakeOrchestrator) ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error) {
	return orchestrator.DeleteResult{}, nil
}
func (o *fakeOrchestrator) ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error) {
	return orchestrator.OnboardingResult{}, nil
}
func (o *fakeOrchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return "", nil
}

var errRunNotWired = errors.New("Run is not wired in this test")

func swallowTurnDone(execute func(string, json.RawMessage) (string, error)) func(string, json.RawMessage) (string, error) {
	return func(name string, args json.RawMessage) (string, error) {
		result, err := execute(name, args)
		if errors.Is(err, orchestrator.ErrAgentTurnDone) {
			return result, nil
		}
		return result, err
	}
}

func (o *fakeOrchestrator) Run(_ context.Context, systemPrompt, _ string, _ []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(string, json.RawMessage) (string, error)) (string, error) {
	o.gotRunPrompt = systemPrompt
	o.gotRunTools = tools
	if o.runFn == nil {
		return "", errRunNotWired
	}
	return o.runFn(swallowTurnDone(execute))
}
func (o *fakeOrchestrator) ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error) {
	return orchestrator.CategoryCreateResult{}, nil
}
func (o *fakeOrchestrator) ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error) {
	return orchestrator.AccountManageResult{}, nil
}

func (o *fakeOrchestrator) ClassifyCategories(ctx context.Context, message string, rows []orchestrator.ClassifyRow, taxonomy []orchestrator.TaxonomyEntry) []orchestrator.Pair {
	return o.classifyPairs
}

type fakeActionsRepo struct {
	rows    []*pendingaction.PendingAction
	nextID  uint64
	deleted []uint64
}

func (r *fakeActionsRepo) Insert(a *pendingaction.PendingAction) error {
	r.nextID++
	a.ID = r.nextID
	r.rows = append(r.rows, a)
	return nil
}

func (r *fakeActionsRepo) NextForUser(userID uint64) (*pendingaction.PendingAction, error) {
	var best *pendingaction.PendingAction
	for _, a := range r.rows {
		if a.UserID != userID {
			continue
		}
		if best == nil || a.Position < best.Position || (a.Position == best.Position && a.ID < best.ID) {
			best = a
		}
	}
	if best == nil {
		return nil, pendingaction.ErrNoPendingAction
	}
	return best, nil
}

func (r *fakeActionsRepo) Update(a *pendingaction.PendingAction) error {
	for i, row := range r.rows {
		if row.ID == a.ID {
			r.rows[i] = a
			return nil
		}
	}
	return errors.New("fakeActionsRepo: update: no row with that id")
}

func (r *fakeActionsRepo) Delete(id uint64) error {
	r.deleted = append(r.deleted, id)
	kept := r.rows[:0]
	for _, a := range r.rows {
		if a.ID != id {
			kept = append(kept, a)
		}
	}
	r.rows = kept
	return nil
}

func (r *fakeActionsRepo) CountForUser(userID uint64) (int64, error) {
	var n int64
	for _, a := range r.rows {
		if a.UserID == userID {
			n++
		}
	}
	return n, nil
}

type fakeMetricRepo struct {
	logged        []loggedIntent
	resolved      []string
	resolvedIDs   [][]uint
	queuedIntents []string
}

type loggedIntent struct {
	intent  string
	outcome string
}

func (f *fakeMetricRepo) Log(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error {
	f.logged = append(f.logged, loggedIntent{intent: intent, outcome: outcome})
	return nil
}

func (f *fakeMetricRepo) Resolve(userID uint64, outcome string, movementIDs []uint) error {
	f.resolved = append(f.resolved, outcome)
	f.resolvedIDs = append(f.resolvedIDs, movementIDs)
	return nil
}

func (f *fakeMetricRepo) SetIntentIfQueued(userID uint64, intent string) error {
	f.queuedIntents = append(f.queuedIntents, intent)
	return nil
}

type fakeSubcategoryRepoFull struct {
	byCategoryAndSub map[string]*subcategory.Subcategory
	all              []subcategory.Subcategory
	allErr           error
	categories       []string
	owned            []subcategory.Subcategory
	ownedErr         error
	deletedUserID    uint64
	deletedID        uint64
	deleteCalls      int
	deleteErr        error
	reloadCalls      int
}

func (r *fakeSubcategoryRepoFull) FindByCategoryAndSubcategory(userID uint64, category, sub string) (*subcategory.Subcategory, error) {
	s, ok := r.byCategoryAndSub[category+"|"+sub]
	if !ok {
		return nil, subcategory.ErrSubcategoryNotFound
	}
	return s, nil
}
func (r *fakeSubcategoryRepoFull) FindAllForUser(userID uint64) ([]subcategory.Subcategory, error) {
	return r.all, r.allErr
}
func (r *fakeSubcategoryRepoFull) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	return r.categories, nil
}
func (r *fakeSubcategoryRepoFull) IconForCategory(userID uint64, category string) string {
	return "📂"
}
func (r *fakeSubcategoryRepoFull) Insert(s *subcategory.Subcategory) error { return nil }
func (r *fakeSubcategoryRepoFull) Reload() error {
	r.reloadCalls++
	return nil
}
func (r *fakeSubcategoryRepoFull) Delete(userID uint64, id uint64) error {
	r.deletedUserID, r.deletedID = userID, id
	r.deleteCalls++
	return r.deleteErr
}
func (r *fakeSubcategoryRepoFull) FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error) {
	return r.owned, r.ownedErr
}

type fakeAccountRepoFull struct {
	byCurrency   map[currency.Currency]*account.Account
	byUserID     []account.Account
	byUserIDErr  error
	byID         map[uint64]*account.Account
	inserted     []account.Account
	insertErr    error
	renamedID    uint64
	renamedName  string
	renameErr    error
	unsetCalls   []currency.Currency
	setDefaultID uint64
}

func (r *fakeAccountRepoFull) Insert(a *account.Account) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	a.ID = uint(len(r.inserted) + 100)
	r.inserted = append(r.inserted, *a)
	r.byUserID = append(r.byUserID, *a)
	return nil
}
func (r *fakeAccountRepoFull) FindDefaultByCurrency(userID uint64, c currency.Currency) (*account.Account, error) {
	a, ok := r.byCurrency[c]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}
func (r *fakeAccountRepoFull) HasDefaultForCurrency(userID uint64, c currency.Currency) bool {
	if _, ok := r.byCurrency[c]; ok {
		return true
	}
	for _, a := range r.byUserID {
		if a.Currency == c && a.IsDefault {
			return true
		}
	}
	return false
}
func (r *fakeAccountRepoFull) FindByUserID(userID uint64) ([]account.Account, error) {
	return r.byUserID, r.byUserIDErr
}
func (r *fakeAccountRepoFull) GetAccount(id uint64) (*account.Account, error) {
	a, ok := r.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}
func (r *fakeAccountRepoFull) Rename(accountID uint64, name string) error {
	if r.renameErr != nil {
		return r.renameErr
	}
	r.renamedID, r.renamedName = accountID, name
	return nil
}
func (r *fakeAccountRepoFull) UnsetDefault(userID uint64, cur currency.Currency) error {
	r.unsetCalls = append(r.unsetCalls, cur)
	delete(r.byCurrency, cur)
	return nil
}
func (r *fakeAccountRepoFull) SetDefault(accountID uint64) error {
	r.setDefaultID = accountID
	return nil
}

type fakeMovementRepoFull struct {
	inserted           []movement.Movement
	batches            [][]movement.Movement
	balances           map[uint64]string
	replacedOldIDs     []uint
	replaced           []movement.Movement
	deletedIDs         []uint
	similar            []movement.Movement
	similarErr         error
	insertErr          error
	openings           []movement.AccountOpening
	reassignFrom       uint64
	reassignTo         uint64
	reassignCalls      int
	countForUser       int64
	countBySubcategory int64
	countErr           error
	reassignedFrom     uint64
	reassignedTo       uint64
	reassignedUser     uint64
	reassignSubCalls   int
	reassignSubErr     error
	topDescriptions    []string
	dayCounts          []movement.DayCount
	sumRows            func(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
}

func (r *fakeMovementRepoFull) InsertBatch(ms []movement.Movement) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.inserted = ms
	r.batches = append(r.batches, ms)
	return nil
}
func (r *fakeMovementRepoFull) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	s, ok := r.balances[accountID]
	if !ok {
		return decimal.Zero, nil
	}
	d, _ := decimal.NewFromString(s)
	return d, nil
}
func (r *fakeMovementRepoFull) ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error {
	r.replacedOldIDs = oldIDs
	r.replaced = newMovements
	return nil
}
func (r *fakeMovementRepoFull) FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error) {
	return r.similar, nil
}
func (r *fakeMovementRepoFull) FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error) {
	return r.similar, r.similarErr
}
func (r *fakeMovementRepoFull) SoftDeleteByIDs(ids []uint) error {
	r.deletedIDs = ids
	return nil
}
func (r *fakeMovementRepoFull) InsertAccountsWithOpenings(items []movement.AccountOpening) error {
	r.openings = items
	return nil
}
func (r *fakeMovementRepoFull) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	if r.sumRows != nil {
		return r.sumRows(q, groupBy)
	}
	return nil, nil
}
func (r *fakeMovementRepoFull) CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error) {
	return r.dayCounts, nil
}
func (r *fakeMovementRepoFull) ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	return nil, nil
}
func (r *fakeMovementRepoFull) ReassignAccount(fromID, toID uint64) error {
	r.reassignFrom, r.reassignTo = fromID, toID
	r.reassignCalls++
	return nil
}
func (r *fakeMovementRepoFull) CountForUser(userID uint64) (int64, error) {
	return r.countForUser, nil
}
func (r *fakeMovementRepoFull) CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error) {
	return r.countBySubcategory, r.countErr
}
func (r *fakeMovementRepoFull) ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error {
	r.reassignedFrom, r.reassignedTo, r.reassignedUser = fromID, toID, userID
	r.reassignSubCalls++
	return r.reassignSubErr
}
func (r *fakeMovementRepoFull) TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error) {
	return r.topDescriptions, nil
}

type fakeConvStore struct {
	flowName, stepName string
	data               conversation.Data
	updatedAt          time.Time
	found              bool
}

func (s *fakeConvStore) Get(userID uint64) (string, string, conversation.Data, time.Time, bool, error) {
	return s.flowName, s.stepName, s.data, s.updatedAt, s.found, nil
}
func (s *fakeConvStore) Set(userID uint64, flowName, stepName string, data conversation.Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	s.updatedAt = time.Now()
	return nil
}
func (s *fakeConvStore) Clear(userID uint64) error {
	s.found = false
	return nil
}

type stubChatHistory struct{}

func (stubChatHistory) Recent(userID uint64) ([]chathistory.Turn, error)    { return nil, nil }
func (stubChatHistory) Append(userID uint64, question, answer string) error { return nil }

func strPtr(s string) *string { return &s }

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("decimal.NewFromString(%q): %v", s, err)
	}
	return d
}

func uint64Ptr(v uint64) *uint64 { return &v }

func newSubForTest(id uint, category, sub string) *subcategory.Subcategory {
	s := &subcategory.Subcategory{Category: category, Subcategory: sub}
	s.ID = id
	return s
}

func acct(id uint64, cur currency.Currency, def bool) account.Account {
	return account.Account{Model: gorm.Model{ID: uint(id)}, UserID: 1, Currency: cur, IsDefault: def}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newLoopServices(t *testing.T) *fakeServices {
	t.Helper()
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	engine.Register(flow.NewMovementDeleteFlow())
	engine.Register(flow.NewMovementUpdateConfirmFlow())
	return &fakeServices{
		engine:        engine,
		orch:          &fakeOrchestrator{},
		actions:       &fakeActionsRepo{},
		movements:     &fakeMovementRepoFull{},
		accounts:      &fakeAccountRepoFull{},
		subcategories: &fakeSubcategoryRepoFull{},
		chatHistory:   &stubChatHistory{},
		metrics:       &fakeMetricRepo{},
	}
}

func newDispatchServices(t *testing.T, repo *fakeActionsRepo) *fakeServices {
	t.Helper()
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	engine.Register(flow.NewMovementDeleteFlow())
	return &fakeServices{
		engine:        engine,
		actions:       repo,
		movements:     &fakeMovementRepoFull{},
		accounts:      &fakeAccountRepoFull{},
		subcategories: &fakeSubcategoryRepoFull{},
		chatHistory:   &stubChatHistory{},
		metrics:       &fakeMetricRepo{},
		orch:          &fakeOrchestrator{},
	}
}
