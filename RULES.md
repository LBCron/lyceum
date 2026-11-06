# RULES.md: Lyceum

**Owner-owned and immutable to the agent.** The agent may propose additions; only the owner adds them.

## Principles

1. **Never guess.** Anything unspecified: ask, or mark `[NEEDS CLARIFICATION: …]`.
2. **Done means verifiable, not describable.** Prove it with a real run before claiming it.
3. **Gates are gates.** No skipping ahead, no "while I'm here".
4. **A failed check stops the work.** Report it. No silent retries, no workarounds.
5. **The spec is the contract.** If code needs something the spec doesn't say, the spec changes first, by the owner.

## ✅ Always

- Read AGENTS.md and RULES.md at session start; load the rest just in time.
- Show real command output for every claim that something works.
- Cite the AC IDs a change proves, in the test names.
- Record every owner decision as a new row in `lyceum/decisions.md`; never edit or renumber old rows.

## ⚠️ Ask first

- Adding, removing or upgrading a dependency.
- Changing a persisted format: plan frontmatter, record frontmatter, event or run schemas, TESTING.md frontmatter.
- Changing anything in `lyceum/spec/`, `lyceum/SPEC.md`, AGENTS.md or TESTING.md.
- Widening scope beyond the approved task.
- Any publish (npm, brew, GitHub release) or anything else hard to reverse.

## 🚫 Never

- Never edit RULES.md.
- Never commit on `main` or merge; never use an agent identity in git.
- Never commit secrets, tokens or credentials.
- Never delete, skip or weaken a failing test.
- Never write application code before the spec gate is approved.
- Never put design notes, reviews, local paths or tooling references in the repo.
