package settings

import (
	"context"
	"errors"
	"time"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

var errFake = errors.New("db down")

type fakeStateStore struct {
	flowName  string
	stepName  string
	data      conversation.Data
	updatedAt time.Time
	found     bool
}

func (s *fakeStateStore) Get(userID uint64) (string, string, conversation.Data, time.Time, bool, error) {
	return s.flowName, s.stepName, s.data, s.updatedAt, s.found, nil
}

func (s *fakeStateStore) Set(userID uint64, flowName, stepName string, data conversation.Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	s.updatedAt = time.Now()
	return nil
}

func (s *fakeStateStore) Clear(userID uint64) error {
	s.found = false
	return nil
}

type fakeOwnedLister struct {
	owned []subcategory.Subcategory
	err   error
}

func (f fakeOwnedLister) FindOwnedByUser(uint64) ([]subcategory.Subcategory, error) {
	return f.owned, f.err
}

func ownedSub(id uint, category, sub, icon string) subcategory.Subcategory {
	s := subcategory.Subcategory{Category: category, Subcategory: sub, Icon: icon}
	s.ID = id
	return s
}

type testServices struct {
	engine *conversation.Engine

	accounts     []account.Account
	accountsErr  error
	all          []subcategory.Subcategory
	allErr       error
	owned        []subcategory.Subcategory
	ownedErr     error
	categories   []string
	byCatSub     map[string]*subcategory.Subcategory
	descriptions []string
	rem          *reminder.Reminder

	match        *orchestrator.CategoryMatch
	proposal     *orchestrator.CategoryProposal
	classifyErr  error
	onboarding   orchestrator.OnboardingResult
	accManage    orchestrator.AccountManageResult
	accManageErr error
	calls        int
	gotText      string
	gotTaxonomy  []orchestrator.TaxonomyEntry

	texts       []string
	resolved    []string
	groqHandled bool
	startedFlow string
}

func (s *testServices) FindUserAccounts(uint64) ([]account.Account, error) {
	return s.accounts, s.accountsErr
}

func (s *testServices) SubcategoriesFindAllForUser(uint64) ([]subcategory.Subcategory, error) {
	return s.all, s.allErr
}

func (s *testServices) FindSubcategory(_ uint64, category, sub string) (*subcategory.Subcategory, error) {
	if found, ok := s.byCatSub[category+"|"+sub]; ok {
		return found, nil
	}
	return nil, subcategory.ErrSubcategoryNotFound
}

func (s *testServices) DistinctCategoriesForUser(uint64) ([]string, error) {
	return s.categories, nil
}

func (s *testServices) FindOwnedSubcategories(uint64) ([]subcategory.Subcategory, error) {
	return s.owned, s.ownedErr
}

func (s *testServices) TopDescriptionsBySubcategory(uint64, uint64, int) ([]string, error) {
	return s.descriptions, nil
}

func (s *testServices) RemindersFindByUserID(uint64) (*reminder.Reminder, error) {
	if s.rem == nil {
		return nil, errFake
	}
	return s.rem, nil
}

func (s *testServices) ResolveAccountManage(_ context.Context, _ string, _ []orchestrator.AccountOption) (orchestrator.AccountManageResult, error) {
	return s.accManage, s.accManageErr
}

func (s *testServices) ClassifyOnboarding(context.Context, string) (orchestrator.OnboardingResult, error) {
	return s.onboarding, nil
}

func (s *testServices) ClassifyCategoryCreate(_ context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error) {
	s.calls++
	s.gotText, s.gotTaxonomy = text, taxonomy
	if s.classifyErr != nil {
		return orchestrator.CategoryCreateResult{}, s.classifyErr
	}
	return orchestrator.CategoryCreateResult{Match: s.match, Proposal: s.proposal}, nil
}

func (s *testServices) SendText(_ context.Context, _ messenger.Chat, text string) {
	s.texts = append(s.texts, text)
}

func (s *testServices) SendPrompt(context.Context, messenger.Chat, conversation.Prompt) {}

func (s *testServices) StartFlow(_ context.Context, _ messenger.Chat, userID uint64, flowName string, seed conversation.Data, _ string) error {
	s.startedFlow = flowName
	_, err := s.engine.StartWithData(userID, flowName, seed)
	return err
}

func (s *testServices) EngineStartWithData(userID uint64, flowName string, seed conversation.Data) (conversation.Prompt, error) {
	return s.engine.StartWithData(userID, flowName, seed)
}

func (s *testServices) ResolveMetric(_ context.Context, _ uint64, outcome string, _ ...uint) {
	s.resolved = append(s.resolved, outcome)
}

func (s *testServices) HandleGroqError(context.Context, messenger.Chat, uint64, string, error) (bool, error) {
	return s.groqHandled, nil
}
