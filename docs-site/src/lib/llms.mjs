// Renders llms.txt, llms-full.txt and per-page Markdown from the docs collection.
// Pure functions over { id, title, description, body } so they can be tested without Astro.

export const SUMMARY = 'Lyceum is an open-source CLI for spec-driven development with a coding agent, gated by approvals and recorded test runs.';

// Mirrors the sidebar in astro.config.mjs. A page in the collection but in no group fails the build.
export const GROUPS = [
  { label: 'Start', ids: ['start/what-is-lyceum', 'start/install', 'start/quickstart'] },
  { label: 'Concepts', ids: ['concepts/units-and-gates', 'concepts/approvals', 'concepts/acceptance-criteria'] },
  { label: 'Guides', ids: ['guides/agents', 'guides/ci', 'guides/existing-projects'] },
  { label: 'Reference', ids: ['reference/commands', 'reference/cli', 'reference/config'] },
];

// The landing page is hero markup and an inline SVG: listed by title and description, body left out.
export const LANDING = 'index';

export const mdUrl = (origin, id) => `${origin.replace(/\/$/, '')}/${id}.md`;

export function pageMarkdown(entry) {
  const head = `# ${entry.title}\n\n${entry.description ? entry.description + '\n\n' : ''}`;
  return entry.id === LANDING ? head.trimEnd() + '\n' : head + entry.body.trim() + '\n';
}

function ordered(entries) {
  const byId = new Map(entries.map((e) => [e.id, e]));
  const listed = new Set([LANDING, ...GROUPS.flatMap((g) => g.ids)]);
  const missing = entries.filter((e) => !listed.has(e.id)).map((e) => e.id);
  if (missing.length) throw new Error(`llms: page(s) in no group, add them to GROUPS in src/lib/llms.mjs: ${missing.join(', ')}`);
  const absent = [...listed].filter((id) => !byId.has(id));
  if (absent.length) throw new Error(`llms: GROUPS names page(s) that do not exist: ${absent.join(', ')}`);
  return { byId };
}

const link = (origin, e) => `- [${e.title}](${mdUrl(origin, e.id)})${e.description ? `: ${e.description}` : ''}`;

export function llmsTxt(entries, origin) {
  const { byId } = ordered(entries);
  const out = ['# Lyceum', '', `> ${SUMMARY}`, '', '## Overview', '', link(origin, byId.get(LANDING)), ''];
  for (const g of GROUPS) out.push(`## ${g.label}`, '', ...g.ids.map((id) => link(origin, byId.get(id))), '');
  out.push('## Optional', '', `- [Full documentation in one file](${origin.replace(/\/$/, '')}/llms-full.txt)`, '');
  return out.join('\n');
}

export function llmsFull(entries) {
  const { byId } = ordered(entries);
  const ids = [LANDING, ...GROUPS.flatMap((g) => g.ids)];
  return `# Lyceum\n\n> ${SUMMARY}\n\n` + ids.map((id) => pageMarkdown(byId.get(id)).replace(/^# /, '## ').trimEnd()).join('\n\n') + '\n';
}
