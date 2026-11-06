---
name: lyceum-test
description: Use when the owner describes one test case to add to a Lyceum project, to write it and prove it passes as a test unit.
---

# lyceum-test

Run `lyceum status` first. If it names a unit in flight, its printed next action wins over anything here.
Every `lyceum` command ends with a `Next:` line; follow it.

Gates for a test: `write` → `run` (`lyceum test`) → `close`. No owner approval.

1. No unit on this branch for this test: run `lyceum add test "<title>"`. With no title, ask for one.
2. `write`: pick the test's level (unit, integration, …) from TESTING.md and write the one test the owner
   described, in a file matching `test_files`. If it proves an AC, carry the ID in its name or a comment
   (e.g. `TestExport_AC14`). Change only test files. Commit.
3. `run`: with a clean tree, run `lyceum test`. It clears `run` when the new test passes in the full suite.
   Quote the result; never claim it from reasoning.
4. The new test fails on the current code: STOP. Don't change the test or the code. Tell the owner the code does
   not do what the test expects, and offer to keep it as a bug. With their go-ahead, run `lyceum close`: it
   closes this unit and starts a bug on this branch at `diagnose`, with this test as its repro. Continue with
   `lyceum-bug`.
5. Once `run` clears, run `lyceum close`. It sets the plan to closed and opens a PR, or prints the push and merge
   commands. A test unit has no owner approval, so there is no plan to amend.

No TESTING.md yet: `lyceum test` says so. Draft it from the detected frameworks without interviewing, then tell
the owner to run `lyceum approve testing` in their own terminal.

Approvals are the owner's: never run `lyceum approve` on your own initiative. One approval per command: the hook
denies two in one shell call. After a gate clears, any confirmation to proceed is a structured yes/no, never an open
question.

Never change application code in a test unit, weaken a test to make it pass, edit CLI-owned plan fields or
`lyceum/events.jsonl`, or work around a hook refusal (stop and tell the owner).
