package server

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/config"
	healthctrl "lopiibot.com/internal/controller/health"
	invitationctrl "lopiibot.com/internal/controller/invitation"
	messagingctrl "lopiibot.com/internal/controller/messaging"
	"lopiibot.com/internal/conversation"
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

	invitationRepo := invitation.NewRepository(conn)
	invitationController, err := invitationctrl.NewController(invitationRepo, conf.Telegram.Username)
	if err != nil {
		return err
	}

	invitationController.RegisterRoutes(ginEngine)

	userRepo := user.NewRepository(conn)
	accountRepo := account.NewRepository(conn)
	// subcategoryRepo := subcategory.NewRepository(conn)
	convoRepo := conversation.NewRepository(conn)

	// El motor de conversaciones se arma una vez. Cada Flow se registra
	// acá también una sola vez (son estáticos, no dependen de ningún
	// usuario en particular) y queda validado antes de levantar el bot:
	// si algún Step referencia un paso inexistente, el server no arranca.
	convoEngine := conversation.NewEngine(convoRepo)

	accountSetupFlow, err := account.NewSetupFlow(accountRepo, accountRepo)
	if err != nil {
		return err
	}
	convoEngine.Register(accountSetupFlow)

	// subcategorySetupFlow, err := subcategory.NewSetupFlow(subcategoryRepo, subcategoryRepo)
	// if err != nil {
	// 	return err
	// }
	// convoEngine.Register(subcategorySetupFlow)

	tgBot, err := bot.New(conf.Telegram.Token)
	if err != nil {
		return err
	}
	messagingController := messagingctrl.NewController(userRepo, invitationRepo, convoEngine)
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

	if err := database.RunMigrations("../../migrations"); err != nil {
		return nil, err
	}

	return conn, nil
}
