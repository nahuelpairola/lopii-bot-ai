# CLAUDE.md — lopii-finance-bot

@AGENTS.md

Everything above is tool-agnostic and lives in `AGENTS.md` so Codex, Cursor and the rest read the
same rules. **Edit `AGENTS.md`, not this file**, unless the rule is specific to Claude Code.
Below is only what is.

## Session rules

- **Subagents run on `sonnet`.** The three agents in `.claude/agents/` pin it in their own
  frontmatter (they were pinned to `haiku` until 2026-08-19, silently overriding this rule).
  Any *other* `Agent` call passes `model: "sonnet"` explicitly, unless the user asks otherwise.
  Exception: `subagent_type: "fork"` always inherits the parent's model — a `model` override is
  ignored, so this rule does not apply to forks.
- **Codegraph overrides skill-default exploration.** Any superpowers step that says "explore the
  codebase", "read relevant files", or spawns an `Explore`/`general-purpose` subagent for code
  lookup uses `codegraph_explore` instead — one call returns verbatim source plus the call graph,
  against dozens of `Read`/`Grep` round-trips. This applies *mid-skill* (brainstorming,
  writing-plans, systematic-debugging), not just to standalone questions: skills do not know
  codegraph exists, so the substitution has to be made by hand every time.
- **Codegraph does not load a package's `AGENTS.md`.** Each of the fourteen packages listed in
  `AGENTS.md` keeps its traps in `internal/<pkg>/AGENTS.md`, with a one-line `CLAUDE.md` beside it
  that imports it. Nested files load when Claude Code *reads* a file in that subtree — and the
  rule above means it often does not. Open the package's file deliberately: nothing does it for
  you, and those files hold what compiles fine and behaves wrong.
- **Stack mandate, always.** Codegraph before manual grep/read. The matching superpowers skill
  (brainstorming / systematic-debugging / writing-plans / TDD) before any feature or fix. Ponytail
  discipline on every diff. Caveman-compressed output. None of these are skippable for a "simple"
  task — that rationalization is exactly what each skill already warns about.
- **WIP=1 for delegated work.** One flow or fix active per subagent at a time. Do not start a
  second before the first has completion evidence. Parallel activation splits the reasoning budget
  and nothing finishes properly.
- **Completion evidence, not self-assessment.** Nothing reports "done" without having run
  `bash check.sh` and shown the output. `check: OK` is the only valid signal — never the agent's
  own reading of its diff. The script already excuses the one expected red by name, so a red from
  it is a real red.

## Project subagents

Use `repo-investigator`, `repo-builder` and `repo-reviewer` (`.claude/agents/`) for locate, edit
and review work in this repo instead of generic `Explore`/`general-purpose`/`code-reviewer` — they
carry this repo's rules (money type, currency handling, `conversation_states` access, taxonomy)
built into their prompts.
