---
id: d20234-add-the-lyceum-logo-to-the-docs-site-fav
type: chore
title: Add the Lyceum logo to the docs site, favicon and README
branch: chore/d20234-add-the-lyceum-logo-to-the-docs-site-fav
status: implemented
gate: close
files: [README.md, docs/assets/lockup.svg, docs/assets/lockup-dark.svg, docs-site/astro.config.mjs, docs-site/public/favicon.svg, docs-site/public/favicon-32.png, docs-site/public/apple-touch-icon.png, docs-site/src/assets/mark.svg, docs-site/src/assets/mark-dark.svg]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/d20234-add-the-lyceum-logo-to-the-docs-site-fav.md: sha256:53eb44251ed39406b1fad876e75837ad585a69de4dfbf2f582a462c3c87b2f7b
    commit: 0b91e12
    by: oritromax@gmail.com
    at: 2026-11-08T06:01:35Z
---

## What and why

Lyceum has no logo (issue #22). The mark is final ("Cut ends": an L with diagonal end cuts and an amber
pillar). Land it on the docs site header, the favicon and the README. The site's blue accent stays as it is;
retheming it is a separate decision.

## Spec delta

None. No behaviour change in the CLI.

## Steps

1. Add `mark.svg` and `mark-dark.svg` to `docs-site/src/assets/` and wire them as the Starlight `logo`
   (light/dark). The site title text stays.
2. Replace `docs-site/public/favicon.svg` with the new tile and add `favicon-32.png` and
   `apple-touch-icon.png` as fallbacks, linked from `head`.
3. Add `lockup.svg` and `lockup-dark.svg` to `docs/assets/` and a `<picture>` header at the top of
   `README.md` that follows `prefers-color-scheme`.
4. `npm ci && npm run build` in `docs-site/` exits 0; check the header in light and dark. `lyceum test`.

## Out of scope (follow-ups)

Site accent colour, Open Graph images (#19), a landing-page hero logo.

## Progress

1. Done.
2. Done.
3. Done.
4. Done: build exits 0 (13 pages); header mark checked in light and dark.

## Log
