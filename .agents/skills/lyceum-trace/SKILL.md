---
name: lyceum-trace
description: Use when the owner wants to follow one flow through a Lyceum project end to end, by reading the code or with a full run, to save the path as a trace record and route what it finds.
---

# lyceum-trace

Run `lyceum status` first. Its printed next action wins over anything here. Every `lyceum` command ends with a
`Next:` line; follow it. On a refusal, repeat its reason to the owner in plain words and do what it says.

trace never fixes anything: it changes no code, not even temporary logging. It is not a unit.

## 1. Ask which flow

With no question from the owner, ask which flow to trace before reading any code:
- Numbered questions, lettered options, your recommended option first, then stop and wait.
- Use the harness's structured-question tool when it has one.
- Never ask what the owner already answered.

## 2. Look it up

Run `lyceum trace "<question>"`. It lists candidates with their freshness, spec areas first.
- `fresh` and it answers the question: summarise it, with its recorded date.
- `drifted`: summarise it, name the changed paths, and offer to refresh only those parts.
- `broken`: say it no longer matches the code, and offer to redo it.
- Nothing matches: trace it (step 3).

## 3. Ask the mode

Ask the owner: code-only (read the flow) or a full run (also run it against a test environment). A full run needs
an environment declared in the approved TESTING.md; with none, the CLI refuses: tell the owner and offer code-only.

## 4. Code-only

Read the flow from its entry point to its end. Write it as ordered steps; each records the symbol, data in and
out, side effects, branches and failure points. Mark every step `[read]`.

## 5. Full run

1. Do the code read first (step 4).
2. Run `lyceum trace --run <env> --trigger "<command>" "<question>"` without `--yes`. It prints the commands and
   their side effects and runs nothing.
3. Tell the owner those commands and side effects (for example "creates 1 order in the test DB") and ask in chat.
   Stop and wait.
4. Only when the owner agrees in this conversation, run the same command with `--yes`. Never add `--yes` on your
   own initiative. It runs `up`, `seed`, the trigger, `reset` and `down`, and records every command.
5. Paste the run block it prints into the record. Mark each step `[observed]` where the run shows it, `[read]`
   otherwise. A mismatch between the run and the code is a finding.
6. A step that logs, DB state or responses can't show stays `[read]`. You may suggest instrumentation as a
   `lyceum chore`; never add it here.

## 6. Save the record

Always save the trace; don't ask whether to.

1. Write `lyceum/trace/<slug>.md` with frontmatter `kind: trace`, `title:` and `cites:`: one `- path:` per file
   or folder the flow passes through, each with an optional `symbol:`. Never cite line numbers. Leave out `id`,
   `created` and the hashes.
2. The body has `## Steps`: one top-level bullet per step, in order, each starting `[read]` or `[observed]`,
   details as indented bullets. Then `## Findings`. Then a fenced `mermaid` block holding a `sequenceDiagram` of
   the flow.
3. Run `lyceum record lyceum/trace/<slug>.md`. It fills in the rest and commits that one file. A refusal names
   the field or the unmarked step: fix it and run it again.

## 7. Route the findings

Findings are routed, never fixed:
- The code contradicts the spec, or the run contradicts the code: offer `lyceum add bug "<title>"`, written out
  with a concrete title that says what is wrong, e.g. `lyceum add bug "Login accepts an expired token"`.
  Name the command; "a bug unit" alone is not the offer.
- Behaviour the spec doesn't cover: flag the spec drift, and offer `lyceum add feat "<title>"` or an as-built
  update of that area.

Then ask once whether to export the trace to `docs/`. If yes, run `lyceum export <record>`.

## Never

- Change code, config or tests, not even temporary logging, or fix a finding. Only `lyceum/trace/` changes, and
  `docs/` after a confirmed export.
- Run a full run without the owner's yes in chat, or against an environment TESTING.md doesn't declare.
- Cite line numbers, or edit `id`, `created` or the hashes by hand.
- Fake a terminal, retry a refused command, or work around a hook refusal. A refusal means stop and tell the owner.
