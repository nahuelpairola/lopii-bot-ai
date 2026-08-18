# internal/nudges

The contextual-tip dispatcher: it decides *which* tip fires, *when*, and renders the tappable
questions. **Use `codegraph_explore` for structure** — this file is only for what reading the code
will not tell you.

## `MarkSent` is once-ever, `MarkSentAgain` is not

`MarkSent` no-ops on conflict — a tip only ever sends once. The one recurring tip (the question
menu) needs its `sent_at` to actually advance so the cooldown re-arms, so it calls
`MarkSentAgain` instead, which upserts. It is used as a method value
(`mark = s.NudgesMarkSentAgain`) — grepping for a direct call will not find the wiring.

## A tip's tap must not be eaten by an open flow

`HandleCallback` runs in `handleConversationInput` **before** `engine.Handle`. A tip's button is not
an option of any flow, so an open flow would otherwise swallow it as if it were one. The lookup is
read-only and leaves the open flow untouched, which is why jumping the queue is safe here.

## The gates are the feature

`Maybe` is called after a successful turn, and almost always does nothing — that restraint is the
point. Each tip declares when it is eligible; a tip that fires too often is worse than one that
never fires, because the user learns to ignore the whole channel.

Copy and thresholds are values someone tuned, not defaults. Changing one is a product decision.
