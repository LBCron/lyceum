---
id: e0b7ea-docs-site-ci-build-on-pull-requests-depl
type: chore
title: 'Docs site CI: build on pull requests, deploy on merge, set site for the sitemap'
branch: chore/e0b7ea-docs-site-ci-build-on-pull-requests-depl
status: in-progress
gate: build
files: [.github/workflows/docs-site.yml, docs-site/astro.config.mjs, docs-site/README.md]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/e0b7ea-docs-site-ci-build-on-pull-requests-depl.md: sha256:6b97d30caf428403ace64939901aa620507e87f355f79ed7c1744f4fba5193cf
    commit: d6dec6e
    by: oritromax@gmail.com
    at: 2026-11-08T06:19:25Z
---

## What and why

Closes #15, #16 and #21 (milestone "Docs site"). The docs site is deployed by hand with a direct upload, and
nothing checks that it builds before merge. One workflow does both, and `site` turns the sitemap on.

`lyceum-docs` is a direct-upload Pages project, which Cloudflare cannot convert to Git integration, so the
deploy is a CI job that runs `wrangler pages deploy`. Per-PR preview deploys are out of scope: fork PRs cannot
read secrets and a per-branch deploy is its own design.

## Spec delta

None. No behaviour change in the CLI.

## Steps

1. `astro.config.mjs`: set `site: 'https://lyceum-docs.pages.dev'` (#16). The build then writes
   `sitemap-index.xml` and the "requires the `site` option" warning disappears.
2. `.github/workflows/docs-site.yml` (#21): on `pull_request` with `paths: [docs-site/**, the workflow]`, a
   `build` job on Node 22 runs `npm ci && npm run build` in `docs-site/`. Read-only permissions.
3. Same file (#15): a `deploy` job runs only on `push` to `main` (same paths), needs `build`, and runs
   `wrangler pages deploy dist --project-name=lyceum-docs --branch=main` from `docs-site/`, wrangler pinned to
   an exact version. It reads `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` from repo secrets, and skips
   with a notice (not a failure) while either is missing. Concurrency group so deploys never overlap.
4. `docs-site/README.md`: describe the CI build and the deploy, the two secrets, and the token permission
   (Account: Cloudflare Pages: Edit, scoped to one account).
5. Verify: `npm ci && npm run build` exits 0 and `dist/sitemap-index.xml` exists; the workflow parses with
   `actionlint`. The deploy path is proven only live, by the first merge after the secrets exist.
   `lyceum test`, then stop at `close`.

## Out of scope (follow-ups)

Per-PR preview deploys, a custom domain, generated CLI reference (#17), llms.txt (#18).

## Progress

1. Done: `site` set; build writes `sitemap-index.xml` and `sitemap-0.xml`.
2. Done: build job, with a sitemap assertion.
3. Done: deploy job on push to main, wrangler 4.158.0, skips with a notice when a secret is missing. Not exercised locally.
4. Done: README section with the secrets and token permission.
5. Done: build exits 0 (13 pages), `actionlint` clean. The deploy is proven only by the first merge.

## Log
