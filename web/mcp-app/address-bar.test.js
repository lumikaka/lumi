import assert from 'node:assert/strict'
import test from 'node:test'
import { createElement as h } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MemoryRouter } from 'react-router-dom'
import { createServer } from 'vite'
import react from '@vitejs/plugin-react'

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
      const html = renderToStaticMarkup(h(MemoryRouter, { initialEntries: entries }, h(AddressBar, { base, confirmLeave: () => true })))
      assert.ok(html.includes(`aria-label="${label}"`))
      assert.ok(html.includes(`value="ui://lumi${entries.at(-1)}"`))
      assert.equal(html.includes('disabled=""'), label === '返回')
    }
  } finally {
    await vite.close()
  }
})
