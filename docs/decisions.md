# Key Design Decisions - lopii-finance-bot

> Why the code is shaped the way it is: the measurements, the incidents and the rejected
> alternatives behind each choice. **This is the long form.** A code comment states the
> conclusion and points here; a package `AGENTS.md` states the trap. The rationale itself lives
> here, once.
>
> For what exists, ask `codegraph_explore` - the feature inventory that used to live here was
> deleted because nobody re-derived it and it ended up claiming shipped features were unstarted.
> Anti-patterns are in [ARCHITECTURE.md](ARCHITECTURE.md#anti-patterns--what-not-to-do).

## Contents

- [Interaction principles](#interaction-principles)
- [The money model](#the-money-model)
- [Movements: mutation and reference resolution](#movements-mutation-and-reference-resolution)
- [Conversation engine and flows](#conversation-engine-and-flows)
- [Taxonomy](#taxonomy)
- [The agent loop and QUERY](#the-agent-loop-and-query)
- [Groq quota, the 429 queue and rate limits](#groq-quota-the-429-queue-and-rate-limits)
- [Notifications and reminders](#notifications-and-reminders)
- [Package layout, metrics and tooling](#package-layout-metrics-and-tooling)

## Interaction principles

- **No Telegram commands for end users.** All interaction is free text → LLM orchestrator → intent routing. The only exceptions are `/start` (onboarding deep-link, all users) and admin commands like `/new-invite`.
- **CREATE is frictionless; UPDATE/DELETE always confirm.** A confident CREATE classification inserts immediately with no `conversation.Engine` involvement — recording new movements should have zero friction. UPDATE and DELETE, by contrast, always stop for explicit user confirmation before mutating, because they touch data the user already trusts as recorded truth. The one exception: a CREATE the router flags as ambiguous, or one `resolveCandidates` finds a plausible existing match for, stops at the `movement_confirm_intent` gate (reescribir/cancelar) before Call 2 CREATE ever runs. A genuine CREATE gap (PENDING_REVIEW category, or a transfer naming a non-existent account) drops into a guided `ChoiceStep` flow.
- **The bot never guesses the user's real intent — only the user resolves ambiguity.** When a CREATE is ambiguous, the confirm gate offers exactly "reescribir" or "cancelar", never a reroute to UPDATE/DELETE. Even if the message probably meant a correction, the bot doesn't act on that guess; the user is expected to send a clearer message. Same principle behind the gap-fill flow's Cancelar option — an escape hatch, not a smarter flow.

## The money model

- **A USD purchase/sale is two attributed transfers, not an expense.** Buying USD debits a real ARS account and credits a real USD account (both `transfer`, `Inversiones | Dólares`, one `transaction_id`) — so ARS balances stay correct and the operation is excluded from cash-flow summaries (`WHERE type != 'transfer'`) instead of inflating spending. Same-currency inter-account moves are `Sistema | Transferencia`. No implicit FX conversion of a balance is introduced — the two legs are simply in different currencies at the stated rate.
- **FCI redemption gain is computed by app code, never the LLM.** The rule is `redeemed_amount >= balance_before` (via `movement.SumAmountForAccount` on the destination account) combined with the account's `IsDefault` flag to distinguish a full/partial redemption from a subscription — both look like an identical negative-transfer leg under the same subcategory, so `IsDefault` is the actual discriminator the LLM cannot see.
- **ARS/USD strictly separated.** No implicit FX conversion anywhere. Totals and summaries are reported per currency.
- **Balance is always computed.** No `balance` column on `accounts`. Always `SUM(amount)` from `movements`.
- **Anomaly detection — the insufficient-funds confirm gate.** Correct numbers is only half the goal; the other half is *surfacing* a suspicious computation instead of burying it. A well-formed movement that drives an account into or deeper into negative (`after < 0 && after < before`, per affected account, `after = SumAmountForAccount + Σ deltas`) stops at a confirm gate — **Registrar igual / Reescribir / Falta registrar algo** — showing the shortfall; the user decides. Distinct from the guard's *malformed*-row rejections (amount 0, currency mismatch): those are rewritten, this is confirmed. Reuses the `ChoiceStep` confirm pattern (`movement_confirm_flow.go`); friction only on the anomaly path, so frictionless CREATE is intact. Deterministic (balances-map input → pure check), Layer-1 testable. Same spec as below.
- **Signed, account-attributed movements — the money model (money precision).** Every movement (expense/income/transfer) is attributed to a real account and stores a **signed** amount so `balance = SUM(amount)` is the whole accounting: `expense` negative on its source, `income` positive on its destination, transfer legs signed out/in. The sign is **internal to storage** and owned by the app (a guard normalizes `expense`→negative, `income`→positive on write), never by the LLM; it never escapes storage — user *and* LLM (as an UPDATE/DELETE candidate) both see `amount.Abs()`, with direction carried by the movement type. Account resolution is app-side and deterministic (LLM-matched → default of currency → gap-fill), because the account list shown to the LLM doesn't mark the default. This supersedes the earlier "expense/income are `account_id = NULL`, positive" rule, which the onboarding real-accounts redesign left incoherent (spending never debited the real balance) and which let malformed rows through (a negative `expense`, and a `0.00` from feeding a signed candidate into a positive-reasoning UPDATE prompt). Full rules in [business-rules.md](business-rules.md#the-accounting-model); spec + guard in `docs/superpowers/specs/2026-07-07-signed-account-attributed-movements-design.md`. **The single place where a wrong sign or `account_id` is a financial bug, not a cosmetic one.**
- **Accounts hold fixed monetary amounts, never asset positions.** An account's balance is a single ARS or USD number — the current value. The bot does not model stocks/ETFs/cedears/FCI cuotapartes as units × price, does not auto-revalue, and does not accrue interest. Gains (rendimiento) belong to the account that earned them — recorded as `income` (`Sistema | Rendimiento inversión`) attributed to that account, never silent balance bumps; any account can have its own (explicit when stated, app-computed on an FCI redemption). `ClassifyOnboarding` extracts monetary balances only — never units/shares/tickers.

## Movements: mutation and reference resolution

- **Movement mutation operates on movement IDs, not `transaction_id`.** `movement.SoftDeleteByIDs`/`ReplaceMovements` take a list of primary-key IDs. This means a standalone/ungrouped movement (`transaction_id IS NULL`) is corrected or deleted through the exact same code path as a multi-row compound transaction — no special-casing for "is this movement part of a group."
- **One reference-resolution mechanism, `resolveCandidates`, used everywhere.** UPDATE, DELETE, and CREATE's duplicate-check all go through it, anchored on a mentioned date or on recency of *entry* (`created_at`) otherwise. Textual relevance is **scored** in Go (`scoreGroup`: the fraction of a candidate's own description tokens the message names, accent-folded, or a literal amount match, plus a capped date-proximity tie-break) and the candidates are ranked by that score before being cut to five — the DB layer no longer pre-filters by `pg_trgm` similarity, it just supplies the window. 0 candidates errors out (or, for CREATE, just proceeds — no duplicate found), 1 proceeds to confirm, 2+ shows a picker. There used to be a second mechanism (`LastTransactionStore`, an in-memory per-user "last transaction" checked before the DB search) — removed because two overlapping "what does this refer to" mechanisms was a source of silent wrong matches, not a performance win worth keeping.
- **`Movement.Subcategory` is the one exception to "bare FK, manual lookup."** Every other FK in the codebase (`Account.UserID`, `Invitation.CreatedBy/UsedBy`, `Subcategory.UserID`, `Movement.UserID`/`AccountID`) is a bare `uint64`/`*uint64` with manual repository lookups, even though a real Postgres FK backs every one. `Movement.Subcategory *subcategory.Subcategory` breaks that pattern deliberately: the FK already existed (no migration needed, Go-level-only change), and `Movement` is the one entity with a growing reporting/analytics surface (monthly summaries today, `QUERY` intent tomorrow) where list-shaped queries with names attached recur — every future method benefits from `.Preload("Subcategory")` instead of re-implementing `buildSubcategoryIndex`-style plumbing. The other four models are single-row lookups by ID with no comparable multiplying need.
- **UPDATE = atomic DELETE + INSERT.** Editing a movement means soft-deleting the old one(s) and inserting the new one(s) in a single transaction. Never partial patch.
- **Candidate search ranks by description coverage, and the window holds 60 rows — both numbers came
  from one incident.** On 2026-08-16 a user sent nine messages in twenty minutes trying to record a
  refund against a card debit; eight ended `abandoned`. The movement existed (`Débito tarjeta Mercado
  Pago`, `-61306.49`, dated 08-04) but sat at **row 32 of a 30-row window**, so it was never a
  candidate. Raising the limit alone does not fix it: eight rows in that window share the words
  "mercado"/"pago"/"tarjeta", the boolean matcher rated all of them equal, and the cut kept the five
  most recent — the right row still loses. Ranking alone does not fix it either, because the row is
  out of reach. Scored, it wins outright: coverage 4/4 = 1.00 against 0.67 for the best of the noise
  (`Transferencia a Mercado Pago`, 2/3). Coverage is a **fraction of the candidate's own tokens**, not
  a count of hits — counting hits ties `Transferencia Banco Galicia a Mercado Pago` with the correct
  row at 2 apiece. The date term is capped at 0.25 so it can only break ties: on this case it
  contributed 0.019 to the right row against 0.025 to the noise, and the coverage gap decided it
  anyway. 60 rows is ~14 days at production rates (measured 2026-08-20: row 30 = 8.5 days, row 50 =
  12.7), which is also what a relative reference needs since the model stopped sending dates for one.
  **Paging ("show me five more") was designed and rejected**: measured against the whole
  post-stage-5 record, it fixes zero of the nine live failures, and with ranking in place page two is
  by construction the next-least-similar rows. What recovers a miss is a new search with better
  words, not more of the same ranking.

## Conversation engine and flows

- **Pre-seeded, gap-only conversation flows via `Engine.StartWithData` + `Step.Skip`.** A flow can start already carrying data (e.g. an LLM classification or a resolved reference-resolution candidate) and skip every step whose data is already known, re-evaluating skip conditions after each `Advance` rather than only once at start. This is the reusable mechanism behind CREATE's gap-fill.
- **Conversation state is DB-backed JSONB.** `conversation_states` table, one row per user. Do not move to in-memory without explicit discussion.
- **Mandatory onboarding steps piggyback on the one-flow-at-a-time rule.** There's no `users.onboarding_completed` flag. A step is "mandatory" simply because it's auto-started (`engine.Start`/`startFlowIfNotBusy`) right after the previous one finishes, and `Engine.InProgress` (backed by `conversation_states`) blocks any other flow from starting until it's done. If the user disappears mid-flow, state persists in Postgres and resumes on their next message — no extra bookkeeping needed. See the account-create flow in `internal/flow/account_create_flow.go` for the reference implementation.
- **Onboarding accounts are created atomically with opening movements.** `movement.InsertAccountsWithOpenings` (a cross-repo method wrapping both `account` and `movement` tables in a single `gorm.DB.Transaction`) inserts N accounts + N opening `transfer` movements in one shot: a mid-insert failure rolls back all, so onboarding never leaves a user with half their accounts. Opening balance is captured in the first movement; later corrections/additions use the standard CREATE/UPDATE/DELETE flows.
- **The first account per currency is a throwaway default.** Every onboarding creates at least one account per currency (ARS, USD, or both). The first per currency is flagged `IsDefault=true` — a seed account that satisfies the partial unique index `(user_id, currency) WHERE is_default = TRUE`. If the user later creates additional accounts in that currency via ACCOUNT_CREATE, they are never `IsDefault=true`, and may reset the original default if desired. The "throwaway" framing is accurate: on `/admin/users/:telegramID/reset`, all are soft-deleted, freeing the default slot for a fresh onboarding.
- **Onboarding confirm gate uses Confirmar/Reescribir, no silent insertion.** After `ClassifyOnboarding` parses the user's accounts, a `onboarding_confirm` flow displays the summary with two buttons: Confirmar (proceeds to atomic insertion + receipt) and Reescribir (returns to the free-text collection step for a full retype). Per-field editing (Task 11, deferred) is not yet implemented. This mirrors the confirm gate on other high-stakes operations (UPDATE/DELETE) — onboarding's account setup is not frictionless, it stops for review.
- **The resume gate is `conversation.Engine`-level, not per-flow.** Every registered flow gets idle-timeout/retry-escalation handling for free, applied centrally in `Handle` before any step-specific `Process` runs — adding it to each of the 7 flows individually would mean 7 near-identical implementations and 7 places to forget it on flow #8. The one cost is that `conversation` can't know the actual Spanish copy per flow (it doesn't import `messaging`), so `NewEngine` takes an injected `resumeLabel func(flowName string) string` instead.

## Taxonomy

- **Taxonomy is closed.** LLM may only assign existing category/subcategory names. Unknown or low-confidence → `PENDING_REVIEW | PENDING_REVIEW`.
- **`subcategory.Cache` splits global/perUser instead of copying global rows per user.** The seeded taxonomy is ~90 rows shared by every user; once users can create their own subcategories, duplicating those 90 rows into a per-user slice would waste memory and (worse) require re-syncing every user's copy whenever a global row changes. `global []Subcategory` stays one shared backing slice; `perUser map[uint64][]Subcategory` holds only what each user actually created — usually a handful.
- **Icons moved from a static Go map to a DB column.** `subcategory.CategoryIcon`/`IconFor` was fine while only the admin-seeded taxonomy existed ("update the map by hand when a new category appears"); a user creating their own category needs to choose its icon at creation time, which a compiled-in map structurally cannot hold. The DB row is now the only source of truth; every icon lookup goes through a real `Subcategory.Icon` field or a `Cache` method, never a category-name-keyed static map.

## The agent loop and QUERY

- **Query responses = LLM-formatted, via a read-only agent loop.** For `query` intents the typed tools fetch structured data from the DB (invariants in Go), the loop feeds it back, and Groq writes the human-readable Spanish response. New query capabilities are a drop-in `AgentTool` + executor case, never a new flow — the "sin tanto desarrollo de feature" payoff on the safe (read) half. Mutations stay on their guarded CREATE/UPDATE/DELETE paths; the loop never mutates and has no `run_sql`.
- **The conversation thread lives in Postgres, bounded by a TTL window and a turn cap.** Follow-ups need the prior turn's referent, but the stateless paths hold no flow state. Chosen: a dedicated `chat_turns` table (survives Render redeploys mid-conversation, unlike RAM/ephemeral disk) read only within a configurable TTL and capped at N turns. It started as `query_turns`, QUERY-only; the agent loop reads the same thread for every intent, so the table and its package were renamed (`20260731120000`, `queryhistory`→`chathistory`) — the old name had become a lie — so an old conversation never loads and the LLM prompt never saturates. Ephemeral by design: hard-pruned on `Append`, never soft-deleted, since a turn carries no accounting value and must actually disappear. History is textual context only; tools re-run every call, so a cached number can never leak. Best-effort throughout — a history read/write failure degrades to today's stateless behavior, never breaks the answer.
- **The unified agent loop replaces the router, one intent class at a time — the router stays alive until the last stage.** `orchestrator.Run` is a tool-calling loop with the full taxonomy and account list in context, replacing a 10-way intent classifier that has to decide blind. It is being adopted in five stages, and the staging trick is that `Run` is **added, never swapped**: the router keeps gating which intents reach the loop, so any stage can be bisected and reverted on its own. Stage 1 landed `Run` with nothing calling it; stage 2 routed `UPDATE`/`DELETE` (the two worst-performing intents) plus the `ask_user` primitive and `pending_actions`. The old flows a stage replaces are **not deleted when it lands** — they are the cheap way back until the stage's own gate passes, which is a production measurement, not a green build.
- **One transparent `search` for the QUERY tools, not three typed filters (2026-08-14).** `sum_movements` and `list_movements` used to take `category`, `subcategory` and `description` separately, which asked the model a question it cannot answer reliably: *which field does this name live in?* On 2026-08-13 it guessed `category="lote"` — "lote" is a word in the **description** of five movements spread across four subcategories — got zero rows, and narrated "$0" over $30.343,74 of real spending. The replacement is a single `search` matching category OR subcategory OR description, case- and accent-insensitively, folded in SQL through the `unaccent` extension so all three legs use one dictionary (folding taxonomy in Go with `foldAccents` and descriptions with `unaccent` would give two semantics inside one parameter). **The price is precision**: `search="Salud"` can now pull in a *"seguro de salud"* filed under Transporte, where the old exact match would not have. That is judged the better failure — a slightly wide answer the user can see and correct, versus a confident zero they cannot. The non-obvious corollary is what made it worth doing: with one filter, "the filter matched nothing" and "there were no movements" stop being the same zero, which is what allowed the single ambiguous `msgQueryNoRows` to be replaced by four messages that each state a verified fact — and the "term exists nowhere" case to become a hard error the model cannot narrate as $0. `MovementQuery` keeps `Category`/`Subcategory` as exact filters, unused by any tool: the Mini App's drill sets them from a name the app itself produced, where exact equality is correct and a fuzzy match would stop the leaf reconciling against the total that led the user there.
- **An absence is a fact and needs a tool behind it — and that rule lives in the prompt, not in Go (2026-08-14).** The query system prompt used to say to mark anything "without data" as `sin registros`. That phrase conflates two different facts — *queried and came back empty* versus *never queried* — and the model took the wide reading: asked about three things, it queried two and answered "No hay registros de Cuota préstamo en agosto" over $80.000 of real spending, quoting the prompt's own wording. The fix extends the rule that already governed amounts ("never invent or estimate a number without a tool behind it") to **absences**, and pairs it with an instruction to request every tool a multi-entity question needs **in the same round** — which the loop already executed but nobody had asked for (29 of 30 rounds emitted a single tool call). **The price:** the guarantee is a prompt instruction, so a disobedient model can still deny. A deterministic alternative was available — Go appending a fixed clause on the door-2 path, which is knowable with certainty because door 2 is only reached when the round cap ran out with the model still requesting tools — and was rejected in favour of natural prose. The compensating control is an eval that measures compliance rather than assuming it; note its blocklist of denial phrases is inherently incomplete, having passed green on its first run while the model denied three entities with a phrasing the list lacked. The round cap went 2→3 in the same change, but the cap was never the cause: with batching the third round is rarely reached, and the arithmetic that once forbade it had expired (the forced narration moved to its own model bucket, the router that reserved quota was deleted, and a 429 stopped being fatal once the fallback chain landed).

- **The QUERY round cap is 3, and the arithmetic that justified 2 expired (2026-08-14).** The old
  reasoning - "cap 3 means 4 calls against an 8000 TPM bucket, so the last always 429s" - failed on
  three measured points: the forced narration left the bucket (its own model, ceiling 400, 788
  tokens measured), the router that reserved ~670 for the next message was deleted in stage 5, and
  a 429 stopped being fatal once `roundWithFallback` walked the chain (on 2026-08-14, 20 of 52
  calls bounced and no query was lost). Budget at cap 3 against 8000, average case (first-round
  prompt 1209, +320 per round, measured on `llm_calls`): `(1209+640)+(1529+1024)+(1849+1024) =
  7275`, ~9% of air. At p95 (prompt 2409) it overflows, and that is accepted: the third round is
  rare - the prompt asks the model to batch its tools into one round - and overflowing costs a
  model hop, not the answer. The change mattered because **21% of turns exhausted the cap of 2**
  (11 of 53 measured), and that door is where the forced narration was denying things it had never
  queried.
- **The QUERY loop answers through two doors, and both are normal.** Door 1 is a round returning
  content instead of tool calls - the common path, 16 of 22 measured queries narrate this way
  (round 0 asks for tools, round 1 narrates). Door 2 is the forced exit: the cap ran out with the
  model still requesting tools, so a final `tool_choice:"none"` call makes it narrate with what it
  has; only an empty final answer returns `ErrQueryMaxIterations`. Anyone touching the token caps
  needs both in mind: capping rounds "because they only pick tools" truncates the real answer of
  most queries. **That reasoning was tried once and was wrong.**

- **The model cannot turn a weekday into a date, and no prompt fixes it.** Measured 2026-08-19
against the real model with the "hoy" pinned to `miércoles 2026-08-19` and the production case
("la compra de locro **del lunes**", the Monday being the 17th): `gpt-oss-20b` answered the 15th
with only the date in the prompt, the **14th** once the weekday was added, and the **14th again**
with a table spelling out `lunes 2026-08-17`. `gpt-oss-120b` sent no date at all. Four runs, four
wrong answers — it ignores the fact even when it is written in front of it.
So `date_from` now asks for a date **only** when the message spells out day and month
("el débito del 4 de agosto"), which the model transcribes correctly; a relative reference travels
with no date and `resolveCandidates` falls back to the `created_at` window, where the text match
finds the movement. That is what the 120b did by accident on the real case, and it would have
worked. Rejected: computing the date in Go (a date-expression parser for one phrasing) and
widening the window (it would undo the 24h margin the 04/08 case needed). Known gap, accepted:
**"el lunes" still sends a wrong date** — `TestAgentDateAnchorEval` keeps that case red on purpose.

## Groq quota, the 429 queue and rate limits

- **A terminal Groq 429 is a typed error (`orchestrator.RateLimitedError`), not a string to re-parse.** `Client.send`'s existing retry loop already computes the best available wait (header priority over body-parsed text); wrapping that wait in a struct returned via `errors.As` means the pending-jobs queue (and any future consumer) never re-derives or re-parses anything Groq said — it reads `RetryAfter` off the error itself. The alternative (checking `errors.Is(err, someSentinel)` and separately re-parsing the body for the wait) would duplicate parsing logic `send` already did.
- **The drain replays a queued job through the exact same webhook handler, not a parallel code path.** `RunJobDrain` calls `handleFreeText`/`proceedToUpdateConfirm` directly — the same functions the Telegram webhook calls — so a replayed message gets identical routing, guard checks, and copy as a live one; a second "replay" implementation would drift from the live path the first time either changed. The cost of this choice is real: those handlers now also enqueue/ack/gate, so the drain calling them needed its own guard (below) instead of being a passive replay.
- **The FIFO ordering invariant lives at the webhook boundary (`handleConversationInput`), not inside `handleFreeText`.** Because the drain's replay calls `handleFreeText` directly, a guard placed inside `handleFreeText` would see the drain's own in-flight job as "pending" and re-enqueue it — an infinite loop. `enqueueBehindPending` sits one level up, in the webhook-only `handleConversationInput`, which the drain never touches — so it enforces "don't process a new message ahead of one already queued" for live traffic without ever seeing a replay.
- **A ctx flag (`isReplaying`) tells a Groq-error site whether it's live or under replay — cheaper than threading a parameter through every call.** `handleGroqError` behaves differently in each case (live: enqueue + ack; replay: propagate the error so the drain re-gates and leaves the job) but is called from deep inside shared flow logic (`startMovementCreate`, `startMovementUpdate`, etc.) that both paths share. Passing an explicit bool through every intermediate signature would touch far more call sites than a `context.WithValue` flag set once at the drain's replay entrypoint.
- **Give-up ceiling is 2 hours, not a daily reset.** Groq's TPD (tokens-per-day) limit is *rolling*, not a fixed midnight reset — the largest wait actually observed via `x-ratelimit-reset-tokens` is on the order of ~16 minutes. `maxJobAge=2h` gives ~7× margin over that observed ceiling while still recognizing a job stuck far longer as a permanent failure (dead API key, billing issue, provider outage) rather than retrying it forever. Give-up runs *before* the replay call, on `CreatedAt` age alone, so an abandoned job never costs another Groq call.
- **A parked action is not a queued message.** `pending_llm_jobs` caches a *message* that could not be processed (terminal Groq 429); `pending_actions` holds an *already-interpreted action* missing an answer only the user has. Different lifetimes, different drain triggers, deliberately not merged. `pending_actions` drains one at a time (WIP=1), which is also what keeps `intent_events`' "last pending" correlation honest — and why a loop turn that parks nothing must resolve its own metric, or the next message flips it to `abandoned`. The same duty binds the other end: a parked action the user **cancels** resolves its own metric too. It did not until 2026-08-19, which is why the table held 121 `abandoned` against 2 `update_cancelled` — a user's deliberate "no" was indistinguishable from a flow that died.

- **`maxAgentCompletionTokens` is 1500, and the number is measured, not round (2026-08-12).** Groq
  charges `prompt + max_completion_tokens` against the quota whether the completion uses it or not,
  so this constant is quota spent on every call. The worst real case in 45 days - a 7-movement
  message on 2026-08-10 - used **1183** completion tokens; it used 1949 before `record_movements`
  stopped emitting category and subcategory (~40% less output per movement). Distribution over 77
  messages carrying movements: 84% one movement, 13% two, exactly one 5 and one 7. At ~170 tokens
  per movement, 1500 covers 8-9. **Why not 1000**, which the completion distribution suggested:
  those completions were almost all single-movement, so cutting there would have truncated the
  batch of 7 - the only case this number exists for. That is the same sampling error that had
  already cost once. History: 2048 left a 7-movement batch on the edge (two prior attempts of the
  same message returned 400 with the JSON cut in half), 3000 fit, 4096 put every call over the
  8000 TPM ceiling. Dropping 2500 to 1500 is ~17% less daily quota per message. **What it does not
  fix:** bursts. At 8000 TPM one call per minute fits at either value; two would need a cap under
  500. The minute ceiling is the `pending_llm_jobs` queue's job, not this number. Revisit if a
  message ever carries more than 8 movements - a bulk import, say - where the truncated-JSON 400
  returns.
- **`userLocks` exists because the 429 was an accidental serializer, and the fallback chain removed
  it (2026-08-12).** Every piece of per-user state assumes one message in flight: `conversation_states`
  has `user_id` as PRIMARY KEY, `pending_actions` drains one at a time, and `intent_events.Resolve`
  closes "the user's most recent pending". Rate limiting used to hold that invariant without
  anyone designing it - two messages in a row did not both fit in the TPM, so the second queued and
  the drain processed them in file. Adding the model fallback chain meant the second message stopped
  bouncing, and both ran at once. It showed on the first test: two messages in the same second left
  one `intent_event` carrying the other's movement. The movements themselves were fine - they do not
  depend on the invariant - but the next case does: two creates that both open a category gap
  overwrite each other's `conversation_states` row, and the first is lost silently.
- **`sameTurnCalls` lists the Groq call pairs that can collide inside ONE turn, and the list is
  evidence-based (2026-08-13).** Groq's ceilings are per model, so two calls of the same turn
  pointing at the same model compete: the first reserves, the second bounces. Having `create` on
  `agent`'s model cost ~95 seconds and ~11.000 tokens burned on retries that could not advance.
  **What is deliberately NOT in the table matters as much as what is:** `update` and `onboarding`
  live in flow steps that arrive in *later* messages, not in the agent's turn, and cross-turn
  competition is already covered by the fallback chain. `query`+`create` and
  `classifier`+`narration` are also absent although they share a model today - they only appear if
  the table is read as transitive, and no trace shows either pair actually co-occurring in one
  turn, while every listed pair has one. Add a pair when a trace shows it, not when it looks
  possible. Since 2026-08-17 the four pairs cannot all be satisfied - Groq retired the third usable
  model - so `TestEveryConfigFile_HasNoSameTurnModelCollision` is red on purpose. The table is left
  intact: it describes what really collides, and that did not change because a model went away.

## Notifications and reminders

- **System→user notification engine: shared `send()` + ticker, per-notifier trigger/query stays specific.** `internal/notifier.Sweeper` is one `time.Ticker` goroutine whose `tick` calls a `sweepX` per notifier — today three: `sweepReminders`, `sweepWeeklySummary`, `sweepRetention`. The *only* shared asset is the ticker and an injected `send(ctx, chatID, text)` — deliberately reachable outside the sweeper so a future admin-triggered broadcast can call it directly without going through the ticker. Everything else (candidate query, fire condition, cadence, guard) is each notifier's own; there is no polymorphic `notifications` table, no notification-type registry, no templating engine. A `notifications(type, payload jsonb)` table would force a lowest-common-denominator schema and lose typed columns/FKs for a gain (one shared table) nobody needs — the reminder and a future Cafecito prompt or weekly summary don't share a data shape, only a delivery mechanism. Adding a second tenant is a ~20-line sibling function and one line in `tick`; a `[]notifier` abstraction is deferred to the third tenant (`// ponytail:` marked in `sweeper.go`).
- **Reminder window stored as minutes-since-midnight, not a SQL `time`.** `window_start_min`/`window_end_min` are plain ints (ART, e.g. 20:00 = 1200). The fire target is the window's midpoint (20:00–21:00 → 20:30), which needs sub-hour precision — a `time.Time`/`time` column adds Go/GORM marshaling overhead for exactly the arithmetic two ints already do (`(start+end)/2`). The midpoint itself is never stored, only derived (`Reminder.MidpointMin()`).
- **Reminder delete == disable, no `deleted_at`.** With one `reminders` row per user, "borrar el recordatorio" and "apagar el recordatorio" are the same user-facing fact — stop reminding me. `enabled=false` covers both; re-enabling is just sending a new window. Skips a second code path and a `deleted_at` column for a distinction the user never perceives.

## Package layout, metrics and tooling

- **Repo-specific subagents over generic ones for locate/edit/review.** `.claude/agents/repo-investigator`, `repo-builder`, `repo-reviewer` mirror the caveman plugin's `cavecrew-*` pattern (narrow tool grants, hard scope refusal, compressed output) but carry this repo's own rules — money type, currency handling, `conversation_states` access, closed taxonomy — baked into their prompts. Use them instead of generic `Explore`/`general-purpose`/`code-reviewer` agents for work scoped to this repo. `repo-builder` enforces the completion-evidence rule (`CLAUDE.md` §7) at the agent level: it will not return a receipt without a clean `bash check.sh`.
- **Métricas end-to-end por outcome, no por confidence autoreportada ni confirmación forzada.** Se mide '¿el usuario logró lo que quería?' con la señal que los gates confirm/cancel existentes ya producen + el outcome del terminal de cada flow, correlacionado por la invariante WIP=1 (último pending). No se agregó `confidence` (cambio de prompt en la ruta caliente + autoreporte débil) ni confirmación a CREATE frictionless (rompe el principio 'CREATE sin fricción'); el feedback de CREATE frictionless se infiere del follow-up UPDATE/DELETE, que ya se loguea.
- **`internal/controller/messaging` was split by cluster, one package per reason to change.** It had grown to ~10k non-test lines across 52 files — 4× the next package — and go.dev's module-layout guidance says the answer is to split off supporting packages under `internal/`, not to document around it. Done in stages, each a pure move verified by the full test protocol: copy→`messages`, flows→`flow`, the unified loop→`agent`, free-text reads→`query`, contextual tips→`nudges`, the 429 queue→`pendingjob`, the configuration wizards→`settings`. The edge kept 1.6k lines across 20 files: webhook handlers, `/start`, the bridges, `userLocks`, tracing, metric outcomes.
  The seam that made it work is the **consumer-defined interface**: each cluster declares the narrow set of methods it needs (`flow.runner`, `agent`'s `agentServices`, `settings.Services`, …) and `*controller` implements all of them structurally through one-line bridge files. No cluster imports a repository, and none imports another cluster's internals. The methods are exported although most interfaces are not — an interface with unexported methods can only be satisfied from inside its own package, and the implementation is at the edge.
  Two things were deliberately *not* moved. The wizard **starts** stay out of `flow` because they call the LLM and `flow` must not import `orchestrator` — they went to `settings`, which `flow` reaches back through two runner methods. And the tests that exercise a finish through the `*controller` stayed at the edge: they test webhook→engine→finish, which is the edge's job. Using `flow` does not make a test a `flow` test — only the builder/step tests moved.
