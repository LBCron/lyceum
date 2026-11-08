import { test } from 'node:test';
import assert from 'node:assert/strict';
import { GROUPS, LANDING, llmsTxt, llmsFull, pageMarkdown, mdUrl } from './llms.mjs';

const all = () => [LANDING, ...GROUPS.flatMap((g) => g.ids)].map((id) => ({
  id, title: `T ${id}`, description: id === 'reference/cli' ? '' : `D ${id}`, body: id === LANDING ? '<svg>hero</svg>' : `body of ${id}`,
}));

test('llms.txt: groups in sidebar order with .md links on the site origin', () => {
  const t = llmsTxt(all(), 'https://x.example/');
  const heads = [...t.matchAll(/^## (.+)$/gm)].map((m) => m[1]);
  assert.deepEqual(heads, ['Overview', ...GROUPS.map((g) => g.label), 'Optional']);
  assert.match(t, /^# Lyceum$/m);
  assert.match(t, /^> .+/m);
  assert.match(t, /\[T start\/install\]\(https:\/\/x\.example\/start\/install\.md\): D start\/install/);
  assert.match(t, /llms-full\.txt/);
});

test('a page with no description has no trailing colon', () => {
  assert.match(llmsTxt(all(), 'https://x.example'), /\[T reference\/cli\]\(https:\/\/x\.example\/reference\/cli\.md\)$/m);
});

test('a page in no group fails the build', () => {
  const e = [...all(), { id: 'guides/new', title: 'N', description: '', body: 'x' }];
  assert.throws(() => llmsTxt(e, 'https://x.example'), /no group.*guides\/new/);
  assert.throws(() => llmsFull(e), /no group/);
});

test('a group naming a missing page fails the build', () => {
  assert.throws(() => llmsTxt(all().slice(1), 'https://x.example'), /do not exist/);
});

test('the landing body is left out, every other body is in llms-full', () => {
  const f = llmsFull(all());
  assert.doesNotMatch(f, /<svg/);
  for (const g of GROUPS) for (const id of g.ids) assert.ok(f.includes(`body of ${id}`), id);
  assert.ok(f.indexOf('body of start/install') < f.indexOf('body of concepts/approvals'));
});

test('per-page markdown starts with the title and description', () => {
  assert.equal(pageMarkdown({ id: 'a', title: 'A', description: 'd', body: '\nb\n' }), '# A\n\nd\n\nb\n');
  assert.equal(pageMarkdown({ id: LANDING, title: 'L', description: 'd', body: '<svg/>' }), '# L\n\nd\n');
});

test('mdUrl trims a trailing slash on the origin', () => {
  assert.equal(mdUrl('https://x.example/', 'a/b'), 'https://x.example/a/b.md');
});
