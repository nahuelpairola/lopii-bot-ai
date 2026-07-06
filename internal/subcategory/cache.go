package subcategory

import "sort"

// allLoader is the local interface Cache needs from the DB-backed
// repository: FindAll loads every row (global + user-created) at server
// startup and again on Reload() after an Insert; Insert writes a new
// user-created row straight through to Postgres — Cache is read-through
// for lookups but never mutates its own in-memory slices directly.
type allLoader interface {
	FindAll() ([]Subcategory, error)
	Insert(s *Subcategory) error
}

// Cache holds every subcategory in memory: global is the shared ~90-row
// seeded taxonomy (same backing slice read by every user, never copied
// per user); perUser holds only each user's own custom rows — usually a
// handful. Loaded once at server startup (see server.go) and refreshed
// via Reload() after a user creates a new subcategory (see
// internal/controller/messaging/subcategory_setup_finish.go).
type Cache struct {
	loader  allLoader
	global  []Subcategory
	perUser map[uint64][]Subcategory
}

func NewCache(loader allLoader) (*Cache, error) {
	c := &Cache{loader: loader}
	if err := c.Reload(); err != nil {
		return nil, err
	}
	return c, nil
}

// Reload re-runs the loader query and rebuilds global/perUser from
// scratch. A full reload (not an incremental add) is one extra query per
// subcategory *created* — a rare event, not per message — so simplicity
// wins over an incremental Cache.Add.
func (c *Cache) Reload() error {
	all, err := c.loader.FindAll()
	if err != nil {
		return err
	}
	global := make([]Subcategory, 0, len(all))
	perUser := make(map[uint64][]Subcategory)
	for _, s := range all {
		if s.IsGlobal {
			global = append(global, s)
			continue
		}
		if s.UserID != nil {
			perUser[*s.UserID] = append(perUser[*s.UserID], s)
		}
	}
	c.global = global
	c.perUser = perUser
	return nil
}

func (c *Cache) FindAllForUser(userID uint64) ([]Subcategory, error) {
	combined := make([]Subcategory, 0, len(c.global)+len(c.perUser[userID]))
	combined = append(combined, c.global...)
	combined = append(combined, c.perUser[userID]...)
	return combined, nil
}

func (c *Cache) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	all, _ := c.FindAllForUser(userID)
	seen := make(map[string]bool)
	categories := make([]string, 0, len(all))
	for _, s := range all {
		if seen[s.Category] {
			continue
		}
		seen[s.Category] = true
		categories = append(categories, s.Category)
	}
	sort.Strings(categories)
	return categories, nil
}

// FindByCategoryAndSubcategory searches the user's own rows first, falls
// back to the shared global taxonomy — two different users can have a
// same-named custom category (the DB's own unique index is scoped by
// user_id), so userID must disambiguate.
func (c *Cache) FindByCategoryAndSubcategory(userID uint64, category, sub string) (*Subcategory, error) {
	for _, s := range c.perUser[userID] {
		if s.Category == category && s.Subcategory == sub {
			found := s
			return &found, nil
		}
	}
	for _, s := range c.global {
		if s.Category == category && s.Subcategory == sub {
			found := s
			return &found, nil
		}
	}
	return nil, ErrSubcategoryNotFound
}

// IconForCategory finds any row of the given category (user's own first,
// then global) and returns its icon — used where only a category name is
// known, not a full category+subcategory pair (e.g. the CREATE gap-fill
// category picker in movement_create_flow.go).
func (c *Cache) IconForCategory(userID uint64, category string) string {
	for _, s := range c.perUser[userID] {
		if s.Category == category {
			return iconOrFallback(s.Icon)
		}
	}
	for _, s := range c.global {
		if s.Category == category {
			return iconOrFallback(s.Icon)
		}
	}
	return "📂"
}

// Insert writes a new subcategory straight through to the DB — callers
// must follow up with Reload() to make it visible in the cache (see
// internal/controller/messaging/subcategory_setup_finish.go).
func (c *Cache) Insert(s *Subcategory) error {
	return c.loader.Insert(s)
}

func iconOrFallback(icon string) string {
	if icon == "" {
		return "📂"
	}
	return icon
}
