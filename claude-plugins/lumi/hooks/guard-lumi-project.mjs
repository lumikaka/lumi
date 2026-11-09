#!/usr/bin/env node
// Claude Code PreToolUse(Bash) guard for Lumi projects.
//
// Outside a Lumi project the hook exits 0 without output, so the normal
// permission flow runs unchanged. Inside one (a directory, or an ancestor,
// holding project.sqlite or .lumi/), it denies Bash commands that name the
// project database, .lumi/ or assets/ directly: those files are only written
// through the Lumi MCP tools and the Lumi CLI. This is a best-effort text check,
// paired with the Read/Edit deny rules suggested in claude-plugins/README.md.
import { existsSync, readFileSync, realpathSync, statSync } from 'node:fs';
import { dirname, isAbsolute, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const protectedNames = ['project.sqlite', '.lumi', 'assets'];

export function lumiProjectRoot(start) {
  if (!start || !isAbsolute(start)) return null;
  // Walk the logical path; protectedReference also checks its real path.
  let dir = resolve(start);
  for (;;) {
    try {
      if (statSync(join(dir, 'project.sqlite')).isFile()) return dir;
    } catch { /* not here */ }
    try {
      if (statSync(join(dir, '.lumi')).isDirectory()) return dir;
    } catch { /* not here */ }
    const parent = dirname(dir);
    if (parent === dir) return null;
    dir = parent;
  }
}

function escape(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// Returns the protected name a command touches, or null.
export function protectedReference(command, root) {
  const boundary = String.raw`(?:^|[\s'"=:;&|<>(\x60])`;
  const end = String.raw`(?=$|[\s'"/;&|<>)\x60*?\[])`;
  const roots = [root];
  try { roots.push(realpathSync(root)); } catch { /* keep given root */ }
  for (const name of protectedNames) {
    const bare = name === 'project.sqlite' ? String.raw`project\.sqlite(?:-[A-Za-z]+)?` : escape(name);
    // Relative to the project root: name, ./name, name/..., project.sqlite-wal.
    if (new RegExp(`${boundary}(?:\\./)?${bare}${end}`).test(command)) return name;
    // Absolute paths into this project, with or without a trailing segment.
    const collapsed = command.replace(/\/{2,}/g, '/');
    for (const base of roots) {
      const prefix = escape(base.endsWith(sep) ? base : base + sep);
      if (new RegExp(`${prefix}${bare}${end}`).test(collapsed)) return name;
    }
  }
  return null;
}

export function decide(input, env = process.env) {
  if (input?.tool_name !== 'Bash') return null;
  const command = String(input?.tool_input?.command ?? '');
  if (!command) return null;
  for (const start of [input.cwd, env.CLAUDE_PROJECT_DIR]) {
    const root = lumiProjectRoot(start);
    if (!root) continue;
    const name = protectedReference(command, root);
    if (!name) return null;
    return {
      hookSpecificOutput: {
        hookEventName: 'PreToolUse',
        permissionDecision: 'deny',
        permissionDecisionReason: `Lumi 项目写保护：${root} 是 Lumi 绘本项目，命令直接访问了 ${name}。project.sqlite、.lumi/ 与 assets/ 只能通过 Lumi MCP 工具或 lumi CLI 读写；请改用对应工具（看图用 read_image，导入用 import_image，查看项目用 project_overview / web_url）。`,
      },
    };
  }
  return null;
}

if (process.argv[1] && existsSync(process.argv[1]) && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))) {
  let input;
  try {
    input = JSON.parse(readFileSync(0, 'utf8'));
  } catch {
    process.exit(0); // Unparseable input: make no decision.
  }
  const result = decide(input);
  if (result) process.stdout.write(JSON.stringify(result) + '\n');
  process.exit(0);
}
