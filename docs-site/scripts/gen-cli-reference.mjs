// Generates src/content/docs/reference/cli.md from the lyceum binary's own help output.
// Usage: node scripts/gen-cli-reference.mjs   (binary: $LYCEUM_BIN, default ../dist/lyceum)
// It fails, with a message, when the help does not look the way it expects, so a changed help
// template breaks the build instead of producing a wrong page.
import { execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const SECTION = /^([A-Z][A-Za-z ]*):$/;
const FLAG = /^\s+(?:(-\w), )?(--[\w-]+)(?: (\w+))?\s{2,}(\S.*)$/;
const ENTRY = /^ {2}(\S+)\s{2,}(\S.*)$/;
const NAMELESS = /^ {2}(\S+)\s*$/;

export function parseMenu(text) {
  const groups = [];
  let current = null;
  for (const line of text.split('\n')) {
    const sec = line.match(SECTION);
    if (sec) {
      if (['Flags', 'Global Flags'].includes(sec[1])) break;
      if (sec[1] === 'Usage') { current = null; continue; }
      current = { title: sec[1], commands: [] };
      groups.push(current);
      continue;
    }
    if (!current) continue;
    if (line.trim() === '') { current = null; continue; }
    const m = line.match(ENTRY);
    if (m) { current.commands.push({ name: m[1], summary: m[2].trim() }); continue; }
    const bare = line.match(NAMELESS);
    if (bare) throw new Error(`command "${bare[1]}" has no description in the menu`);
    throw new Error(`cannot parse menu line: ${JSON.stringify(line)}`);
  }
  if (groups.length === 0) throw new Error('no command groups found in the root help');
  return groups;
}

export function parseHelp(text) {
  const lines = text.split('\n');
  const out = { description: '', usage: [], subcommands: [], flags: [], globalFlags: [] };
  const head = [];
  let i = 0;
  while (i < lines.length && lines[i] !== 'Usage:') head.push(lines[i++]);
  out.description = head.join('\n').trim();
  let section = null;
  for (; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim() === '') { section = null; continue; }
    const sec = line.match(SECTION);
    if (sec) { section = sec[1]; continue; }
    if (section === 'Usage') out.usage.push(line.trim());
    else if (section === 'Available Commands') {
      const m = line.match(ENTRY) ?? line.match(NAMELESS);
      if (!m) throw new Error(`cannot parse subcommand line: ${JSON.stringify(line)}`);
      out.subcommands.push({ name: m[1], summary: (m[2] ?? '').trim() });
    } else if (section === 'Flags' || section === 'Global Flags') {
      const m = line.match(FLAG);
      if (!m) throw new Error(`cannot parse flag line: ${JSON.stringify(line)}`);
      const flag = { short: m[1] ?? '', long: m[2], value: m[3] ?? '', desc: m[4].trim() };
      (section === 'Flags' ? out.flags : out.globalFlags).push(flag);
    }
  }
  if (out.usage.length === 0) throw new Error('no Usage section in the help output');
  return out;
}

const cell = (s) => s.replace(/\|/g, '\\|').replace(/</g, '&lt;').replace(/>/g, '&gt;');

function flagRow(f) {
  const name = [f.short, f.long + (f.value ? ` <${f.value}>` : '')].filter(Boolean).join(', ');
  // inside a code span entities are not decoded, so the raw characters are right here
  return `| \`${name.replace(/\|/g, '\\|')}\` | ${cell(f.desc)} |`;
}

function flagTable(flags) {
  const rows = flags.filter((f) => f.long !== '--help');
  if (rows.length === 0) return [];
  return ['| Flag | Meaning |', '|---|---|', ...rows.map(flagRow), ''];
}

export function render(groups, rootHelp, helps) {
  const md = [
    '---',
    'title: CLI reference',
    'description: Every lyceum command and flag, generated from the binary.',
    '---',
    '',
    'This page is generated from `lyceum --help` and each command\'s help when the site is built, so it matches the CLI. Hidden commands used by skills, hooks and CI are listed under [Commands](/reference/commands/#internal).',
    '',
    '## Global flags',
    '',
    ...flagTable(rootHelp.flags),
  ];
  for (const g of groups) {
    md.push(`## ${g.title}`, '');
    for (const c of g.commands) {
      const h = helps.get(c.name);
      md.push(`### \`lyceum ${c.name}\``, '', cell(h.description || c.summary), '', '```text', ...h.usage, '```', '');
      md.push(...flagTable(h.flags));
      if (h.subcommands.length) {
        md.push('Subcommands:', '', ...h.subcommands.map((s) => `- \`${s.name}\`${s.summary ? ` — ${cell(s.summary)}` : ''}`), '');
      }
    }
  }
  return md.join('\n').replace(/\n{3,}/g, '\n\n').trimEnd() + '\n';
}

function run(bin, args) {
  try {
    return execFileSync(bin, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  } catch (e) {
    throw new Error(`could not run ${bin} ${args.join(' ')}: ${e.message.split('\n')[0]}`);
  }
}

export function generate(bin) {
  const rootText = run(bin, ['--help']);
  const groups = parseMenu(rootText);
  const rootHelp = parseHelp(rootText);
  const helps = new Map();
  for (const g of groups) for (const c of g.commands) helps.set(c.name, parseHelp(run(bin, [c.name, '--help'])));
  return render(groups, rootHelp, helps);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const bin = process.env.LYCEUM_BIN || resolve(here, '../../dist/lyceum');
  try {
    const md = generate(bin);
    const target = resolve(here, '../src/content/docs/reference/cli.md');
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, md);
    console.log(`gen-cli-reference: wrote ${target}`);
  } catch (e) {
    console.error(`gen-cli-reference: ${e.message}`);
    console.error('Build the binary first: go build -o dist/lyceum ./cmd/lyceum (from the repo root), or set LYCEUM_BIN.');
    process.exit(1);
  }
}
