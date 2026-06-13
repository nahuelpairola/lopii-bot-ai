package server

import (
	"github.com/gin-gonic/gin"
	"lopiibot.com/src/config"
	controller "lopiibot.com/src/infrastructure/server/controllers"
	"lopiibot.com/src/registry"
)

type httpServer struct {
	engine *gin.Engine
}

var server httpServer

func InitServer(conf *config.Config) error {

	r, err := registry.NewRegistry(conf)
	if err != nil {
		return err
	}

	appContainer, err := r.InitAppContainer()
	if err != nil {
		return err
	}

	ginEngine, err := setupRouter(appContainer)
	if err != nil {
		return err
	}

	server = httpServer{
		engine: ginEngine,
	}

	return server.engine.Run()
}

func setupRouter(appContainer *registry.AppContainer) (*gin.Engine, error) {
	router := gin.Default()

	healthController := controller.NewHealthController()
	healthController.RegisterRoutes(router)

	return router, nil
}
