package orchestrator

import "time"

// Config is the orchestrator's own config shape — deliberately not the
// same type as internal/config's groq struct, so this package stays a
// pure leaf with no dependency on internal/config. server.go maps one
// to the other at wiring time.
type Config struct {
	APIKey         string
	BaseURL        string
	RouterModel    string
	CreateModel    string
	UpdateModel    string
	DeleteModel    string
	TimeoutSeconds int
}

// Orchestrator wires the Groq client to the 4 call types (router,
// create, update-resolve, delete-resolve), each with its own
// independently configured model.
type Orchestrator struct {
	client      *Client
	routerModel string
	createModel string
	updateModel string
	deleteModel string
}

func New(cfg Config) *Orchestrator {
	return &Orchestrator{
		client:      NewClient(cfg.APIKey, cfg.BaseURL, time.Duration(cfg.TimeoutSeconds)*time.Second),
		routerModel: cfg.RouterModel,
		createModel: cfg.CreateModel,
		updateModel: cfg.UpdateModel,
		deleteModel: cfg.DeleteModel,
	}
}
