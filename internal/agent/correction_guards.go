package agent

import (
	"errors"
	"fmt"
	"strings"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/movement"
)

var (
	errAmbiguousSetAll      = errors.New("correction: set de monto sobre varios movimientos")
	errAccountOnTransfer    = errors.New("correction: no se puede cambiar la cuenta de una transferencia")
	errRefundToOtherAccount = errors.New("correction: el reintegro entró en otra cuenta")
)

type guardContext struct {
	NamedAccount string
	Scope        string
	Message      string
}

const (
	scopeOne = "one"
	scopeAll = "all"
)

func applyChanges(rows []movement.MovementRow, changes []correctionChange) ([]movement.MovementRow, error) {
	if len(rows) == 0 {
		return nil, errors.New("correction: no hay filas que corregir")
	}
	isTransfer := isTransferGroup(rows)

	out := make([]movement.MovementRow, len(rows))
	copy(out, rows)

	for _, ch := range changes {
		if isTransfer && ch.Field == fieldAccount {
			return nil, errAccountOnTransfer
		}
		for i := range out {
			applied, err := applyChange(out[i], ch)
			if err != nil {
				return nil, fmt.Errorf("fila %d: %w", i, err)
			}
			out[i] = applied
		}
	}
	return out, nil
}

func isTransferGroup(rows []movement.MovementRow) bool {
	if len(rows) < 2 {
		return false
	}
	for _, r := range rows {
		if r.Type != constants.Transfer {
			return false
		}
	}
	return true
}

func applyChangesToSet(groups [][]movement.MovementRow, changes []correctionChange, ctx guardContext) ([][]movement.MovementRow, error) {
	if len(groups) == 0 {
		return nil, errors.New("correction: no hay grupos que corregir")
	}

	for _, ch := range changes {
		if ch.Field == fieldAmount && ch.Op == opSet && ctx.Scope == scopeAll && len(groups) > 1 {
			return nil, errAmbiguousSetAll
		}
		for _, g := range groups {
			if err := guardRefundDirection(g, []correctionChange{ch}, ctx.Message); err != nil {
				return nil, err
			}
		}
		if err := guardRefundAccount(groups, ch, ctx); err != nil {
			return nil, err
		}
	}

	out := make([][]movement.MovementRow, 0, len(groups))
	for _, g := range groups {
		applied, err := applyChanges(g, changes)
		if err != nil {
			return nil, err
		}
		out = append(out, applied)
	}
	return out, nil
}

func guardRefundAccount(groups [][]movement.MovementRow, ch correctionChange, ctx guardContext) error {
	if ch.Field != fieldAmount || (ch.Op != opSubtract && ch.Op != opMultiply) {
		return nil
	}
	if ctx.NamedAccount == "" {
		return nil
	}
	for _, g := range groups {
		for _, r := range g {
			if r.AccountName != "" && !strings.EqualFold(r.AccountName, ctx.NamedAccount) {
				return fmt.Errorf("%w: %s, no %s", errRefundToOtherAccount, ctx.NamedAccount, r.AccountName)
			}
		}
	}
	return nil
}

var errRefundThatGrows = errors.New("correction: un reintegro no puede aumentar el gasto")

var refundWords = []string{"devolvi", "reintegr", "reembols", "me devolv", "bonific"}

func guardRefundDirection(rows []movement.MovementRow, changes []correctionChange, message string) error {
	if !mentionsRefund(message) {
		return nil
	}
	for _, ch := range changes {
		if ch.Field != fieldAmount {
			continue
		}
		if grows(rows, ch) {
			return fmt.Errorf("%w: %s %s", errRefundThatGrows, ch.Op, ch.Value)
		}
	}
	return nil
}

func mentionsRefund(message string) bool {
	folded := foldAccents(strings.ToLower(message))
	for _, w := range refundWords {
		if strings.Contains(folded, w) {
			return true
		}
	}
	return false
}

func grows(rows []movement.MovementRow, ch correctionChange) bool {
	for _, row := range rows {
		antes, err := movement.ParseARAmount(row.Amount)
		if err != nil {
			continue
		}
		corregida, err := applyChange(row, ch)
		if err != nil {
			continue
		}
		despues, err := movement.ParseARAmount(corregida.Amount)
		if err != nil {
			continue
		}
		if despues.Abs().GreaterThan(antes.Abs()) {
			return true
		}
	}
	return false
}
