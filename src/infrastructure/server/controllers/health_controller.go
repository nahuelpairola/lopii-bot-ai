package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"lopiibot.com/src/registry"
)

type healthController struct {
	appContainer *registry.AppContainer
}

func NewHealthController(app *registry.AppContainer) *healthController {
	return &healthController{
		appContainer: app,
	}
}

func (c *healthController) RegisterRoutes(engine *gin.Engine) {
	engine.Any("health/internal", c.internalHealthCheck)
	engine.HEAD("health/external", c.externalHealthCheck)
}

func (c *healthController) externalHealthCheck(ctx *gin.Context) {
	if err := c.appContainer.HealthHandler.IsHealthy(); err != nil {
		ctx.JSON(http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, "OK")
}

func (c *healthController) internalHealthCheck(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, "OK")
}
