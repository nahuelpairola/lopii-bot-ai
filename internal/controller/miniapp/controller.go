package miniapp

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/movement"
)

const hxRequestHeader = "HX-Request"

const (
	EntryPath      = templates.RouteOverview
	MenuButtonText = templates.AppName
)

type movementReader interface {
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
	MonthlyDeltasForAccount(accountID uint64) ([]movement.MonthlyDelta, error)
	ListForAccount(accountID uint64, from, to time.Time, limit, offset int) ([]movement.Movement, error)
	ListForUser(q movement.MovementQuery, limit, offset int) ([]movement.Movement, error)
}

type accountReader interface {
	FindByUserID(userID uint64) ([]account.Account, error)
}

type subcategoryReader interface {
	IconForCategory(userID uint64, category string) string
}

type controller struct {
	movements     movementReader
	accounts      accountReader
	subcategories subcategoryReader
	users         userLookup
	botToken      string
	botUsername   string
}

func NewController(
	movements movementReader,
	accounts accountReader,
	subcategories subcategoryReader,
	users userLookup,
	botToken string,
	botUsername string,
) *controller {
	return &controller{
		movements:     movements,
		accounts:      accounts,
		subcategories: subcategories,
		users:         users,
		botToken:      botToken,
		botUsername:   botUsername,
	}
}

func (c *controller) RegisterRoutes(engine *gin.Engine) {
	app := engine.Group(templates.AppPrefix)
	registerStatic(app)

	authed := app.Group("")
	authed.Use(authInitData(c.botToken, c.users))
	authed.GET("/"+templates.TabOverview, c.handleOverview)

	authed.GET("/"+templates.TabCategories, c.handleCategories)
	authed.GET("/"+templates.TabAccounts, c.handleAccounts)
	authed.GET("/"+templates.TabEvolution, c.handleEvolution)

}
