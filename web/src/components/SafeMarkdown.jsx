import { createElement } from 'react'

import { parseMarkdownBlocks, parseMarkdownInline } from './safeMarkdown.js'

function InlineMarkdown({ value }) {
  return parseMarkdownInline(value).map((token, index) => {
    const key = `${index}-${token.type}`
    if (token.type === 'code') return <code key={key}>{token.text}</code>
    if (token.type === 'link') return <a href={token.href} key={key} rel="noopener noreferrer" target={/^https?:/iu.test(token.href) ? '_blank' : undefined}>{token.text}</a>
    return <span key={key}>{token.text}</span>
  })
}

export default function SafeMarkdown({ value = '' }) {
  const blocks = parseMarkdownBlocks(value)
  return (
    <div className="safe-markdown">
      {blocks.map((block, index) => {
        const key = `${index}-${block.type}`
        if (block.type === 'heading') return createElement(`h${block.level}`, { key }, <InlineMarkdown value={block.text} />)
        if (block.type === 'blockquote') return <blockquote key={key}><InlineMarkdown value={block.text} /></blockquote>
        if (block.type === 'code') return <pre key={key}><code data-language={block.language || undefined}>{block.text}</code></pre>
        if (block.type === 'ordered_list') return <ol key={key}>{block.items.map((item, itemIndex) => <li key={`${itemIndex}-${item.slice(0, 16)}`}><InlineMarkdown value={item} /></li>)}</ol>
        if (block.type === 'unordered_list') return <ul key={key}>{block.items.map((item, itemIndex) => <li key={`${itemIndex}-${item.slice(0, 16)}`}><InlineMarkdown value={item} /></li>)}</ul>
        return <p key={key}><InlineMarkdown value={block.text} /></p>
      })}
    </div>
  )
}
