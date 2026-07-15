package subcategory

import (
	"errors"
	"testing"
)

type fakeLoader struct {
	subs []Subcategory
	err  error
}

func (f *fakeLoader) FindAll() ([]Subcategory, error) { return f.subs, f.err }
func (f *fakeLoader) Insert(s *Subcategory) error {
	f.subs = append(f.subs, *s)
	return nil
}

func u(id uint64) *uint64 { return &id }

func TestNewCache_PropagatesLoaderError(t *testing.T) {
	wantErr := errors.New("db down")
	if _, err := NewCache(&fakeLoader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestCache_FindAllForUser_IncludesGlobalAndOwnRowsOnly(t *testing.T) {
	loader := &fakeLoader{subs: []Subcategory{
		{Category: "Alimentación", Subcategory: "Café", IsGlobal: true},
		{Category: "Mascotas", Subcategory: "Veterinario", UserID: u(1)},
		{Category: "Mascotas", Subcategory: "Otro user", UserID: u(2)},
	}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	got, _ := cache.FindAllForUser(1)
	if len(got) != 2 {
		t.Fatalf("FindAllForUser(1) = %d rows, want 2 (1 global + 1 own)", len(got))
	}
}

func TestCache_DistinctCategoriesForUser_SortedUnique(t *testing.T) {
	loader := &fakeLoader{subs: []Subcategory{
		{Category: "Transporte", Subcategory: "Nafta", IsGlobal: true},
		{Category: "Alimentación", Subcategory: "Café", IsGlobal: true},
		{Category: "Alimentación", Subcategory: "Supermercado", IsGlobal: true},
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

func TestDistinctCategoriesForUser_ExcludesReserved(t *testing.T) {
	loader := &fakeLoader{subs: []Subcategory{
		{Category: "Otros", Subcategory: "Regalos / donaciones", IsGlobal: true},
		{Category: "Sistema", Subcategory: "Saldo inicial", IsGlobal: true},
		{Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW", IsGlobal: true},
	}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	cats, err := cache.DistinctCategoriesForUser(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cats {
		if c == "Sistema" || c == "PENDING_REVIEW" {
			t.Errorf("reserved category %q leaked into picker list %v", c, cats)
		}
	}
	if len(cats) != 1 || cats[0] != "Otros" {
		t.Errorf("cats = %v, want [Otros]", cats)
	}
}

func TestIsReserved(t *testing.T) {
	for _, name := range []string{"Sistema", "sistema", "PENDING_REVIEW", "pending_review", " Sistema "} {
		if !IsReserved(name) {
			t.Errorf("IsReserved(%q) = false, want true", name)
		}
	}
	if IsReserved("Otros") {
		t.Error("IsReserved(Otros) = true, want false")
	}
}

func TestCache_FindByCategoryAndSubcategory_PerUserIsolation(t *testing.T) {
	loader := &fakeLoader{subs: []Subcategory{
		{Category: "Mascotas", Subcategory: "Veterinario", UserID: u(1), Icon: "🐶"},
		{Category: "Mascotas", Subcategory: "Veterinario", UserID: u(2), Icon: "🐱"},
	}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	s1, err := cache.FindByCategoryAndSubcategory(1, "Mascotas", "Veterinario")
	if err != nil || s1.Icon != "🐶" {
		t.Errorf("user 1: got %+v, err %v, want icon 🐶", s1, err)
	}
	s2, err := cache.FindByCategoryAndSubcategory(2, "Mascotas", "Veterinario")
	if err != nil || s2.Icon != "🐱" {
		t.Errorf("user 2: got %+v, err %v, want icon 🐱", s2, err)
	}
}

func TestCache_FindByCategoryAndSubcategory_FallsBackToGlobal(t *testing.T) {
	loader := &fakeLoader{subs: []Subcategory{{Category: "Transporte", Subcategory: "Nafta", IsGlobal: true}}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	s, err := cache.FindByCategoryAndSubcategory(1, "Transporte", "Nafta")
	if err != nil || s.Category != "Transporte" {
		t.Errorf("got %+v, err %v, want the global row", s, err)
	}
}

func TestCache_FindByCategoryAndSubcategory_NotFound(t *testing.T) {
	cache, err := NewCache(&fakeLoader{})
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if _, err := cache.FindByCategoryAndSubcategory(1, "X", "Y"); !errors.Is(err, ErrSubcategoryNotFound) {
		t.Errorf("err = %v, want ErrSubcategoryNotFound", err)
	}
}

func TestCache_IconForCategory_OwnBeforeGlobal(t *testing.T) {
	loader := &fakeLoader{subs: []Subcategory{
		{Category: "Mascotas", Subcategory: "Veterinario", IsGlobal: true, Icon: "🗂️"},
		{Category: "Mascotas", Subcategory: "Comida", UserID: u(1), Icon: "🐶"},
	}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if got := cache.IconForCategory(1, "Mascotas"); got != "🐶" {
		t.Errorf("IconForCategory = %q, want the user's own row's icon 🐶", got)
	}
}

func TestCache_IconForCategory_FallbackWhenUnknown(t *testing.T) {
	cache, err := NewCache(&fakeLoader{})
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if got := cache.IconForCategory(1, "Inventada"); got != "📂" {
		t.Errorf("IconForCategory = %q, want fallback 📂", got)
	}
}

func TestCache_Reload_PicksUpNewRow(t *testing.T) {
	loader := &fakeLoader{subs: []Subcategory{{Category: "Transporte", Subcategory: "Nafta", IsGlobal: true}}}
	cache, err := NewCache(loader)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	loader.subs = append(loader.subs, Subcategory{Category: "Mascotas", Subcategory: "Veterinario", UserID: u(1)})
	if err := cache.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if _, err := cache.FindByCategoryAndSubcategory(1, "Mascotas", "Veterinario"); err != nil {
		t.Errorf("after Reload, expected the new row to be found, got err %v", err)
	}
}
