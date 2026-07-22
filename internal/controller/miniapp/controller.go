package miniapp

import (
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/movement"
)

// hxRequestHeader is htmx's marker on every request it issues. Its presence
// is how the app tells a data-fetch (htmx, carries initData) from a
// full-page navigation (plain browser, no initData yet).
const hxRequestHeader = "HX-Request"

// EntryPath is the Mini App landing route (Telegram menu button → here).
// MenuButtonText is the button's label. Exported for server.go's
// SetChatMenuButton; single source is templates.
const (
	EntryPath      = templates.RouteOverview
	MenuButtonText = templates.AppName
)

// movementReader is the movement-repo surface this package needs — repo
// convention, consumer-local interface (grows as later tasks add views).
type movementReader interface {
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
	MonthlyDeltasForAccount(accountID uint64) ([]movement.MonthlyDelta, error)
}

// accountReader is the account-repo surface this package needs.
type accountReader interface {
	FindByUserID(userID uint64) ([]account.Account, error)
}

// subcategoryReader is the taxonomy surface this package needs: just the icon
// lookup, so a category renders with the same emoji the bot uses in chat.
type subcategoryReader interface {
	IconForCategory(userID uint64, category string) string
}

type controller struct {
	movements     movementReader
	accounts      accountReader
	subcategories subcategoryReader
	users         userLookup
	botToken      string
}

func NewController(movements movementReader, accounts accountReader, subcategories subcategoryReader, users userLookup, botToken string) *controller {
	return &controller{
		movements:     movements,
		accounts:      accounts,
		subcategories: subcategories,
		users:         users,
		botToken:      botToken,
	}
}

// RegisterRoutes mounts every /app route on engine, all guarded by
// authInitData except the static asset mount (CSS/JS need to load before
// any auth check can run client-side).
func (c *controller) RegisterRoutes(engine *gin.Engine) {
	app := engine.Group(templates.AppPrefix)
	registerStatic(app)

	authed := app.Group("")
	authed.Use(authInitData(c.botToken, c.users))
	authed.GET("/"+templates.TabOverview, c.handleOverview)
	// The subcategory drill is the same route with a ?category= param, not a
	// path segment: real category names contain "/".
	authed.GET("/"+templates.TabCategories, c.handleCategories)
	authed.GET("/"+templates.TabAccounts, c.handleAccounts)
	authed.GET("/"+templates.TabMatrix, c.handleMatrix)
}
