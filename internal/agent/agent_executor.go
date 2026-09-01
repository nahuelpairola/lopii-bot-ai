package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messages"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

const (
	resultNoCandidates = "no encontré ningún movimiento que coincida con eso"
	resultParked       = "listo, la app sigue con eso y le pide confirmación al usuario"
	resultNotWiredYet  = "esa herramienta todavía no está disponible"

	questionKeyCandidate = "candidate"

	questionKeyChange = "change"
)

type parkedAction struct {
	Tool      string
	Payload   agentPayload
	Questions []pendingaction.OpenQuestion
}

type agentPayload struct {
	Change            string                `json:"change,omitempty"`
	Candidates        []flow.CandidateGroup `json:"candidates,omitempty"`
	Chosen            int                   `json:"chosen"`
	Seed              map[string]any        `json:"seed,omitempty"`
	GaveChangeValue   bool                  `json:"gave_change_value,omitempty"`
	PickedChangeField bool                  `json:"picked_change_field,omitempty"`
	PickedField       string                `json:"picked_field,omitempty"`
	ChangeAnswer      string                `json:"change_answer,omitempty"`
	Scope             string                `json:"scope,omitempty"`
	Changes           []correctionChange    `json:"changes,omitempty"`
	SearchText        string                `json:"search_text,omitempty"`
	DateFrom          string                `json:"date_from,omitempty"`
	DateTo            string                `json:"date_to,omitempty"`
}

type agentExecutor struct {
	ctx      context.Context
	svc      agentServices
	userID   uint64
	userText string

	taxonomy []orchestrator.TaxonomyEntry

	parked       []parkedAction
	reply        string
	answerQuery  bool
	settingsArea string
	replyButtons []conversation.Button
	noCandidates bool
	wrote        bool
	inserted     []movement.Movement
}

func newAgentExecutor(ctx context.Context, svc agentServices, userID uint64, userText string, taxonomy []orchestrator.TaxonomyEntry) *agentExecutor {
	return &agentExecutor{ctx: ctx, svc: svc, userID: userID, userText: userText, taxonomy: taxonomy}
}

