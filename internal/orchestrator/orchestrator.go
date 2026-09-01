package orchestrator

import "time"

type Config struct {
	APIKey              string
	BaseURL             string
	CreateModel         string
	UpdateModel         string
	QueryModel          string
	AgentModel          string
	ClassifierModel     string
	AgentFallbackModels []string
	QueryFallbackModels []string
	NarrationModel      string
	TimeoutSeconds      int
	Recorder            LLMRecorder
}

type Orchestrator struct {
	client          *Client
	createModel     string
	updateModel     string
	queryModel      string
	agentModel      string
	classifierModel string
	agentFallbacks  []string
	queryFallbacks  []string
	narrationModel  string
}

func New(cfg Config) *Orchestrator {
	return &Orchestrator{
		client:          NewClient(cfg.APIKey, cfg.BaseURL, time.Duration(cfg.TimeoutSeconds)*time.Second, cfg.Recorder),
		createModel:     cfg.CreateModel,
		updateModel:     cfg.UpdateModel,
		queryModel:      cfg.QueryModel,
		agentModel:      cfg.AgentModel,
		classifierModel: cfg.ClassifierModel,
		agentFallbacks:  cfg.AgentFallbackModels,
		queryFallbacks:  cfg.QueryFallbackModels,
		narrationModel:  cfg.NarrationModel,
	}
}
