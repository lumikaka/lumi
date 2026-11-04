// Disposable browser integration fixture, not Cindy Host authentication.
import { createServer } from 'node:http'
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { pathToFileURL, fileURLToPath } from 'node:url'
import { createRequire } from 'node:module'
import { randomUUID } from 'node:crypto'
import { buildCindy } from '../../web/cindy-app/build.mjs'
const require = createRequire(import.meta.url)
const { Runtime, runCLI } = require('../../cindy-plugins/lumi/node/runtime.cjs')
const { WorkspaceGateway } = require('../../cindy-plugins/lumi/node/workspace.cjs')
const { envelope } = require('../../cindy-plugins/lumi/node/errors.cjs')
const repo = fileURLToPath(new URL('../../', import.meta.url))
const base = await mkdtemp(path.join(os.tmpdir(), 'lumi-cindy-workspace-'))
const dataDir = path.join(base, 'data'), home = path.join(base, 'home')
for (const dir of [dataDir, home]) await mkdir(dir)
const executable = path.join(repo, 'cindy-plugins/lumi/bin/lumi')
const build = JSON.parse(await readFile(path.join(repo, 'cindy-plugins/lumi/build-info.json')))
const env = { HOME: home, PATH: '/usr/bin:/bin', TMPDIR: os.tmpdir() }
const runtime = new Runtime({ executable, build, dataDir, env })
const status = () => runCLI(executable, ['status', '--data-dir', dataDir, '--json'], { cwd: base, env })
const fixtures = {}, pages = new Map(), log = []
const ok = result => { if (!result.success) throw new Error(JSON.stringify(result.error)); return result.data }
const call = (fixture, name, args = {}) => runtime.execute('call_tool', { context: fixture.context, arguments: { name, args } }).then(ok)
for (const slot of ['a', 'b']) {
  const workdir = path.join(base, slot); await mkdir(workdir)
  const fixture = fixtures[slot] = { context: { session_id: `fixture-${slot}`, workdir, workdir_is_local: true, workdir_is_read_only: false } }
  await call(fixture, 'project_init', { idea: `Synthetic ${slot} story`, name: `Cindy Fixture ${slot.toUpperCase()}`, generation_language: 'en' })
  let setup = await call(fixture, 'project_setup', { action: 'get' })
  setup = await call(fixture, 'project_setup', { action: 'update', expected_revision: setup.revision, project_name: `Cindy Fixture ${slot.toUpperCase()}`, generation_language: 'en', overall_style: 'Fixed test icon, no models', generation_brief: 'Explicit integration test', picture_book: { format: 'classic_picture_book', aspect_ratio: { mode: 'landscape' }, large_image_minimal_text: false } })
  await call(fixture, 'project_setup', { action: 'finalize', expected_revision: setup.revision })
  fixture.project = (await call(fixture, 'project_overview')).project
  fixture.chapter = await call(fixture, 'chapters', { action: 'create', chapter_code: 'vol01.ch01', title: `${slot} chapter`, expected_revision: fixture.project.revision })
  fixture.chapter = await call(fixture, 'chapters', { action: 'write', chapter_uuid: fixture.chapter.uuid, content: `${slot} complete chapter story.`, expected_revision: fixture.chapter.revision })
  fixture.pageList = []
  for (let index = 1; index <= 3; index++) {
    const state = await call(fixture, 'pages', { action: 'list', chapter_uuid: fixture.chapter.uuid })
    fixture.pageList.push(await call(fixture, 'pages', { action: 'create', chapter_uuid: fixture.chapter.uuid, title: `${slot} page ${index}`, description_md: `${slot} page ${index} full prose.`, storyboard_md: `${slot} complete visual direction ${index}.`, expected_revision: state.state.revision }))
  }
  const premise = await call(fixture, 'settings', { action: 'list' })
  fixture.setting = await call(fixture, 'settings', { action: 'create', title: `${slot} setting`, asset_type: 'character', summary: `${slot} fixed geometry.`, expected_revision: premise.premise.revision })
  fixture.peer = [...runtime.connections.values()].find(record => record.sessions.has(fixture.context.session_id)).peer
  // Synthetic icon, explicit test setup only. Product imports remain attached
  // to Host-authorized bytes; no fixture path enters the product call_tool.
  const image = ok(await fixture.peer.call('import_image', { target_uuid: fixture.setting.uuid, slot: 'setting_image', expected_revision: fixture.setting.revision, path: path.join(repo, 'cindy-plugins/lumi/assets/icon.png') }))
  fixture.image = image
  for (let version = 0; version < 2; version++) {
    const prepared = await call(fixture, 'prepare_page_image', { page_uuid: fixture.pageList[0].uuid, setting_uuids: [], additional_direction: `Synthetic version ${version + 1}` })
    ok(await fixture.peer.call('import_image', { ...prepared.image_write_back, prompt: prepared.generation_prompt, preparation: prepared.preparation, path: path.join(repo, 'cindy-plugins/lumi/assets/icon.png') }))
  }
}
const ownedNonce = (await status()).nonce
await buildCindy({ entry: 'fixture-main.jsx', target: pathToFileURL(path.join(base, 'workspace.html')) })
let html = await readFile(path.join(base, 'workspace.html'), 'utf8')
html = html.replace("connect-src 'none'", "connect-src 'self'")
await writeFile(path.join(base, 'workspace.html'), html)
const server = createServer(async (request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1')
  const send = value => { response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify(value)) }
  try {
    if (url.pathname === '/') { response.setHeader('Content-Type', 'text/html'); response.end(html); return }
    if (url.pathname === '/blocked.html') { response.setHeader('Content-Type', 'text/html'); response.end(await readFile(path.join(repo, 'cindy-plugins/lumi/workspace.html'))); return }
    if (url.pathname === '/stats') { send({ log, open_pages: pages.size }); return }
    const launch = /^\/launch\/([ab])\/(book|chapters|chapter|page|settings|setting|candidates)$/.exec(url.pathname)
    if (launch) {
      const fixture = fixtures[launch[1]], view = launch[2], key = randomUUID(), sender = {}, lifecycle = new AbortController()
      await call(fixture, 'project_overview')
      fixture.peer = [...runtime.connections.values()].find(record => record.sessions.has(fixture.context.session_id)).peer
      const grant = { context: fixture.context, project_uuid: fixture.project.uuid, signal: lifecycle.signal }
      const gateway = new WorkspaceGateway({ runtime, sender, authority: { resolve: value => value === sender ? grant : null } })
      pages.set(key, { gateway, lifecycle, fixture })
      const name = view === 'setting' ? 'review_settings' : `review_${view}`
      const args = ['chapter', 'page', 'candidates'].includes(view) ? { chapter_uuid: fixture.chapter.uuid, ...(view === 'chapter' ? {} : { page_uuid: fixture.pageList[0].uuid, ...(view === 'candidates' ? { kind: 'page' } : {}) }) } : view === 'setting' ? { setting_uuid: fixture.setting.uuid } : {}
      const result = await fixture.peer.request('tools/call', { name, arguments: args })
      send({ key, result }); return
    }
    if (request.method === 'POST') {
      const route = /^\/(call|close)\/([a-f0-9-]+)$/.exec(url.pathname)
      if (route) {
        const page = pages.get(route[2]); if (!page) { response.statusCode = 410; send({ transport_error: { code: 'workspace_revoked', message: 'Test page closed' } }); return }
        if (route[1] === 'close') { page.lifecycle.abort(); await page.gateway.close(); pages.delete(route[2]); send({ closed: true }); return }
        let text = ''; for await (const chunk of request) { text += chunk; if (text.length > 262144) throw new Error('Fixture input too large') }
        const body = JSON.parse(text)
        log.push({ at: Date.now(), name: body.name, resource: body.args?.resource, action: body.args?.action, project: body.project })
        send(await page.gateway.call(body.name, body.args, { project: body.project })); return
      }
      const mutate = /^\/mutate\/([ab])$/.exec(url.pathname)
      if (mutate) {
        const fixture = fixtures[mutate[1]], page = await call(fixture, 'pages', { action: 'get', chapter_uuid: fixture.chapter.uuid, page_uuid: fixture.pageList[0].uuid })
        send(await call(fixture, 'pages', { action: 'update', chapter_uuid: fixture.chapter.uuid, page_uuid: page.uuid, title: `${mutate[1]} external WS change`, expected_revision: page.revision })); return
      }
    }
    response.statusCode = 404; send({ error: 'Fixture route not found' })
  } catch (error) { send({ transport_error: envelope(error).error }) }
})
server.listen(0, '127.0.0.1', async () => {
  const manifest = { kind: 'synthetic-page-authority-real-cli-not-Cindy-Host', pid: process.pid, base, browser_url: `http://127.0.0.1:${server.address().port}`, fixtures: Object.fromEntries(Object.entries(fixtures).map(([slot, fixture]) => [slot, { project_uuid: fixture.project.uuid, chapter_uuid: fixture.chapter.uuid, page_uuid: fixture.pageList[0].uuid, workdir: fixture.context.workdir }])), data_dir: dataDir }
  await writeFile(path.join(base, 'manifest.json'), JSON.stringify(manifest, null, 2))
  console.log(JSON.stringify(manifest))
})
let closing = false
async function close() {
  if (closing) return; closing = true
  for (const page of pages.values()) { page.lifecycle.abort(); await page.gateway.close() }
  await runtime.close()
  const current = await status()
  if (current.running) {
    if (current.nonce !== ownedNonce) throw new Error('Refusing to stop an unrecognized CLI instance')
    await fetch(`${current.endpoint}/api/v1/cli-instance`, { method: 'DELETE', headers: { 'If-Match': ownedNonce } })
  }
  server.close(); process.exit(0)
}
process.once('SIGTERM', close); process.once('SIGINT', close)
