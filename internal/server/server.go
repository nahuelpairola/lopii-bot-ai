package server

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"lopiibot.com/internal/config"
	healthctrl "lopiibot.com/internal/controller/health"
	invitationctrl "lopiibot.com/internal/controller/invitation"
	messagingctrl "lopiibot.com/internal/controller/messaging"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/health"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/user"
)

type httpServer struct {
	engine *gin.Engine
}

var server httpServer

func InitServer(conf *config.Config) error {

	ginEngine := gin.Default()

	conn, err := initializeDatabase(conf)
	if err != nil {
		return err
	}

	healthChecker := health.NewHealthChecker(conn)
	healthController := healthctrl.NewController(healthChecker)
	healthController.RegisterRoutes(ginEngine)

	userRepo := user.NewRepository(conn)

	invitationRepo := invitation.NewRepository(conn)
	invitationController, err := invitationctrl.NewController(invitationRepo, conf.Telegram.Username)
	if err != nil {
		return err

	}

	invitationController.RegisterRoutes(ginEngine)

	tgBot, err := bot.New(conf.Telegram.Token)
	if err != nil {
		return err
	}

	messagingController := messagingctrl.NewController(userRepo, invitationRepo)
	messagingController.RegisterHandlers(tgBot)
	go tgBot.Start(context.Background())

	server = httpServer{engine: ginEngine}

	return server.engine.Run()
}

func initializeDatabase(conf *config.Config) (*database.Connection, error) {
	conn, err := database.Initialize(database.Creds{
		Host:     conf.Database.Host,
		Name:     conf.Database.Name,
		Port:     conf.Database.Port,
		User:     conf.Database.User,
		Password: conf.Database.Password,
	})
	if err != nil {
		return nil, err
	}

	if conf.Database.RunMigrations {
		if err = database.RunMigrations("../../migrations"); err != nil {
			fmt.Printf(err.Error())
			return nil, err
		}
	}

	return conn, nil
}
