---
id: 0c520e-docs-site-generate-llms-txt-and-llms-ful
type: chore
title: 'Docs site: generate llms.txt and llms-full.txt at build time'
branch: chore/0c520e-docs-site-generate-llms-txt-and-llms-ful
status: in-progress
gate: build
files: [docs-site/src/lib/llms.mjs, docs-site/src/lib/llms.test.mjs, docs-site/src/pages/llms.txt.ts, docs-site/src/pages/llms-full.txt.ts, "docs-site/src/pages/[...slug].md.ts", docs-site/package.json, docs-site/README.md, .github/workflows/docs-site.yml]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/0c520e-docs-site-generate-llms-txt-and-llms-ful.md: sha256:d69e01c0397dbe648f133fe88bd1a9d7c167059a28bb259b06ad402b088d1f88
    commit: 592c3a6
    by: oritromax@gmail.com
    at: 2026-11-08T07:21:03Z
---

## What and why

Closes #18. Serve `llms.txt`, `llms-full.txt` and the raw Markdown of every page so agents can read the docs
directly. No new dependency and no CLI change: Astro endpoints read the docs content collection at build time
(a probe build produced `/llms.txt` and a `.md` for every page), so the files cannot drift from the pages, and
the generated CLI reference from #17 is included because it is in the collection.

## Spec delta

None. No behaviour change in the CLI.

## Steps

1. `src/lib/llms.mjs`: pure functions over `{id, title, description, body}` entries. `pageMarkdown` prepends
   `# <title>` and the description to the body; `llmsTxt` renders the llms.txt format (H1, a one-line
   blockquote summary, then one section per sidebar group with `- [title](url): description` links to the
   `.md` URLs); `llmsFull` concatenates every page in sidebar order. Group order and membership come from one
   list in this file that mirrors the sidebar, and the build fails if a page is in the collection but in no
   group, so a new page cannot be silently left out.
2. `src/pages/llms.txt.ts`, `llms-full.txt.ts` and `[...slug].md.ts` are the three endpoints. The landing page
   (`index.mdx`, hero markup and an inline SVG) is listed by title and description only and its body is left
   out of `llms-full.txt`, since its content is prose that repeats other pages.
3. `src/lib/llms.test.mjs` (`node --test`): ordering, link URLs built from the `site` origin, the
   unlisted-page failure, the landing-page exclusion. `package.json` `test` runs these and the generator tests.
4. `.github/workflows/docs-site.yml`: the existing asserts also require `dist/llms.txt`, `dist/llms-full.txt`
   and `dist/start/install.md`. `docs-site/README.md`: one paragraph on the three outputs.
5. Verify: `npm test`; build exits 0 and the three outputs exist; every link in `llms.txt` resolves to a file
   in `dist/`; `llms-full.txt` contains the CLI reference and no `<svg`; `actionlint`. `lyceum test`, then stop
   at `close`.

## Out of scope (follow-ups)

Content negotiation (`Accept: text/markdown` on the HTML URLs), a "copy as Markdown" button, per-version
files (#20), and Open Graph images (#19).

## Progress

## Log
