#!/usr/bin/env bash
# Release checks AC90 (feat) and AC91 (bug) end to end, plus refactor, chore, doc and test flows (FLOW-*), against a live agent harness (distribution.md
# "Release checks"). Throwaway scratch repos only: the script plays the owner for the judgment gates,
# approving in a real pseudo-terminal with every harness marker unset, exactly as the owner would in their own
# terminal. Never point it at a real repository.
#
# Usage: scripts/release-check-flows.sh <harness>    (claude, codex, commandcode, opencode, pi)
# Prints  RESULT|<harness>|<AC>|PASS|FAIL|<note>  lines; KEEP=1 keeps the repos and logs.
set -uo pipefail

h=${1:?usage: $0 <harness>}
command -v lyceum >/dev/null || { echo "lyceum is not on PATH" >&2; exit 2; }
command -v script >/dev/null || { echo "script(1) is needed for the owner's terminal" >&2; exit 2; }
WORK=$(mktemp -d "${TMPDIR:-/tmp}/lyceum-flows-$h.XXXXXX")
T=${AGENT_TIMEOUT:-900}
r() { echo "RESULT|$h|$1|$2|$3"; }
want() { [ -z "${FLOWS:-}" ] || [[ " $FLOWS " == *" $1 "* ]]; } # FLOWS="feat bug refactor chore doc test" limits the run

agent() { # <prompt> <log>
	case $h in
	claude) timeout "$T" env -u ANTHROPIC_API_KEY -u ANTHROPIC_BASE_URL -u AI_AGENT claude -p --setting-sources project \
		--permission-mode acceptEdits --allowedTools Bash Read Write Edit --max-turns 60 "$1" ;;
	codex) timeout "$T" codex exec -s workspace-write --skip-git-repo-check --dangerously-bypass-hook-trust "$1" ;;
	commandcode) timeout "$T" commandcode -p --trust --yolo --no-auto-update "$1" ;;
	pi) timeout "$T" pi -p -a --no-session "$1" ;;
	opencode) timeout "$T" opencode run -m "${OPENCODE_MODEL:-deepseek/deepseek-v4-pro}" "$1" ;;
	esac >"$2" 2>&1 </dev/null
}

owner_approve() { # <gate>: the owner, in a real terminal, outside any harness
	printf '%s\n' "$1" | env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u CODEX_THREAD_ID -u CODEX_CI \
		-u OPENCODE_TERMINAL -u PI_CODING_AGENT -u COMMANDCODE_SCRATCHPAD \
		script -qec "lyceum approve $1" /dev/null
}

has_acs() { # <plan>: the frontmatter acs: list is non-empty, inline or block form
	awk '/^---$/{n++; next} n==1' "$1" | grep -Eq '^acs: *\[ *"?AC[0-9]|^  *- *"?AC[0-9]'
}

gate_of() { sed -n 's/^gate: //p' "$1"; }
closed() { grep -q '^status: closed' "$1" && grep -q '"type":"close"' lyceum/events.jsonl; }
chk() { lyceum check >/dev/null 2>&1 && echo "check clean" || echo "check: $(lyceum check 2>&1 | head -1)"; }

