#!/usr/bin/env node
// Generate a local development plugin from the same skill/assets as releases.
import { execFileSync } from 'node:child_process';
import { accessSync, constants, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { delimiter, dirname, isAbsolute, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const selector = 'lumi-dev@lumi-dev-local';
const marketplaceName = 'lumi-dev-local';
let out = resolve(repo, 'build/codex-dev');
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
const marker = '.lumi-codex-dev';
const owner = `${repo}\n`;
if (existsSync(marketplace) && (!existsSync(join(marketplace, marker)) || readFileSync(join(marketplace, marker), 'utf8') !== owner)) {
  throw new Error(`Refusing to replace an unmanaged marketplace: ${marketplace}`);
}

let codex;
let registered = false;
const env = { ...process.env, CODEX_HOME: process.env.CODEX_HOME || join(homedir(), '.codex') };
function runCodex(args) {
  return execFileSync(codex, args, { env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] });
}
if (install || uninstall) {
  if (process.platform !== 'darwin') throw new Error('Only macOS Codex App is supported');
  const candidates = ['/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex',
    ...(process.env.PATH || '').split(delimiter).map(dir => resolve(dir, 'codex'))];
  codex = candidates.find(path => {
    try { accessSync(path, constants.X_OK); return true; } catch { return false; }
  });
  if (!codex) throw new Error('Install Codex App first; no bundled or PATH codex was found');
  if (!isAbsolute(env.CODEX_HOME)) throw new Error('CODEX_HOME must be absolute');
  console.log(`Codex: ${codex}\n${runCodex(['--version']).trim()}\nCODEX_HOME: ${env.CODEX_HOME}`);
  const list = JSON.parse(runCodex(['plugin', 'marketplace', 'list', '--json']));
  const existing = list.marketplaces.find(item => item.name === marketplaceName);
  if (existing && resolve(existing.root) !== marketplace) {
    throw new Error(`${marketplaceName} already belongs to ${existing.root}; use that checkout or remove its registration first`);
  }
  registered = Boolean(existing);
}

if (uninstall) {
  if (registered) {
    console.log(runCodex(['plugin', 'remove', selector, '--json']).trim());
    console.log(runCodex(['plugin', 'marketplace', 'remove', marketplaceName, '--json']).trim());
  }
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
let registrationAttempted = false;
try {
  const plugin = join(payload, 'plugins/lumi-dev');
  cpSync(join(repo, 'codex-plugins/lumi'), plugin, { recursive: true });
  cpSync(join(repo, 'codex-plugins/lumi-dev'), plugin, { recursive: true });
  const mcpPath = join(plugin, '.mcp.json');
  const mcp = JSON.parse(readFileSync(mcpPath, 'utf8'));
  const server = mcp.mcpServers['lumi-dev'];
  if (server.command !== '__LUMI_ABSOLUTE_BINARY__' || 'cwd' in server) throw new Error('Invalid development MCP template');
  server.command = binary;
  server.args = server.args.map(arg => arg === '__LUMI_DEV_DATA_DIR__' ? dataDir : arg);
  writeFileSync(mcpPath, JSON.stringify(mcp, null, 2) + '\n');

  // Keep all authoring instructions and references shared. Only the CLI entry
  // changes so init/open/stop cannot silently invoke the release installation.
  const quote = value => `'${value.replaceAll("'", "'\\''")}'`;
  const prefix = quote(binary);
  const flags = `--environment development --data-dir ${quote(dataDir)}`;
  writeFileSync(join(plugin, 'skills/create-picture-book/references/cli.md'), `# 开发版 CLI 入口

本插件为 lumi-dev，MCP 服务名为 lumi-dev。仅使用当前插件的工具和下面的开发 CLI。
技能中 lumi 代表此入口；不要使用 PATH 上的 lumi 或正式安装目录。

在当前绘本文件夹运行，环境和数据目录参数放在子命令之后：

\`\`\`sh
${prefix} init ${flags} --json
${prefix} project info ${flags} --json
${prefix} open ${flags} --json
${prefix} status ${flags} --json
${prefix} stop ${flags} --json
\`\`\`

重新构建后，先结束开发任务并停止旧开发后台实例，再新开 Codex App 会话。
仅刷新二进制不会替换已经运行的后台。使用独立的测试绘本目录，应用数据隔离不会复制项目文件。
`);
  mkdirSync(join(payload, '.agents/plugins'), { recursive: true });
  writeFileSync(join(payload, '.agents/plugins/marketplace.json'), JSON.stringify({
    name: marketplaceName,
    interface: { displayName: 'Lumi Development' },
    plugins: [{ name: 'lumi-dev', source: { source: 'local', path: './plugins/lumi-dev' },
      policy: { installation: 'AVAILABLE', authentication: 'ON_INSTALL' }, category: 'Productivity' }],
  }, null, 2) + '\n');
  writeFileSync(join(payload, marker), owner);
  if (existsSync(marketplace)) renameSync(marketplace, previous);
  renameSync(payload, marketplace);
  switched = true;
  if (install) {
    registrationAttempted = true;
    console.log(runCodex(['plugin', 'marketplace', 'add', marketplace, '--json']).trim());
    console.log(runCodex(['plugin', 'add', selector, '--json']).trim());
  }
} catch (error) {
  if (switched) rmSync(marketplace, { recursive: true, force: true });
  if (existsSync(previous)) renameSync(previous, marketplace);
  if (registrationAttempted) {
    try {
      if (registered && existsSync(marketplace)) {
        runCodex(['plugin', 'marketplace', 'add', marketplace, '--json']);
        runCodex(['plugin', 'add', selector, '--json']);
      } else if (!registered) {
        runCodex(['plugin', 'remove', selector, '--json']);
        runCodex(['plugin', 'marketplace', 'remove', marketplaceName, '--json']);
      }
    } catch (restoreError) { console.error(`Could not restore development plugin registration: ${restoreError.message}`); }
  }
  throw error;
} finally {
  rmSync(stage, { recursive: true, force: true });
}
console.log(`${install ? 'Installed/refreshed' : 'Generated'} ${selector}\nCLI: ${binary}\nData: ${dataDir}\nMarketplace: ${marketplace}`);
console.log('After rebuilding, finish active development work and run make codex-dev-stop. Open a NEW Codex App chat.');
