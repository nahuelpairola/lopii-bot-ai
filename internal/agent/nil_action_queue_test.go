package agent

import (
	"testing"

	"lopiibot.com/internal/pendingaction"
)

func TestLoop_NilActionQueue_InsertUpdateDeleteAreNoOps(t *testing.T) {
	l := &Loop{}
	action := &pendingaction.PendingAction{}

	if err := l.insertAction(action); err != nil {
		t.Errorf("insertAction with no queue = %v, want nil", err)
	}
	if err := l.updateAction(action); err != nil {
		t.Errorf("updateAction with no queue = %v, want nil", err)
	}
	if err := l.deleteAction(1); err != nil {
		t.Errorf("deleteAction with no queue = %v, want nil", err)
	}
}
