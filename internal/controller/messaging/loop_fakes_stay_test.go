package messaging

import (
	"encoding/json"
	"errors"

	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

type fakeActionsRepo struct {
	rows    []*pendingaction.PendingAction
	nextID  uint64
	deleted []uint64
}

func (r *fakeActionsRepo) Insert(a *pendingaction.PendingAction) error {
	r.nextID++
	a.ID = r.nextID
	r.rows = append(r.rows, a)
	return nil
}

func (r *fakeActionsRepo) NextForUser(userID uint64) (*pendingaction.PendingAction, error) {
	var best *pendingaction.PendingAction
	for _, a := range r.rows {
		if a.UserID != userID {
			continue
		}
		if best == nil || a.Position < best.Position || (a.Position == best.Position && a.ID < best.ID) {
			best = a
		}
	}
	if best == nil {
		return nil, pendingaction.ErrNoPendingAction
	}
	return best, nil
}

func (r *fakeActionsRepo) Update(a *pendingaction.PendingAction) error {
	for i, row := range r.rows {
		if row.ID == a.ID {
			r.rows[i] = a
			return nil
		}
	}
	return errors.New("fakeActionsRepo: update: no row with that id")
}

func (r *fakeActionsRepo) Delete(id uint64) error {
	r.deleted = append(r.deleted, id)
	kept := r.rows[:0]
	for _, a := range r.rows {
		if a.ID != id {
			kept = append(kept, a)
		}
	}
	r.rows = kept
	return nil
}

func (r *fakeActionsRepo) CountForUser(userID uint64) (int64, error) {
	var n int64
	for _, a := range r.rows {
		if a.UserID == userID {
			n++
		}
	}
	return n, nil
}

var errRunNotWired = errors.New("Run is not wired in this test")

func swallowTurnDone(execute func(string, json.RawMessage) (string, error)) func(string, json.RawMessage) (string, error) {
	return func(name string, args json.RawMessage) (string, error) {
		result, err := execute(name, args)
		if errors.Is(err, orchestrator.ErrAgentTurnDone) {
			return result, nil
		}
		return result, err
	}
}
