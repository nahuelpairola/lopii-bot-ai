package messaging

import (
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/subcategory"
)

type fakeSubcatFinishRepo struct {
	inserted      []subcategory.Subcategory
	reloaded      bool
	owned         []subcategory.Subcategory
	deletedUserID uint64
	deletedID     uint64
	deleteCalls   int
	deleteErr     error
}

func (r *fakeSubcatFinishRepo) FindByCategoryAndSubcategory(userID uint64, category, sub string) (*subcategory.Subcategory, error) {
	return nil, subcategory.ErrSubcategoryNotFound
}
func (r *fakeSubcatFinishRepo) FindAllForUser(userID uint64) ([]subcategory.Subcategory, error) {
	return nil, nil
}
func (r *fakeSubcatFinishRepo) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	return nil, nil
}
func (r *fakeSubcatFinishRepo) IconForCategory(userID uint64, category string) string { return "📂" }
func (r *fakeSubcatFinishRepo) Insert(s *subcategory.Subcategory) error {
	r.inserted = append(r.inserted, *s)
	return nil
}
func (r *fakeSubcatFinishRepo) Reload() error {
	r.reloaded = true
	return nil
}
func (r *fakeSubcatFinishRepo) Delete(userID uint64, id uint64) error {
	r.deletedUserID, r.deletedID = userID, id
	r.deleteCalls++
	return r.deleteErr
}
func (r *fakeSubcatFinishRepo) FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error) {
	return r.owned, nil
}

func TestInsertNewSubcategory_NewCategory_SetsIsGlobalFalseAndIcon(t *testing.T) {
	repo := &fakeSubcatFinishRepo{}
	c := &controller{subcategories: repo}

	data := conversation.Data{
		conversation.UserIDKey:    uint64(7),
		"category":                "Mascotas",
		"category_is_new":         "true",
		"category_icon":           "🐶",
		"subcategory":             "Veterinario",
		"subcategory_description": "Consultas y controles de mascotas",
	}

	if err := c.insertNewSubcategory(data); err != nil {
		t.Fatalf("insertNewSubcategory: %v", err)
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(repo.inserted))
	}
	got := repo.inserted[0]
	if got.IsGlobal || got.UserID == nil || *got.UserID != 7 || got.Icon != "🐶" || got.Category != "Mascotas" || got.Subcategory != "Veterinario" {
		t.Errorf("inserted row = %+v, want user-scoped Mascotas›Veterinario with icon 🐶", got)
	}
	if got.Description != "Consultas y controles de mascotas" {
		t.Errorf("inserted row's Description = %q, want the description step's answer", got.Description)
	}
	if !repo.reloaded {
		t.Error("expected Cache.Reload to be called after Insert")
	}
}

func TestFinishSubcategorySetupFlow_Cancelled_SkipsInsert(t *testing.T) {
	repo := &fakeSubcatFinishRepo{}
	c := &controller{subcategories: repo}

	c.finishSubcategorySetupFlow(nil, &messenger.FakeChat{}, conversation.Data{"cancelled": "true"})

	if len(repo.inserted) != 0 {
		t.Error("expected no insert when cancelled=true")
	}
}
