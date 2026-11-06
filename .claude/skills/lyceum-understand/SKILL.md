---
name: lyceum-understand
description: Use when the owner wants to understand a Lyceum project or part of it, to answer from a fresh spec area or record, or read the code and save the answer as a record.
---

# lyceum-understand

Run `lyceum status` first. Its printed next action wins over anything here. Every `lyceum` command ends with a
`Next:` line; follow it. On a refusal, repeat its reason to the owner in plain words and do what it says.

understand explains; it changes no code, not even temporary logging. It is not a unit: no branch, plan or gates.

## 1. Ask what to understand

With no hint from the owner, ask what they want to understand before reading any code. Asking is your next
action after `lyceum status`:
- Take the options only from `lyceum status`, the area registry in `lyceum/SPEC.md`, and directory and file names
  from a listing. Never open a source file, test, config file, RULES.md or TESTING.md before the owner answers;
  file names are fine, file contents are not.
- Numbered questions, lettered options, your recommended option first, then stop and wait.
- Use the harness's structured-question tool when it has one.
- Never ask what the owner already answered. Options are concrete parts of this project.

## 2. Look it up

Run `lyceum understand "<hint>"`. It lists candidates with their freshness: spec areas first, then records in
`lyceum/understand/` and `lyceum/trace/`, then `docs/`. Match the hint to them by meaning.

- Prefer a spec area that answers the hint, then a record.
- `fresh`: summarise it, with its recorded date. No new record is needed.
- `drifted`: summarise it, name the changed paths, and offer to refresh only those parts.
- `broken`: say it no longer matches the code, and offer to redo it.
- `invalid`: fix the file, then run `lyceum record <file>`.
- Nothing matches: work it out from the code (step 3).

## 3. Read the code

- If the output warns that the scope is over `understand.warn_threshold`, or the hint is broad, tell the owner
  the measured scope (files, lines) and ask before reading. Offer to narrow it.
- Read the code and answer the owner in plain words.

## 4. Save the record

Always save the answer; don't ask whether to.

1. Write `lyceum/understand/<slug>.md` with frontmatter `kind: understand`, `title:` and `cites:`: one
   `- path:` per file or folder the answer rests on, each with an optional `symbol:` (a function, type or
   route). Never cite line numbers. Leave out `id`, `created` and the hashes: the CLI fills them in.
2. The body is the answer: what the part does, its main pieces and how they connect, in plain words.
3. Run `lyceum record lyceum/understand/<slug>.md`. It fills in `id`, `created` and each cite's hash, checks the
   format, and commits that one file. A refusal names the field: fix it and run it again. If it refuses because
   other files are staged, ask the owner what to do with them.

## 5. Offer the export

Ask once whether to export the record to `docs/`. If yes, run `lyceum export <record>`. This is the only
question about saving.

A bug or a gap you notice while reading: tell the owner and offer `lyceum add bug "<title>"` or
`lyceum add feat "<title>"`. Never fix it here.

## Never

- Read a file's contents, other than `lyceum/SPEC.md`, before asking what to understand when the owner gave no
  hint.
- Change code, config or tests, not even temporary logging. Only `lyceum/understand/` changes, and `docs/` after
  a confirmed export.
- Cite line numbers, or edit `id`, `created` or the hashes by hand.
- Fake a terminal, retry a refused command, or work around a hook refusal. A refusal means stop and tell the owner.
