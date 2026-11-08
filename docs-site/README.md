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

## CI and deploy

`.github/workflows/docs-site.yml` runs on changes under `docs-site/`:

- On a pull request it runs `npm ci && npm run build` and checks that `dist/sitemap-index.xml` was written.
- On a push to `main` it builds, then uploads `dist` to the `lyceum-docs` Cloudflare Pages project with
  `wrangler pages deploy`. The project is a direct-upload project, so the deploy is a CI job rather than Pages'
  Git integration. Preview deploys for pull requests are not set up.

The deploy needs two repository secrets. Without them it skips with a notice and does not fail.

| Secret | Value |
|---|---|
| `CLOUDFLARE_API_TOKEN` | A custom token with one permission: Account, Cloudflare Pages, Edit, limited to one account |
| `CLOUDFLARE_ACCOUNT_ID` | The Cloudflare account ID (not secret) |
