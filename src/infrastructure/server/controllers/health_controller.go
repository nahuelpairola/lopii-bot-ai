package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type healthController struct {
}

func NewHealthController() *healthController {
	return &healthController{}
}

func (c *healthController) RegisterRoutes(engine *gin.Engine) {
	engine.GET("health", c.HealthCheck)
}

func (c *healthController) HealthCheck(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, "OK")
}
