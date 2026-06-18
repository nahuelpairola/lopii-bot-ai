package server

import (
	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/config"
	healthCtrll "lopiibot.com/internal/controller/health"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/health"
)

type httpServer struct {
	engine *gin.Engine
}

var server httpServer

func InitServer(conf *config.Config) error {

	ginEngine := gin.Default()

	database, err := database.Initialize(database.Creds{
		Host:     conf.Database.Host,
		Name:     conf.Database.Name,
		Port:     conf.Database.Port,
		User:     conf.Database.User,
		Password: conf.Database.Password,
	})
	if err != nil {
		return err
	}

	healthChecker := health.NewHealthChecker(database)
	healthController := healthCtrll.NewController(healthChecker)
	healthController.RegisterRoutes(ginEngine)

	server = httpServer{
		engine: ginEngine,
	}

	return server.engine.Run()
}
