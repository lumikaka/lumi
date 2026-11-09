#!/usr/bin/env node
// Assemble host-specific plugin skills from plugin-skills/.
//
// Source files may contain line-level host blocks:
//   <!-- host:codex -->   lines kept only for Codex
//   <!-- host:claude -->  lines kept only for Claude Code (may follow a codex block)
//   <!-- /host -->        end of the block
// Marker lines themselves are removed. Files without markers are shared verbatim.
//
//   node scripts/build-skills.mjs          write codex-plugins/ and claude-plugins/ skills
//   node scripts/build-skills.mjs --check  fail when a committed skill differs from the source
import { mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
export const source = join(repo, 'plugin-skills');
export const outputs = {
  codex: join(repo, 'codex-plugins/lumi/skills'),
  claude: join(repo, 'claude-plugins/lumi/skills'),
};
const marker = /^<!-- (host:(codex|claude)|\/host) -->$/;

export function assemble(text, host, name = 'input') {
  if (!(host in outputs)) throw new Error(`Unknown host: ${host}`);
  const kept = [];
  let block = null;
  text.split('\n').forEach((line, index) => {
    const match = line.match(marker);
    if (!match) {
      if (block === null || block === host) kept.push(line);
      return;
    }
    if (match[1] === '/host') {
      if (block === null) throw new Error(`${name}:${index + 1}: <!-- /host --> without an open block`);
      block = null;
    } else {
      block = match[2];
    }
  });
  if (block !== null) throw new Error(`${name}: unterminated host block`);
  return kept.join('\n');
}

function files(dir) {
  return readdirSync(dir).sort().flatMap(entry => {
    const path = join(dir, entry);
    return statSync(path).isDirectory() ? files(path) : [path];
  });
}

// Returns {relativePath: Buffer} for one host. Non-Markdown files are copied as bytes.
export function build(host) {
  const result = {};
  for (const path of files(source)) {
    const rel = relative(source, path).split('\\').join('/');
    const bytes = readFileSync(path);
    result[rel] = rel.endsWith('.md') ? Buffer.from(assemble(bytes.toString('utf8'), host, rel), 'utf8') : bytes;
  }
  return result;
}

export function diff(host) {
  const expected = build(host);
  const root = outputs[host];
  const actual = new Set();
  const problems = [];
  try {
    for (const path of files(root)) actual.add(relative(root, path).split('\\').join('/'));
  } catch { /* missing output directory: every file is reported below */ }
  for (const [rel, bytes] of Object.entries(expected)) {
    if (!actual.has(rel)) problems.push(`missing ${host}/${rel}`);
    else if (!readFileSync(join(root, rel)).equals(bytes)) problems.push(`differs ${host}/${rel}`);
    actual.delete(rel);
  }
  for (const rel of actual) problems.push(`extra ${host}/${rel}`);
  return problems;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const check = process.argv.includes('--check');
  const unknown = process.argv.slice(2).filter(arg => arg !== '--check');
  if (unknown.length) throw new Error(`Unknown option: ${unknown.join(' ')}`);
  if (check) {
    const problems = Object.keys(outputs).flatMap(diff);
    if (problems.length) {
      console.error(`Generated skills are out of date; run node scripts/build-skills.mjs\n${problems.join('\n')}`);
      process.exit(1);
    }
    console.log('Skills match plugin-skills/.');
  } else {
    for (const [host, root] of Object.entries(outputs)) {
      for (const rel of Object.keys(build(host))) rmSync(join(root, rel), { force: true });
      for (const [rel, bytes] of Object.entries(build(host))) {
        mkdirSync(dirname(join(root, rel)), { recursive: true });
        writeFileSync(join(root, rel), bytes);
      }
      console.log(`Wrote ${relative(repo, root)}`);
    }
  }
}
