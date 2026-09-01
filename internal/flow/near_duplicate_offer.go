package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
)

const (
	NearDupPrefix  = "nd:"
	NearDupSeparte = "sep"
	NearDupMerge   = "mrg"
	NearDupReplace = "rpl"

	nearDupRecentLimit = 20
)

func nearDuplicateButtons(insertedID, priorID uint) []conversation.Button {
	id := func(action string) string {
		return fmt.Sprintf("%s%s:%d:%d", NearDupPrefix, action, insertedID, priorID)
	}
	return []conversation.Button{
		{Label: "Va aparte", Data: id(NearDupSeparte)},
		{Label: "Sumalo a ese", Data: id(NearDupMerge)},
		{Label: "Reemplazalo", Data: id(NearDupReplace)},
	}
}

func MaybeNearDuplicate(r runner, userID uint64, inserted []movement.Movement) []conversation.Button {
	if len(inserted) == 0 {
		return nil
	}
	sameTurn := make([]uint, 0, len(inserted))
	for _, m := range inserted {
		sameTurn = append(sameTurn, m.ID)
	}

	priors, err := r.FindRecentlyCreatedForUser(
		userID, NearDuplicateWindowStart(time.Now()), nearDupRecentLimit)
	if err != nil {
		return nil
	}

	for _, m := range inserted {
		if prior := FindNearDuplicate(m, sameTurn, priors); prior != nil {
			slog.Info("near duplicate flagged", "user_id", userID, "inserted", m.ID, "prior", prior.ID)
			return nearDuplicateButtons(m.ID, prior.ID)
		}
	}
	return nil
}

func HandleNearDuplicateChoice(ctx context.Context, r runner, chat messenger.Chat, userID uint64, data string) bool {
	if !strings.HasPrefix(data, NearDupPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(data, NearDupPrefix), ":")
	if len(parts) != 3 {
		return true
	}
	action := parts[0]
	insertedID, err1 := strconv.ParseUint(parts[1], 10, 64)
	priorID, err2 := strconv.ParseUint(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return true
	}

	if action == NearDupSeparte {
		r.SendText(ctx, chat, MsgNearDupSeparate)
		return true
	}

	if err := ApplyNearDuplicateChoice(r, userID, action, uint(insertedID), uint(priorID)); err != nil {
		slog.ErrorContext(ctx, "near duplicate choice failed",
			"user_id", userID, "action", action, "inserted", insertedID, "prior", priorID, "err", err)
		r.SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return true
	}
	r.SendText(ctx, chat, MsgNearDupMerged)
	return true
}

func ApplyNearDuplicateChoice(r runner, userID uint64, action string, insertedID, priorID uint) error {
	recent, err := r.FindRecentlyCreatedForUser(userID, NearDuplicateWindowStart(time.Now()), nearDupRecentLimit)
	if err != nil {
		return fmt.Errorf("near duplicate: find recent: %w", err)
	}
	var inserted, prior *movement.Movement
	for i := range recent {
		switch recent[i].ID {
		case insertedID:
			inserted = &recent[i]
		case priorID:
			prior = &recent[i]
		}
	}
	if inserted == nil || prior == nil {
		return fmt.Errorf("near duplicate: ya no están las dos filas (inserted=%v prior=%v)", inserted != nil, prior != nil)
	}

	merged := *prior
	switch action {
	case NearDupMerge:
		merged.Amount = prior.Amount.Add(inserted.Amount)
	case NearDupReplace:
		merged.Amount = inserted.Amount
	default:
		return fmt.Errorf("near duplicate: acción desconocida %q", action)
	}
	merged.ID = 0

	if err := r.ReplaceMovements([]uint{prior.ID}, []movement.Movement{merged}); err != nil {
		return fmt.Errorf("near duplicate: replace: %w", err)
	}
	if err := r.SoftDeleteByIDs([]uint{inserted.ID}); err != nil {
		return fmt.Errorf("near duplicate: delete inserted: %w", err)
	}
	return nil
}
