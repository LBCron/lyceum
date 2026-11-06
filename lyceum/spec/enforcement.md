# Enforcement

**The honest claim:** local checks catch a sloppy or over-eager agent. CI plus branch protection catches
everything else. Nothing that runs locally as the same OS user is absolute, and the docs say so plainly.
Run records in `.lyceum/runs/` are local and untracked, so a hand-written `gate-clear` event is not checked against
them; CI's protection is that `check --ci` and the CI job re-run the suite themselves.

## Layers

| Layer | Mechanism | Strength |
|---|---|---|
| 1 | AGENTS.md, RULES.md and skills: stop at gates, never approve, never fake a terminal | Prose |
| 2 | CLI refuses illegal transitions (e.g. `close` before `verify`) | Stops honest overreach |
| 3 | `approve` is human-only: TTY, typed confirmation, harness env refusal | Friction |
| 4 | Approvals bound to file hashes; `check` recomputes them | Detects edits after approval |
| 5 | Approving the gate that opens code (`spec` for feat, `plan` for refactor/chore) refuses a dirty tree outside `lyceum/` | Catches code written before approval |
| 6 | `check` in pre-commit (warn) and CI (fail) | The wall, with branch protection |
| 7 | Harness hooks restrict writes to the plan's `files:` and `lyceum/` while a build gate is open | Best UX where supported |
| 8 | Signed approvals (opt-in): an approval counts only in a commit signed by the owner's key | Effectively unforgeable |

## `lyceum approve [gate]`

1. **Bare `approve`** lists every pending judgment gate for the current unit (plus `testing` if TESTING.md is
   unapproved or changed) and walks through them in one session.
2. **Refuses** when any known harness environment variable is set. This and the TTY check run before anything else,
   so an agent always gets the plain "approval is for the owner" refusal, whatever gate it names.
3. **Refuses** unless stdin and stdout are TTYs. List (harness research 2026-10-24): `CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT` (Claude Code),
   `CODEX_THREAD_ID`, `CODEX_CI` (Codex), `OPENCODE_TERMINAL` (OpenCode), `PI_CODING_AGENT` (Pi),
   `COMMANDCODE_SCRATCHPAD` (Command Code), and the generic `AI_AGENT`. Command Code sets no deliberate marker, so its
   entry is the best available and may change between releases; the Codex names were confirmed live. Extended as discovered.
4. Shows a summary: unit, gate, files to be approved with their sha256, and the diff since the last approval of this gate.
   For `cause`, the summary also shows the repro test, its recorded failing output and the fix diff.
5. Requires the user to type the gate name exactly. The prompt names the exact text it accepts. In a bare walk,
   a refused confirmation stops the walk at that gate with one refusal, not one per remaining gate.
6. For the gate that opens code (`spec` for feat, `plan` for refactor and non-trivial chore): refuses if `git status` shows changes outside `lyceum/`, and lists them.
7. Writes one `approve` event: `unit`, `gate`, `hashes` (sha256 per approved file), `commit`, `by` (git email),
   `at`, `parent` (parent process name). Mirrors it into the plan frontmatter.
   The approval commit contains the event, the plan and every approved file, so what was approved is what is committed.
8. If `by` is not the configured owner, the event is written with `counts: false` and the gate does not clear.
9. With `approvals.signed: true`, also creates a signed commit containing the event; `check` verifies the signature
   against the owner's key.
