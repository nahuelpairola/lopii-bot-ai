package server

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/config"
	adminctrl "lopiibot.com/internal/controller/admin"
	healthctrl "lopiibot.com/internal/controller/health"
	messagingctrl "lopiibot.com/internal/controller/messaging"
	miniappctrl "lopiibot.com/internal/controller/miniapp"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/health"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/logging"
	"lopiibot.com/internal/messenger/telegram"
	"lopiibot.com/internal/metric"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/notifier"
	"lopiibot.com/internal/nudges"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/quote"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/summary"
	"lopiibot.com/internal/user"
)

const quoteTimeoutSeconds = 60

func InitServer(conf *config.Config) error {
	logging.Init(conf.Log.Level, conf.Log.Format)

	gin.SetMode(conf.Server.GinMode)
	ginEngine := gin.New()
	ginEngine.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipPaths: []string{"/health/internal", "/health/external"}}))
	ginEngine.Use(gin.Recovery())

	conn, err := initializeDatabase(conf)
	if err != nil {
		return err
	}

	tgBot, err := initializeBot(conf, ginEngine)
	if err != nil {
		return err
	}

	healthChecker := health.NewHealthChecker(conn)
	invitationRepo := invitation.NewRepository(conn)
	userRepo := user.NewRepository(conn)
	accountRepo := account.NewRepository(conn)
	movementRepo := movement.InitRepository(conn)
	reminderRepo := reminder.NewRepository(conn)
	nudgeRepo := nudges.NewRepository(conn)
	jobsRepo := pendingjob.NewRepository(conn)
	metricRepo := metric.InitRepository(conn)
	chatHistoryRepo := chathistory.InitRepository(
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
	actionsRepo := pendingaction.NewRepository(conn)

	tgTransport := telegram.New(tgBot, userRepo, conf.Server.BaseHost)

	llmOrchestrator := buildOrchestrator(conf, llmCallRecorder{insert: metricRepo.InsertLLMCall})

	conversationEngine := conversation.NewEngine(conversationRepo, messagingctrl.FlowResumeLabel)
	registerFlows(conversationEngine, subcategoryCache, accountRepo, movementRepo)

	healthController := healthctrl.NewController(healthChecker)
	messagingController := messagingctrl.NewController(
		userRepo, invitationRepo, accountRepo, movementRepo, subcategoryCache, conversationEngine,
		llmOrchestrator, metricRepo, chatHistoryRepo, reminderRepo, metricRepo, nudgeRepo, jobsRepo, actionsRepo,
	)
	adminController := adminctrl.NewController(userRepo, accountRepo, movementRepo, conversationEngine, tgTransport)
	miniappController := miniappctrl.NewController(movementRepo, accountRepo, subcategoryCache, userRepo, conf.Telegram.Token, conf.Telegram.Username)

	healthController.RegisterRoutes(ginEngine)
	adminController.RegisterRoutes(ginEngine)
	tgTransport.RegisterCommand("/start", messagingController.HandleStart)
	ginEngine.POST("/webhook/telegram", gin.WrapH(tgTransport.Serve(messagingController.Handle)))
	miniappController.RegisterRoutes(ginEngine)

	quoteRepo := quote.NewRepository(conn)
	summaryBuilder := summary.NewBuilder(movementRepo, accountRepo, subcategoryCache, quoteRepo)
	quoteClient := quote.NewClient(quote.Config{TimeoutSeconds: quoteTimeoutSeconds})
	sweeper := notifier.NewSweeper(tgTransport, reminderRepo, movementRepo, userRepo, metricRepo, summaryBuilder, quoteRepo, quoteClient)
	go sweeper.Run(context.Background(), time.Duration(conf.Reminders.SweepIntervalMinutes)*time.Minute)
	go pendingjob.Run(context.Background(), messagingController, jobsRepo, tgTransport, pendingjob.JobDrainInterval)

	return ginEngine.Run(":" + conf.Server.Port)
}
