---
id: db7aa4-ac087-set-user-name-in-the-extra-repos
type: test
title: 'ac087: set user.name in the extra repos'
branch: test/db7aa4-ac087-set-user-name-in-the-extra-repos
status: in-progress
gate: write
approvals: []
---

## What and why

`ac087_init_brownfield` (AC87) creates four extra repos and sets only `user.email` in each, so its commits
take the author name from the OS account. GitHub's Ubuntu runner account has an empty name, and git refuses the
commit (`fatal: empty ident name`). Set `user.name` alongside `user.email` in each extra repo.

## Spec delta

## Steps

1. Add `config user.name "Test Owner"` after each extra repo's `user.email` line in
   `cmd/lyceum/testdata/script/ac087_init_brownfield.txtar`.
2. `lyceum test`; then `lyceum close`.

## Progress

## Log