10. Every harness or TTY refusal says to run the command in a separate terminal the agent doesn't control, and names
   the in-agent shell (e.g. Claude Code's `!` prefix) as a place it doesn't work.
11. `approve testing` parses TESTING.md with the same parser `lyceum test` uses, before the summary. An unparseable
   file is refused with every error listed in one output, and no event is written.
12. The summary for `spec`, `project` and `cause` lists every line in the hashed files that carries a provenance
   marker (`[AGENT-PROPOSED`, `[DERIVED`), so the owner sees which text is the agent's and not his.
13. `approve cause` refuses when the bug's plan names an AC that no area file defines, and its event hashes
   the area files those ACs are written in.

### Harness-prompt approval

A convenience route for harnesses whose hook can force a human prompt the agent can't answer: Claude Code
(`permissionDecision: "ask"`), OpenCode 2.x (`ctx.permission.hook("evaluate")` setting `effect: "ask"`) and Pi (the
shim calls `ctx.ui.confirm`; with no UI, `hasUI` false, it denies). Codex and Command Code have no such prompt (Codex
parses `"ask"` but runs the tool anyway), so their hooks keep denying `lyceum approve`.

1. The agent runs `lyceum approve <gate>`. The hook sees it and, when `approvals.harness_prompt` is `allow` and the
   harness supports a prompt, answers "ask" instead of deny, with a reason naming the unit, gate and files.
2. Before answering, the hook writes a one-use grant `.lyceum/approve-grant.json` (local, gitignored): unit, gate,
   harness, the exact command, the harness permission mode when the payload has one, `at`, and a 2-minute expiry.
3. The human confirms in the harness's own prompt; the command runs. `approve` skips the env and TTY refusals and
   the typed confirmation only when a matching, unexpired grant exists, consumes it, and runs every other step
   (summary printed, dirty-tree refusal, owner check, hashes, commit).
4. The event records `via: harness-prompt`, the harness and the permission mode; terminal approvals record
   `via: terminal`. In auto-approve modes (Claude Code `--dangerously-skip-permissions`, yolo) no prompt appears;
   that is the user's choice and is not blocked, but the event still says so. In Claude Code's `auto` permission mode the hook's "ask" still shows the
   prompt (verified live); the event records `permission_mode: auto`.

Hook answers for the route: Claude Code gets `permissionDecision: "ask"` with the reason; the Pi and OpenCode
shims get `{"decision":"ask","reason":"…"}`. The Pi shim then calls `ctx.ui.confirm` with the reason and blocks unless
the human says yes (and blocks when `ctx.hasUI` is false). The OpenCode shim lets the call through `execute.before` on
"ask" and its `ctx.permission.hook("evaluate")` (action `shell`, command in `resources[0]`) asks the hook again and
sets `effect` to `ask` or `deny` with the reason as `message`. A grant matches when its unit and gate equal the ones
`approve` resolves, it is unexpired, and the environment carries the grant's harness marker.

The CLI cannot prove a human clicked: the grant is a file the agent could write. The route is friction-reducing,
not a security boundary. `approvals.harness_prompt: deny` turns it off, and `check --ci` then fails on a
counting approval with `via: harness-prompt` for the unit on the branch being checked; approvals of other units stay
as they were.

The approved files per gate:

| Gate | Files hashed |
|---|---|
| `spec` | The plan file + every area file it names + `lyceum/SPEC.md`, whose registry rows and AC ranges a feat updates (this one approval covers the plan too). A plan file is hashed, at every gate, without its `## Progress` and `## Log` sections, which the agent keeps updating, and without `files:`, the planned scope |
| `project` | Every registered area file + `lyceum/SPEC.md`, so the owner approves the registry the agent wrote at greenfield `init` |
| `plan` | The plan file |
| `intent` | The plan file's intent only: the `## What and why` section and the `goal:` and `benchmark:` fields, so writing Steps and `public_surface:` before `approve plan` does not break it |
| `cause` (bug) | The plan file + every file in the fix diff |
| `testing` | TESTING.md |
| `tests` (refactor at `verify`; bug between `repro` and `cause`) | Each test file that differs from its fingerprint |

The `testing` gate is repo-level: `approve testing` works with or without a unit and its event has an empty
`unit`; it is pending when TESTING.md has no counting approval or its hash changed since the last one; re-approving
an unchanged TESTING.md is allowed and records a new event. With `--json`, a bare `approve` walk prints exactly one
final object.

## Refusals

Every refusal from any command prints a one-line plain-language reason and the exact command or action that fixes
it. Example: `Can't approve yet: src/a.go has changes made before the plan was approved. Review them, then commit
or stash: git stash`. The agent repeats it in plain words.

## `lyceum check`

Runs locally (and from the pre-commit hook) as a warning, and with `--ci` as a hard failure. The pre-commit
hook never blocks a commit: it prints whatever `check` prints, findings or a refusal, and exits 0. It runs
`lyceum check --pre-commit`, an internal flag; a hand-run `check` reports and writes no `bypass` event.

| Check | Fails when |
|---|---|
| Approval integrity | A hashed file differs from its hash in the approval relied on for it: per file, the latest counting approval whose hashes include that file, whichever gate or unit wrote it. A unit's `approve spec` therefore covers the area files it hashed, and the earlier `project` approval is no longer relied on for them. Drift is a pending re-approval, listed by `status`, and writes no `bypass` event |
| Stale evidence | A feat, refactor, planned chore or doc whose `verify` cleared has a later commit changing a code file; the run no longer describes HEAD. Local `check` warns, `--ci` fails, and `close` refuses until a gate-evidence run at HEAD passes. Bugs keep the Reopen rule |
| Registry | An area file in `lyceum/spec/` has no registry row in `lyceum/SPEC.md`, or a row's AC range omits an AC defined in its file (warning locally, `--ci` fails) |
| Unclaimed ACs | An AC in `lyceum/spec/` that no closed plan lists in `acs:` and no `lyceum/LIBRARY.md` entry names (warning only, locally and in CI; an AC only in `acs_partial:` counts as unclaimed) |
| CLI-owned fields | Plan frontmatter disagrees with the event log |
| Gate before code | The branch diff touches code paths (per the code-path rule below) while the unit's code-opening gate is not cleared: `spec` for feat, `repro` for bug, `plan` for refactor and planned chore. Test files (TESTING.md `test_files`) written during bug `repro` or refactor `baseline` are allowed before it |
| Scope | Changed code files (excluding TESTING.md `test_files`) are outside the plan's `files:` list (warning only; the agent updates `files:`, which no approval hashes, and says so at the next stop). Not run for trivial chores, which the Chore check covers |
| Chore | Diff touches code, a manifest, a lockfile or CI config with no approved plan |
| No unit | Branch has no unit and the diff touches code, a manifest, a lockfile or CI config |
| TESTING.md | Changed since its approval while a mechanical gate relies on it |
| Owner | A gate is cleared by an approval with `counts: false` |
| Reopen | A bug at `cause` has a commit after its verify clear that changes a non-test file outside `lyceum/`; verify must clear again (local `check` writes the `gate-reopen`, `--ci` fails) |
| Stale records | Records whose citations changed (warning only, never a failure) |

"Code, manifest, lockfile or CI config" is any path outside `lyceum/`, `docs/` and `*.md`/`*.txt` at any depth.
CI config such as `.github/workflows/*.yml` counts as code. A `bypass` event records a commit
going past a check: the pre-commit hook writes one per failing finding on committed content or on what the
commit has staged. It is the only writer: `check`, `status`, `record` and `test` report, and a commit Lyceum makes itself
writes none. A finding caused only by changes the commit leaves out, approval drift, TESTING.md drift, and warning-only
checks are reported without one; `bypass` events already written stay as they are. The hook leaves the line
uncommitted in the working tree, and the next Lyceum commit carries it. Clean-tree checks ignore the event log only
when all it differs from HEAD by is `bypass` lines at its end; any other change to the log still counts.

## CI

- A GitHub Action shipped from this repo (`uses: oritromax/lyceum/check-action@v1`) runs `lyceum check --ci` on pull requests. `lyceum check --ci` runs in any CI.
- In CI the base branch is often not a local branch: `check` falls back to `origin/<base>`, and if neither exists it
  refuses with the fetch command as the next step. An unborn base branch (no commits yet, as right after `git init`)
  is an empty base, never a refusal. `check --ci` in a repo where Lyceum is not set up fails.
- `init` offers to install the pre-commit hook and the CI workflow, defaulting to yes. While the build's CI
  Action is unpublished (a build-time setting), `init` neither offers nor writes the workflow and says CI arrives with
  the published Action.
- Docs tell the owner to require the check in branch protection, because without that CI only reports.

## Harness hooks

All five v1 harnesses support a blocking pre-tool hook, and `init` installs one that calls
`lyceum hook pre-tool --harness <name>`: a JSON command hook for Claude Code (`.claude/settings.json`), Codex
(`.codex/hooks.json`) and Command Code (`.commandcode/settings.json`); a small JS/TS shim for OpenCode
(`.opencode/plugins/`) and Pi (`.pi/extensions/`). Codex and Pi load project hooks only after the user trusts the
project, so `init` says so. It blocks writes outside `lyceum/`, `docs/` and the plan's `files:` while
no code gate is cleared, and blocks any shell command that runs `lyceum approve` or `lyceum eject`.
A here-document body fed to `cat` or `tee` and a `git commit`, `tag` or `merge` message are data; text it can't read, or that could run, is matched as a command.
The entry point reads the harness's own pre-tool payload on stdin, decides allow or deny, and answers in that
harness's own deny format; the OpenCode and Pi shims pass the payload through unchanged and act on the answer.
Payload and answer formats per harness are taken from captured live payloads (fixtures in the repo), never assumed.
One entry point serves file writes (path checked against the rule above) and shell calls (`lyceum approve` blocked).
A shell command carrying more than one `lyceum approve` is denied with the reason "one approval per command", and
no grant is written, so one human click always approves exactly one gate.

Agents also write files through the shell (`printf hi > f`); the hook does not parse shell writes, which are left to
`check`.

Hook decision rules:
- A write is decided exactly as `check`'s gate-before-code rule decides one changed path: allowed when it is not
  code (code-path rule), when the unit's code-opening gate is cleared, or when it is a test file at a gate that allows
  test files. The repo is found from the payload's `cwd`; when the payload has none, from the directory of the first absolute
  path in the call; only then from the hook process's own working directory, which may be a long-running harness
  server's. Paths are made relative to the repo root; paths outside the repo, and every write when the repo has no
  Lyceum setup or the branch has no unit, are allowed. The plan's `files:` list stays a `check` warning, never a block.
- Write tools: Claude Code `Write`, `Edit`, `MultiEdit`, `NotebookEdit`; Codex `apply_patch` (every path in its
  `*** Add File:`, `*** Update File:`, `*** Delete File:` and `*** Move to:` lines); Command Code `write_file`,
  `edit_file`; Pi and OpenCode `write`, `edit`. Any other tool is allowed, except shell tools.
- Shell tools (Claude Code/Codex `Bash`, Command Code `shell_command`, Pi `bash`, OpenCode `shell`) are denied when
  the command invokes `lyceum approve` (with or without a path or `npx` before `lyceum`, and with flags between).
- Answers: Claude Code, Codex, Command Code get `{"hookSpecificOutput":{"hookEventName":"PreToolUse",
  "permissionDecision":"deny","permissionDecisionReason":"…"}}` on deny and empty output on allow, exit 0. Pi and
  OpenCode get Lyceum's own shim answer, one line `{"decision":"allow"}` or `{"decision":"deny","reason":"…"}`.
  The reason names the gate and the next step, and tells the agent not to retry.
- A payload that can't be parsed is allowed, with a one-line warning on stderr (hooks are friction). It also writes
  `.lyceum/hook-warning.json` (local, gitignored: harness, time, error); the next successful parse removes it, and
  `lyceum status` shows a warning while it exists, so a harness payload change can't silently disable the hook. An unknown
  `--harness` value is a refusal (exit 2).
The hook entry point must meet NF2.

## Acceptance criteria

| # | Criterion |
|---|---|
| AC39 | Given `lyceum approve plan` with stdin not a TTY, when it runs, then it exits non-zero and writes no event |
| AC40 | Given a TTY and `CLAUDECODE=1` in the environment, when `approve` runs, then it refuses and names the variable |
| AC41 | Given a TTY, no harness variables, and the user types a word other than the gate name, when `approve` runs, then no event is written |
| AC42 | Given a successful `approve plan`, then exactly one `approve` event exists with `unit`, `gate`, the sha256 of the plan file, `commit`, `by`, `at` and `parent`, and the plan frontmatter mirrors it |
| AC43 | Given an approved plan, when the plan body is edited afterwards, then `check` reports that the `plan` approval no longer matches |
| AC44 | Given uncommitted changes to `src/a.go`, when `approve plan` runs, then it refuses and lists `src/a.go` |
| AC45 | Given an approval by an email that is not `owner`, then the event has `counts: false` and the gate does not clear |
| AC46 | Given bare `approve` with two pending gates, when it runs, then both are presented in order in one session |
| AC47 | Given a `feat` at gate `spec` and a committed diff touching `src/`, when `check` runs from the pre-commit hook, then it warns and writes a `bypass` event and exits 0; with `--ci` it exits non-zero (revised) |
| AC48 | Given a branch with no unit whose diff touches `src/main.go`, when `check --ci` runs, then it fails |
| AC49 | Given a branch with no unit whose diff touches only `README.md` and `docs/x.md`, when `check --ci` runs, then it passes |
| AC50 | Given TESTING.md edited after approval, when a mechanical gate would clear, then it does not clear and `check` names TESTING.md |
| AC51 | Given `approvals.signed: true` and an approval commit not signed by the owner's key, then `check --ci` fails |
| AC52 | Given `init` and the user accepts defaults, then a pre-commit hook calling `lyceum check` is installed, and a CI workflow calling `lyceum check --ci` when the build's CI Action is published |
| AC53 | Given each wired harness's hook installed (proven per harness by its captured payload fixture) and a unit at `plan` not approved, when the agent writes `src/x.ts`, then the write is blocked with a message naming the gate |
| AC108 | Given any refusal from any command, then the output contains a reason and at least one concrete next step (command or action) |
| AC54 | Given the hook installed, when the agent runs a shell command containing `lyceum approve`, then it is blocked, unless the harness-prompt route applies |
| AC121 | Given `lyceum approve spec` run with a harness env variable set or without a TTY, then the refusal says to run it in a separate terminal and names the in-agent shell as not working |
| AC122 | Given `approvals.harness_prompt: allow` and a unit at `spec`, when the Claude Code, OpenCode or Pi hook receives a `lyceum approve spec` shell call, then it answers "ask" in that harness's format and writes a grant; when `approve spec` then runs inside the harness with the grant, the gate clears and the event has `via: harness-prompt`; with no grant, an expired one, or one for another gate, it refuses as before. For Codex and Command Code the hook denies |
| AC123 | Given `approvals.harness_prompt: deny`, then the hook denies `lyceum approve` for every harness, and `check --ci` fails on a counting approval event with `via: harness-prompt` for the branch's unit, and passes when such an event belongs only to another unit |
| AC118 | Given a captured pre-tool payload from each of the five harnesses, when `lyceum hook pre-tool --harness <name>` reads it, then a write outside the allowed paths and a shell call running `lyceum approve` each get that harness's deny answer, and an allowed write gets its allow answer |
| AC139 | Given a repo whose base branch is unborn (no commits yet), when `check` runs, then it treats the base as empty and reports findings, never the missing-base refusal |
| AC140 | Given the pre-commit hook installed, when `lyceum check` refuses or reports findings at commit time, then the commit is made and the output is shown |
| AC141 | Given a TESTING.md with two errors the `lyceum test` parser rejects, when `approve testing` runs, then it refuses before the summary, names both errors in one output, and writes no event |
| AC142 | Given the hook installed, when the agent runs one shell command containing `lyceum approve spec` and `lyceum approve project`, then it is denied with a reason naming one approval per command and no grant file exists |
| AC143 | Given bare `approve` with two pending gates, when the confirmation for the first is typed wrong, then the walk stops with one refusal, and the prompt shown named the exact text it accepts |
| AC144 | Given `approve project` covering `spec/orders.md` and a feat whose `approve spec` hashed an edited `spec/orders.md`, when `check` runs, then no approval-integrity finding names `orders.md` and no `bypass` event is written |
| AC145 | Given an approved area file edited and not committed, when `check` runs from the pre-commit hook, then the drift is reported and no `bypass` event is written; given a committed code change before the code-opening gate, then one is (revised) |
| AC146 | Given a feat whose `verify` cleared at commit X, when a later commit changes a code file, then local `check` warns naming the file, `check --ci` fails, and `close` refuses until `lyceum test` passes at HEAD |
| AC147 | Given an area file with no registry row, or a row whose range omits an AC in its file, when `check --ci` runs, then it fails naming the file and the missing AC; locally it warns |
| AC148 | Given `approve spec` on files containing an `[AGENT-PROPOSED: …]` line, then the summary lists that line with its file before the confirmation |
| AC166 | Given `approve project` covering `lyceum/SPEC.md` and a feat that updates a registry range before its `approve spec`, when `approve spec` runs, then the event hashes `lyceum/SPEC.md`, and when `check --ci` runs, then no approval-integrity finding names it |
| AC149 | Given a bug whose plan `acs:` names AC50 and no area file has an AC50 row, when `approve cause` runs, then it refuses naming AC50; once the row is added, the event hashes that area file |
| AC167 | Given an area file whose ACs are bullets `- **ACn** …`, then it is read the same as one with table rows: `approve cause` finds the file of a bullet AC and hashes it, `check --ci` fails on a bullet AC the registry range omits, and release coverage and the unclaimed-AC warning skip a bullet AC marked withdrawn or `(Deferred after v1, …)`, also when the marker sits on a continuation line; an ID that does not lead a row or bullet defines nothing |
| AC169 | Given a committed finding that is true and the hook installed, when `check`, `status` or `record` runs, then the finding is reported, no `bypass` event is written and the tree stays clean |
| AC170 | Given the hook installed and a `feat` at `spec`, when someone commits staged code, then the commit is made and one `bypass` event names that code and no unstaged file |
| AC171 | Given a `feat` at `build` and a committed edit to an approved TESTING.md with the hook installed, then `check` reports the `testing` finding, no `bypass` event is written, and `amend` proceeds |
| AC172 | Given uncommitted `bypass` lines at the end of the event log, when `amend`, `test` and `close` run, then none refuses or loses gate evidence for them and the next Lyceum commit carries them; given any other uncommitted change to the log, then `amend` still refuses |
| AC173 | Given the hook, when a shell command carries `lyceum approve` only as data (a here-document body fed to `cat` or `tee`, a `git commit`, `tag` or `merge` message), then it is allowed and no grant is written; when the text can run (`sh -c`, `bash -lc`, `eval`, after `;`, `&&` or `\|`, in `$( )`, a here-document fed to a shell or piped on, a written file then run), then the hook still stops it |
| AC174 | Given a change to a project-approved file, when `check` reports it, then the to-fix line names the open unit's gate that will hash it: `spec` for a feat whose plan names the area (through `amend` once past `spec`), `cause` for a bug whose branch changes it, and `project` when no unit covers it |
| AC175 | Given a feat at `build` with `spec` approved, when the agent adds a file to `files:` and commits, then `check --ci` has no approval-integrity finding and the scope warning clears; when another agent-written key changes, then it still names the `spec` approval |
| AC178 | Given a clean-tree refusal over an untracked file, then the refusal names the file and a step says to ignore it if a tool writes it; given only tracked changes, then no such step; the file still refuses until it is ignored or removed |
| AC181 | Given a pull request in a repo whose workflow uses `oritromax/lyceum/check-action@v1`, then the Action installs the released binary for the runner, verifies its checksum, and fails the job exactly when `lyceum check --ci` exits non-zero |
