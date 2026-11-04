import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { copyNative } from './copy-native.mjs';
import { fixtureMediaTool } from './media-schema.mjs';
const source = fileURLToPath(new URL('./', import.meta.url));
const repo = fileURLToPath(new URL('../../../../../', import.meta.url));
const product = path.join(repo, 'build/cindy-dev/lumi-dev');
const target = path.join(repo, 'build/cindy-probe/lumi-host-acceptance-20261101');
const options = process.argv.slice(2);
if (options.length && (options.length !== 2 || options[0] !== '--data-dir'
  || !/^\/private\/tmp\/lumi-cindy-[A-Za-z0-9-]+$/.test(options[1]))) throw new Error('Only --data-dir /private/tmp/lumi-cindy-<isolated-acceptance-name> is accepted');
const dataDir = options[1] || '/private/tmp/lumi-cindy-host-acceptance-20261101';
const hash = value => crypto.createHash('sha256').update(value).digest('hex');
fs.mkdirSync(target, { recursive: true });
for (const dir of ['node', 'assets']) fs.cpSync(path.join(product, dir), path.join(target, dir), { recursive: true });
for (const name of fs.readdirSync(path.join(product, 'bin'))) copyNative(path.join(product, 'bin', name), path.join(target, 'bin', name));
const body = fs.readFileSync(path.join(product, 'main.js'));
fs.writeFileSync(path.join(target, 'main.js'), Buffer.concat([fs.readFileSync(path.join(source, 'main-prefix.js')), body, fs.readFileSync(path.join(source, 'main-suffix.js'))]));
for (const file of ['task.html', 'task.js']) fs.copyFileSync(path.join(source, file), path.join(target, file));
const build = JSON.parse(fs.readFileSync(path.join(product, 'build-info.json')));
fs.writeFileSync(path.join(target, 'build-info.json'), JSON.stringify({ ...build, product_main_sha256: hash(body), harness: true }, null, 2) + '\n');
const config = JSON.parse(fs.readFileSync(path.join(product, 'node/runtime-config.json')));
config.data_dir = dataDir;
fs.writeFileSync(path.join(target, 'node/runtime-config.json'), JSON.stringify(config, null, 2) + '\n');
const productManifest = JSON.parse(fs.readFileSync(path.join(product, 'ghost.json')));
const fixture = { type: 'string', enum: ['a', 'b'] }, empty = {};
const tool = (name, description, properties, required = []) => ({ name, description, parameters: { type: 'object', properties, required, additionalProperties: false } });
const media = fixtureMediaTool(productManifest.tools.find(t => t.name === 'receive_media'));
media.description = '仅把顶层授权图片交给 Host 创建的 a/b 临时项目，使用未修改的产品接收适配器。';
const manifest = { schemaVersion: 3, minCindyVersion: '0.1.121', id: 'lumi-host-acceptance-20261101', name: 'Lumi Host Acceptance',
  description: '临时验收：仅使用 Host 为本探针创建的独立空任务测试产品适配器与任务页边界；不启动其他 Agent，不访问已有作品。',
  whenToUse: '仅用于已授权的 Lumi Step 4–6 本机产品验收。', version: '0.0.1', entry: 'main.js', icon: 'assets/icon.png',
  launch: 'resident', sessionContext: true, node: productManifest.node, cindy: productManifest.cindy, agent: { tasks: true },
  taskView: { title: 'Lumi 验收', html: 'task.html' }, tools: [
    tool('create_fixture', '创建本探针拥有的独立空任务；更新后可用前次回执 task_id 经 Host 归属校验恢复，不重复创建。不发送消息、不启动 Agent。', { fixture, task_id: { type: 'string', pattern: '^[0-9a-f-]{36}$' } }, ['fixture']),
    tool('run', '在 Host 创建的临时项目执行产品允许工具；不接收任何目录或自报上下文。', { fixture, name: { type: 'string' }, args: { type: 'object' } }, ['fixture', 'name']), media,
    tool('open_probe', '打开当前任务或临时任务的最小只读诊断页；不抢焦点、不读作品。', { fixture }),
    tool('report', '读取有界页面诊断与临时任务标识；不读取任务消息。', empty),
    tool('release_fixture', '撤销临时任务连接，保留用户项目与共享服务。', { fixture }, ['fixture']),
  ] };
fs.writeFileSync(path.join(target, 'ghost.json'), JSON.stringify(manifest, null, 2) + '\n');
const receipt = { kind: 'host-integration-harness-not-normal-product-binding', author: path.relative(repo, target),
  product_build_id: build.build_id, product_main_sha256: hash(body), cli_sha256: build.cli_sha256, data_dir: config.data_dir };
fs.writeFileSync(path.join(target, 'probe-build.json'), JSON.stringify(receipt, null, 2) + '\n');
console.log(JSON.stringify(receipt, null, 2));
