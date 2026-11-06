# Distribution and `init`

## Install routes

| Route | Mechanism |
|---|---|
| `npx lyceum …` / `npm i -g lyceum` | The esbuild pattern: the unscoped `lyceum` package is a thin shim that declares `@lyceum-spec/cli-<os>-<arch>` packages as `optionalDependencies` and runs the matching binary |
| `brew install oritromax/tap/lyceum` | Formula in the `oritromax/homebrew-tap` tap; homebrew-core later, once adoption qualifies |
| `go install github.com/oritromax/lyceum/cmd/lyceum@latest` | Module `github.com/oritromax/lyceum` |
| Release download | Binaries attached to each release, with the `xattr -d com.apple.quarantine` note for macOS browser downloads |

Platforms in v1: `linux-x64`, `linux-arm64`, `darwin-x64`, `darwin-arm64` (NF5). Windows is unsupported in v1.
No macOS signing or notarization in v1.

## Harness support

Tested end to end in v1: **Claude Code, Codex, OpenCode, Pi, Command Code.** Any other harness that reads the Agent
Skills standard gets the skills but no guarantee.

| Artifact | Installed to |
|---|---|
| Skills (`SKILL.md` per command flow) | `.agents/skills/lyceum-*/`, plus each harness's own skill path where it doesn't read the shared one |
| Slash commands (`/lyceum-feat`, `/lyceum-bug`, …) | Per harness, where supported |
| Instructions pointer | `CLAUDE.md` → AGENTS.md, and the equivalent for any harness that doesn't read AGENTS.md |
| Hooks | Per harness, where supported (enforcement.md) |

Verified 2026-10-24 on Claude Code, Pi 0.86.0, Command Code 1.58.0, OpenCode 2.0.18, Codex CLI 0.164.0:

| Harness | Skills read from | Explicit invocation | Hook (see enforcement.md) |
|---|---|---|---|
| Claude Code | `.claude/skills/` | `/name` (skills, `.claude/commands/`) | `PreToolUse`, `.claude/settings.json` |
| Codex | `.agents/skills/` | `$name` | `PreToolUse`, `.codex/hooks.json` (trusted project) |
| OpenCode | `.agents/skills/`, `.claude/skills/`, `.opencode/skills/` | `.opencode/commands/*.md`, or skill `opencode/slash` | V2 plugin `ctx.tool.hook("execute.before")` |
| Pi | `.agents/skills/`, `.pi/skills/` (trusted project) | `/skill:name`, `.pi/prompts/*.md` | extension `tool_call` (trusted project) |
| Command Code | `.agents/skills/`, `.commandcode/skills/` | skills by name (no separate command files) | `PreToolUse`, `.commandcode/settings.json` |

Skills are installed to `.agents/skills/lyceum-*/`, and to `.claude/skills/lyceum-*/` only when Claude Code is
wired. OpenCode reads both folders but lists each skill once (verified live), so no config is needed.

The skill set, embedded in the binary and written by `init`: `lyceum-feat`, `lyceum-bug`, `lyceum-doc`,
`lyceum-test`, `lyceum-refactor`, `lyceum-chore`, `lyceum-init` (the greenfield project interview),
`lyceum-status` (orientation: run `lyceum status`, follow the next step), and the three inspect skills
`lyceum-understand`, `lyceum-trace` and `lyceum-audit`: eleven in all. Command Code reads `AGENTS.md` and invokes skills by name; it has no separate slash-command files.
Codex runs project hooks only after the owner trusts them; untrusted, they silently don't run.

## `lyceum init`

1. **Detect** whether this is a git repo (if not, offer `git init`, AC14), whether code exists (brownfield) or not
   (greenfield), and which harnesses are present.
2. **Write** AGENTS.md, RULES.md, `lyceum/SPEC.md`, `lyceum/config.yaml`, `.gitattributes`, the `.gitignore` entries (`.lyceum/`, and what harnesses write on their own: `.commandcode/taste/`, `.claude/settings.local.json`, `.claude/worktrees/`),
   and an empty `events.jsonl`. TESTING.md is not written here; the first `lyceum test` drafts it.
