#!/usr/bin/env bash
# Release check for the inspect side (M4b): understand, record, freshness, trace with export, audit, and the
# live-only AC68, AC74 and AC80,
# driven through a live agent harness. Throwaway scratch repos only; never point it at a real repository.
#
# Usage: scripts/release-check-inspect.sh <harness>    (claude, codex, commandcode, opencode, pi)
# Prints  RESULT|<harness>|<check>|PASS|FAIL|<note>  lines; KEEP=1 keeps the repo and logs.
set -uo pipefail

h=${1:?usage: $0 <harness>}
command -v lyceum >/dev/null || { echo "lyceum is not on PATH" >&2; exit 2; }
command -v script >/dev/null || { echo "script(1) is needed for the owner's terminal" >&2; exit 2; }
WORK=$(mktemp -d "${TMPDIR:-/tmp}/lyceum-inspect-$h.XXXXXX")
T=${AGENT_TIMEOUT:-900}
r() { echo "RESULT|$h|$1|$2|$3"; }

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

# offered <log> <skill file>...: the agent's first line offering a bug unit. A log can echo skill text, which writes
# add bug "<title>" itself, so any line holding a whole installed skill line (30+ chars) is dropped first; what is left
# is the agent's own words, which must name add bug or offer to open a bug unit, not merely say "bug".
offered() {
	local log=$1 pat s; shift
	pat=$(mktemp)
	sed 's/^[[:space:]]*//; s/[[:space:]]*$//' "$@" | awk 'length >= 30' | sort -u >"$pat"
	sed 's/\x1b\[[0-9;]*[A-Za-z]//g' "$log" | grep -vFf "$pat" |
		grep -m1 -iE 'add bug|\b(offer|recommend|suggest|propose|open|start|raise|file)[^.]{0,60}\bbug unit'
	s=$?; rm -f "$pat"; return $s
}

owner_approve() { # <gate>
	printf '%s\n' "$1" | env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u CODEX_THREAD_ID -u CODEX_CI \
		-u OPENCODE_TERMINAL -u PI_CODING_AGENT -u COMMANDCODE_SCRATCHPAD \
		script -qec "lyceum approve $1" /dev/null
}

L="$WORK/logs"; mkdir -p "$L"
R="$WORK/repo"; mkdir -p "$R/cmd/shop" "$R/internal/cart" && cd "$R" || exit 1
git init -q -b main && git config user.email rc@lyceum.test && git config user.name rc
cat >go.mod <<'EOF'
module example.com/shop

go 1.22
EOF
cat >internal/cart/cart.go <<'EOF'
package cart

// Item is one line in a cart.
type Item struct {
	SKU   string
	Price int // cents
	Qty   int
}

// Total returns the cart total in cents, applying a 10% discount when it is 10000 cents or more.
func Total(items []Item) int {
	sum := 0
	for _, it := range items {
		sum += it.Price * it.Qty
	}
	if sum >= 10000 {
		sum -= sum / 10
	}
	return sum
}
EOF
cat >internal/cart/cart_test.go <<'EOF'
package cart

import "testing"

func TestTotal(t *testing.T) {
	if got := Total([]Item{{"a", 5000, 2}}); got != 9000 {
		t.Fatalf("got %d", got)
	}
}
EOF
cat >cmd/shop/main.go <<'EOF'
package main

import (
	"fmt"

	"example.com/shop/internal/cart"
)

func main() {
	fmt.Println(cart.Total([]cart.Item{{SKU: "a", Price: 5000, Qty: 2}}))
}
EOF
cat >TESTING.md <<'EOF'
---
lyceum: 1
suite: go test ./...
single: go test ./... -run {name}
test_files: ["**/*_test.go"]
timeout: 5m
---

# TESTING.md

Go module. Tests live beside the code as `*_test.go`.
EOF
git add -A && git commit -q -m "shop"
lyceum init --harness "$h" </dev/null >"$L/init.log" 2>&1
git add -A && git commit -q -m "lyceum init"
owner_approve testing >"$L/approve-testing.log" 2>&1
# Untouched copies for the AC68 and AC80 runs at the end. init writes the repo's absolute .git path into
# .codex/config.toml, so each copy re-runs init (the owner's setup) to name its own .git, not the original's.
for c in ac68 ac80; do
	cp -a "$R" "$WORK/$c" && (cd "$WORK/$c" && lyceum init --harness "$h" </dev/null >"$L/init-$c.log" 2>&1 &&
		git add -A && { git diff --cached --quiet || git commit -q -m "lyceum init: this copy"; })