func wiredAgentTools() []orchestrator.AgentTool {
	wired := map[string]bool{
		orchestrator.ToolRecordMovements: true,
		orchestrator.ToolCorrectMovement: true,
		orchestrator.ToolDeleteMovements: true,
		orchestrator.ToolAnswerQuery:     true,
		orchestrator.ToolManageSettings:  true,
		orchestrator.ToolReplyHelp:       true,
		orchestrator.ToolAskRewrite:      true,
	}
	all := orchestrator.AgentTools()
	out := make([]orchestrator.AgentTool, 0, len(wired))
	for _, t := range all {
		if wired[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func (e *agentExecutor) execute(name string, args json.RawMessage) (string, error) {
	switch name {
	case orchestrator.ToolCorrectMovement:
		var a struct {
			Change   string             `json:"change"`
			Scope    string             `json:"scope"`
			Changes  []correctionChange `json:"changes"`
			DateFrom string             `json:"date_from"`
			DateTo   string             `json:"date_to"`
		}
		_ = json.Unmarshal(args, &a)
		defaultChangeOps(a.Changes)
		return e.park(parkRequest{
			tool: orchestrator.ToolCorrectMovement, change: a.Change,
			question: flow.MsgPickUpdateCandidate(nil), scope: a.Scope, changes: a.Changes,
			dateFrom: a.DateFrom, dateTo: a.DateTo,
		})
	case orchestrator.ToolRecordMovements:
		return e.record(args)
	case orchestrator.ToolDeleteMovements:
		return e.park(parkRequest{tool: orchestrator.ToolDeleteMovements, question: flow.MsgPickDeleteCandidate(nil)})
	case orchestrator.ToolAnswerQuery:
		e.answerQuery = true
		return "ya le contestaste la consulta al usuario", orchestrator.ErrAgentTurnDone
	case orchestrator.ToolManageSettings:
		var a struct {
			Area string `json:"area"`
		}
		_ = json.Unmarshal(args, &a)
		e.settingsArea = a.Area
		return "ya abriste la configuración que pidió el usuario", orchestrator.ErrAgentTurnDone
	case orchestrator.ToolReplyHelp:
		e.reply = messages.MsgHelp
		return "ya le mandaste al usuario la explicación de qué podés hacer", orchestrator.ErrAgentTurnDone
	case orchestrator.ToolAskRewrite:
		e.reply = messages.MsgAskRewrite
		return "ya le pediste al usuario que lo reescriba", orchestrator.ErrAgentTurnDone
	default:
		return resultNotWiredYet, nil
	}
}

func resultRecorded(n int) string {
	return fmt.Sprintf("registrados: %d movimientos", n)
}

func (e *agentExecutor) record(args json.RawMessage) (string, error) {
	var result orchestrator.CreateResult
	if err := json.Unmarshal(args, &result); err != nil {
		return "no pude leer los movimientos, pedile al usuario que lo reescriba", nil
	}
	if len(result.Movements) == 0 {
		return "no venía ningún movimiento", nil
	}
	e.classify(result.Movements)

	result.Normalize()

	accounts, _ := e.svc.FindUserAccounts(e.userID)
	seed := buildCreateSeed(result, e.taxonomy, accounts, e.userText)
	seed[conversation.UserIDKey] = e.userID

	hasGaps := len(conversation.DecodeStringSlice(seed, conversation.KeyPendingCategoryGaps)) > 0 ||
		len(conversation.DecodeStringSlice(seed, conversation.KeyPendingAccountGaps)) > 0
	hasFirst := flow.NeedsFirstAccount(seed, func(cur currency.Currency) bool {
		return e.svc.AccountsHasDefaultForCurrency(e.userID, cur)
	})
	if hasGaps || hasFirst {
		return e.parkCreate(seed)
	}

	inserted, err := e.svc.ResolveAndInsertMovements(seed)
	if err != nil {
		var short *flow.InsufficientFunds
		if errors.As(err, &short) {
			return e.parkFundsGate(seed, short)
		}
		return "", fmt.Errorf("record_movements: %w", err)
	}
	e.wrote = true
	e.inserted = inserted
	e.reply = flow.MsgConfirmMovements(inserted)
	e.replyButtons = e.svc.MaybeNearDuplicate(e.userID, inserted)
	return resultRecorded(len(inserted)), orchestrator.ErrAgentTurnDone
}

func (e *agentExecutor) classify(movements []orchestrator.MovementDraft) {
	if len(movements) == 0 {
		return
	}

	structCat, structSub, hasStructural := orchestrator.StructuralPair(movements)

	rows := make([]orchestrator.ClassifyRow, 0, len(movements))
	for _, m := range movements {
		rows = append(rows, orchestrator.ClassifyRow{
			Description: m.Description,
			Type:        m.Type,
			AccountName: m.AccountNameGuess,
		})
	}
	pairs := e.svc.ClassifyCategories(e.ctx, e.userText, rows, e.taxonomy)
	for i := range movements {
		cat, sub := "", ""
		if i < len(pairs) {
			cat, sub = pairs[i].Category, pairs[i].Subcategory
		}
		if hasStructural && (cat == "" || cat == constants.PendingReview) {
			cat, sub = structCat, structSub
		}
		movements[i].Category, movements[i].Subcategory = cat, sub
	}
}

type parkRequest struct {
	tool     string
	change   string
	question string
	scope    string
	changes  []correctionChange
	dateFrom string
	dateTo   string
}

func (e *agentExecutor) park(req parkRequest) (string, error) {
	tool, change, question := req.tool, req.change, req.question
	groups, err := resolveCandidates(e.svc, e.userID, e.userText, req.dateFrom, req.dateTo)
	if err != nil {
		return "", fmt.Errorf("%s: resolve candidates: %w", tool, err)
	}
	if len(groups) == 0 {
		e.noCandidates = true
		e.reply = messages.MsgNoCandidatesFound
		return resultNoCandidates, orchestrator.ErrAgentTurnDone
	}

	candidates := make([]flow.CandidateGroup, 0, len(groups))
	options := make([]string, 0, len(groups))
	for _, g := range groups {
		candidates = append(candidates, toCandidateGroup(g))
		options = append(options, candidateLabel(g))
	}

	action := parkedAction{Tool: tool, Payload: agentPayload{
		Change: change, Candidates: candidates, Chosen: -1,
		Scope: req.scope, Changes: req.changes,
		SearchText: e.userText, DateFrom: req.dateFrom, DateTo: req.dateTo,
	}}
	switch {
	case len(candidates) == 1:
		action.Payload.Chosen = 0
	case isBatchCorrection(action.Payload):
	default:
		if !matchesMessage(groups[0], e.userText) {
			question = flow.MsgPickRecentFallback
		}
		question += " " + flow.MsgCanRetypeToSearch
		action.Questions = []pendingaction.OpenQuestion{{
			Key: questionKeyCandidate, Prompt: question, Options: options,
		}}
	}
	e.parked = append(e.parked, action)
	return resultParked, orchestrator.ErrAgentTurnDone
}

func isBatchCorrection(p agentPayload) bool {
	return p.Scope == scopeAll && len(p.Changes) > 0 && len(p.Candidates) > 1
}

func (e *agentExecutor) parkCreate(seed conversation.Data) (string, error) {
	e.parked = append(e.parked, parkedAction{
		Tool:    orchestrator.ToolRecordMovements,
		Payload: agentPayload{Seed: seed, Chosen: 0},
	})
	return "pendiente: faltan datos, la app se los pide al usuario", orchestrator.ErrAgentTurnDone
}

func (e *agentExecutor) parkFundsGate(seed conversation.Data, short *flow.InsufficientFunds) (string, error) {
	gateSeed := conversation.CopyData(seed)
	gateSeed[conversation.KeyGatePrompt] = flow.MsgInsufficientFunds(short.Shortfalls)
	e.parked = append(e.parked, parkedAction{
		Tool:    orchestrator.ToolRecordMovements,
		Payload: agentPayload{Seed: gateSeed, Chosen: 0},
	})
	return "pendiente: el saldo no alcanza, la app le pide confirmación al usuario", orchestrator.ErrAgentTurnDone
}

func toCandidateGroup(g transactionGroup) flow.CandidateGroup {
	rows := make([]movement.MovementRow, 0, len(g.Movements))
	ids := make([]string, 0, len(g.Movements))
	for _, m := range g.Movements {
		rows = append(rows, movementToRow(m))
		ids = append(ids, strconv.FormatUint(uint64(m.ID), 10))
	}
	return flow.CandidateGroup{TransactionID: g.TransactionID, OldIDs: ids, Rows: rows}
}
