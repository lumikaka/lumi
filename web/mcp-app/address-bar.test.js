import assert from 'node:assert/strict'
import test from 'node:test'
import { createElement as h } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MemoryRouter } from 'react-router-dom'
import { createServer } from 'vite'
import react from '@vitejs/plugin-react'
import { workspaceHistoryEntries } from './workspace.js'

test('the address and back target reflect the current plugin route and its own memory history', async () => {
  const vite = await createServer({ configFile: false, plugins: [react()], server: { middlewareMode: true, watch: null, hmr: false, ws: false } })
  try {
    const { default: AddressBar } = await vite.ssrLoadModule('/mcp-app/AddressBar.jsx')
    const base = '/projects/01989abc-def0-7000-8000-000000000001'
    const deep = base + '/chapters/01989abc-def0-7000-8000-000000000002/sections/01989abc-def0-7000-8000-000000000003?panel=candidates'
    for (const [entries, label] of [
      [[base + '/'], '返回'],
      [[deep], '返回章节列表'],
      [[base + '/premise/assets/01989abc-def0-7000-8000-000000000004'], '返回设定列表'],
      [[base + '/candidates'], '返回作品'],
      [[base + '/premise', deep], '返回上一页'],
    ]) {
      const initialEntries = entries.length === 1 ? workspaceHistoryEntries({ project_uuid: base.slice('/projects/'.length), path: entries[0] }) : entries
      const html = renderToStaticMarkup(h(MemoryRouter, { initialEntries }, h(AddressBar, { confirmLeave: () => true })))
      const buttons = html.match(/<button\b[^>]+>/g)
      assert.ok(buttons[0].includes(`aria-label="${label}"`))
      assert.ok(html.includes(`value="ui://lumi${entries.at(-1)}"`))
      assert.equal(buttons[0].includes('disabled=""'), label === '返回')
      assert.match(buttons[1], /aria-label="前进"[^>]*disabled=""/)
    }
  } finally {
    await vite.close()
  }
})
