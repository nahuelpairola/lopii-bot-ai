# internal/settings

The configuration wizards: everything reachable from the agent's `manage_settings` tool —
accounts, creating a category, removing or merging one, reminders. `Dispatch` picks the area and
the specialist reads the repos, asks the LLM, seeds a `flow` and starts it. **Use
`codegraph_explore` for structure** — this file is only for what reading the code will not tell you.

## Why this is a package and not part of `flow`

`flow` must not import `orchestrator`. These starts all call the LLM, so they cannot live there —
and they cannot live in `agent` either, because they are the app-side UI the loop parks *into*.

This is the **only** caller of `ResolveAccountManage`, `ClassifyOnboarding` and
`ClassifyCategoryCreate`. If a second one appears, that is the moment to ask whether it really
belongs here.

`flow` reaches back for exactly two things, through the runner: `StartAccountCreate` and
`SuggestMergeTarget`. Both need the LLM; nothing else crosses.

## A 429 is answered before the fallback, never after

Every LLM call here has a fallback for when the model is unhelpful — the 7-step wizard, the manual
picker. **The rate-limit check goes first.** Skip it and a quota problem disguises itself as "no te
entendí": the user gets charged seven questions for something that would have resolved itself in
seconds, and the message that was supposed to be queued is gone.

The shape to copy is in `StartSubcategorySetup`: `HandleGroqError` first, and only then the wizard.

## Never trust an id the model returns

`StartAccountManage` gets a `MatchedAccountID` from the LLM and looks it up in the user's own list
before seeding it. A hallucinated id must degrade to the picker, never select someone's account.
`SuggestMergeTarget` does the same twice over: it drops a match that resolves to the source row
itself, and one that resolves to nothing.

`SuggestMergeTarget` returns nil for *any* doubt — error, timeout, a proposal where a match was
asked for. nil means "no suggestion" and the flow falls back to the manual picker. It never blocks.

## An account-manage message is read by Go, and the model is only the fallback

`StartAccountManage` gets three things from the text (`account_match.go`) before trusting the
model: the account, whether it is a balance adjustment, and the stated total. Asking the model
for the last two was tried and measured: it left them empty on every real phrase. The model's
`MatchedAccountID` survives only as the fallback for an alias the text does not spell out.

- The account match is on **whole words with no minimum length**. `flow.TokenCoverage` skips
  tokens under four runes, so reusing it makes "FCI" unmatchable. Whole words are also what keeps
  `fcis` from matching `FCI` and `pago de monotributo` from matching `Mercado Pago`.
- **No currency cue resolves to ARS**, on purpose (`docs/decisions.md`). Same name *and* same
  currency stays ambiguous and opens the picker: those duplicates exist in production.
- **The total is accepted only in an unambiguous shape** (`amountShapes`). US format
  (`3,605.24`) is dropped because `movement.ParseARAmount` would read it as 3.60524; a number
  followed by `mil`/`k`/`lucas`, two different numbers, or a negative also drop it. A dropped
  total still counts as "a number was stated", so the flow asks for the amount instead of
  showing the menu. Tokens with `/` or `:` are dates or times, never amounts, and a number that
  is part of an account name is never the total.
- **The rename/default/create/delete stems win over the adjust stems.** "Cambiar nombre" carries
  no adjust stem, but "nueva cuenta FCI en USD 3614,66" carries a number: without the block it
  would open on an adjustment confirm for an account that was meant to be created.

## Two guards that exist because of a real dead end

- **`StartCategoryManage` checks for owned categories first.** With none, the picker would show only
  "Cancelar" — a dead end dressed as a flow. It also **resolves the metric**: the bot understood and
  answered correctly, and without that the event stays pending and the sweeper marks it
  `abandoned`, which reads as a bot failure. That happened for real.
- **The taxonomy sent to the LLM excludes reserved categories and the source row**, or the model
  matches a row to itself.

Why: `docs/decisions.md`, section **Conversation engine and flows**.
