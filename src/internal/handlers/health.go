package handlers

type healthRepository interface {
	IsHealthy() error
}

type healthHandler struct {
	repository healthRepository
}

func NewHealthHandler(repo healthRepository) *healthHandler {
	return &healthHandler{
		repository: repo,
	}
}

func (h *healthHandler) IsHealthy() error {
	return h.repository.IsHealthy()
}
