# internal/nudges

The contextual-tip dispatcher: it decides *which* tip fires, *when*, and renders the tappable
questions. **Use `codegraph_explore` for structure** — this file is only for what reading the code
will not tell you.

## `nudges` and `nudge` are two packages on purpose

`internal/nudge` is storage only — the `user_nudges` table, once-ever and cooldown bookkeeping.
This package is the dispatcher, and it reaches that storage **through the `Services` interface**,
never by importing it. Keeping them apart is what lets the gates be tested without a DB.

Don't merge them because the names look redundant.

## A tip's tap must not be eaten by an open flow

`HandleCallback` runs in `handleConversationInput` **before** `engine.Handle`. A tip's button is not
an option of any flow, so an open flow would otherwise swallow it as if it were one. The lookup is
read-only and leaves the open flow untouched, which is why jumping the queue is safe here.

## The gates are the feature

`Maybe` is called after a successful turn, and almost always does nothing — that restraint is the
point. Each tip declares when it is eligible; a tip that fires too often is worse than one that
never fires, because the user learns to ignore the whole channel.

Copy and thresholds are values someone tuned, not defaults. Changing one is a product decision.
