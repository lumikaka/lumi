import { spawnSync } from 'node:child_process';
import { lstatSync, mkdtempSync, readFileSync, readdirSync, renameSync, rmSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const defaultRepo = fileURLToPath(new URL('../../', import.meta.url));

// .cindy is a ZIP with ghost.json at its root. Import it through the normal
// application file-opening flow; do not write Cindy's installed plugin files.
export function installDev({ repo = defaultRepo, app = 'Cindy', platform = process.platform,
  run = spawnSync, log = console.log } = {}) {
  if (platform !== 'darwin') throw new Error('开发版安装目前仅支持 macOS。');
  if (!app.trim()) throw new Error('请通过 CINDY_APP 指定 Cindy 应用名称或 .app 路径。');
  const author = path.join(repo, 'build/cindy-dev/lumi-dev');
  for (const dir of ['build', 'build/cindy-dev', 'build/cindy-dev/lumi-dev']) {
    if (lstatSync(path.join(repo, dir)).isSymbolicLink()) throw new Error('开发构建目录不能是符号链接。');
  }
  const manifest = JSON.parse(readFileSync(path.join(author, 'ghost.json'), 'utf8'));
  if (manifest.id !== 'lumi-dev' || manifest.command !== 'lumi-dev') {
    throw new Error('开发包身份必须为 lumi-dev；请先运行 make cindy-dev。');
  }
  if (typeof manifest.version !== 'string' || !/^\d+\.\d+\.\d+(?:-[\w.-]+)?(?:\+[\w.-]+)?$/.test(manifest.version)) {
    throw new Error('开发包缺少有效版本号；请重新构建。');
  }
  const files = [];
  function collect(dir) {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (entry.name.startsWith('.') || entry.name === 'node_modules' || entry.name.endsWith('.cindy')) continue;
      const file = path.join(dir, entry.name);
      if (entry.isSymbolicLink()) throw new Error(`开发包不能包含符号链接：${path.relative(author, file)}`);
      if (entry.isDirectory()) collect(file);
      else if (entry.isFile()) files.push(`./${path.relative(author, file)}`);
      else throw new Error(`开发包包含不支持的文件：${path.relative(author, file)}`);
    }
  }
  collect(author);

  const packageName = `lumi-dev-${manifest.version}.cindy`;
  const archive = path.join(author, packageName);
  const temp = mkdtempSync(path.join(repo, 'build/cindy-dev/.package-'));
  function execute(command, args, cwd, message) {
    const result = run(command, args, { cwd, encoding: 'utf8', timeout: 120000 });
    if (result.error || result.status !== 0) {
      const detail = result.error?.message || result.stderr?.trim() || result.signal || `退出码 ${result.status}`;
      throw new Error(`${message}：${detail}`);
    }
  }
  try {
    // Start a fresh archive so removed source files cannot survive from an old ZIP.
    // Info-ZIP preserves ordinary Unix file modes, including bin/lumi_dev's 0755.
    const temporaryArchive = path.join(temp, packageName);
    execute('/usr/bin/zip', ['-X', '-q', temporaryArchive, ...files.sort()], author, '开发版打包失败');
    renameSync(temporaryArchive, archive);
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
  log(`开发安装包已生成：${archive}`);
  execute('/usr/bin/open', ['-a', app, archive], repo, `无法将开发包交给 ${app}，请确认 Cindy 已安装且 CINDY_APP 正确`);
  log('已提交 Lumi 开发版安装，请在 Cindy 中完成导入及所需确认；更新时选择已有的 lumi-dev 实例。');
  log('本命令不等待或核验安装结果，也不会停止或切换运行中的共享后台。');
  return archive;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    const args = process.argv.slice(2);
    if (args.length && (args.length !== 2 || args[0] !== '--app' || !args[1])) {
      throw new Error('用法：node cindy-plugins/scripts/install-dev.mjs [--app Cindy]');
    }
    installDev({ app: args[1] ?? 'Cindy' });
  } catch (error) {
    console.error(`错误：打包或提交开发版安装失败。${error.message}`);
    process.exitCode = 1;
  }
}
