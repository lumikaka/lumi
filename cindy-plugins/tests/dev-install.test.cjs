'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const { mkdtempSync, copyFileSync, mkdirSync, writeFileSync, readFileSync, rmSync, readdirSync } = require('node:fs');
const os = require('node:os');
const path = require('node:path');

// Exercise the real Make recipes without compiling or touching Cindy's installation.
function runMake(t, target, buildStatus = 0, installStatus = 0) {
  const root = mkdtempSync(path.join(os.tmpdir(), 'lumi-cindy-dev-install-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  copyFileSync(path.resolve(__dirname, '../../Makefile'), path.join(root, 'Makefile'));
  const bin = path.join(root, 'test-bin');
  mkdirSync(bin);
  const calls = path.join(root, 'calls');
  writeFileSync(path.join(bin, 'node'), `#!/bin/sh
printf '%s\\n' "$*" >> "$LUMI_CINDY_TEST_CALLS"
case "$1" in
  cindy-plugins/scripts/build.mjs)
    if [ "$#" -ne 5 ] || [ "$5" != '/tmp/lumi dev test' ]; then exit 98; fi
    exit "$LUMI_CINDY_TEST_BUILD_STATUS" ;;
  cindy-plugins/scripts/install-dev.mjs)
    if [ "$#" -ne 3 ] || [ "$3" != '/Applications/Cindy Test.app' ]; then exit 98; fi
    if [ "$LUMI_CINDY_TEST_INSTALL_STATUS" -ne 0 ]; then
      printf '%s\\n' '错误：打包或提交开发版安装失败。' >&2
      exit "$LUMI_CINDY_TEST_INSTALL_STATUS"
    fi
    printf '%s\\n' '已提交 Lumi 开发版安装，请在 Cindy 中完成导入及所需确认。' ;;
  *) exit 97 ;;
esac
`, { mode: 0o755 });
  const result = spawnSync('make', ['--no-print-directory', target, 'CINDY_DEV_DATA_DIR=/tmp/lumi dev test', 'CINDY_APP=/Applications/Cindy Test.app'], {
    cwd: root, encoding: 'utf8', timeout: 10000,
    env: { ...process.env, MAKEFLAGS: '', MFLAGS: '', MAKELEVEL: '0', MAKEOVERRIDES: '',
      PATH: `${bin}${path.delimiter}${process.env.PATH}`,
      LUMI_CINDY_TEST_CALLS: calls, LUMI_CINDY_TEST_BUILD_STATUS: String(buildStatus),
      LUMI_CINDY_TEST_INSTALL_STATUS: String(installStatus) },
  });
  assert.ifError(result.error);
  return { ...result, calls: readFileSync(calls, 'utf8').trim().split('\n') };
}

test('cindy-dev remains build only and forwards the development data directory', t => {
  const result = runMake(t, 'cindy-dev');
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.calls, ['cindy-plugins/scripts/build.mjs --channel development --data-dir /tmp/lumi dev test']);
  assert.doesNotMatch(result.stderr, /自动安装|安装成功/);
});

test('cindy-dev-install builds before submitting installation and forwards the Cindy app path', t => {
  const result = runMake(t, 'cindy-dev-install');
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.calls, [
    'cindy-plugins/scripts/build.mjs --channel development --data-dir /tmp/lumi dev test',
    'cindy-plugins/scripts/install-dev.mjs --app /Applications/Cindy Test.app',
  ]);
  assert.match(result.stdout, /已提交.*安装/);
  assert.doesNotMatch(result.stdout + result.stderr, /安装成功|插件已更新/);
});

test('cindy-dev-install stops at a build failure and reports it in Chinese', t => {
  const result = runMake(t, 'cindy-dev-install', 23);
  assert.notEqual(result.status, 0);
  assert.equal(result.calls.length, 1);
  assert.match(result.stderr, /错误：Cindy 开发版构建失败，未执行安装/);
  assert.doesNotMatch(result.stdout + result.stderr, /自动安装被阻塞|开发版已构建|安装成功/);
});

test('cindy-dev-install propagates a packaging or handoff failure', t => {
  const result = runMake(t, 'cindy-dev-install', 0, 23);
  assert.notEqual(result.status, 0);
  assert.equal(result.calls.length, 2);
  assert.match(result.stderr, /错误：打包或提交开发版安装失败/);
  assert.doesNotMatch(result.stdout, /已提交|安装成功/);
});

