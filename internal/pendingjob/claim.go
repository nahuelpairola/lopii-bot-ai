package pendingjob

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ErrClaimedElsewhere = errors.New("pendingjob: job claimed by another instance")

type replayClaim struct {
	mu      sync.Mutex
	claim   func() (bool, error)
	ran     bool
	claimed bool
	err     error
}

type claimKey struct{}

func newReplayClaim(claim func() (bool, error)) *replayClaim {
	return &replayClaim{claim: claim}
}

func withClaim(ctx context.Context, c *replayClaim) context.Context {
	return context.WithValue(ctx, claimKey{}, c)
}

func WithClaim(ctx context.Context, claim func() (bool, error)) context.Context {
	return withClaim(ctx, newReplayClaim(claim))
}

func ClaimReplay(ctx context.Context) error {
	c, _ := ctx.Value(claimKey{}).(*replayClaim)
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ran {
		c.ran = true
		c.claimed, c.err = c.claim()
	}
	if c.err != nil {
		return fmt.Errorf("pendingjob: claim: %w", c.err)
	}
	if !c.claimed {
		return ErrClaimedElsewhere
	}
	return nil
}

func (c *replayClaim) state() (ran, claimed bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ran, c.claimed, c.err
}
