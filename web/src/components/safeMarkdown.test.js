import assert from 'node:assert/strict'
import test from 'node:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import { isExternalMarkdownUrl, sanitizeMarkdownUrl } from './safeMarkdown.js'
import SafeMarkdown from './safeMarkdownRenderer.js'

function renderMarkdown(value, props = {}) {
	return renderToStaticMarkup(createElement(SafeMarkdown, { value, ...props }))
}

test('safe markdown renders CommonMark and GFM chat content', () => {
  const html = renderMarkdown(`# 标题

**粗体** *斜体* ~~删除~~

- 外层
  - 内层

| 名称 | 状态 |
| --- | --- |
| Lumi | 完成 |

- [x] 已完成

\`\`\`js
alert(1)
\`\`\``)

  assert.match(html, /<h1>标题<\/h1>/)
  assert.match(html, /<strong>粗体<\/strong> <em>斜体<\/em> <del>删除<\/del>/)
  assert.match(html, /<li>外层\s*<ul>\s*<li>内层<\/li>/)
  assert.match(html, /safe-markdown__table-scroll[\s\S]*?<table>[\s\S]*?<th>名称<\/th>[\s\S]*?<td>Lumi<\/td>/)
  assert.match(html, /class="task-list-item"[\s\S]*?<input type="checkbox" disabled="" checked=""\/> 已完成/)
  assert.match(html, /<pre><code class="language-js">alert\(1\)\n<\/code><\/pre>/)
})

test('safe markdown allows useful links and rejects executable protocols', () => {
  assert.equal(sanitizeMarkdownUrl('https://example.com/a'), 'https://example.com/a')
  assert.equal(sanitizeMarkdownUrl('/projects/one'), '/projects/one')
  assert.equal(sanitizeMarkdownUrl('mailto:user@example.com'), 'mailto:user@example.com')
  assert.equal(sanitizeMarkdownUrl('javascript:alert(1)'), '')
  assert.equal(sanitizeMarkdownUrl('data:text/html,bad'), '')
  assert.equal(isExternalMarkdownUrl('https://example.com'), true)
  assert.equal(isExternalMarkdownUrl('/projects/one'), false)

  const html = renderMarkdown('[external](https://example.com) [internal](/projects/one) [mail](mailto:user@example.com) [bad](javascript:alert(1))')
  assert.match(html, /href="https:\/\/example\.com" rel="noopener noreferrer" target="_blank">external<\/a>/)
  assert.match(html, /href="\/projects\/one">internal<\/a>/)
  assert.match(html, /href="mailto:user@example\.com">mail<\/a>/)
  assert.doesNotMatch(html, /javascript:|>bad<\/a>/)
  assert.match(html, /<span>bad<\/span>/)
})

test('safe markdown drops raw HTML and never loads markdown images', () => {
  const html = renderMarkdown(`<img src=x onerror=alert(1)>

![remote](https://example.com/image.png)

![unsafe](data:image/png;base64,bad)`)

  assert.doesNotMatch(html, /<img|onerror|data:image/)
  assert.match(html, /class="safe-markdown__image-link" href="https:\/\/example\.com\/image\.png"[\s\S]*?>remote<\/a>/)
  assert.match(html, /class="safe-markdown__image-alt">unsafe<\/span>/)
})

test('safe markdown delegates valid inline project references and renders invalid ones as text', () => {
	const chapterUuid = '01900000-0000-7000-8000-000000000002'
	const workflowUuid = '01900000-0000-7000-8000-000000000005'
	const valid = `@project/chapters/${chapterUuid}/body`
	assert.equal(sanitizeMarkdownUrl(valid), valid)
	assert.equal(sanitizeMarkdownUrl(`@project/chapters/${chapterUuid}/../body`), '')

	const html = renderMarkdown(`[第三章正文](${valid})已经修改完成。`, {
		renderProjectReference: ({ children }) => createElement('a', { href: '/resolved/chapter' }, children),
	})
	assert.match(html, /<a href="\/resolved\/chapter">第三章正文<\/a>已经修改完成。/)
	assert.match(renderMarkdown(`![第三章正文](${valid})`), /class="safe-markdown__image-alt">第三章正文<\/span>/)

	const invalid = renderMarkdown('[第三章正文](@project/chapters/0190ABCD-EFAB-7ABC-8ABC-ABCDEFABCDEF/body)已经修改完成。', {
		renderProjectReference: () => createElement('a', { href: '/must-not-render' }, 'bad'),
	})
	assert.doesNotMatch(invalid, /must-not-render|@project/)
	assert.match(invalid, /<span>第三章正文<\/span>已经修改完成。/)

	const workflow = renderMarkdown(`[YOLO](@project/workflows/${workflowUuid})已启动。`, {
		renderProjectReference: ({ children }) => createElement('a', { href: '/resolved/workflow' }, children),
	})
	assert.match(workflow, /<a href="\/resolved\/workflow">YOLO<\/a>已启动。/)
})
