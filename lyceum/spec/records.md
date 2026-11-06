# Records: understand, trace, audit

Inspect commands answer questions about the project. They never edit code, not even temporary logging,
and they are not units: no branch, plan or gates. Their output is saved automatically under `lyceum/`.
The only question asked about saving is whether to export to `docs/`.

## CLI and agent split

The CLI never reads code for meaning; the agent does. `lyceum understand "<hint>"` and `lyceum trace "<question>"`
create nothing: they list candidate records (spec areas first, then `lyceum/understand/`, `lyceum/trace/`,
`docs/`) with their freshness, and print the measured scope (files, lines) when no record matches. The agent writes
the record with `cites:` paths only, then runs the hidden `lyceum record <file>`, which fills in `id`, `created`
and each cite's `blob`/`tree`, validates the format, and commits that one file and nothing else. `status` lists
drifted and broken records; `check` warns on them. Freshness compares the cited path's current content
hash, so it does not depend on any commit (AC73).

## Record format

```yaml
---
id: 7c01de-payment-flow        # CLI-owned
kind: understand                # understand | trace | audit
title: How payments work
created: 2026-10-23T10:00:00Z
cites:                          # CLI computes the hashes from the paths the agent lists
  - path: src/payments/service.ts
    symbol: createPayment()     # optional; display only
    blob: "git blob sha"
  - path: src/payments/
    tree: "git tree sha"
---
```

The same `cites:` format is used by as-built spec sections. Citations name symbols, never line
numbers.

## Freshness

The CLI recomputes the blob and tree hashes of every cited path and reports:

| State | Condition | What the agent does |
|---|---|---|
| fresh | All hashes match | Summarises the record directly |
| drifted | A cited file changed, or a cited folder's tree changed | Summary plus the changed paths, and an offer to refresh only those parts |
| broken | A cited path no longer exists | Says the record no longer matches the code and offers to redo it |
| missing | No record matches | Works it out from the code |
| invalid | The record or cited area file can't be parsed, or its cites have no hashes yet | Fixes the file, then runs `lyceum record` |

Drift is file-level, so it means "possibly stale", not "wrong" (from review). `check` lists drifted and broken
records as a warning only.

## `lyceum understand [hint]`

1. No hint: the agent asks what the user wants to understand.
2. Lookup: the CLI lists candidate records from `lyceum/spec/`, `lyceum/understand/`, `lyceum/trace/` and `docs/`
   with their freshness. The agent matches the hint semantically. An approved spec section is the preferred source.
3. Found: summary, freshness state, recorded date, and an offer to refresh if not fresh.
4. Not found: if the hint is broad or missing, the CLI sizes the scope first (file count, lines, migrations)
   and the agent asks before proceeding. Then the agent reads the code and answers.
5. The answer is saved as `lyceum/understand/<hash>-<slug>.md`. The agent asks whether to export to `docs/`.
6. Brownfield `init` offers to run `understand`. Its records seed as-built sections later.

## `lyceum trace [question]`

1. Lookup and freshness as for `understand`.
2. The user chooses the mode: code-only or full run.
3. **Code-only:** the agent reads the flow and writes an ordered path. Each step records the symbol, data in and out,
   side effects, branches and failure points.
4. **Full run:** only against an environment defined in TESTING.md; with none, the CLI refuses and offers
   code-only. The agent first does the code read, then states the side effects (e.g. "creates 1 order in
   the test DB") and asks before running. The CLI runs `up`/`seed`, the trigger and `reset`/`down`, recording each.
The agent asks in chat, then runs `lyceum trace --run <env> --yes`; without `--yes` the CLI prints the
   commands and side effects and runs nothing. The environment must be declared in the approved TESTING.md, and every
   command run is recorded in the trace. Each step is marked **read** or **observed**. Mismatches are findings.
5. Steps that existing logs, DB state or responses can't observe stay read-only. The agent may suggest
   instrumentation as a `chore`.
6. **Findings are routed, never fixed:** code contradicts the spec, or the run contradicts the code → offer
   `add bug`; behaviour the spec doesn't cover → flag spec drift, offer `add feat` or an as-built update.
7. Saved to `lyceum/trace/<hash>-<slug>.md`. The agent writes a Mermaid `sequenceDiagram` block in the record;
   `export` copies it and refuses a trace with none. The agent asks whether to export.

## `lyceum audit`

1. The CLI detects ecosystems from manifests and runs each one's standard audit tool (e.g. `npm audit`,
   `pip-audit`, `govulncheck`, `cargo audit`, `composer audit`, `bundle audit`).
