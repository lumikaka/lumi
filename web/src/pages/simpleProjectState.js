import { comicBodyPageNumber, comicPageRole } from './comicPageRoles.js'

const SIMPLE_PROJECT_SETTINGS_TABS = new Set(['summary', 'profile', 'prompts'])
const SIMPLE_PROJECT_SETTINGS_SUMMARY_SECTIONS = new Set(['project', 'format', 'language', 'models', 'style'])

export function normalizedSimpleProjectSettingsTab(value) {
  return SIMPLE_PROJECT_SETTINGS_TABS.has(value) ? value : 'summary'
}

export function normalizedSimpleProjectSettingsSection(value) {
  return SIMPLE_PROJECT_SETTINGS_SUMMARY_SECTIONS.has(value) ? value : 'project'
}

export function patchSimpleProjectSettingsSearch(search = '', tab = '', section = '') {
  const next = new URLSearchParams(search)
  if (SIMPLE_PROJECT_SETTINGS_TABS.has(tab)) next.set('tab', tab)
  else next.delete('tab')
  if (tab === 'summary' && SIMPLE_PROJECT_SETTINGS_SUMMARY_SECTIONS.has(section)) next.set('section', section)
  else next.delete('section')
  return next
}

export function simpleProjectRouteState(pathname = '', projectUuid = '') {
  const base = `/projects/${encodeURIComponent(projectUuid || '')}`
  const route = pathname.startsWith(base) ? pathname.slice(base.length).replace(/^\/+|\/+$/g, '') : ''
  if (!route) return { key: 'home', assetUuid: '', chapterUuid: '', sectionUuid: '' }
  if (route === 'settings') return { key: 'configuration', assetUuid: '', chapterUuid: '', sectionUuid: '' }
  if (route === 'story') return { key: 'story', assetUuid: '', chapterUuid: '', sectionUuid: '' }
  if (route === 'llm-logs') return { key: 'llm_logs', assetUuid: '', chapterUuid: '', sectionUuid: '' }
  if (route === 'exports') return { key: 'exports', assetUuid: '', chapterUuid: '', sectionUuid: '' }
  if (route === 'premise') return { key: 'settings', assetUuid: '', chapterUuid: '', sectionUuid: '' }
  const trajectory = route.match(/^threads\/([^/]+)\/trajectory$/)
  if (trajectory) return { key: 'trajectory', assetUuid: '', chapterUuid: '', sectionUuid: '', threadUuid: decodeSegment(trajectory[1]) }
  const setting = route.match(/^premise\/assets\/([^/]+)$/)
  if (setting) return { key: 'setting', assetUuid: decodeSegment(setting[1]), chapterUuid: '', sectionUuid: '' }
  if (route === 'chapters') return { key: 'books', assetUuid: '', chapterUuid: '', sectionUuid: '' }
  const page = route.match(/^chapters\/([^/]+)\/sections\/([^/]+)$/)
  if (page) return { key: 'page', assetUuid: '', chapterUuid: decodeSegment(page[1]), sectionUuid: decodeSegment(page[2]) }
  const book = route.match(/^chapters\/([^/]+)\/preview$/)
  if (book) return { key: 'book', assetUuid: '', chapterUuid: decodeSegment(book[1]), sectionUuid: '' }
  const pages = route.match(/^chapters\/([^/]+)$/)
  if (pages) return { key: 'pages', assetUuid: '', chapterUuid: decodeSegment(pages[1]), sectionUuid: '' }
  return { key: 'not_found', assetUuid: '', chapterUuid: '', sectionUuid: '' }
}

export function simpleProjectChatReference(routeState, { asset, chapter, section } = {}) {
  if (routeState?.key === 'setting' && asset?.uuid === routeState.assetUuid) {
    const imageUuid = asset.current_variant?.asset?.uuid || ''
    return readyReference('premise_asset', asset.uuid, asset.title, imageUuid)
  }
  if (routeState?.key === 'page' && section?.uuid === routeState.sectionUuid) {
    const imageUuid = section.current_image?.asset?.uuid || ''
    return readyReference('comic_section', section.uuid, section.title, imageUuid)
  }
  if (routeState?.key === 'pages' && section?.uuid) {
    const imageUuid = section.current_image?.asset?.uuid || ''
    return readyReference('comic_section', section.uuid, section.title, imageUuid)
  }
  if (['chapter', 'pages', 'book'].includes(routeState?.key) && chapter?.uuid === routeState.chapterUuid) {
    return readyReference('chapter', chapter.uuid, [chapter.chapter_code, chapter.title].filter(Boolean).join(' · '))
  }
  return null
}

export function orderedSimplePages(sections = []) {
  return [...sections].sort((left, right) => {
    const roleDifference = pageRoleOrder(comicPageRole(left)) - pageRoleOrder(comicPageRole(right))
    if (roleDifference) return roleDifference
    if (comicPageRole(left) === 'body') {
      const pageDifference = comicBodyPageNumber(sections, left) - comicBodyPageNumber(sections, right)
      if (pageDifference) return pageDifference
    }
    return Number(left?.section_no || 0) - Number(right?.section_no || 0)
  })
}

