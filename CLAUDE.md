# CLAUDE.md — lopii-finance-bot

@AGENTS.md

@internal/movement/AGENTS.md
@internal/conversation/AGENTS.md
@internal/agent/AGENTS.md
@internal/pendingjob/AGENTS.md

Everything above is tool-agnostic and lives in `AGENTS.md` so Codex, Cursor and the rest read the
same rules. **Edit `AGENTS.md`, not this file**, unless the rule is specific to Claude Code.
Below is only what is.

The four package files imported above are the money path: a movement's sign and account, the
JSONB round-trip that turns a number into `float64`, a turn that wrote being re-enqueued, a job
replayed twice. Ignoring any of them records money wrong and **nothing tells you**. They load at
launch instead of on demand because the on-demand path only fires when something reads a file in
that subtree, and that is not guaranteed. The other ten packages stay on demand — open the file
before editing there.

## Session rules

- **Subagents run on `sonnet`.** The three agents in `.claude/agents/` pin it in their own
  frontmatter (they were pinned to `haiku` until 2026-08-19, silently overriding this rule).
  Any *other* `Agent` call passes `model: "sonnet"` explicitly, unless the user asks otherwise.
  Exception: `subagent_type: "fork"` always inherits the parent's model — a `model` override is
  ignored, so this rule does not apply to forks.
- **Pick the tool by what you are doing, not by a fixed order:**

  | Doing | Tool |
  |---|---|
  | Getting oriented: where is X, what calls Y, how does this flow | `codegraph_explore` — one call returns verbatim source plus the call graph |
  | Editing something you have already located | `Read` the file — it also pulls in that package's `AGENTS.md` |
  | Plain-text search, or anything codegraph does not index | `Grep` / `Glob` |

  This applies *mid-skill* too (brainstorming, writing-plans, systematic-debugging): skills say
  "explore the codebase" or spawn an `Explore` subagent, and they do not know codegraph exists,
  so an orientation step is worth redirecting by hand.

- **Reading a file is how a package's `AGENTS.md` gets loaded.** Ten of the fourteen load on
  demand, and only when something reads a file in that subtree — `codegraph_explore` does not
  count. That is the reason the table above sends an *edit* through `Read`: it is the cheap way
  to arrive with the traps in hand. When you skip it, open the package's file deliberately.
  (The four money-path ones are imported at the top of this file and are always loaded.)
- **Stack mandate.** The matching superpowers skill (brainstorming / systematic-debugging /
  writing-plans / TDD) before any feature or fix. Ponytail discipline on every diff.
  Caveman-compressed output. Those are not skippable for a "simple" task — that rationalization
  is exactly what each skill already warns about. Tool choice is the one thing above that is a
  judgement call, not a mandate.
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
