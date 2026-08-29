import { createElement } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import { parseProjectReference } from '../pages/projectReferences.js'
import { isExternalMarkdownUrl, sanitizeMarkdownUrl } from './safeMarkdown.js'

const markdownPlugins = [remarkGfm]

function MarkdownLink({ node: _node, href = '', children, ...props }) {
  if (!href) return createElement('span', null, children)

  const external = isExternalMarkdownUrl(href)
  return createElement('a', {
    ...props,
    href,
    rel: external ? 'noopener noreferrer' : undefined,
    target: external ? '_blank' : undefined,
  }, children)
}

function MarkdownImageLink({ node: _node, src = '', alt = '', title }) {
  const label = alt || title || src
  if (!src) return createElement('span', { className: 'safe-markdown__image-alt' }, label)
	if (parseProjectReference(src)) return createElement('span', { className: 'safe-markdown__image-alt' }, label)

  const external = isExternalMarkdownUrl(src)
  return createElement('a', {
    className: 'safe-markdown__image-link',
    href: src,
    rel: external ? 'noopener noreferrer' : undefined,
    target: external ? '_blank' : undefined,
    title,
  }, label || src)
}

function MarkdownTable({ node: _node, children, ...props }) {
  return createElement('div', { className: 'safe-markdown__table-scroll' }, createElement('table', props, children))
}

const markdownComponents = {
  a: MarkdownLink,
  img: MarkdownImageLink,
  table: MarkdownTable,
}

export default function SafeMarkdown({ value = '', renderProjectReference }) {
	const components = renderProjectReference ? {
		...markdownComponents,
		a: ({ node: _node, href = '', children, ...props }) => {
			const reference = parseProjectReference(href)
			if (reference) return renderProjectReference({ children, href, props, reference })
			return createElement(MarkdownLink, { ...props, href }, children)
		},
	} : {
		...markdownComponents,
		a: ({ node: _node, href = '', children, ...props }) => {
			if (parseProjectReference(href)) return createElement('span', null, children)
			return createElement(MarkdownLink, { ...props, href }, children)
		},
	}
  return createElement('div', { className: 'safe-markdown' }, createElement(ReactMarkdown, {
	components,
    remarkPlugins: markdownPlugins,
    skipHtml: true,
    urlTransform: sanitizeMarkdownUrl,
  }, String(value ?? '')))
}
