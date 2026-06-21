package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type healthChecker interface {
	IsHealthy() error
}

type controller struct {
	healthChecker
}

func NewController(healthChecker healthChecker) *controller {
	return &controller{healthChecker: healthChecker}
}

func (c *controller) RegisterRoutes(router *gin.Engine) {
	router.Any("/health/internal", c.internalHealthCheck)
	router.HEAD("/health/external", c.externalHealthCheck)
}

func (c *controller) internalHealthCheck(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, "OK")
}

func (c *controller) externalHealthCheck(ctx *gin.Context) {
	if err := c.IsHealthy(); err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.JSON(http.StatusOK, "OK")
}
