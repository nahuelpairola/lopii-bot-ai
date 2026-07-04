package subcategory

import "sort"

// globalLoader is the local interface Cache needs from the DB-backed
// repository — just enough to load the global taxonomy once at startup.
type globalLoader interface {
	FindAllGlobal() ([]Subcategory, error)
}

// Cache holds every global subcategory in memory, loaded once at server
// startup (see server.go). Subcategories are 100% global today — no
// user-creation flow exists yet — so this safely replaces repeated DB
// reads from the messaging package's CREATE gap-fill flow and free-text
// classification, both of which otherwise hit the DB on every message.
// When a subcategory-creation flow is built, it must invalidate/refresh
// this cache after every Insert — there's nothing to invalidate against
// yet because nothing can mutate the table at runtime today.
type Cache struct {
	all []Subcategory
}

func NewCache(loader globalLoader) (*Cache, error) {
	all, err := loader.FindAllGlobal()
	if err != nil {
		return nil, err
	}
	return &Cache{all: all}, nil
}

func (c *Cache) FindAllForUser(userID uint64) ([]Subcategory, error) {
	return c.all, nil
}

func (c *Cache) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	seen := make(map[string]bool)
	categories := make([]string, 0, len(c.all))
	for _, s := range c.all {
		if seen[s.Category] {
			continue
		}
		seen[s.Category] = true
		categories = append(categories, s.Category)
	}
	sort.Strings(categories)
	return categories, nil
}

func (c *Cache) FindByCategoryAndSubcategory(category, sub string) (*Subcategory, error) {
	for _, s := range c.all {
		if s.Category == category && s.Subcategory == sub {
			found := s
			return &found, nil
		}
	}
	return nil, ErrSubcategoryNotFound
}