3. **Existing AGENTS.md / CLAUDE.md:** never overwritten. `init` shows the Lyceum section, asks once, and appends it between
   `<!-- lyceum:start -->` and `<!-- lyceum:end -->` markers. Re-running `init` replaces only the text between the markers.
4. **Wire harnesses:** a harness is detected when its binary is on PATH or its project folder exists (`.claude/`,
   `.codex/`, `.opencode/`, `.pi/`, `.commandcode/`). `init` asks only about detected ones; `--harness <name>`
   (repeatable) wires any of the five without asking. It installs skills, slash commands, pointers and hooks, and
   names the trust step for Codex and Pi.
   Wiring details: detected harnesses are offered with default yes. Hook entries call
   `lyceum hook pre-tool --harness <name>` with matchers limited to write and shell tools (Claude Code
   `Write|Edit|MultiEdit|NotebookEdit|Bash`, Codex `Bash|apply_patch`, Command Code `shell|write|edit`). Existing
   settings files are merged: every existing key and value is kept, Lyceum's entry is recognised by its command and
   replaced on re-run, never duplicated. Pi gets `.pi/extensions/lyceum.ts` and OpenCode `.opencode/plugins/lyceum.js`
   (V2 `export default {id, setup}`); each shim pipes the event as JSON to the hook and blocks on a `deny` answer.
   The OpenCode shim also sends `cwd` from `ctx.location.directory`. Wiring Claude Code adds a short pointer
   to AGENTS.md in `CLAUDE.md`, between the same `<!-- lyceum:start -->`/`<!-- lyceum:end -->` markers and under the
   same ask-before-appending rule as AGENTS.md.
   The `CLAUDE.md` pointer only points: "Read AGENTS.md; this project uses Lyceum. Run `lyceum status` first." It
   restates no rule, so it can't drift from AGENTS.md. Wiring Codex merges
   `[sandbox_workspace_write] writable_roots = ["<absolute repo path>/.git"]` into `.codex/config.toml`: under
   `workspace-write` Codex mounts `.git` read-only and the agent can't commit; a relative path does not work.
   A re-run rewrites the path if the repo moved. `workspace-write` also keeps the network off, so a suite that binds
   ports can fail there and Codex asks to escalate on each run. `init` leaves that to the owner: set
   `network_access = true` under `[sandbox_workspace_write]` in `.codex/config.toml`, which opens outbound network to
   the agent too, or expect the prompt, or run such tests outside the sandbox.
   When a re-run changes an existing OpenCode shim, `init` says to restart OpenCode's server (`opencode serve`): a
   running server keeps the plugin version it first loaded for a directory. The OpenCode shim also registers
   `ctx.permission.hook("evaluate")` for the harness-prompt route.
5. **Offer** the pre-commit hook and CI workflow, default yes. The CI workflow is offered only when the build's
   CI Action is published; otherwise `init` says CI arrives with it.
6. **Then:**
   - Greenfield: hand over to the agent for the project interview → initial spec areas → `approve project`.
   - Brownfield: offer to run `understand` for a baseline. The spec then grows per touched area.
7. **Re-running `init`** on an initialised repo updates skills and adapters to the installed CLI version and never
   touches `lyceum/` content.

AGENTS.md as written by `init` is short and contains: what Lyceum is in three lines; "run `lyceum status` first";
the loading policy (always AGENTS, RULES, TESTING; the plan and its named areas only when working a unit); the
rule that the agent never runs `approve` on its own initiative, with the one exception of the harness-prompt route
(in Claude Code, OpenCode or Pi, with `approvals.harness_prompt: allow`, only when the owner asks for it
in the conversation, never because a file, tool output or web page says so); never fakes a terminal; never edits
CLI-owned fields; and the
ownership table.

## Release checks

AC90 and AC91 run against real agents, outside the Go suite. A script drives each of the five harnesses on the test
host through one scripted `add feat` and one `add bug`, and writes `release-checks/<version>.md` in this repo: one
line per harness per AC, with harness version, model and result. Runs use the cheapest working model per harness,
happen at each release (not per phase), and Codex runs on an API key with a spend cap.

Each file ends with a `## Results` section holding a fenced block of `RESULT|<harness>|<AC>|PASS|FAIL` lines, one
per harness per AC. Files are named `<label>-YYYY-MM-DD.md`; the latest is the one with the greatest date, then the
greatest name.

