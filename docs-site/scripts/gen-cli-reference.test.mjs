import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseMenu, parseHelp, render } from './gen-cli-reference.mjs';

const ROOT = `Usage:
  lyceum [flags]
  lyceum [command]

Lifecycle:
  init          Set up Lyceum in a repo
  status        Every unit | with a pipe

Change:
  add           Start a unit of work: feat, bug, doc or test

Flags:
  -h, --help      help for lyceum
      --json      Print the outcome as one JSON object on stdout
  -v, --version   version for lyceum

Use "lyceum [command] --help" for more information about a command.
`;

const ADD = `Start a unit of work: feat, bug, doc or test

Usage:
  lyceum add [feat|bug|doc|test] [title] [flags]
  lyceum add [command]

Available Commands:
  feat        
  bug         

Flags:
  -h, --help   help for add

Global Flags:
      --json   Print the outcome as one JSON object on stdout
`;

const INIT = `Set up Lyceum in a repo

Usage:
  lyceum init [flags]

Flags:
      --harness stringArray   Wire this harness: claude, codex
  -h, --help                  help for init
`;

test('menu groups and commands', () => {
  const g = parseMenu(ROOT);
  assert.deepEqual(g.map((x) => x.title), ['Lifecycle', 'Change']);
  assert.equal(g[0].commands.length, 2);
  assert.equal(g[1].commands[0].name, 'add');
});

test('flag with a value, flag without one, short flag', () => {
  const h = parseHelp(INIT);
  const harness = h.flags.find((f) => f.long === '--harness');
  assert.equal(harness.value, 'stringArray');
  assert.equal(h.flags.find((f) => f.long === '--help').short, '-h');
  const root = parseHelp(ROOT);
  assert.equal(root.flags.find((f) => f.long === '--json').value, '');
  assert.equal(root.flags.find((f) => f.long === '--version').short, '-v');
});

test('subcommands may have no description', () => {
  const h = parseHelp(ADD);
  assert.deepEqual(h.subcommands.map((s) => s.name), ['feat', 'bug']);
  assert.equal(h.globalFlags.length, 1);
  assert.equal(h.usage.length, 2);
});

test('a menu command with no description fails', () => {
  assert.throws(() => parseMenu('Lifecycle:\n  init\n\nFlags:\n'), /"init" has no description/);
});

test('an unparseable flag line fails', () => {
  assert.throws(() => parseHelp('x\n\nUsage:\n  lyceum x\n\nFlags:\n  not a flag line\n'), /cannot parse flag line/);
});

test('missing Usage fails', () => {
  assert.throws(() => parseHelp('just text\n'), /no Usage section/);
});

test('render escapes pipes and angle brackets and lists every command', () => {
  const groups = parseMenu(ROOT);
  const helps = new Map([['init', parseHelp(INIT)], ['status', parseHelp(INIT)], ['add', parseHelp(ADD)]]);
  const md = render(groups, parseHelp(ROOT), helps);
  assert.match(md, /### `lyceum init`/);
  assert.match(md, /### `lyceum add`/);
  assert.match(md, /`--harness <stringArray>`/);
  assert.match(md, /^title: CLI reference$/m);
  assert.doesNotMatch(md, /\| `(-h, )?--help/);
});
