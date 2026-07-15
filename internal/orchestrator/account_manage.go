package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const accountManageSystemPromptTemplate = `Sos un asistente que identifica a cuál de las cuentas del usuario se refiere un mensaje sobre gestión de cuentas (renombrar, ajustar saldo, elegir default, crear).
Tu ÚNICA tarea es el match de cuenta. No interpretes qué operación quiere hacer ni extraigas montos.
- Si el mensaje se refiere claramente a UNA cuenta de la lista (por nombre, alias o descripción inequívoca), devolvé su id en matched_account_id.
- Si el mensaje pide crear una cuenta NUEVA (que no está en la lista), devolvé wants_new_account=true.
- Si no es claro a qué cuenta se refiere (o podría ser más de una), devolvé matched_account_id=null y wants_new_account=false.
Nunca inventes un id que no esté en la lista.

CUENTAS DEL USUARIO (id | nombre (moneda)):
%s`

var accountManageTool = toolSchema{
	Name:        "resolve_account",
	Description: "Identifica la cuenta del usuario a la que se refiere el mensaje, o si pide crear una nueva",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"matched_account_id": {"type": ["integer", "null"]},
			"wants_new_account": {"type": ["boolean", "string", "null"]}
		}
	}`),
}

type accountManageArgs struct {
	MatchedAccountID *uint64  `json:"matched_account_id"`
	WantsNewAccount  flexBool `json:"wants_new_account"`
}

func (o *Orchestrator) ResolveAccountManage(ctx context.Context, text string, accounts []AccountOption) (AccountManageResult, error) {
	systemPrompt := fmt.Sprintf(accountManageSystemPromptTemplate, buildAccountsBlock(accounts))

	raw, err := o.client.chatCompletion(ctx, callTypeAccountManage, o.createModel, systemPrompt, text, accountManageTool)
	if err != nil {
		return AccountManageResult{}, fmt.Errorf("orchestrator: resolve account manage: %w", err)
	}

	var args accountManageArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return AccountManageResult{}, fmt.Errorf("orchestrator: parse account manage result: %w", err)
	}
	return AccountManageResult{MatchedAccountID: args.MatchedAccountID, WantsNewAccount: bool(args.WantsNewAccount)}, nil
}
