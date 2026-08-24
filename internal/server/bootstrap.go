package server

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/config"
	miniappctrl "lopiibot.com/internal/controller/miniapp"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/orchestrator"
)

// Construir una pieza, no ordenar el arranque: el orden vive en InitServer
// (server.go) y se lee de arriba a abajo, que es lo único que importa cuando el
// server no levanta.

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

func initializeBot(conf *config.Config, engine *gin.Engine) (*bot.Bot, error) {
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

// buildOrchestrator mapea la config a orchestrator.Config. El mapeo es a mano y
// campo por campo a propósito: orchestrator es una hoja sin dependencia de
// internal/config, así que este archivo es el único lugar donde los dos nombres
// se tocan.
func buildOrchestrator(conf *config.Config, recorder orchestrator.LLMRecorder) *orchestrator.Orchestrator {
	orch := orchestrator.New(orchestrator.Config{
		APIKey:              conf.Groq.APIKey,
		BaseURL:             conf.Groq.BaseURL,
		CreateModel:         conf.Groq.CreateModel,
		UpdateModel:         conf.Groq.UpdateModel,
		QueryModel:          conf.Groq.QueryModel,
		AgentModel:          conf.Groq.AgentModel,
		AgentFallbackModels: conf.Groq.AgentFallbackModels,
		QueryFallbackModels: conf.Groq.QueryFallbackModels,
		NarrationModel:      conf.Groq.NarrationModel,
		ClassifierModel:     conf.Groq.ClassifierModel,
		TimeoutSeconds:      conf.Groq.TimeoutSeconds,
		Recorder:            recorder,
	})

	// Avisa, no aborta: un operador puede tener una razón que la tabla no contempla, y
	// tumbar el server por eso es peor que el problema que evita. El corte de verdad es
	// el test sobre config/*.toml, que corre antes del deploy.
	for _, conflicto := range config.ModelBucketConflicts(conf.Groq) {
		slog.Warn("config: llamadas del mismo turno comparten modelo", "detalle", conflicto)
	}

	return orch
}
