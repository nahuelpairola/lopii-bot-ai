package pendingjob

import (
	"context"
	"errors"
	"testing"
)

func TestClaimReplay_OutsideAReplayIsANoOp(t *testing.T) {
	if err := ClaimReplay(context.Background()); err != nil {
		t.Fatalf("outside a replay the claim must be free, got %v", err)
	}
}

func TestClaimReplay_DeletesOnceEvenWhenCalledTwice(t *testing.T) {
	calls := 0
	ctx := WithClaim(context.Background(), func() (bool, error) { calls++; return true, nil })
	if err := ClaimReplay(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ClaimReplay(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("claim ran %d times, want 1: record and the loop both claim in one turn", calls)
	}
}

func TestClaimReplay_LostClaimIsErrClaimedElsewhere(t *testing.T) {
	ctx := WithClaim(context.Background(), func() (bool, error) { return false, nil })
	if err := ClaimReplay(ctx); !errors.Is(err, ErrClaimedElsewhere) {
		t.Fatalf("want ErrClaimedElsewhere, got %v", err)
	}
}

func TestClaimReplay_DeleteErrorIsReturned(t *testing.T) {
	dbErr := errors.New("connection reset")
	ctx := WithClaim(context.Background(), func() (bool, error) { return false, dbErr })
	err := ClaimReplay(ctx)
	if !errors.Is(err, dbErr) || errors.Is(err, ErrClaimedElsewhere) {
		t.Fatalf("want the delete error, not a lost claim, got %v", err)
	}
}
