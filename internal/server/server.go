package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/config"
	adminctrl "lopiibot.com/internal/controller/admin"
	healthctrl "lopiibot.com/internal/controller/health"
	invitationctrl "lopiibot.com/internal/controller/invitation"
	messagingctrl "lopiibot.com/internal/controller/messaging"
	miniappctrl "lopiibot.com/internal/controller/miniapp"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/health"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/logging"
	"lopiibot.com/internal/metric"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/notifier"
	"lopiibot.com/internal/nudge"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/queryhistory"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/summary"
	"lopiibot.com/internal/user"
)

type httpServer struct {
	engine *gin.Engine
}

var server httpServer

// llmCallRecorder adapta orchestrator.LLMRecorder a metric. Fire-and-forget en
// goroutine: la métrica no debe agregar latencia ni romper el flujo del usuario.
type llmCallRecorder struct {
	insert func(*metric.LLMCall) error
}

func (r llmCallRecorder) Record(c orchestrator.LLMCall) {
	go func() {
		if err := r.insert(&metric.LLMCall{
			TraceID:                    c.TraceID,
			CallType:                   c.CallType,
			Model:                      c.Model,
			PromptTokens:               c.PromptTokens,
			CompletionTokens:           c.CompletionTokens,
			TotalTokens:                c.TotalTokens,
			LatencyMs:                  c.LatencyMs,
			HTTPStatus:                 c.HTTPStatus,
			Attempts:                   c.Attempts,
			Error:                      c.Err,
			RateLimitRemainingRequests: c.RateLimitRemainingRequests,
			RateLimitRemainingTokens:   c.RateLimitRemainingTokens,
		}); err != nil {
			slog.Error("llm_call insert failed", "err", err)
		}
	}()
}

func InitServer(conf *config.Config) error {
	// First statement: everything after this — including a failed DB connect —
	// is logged through the configured handler.
	logging.Init(conf.Log.Level, conf.Log.Format)

	ginEngine := gin.New()
	// /health/internal is polled continuously by the platform; its access log is
	// pure noise and its latency/status add nothing. Every other route — external
	// health, webhook, invitations, admin — stays logged. SkipPaths suppresses only
	// the log line: the route still serves 200 OK unchanged.
	ginEngine.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipPaths: []string{"/health/internal"}}))
	ginEngine.Use(gin.Recovery())

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
	nudgeRepo := nudge.NewRepository(conn)
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
		Recorder:       llmCallRecorder{insert: metricRepo.InsertLLMCall},
	})

	conversationEngine := conversation.NewEngine(conversationRepo, messagingctrl.FlowResumeLabel)
	conversationEngine.Register(messagingctrl.NewMovementCreateFlow(subcategoryCache, accountRepo))
	conversationEngine.Register(messagingctrl.NewMovementConfirmFlow())
	conversationEngine.Register(messagingctrl.NewMovementUpdatePickFlow())
	conversationEngine.Register(messagingctrl.NewMovementUpdateConfirmFlow())
	conversationEngine.Register(messagingctrl.NewMovementDeleteFlow())
	conversationEngine.Register(messagingctrl.NewAccountCreateFlow())
	conversationEngine.Register(messagingctrl.NewAccountManageFlow(movementRepo))
	conversationEngine.Register(messagingctrl.NewAccountMoveOfferFlow())
	conversationEngine.Register(messagingctrl.NewSubcategorySetupFlow(subcategoryCache))
	conversationEngine.Register(messagingctrl.NewCategoryMatchOfferFlow())
	conversationEngine.Register(messagingctrl.NewCategoryProposalConfirmFlow())
	conversationEngine.Register(messagingctrl.NewCategoryManagePickFlow(subcategoryCache))
	conversationEngine.Register(messagingctrl.NewCategoryManageTargetFlow(subcategoryCache))
	conversationEngine.Register(messagingctrl.NewMovementNegativeConfirmFlow())
	conversationEngine.Register(messagingctrl.NewReminderSetupFlow())

	healthController := healthctrl.NewController(healthChecker)
	invitationController, err := invitationctrl.NewController(invitationRepo, conf.Telegram.Username)
	if err != nil {
		return err
	}
	messagingController := messagingctrl.NewController(
		userRepo, invitationRepo, accountRepo, movementRepo, subcategoryCache, conversationEngine,
		llmOrchestrator, metricRepo, queryHistoryRepo, reminderRepo, metricRepo, nudgeRepo,
	)
	adminController := adminctrl.NewController(userRepo, accountRepo, movementRepo, conversationEngine, tgBot)
	miniappController := miniappctrl.NewController(movementRepo, accountRepo, userRepo, conf.Telegram.Token)

	healthController.RegisterRoutes(ginEngine)
	invitationController.RegisterRoutes(ginEngine)
	adminController.RegisterRoutes(ginEngine)
	messagingController.RegisterHandlers(tgBot)
	miniappController.RegisterRoutes(ginEngine)

	summaryBuilder := summary.NewBuilder(movementRepo, accountRepo)
	sweeper := notifier.NewSweeper(tgBot, reminderRepo, movementRepo, userRepo, metricRepo, summaryBuilder)
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
	}, conf.Log.Level == "debug")
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

	// The Mini App menu button is cosmetic — register it best-effort, OFF the
	// boot critical path. A slow or failing Telegram call here must never
	// delay or abort the webhook loop (the bot's core function).
	go func() {
		if _, err := tgBot.SetChatMenuButton(context.Background(), &bot.SetChatMenuButtonParams{
			MenuButton: &models.MenuButtonWebApp{
				Type:   models.MenuButtonTypeWebApp,
				Text:   miniappctrl.MenuButtonText,
				WebApp: models.WebAppInfo{URL: conf.Server.BaseHost + miniappctrl.EntryPath},
			},
		}); err != nil {
			slog.Error("miniapp: SetChatMenuButton failed", "err", err)
		}
	}()

	go tgBot.StartWebhook(context.Background())
	return tgBot, nil
}
