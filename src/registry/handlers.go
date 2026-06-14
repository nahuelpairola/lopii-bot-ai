package registry

import (
	"lopiibot.com/src/infrastructure/database"
	healthrepo "lopiibot.com/src/infrastructure/database/health_repository"
	handlersInternal "lopiibot.com/src/internal/handlers"
)

type healthHandler interface {
	IsHealthy() error
}

type handlers struct {
	health healthHandler
}

func (r *registry) initHandlers(db *database.Connection) *handlers {
	healthRepo := healthrepo.NewHealthRepository(db)
	healthH := handlersInternal.NewHealthHandler(healthRepo)

	return &handlers{
		health: healthH,
	}
}
