export function sanitizeMarkdownUrl(value = '') {
  const candidate = String(value).trim().replace(/[\u0000-\u001f\u007f]/gu, '')
  if (!candidate || candidate.startsWith('//')) return ''
  if (/^(?:#|\/|\.\/|\.\.\/|\?)/u.test(candidate)) return candidate
  const match = candidate.match(/^([a-z][a-z0-9+.-]*):/iu)
  if (!match) return ''
  return ['http', 'https', 'mailto'].includes(match[1].toLowerCase()) ? candidate : ''
}

export function parseMarkdownBlocks(value = '') {
  const lines = String(value).replace(/\r\n?/gu, '\n').split('\n')
  const blocks = []
  let paragraph = []
  const flushParagraph = () => {
    if (!paragraph.length) return
    blocks.push({ type: 'paragraph', text: paragraph.join('\n') })
    paragraph = []
  }

  for (let index = 0; index < lines.length;) {
    const line = lines[index]
    const fence = line.match(/^\s*```([\w+-]*)\s*$/u)
    if (fence) {
      flushParagraph()
      const content = []
      index += 1
      while (index < lines.length && !/^\s*```\s*$/u.test(lines[index])) {
        content.push(lines[index])
        index += 1
      }
      if (index < lines.length) index += 1
      blocks.push({ type: 'code', language: fence[1], text: content.join('\n') })
      continue
    }
    if (!line.trim()) {
      flushParagraph()
      index += 1
      continue
    }
    const heading = line.match(/^(#{1,6})\s+(.+)$/u)
    if (heading) {
      flushParagraph()
      blocks.push({ type: 'heading', level: heading[1].length, text: heading[2] })
      index += 1
      continue
    }
    if (/^\s*>\s?/u.test(line)) {
      flushParagraph()
      const values = []
      while (index < lines.length && /^\s*>\s?/u.test(lines[index])) {
        values.push(lines[index].replace(/^\s*>\s?/u, ''))
        index += 1
      }
      blocks.push({ type: 'blockquote', text: values.join('\n') })
      continue
    }
    const unordered = line.match(/^\s*[-*+]\s+(.+)$/u)
    const ordered = line.match(/^\s*\d+[.)]\s+(.+)$/u)
    if (unordered || ordered) {
      flushParagraph()
      const type = ordered ? 'ordered_list' : 'unordered_list'
      const matcher = ordered ? /^\s*\d+[.)]\s+(.+)$/u : /^\s*[-*+]\s+(.+)$/u
      const items = []
      while (index < lines.length) {
        const item = lines[index].match(matcher)
        if (!item) break
        items.push(item[1])
        index += 1
      }
      blocks.push({ type, items })
      continue
    }
    paragraph.push(line)
    index += 1
  }
  flushParagraph()
  return blocks
}

export function parseMarkdownInline(value = '') {
  const text = String(value)
  const tokens = []
  const pattern = /(`[^`\n]+`|\[[^\]\n]+\]\([^\s)]+\))/gu
  let cursor = 0
  for (const match of text.matchAll(pattern)) {
    if (match.index > cursor) tokens.push({ type: 'text', text: text.slice(cursor, match.index) })
    const token = match[0]
    if (token.startsWith('`')) {
      tokens.push({ type: 'code', text: token.slice(1, -1) })
    } else {
      const parts = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/u)
      const href = sanitizeMarkdownUrl(parts?.[2] || '')
      tokens.push(href ? { type: 'link', text: parts[1], href } : { type: 'text', text: parts?.[1] || token })
    }
    cursor = match.index + token.length
  }
  if (cursor < text.length) tokens.push({ type: 'text', text: text.slice(cursor) })
  return tokens
}
