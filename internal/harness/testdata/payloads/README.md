# Pre-tool payload fixtures

One `write.json` and one `shell.json` per harness: what each harness sends its pre-tool hook when the agent writes
`note.txt` and runs `echo lyceum approve spec`. Session IDs, call IDs and paths are replaced with `SESSION`,
`CALL`, `/repo` and `/home/user`. Captured 2026-10-24 from live runs. Codex also has `edit.json` (an `apply_patch` update).

| Harness | Version | Source | Write: tool, path field | Shell: tool, command field |
|---|---|---|---|---|
| Claude Code | subscription CLI | live | `Write`, `tool_input.file_path` | `Bash`, `tool_input.command` |
| Command Code | 1.58.0 | live | `write_file`, `tool_input.file_path` | `shell_command`, `tool_input.command` |
| Pi | 0.86.0 | live (extension `tool_call` event: `toolName`, `input`) | `write`, `input.path` | `bash`, `input.command` |
| OpenCode | 2.0.18 | live (V2 plugin `ctx.tool.hook("execute.before")` event) | `write`, `input.path` | `shell`, `input.command` |
| Codex | 0.164.0 | live; `apply_patch` carries the patch text in `tool_input.command`, with **absolute** paths in `*** Add/Update/Delete File:` lines | `apply_patch` | `Bash`, `tool_input.command` |

Not captured: edit tools (`Edit`, `edit_file`, `edit`). They carry the same path field as write in every harness's
docs, and the adapters read them as writes by that field.

## Deny answers (verified live 2026-10-24: the shell call did not run in all three)

| Harness | How the hook denies | Verified |
|---|---|---|
| Claude Code, Codex, Command Code | stdout `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"<reason>"}}`, exit 0 | Command Code and Codex live; Claude Code known |
| Pi | extension returns `{ block: true, reason }` from `pi.on("tool_call")` | live |
| OpenCode 2.x | V2 plugin (`export default { id, setup(ctx) }`) throws from `ctx.tool.hook("execute.before")`; the V1 `"tool.execute.before"` export fails to load in 2.0 | live |

Allow: empty stdout and exit 0 (JSON harnesses); return nothing (Pi); return normally (OpenCode).

## Shell writes

Agents also write files through the shell: asked to create `note.txt`, Codex ran `printf hi > note.txt` as `Bash`.
A pre-tool hook can't reliably parse every shell write, so the hook blocks write tools by path and shell calls
only for `lyceum approve`; shell writes are left to `check`.

## Codex hook trust

Project hooks in `.codex/hooks.json` did not run at all until trusted (no payloads without
`--dangerously-bypass-hook-trust`, used here for capture only). In normal use the owner trusts them once.

## OpenCode permission evaluate

`permission.json` is the event OpenCode 2.0.18 passes to `ctx.permission.hook("evaluate")` for a shell call
(captured live 2026-10-25): `action: "shell"`, the command in `resources[0]`, `effect: "allow"`. It fires *after*
`ctx.tool.hook("execute.before")` for the same call. Setting `effect = "ask"` and `message` raised OpenCode's own
permission prompt; under non-interactive `opencode run` the prompt is auto-rejected ("The user declined this tool
call"), so no human means no approval.