function fixture(t) {
  const repo = mkdtempSync(path.join(os.tmpdir(), 'lumi cindy install-'));
  t.after(() => rmSync(repo, { recursive: true, force: true }));
  const author = path.join(repo, 'build/cindy-dev/lumi-dev');
  mkdirSync(path.join(author, 'bin'), { recursive: true });
  writeFileSync(path.join(author, 'ghost.json'), JSON.stringify({ id: 'lumi-dev', command: 'lumi-dev', version: '0.4.0' }));
  writeFileSync(path.join(author, 'main.js'), '// plugin source\n');
  writeFileSync(path.join(author, 'bin/lumi_dev'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  return { repo, author };
}

test('local .cindy import packages the root files and executable, excludes old packages, and reports only submission', async t => {
  const { installDev } = await import('../scripts/install-dev.mjs');
  const { repo, author } = fixture(t);
  writeFileSync(path.join(author, 'removed.js'), '// removed later');
  writeFileSync(path.join(author, '.hidden'), 'not shipped');
  writeFileSync(path.join(author, 'old.cindy'), 'not nested');
  const logs = [], opened = [];
  const options = { repo, platform: 'darwin', app: '/Applications/Cindy Test.app', log: v => logs.push(v),
    run(command, args, opts) {
      if (command === '/usr/bin/open') { opened.push(args); return { status: 0 }; }
      return spawnSync(command, args, opts);
    } };
  const archive = installDev(options);
  rmSync(path.join(author, 'removed.js'));
  installDev(options);
  const listing = spawnSync('/usr/bin/unzip', ['-Z', '-1', archive], { encoding: 'utf8' });
  assert.equal(listing.status, 0, listing.stderr);
  assert.deepEqual(listing.stdout.trim().split('\n').sort(), ['bin/lumi_dev', 'ghost.json', 'main.js']);
  const modes = spawnSync('/usr/bin/unzip', ['-Z', '-l', archive], { encoding: 'utf8' });
  assert.equal(modes.status, 0, modes.stderr);
  assert.match(modes.stdout, /^-rwxr-xr-x .*bin\/lumi_dev$/m);
  assert.deepEqual(opened, Array(2).fill(['-a', '/Applications/Cindy Test.app', archive]));
  assert.match(logs.join('\n'), /已提交.*安装/);
  assert.match(logs.join('\n'), /不等待或核验安装结果/);
  assert.doesNotMatch(logs.join('\n'), /安装成功|插件已更新/);
  assert.deepEqual(readdirSync(path.dirname(author)), ['lumi-dev']);
});

test('packaging failure stops before opening Cindy and cleans temporary files', async t => {
  const { installDev } = await import('../scripts/install-dev.mjs');
  const { repo, author } = fixture(t);
  const calls = [];
  assert.throws(() => installDev({ repo, platform: 'darwin', log() {}, run(command) {
    calls.push(command); return { status: 23, stderr: 'disk full' };
  } }), /开发版打包失败.*disk full/);
  assert.deepEqual(calls, ['/usr/bin/zip']);
  assert.deepEqual(readdirSync(path.dirname(author)), ['lumi-dev']);
});

test('Cindy launch errors are failures, never reported as submitted installation', async t => {
  const { installDev } = await import('../scripts/install-dev.mjs');
  const { repo } = fixture(t);
  const logs = [];
  assert.throws(() => installDev({ repo, platform: 'darwin', log: v => logs.push(v), run(command, args, opts) {
    if (command === '/usr/bin/open') return { status: 1, stderr: 'application not found' };
    return spawnSync(command, args, opts);
  } }), /无法将开发包交给 Cindy.*application not found/);
  assert.doesNotMatch(logs.join('\n'), /已提交|安装成功/);
});

test('the installer refuses the production identity before packaging or opening Cindy', async t => {
  const { installDev } = await import('../scripts/install-dev.mjs');
  const { repo, author } = fixture(t);
  writeFileSync(path.join(author, 'ghost.json'), JSON.stringify({ id: 'lumi', command: 'lumi', version: '0.4.0' }));
  assert.throws(() => installDev({ repo, platform: 'darwin', run() { assert.fail('must not execute'); } }), /开发包身份必须为 lumi-dev/);
});
