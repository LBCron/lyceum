package cli

import "testing"

// AC54: a shell command line that invokes lyceum approve, however it is spelled, is blocked.
func TestRunsApprove_AC54(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"lyceum approve", true},
		{"lyceum approve spec", true},
		{"echo lyceum approve spec", true},
		{"npx lyceum approve", true},
		{"./dist/lyceum --json approve spec", true},
		{"/usr/local/bin/lyceum approve plan", true},
		{"cd x && lyceum approve", true},
		{"git status;lyceum approve", true},
		{`sh -c 'lyceum approve spec'`, true},
		{`sh -c "lyceum  approve"`, true},
		{"$(which lyceum) approve", true},
		{"FOO=1 lyceum approve", true},
		{"git status", false},
		{"lyceum status", false},
		{"lyceum status && echo approve", false},
		{"lyceum check --ci", false},
		{"approve lyceum", false},
		{"lyceumx approve", false},
		{"grep -r approve lyceum/", false},
		{"", false},
		{"git config alias.x '!lyceum approve spec'", true},
	}
	for _, c := range cases {
		if got := runsLyceum(c.line, "approve"); got != c.want {
			t.Errorf("runsLyceum(%q, approve) = %v; want %v", c.line, got, c.want)
		}
	}
}

// AC137: a shell command line that invokes lyceum eject is blocked in every form lyceum approve is.
func TestRunsEject_AC137(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"lyceum eject", true},
		{"echo lyceum eject", true},
		{"npx lyceum eject", true},
		{"./dist/lyceum --json eject", true},
		{"/usr/local/bin/lyceum eject", true},
		{"cd x && lyceum eject", true},
		{"git status;lyceum eject", true},
		{`sh -c 'lyceum eject'`, true},
		{`sh -c "lyceum  eject"`, true},
		{"$(which lyceum) eject", true},
		{"FOO=1 lyceum eject", true},
		{"lyceum approve spec && lyceum eject", true},
		{"lyceum status", false},
		{"lyceum status && echo eject", false},
		{"lyceum approve spec", false},
		{"eject lyceum", false},
		{"lyceumx eject", false},
		{"grep -r eject lyceum/", false},
	}
	for _, c := range cases {
		if got := runsLyceum(c.line, "eject"); got != c.want {
			t.Errorf("runsLyceum(%q, eject) = %v; want %v", c.line, got, c.want)
		}
	}
}

// AC122: the gate a shell command line running lyceum approve names, for the harness-prompt route.
func TestPromptGate_AC122(t *testing.T) {
	cases := []struct {
		line string
		want string
		ok   bool
	}{
		{"lyceum approve spec", "spec", true},
		{"./dist/lyceum --json approve spec", "spec", true},
		{"lyceum approve --json plan", "plan", true},
		{"$(which lyceum) approve testing", "testing", true},
		{"cd x && lyceum approve spec", "spec", true},
		{"lyceum approve", "", false},
		{"lyceum approve --json", "", false},
		{"lyceum approve; echo spec", "", false},
		{"lyceum status", "", false},
	}
	for _, c := range cases {
		if got, ok := promptGate(c.line); string(got) != c.want || ok != c.ok {
			t.Errorf("promptGate(%q) = %q, %v; want %q, %v", c.line, got, ok, c.want, c.ok)
		}
	}
}

