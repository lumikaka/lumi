#!/usr/bin/env node
// Generate a local Claude Code development plugin from the same skill, hooks
// and assets as the release plugin, and register it through the official
// `claude plugin` CLI. The marketplace is a local directory, so Claude Code
// loads the plugin in place: refreshing rewrites these files and the plugin
// cache is never edited by hand.
//
//   node scripts/dev-claude.mjs [--out DIR] [--data-dir ABS] [--install | --uninstall]
import { execFileSync } from 'node:child_process';
import { accessSync, constants, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { delimiter, dirname, isAbsolute, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const pluginName = 'lumi-dev';
const marketplaceName = 'lumi-dev-local';
const selector = `${pluginName}@${marketplaceName}`;
let out = resolve(repo, 'build/claude-dev');
let dataDir = join(homedir(), '.lumi_dev');
let install = false;
let uninstall = false;
for (let i = 2; i < process.argv.length; i++) {
  const arg = process.argv[i];
  if (arg === '--install') install = true;
  else if (arg === '--uninstall') uninstall = true;
  else if (arg === '--out' || arg === '--data-dir') {
    const value = process.argv[++i];
    if (!value || value.startsWith('--')) throw new Error(`${arg} requires a value`);
    if (arg === '--out') out = resolve(value);
    else {
      if (!isAbsolute(value)) throw new Error('--data-dir must be absolute');
      dataDir = value;
    }
  } else throw new Error(`Unknown option: ${arg}`);
}
if (install && uninstall) throw new Error('Choose --install or --uninstall');
mkdirSync(out, { recursive: true });
out = realpathSync(out);
const marketplace = join(out, 'marketplace');
const marker = '.lumi-claude-dev';
const owner = `${repo}\n`;
if (existsSync(marketplace) && (!existsSync(join(marketplace, marker)) || readFileSync(join(marketplace, marker), 'utf8') !== owner)) {
  throw new Error(`Refusing to replace an unmanaged marketplace: ${marketplace}`);
}

let claude;
let registered = false;
let installed = false;
function runClaude(args) {
  return execFileSync(claude, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] });
}
// --json results put one JSON object on the last stdout line.
function lastJSON(text) {
  const line = text.trim().split('\n').at(-1);
  return JSON.parse(line);
}
if (install || uninstall) {
  const candidates = process.env.CLAUDE_BIN ? [process.env.CLAUDE_BIN]
    : (process.env.PATH || '').split(delimiter).filter(Boolean).map(dir => resolve(dir, 'claude'));
  claude = candidates.find(path => {
    try { accessSync(path, constants.X_OK); return true; } catch { return false; }
  });
  if (!claude) throw new Error('Install Claude Code first; no claude CLI was found on PATH (or set CLAUDE_BIN)');
  console.log(`Claude Code: ${claude}\n${runClaude(['--version']).trim()}`);
  const list = JSON.parse(runClaude(['plugin', 'marketplace', 'list', '--json']));
  const existing = list.find(item => item.name === marketplaceName);
  if (existing && (existing.source !== 'directory' || !existing.path || resolve(existing.path) !== marketplace)) {
    throw new Error(`${marketplaceName} already belongs to ${existing.path || existing.source}; use that checkout or run claude plugin marketplace remove ${marketplaceName} first`);
  }
  registered = Boolean(existing);
  if (registered) {
    installed = JSON.parse(runClaude(['plugin', 'list', '--json'])).some(item => item.id === selector);
  }
}

if (uninstall) {
  if (installed) console.log(runClaude(['plugin', 'uninstall', selector, '--scope', 'user', '--json']).trim());
  if (registered) console.log(runClaude(['plugin', 'marketplace', 'remove', marketplaceName, '--json']).trim());
  rmSync(marketplace, { recursive: true, force: true });
  console.log('Removed the development plugin. CLI, application data and projects were preserved.');
  process.exit(0);
}

