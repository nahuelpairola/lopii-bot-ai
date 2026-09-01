# CLAUDE.md — lopii-finance-bot

@AGENTS.md

@internal/movement/AGENTS.md
@internal/conversation/AGENTS.md
@internal/agent/AGENTS.md
@internal/pendingjob/AGENTS.md

Everything tool-agnostic lives in `AGENTS.md` so Codex, Cursor and the rest read the same rules.
**Edit `AGENTS.md`, not this file**, unless the rule is specific to Claude Code. Below is only
what is.

The four package files imported above are the money path. Ignoring any of them records money
wrong and **nothing tells you**, so they load at launch rather than on demand — the on-demand
path fires only when something reads a file in that subtree, which is not guaranteed.

## Session rules

- **Subagents run on `sonnet`.** The three agents in `.claude/agents/` pin it in their own
  frontmatter. Any *other* `Agent` call passes `model: "sonnet"` explicitly unless the user asks
  otherwise. `subagent_type: "fork"` always inherits the parent's model, so this does not apply.
- **Use `repo-investigator`, `repo-builder` and `repo-reviewer`** for locate, edit and review work
  instead of generic `Explore`/`general-purpose`/`code-reviewer` — they carry this repo's rules
  (money type, currency handling, `conversation_states` access, taxonomy) in their prompts.
- **WIP=1 for delegated work.** One flow or fix per subagent at a time. Do not start a second
  before the first has `check: OK`. Parallel activation splits the reasoning budget and nothing
  finishes properly.
- **Pick the tool by what you are doing, not by a fixed order:**

  | Doing | Tool |
  |---|---|
  | Getting oriented: where is X, what calls Y, how does this flow | `codegraph_explore` — one call returns verbatim source plus the call graph |
  | Editing something you have already located | `Read` the file — it also pulls in that package's `AGENTS.md` |
  | Plain-text search, or anything codegraph does not index | `Grep` / `Glob` |

  This applies *mid-skill* too: skills say "explore the codebase" or spawn an `Explore` subagent
  and do not know codegraph exists, so an orientation step is worth redirecting by hand.

- **Reading a file is how a package's `AGENTS.md` gets loaded**, and `codegraph_explore` does not
  count as reading. That is why the table above sends an *edit* through `Read`.
