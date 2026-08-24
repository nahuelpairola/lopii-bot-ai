// Package server es el composition root: arma todo y lo enciende.
//
// InitServer se lee de arriba a abajo a propósito — el orden ES la información
// cuando el server no levanta. Lo que NO es orden de arranque vive al lado:
// recorder.go (el adapter de telemetría), flows.go (los 15 registros de flujo)
// y bootstrap.go (construir base, bot y orchestrator).
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

// quoteTimeoutSeconds: el fetch más grande es el sembrado de 2.9 MB, una vez
// por deploy. 60 s le sobra y no bloquea nada — corre en la goroutine del sweeper.
const quoteTimeoutSeconds = 60

func InitServer(conf *config.Config) error {
	// First statement: everything after this — including a failed DB connect —
	// is logged through the configured handler.
	logging.Init(conf.Log.Level, conf.Log.Format)

	ginEngine := gin.New()
	// /health/internal is polled continuously by the platform; its access log is
	// pure noise and its latency/status add nothing. Every other route — external
	// health, webhook, invitations, admin — stays logged. SkipPaths suppresses only
	// the log line: the route still serves 200 OK unchanged.
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

	// Repositorios. Uno por tabla, todos sobre la misma conexión.
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

	llmOrchestrator := buildOrchestrator(conf, llmCallRecorder{insert: metricRepo.InsertLLMCall})

	conversationEngine := conversation.NewEngine(conversationRepo, messagingctrl.FlowResumeLabel)
	registerFlows(conversationEngine, subcategoryCache, accountRepo, movementRepo)

	// Controllers. metricRepo entra DOS veces —como metrics y como traces— porque
	// las dos lecturas salen del mismo repo; no es un error de tipeo.
	healthController := healthctrl.NewController(healthChecker)
	messagingController := messagingctrl.NewController(
		userRepo, invitationRepo, accountRepo, movementRepo, subcategoryCache, conversationEngine,
		llmOrchestrator, metricRepo, chatHistoryRepo, reminderRepo, metricRepo, nudgeRepo, jobsRepo, actionsRepo,
	)
	adminController := adminctrl.NewController(userRepo, accountRepo, movementRepo, conversationEngine, tgBot)
	miniappController := miniappctrl.NewController(movementRepo, accountRepo, subcategoryCache, userRepo, invitationRepo, conf.Telegram.Token, conf.Telegram.Username)

	healthController.RegisterRoutes(ginEngine)
	adminController.RegisterRoutes(ginEngine)
	messagingController.RegisterHandlers(tgBot)
	miniappController.RegisterRoutes(ginEngine)

	// tgTransport alcanza a un usuario que no acaba de escribir (el sweeper, el
	// drenaje de 429): resuelve un messenger.Chat a partir de un userID, en vez
	// de una respuesta a un update entrante. Es el mismo adapter que RegisterHandlers
	// usará para el borde del webhook cuando ese wiring migre (Task 6) — por eso
	// vive acá y no adentro de un paquete consumer.
	tgTransport := telegram.New(tgBot, userRepo)

	// Las dos goroutines de fondo: el sweeper (recordatorios, resumen semanal,
	// retención de trazas, cotizaciones) y el drenaje de la cola de 429.
	summaryBuilder := summary.NewBuilder(movementRepo, accountRepo, subcategoryCache)
	quoteRepo := quote.NewRepository(conn)
	quoteClient := quote.NewClient(quote.Config{TimeoutSeconds: quoteTimeoutSeconds})
	sweeper := notifier.NewSweeper(tgTransport, reminderRepo, movementRepo, userRepo, metricRepo, summaryBuilder, quoteRepo, quoteClient)
	go sweeper.Run(context.Background(), time.Duration(conf.Reminders.SweepIntervalMinutes)*time.Minute)
	go pendingjob.Run(context.Background(), messagingController, jobsRepo, tgTransport, pendingjob.JobDrainInterval)

	return ginEngine.Run(":" + conf.Server.Port)
}
