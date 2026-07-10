package server

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/config"
	adminctrl "lopiibot.com/internal/controller/admin"
	healthctrl "lopiibot.com/internal/controller/health"
	invitationctrl "lopiibot.com/internal/controller/invitation"
	messagingctrl "lopiibot.com/internal/controller/messaging"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/health"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/metric"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/notifier"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/queryhistory"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
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

	tgBot, err := inititalizeBot(conf, ginEngine)
	if err != nil {
		return err
	}

	healthChecker := health.NewHealthChecker(conn)
	invitationRepo := invitation.NewRepository(conn)
	userRepo := user.NewRepository(conn)
	accountRepo := account.NewRepository(conn)
	movementRepo := movement.InitRepository(conn)
	reminderRepo := reminder.NewRepository(conn)
	metricRepo := metric.InitRepository(conn)
	queryHistoryRepo := queryhistory.InitRepository(
		conn,
		time.Duration(conf.Query.HistoryTtlMinutes)*time.Minute,
		conf.Query.HistoryLimit,
	)
	subcategoryRepo := subcategory.NewRepository(conn)
	subcategoryCache, err := subcategory.NewCache(subcategoryRepo)
	if err != nil {
		return err
	}
	conversationRepo := conversation.NewRepository(conn)

	llmOrchestrator := orchestrator.New(orchestrator.Config{
		APIKey:         conf.Groq.APIKey,
		BaseURL:        conf.Groq.BaseURL,
		RouterModel:    conf.Groq.RouterModel,
		CreateModel:    conf.Groq.CreateModel,
		UpdateModel:    conf.Groq.UpdateModel,
		DeleteModel:    conf.Groq.DeleteModel,
		QueryModel:     conf.Groq.QueryModel,
		TimeoutSeconds: conf.Groq.TimeoutSeconds,
	})

	conversationEngine := conversation.NewEngine(conversationRepo, messagingctrl.FlowResumeLabel)
	conversationEngine.Register(messagingctrl.NewOnboardingCollectFlow())
	conversationEngine.Register(messagingctrl.NewOnboardingConfirmFlow())
	conversationEngine.Register(messagingctrl.NewMovementCreateFlow(subcategoryCache, accountRepo))
	conversationEngine.Register(messagingctrl.NewMovementConfirmFlow())
	conversationEngine.Register(messagingctrl.NewMovementUpdatePickFlow())
	conversationEngine.Register(messagingctrl.NewMovementUpdateConfirmFlow())
	conversationEngine.Register(messagingctrl.NewMovementDeleteFlow())
	conversationEngine.Register(messagingctrl.NewAccountCreateFlow())
	conversationEngine.Register(messagingctrl.NewSubcategorySetupFlow(subcategoryCache))
	conversationEngine.Register(messagingctrl.NewMovementNegativeConfirmFlow())
	conversationEngine.Register(messagingctrl.NewReminderSetupFlow())

	healthController := healthctrl.NewController(healthChecker)
	invitationController, err := invitationctrl.NewController(invitationRepo, conf.Telegram.Username)
	if err != nil {
		return err
	}
	messagingController := messagingctrl.NewController(
		userRepo, invitationRepo, accountRepo, movementRepo, subcategoryCache, conversationEngine,
		llmOrchestrator, metricRepo, queryHistoryRepo, reminderRepo,
	)
	adminController := adminctrl.NewController(userRepo, accountRepo, movementRepo, conversationEngine, tgBot)

	healthController.RegisterRoutes(ginEngine)
	invitationController.RegisterRoutes(ginEngine)
	adminController.RegisterRoutes(ginEngine)
	messagingController.RegisterHandlers(tgBot)

	sweeper := notifier.NewSweeper(tgBot, reminderRepo, movementRepo, userRepo)
	go sweeper.Run(context.Background(), time.Duration(conf.Reminders.SweepIntervalMinutes)*time.Minute)

	server = httpServer{engine: ginEngine}
	return server.engine.Run(":" + conf.Server.Port)
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

func inititalizeBot(conf *config.Config, engine *gin.Engine) (*bot.Bot, error) {
	tgBot, err := bot.New(conf.Telegram.Token)
	if err != nil {
		return nil, err
	}

	webhookBot := conf.Server.BaseHost + "/webhook/telegram"
	_, err = tgBot.SetWebhook(context.Background(), &bot.SetWebhookParams{
		URL: webhookBot,
	})
	if err != nil {
		return nil, err
	}

	engine.POST("/webhook/telegram", gin.WrapH(tgBot.WebhookHandler()))
	go tgBot.StartWebhook(context.Background())
	return tgBot, nil
}
