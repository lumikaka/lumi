import assert from 'node:assert/strict'
import test from 'node:test'
import { createElement as h } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { createServer } from 'vite'
import react from '@vitejs/plugin-react'

test('normal Web chapter thumbnails keep page links and native images with a host adapter available', async () => {
  const vite = await createServer({ configFile: false, plugins: [react()], server: { middlewareMode: true, watch: null, hmr: false, ws: false } })
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } })
  try {
    const { SimplePagesPage, SimpleImage } = await vite.ssrLoadModule('/src/pages/SimpleProjectPages.jsx')
    const { WorkspaceRuntime } = await vite.ssrLoadModule('/src/workspace/runtime.jsx')
    const section = { uuid: 'page', title: 'Page one', page_role: 'body', section_no: 1, current_image: { asset: { uuid: 'asset', status: 'ready', content_url: '/original.png' } } }
    client.setQueryData(['story-chapters', 'project', 'active'], {items:[{uuid:'chapter',title:'Book'}]})
    client.setQueryData(['story-chapter', 'project', 'chapter'], { title: 'Book', chapter_code: 'vol01.ch01' })
    client.setQueryData(['comic-sections', 'project', 'chapter'], { items: [section] })
    client.setQueryData(['production-tasks', 'project'], { items: [] })
    const html = renderToStaticMarkup(h(QueryClientProvider, { client }, h(MemoryRouter, { initialEntries: ['/projects/project/chapters/chapter'] }, h(Routes, null, h(Route, { path: '/projects/:projectUuid/chapters/:chapterUuid', element: h(SimplePagesPage, { projectUuid: 'project' }) })))))
    assert.match(html, /class="simple-page-grid"><article class="simple-page-card-wrap"><a/)
    assert.match(html, /href="\/projects\/project\/chapters\/chapter\/sections\/page"/)
    assert.match(html, /src="\/original.png"/)
    assert.doesNotMatch(html, /book-preview__hit-area|图片将在可见时加载/)
    assert.doesNotMatch(html, /\/preview|阅读预览|全屏阅读/)
    const props = { asset: section.current_image.asset, alt: 'Original image' }
    const before = renderToStaticMarkup(h(SimpleImage, props))
    const host = renderToStaticMarkup(h(WorkspaceRuntime.Provider, { value: { Image: () => h('img', { src: '/host.png', alt: 'Host image' }) } }, h(SimpleImage, props)))
    assert.match(host, /src="\/host.png"/)
    assert.equal(renderToStaticMarkup(h(SimpleImage, props)), before, 'a host runtime must not replace the default Web runtime')
  } finally {
    client.clear()
    await vite.close()
  }
})
