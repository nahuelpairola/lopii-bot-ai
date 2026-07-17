package miniapp

import (
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/movement"
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

type controller struct {
	movements movementReader
	accounts  accountReader
	users     userLookup
	botToken  string
}

func NewController(movements movementReader, accounts accountReader, users userLookup, botToken string) *controller {
	return &controller{movements: movements, accounts: accounts, users: users, botToken: botToken}
}

// RegisterRoutes mounts every /app route on engine, all guarded by
// authInitData except the static asset mount (CSS/JS need to load before
// any auth check can run client-side).
func (c *controller) RegisterRoutes(engine *gin.Engine) {
	app := engine.Group("/app")
	registerStatic(app)

	authed := app.Group("")
	authed.Use(authInitData(c.botToken, c.users))
	authed.GET("/resumen", c.handleResumen)
	authed.GET("/categorias", c.handleCategorias)
	authed.GET("/categorias/:category", c.handleCategoriaDrill)
	authed.GET("/cuentas", c.handleCuentas)
}