setup() { # <dir>: a small shell project with an approved TESTING.md and one registered area
	mkdir -p "$1/src" "$1/tests" && cd "$1" || exit 1
	git init -q -b main && git config user.email rc@lyceum.test && git config user.name rc
	cat >t.sh <<'EOF'
#!/bin/sh
# Runs every tests/*_test.sh, or only those whose name contains $1.
rc=0
for f in tests/*_test.sh; do
	[ -e "$f" ] || continue
	case $f in *"${1:-}"*) sh "$f" || { echo "FAIL $f"; rc=1; } ;; esac
done
exit $rc
EOF
	cat >TESTING.md <<'EOF'
---
lyceum: 1
suite: sh ./t.sh
single: sh ./t.sh {name}
test_files: ["tests/*_test.sh"]
timeout: 2m
---

# TESTING.md

Shell project. Each test is a POSIX sh file `tests/<name>_test.sh` that exits non-zero on failure and names the AC
it proves in a comment (`# AC12`). `{name}` for `single` is the file name without `_test.sh`.
EOF
	git commit -q --allow-empty -m init
	lyceum init --harness "$h" </dev/null >"$L/init.log" 2>&1 || return 1
	mkdir -p lyceum/spec
	printf '# Tools\n\nSmall shell functions under `src/`.\n' >lyceum/spec/tools.md
	sed -i 's/^|---|---|---|---|$/&\n| tools | `spec\/tools.md` | Small shell functions | |/' lyceum/SPEC.md
	git add -A && git commit -q -m "lyceum init"
	owner_approve testing >"$L/approve-testing.log" 2>&1
	owner_approve project >"$L/approve-project.log" 2>&1
	grep -q '"gate":"testing"' lyceum/events.jsonl && grep -q '"gate":"project"' lyceum/events.jsonl
}

# ---- AC90: feat ----
L="$WORK/feat.logs"; mkdir -p "$L"
if ! want feat; then :
elif setup "$WORK/feat"; then
	lyceum add feat "Greeting" >"$L/add.log" 2>&1
	plan=$(ls lyceum/plan/*.md | head -1)
	agent "Use the lyceum-feat skill. Feature: a POSIX sh function greet in src/greet.sh; greet NAME prints
'hello NAME'. The request answers everything, so do not interview: draft the spec delta, one AC and the plan for
area tools, then stop at the spec gate as the skill says." "$L/draft.log"
	if [ "$(gate_of "$plan")" = spec ] && has_acs "$plan"; then
		git add -A && git commit -q -m "feat draft" 2>/dev/null
		owner_approve spec >"$L/approve-spec.log" 2>&1
		if [ "$(gate_of "$plan")" = build ]; then
			agent "Use the lyceum-feat skill and continue the unit in flight: build it, commit, and run lyceum test
until verify clears. Report the CLI's result." "$L/build.log"
			if [ "$(gate_of "$plan")" = close ] && grep -q '"type":"gate-clear".*"gate":"verify"' lyceum/events.jsonl; then
				agent "Use the lyceum-feat skill: the unit's last gate is cleared, so close it." "$L/close.log"
				if closed "$plan"; then r AC90 PASS "add → spec (owner) → build → verify → close; $(chk)"
				else r AC90 FAIL "verify cleared but not closed, see $L/close.log"; fi
			else r AC90 FAIL "after build: gate $(gate_of "$plan"), see $L/build.log"; fi
		else r AC90 FAIL "owner approve spec did not clear, see $L/approve-spec.log"; fi
	else r AC90 FAIL "draft: gate $(gate_of "$plan"), acs missing or gate moved, see $L/draft.log"; fi
else r AC90 FAIL "setup failed, see $L"; fi

# ---- AC91: bug ----
L="$WORK/bug.logs"; mkdir -p "$L"
if ! want bug; then :
elif setup "$WORK/bug"; then
	printf 'add() { echo $(($1 - $2)); }\n' >src/add.sh
	git add -A && git commit -q -m "add function"
	lyceum add bug "add subtracts instead of adding" >"$L/add.log" 2>&1
	plan=$(ls lyceum/plan/*.md | head -1)
	agent "Use the lyceum-bug skill. Bug: in src/add.sh, 'add 2 3' prints -1; it should print 5. The report
answers everything, so do not interview. Take the bug through repro, diagnose, fix and verify, committing as the
skill says, and stop at the cause gate." "$L/work.log"
	ok_repro=$(grep -c '"type":"gate-clear".*"gate":"repro"' lyceum/events.jsonl)
	ok_verify=$(grep -c '"type":"gate-clear".*"gate":"verify"' lyceum/events.jsonl)
	if [ "$(gate_of "$plan")" = cause ] && [ "$ok_repro" -ge 1 ] && [ "$ok_verify" -ge 1 ]; then
		owner_approve cause >"$L/approve-cause.log" 2>&1
		if [ "$(gate_of "$plan")" = close ]; then
			agent "Use the lyceum-bug skill: the cause is approved, so close the unit." "$L/close.log"
			if closed "$plan"; then r AC91 PASS "add → repro → diagnose → fix → verify → cause (owner) → close; $(chk)"
			else r AC91 FAIL "cause approved but not closed, see $L/close.log"; fi
		else r AC91 FAIL "owner approve cause did not clear, see $L/approve-cause.log"; fi
	else r AC91 FAIL "gate $(gate_of "$plan"), repro clears $ok_repro, verify clears $ok_verify, see $L/work.log"; fi
else r AC91 FAIL "setup failed, see $L"; fi

# ---- refactor, chore, doc, test: each unit type through close (M4a) ----
L="$WORK/refactor.logs"; mkdir -p "$L"
if ! want refactor; then :
elif setup "$WORK/refactor"; then
	printf 'total() { s=0; for n in "$@"; do s=$((s + n)); done; echo $s; }\nmean() { s=0; for n in "$@"; do s=$((s + n)); done; echo $((s / $#)); }\n' >src/stats.sh
	printf '# AC0\n. ./src/stats.sh\n[ "$(total 1 2 3)" = 6 ] && [ "$(mean 2 4)" = 3 ]\n' >tests/stats_test.sh
	git add -A && git commit -q -m "stats"
	lyceum refactor "Share the sum loop" >"$L/add.log" 2>&1
	plan=$(ls lyceum/plan/*.md | head -1)
	agent "Use the lyceum-refactor skill. Refactor: in src/stats.sh, mean repeats total's loop; make mean call total.
Behaviour must not change. Not a performance goal. Do not interview: write the intent and stop at the intent gate." "$L/intent.log"
	git add -A && git commit -q -m "intent" 2>/dev/null
	owner_approve intent >"$L/approve-intent.log" 2>&1
	agent "Use the lyceum-refactor skill and continue the unit in flight: clear baseline, write the plan, and stop
at the plan gate." "$L/plan.log"
	git add -A && git commit -q -m "plan" 2>/dev/null
	if [ "$(gate_of "$plan")" = baseline ]; then # the agent added characterization tests: the owner approves them
		owner_approve tests >"$L/approve-tests.log" 2>&1
		agent "Use the lyceum-refactor skill and continue the unit in flight: the characterization tests are
approved. Clear baseline, write the plan, and stop at the plan gate." "$L/plan2.log"
		git add -A && git commit -q -m "plan" 2>/dev/null
	fi
	if [ "$(gate_of "$plan")" = plan ]; then
		owner_approve plan >"$L/approve-plan.log" 2>&1
		agent "Use the lyceum-refactor skill and continue: build the steps, keep tests green, clear verify, then close." "$L/build.log"
		if closed "$plan"; then r FLOW-refactor PASS "intent (owner) → baseline → plan (owner) → build → verify → close; $(chk)"
		else r FLOW-refactor FAIL "gate $(gate_of "$plan"), see $L/build.log"; fi
	else r FLOW-refactor FAIL "not at plan: gate $(gate_of "$plan"), see $L/plan.log"; fi
else r FLOW-refactor FAIL "setup failed, see $L"; fi

L="$WORK/chore.logs"; mkdir -p "$L"
if ! want chore; then :
elif setup "$WORK/chore"; then
	lyceum chore "Add a README" >"$L/add.log" 2>&1
	plan=$(ls lyceum/plan/*.md | head -1)
	agent "Use the lyceum-chore skill. Chore: add a README.md with one paragraph saying this repo holds small shell
functions under src/. Then close the unit." "$L/work.log"
	if closed "$plan"; then r FLOW-chore PASS "trivial chore → close; $(chk)"
	else r FLOW-chore FAIL "gate $(gate_of "$plan"), see $L/work.log"; fi
else r FLOW-chore FAIL "setup failed, see $L"; fi

L="$WORK/doc.logs"; mkdir -p "$L"
if ! want doc; then :
elif setup "$WORK/doc"; then
	lyceum add doc "How to run the tests" >"$L/add.log" 2>&1
	plan=$(ls lyceum/plan/*.md | head -1)
	agent "Use the lyceum-doc skill. Doc: docs/testing.md explaining that 'sh ./t.sh' runs every test. Run what it
shows, clear verify, then close the unit." "$L/work.log"
	if closed "$plan"; then r FLOW-doc PASS "draft → verify → close; $(chk)"
	else r FLOW-doc FAIL "gate $(gate_of "$plan"), see $L/work.log"; fi
else r FLOW-doc FAIL "setup failed, see $L"; fi

L="$WORK/test.logs"; mkdir -p "$L"
if ! want test; then :
elif setup "$WORK/test"; then
	printf 'double() { echo $(($1 * 2)); }\n' >src/double.sh
	git add -A && git commit -q -m "double"
	lyceum add test "Cover double" >"$L/add.log" 2>&1
	plan=$(ls lyceum/plan/*.md | head -1)
	agent "Use the lyceum-test skill. Add a test for double in src/double.sh (double 4 prints 8). Clear run, then
close the unit." "$L/work.log"
	if closed "$plan"; then r FLOW-test PASS "write → run → close; $(chk)"
	else r FLOW-test FAIL "gate $(gate_of "$plan"), see $L/work.log"; fi
else r FLOW-test FAIL "setup failed, see $L"; fi

if [ "${KEEP:-0}" = 1 ]; then echo "KEPT|$WORK"; else rm -rf "$WORK"; fi