const binary = join(out, 'lumi_dev');
accessSync(binary, constants.X_OK);
const stage = mkdtempSync(join(out, '.marketplace-'));
const previous = join(stage, 'previous');
const payload = join(stage, 'payload');
let switched = false;
try {
  const plugin = join(payload, 'plugins', pluginName);
  cpSync(join(repo, 'claude-plugins/lumi'), plugin, { recursive: true });
  cpSync(join(repo, 'claude-plugins/lumi-dev'), plugin, { recursive: true });
  rmSync(join(plugin, '.DS_Store'), { force: true });
  const mcpPath = join(plugin, '.mcp.json');
  const mcp = JSON.parse(readFileSync(mcpPath, 'utf8'));
  const server = mcp.mcpServers?.[pluginName];
  if (Object.keys(mcp.mcpServers || {}).length !== 1 || !server || server.command !== '__LUMI_ABSOLUTE_BINARY__' || 'cwd' in server) {
    throw new Error('Invalid development MCP template');
  }
  server.command = binary;
  server.args = server.args.map(arg => arg === '__LUMI_DEV_DATA_DIR__' ? dataDir : arg);
  if (server.args.includes('__LUMI_DEV_DATA_DIR__') || server.args[2] !== 'claude') throw new Error('Invalid development MCP template');
  writeFileSync(mcpPath, JSON.stringify(mcp, null, 2) + '\n');

  // All authoring instructions stay shared; only the CLI entry changes so
  // init/open/stop cannot silently invoke a release installation.
  const quote = value => `'${value.replaceAll("'", "'\\''")}'`;
  const prefix = quote(binary);
  const flags = `--environment development --data-dir ${quote(dataDir)}`;
  writeFileSync(join(plugin, 'skills/create-picture-book/references/cli.md'), `# 开发版 CLI 入口

本插件为 ${pluginName}，MCP 服务名为 ${pluginName}。仅使用当前插件的工具和下面的开发 CLI。
技能中 lumi 代表此入口；不要使用 PATH 上的 lumi 或正式安装目录。

在当前绘本文件夹运行，宿主、环境和数据目录参数放在子命令之后：

\`\`\`sh
${prefix} init ${flags} --json
${prefix} project info ${flags} --json
${prefix} open ${flags} --json
${prefix} status ${flags} --json
${prefix} stop ${flags} --json
\`\`\`

重新构建后，先结束开发任务并停止旧开发后台实例（make claude-dev-stop），再新开 Claude Code 会话或运行 /reload-plugins。
仅刷新二进制不会替换已经运行的后台。generate_page_image 需要创作协议 v6；旧后台对该工具返回 cli_upgrade_required，其余工具不受影响。开发后台与 Codex、Cindy 开发版共用同一数据目录，停止前先确认其他宿主的开发任务已结束。使用独立的测试绘本目录，应用数据隔离不会复制项目文件。
`);
  mkdirSync(join(payload, '.claude-plugin'), { recursive: true });
  writeFileSync(join(payload, '.claude-plugin/marketplace.json'), JSON.stringify({
    name: marketplaceName,
    owner: { name: 'Lumi' },
    description: 'Lumi 本地开发 marketplace，由 scripts/dev-claude.mjs 生成，仅供本机开发验证。',
    plugins: [{
      name: pluginName,
      source: `./plugins/${pluginName}`,
      description: '使用当前 Lumi 源码在 Claude Code 中开发和验证绘本创作流程。',
    }],
  }, null, 2) + '\n');
  writeFileSync(join(payload, marker), owner);
  if (existsSync(marketplace)) renameSync(marketplace, previous);
  renameSync(payload, marketplace);
  switched = true;
} catch (error) {
  if (switched) rmSync(marketplace, { recursive: true, force: true });
  if (existsSync(previous)) renameSync(previous, marketplace);
  throw error;
} finally {
  rmSync(stage, { recursive: true, force: true });
}
// Register after the files are in place; a failed CLI call leaves the new
// files and any existing registration as they are, so rerunning is safe.
if (install) {
  if (registered) console.log(runClaude(['plugin', 'marketplace', 'update', marketplaceName, '--json']).trim());
  else {
    console.log(runClaude(['plugin', 'marketplace', 'add', marketplace, '--scope', 'user', '--json']).trim());
    registered = true;
  }
  if (!installed) {
    const result = lastJSON(runClaude(['plugin', 'install', selector, '--scope', 'user', '--json']));
    console.log(JSON.stringify(result));
  }
}
console.log(`${install ? 'Installed/refreshed' : 'Generated'} ${selector}\nCLI: ${binary}\nData: ${dataDir}\nMarketplace: ${marketplace}`);
console.log('The development daemon is shared with the Codex and Cindy development plugins (same data dir).');
console.log('After a creation protocol upgrade, finish development work in every host, run make claude-dev-stop, then open a NEW Claude Code session or run /reload-plugins.');