done

# ---- understand → record → fresh ----
# The prompt carries the owner's question and nothing about saving, so a committed record proves AC74.
agent "Use the lyceum-understand skill. Question: how is the cart total computed, including any discount?" "$L/understand.log"
rec=$(ls lyceum/understand/*.md 2>/dev/null | head -1)
if [ -n "$rec" ] && grep -q '^lyceum: record' <<<"$(git log --format=%s -1 -- "$rec")" && grep -q 'blob:' "$rec"; then
	st=$(lyceum status --json 2>/dev/null)
	if echo "$st" | grep -q '"records_fresh":[1-9]'; then r UNDERSTAND PASS "record $(basename "$rec") committed via lyceum record, fresh"
	else r UNDERSTAND FAIL "record not fresh: $(echo "$st" | head -c 300)"; fi
else r UNDERSTAND FAIL "no recorded understand record, see $L/understand.log"; fi
# AC74: stdin is /dev/null, so a question would have ended the run before the record; also no save question in the log.
if [ -n "$rec" ] && grep -q '^lyceum: record' <<<"$(git log --format=%s -1 -- "$rec")"; then
	if q=$(grep -iE '\bsave\b[^.]*\?' "$L/understand.log"); then r AC74 FAIL "asked about saving: $(echo "$q" | head -1 | head -c 200)"
	else r AC74 PASS "record $(basename "$rec") committed via lyceum record in a run nobody could answer; no save question"; fi
else r AC74 FAIL "no record committed via lyceum record, see $L/understand.log"; fi

# ---- drift: change a cited file ----
if [ -n "$rec" ]; then
	sed -i 's/sum -= sum \/ 10/sum -= sum \/ 5/' internal/cart/cart.go
	git commit -qam "change discount"
	# Output is captured, not piped: under pipefail, grep -q exiting at its match can SIGPIPE the writer and fail.
	st=$(lyceum status 2>&1)
	if grep -qi 'drifted' <<<"$st" && grep -q 'internal/cart/cart.go' <<<"$st"; then
		r DRIFT PASS "editing internal/cart/cart.go marks the record drifted and names the file"
	else r DRIFT FAIL "$(tail -5 <<<"$st" | tr '\n' ' ')"; fi
	ck=$(lyceum check 2>&1); s=$?
	if grep -qi 'drift' <<<"$ck"; then r DRIFT-CHECK PASS "check warns on the drifted record"
	else r DRIFT-CHECK FAIL "check gave no drift warning (exit $s): $(tr '\n' ' ' <<<"$ck" | head -c 300)"; fi
fi

# ---- trace, code-only, then export ----
BEFORE=$(git rev-parse HEAD) # inspect work starts here; the owner's own commits above are not inspect output
agent "Use the lyceum-trace skill. Question: trace what happens when cmd/shop runs, from main to the printed
total. Use code-only mode (no environment is defined). Save the trace as the skill says, then export it to docs/." "$L/trace.log"
tr=$(ls lyceum/trace/*.md 2>/dev/null | head -1)
steps_ok() { local st; st=$(awk '/^## /{f=($0=="## Steps")} f && /^- /' "$1"); [ -n "$st" ] && grep -vqE '\[(read|observed)\]' <<<"$st" && return 1; return 0; }
if [ -n "$tr" ] && grep -q 'sequenceDiagram' "$tr" && steps_ok "$tr"; then
	ex=$(ls docs/trace/*.md 2>/dev/null | head -1)
	if [ -n "$ex" ] && grep -q 'lyceum export' "$ex" && grep -q 'sequenceDiagram' "$ex"; then
		r TRACE PASS "trace recorded with marked steps and a sequence diagram; exported to $ex"
	else r TRACE FAIL "trace recorded but no export, see $L/trace.log"; fi
else r TRACE FAIL "no valid trace record, see $L/trace.log"; fi
changed=$(git diff --name-only "$BEFORE" HEAD 2>/dev/null)
if [ -n "$changed" ] && grep -vqE '^(lyceum/|docs/)' <<<"$changed"; then
	r AC67 FAIL "inspect commits touched files outside lyceum/ and docs/"
else r AC67 PASS "inspect work changed only lyceum/ and docs/"; fi

# ---- audit ----
agent "Use the lyceum-audit skill. Audit this project's dependencies and write the summary as the skill says." "$L/audit.log"
au=$(ls lyceum/audit/*.md 2>/dev/null | head -1)
if [ -n "$au" ] && grep -q 'govulncheck' "$au" && git diff --quiet HEAD -- go.mod; then
	if grep -q '[A-Za-z]' <<<"$(awk '/^## Summary/{f=1;next} /^## /{f=0} f' "$au")"; then r AUDIT PASS "govulncheck ran, raw output recorded, summary written, go.mod unchanged"
	else r AUDIT FAIL "audit record has an empty summary"; fi
else r AUDIT FAIL "no audit record or go.mod changed, see $L/audit.log"; fi

lyceum log >"$L/log.txt" 2>&1 && r LOG PASS "$(grep -c . "$L/log.txt") entries" || r LOG FAIL "log failed"

# ---- AC68: bare understand asks before reading any code (fresh copy) ----
# A read sets a file's access time (relatime updates one older than a day), whatever tool did the reading. git and
# lyceum status don't read contents once the index is fresh; a bare "lyceum understand" counts lines, so a lyceum
# wrapper on PATH puts the access times back after each lyceum call. Reads through git objects are not seen.
cd "$WORK/ac68" || exit 1
sleep 1 # the refreshed index must be newer than the files, or git re-reads them as racily clean
git config core.trustctime false && git update-index -q --refresh
git ls-files -z internal cmd | xargs -0 realpath >"$WORK/ac68-watch"
mkdir -p "$WORK/bin"
cat >"$WORK/bin/lyceum" <<EOF
#!/usr/bin/env bash
mapfile -t w <"$WORK/ac68-watch"; a=()
for f in "\${w[@]}"; do a+=("\$(stat -c %x "\$f")"); done
"$(command -v lyceum)" "\$@"; s=\$?
for i in "\${!w[@]}"; do touch -a -d "\${a[\$i]}" "\${w[\$i]}"; done
exit \$s
EOF
chmod +x "$WORK/bin/lyceum"
probe="$WORK/ac68-probe"; echo x >"$probe"; touch -a -d 2000-01-01 "$probe"; cat "$probe" >/dev/null
# Calibration: some harnesses (OpenCode) touch source access times on startup without any read tool call. A
# do-nothing run shows whether access times mean anything for this harness; if not, reads are judged from the
# harness's tool-call log instead: a read verb on the same line as a watched path, before the question.
xargs -d '\n' touch -a -d 2000-01-01 <"$WORK/ac68-watch"
PATH="$WORK/bin:$PATH" agent "Reply with the single word OK. Do not run any command, use any tool or read any file." "$L/ac68-calibrate.log"
noisy=$(while read -r f; do [ "$(stat -c %X "$f")" -ge 1000000000 ] && echo "${f#"$WORK/ac68/"}"; done <"$WORK/ac68-watch")
[ -z "$(git status --porcelain)" ] || { r AC68 FAIL "the do-nothing calibration run changed the tree, see $L/ac68-calibrate.log"; }
xargs -d '\n' touch -a -d 2000-01-01 <"$WORK/ac68-watch"
B=$(git rev-parse HEAD)
PATH="$WORK/bin:$PATH" agent "Use the lyceum-understand skill." "$L/ac68.log"
if [ -z "$noisy" ]; then
	seen=$(while read -r f; do [ "$(stat -c %X "$f")" -ge 1000000000 ] && echo "${f#"$WORK/ac68/"}"; done <"$WORK/ac68-watch")
	how="access times"
else
	# up to the first question; a listing (find/ls/tree) prints paths without a read verb and does not count
	seen=$(sed 's/\x1b\[[0-9;]*[A-Za-z]//g' "$L/ac68.log" | awk '/\?[[:space:]`"*)]*$|Asked [0-9]+ question/{exit} {print}' |
		grep -E '(^|[^[:alnum:]_])(Read|read|cat|head|tail|less|more|bat|sed|awk|grep|rg|view|open)[^[:alnum:]_].*(cmd|internal)/[[:alnum:]_./-]+\.go' |
		grep -oE '(cmd|internal)/[[:alnum:]_./-]+\.go' | sort -u)
	how="tool-call log (access times moved on a do-nothing run: $(echo "$noisy" | tr '\n' ' '))"
fi
asked=$(sed 's/\x1b\[[0-9;]*[A-Za-z]//g' "$L/ac68.log" | grep -E '\?[[:space:]`"*)]*$' | head -1)
# a harness question tool (OpenCode prints "Asked 1 question") counts; the dismissal comes from stdin being /dev/null
[ -z "$asked" ] && asked=$(sed 's/\x1b\[[0-9;]*[A-Za-z]//g' "$L/ac68.log" | grep -m1 -E 'Asked [0-9]+ questions?')
if ls lyceum/understand/*.md >/dev/null 2>&1 || [ "$(git rev-parse HEAD)" != "$B" ]; then r AC68 FAIL "wrote or committed without asking, see $L/ac68.log"
elif [ -z "$asked" ]; then r AC68 FAIL "no question in the output, see $L/ac68.log"
elif [ "$how" = "access times" ] && [ "$(stat -c %X "$probe")" -lt 1000000000 ]; then r AC68 FAIL "access times don't update under $WORK, so reads can't be seen; rerun with another TMPDIR"
elif [ -n "$seen" ]; then r AC68 FAIL "asked, but read code first (by $how): $(echo "$seen" | tr '\n' ' ')"
else r AC68 PASS "asked: $(echo "$asked" | head -c 150); no code file read (by $how), no record, no commit"; fi

# ---- AC80: a trace that finds the code contradicting the spec offers add bug, changes no code (fresh copy) ----
# The owner writes and approves a cart area whose threshold the code breaks: cart.Total discounts at 10000 cents.
cd "$WORK/ac80" || exit 1
mkdir -p lyceum/spec
cat >lyceum/spec/cart.md <<'EOF'
# Cart

The cart and its total.

## Acceptance criteria

| ID | Criterion |
|---|---|
| AC1 | Given a cart totalling 20000 cents or less, then `cart.Total` returns the plain sum, with no discount |
| AC2 | Given a cart totalling more than 20000 cents, then `cart.Total` takes 10% off |
EOF
echo '| cart | `spec/cart.md` | Cart items and the total with its discount | AC1–AC2 |' >>lyceum/SPEC.md
git add -A && git commit -q -m "spec: cart"
owner_approve project >"$L/approve-project.log" 2>&1
B=$(git rev-parse HEAD); heads=$(git for-each-ref refs/heads | wc -l)
agent "Use the lyceum-trace skill. Question: trace how cmd/shop computes the total it prints. Use code-only mode
(no environment is defined)." "$L/ac80.log"
offer=$(offered "$L/ac80.log" .agents/skills/*/SKILL.md)
changed=$({ git diff --name-only "$B"; git ls-files -o --exclude-standard; } | grep -vE '^(lyceum/|docs/|\.)' | sort -u)
if [ -n "$changed" ]; then r AC80 FAIL "changed outside lyceum/ and docs/: $(echo "$changed" | tr '\n' ' ')"
elif ! grep -q '"units":\[\]' <<<"$(lyceum status --json 2>/dev/null)" || [ "$(git for-each-ref refs/heads | wc -l)" != "$heads" ]; then
	r AC80 FAIL "a unit or branch was opened, see $L/ac80.log"
elif [ -z "$offer" ]; then r AC80 FAIL "no add bug offer, see $L/ac80.log"
else r AC80 PASS "offered: $(echo "$offer" | head -c 150); no code changed, no unit opened"; fi

if [ "${KEEP:-0}" = 1 ]; then echo "KEPT|$WORK"; else rm -rf "$WORK"; fi