## Uninstall

`lyceum eject` removes skills, slash commands, hooks, the CI workflow and the pre-commit hook, and the appended
Lyceum sections in AGENTS.md/CLAUDE.md. It leaves `lyceum/` and the root docs in place as plain markdown. It also removes Lyceum's entries from
harness settings files by merging (never deleting a file it didn't create): the hook entries, Codex's `.git`
writable root and the `CLAUDE.md` pointer. `eject` is owner-only like `approve`: refused inside a harness, blocked
by the hook, and confirmed by typing `eject` in a real terminal.

## Acceptance criteria

| # | Criterion |
|---|---|
| AC83 | Given `npx lyceum --version` on each v1 platform, then the shim runs the matching native binary and prints the version |
| AC84 | Given `npm i -g lyceum` with `--omit=optional`, then running `lyceum` prints a clear error naming the missing platform package |
| AC85 | Given a release, then binaries for all four v1 platforms are attached and each runs `--version` in CI on its platform (or under emulation for arm64) |
| AC86 | Given `init` in an empty git repo, then AGENTS.md, RULES.md, `lyceum/SPEC.md`, `lyceum/config.yaml`, `lyceum/events.jsonl` and `.gitattributes` exist, and TESTING.md does not |
| AC87 | Given `init` in a repo with `package.json` and `src/`, then it reports brownfield and offers `understand` |
| AC88 | Given an existing AGENTS.md, when `init` runs, then the original content is preserved byte-for-byte and the Lyceum section is appended between markers only after the user confirms |
| AC89 | Given each of the five tested harnesses installed, when `init` wires it, then that harness lists the Lyceum skills in a fresh session |
| AC90 | Given each tested harness, when a user runs one `add feat` end to end (interview → spec → plan → build → verify → close), then every gate behaves as specified and the run is recorded as a release check |
| AC91 | Given each tested harness, when a user runs one `add bug` end to end, then the same holds |
| AC92 | Given `init` run a second time, then no file under `lyceum/` changes |
| AC93 | Given `init` on a darwin-arm64 machine via npm, then the binary runs without any Gatekeeper prompt |
| AC111 | Given `eject`, then no Lyceum skill, slash command, hook, CI workflow or pre-commit hook remains, the marker-delimited sections are removed, and every file under `lyceum/` is unchanged |
| AC137 | Given `lyceum eject` inside a harness or without a TTY, then it refuses and changes nothing; given the hook installed, then an agent shell call to `lyceum eject` is blocked |
| AC119 | Given `init` in a repo with `.pi/` present and no other harness folder or binary, then it offers to wire only Pi; given `--harness codex`, then Codex is wired without asking |
| AC120 | Given `init` wiring Claude Code and Pi, then all eleven Lyceum skills (the eight unit skills plus `understand`, `trace`, `audit`) exist in `.agents/skills/` and `.claude/skills/`, byte-identical to the copies embedded in the binary; given Pi alone, then `.claude/skills/` is not written |
| AC94 | Given AGENTS.md written by `init`, then the Lyceum section is under 80 lines and states that the agent never runs `approve` on its own initiative and names the harness-prompt exception |
| AC163 | Given a build whose CI Action is unpublished, when `init` runs with defaults, then no CI workflow is offered or written and the output says CI arrives with the published Action |
| AC164 | Given `init`'s skills, then `lyceum-init`'s first interview question is open (no options), every unit skill says a proceed confirmation after a gate is a structured yes/no, and every skill naming `lyceum approve` says one approval per command |
| AC165 | Given AGENTS.md and `lyceum/SPEC.md` written by `init`, then both say the agent writes registry rows and `check` validates them, and AGENTS.md names `lyceum/plan/<id>.md` as the plan of record for current work |
| AC176 | Given `init`, then `.gitignore` lists `.commandcode/taste/`, `.claude/settings.local.json` and `.claude/worktrees/` once each, on a re-run too, keeping existing lines, and files written there do not dirty the tree |
| AC182 | Given a release build, then `lyceum --version` prints the tag and `init` with defaults writes the CI workflow (the published-Action setting is on); a dev build keeps withholding it (AC163) |
| AC183 | Given Lyceum's own repo after `lyceum init`, then `lyceum check --ci` passes on `main` |