// AC173: heredoc bodies and commit messages are data; anything that can run stays caught.
func TestRunsApproveData_AC173(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		// Data.
		{"cat > plan.md <<'EOF'\nthe owner runs lyceum approve testing\nEOF", false},
		{"cat > plan.md <<\"EOF\"\nlyceum approve testing\nEOF\ngit add plan.md && git commit -m plan", false},
		{"cat <<'EOF' > plan.md\nlyceum approve testing\nEOF", false},
		{"cat >> plan.md <<EOF\nlyceum approve testing\nEOF", false},
		{"cat > plan.md <<-'EOF'\n\tlyceum approve testing\n\tEOF", false},
		{"tee plan.md <<'EOF'\nlyceum approve testing\nEOF", false},
		{`git commit -m "ask the owner: lyceum approve spec"`, false},
		{`git commit -m 'lyceum approve spec'`, false},
		{`git commit -am "lyceum approve spec"`, false},
		{`git commit --message="lyceum approve spec"`, false},
		{`git commit --message "lyceum approve spec"`, false},
		{`git commit -m"lyceum approve spec"`, false},
		{`git -C sub commit -m "lyceum approve spec"`, false},
		{`git tag -a v1 -m "lyceum approve spec"`, false},
		{`git add -A && git commit -q -m "lyceum approve spec" && git status`, false},
		{"git commit -m \"$(cat <<'EOF'\nlyceum approve spec, don't (forget)\nEOF\n)\"", false},
		{"git commit -F - <<'EOF'\nlyceum approve spec\nEOF", false},
		{"# don't run it\ncat > p.md <<'EOF'\nlyceum approve spec\nEOF", false},

		// Real approvals beside data.
		{"cat > p.md <<'EOF'\ntext\nEOF\nlyceum approve spec", true},
		{"cat > p.md <<'EOF' && lyceum approve spec\ntext\nEOF", true},
		{`git commit -m "x" && lyceum approve spec`, true},
		{`git commit -m "x"; lyceum approve spec`, true},
		{`git commit -m "x" | lyceum approve spec`, true},
		{`lyceum approve spec && git commit -m "x"`, true},

		// Executed strings.
		{`sh -c "lyceum approve x"`, true},
		{`bash -lc 'lyceum approve x'`, true},
		{`eval "lyceum approve x"`, true},
		{`echo $(lyceum approve x)`, true},
		{"echo `lyceum approve x`", true},
		{`lyceum approve x | cat`, true},
		{`true; lyceum approve x`, true},
		{`true && lyceum approve x`, true},

		// A here-document or message that gets run.
		{"sh <<'EOF'\nlyceum approve spec\nEOF", true},
		{"bash -s <<'EOF'\nlyceum approve spec\nEOF", true},
		{"cat <<'EOF' | sh\nlyceum approve spec\nEOF", true},
		{"cat <<'EOF' |& sh\nlyceum approve spec\nEOF", true},
		{"cat <<'EOF' | xargs -I{} lyceum approve {}\nspec\nEOF", true},
		{"cat > s.sh <<'EOF'\nlyceum approve spec\nEOF\nsh s.sh", true},
		{"cat > s.sh <<'EOF'\nlyceum approve spec\nEOF\nchmod +x s.sh\n./s.sh", true},
		{"cat > s.py <<'EOF'\nos.system('lyceum approve spec')\nEOF\npython3 s.py", true},
		{"tee s.sh <<'EOF'\nlyceum approve spec\nEOF\n./s.sh", true},
		{"cat <<'EOF' > >(sh)\nlyceum approve spec\nEOF", true},
		{"python3 - <<'EOF'\nos.system('lyceum approve spec')\nEOF", true},
		{"x=$(cat <<'EOF'\nlyceum approve spec\nEOF\n)\n$x", true},
		{"eval \"$(cat <<'EOF'\nlyceum approve spec\nEOF\n)\"", true},
		{"cat <<EOF\n$(lyceum approve spec)\nEOF", true},
		{"cat <<EOF\n`lyceum approve spec`\nEOF", true},
		{"cat <<EOF\nEO\\\nF\nlyceum approve spec\nEOF", true},
		{"while read l; do $l; done <<'EOF'\nlyceum approve spec\nEOF", true},
		{"FOO=1 cat <<'EOF'\nlyceum approve spec\nEOF", true},
		{"./cat <<'EOF'\nlyceum approve spec\nEOF", true},
		{`git commit -m "$(lyceum approve spec)"`, true},
		{"git commit -m \"`lyceum approve spec`\"", true},
		{"git commit -m \"$(cat <<'EOF' | sh\nlyceum approve spec\nEOF\n)\"", true},
		{`git commit -m "x" -m "$(lyceum approve spec)"`, true},
		{`git commit -m "lyceum approve spec" | sh`, true},
		{`git -c alias.x='!lyceum approve spec' x -m "y"`, true},
		{`git log --grep="lyceum approve spec"`, true},
		{`bash -c 'git commit -m "lyceum approve spec"'`, true},

		// Text this reader can't trust stays whole.
		{"cat <<'EOF'\nlyceum approve spec", true},
		{`echo 'unterminated lyceum approve spec`, true},
		{"echo $'\\'' ; git commit -m ' ; lyceum approve spec ; '", true},
	}
	for _, c := range cases {
		if got := runsLyceum(c.line, "approve"); got != c.want {
			t.Errorf("runsLyceum(%q, approve) = %v; want %v", c.line, got, c.want)
		}
	}
	if g, ok := promptGate("cat > p.md <<'EOF'\nlyceum approve testing\nEOF"); ok {
		t.Errorf("promptGate of a heredoc body = %q; want none", g)
	}
	if g, ok := promptGate("cat > p.md <<'EOF'\ntext\nEOF\nlyceum approve spec"); !ok || g != "spec" {
		t.Errorf("promptGate after a heredoc = %q, %v; want spec", g, ok)
	}
}