2. If a tool is missing, the agent asks the user to install it or to allow the agent to.
3. Report-only: it never changes packages or code. Saved as `lyceum/audit/<date>-<hash>.md`. Like the test
   runner, the CLI parses nothing: the record holds each tool, command, exit code and raw output; the agent adds a
   summary section. A missing tool is a refusal naming how to install it.
4. Each finding the owner acts on becomes a `chore` (upgrade) or `add bug` (defect).

## As-built spec sections

Area files may carry `cites:` frontmatter for as-built sections. Freshness covers them exactly as it covers
records, and `status` lists a drifted or broken area. `lyceum record lyceum/spec/<area>.md` fills in only the cite
hashes (no `id` or `created`) and commits the file. Area files are approved by hash, so this is a spec edit: `check`
then flags the approval that covered the file until the owner re-approves.

## Export

`lyceum export <record> [path]` writes a human-facing copy under `docs/` (default `docs/<kind>/<slug>.md`).
The agent runs it when the user says yes to exporting, so nobody needs to remember it. The record in `lyceum/` stays the source; the export carries a header linking back to it.

## Acceptance criteria

| # | Criterion |
|---|---|
| AC67 | Given `understand`, `trace` or `audit` with any input, when it completes, then `git diff` shows changes only under `lyceum/` (or `docs/` after a confirmed export) |
| AC68 | Given bare `lyceum understand`, then the agent asks what to understand before reading any code |
| AC69 | Given a record whose cited file is unchanged, when freshness is computed, then the state is `fresh` |
| AC70 | Given a record citing `src/a.ts`, when `src/a.ts` changes, then the state is `drifted` and the output names `src/a.ts` |
| AC71 | Given a record citing `src/a.ts`, when it is deleted or renamed, then the state is `broken` |
| AC72 | Given a cited folder, when a new file is added in it, then the state is `drifted` |
| AC73 | Given a record created on a unit branch that was then squash-merged and the branch deleted, when freshness is computed on main, then it still resolves (no dependency on the old commit) |
| AC74 | Given a completed `understand`, then a record exists in `lyceum/understand/` without the user being asked whether to save it |
| AC75 | Given a hint-less `understand` on a repo over a size threshold, then the CLI prints the measured scope and the agent asks before reading. The threshold is `understand.warn_threshold`, default 2,000 files or 200,000 lines |
| AC76 | Given an approved spec area and an `understand` record on the same topic, when the lookup runs, then the spec area is listed first |
| AC77 | Given `trace` full-run with no environment in TESTING.md, then the CLI refuses the full run and offers code-only |
| AC78 | Given `trace` full-run with an environment defined, then the side effects are shown and nothing runs until the user confirms, and every command run is recorded |
| AC79 | Given a full-run trace, then every step in the record is marked `read` or `observed` |
| AC80 | Given a trace finding that the code contradicts the spec, then the agent offers `add bug` and no code file changed |
| AC81 | Given a trace exported to `docs/`, then the exported file contains a Mermaid `sequenceDiagram` block |
| AC82 | Given `audit` on a Node project with `npm` installed, then `npm audit` ran, a report exists in `lyceum/audit/`, and `package.json` and the lockfile are unchanged |
| AC133 | Given `lyceum record <file>` on an agent-written record with `cites:` paths only, then `id`, `created` and each cite's hash are filled in and exactly that file is committed; given an invalid record, then it refuses naming the field |
| AC134 | Given `trace --run <env>` without `--yes`, then the commands and side effects are printed and nothing runs; with `--yes`, then each command runs and is recorded in the trace |
| AC135 | Given `audit` on a Go project with `govulncheck` missing, then it refuses and names how to install it, and no record is written |
| AC136 | Given a record or an area file whose cited file changed, when `status` runs, then it lists the record or area as drifted and names the file |
| AC138 | Given an area file with `cites:` paths and no hashes, when `lyceum record lyceum/spec/<area>.md` runs, then each cite's hash is filled in, no `id` or `created` is added, only that file is committed, and `status` lists the area as fresh; given a record that fails to parse, then `status` lists it as `invalid` with the reason and `check` warns |
