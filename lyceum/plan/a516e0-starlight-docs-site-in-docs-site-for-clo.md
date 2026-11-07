---
id: a516e0-starlight-docs-site-in-docs-site-for-clo
type: chore
title: Starlight docs site in docs-site for Cloudflare Pages
branch: chore/a516e0-starlight-docs-site-in-docs-site-for-clo
status: in-progress
gate: build
files: [docs-site/package.json, docs-site/package-lock.json, docs-site/astro.config.mjs, docs-site/tsconfig.json, docs-site/.gitignore, docs-site/README.md, docs-site/public/favicon.svg, docs-site/public/_headers, docs-site/src/content.config.ts, docs-site/src/content/docs/index.mdx, docs-site/src/content/docs/start/what-is-lyceum.md, docs-site/src/content/docs/start/install.md, docs-site/src/content/docs/start/quickstart.md, docs-site/src/content/docs/concepts/units-and-gates.md, docs-site/src/content/docs/concepts/approvals.md, docs-site/src/content/docs/concepts/acceptance-criteria.md, docs-site/src/content/docs/guides/agents.md, docs-site/src/content/docs/guides/ci.md, docs-site/src/content/docs/guides/existing-projects.md, docs-site/src/content/docs/reference/commands.md, docs-site/src/content/docs/reference/config.md]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/a516e0-starlight-docs-site-in-docs-site-for-clo.md: sha256:e27e62a5ca7d1f31afc97cd63e77550daf5e650b361eb54b9bb37c5d00ea0831
    commit: 9fcb1e2
    by: oritromax@gmail.com
    at: 2026-11-07T12:52:41Z
---

## What and why

Lyceum has a README and one `docs/ci.md`; the model (units, gates, approvals, AC-to-test mapping) needs a
teaching surface. Scaffold a Starlight site in `docs-site/` that deploys to Cloudflare Pages. The README stays
as it is and the site is additive. `lyceum.pages.dev` is already taken by another site, so the Pages project
name is picked at connect time (owner action, outside this unit).

New dependencies (ask-first, covered by this approval): `astro` and `@astrojs/starlight`, pinned exactly in
`docs-site/package.json`, in a separate npm project that does not touch the root `package.json` (the published
npm package ships only `bin/` and `README.md`).

## Spec delta

None. No behaviour change in the CLI.

## Steps

1. Scaffold `docs-site/` (Astro + Starlight, static output, Pagefind search, edit-on-GitHub links to this repo).
2. Four sections in the sidebar: Start, Concepts, Guides, Reference. Content drawn from the README, `docs/ci.md`
   and the spec; nothing invented. Command reference comes from `lyceum --help` output.
3. `public/_headers` with cache and security headers; no `_redirects` yet.
4. `docs-site/README.md`: local dev, build, and the Cloudflare Pages settings (root `docs-site`, build
   `npm run build`, output `dist`).
5. `npm ci && npm run build` in `docs-site/` must exit 0; open the built pages and check the sidebar and links.
   `lyceum test`, then stop at that gate.

## Out of scope (follow-ups)

Docs versioning, `llms.txt`, generated CLI reference, OG images, a CI build job, the Pages project and domain.

## Progress

1. Done: scaffold, `astro` 7.4.7 and `@astrojs/starlight` 0.42.10 pinned exactly.
2. Done: 12 pages (landing plus Start, Concepts, Guides, Reference), gate-flow diagram as inline SVG.
3. Done: `public/_headers`.
4. Done: `docs-site/README.md`.
5. Done: `npm ci` and `npm run build` exit 0, 13 pages built; sidebar groups and internal links checked on every page.
   `lyceum test` pending.

## Log
