'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const { mkdtempSync, copyFileSync, mkdirSync, writeFileSync, readFileSync, rmSync } = require('node:fs');
const os = require('node:os');
const path = require('node:path');

// Exercise the real Make recipes without compiling or touching Cindy's installation.
function runMake(t, target, buildStatus = 0) {
  const root = mkdtempSync(path.join(os.tmpdir(), 'lumi-cindy-dev-install-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  copyFileSync(path.resolve(__dirname, '../../Makefile'), path.join(root, 'Makefile'));
  const bin = path.join(root, 'test-bin');
  mkdirSync(bin);
  const calls = path.join(root, 'calls');
  writeFileSync(path.join(bin, 'node'), `#!/bin/sh
printf '%s\\n' "$*" >> "$LUMI_CINDY_TEST_CALLS"
if [ "$1" != 'cindy-plugins/scripts/build.mjs' ]; then exit 97; fi
if [ "$#" -ne 5 ] || [ "$5" != '/tmp/lumi dev test' ]; then exit 98; fi
exit "$LUMI_CINDY_TEST_BUILD_STATUS"
`, { mode: 0o755 });
  const result = spawnSync('make', ['--no-print-directory', target, 'CINDY_DEV_DATA_DIR=/tmp/lumi dev test'], {
    cwd: root, encoding: 'utf8', timeout: 10000,
    env: { ...process.env, MAKEFLAGS: '', MFLAGS: '', MAKELEVEL: '0', MAKEOVERRIDES: '',
      PATH: `${bin}${path.delimiter}${process.env.PATH}`,
      LUMI_CINDY_TEST_CALLS: calls, LUMI_CINDY_TEST_BUILD_STATUS: String(buildStatus) },
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

test('cindy-dev-install builds first, then fails clearly when no official terminal installer is available', t => {
  const result = runMake(t, 'cindy-dev-install');
  assert.notEqual(result.status, 0);
  assert.equal(result.calls.length, 1);
  assert.match(result.calls[0], /build\.mjs --channel development --data-dir \/tmp\/lumi dev test$/);
  assert.match(result.stderr, /自动安装被阻塞.*官方终端安装及回执入口/);
  assert.match(result.stderr, /build\/cindy-dev\/lumi-dev\//);
  assert.match(result.stderr, /本命令未执行安装，未切换运行中的共享后台/);
  assert.doesNotMatch(result.stdout + result.stderr, /安装成功|插件已更新/);
});

test('cindy-dev-install stops at a build failure and reports it in Chinese', t => {
  const result = runMake(t, 'cindy-dev-install', 23);
  assert.notEqual(result.status, 0);
  assert.equal(result.calls.length, 1);
  assert.match(result.stderr, /错误：Cindy 开发版构建失败，未执行安装/);
  assert.doesNotMatch(result.stdout + result.stderr, /自动安装被阻塞|开发版已构建|安装成功/);
});
