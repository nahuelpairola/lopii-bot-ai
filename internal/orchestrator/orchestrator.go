package orchestrator

import "time"

// Config is the orchestrator's own config shape — deliberately not the
// same type as internal/config's groq struct, so this package stays a
// pure leaf with no dependency on internal/config. server.go maps one
// to the other at wiring time.
type Config struct {
	APIKey      string
	BaseURL     string
	CreateModel string
	UpdateModel string
	QueryModel  string
	// AgentModel is the model behind Run — the unified agent loop, and since
	// stage 5 the only path a free-text message takes. CreateModel and
	// UpdateModel outlived the router: they still serve the wizard-side calls
	// (onboarding, category create, account manage), not a second router.
	AgentModel string
	// ClassifierModel es el modelo de la clasificación de categorías, que sale
	// del loop en la etapa 5. Va aparte a propósito: el techo de TPM de Groq es
	// POR MODELO, así que una llamada en otro modelo no le come nada al loop.
	// Cuál conviene lo decide el eval, no esta config.
	ClassifierModel string
	// AgentFallbackModels son los modelos a los que el loop se corre cuando el
	// principal rebota por CUPO, en orden. Los techos de Groq son POR MODELO —
	// verificado leyendo los headers: 8.000 TPM para gpt-oss-20b y otros 8.000
	// para gpt-oss-120b, con TPD independiente cada uno. Un 429 en uno no dice
	// nada del otro.
	//
	// Vacío = comportamiento de antes: el 429 encola y el usuario espera.
	AgentFallbackModels []string
	TimeoutSeconds      int
	Recorder            LLMRecorder // nil-safe
}

// Orchestrator wires the Groq client to the call types that remain after stage
// 5 deleted the router: the agent loop (Run), the read-only query loop, the
// category classifier, and the wizard-side create/update calls — each with its
// own model, because Groq's rate limits are per model.
type Orchestrator struct {
	client          *Client
	createModel     string
	updateModel     string
	queryModel      string
	agentModel      string
	classifierModel string
	agentFallbacks  []string
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
	}
}
