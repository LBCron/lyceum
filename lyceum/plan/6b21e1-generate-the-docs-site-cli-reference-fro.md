---
id: 6b21e1-generate-the-docs-site-cli-reference-fro
type: chore
title: Generate the docs-site CLI reference from lyceum --help at build time
branch: chore/6b21e1-generate-the-docs-site-cli-reference-fro
status: planned
gate: plan
files: [docs-site/scripts/gen-cli-reference.mjs, docs-site/scripts/gen-cli-reference.test.mjs, docs-site/package.json, docs-site/.gitignore, docs-site/astro.config.mjs, docs-site/src/content/docs/reference/commands.md, docs-site/README.md, .github/workflows/docs-site.yml]
approvals: []
---

## What and why

Closes #17. `reference/commands.md` is written by hand, so its flag tables can drift from the CLI. Generate the
command and flag reference from the shipped binary's own help at build time. No CLI change: the generator is a
docs-site script that runs `lyceum --help` and `lyceum <command> --help`.

Decided with the owner: a script, not a Go command (a new command would be a `feat` with a spec area, and
`cobra/doc` is a new dependency); generated at build time and gitignored, not committed; the hand-written flag
tables are dropped.

## Spec delta

None. No behaviour change in the CLI.

## Steps

1. `docs-site/scripts/gen-cli-reference.mjs`, no npm dependencies. It runs the binary named by `LYCEUM_BIN`
   (default `../dist/lyceum`), parses the root menu groups (Lifecycle, Change, Inspect) and each command's
   `--help` (summary, usage lines, flags, subcommands such as `add feat|bug|doc|test`), and writes
   `src/content/docs/reference/cli.md` with Starlight frontmatter. Hidden commands are not listed.
   It exits non-zero, with a message, when the binary is missing, a listed command has no description, or a
   flag line does not parse, so a changed help template breaks the build instead of producing a wrong page.
2. `gen-cli-reference.test.mjs` (`node --test`): parser cases from fixed help text, including a flag with a
   value, a flag without one, a subcommand list, and the failure cases above.
3. `package.json`: `prebuild` and `predev` run the generator. `.gitignore`: ignore the generated `cli.md`.
   `astro.config.mjs`: add `reference/cli` to the Reference sidebar.
4. `reference/commands.md`: remove the per-command flag tables and the menu tables the generated page now
   covers, keep the prose (global flags notes, gate names, what `plan` does on a trivial chore, `test` subsets,
   `log`, `doctor`, the Internal section) and link to the generated reference.
5. `.github/workflows/docs-site.yml`: add `actions/setup-go` and `go build -o dist/lyceum ./cmd/lyceum` ahead
   of the docs build, run `node --test scripts/`, and add `cmd/**`, `internal/cli/**`, `go.mod`, `go.sum` to the
   path filters so a flag change rebuilds and redeploys the docs. `docs-site/README.md`: how to build with Go
   or `LYCEUM_BIN`.
6. Verify: `go build -o dist/lyceum ./cmd/lyceum`, `npm ci && npm run build` exits 0 and the generated page
   lists every menu command and flag; diff it against `lyceum --help` by hand for three commands; point
   `LYCEUM_BIN` at a missing path and at a stub with a nameless command and confirm each fails loudly;
   `actionlint`. `lyceum test`, then stop at `close`.

## Out of scope (follow-ups)

`llms.txt` (#18) will want the generated page; that is its own unit. Hidden and internal commands stay
hand-written. A committed copy of the output and a drift check.

## Progress

## Log