export function storyDocumentBlocks(markdown = '') {
  const blocks = []
  let paragraphLines = []
  const flushParagraph = () => {
    const text = inlineMarkdownText(paragraphLines.join(' '))
    if (text) blocks.push({ type: 'paragraph', text })
    paragraphLines = []
  }
  for (const rawLine of String(markdown || '').replace(/<!--[\s\S]*?-->/g, '').split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line) {
      flushParagraph()
      continue
    }
    const heading = line.match(/^#{1,6}\s+(.+)$/)
    if (heading) {
      flushParagraph()
      blocks.push({ type: 'heading', text: inlineMarkdownText(heading[1]) })
      continue
    }
    paragraphLines.push(line)
  }
  flushParagraph()
  return blocks
}

export function storyPlainText(markdown = '') {
  return storyDocumentBlocks(markdown).map((block) => block.text).join(' ')
}

export function simpleStoryExcerpt(markdown = '', limit = 220) {
  const copy = storyPlainText(markdown)
  if (copy.length <= limit) return copy
  return `${copy.slice(0, Math.max(0, limit)).trimEnd()}…`
}

export function storyboardQuickEditSections(markdown = '') {
  return storyboardSectionRanges(markdown).map(({ label, contentStart, contentEnd }) => ({
    label,
    content: String(markdown || '').slice(contentStart, contentEnd),
  }))
}

export function updateStoryboardQuickEditSection(markdown = '', index, content = '') {
  const source = String(markdown || '')
  const sections = storyboardSectionRanges(source)
  const section = sections[index]
  if (!section) return source
  const newline = source.includes('\r\n') ? '\r\n' : '\n'
  const insertingIntoEmptyBody = section.contentStart === section.contentEnd
  const insertionPrefix = insertingIntoEmptyBody && section.bodyStart === section.headingEnd ? newline : ''
  const insertionSuffix = insertingIntoEmptyBody && section.bodyStart === section.bodyEnd && index < sections.length - 1 ? newline : ''
  return `${source.slice(0, section.contentStart)}${insertionPrefix}${content}${insertionSuffix}${source.slice(section.contentEnd)}`
}

export function firstReadySimpleImage(sections = []) {
  return orderedSimplePages(sections).find((section) => section.current_image?.asset?.status === 'ready' && section.current_image.asset.content_url)?.current_image?.asset || null
}

export function simplePageCounts(sections = []) {
  const body = sections.filter((section) => comicPageRole(section) === 'body')
  return {
    total: body.length,
    ready: body.filter((section) => section.current_image?.asset?.status === 'ready').length,
  }
}

function readyReference(resourceType, resourceUuid, title, imageUuid = '') {
  return {
    localId: `${resourceType}:${resourceUuid}`,
    resource_type: resourceType,
    resource_uuid: resourceUuid,
    title: title || resourceUuid,
    image_file_uuid: imageUuid,
    image_available: Boolean(imageUuid),
    status: 'ready',
  }
}

function pageRoleOrder(role) {
  return { front_cover: 0, body: 1, back_cover: 2 }[role] ?? 1
}

function storyboardSectionRanges(markdown = '') {
  const source = String(markdown || '')
  const headings = [...source.matchAll(/^##[\t ]+(.+?)[\t ]*\r?$/gm)]
  return headings.map((heading, index) => {
    const rawHeadingEnd = heading.index + heading[0].length
    const headingEnd = rawHeadingEnd - (heading[0].endsWith('\r') ? 1 : 0)
    const bodyStart = rawHeadingEnd + (source[rawHeadingEnd] === '\n' ? 1 : 0)
    const bodyEnd = headings[index + 1]?.index ?? source.length
    const body = source.slice(bodyStart, bodyEnd)
    const leadingBlankLines = body.match(/^(?:[\t ]*\r?\n)+/)?.[0].length || 0
    const trailingBlankLines = body.match(/(?:\r?\n[\t ]*)+$/)?.[0].length || 0
    const bodyOnlyContainsWhitespace = !body.trim()
    return {
      label: heading[1].trim(),
      headingEnd,
      bodyStart,
      bodyEnd,
      contentStart: bodyOnlyContainsWhitespace ? bodyStart : bodyStart + leadingBlankLines,
      contentEnd: bodyOnlyContainsWhitespace ? bodyStart : bodyEnd - trailingBlankLines,
    }
  })
}

function inlineMarkdownText(value) {
  return String(value || '')
    .replace(/!\[([^\]]*)\]\([^)]+\)/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/^[-*>]\s+/gm, '')
    .replace(/[*_`~]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

function decodeSegment(value) {
  try { return decodeURIComponent(value) } catch { return value }
}
