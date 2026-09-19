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
- [The Mini App](#the-mini-app)
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
- **Signed, account-attributed movements — the money model (money precision).** The model itself is in [AGENTS.md](../AGENTS.md#the-accounting-model--read-before-touching-any-money-path); what follows is why it replaced the previous one. This supersedes the earlier "expense/income are `account_id = NULL`, positive" rule, which the onboarding real-accounts redesign left incoherent (spending never debited the real balance) and which let malformed rows through (a negative `expense`, and a `0.00` from feeding a signed candidate into a positive-reasoning UPDATE prompt). Full rules in [business-rules.md](business-rules.md#the-accounting-model-money-precision--read-this-before-touching-any-money-path); spec + guard in `docs/superpowers/specs/2026-07-07-signed-account-attributed-movements-design.md`. **The single place where a wrong sign or `account_id` is a financial bug, not a cosmetic one.**
- **A transfer query sees `Sistema | Transferencia`, and gets both legs separately (2026-08-21).** `MovementQuery.apply` excluded reserved categories unconditionally, so asking `type=transfer` selected exactly the rows the next clause deleted — every own-account transfer lives under `Sistema | Transferencia`. Measured in prod: *"¿cuánto transferí este mes de fci a mercado pago?"* answered **$100.000 against $1.548.595,59 real**, and the one surviving row only survived because it was miscategorised `Inversiones | FCI`. `describeEmptyResult`'s reserved probe never fired because it runs on **zero rows only** — the net covers "all hidden", not "mostly hidden", which is worse: plausible, specific, 15× off. The schema's own description already promised the escape hatch ("salvo que se pida `type=transfer`"), so this was a documented contract the code never kept. Two things make the fix safe, and both were measured before writing it: the exemption is **per-subcategory**, because `Sistema | Saldo inicial` is also `type=transfer` (11 rows) and a category-wide exemption would count every opening balance as a transfer; and an ungrouped transfer sum is **split by direction** (`GroupByDirection`) and never carries a total line, because a transfer is two rows and `SUM(ABS(amount))` over both is exactly 2× (14 legs = $2.897.191,18 = 2 × $1.448.595,59). Direction is *not* a tool parameter: returning both labelled answers every phrasing without a new schema field, and this model picks badly among near-identical options. The total-line refusal is the same reasoning that already governs `group_by=type`, and it is what stops the doubling returning through `group_by=account`. Scope was held to the `type=transfer` path — verified call site by call site that nothing in `summary`, `miniapp` or `nudges` ever sets `Type = transfer` on the non-`OnlyReserved` branch.
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
- **A balance adjustment that states the account and the total opens on its confirm screen, and Go decides that, not the model (2026-09-18).** Measured on user 2 that day: two adjustments, each one message + picker + menu + retyping the amount + confirm, and the amount was already in the first message ("Mercado Pago a $207555,66"). Two causes, both verified in `llm_calls`: `resolve_account` returned `null` because the user holds *Mercado Pago* and *FCI* in both ARS and USD, which is the correct answer from the name alone; and the menu and the amount question were unconditional. **The first fix asked the model for the operation and the amount, and it failed against the real model:** gpt-oss-120b left both new fields empty on all three phrases of the eval (one was a 400), while every unit test with a fake was green. So `settings` reads all three from the text: the account (whole words, no minimum length, so "FCI" matches where `flow.TokenCoverage` would skip it), the intent (verb stems, blocked by rename/default/create/delete stems), and the total (a single number in an unambiguous shape). Anything doubtful drops to the question it replaced, never to a guess. **No currency cue means ARS**, chosen with the user over a narrowed picker: a bare `$` is pesos in Argentina, and a wrong choice is contained by the confirm screen, which prints the currency and carries "🏦 Otra cuenta" and "✏️ Cambiar monto". The rules were checked once against every `ACCOUNT_MANAGE` message in `intent_events` as of that day (60, in a throwaway run); `TestStartAccountManage_RealMessages` keeps a sample of those real messages plus hand-made variants for the edges (US format, `mil`, negatives, two numbers).
- **The resume gate is `conversation.Engine`-level, not per-flow.** Every registered flow gets idle-timeout/retry-escalation handling for free, applied centrally in `Handle` before any step-specific `Process` runs — adding it to each registered flow individually would mean one near-identical implementation per flow — fifteen today — and one more place to forget it on every flow added after. The one cost is that `conversation` can't know the actual Spanish copy per flow (it doesn't import `messaging`), so `NewEngine` takes an injected `resumeLabel func(flowName string) string` instead.

## Taxonomy

- **Taxonomy is closed.** LLM may only assign existing category/subcategory names. Unknown or low-confidence → `PENDING_REVIEW | PENDING_REVIEW`.
- **`subcategory.Cache` splits global/perUser instead of copying global rows per user.** The seeded taxonomy is ~90 rows shared by every user; once users can create their own subcategories, duplicating those 90 rows into a per-user slice would waste memory and (worse) require re-syncing every user's copy whenever a global row changes. `global []Subcategory` stays one shared backing slice; `perUser map[uint64][]Subcategory` holds only what each user actually created — usually a handful.
- **Icons moved from a static Go map to a DB column.** `subcategory.CategoryIcon`/`IconFor` was fine while only the admin-seeded taxonomy existed ("update the map by hand when a new category appears"); a user creating their own category needs to choose its icon at creation time, which a compiled-in map structurally cannot hold. The DB row is now the only source of truth; every icon lookup goes through a real `Subcategory.Icon` field or a `Cache` method, never a category-name-keyed static map.

## The agent loop and QUERY

- **Query responses = LLM-formatted, via a read-only agent loop.** For `query` intents the typed tools fetch structured data from the DB (invariants in Go), the loop feeds it back, and Groq writes the human-readable Spanish response. New query capabilities are a drop-in `AgentTool` + executor case, never a new flow — the "sin tanto desarrollo de feature" payoff on the safe (read) half. Mutations stay on their guarded CREATE/UPDATE/DELETE paths; the loop never mutates and has no `run_sql`.
- **The conversation thread lives in Postgres, bounded by a TTL window and a turn cap.** Follow-ups need the prior turn's referent, but the stateless paths hold no flow state. Chosen: a dedicated `chat_turns` table (survives Render redeploys mid-conversation, unlike RAM/ephemeral disk) read only within a configurable TTL and capped at N turns. It started as `query_turns`, QUERY-only; the agent loop reads the same thread for every intent, so the table and its package were renamed (`20260731120000`, `queryhistory`→`chathistory`) — the old name had become a lie — so an old conversation never loads and the LLM prompt never saturates. Ephemeral by design: hard-pruned on `Append`, never soft-deleted, since a turn carries no accounting value and must actually disappear. History is textual context only; tools re-run every call, so a cached number can never leak. Best-effort throughout — a history read/write failure degrades to today's stateless behavior, never breaks the answer.
- **The unified agent loop replaces the router, one intent class at a time — the router stays alive until the last stage.** `orchestrator.Run` is a tool-calling loop with the full taxonomy and account list in context, replacing a 10-way intent classifier that has to decide blind. It is being adopted in five stages, and the staging trick is that `Run` is **added, never swapped**: the router keeps gating which intents reach the loop, so any stage can be bisected and reverted on its own. Stage 1 landed `Run` with nothing calling it; stage 2 routed `UPDATE`/`DELETE` (the two worst-performing intents) plus the `ask_user` primitive and `pending_actions`. The old flows a stage replaces are **not deleted when it lands** — they are the cheap way back until the stage's own gate passes, which is a production measurement, not a green build.
- **One transparent `search` for the QUERY tools, not three typed filters (2026-08-14).** `sum_movements` and `list_movements` used to take `category`, `subcategory` and `description` separately, which asked the model a question it cannot answer reliably: *which field does this name live in?* On 2026-08-13 it guessed `category="lote"` — "lote" is a word in the **description** of five movements spread across four subcategories — got zero rows, and narrated "$0" over $30.343,74 of real spending. The replacement is a single `search` matching category OR subcategory OR description, case- and accent-insensitively, folded in SQL through the `unaccent` extension so all three legs use one dictionary (folding taxonomy in Go with `foldAccents` and descriptions with `unaccent` would give two semantics inside one parameter). **The price is precision**: `search="Salud"` can now pull in a *"seguro de salud"* filed under Transporte, where the old exact match would not have. That is judged the better failure — a slightly wide answer the user can see and correct, versus a confident zero they cannot. The non-obvious corollary is what made it worth doing: with one filter, "the filter matched nothing" and "there were no movements" stop being the same zero, which is what allowed the single ambiguous `msgQueryNoRows` to be replaced by four messages that each state a verified fact — and the "term exists nowhere" case to become a hard error the model cannot narrate as $0. `MovementQuery` keeps `Category`/`Subcategory` as exact filters, unused by any tool: the Mini App's drill sets them from a name the app itself produced, where exact equality is correct and a fuzzy match would stop the leaf reconciling against the total that led the user there.
- **`search` matches every word, not one contiguous substring (2026-09-18).** On 2026-09-17 user 2 asked three times for the insurance of their car and motorbike in August and got $65.825 each time, the real figure being $45.520 (or $10.980 for the motorbike alone). The model sent `search="seguro"` every time, which also pulls in *"seguro del hogar"*. It did not do that out of carelessness: the substring `"seguro de la moto"` matches no row (the description is "Seguro moto"), so the one bare word that returned anything was the over-broad one. That evening the answers were right only because "seguro moto" happened to equal a description literally, and the last one was added up by the agent's fallback model from `chat_turns`, with no tool behind it. Now the term is split into words, connectors (`de`, `del`, `la`, `y`…) are dropped, and every remaining word must appear somewhere in `category + subcategory + description`. **Measured before choosing, over every `search` term production had sent (31) plus the incident phrases:** Postgres FTS with the `spanish` config, the textbook answer, broke "super" (14→0, it does not match inside "Supermercado") and user 3's "HBO"/"Disney" (2→1); tokens with a 4-rune floor broke "FCI" and "HBO". The word-AND left all 31 unchanged and fixed "seguro de la moto" (0→1), "seguro del auto" (0→1) and "FCI Mercado Pago" (0→26). **Rejected:** a `search` array (already rejected on 2026-08-21 — the model does split entities into separate calls); forcing a subcategory breakdown on every ungrouped `search` sum. That would have shown the home insurance slipping in, but it changes the output of every search sum, so it waits for a second case.
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

- **`record_movements` still tells the model to resolve a weekday, and it still gets it wrong.**
  The schema forbids `correct_movement` from computing a date from a weekday, for the reason above.
  But a movement can't be inserted with no date, so the `REGLA DE FECHA` in the prompt
  (`internal/orchestrator/agent_prompt.go`) still asks the model to resolve "el lunes" as the most
  recent one that already happened — and it still gets it wrong. "gasté 5000 el lunes" is saved
  with the wrong date, silently, and no eval covers it: `TestAgentDateAnchorEval` only checks
  `correct_movement`'s arguments. This is an accepted gap, not a fix: dropping the sentence from
  the prompt would not correct the date, it would just leave the model with no guidance at all. The
  real fix is resolving the weekday in Go before calling the model, and that is separate work. The
  weekday stays in "Hoy es" anyway, despite this measurement reading neutral-or-worse for
  correction, because it serves QUERY, where "esta semana" resolves against the real day.

### Why the app ignores the model's `account_id` on expenses (2026-08-22)

Two expenses of user 3 were written to `Banco Galicia` instead of their default ARS account. The
model had emitted `account_id: 25` on both, unasked; one also carried
`account_name_guess: "Banco Galicia (ARS)"` — the literal render of `buildAccountsBlock`'s
`"%d | %s (%s)"`, so it was copying a prompt line, not reading the message.

Measured over the whole `llm_calls` table (52 `record_movements` rows): 30 of 43 `expense` rows
carried an `account_id`, and in **0** of those 30 had the user named an account. 28 landed on the
default anyway, because the account block is ordered by id and user 2's first ARS row *is* their
default. User 3's first ARS row is not. Same model behaviour, two outcomes, decided by primary
keys.

`transfer` measured the opposite way: 8 of 8 legs carried an id and needed it. Two real messages
spell the account as `mercadopago` / `mercadpago`, which no name match resolves — the model's id
was the only thing that saved those legs. That is why the exemption is by movement type and not a
blanket removal of the field.

**Rejected:** validating the model's `account_id` against the message instead of ignoring it. It
leaves the mirror-image hole — when the user names an account and the model puts it *only* in
`account_id`, dropping the id writes silently to the default. Reading the message directly closes
both directions with one rule.

**Rejected:** lowering `TokenCoverage`'s 4-rune floor so an account named `FCI` resolves from the
message. The helper is shared with `GuessNamesOwnAccount` and reference resolution; all 8 measured
`FCI` rows are transfers, which are exempt. The limitation is documented instead.

- **An eval can be impossible by construction, and look merely red (2026-08-12).** Nine cases of
  the agent-loop eval expected `find_movements_to_correct`, which is *only a constant* — it was
  never in `AgentTools()`, so it is never sent to the model. Those nine could not pass no matter
  what the model did, and nobody noticed because a red eval reads like a model problem. Before
  tuning a prompt against a failing case, check that the tool the case expects is actually one
  the model was offered.

**2026-09-01 — the model's arithmetic drifts, so the app took the average back.** Measured
against SQL on an answer the user accepted as good: the model narrated $13.548,71 where
`SUM(ABS(amount))/31` gives $13.550,16, and $9.302,29 where it gives $9.307,52 — ~0.05% off,
with no fixed sign. It divides by hand. This is the third operation the prompt licensed and the
app took back, after summing grouped rows (2026-08-14, it answered 2.031.070 out of two rows
that added to 2.065.070) and splitting transfers (2026-08-21).

**Two operations stayed with the model on purpose, and both are named explicitly in the prompt so
the next measurement can audit them:** subtracting two periods ("how much more than last month"),
and the MONTHLY average. The monthly one is not an oversight — its divisor is the days of *that*
month, not of the range, so it is a different calculation rather than the same one under another
`group_by`. Forbidding it along with the daily rate would have left "how much per month on
average this year" with no path at all, since `spending_report` excludes `group_by=month`.

The same day exposed the more expensive half: `group_by` is a single axis, so "how much per day
AND per category" is not expressible. Without the word "promedio" in the message the model went
looking for the cross-tab in series — `group_by=day`, then `group_by=category`, then
`list_movements limit=50` — and since every round re-injects the previous results, Groq's
reservation (`prompt + max_completion`, charged up front) climbed 1998 → 2903 → 4426 against an
8000 TPM per-model ceiling. Four queries ended in `query_failed`; three never started, dying on a
400 `tool_use_failed` in round 0, one of them generating "No dispongo de una forma de obtener el
desglose de gastos simultáneamente". 15 of the last 30 days' 85 queries (18%) ask for a derived
number.

**The divisor is calendar days, clamped at today.** August whole = 31; "this month" on a
September 3rd = 3, not 30. Dividing by 30 there reports a third of the real rate with nothing
marking it. The result always states the divisor, for the same reason `appendConsultedRange`
exists: it is the only thing that betrays a badly resolved range.

**Rejected: a two-axis `group_by`.** 30 days × 12 categories is ~360 cells in a tool result that
gets re-injected every round — worse than the `list_movements` that broke the ceiling — and
illegible as a Telegram message.

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

- **`qwen/qwen3.6-27b` is deliberately NOT in any chain, and this is the note that keeps it
  out.** It is the obvious candidate every time someone looks at
  `TestEveryConfigFile_HasNoSameTurnModelCollision` sitting red and reaches for a third TPM
  bucket to fix it. It emits its reasoning **inside the content**, so the `<think>` block reaches
  the user. That is not a cost problem that a cap could solve — it breaks the output.
  `qwen/qwen3.8-27b` is a different model and does not show this: none of the live Telegram
  turns below leaked a `<think>` block, so it is the one actually in the fallback lists.
- **`qwen/qwen3.8-27b` is the third TPM bucket, added to both fallback lists (2026-09-04),
  after it did not fit as a fourth free-standing chain link.** It reserves noticeably more
  tokens per round than either gpt-oss — on an identical ~2.460-token agent prompt, Groq's
  `Requested` ran 4.700-6.200 for qwen against 4.100-4.650 for gpt-oss-20b — because the
  round-1 retry re-sends round 0's assistant tool-call message, and qwen's carries more than
  the JSON arguments (see the eval numbers in `internal/orchestrator/agent_loop_eval_test.go`'s
  git history). Standalone as `agentModel`/`queryModel`+`narrationModel` it choked on its own
  8.000 TPM / 200.000 TPD ceilings running the full eval suites back to back — `qwen3.6-27b`
  even harder, with three `tool_use_failed` 400s burning 475-1.498 output tokens each before
  giving up under `tool_choice: required`. As the LAST link of a chain that only reaches it
  after two real buckets already bounced, that ceiling stopped being a blocker: verified live
  against 9 real Telegram turns run by hand (simple expense, referent-less correction — cascaded
  live to this model and closed clean, "Listo, lo dejo en $1.500." —, a two-leg transfer with
  correct signs on both legs, a multi-entity query attributing each total to its own category, a
  duplicate-category-create correctly resolving to the existing pair instead of a new one, a
  two-movement batch in one message, an account-rename gap-fill, an overdraft correctly parking
  into `movement_negative_confirm`, and a relative-date correction ("el lunes") correctly
  arriving with no date and falling through to the picker instead of guessing). It is `console.
  groq.com`'s own "Preview" tier — can be pulled with no notice — but being the LAST link bounds
  that: `roundWithFallback` only advances the chain on a 429, so a discontinued model returns a
  different error and the turn fails exactly as it would with no third link, only on the specific
  day both real buckets are also dry. `qwen/qwen3.6-27b` was not promoted alongside it — see the
  bullet above — and stacking both qwen models was considered and rejected: they are both
  "Preview," so they do not diversify risk against Groq retiring the family, and `qwen3.6-27b`
  never got this same live validation.
- **The fallback lists are not the chain, and used to repeat.** `agentRound` and `queryChain`
  build `[primary] + list`, so a list entry equal to the primary retries a bucket that already
  bounced for nothing. `config.go`'s `AgentFallbackModels`/`QueryFallbackModels` **defaults**
  carried exactly that — `["...120b", "...20b"]` behind an `agentModel` default of `...20b`, an
  artifact of swapping out the retired `llama-3.3-70b` (`fc300ab`) onto a list that already had
  the other gpt-oss model in it. `local.toml` had already overridden it by hand before this was
  named here; the default itself was fixed only once qwen gave the chain a real third bucket to
  fill that slot with (2026-09-04).
- **`QueryFallbackModels` is a separate list from the agent's, not a reuse.** Query's primary
  (120b) is precisely the agent's first substitute, so sharing one list would send the first
  retry to the model that just bounced. qwen sits SECOND in query's list (not last, unlike the
  agent's) because query starts with only one other real bucket instead of two, so there is less
  margin before a 429 goes unrescued — `narrationModel` stays pinned to `gpt-oss-20b` regardless,
  so the forced-narration call (400-token completion cap, tighter than `record_movements`'
  1.500) never routes through qwen under this ordering.
- **`NarrationModel` is chosen for NOT reasoning (2026-08-13).** The forced narration is the last
  call of a query — no tools left to pick, only prose to write. A reasoning model spends the
  completion budget thinking and returns empty; empty falls back to `queryModel`. Writing is
  tens of completion tokens, reasoning is hundreds, so the reasoner can run out before it writes
  anything. The measured numbers live at `maxNarrationCompletionTokens`, with the caveat that
  they were taken against llama-3.3-70b, which no longer exists.
- **`ClassifierModel` points at a different model on purpose (2026-08-12).** Groq's TPM ceiling
  is per model and the loop already enters its own bucket about once every ninety seconds.
  Which model is the eval's call, not the default's.
- **`agentModel` has no default, and that is the safe failure.** Since stage 5, `Run` is the only
  path a free-text message takes, so an environment that fails to declare it does not degrade —
  every loop call gets a 400 from Groq. No default means it does not start instead.

### Why a replayed job is claimed at its first write, not before or after the replay (2026-09-19)

`ecc:silent-failure-hunter` found that the drain replayed a job and then deleted it with the
error discarded. A failed delete, a crash between the two, or two instances draining during a
deploy overlap replayed the message again and recorded the money twice. Render starts the new
instance before it sends SIGTERM to the old one, and the new one starts ungated. Production
showed 102 replays and 0 duplicates from 2026-08-05 to 2026-09-04, so the hole was real and had
not fired yet.

The replay takes up to 22s, nearly all of it the Groq call. The DB write at the end takes
milliseconds. So the job is claimed (deleted, `RowsAffected == 1`) at the moment the turn
stops talking to Groq and starts having effects:

- a crash during the Groq call leaves the job queued for a retry;
- a lost claim means another instance has it, so the turn writes and says nothing.

The only remaining loss is a crash inside the milliseconds between the claim and the commit.
The graceful shutdown covers deploys: SIGTERM stops new jobs, and the in-flight replay finishes
on `context.WithoutCancel` inside a 25s budget, under Render's 30s.

Rejected:

- **Delete before the replay.** A crash anywhere in the 22s loses the message silently; about 1%
  a month at ~55 deploys.
- **A `claimed_at` lease column.** A migration, and deploys would still cut replays; it could
  only apologise afterwards.
- **Delete the job inside `InsertBatch`'s transaction.** `ResolveAndInsertMovements` creates
  accounts and subcategories before that transaction, so a double replay still duplicates
  accounts.
- **River (`JobCompleteTx`).** It is the same pattern, but a dependency with its own migrations
  and a non-pooled connection for about 100 replays a month.

## Notifications and reminders

- **System→user notification engine: shared `send()` + ticker, per-notifier trigger/query stays specific.** `internal/notifier.Sweeper` is one `time.Ticker` goroutine whose `tick` calls a `sweepX` per notifier — today three: `sweepReminders`, `sweepWeeklySummary`, `sweepRetention`. The *only* shared asset is the ticker and an injected `send(ctx, chatID, text)` — deliberately reachable outside the sweeper so a future admin-triggered broadcast can call it directly without going through the ticker. Everything else (candidate query, fire condition, cadence, guard) is each notifier's own; there is no polymorphic `notifications` table, no notification-type registry, no templating engine. A `notifications(type, payload jsonb)` table would force a lowest-common-denominator schema and lose typed columns/FKs for a gain (one shared table) nobody needs — the reminder and a future Cafecito prompt or weekly summary don't share a data shape, only a delivery mechanism. Adding a second tenant is a ~20-line sibling function and one line in `tick`; a `[]notifier` abstraction is deferred to the third tenant (`// ponytail:` marked in `sweeper.go`).
- **Reminder window stored as minutes-since-midnight, not a SQL `time`.** `window_start_min`/`window_end_min` are plain ints (ART, e.g. 20:00 = 1200). The fire target is the window's midpoint (20:00–21:00 → 20:30), which needs sub-hour precision — a `time.Time`/`time` column adds Go/GORM marshaling overhead for exactly the arithmetic two ints already do (`(start+end)/2`). The midpoint itself is never stored, only derived (`Reminder.MidpointMin()`).
- **Reminder delete == disable, no `deleted_at`.** With one `reminders` row per user, "borrar el recordatorio" and "apagar el recordatorio" are the same user-facing fact — stop reminding me. `enabled=false` covers both; re-enabling is just sending a new window. Skips a second code path and a `deleted_at` column for a distinction the user never perceives.
- **The monthly summary resolves over `users`, not `reminders`, because it cannot be turned off (2026-08-31).** The weekly summary is opt-in through `reminders.weekly_summary_enabled`, and the monthly one was first built hanging off the same flag — which meant switching off the weekly silently switched off the monthly too. Making it unconditional looked like deleting that one predicate from `ListMonthlyDue`, and that would have been wrong: the query reads `reminders`, and **5 of the 7 production users have no row in that table at all** (a row is only created when someone configures a reminder or opts into the weekly). Dropping the filter alone would have left the "mandatory" monthly reaching two people. It now `LEFT JOIN`s `reminders` from `users`, and `SetLastMonthlySummaryOn` upserts the row when it is missing. That upsert writes **one column**: reusing the existing `Upsert`, whose `DoUpdates` list covers `enabled` and `weekly_summary_enabled`, would have reset both flags on every monthly send — switching off the daily reminder of every user who had one.
- **`BuildMonthly` returns a `conversation.Prompt`, not the `string` its weekly sibling returns (2026-08-30).** The weekly's `Build` returns text and the sweeper ships it with `messenger.SendText`, which is documented as the no-buttons helper. The monthly carries a WebApp button (it deep-links the Mini App into the reported month), so it cannot go through `SendText` at all — the sweeper calls `chat.Send(ctx, prompt)` directly. This forced `internal/summary` to import `internal/conversation`, verified cycle-free: `conversation`'s only intra-repo import is `internal/database`.

- **The monthly summary prices a day, and reuses the Mini App's “Por día” rule (2026-09-02).** Two accounts asked for it 23 times over four days (six ending in `query_failed`): what a day of living cost is the unit a salaried or freelance reader thinks in, and no surface printed it. `perDayLine` divides by `daysInMonth(to)` rather than the Mini App's `min(period end, today)`. The two rules agree here by construction — the sweeper only ever builds a month that has already closed, so its elapsed days ARE its calendar days; `TestBuildMonthly_PerDayDividesByEveryDayOfTheClosedMonth` names that precondition, because a caller passing an in-progress month would spread the spend over days that have not happened and understate it. **Per-category per-day was rejected.** Prorating a once-a-month debt payment is correct — it accrues daily even though it is paid on one day — but prorating a one-off purchase invents a daily cost that will not repeat, and telling the two apart needs a recurrence flag the taxonomy does not have. One aggregate number sidesteps the classification entirely; the per-category table stays in the Mini App, where it is scanned rather than read.

- **The daily reminder names the user's recurring expenses instead of changing when or how it asks (2026-09-12).** Measured over the full `movements` table: the two active reminder users load on 93% and 70% of days, and the sweeper already skips any user who loaded that day — so **adaptive timing was rejected**: moving the hour rescues no empty day, it only moves the nudge. **Tap-to-record buttons were rejected** too: no layer can ask for a missing amount (`movement_create` has no amount step, the guard refuses zero with `ErrZeroAmount`), and building one to save two taps on an event that fires a few times a week was not worth a new step on the money path; `create_failed` had not fired since 2026-07-30, so there was no live hole to justify it either. What shipped keeps the fifteen `PickMessage` bases and appends up to three descriptions from the last 30 days. **The floor is 2 occurrences, not 3**: with 3, user 3 — the one who actually receives the ping — had zero qualifying groups; with 2 he has three. Descriptions are grouped with `unaccent(lower(btrim(...)))` because `café`/`cafe` and `verdulería`/`verduleria` were separate groups in production, splitting the strongest signal in half. ARS only: no USD description repeated in the window.

## The Mini App

### "Por día" divides by elapsed days, not by the length of the month — 2026-09-02

The Categories table's `%` column was replaced by a per-day cost. A percentage
answers "what share", which is not the question users ask out loud; "how much is my
day costing me" is.

The divisor is calendar days from the period's start through `min(period end,
today)`, one rule for all four presets. Two alternatives were dropped:

- **Nominal length of the period** (30 days for an open September). Comparable
  month to month, but on the 2nd it reports a daily cost spread over 28 days that
  have not happened yet — it understates by construction.
- **Branching on "is the anchor the current month?"** Identical numbers, because a
  closed period's `min(end, today)` IS its last day. Rejected as an implementation,
  not as a rule: two code paths that have to agree forever.

The consequence is the intended reading: Total grows with the period while Por día
flattens. 3M reports what a day cost on average that quarter, not what a day cost
in a month.

`Period.PerDayNote` prints the day count it divided by, taken from the same `now`
and the same window as `Period.DaysElapsed`. The sentence and the column cannot
drift, because a bug in one is a bug in the other.

**Known limitation.** Over 6M and Año the average mixes pesos of different
purchasing power: a peso from last October is not a peso from today, so the figure
understates what a day costs now. Deflating by CPI is parked work; the number is
useful without it, and the limitation is recorded here rather than papered over in
the UI.

## Package layout, metrics and tooling

- **Repo-specific subagents over generic ones for locate/edit/review.** `.claude/agents/repo-investigator`, `repo-builder`, `repo-reviewer` mirror the caveman plugin's `cavecrew-*` pattern (narrow tool grants, hard scope refusal, compressed output) but carry this repo's own rules — money type, currency handling, `conversation_states` access, closed taxonomy — baked into their prompts. Use them instead of generic `Explore`/`general-purpose`/`code-reviewer` agents for work scoped to this repo. `repo-builder` enforces the completion-evidence rule (`CLAUDE.md` §7) at the agent level: it will not return a receipt without a clean `bash check.sh`.
- **Métricas end-to-end por outcome, no por confidence autoreportada ni confirmación forzada.** Se mide '¿el usuario logró lo que quería?' con la señal que los gates confirm/cancel existentes ya producen + el outcome del terminal de cada flow, correlacionado por la invariante WIP=1 (último pending). No se agregó `confidence` (cambio de prompt en la ruta caliente + autoreporte débil) ni confirmación a CREATE frictionless (rompe el principio 'CREATE sin fricción'); el feedback de CREATE frictionless se infiere del follow-up UPDATE/DELETE, que ya se loguea.
- **`internal/controller/messaging` was split by cluster, one package per reason to change.** It had grown to ~10k non-test lines across 52 files — 4× the next package — and go.dev's module-layout guidance says the answer is to split off supporting packages under `internal/`, not to document around it. Done in stages, each a pure move verified by the full test protocol: copy→`messages`, flows→`flow`, the unified loop→`agent`, free-text reads→`query`, contextual tips→`nudges`, the 429 queue→`pendingjob`, the configuration wizards→`settings`. The edge kept 1.6k lines across 20 files: webhook handlers, `/start`, the bridges, `userLocks`, tracing, metric outcomes.
  The seam that made it work is the **consumer-defined interface**: each cluster declares the narrow set of methods it needs (`flow.runner`, `agent`'s `agentServices`, `settings.Services`, …) and `*controller` implements all of them structurally through one-line bridge files. No cluster imports a repository, and none imports another cluster's internals. The methods are exported although most interfaces are not — an interface with unexported methods can only be satisfied from inside its own package, and the implementation is at the edge.
  Two things were deliberately *not* moved. The wizard **starts** stay out of `flow` because they call the LLM and `flow` must not import `orchestrator` — they went to `settings`, which `flow` reaches back through two runner methods. And the tests that exercise a finish through the `*controller` stayed at the edge: they test webhook→engine→finish, which is the edge's job. Using `flow` does not make a test a `flow` test — only the builder/step tests moved.
- **`messages` was later dissolved (2026-09-02):** almost every constant in it had exactly one
  reader, all in `agent`. Inlined or folded into `agent`/`settings` — see that commit for the
  breakdown.

### Why `ArgentinaZone` is a fixed offset and not `time.LoadLocation` (2026-08-31)

`constants.ArgentinaZone` is `time.FixedZone("ART", -3*60*60)`. `time.LoadLocation` would read
the IANA tz database from the host, and the Alpine/scratch images this deploys to ship without
it — the lookup fails at runtime, not at build. Argentina observes no DST, so a fixed offset
loses nothing. If DST ever comes back, the switch is `time.LoadLocation` **plus** an embedded
`time/tzdata` import, not `LoadLocation` alone.

`TestArgentinaZone_IsUTCMinus3` is what keeps the offset honest.

### Why `govulncheck` gates CI while `errcheck` does not (2026-09-19)

CI runs `check.sh` and `govulncheck` as gates; `errcheck` stays informational inside `check.sh`.
The difference is who introduced the finding. `errcheck`'s ~215 findings are debt no change
added, so gating on them fails every PR for someone else's code and teaches everyone to ignore
the exit code. A reachable vulnerability is a live defect in what ships, and a non-blocking job
on a one-person repo is read by nobody — informational was rejected for that reason.

Gating on day one was also rejected: the first `govulncheck` run (2026-09-19, go1.26.4) found 9
reachable vulnerabilities — 7 in the standard library, one each in `quic-go`, `x/net` and
`x/text`, all through the network paths (`orchestrator`, `quote`, `server`). A CI born red
teaches that red is normal, so the bump to go 1.26.6 and the three modules merged first
(PR #96) and the gate came after, green.

`govulncheck` is pinned to a tool version so the tool never changes under a PR. Its
vulnerability database cannot be pinned: a new entry can turn an unrelated PR red. That is the
gate working, not a flake.

