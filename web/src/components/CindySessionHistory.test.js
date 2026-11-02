import assert from 'node:assert/strict'
import test from 'node:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createServer } from 'vite'
import react from '@vitejs/plugin-react'

test('Cindy history renders escaped text, scope, omissions and pagination without chat input', async () => {
  // Compile the actual JSX component; no browser or HTTP listener is needed.
  const vite = await createServer({ configFile: false, plugins: [react()], server: { middlewareMode: true, watch: null, hmr: false, ws: false } })
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } })
  try {
    const { default: CindySessionHistory } = await vite.ssrLoadModule('/src/components/CindySessionHistory.jsx')
    client.setQueryData(['cindy-session-items', 'project', 'thread'], {
      pages: [{
        items: [
          { uuid: 'one', kind: 'user', text: '<script>untrusted source</script>', recorded_at: '2026-01-01T00:00:00Z' },
          { uuid: 'two', kind: 'user', text: '<script>untrusted source</script>', recorded_at: '2026-01-01T00:00:01Z' },
        ],
        coverage: { kind: 'manual_public_text', cutoff_at: '2026-01-01T00:00:02Z', omitted_messages: 1, warnings: ['injected_context_omitted'] },
        cursor_pagination: { has_more: true, next_cursor: 'two' },
      }],
      pageParams: [''],
    })
    const html = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(CindySessionHistory, { projectUuid: 'project', threadUuid: 'thread' })))
    assert.match(html, /未启用自动采集/)
    assert.match(html, /不保证完整聊天历史/)
    assert.match(html, /省略了 1 条/)
    assert.match(html, /加载更多历史/)
    assert.equal((html.match(/&lt;script&gt;untrusted source&lt;\/script&gt;/g) || []).length, 2)
    assert.doesNotMatch(html, /<script|<textarea|<input|contenteditable|发送|trajectory/)
  } finally {
    client.clear()
    await vite.close()
  }
})
