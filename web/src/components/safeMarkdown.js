import { parseProjectReference } from '../pages/projectReferences.js'

export function sanitizeMarkdownUrl(value = '') {
	const projectReference = String(value)
	if (projectReference.startsWith('@project')) return parseProjectReference(projectReference) ? projectReference : ''
  const candidate = String(value).trim().replace(/[\u0000-\u001f\u007f]/gu, '')
  if (!candidate || candidate.startsWith('//')) return ''
  if (/^(?:#|\/|\.\/|\.\.\/|\?)/u.test(candidate)) return candidate
  const match = candidate.match(/^([a-z][a-z0-9+.-]*):/iu)
  if (!match) return ''
  return ['http', 'https', 'mailto'].includes(match[1].toLowerCase()) ? candidate : ''
}

export function isExternalMarkdownUrl(value = '') {
  return /^https?:/iu.test(String(value))
}
