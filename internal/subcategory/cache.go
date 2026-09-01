package subcategory

import "sort"

type allLoader interface {
	FindAll() ([]Subcategory, error)
	Insert(s *Subcategory) error
	Delete(userID uint64, id uint64) error
}

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
		if IsReserved(s.Category) {
			continue
		}
		if seen[s.Category] {
			continue
		}
		seen[s.Category] = true
		categories = append(categories, s.Category)
	}
	sort.Strings(categories)
	return categories, nil
}

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

func (c *Cache) Insert(s *Subcategory) error {
	return c.loader.Insert(s)
}

func (c *Cache) FindOwnedByUser(userID uint64) ([]Subcategory, error) {
	own := c.perUser[userID]
	out := make([]Subcategory, len(own))
	copy(out, own)
	return out, nil
}

func (c *Cache) Delete(userID uint64, id uint64) error {
	return c.loader.Delete(userID, id)
}

func iconOrFallback(icon string) string {
	if icon == "" {
		return "📂"
	}
	return icon
}
