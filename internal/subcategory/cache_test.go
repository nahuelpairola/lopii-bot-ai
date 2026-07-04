package subcategory

import (
	"errors"
	"testing"
)

type fakeGlobalLoader struct {
	subs []Subcategory
	err  error
}

func (f *fakeGlobalLoader) FindAllGlobal() ([]Subcategory, error) { return f.subs, f.err }

func TestNewCache_PropagatesLoaderError(t *testing.T) {
	wantErr := errors.New("db down")
	if _, err := NewCache(&fakeGlobalLoader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestCache_FindAllForUser_ReturnsLoadedGlobals(t *testing.T) {
	loader := &fakeGlobalLoader{subs: []Subcategory{{Category: "Alimentación", Subcategory: "Café"}}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	got, _ := cache.FindAllForUser(1)
	if len(got) != 1 || got[0].Subcategory != "Café" {
		t.Errorf("FindAllForUser = %+v, want 1 row Café", got)
	}
}

func TestCache_DistinctCategoriesForUser_SortedUnique(t *testing.T) {
	loader := &fakeGlobalLoader{subs: []Subcategory{
		{Category: "Transporte", Subcategory: "Nafta"},
		{Category: "Alimentación", Subcategory: "Café"},
		{Category: "Alimentación", Subcategory: "Supermercado"},
	}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	cats, _ := cache.DistinctCategoriesForUser(1)
	if len(cats) != 2 || cats[0] != "Alimentación" || cats[1] != "Transporte" {
		t.Errorf("DistinctCategoriesForUser = %v, want [Alimentación Transporte]", cats)
	}
}

func TestCache_FindByCategoryAndSubcategory_Found(t *testing.T) {
	loader := &fakeGlobalLoader{subs: []Subcategory{{Category: "Transporte", Subcategory: "Nafta"}}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	s, err := cache.FindByCategoryAndSubcategory("Transporte", "Nafta")
	if err != nil {
		t.Fatalf("FindByCategoryAndSubcategory: %v", err)
	}
	if s.Category != "Transporte" {
		t.Errorf("got category %q, want Transporte", s.Category)
	}
}

func TestCache_FindByCategoryAndSubcategory_NotFound(t *testing.T) {
	cache, err := NewCache(&fakeGlobalLoader{})
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if _, err := cache.FindByCategoryAndSubcategory("X", "Y"); !errors.Is(err, ErrSubcategoryNotFound) {
		t.Errorf("err = %v, want ErrSubcategoryNotFound", err)
	}
}
