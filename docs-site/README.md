# Lyceum docs site

The Lyceum documentation site, built with Astro and Starlight. It is a separate npm project from the repo root; the published `lyceum` npm package does not include it.

Pages are Markdown and MDX under `src/content/docs/`. The sidebar is set in `astro.config.mjs`.

## Local development

Requires Node.js 22.23.4 or later.

```sh
npm ci
npm run dev
```

## Build

```sh
npm run build
```

The static site is written to `dist/`, including the Pagefind search index. `npm run preview` serves the build locally.

## Cloudflare Pages

| Setting | Value |
|---|---|
| Root directory | `docs-site` |
| Build command | `npm run build` |
| Build output directory | `dist` |

Response headers are set in `public/_headers`.
