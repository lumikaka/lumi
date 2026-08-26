export const MAX_PROJECT_CHAT_REFERENCES = 16
const PROJECT_CHAT_IMAGE_MIME_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp'])
const PROJECT_CHAT_REFERENCE_QUERY_KEYS = ['chat_reference_type', 'chat_reference_uuid', 'chat_reference_title']

export function consumeProjectChatReferenceQuery(searchParams) {
  const next = new URLSearchParams(searchParams)
  for (const key of PROJECT_CHAT_REFERENCE_QUERY_KEYS) next.delete(key)
  return next
}

export function selectProjectChatImageFiles(files, currentCount = 0) {
  const candidates = Array.from(files || [])
  const images = candidates.filter((file) => PROJECT_CHAT_IMAGE_MIME_TYPES.has(file?.type?.toLowerCase?.()))
  const available = Math.max(MAX_PROJECT_CHAT_REFERENCES - currentCount, 0)
  return {
    acceptedFiles: images.slice(0, available),
    rejectedNonImages: candidates.length - images.length,
    exceededLimit: images.length > available,
  }
}

export function projectChatClipboardFiles(clipboardData) {
  const direct = Array.from(clipboardData?.files || [])
  if (direct.length) return direct
  return Array.from(clipboardData?.items || [])
    .filter((item) => item?.kind === 'file' && typeof item.getAsFile === 'function')
    .map((item) => item.getAsFile())
    .filter(Boolean)
}

export function selectProjectChatClipboardImages(clipboardData, currentCount = 0) {
  const files = projectChatClipboardFiles(clipboardData)
  return {
    hasImages: files.some((file) => PROJECT_CHAT_IMAGE_MIME_TYPES.has(file?.type?.toLowerCase?.())),
    ...selectProjectChatImageFiles(files, currentCount),
  }
}

export function removeProjectChatAttachment(attachments, localId) {
  return attachments.filter((attachment) => attachment.localId !== localId)
}

export function readyProjectChatReferences(references) {
  return references
    .filter((reference) => reference.status === 'ready' && reference.resource_type && reference.resource_uuid)
    .map(({ resource_type, resource_uuid }) => ({ resource_type, resource_uuid }))
}

export function referenceKey(reference) {
  return `${reference?.resource_type || ''}:${reference?.resource_uuid || ''}`
}

export function appendProjectChatReference(references, reference) {
  if (!reference?.resource_type || !reference?.resource_uuid) return references
  if (references.some((item) => referenceKey(item) === referenceKey(reference))) return references
  if (references.filter((item) => item.status !== 'error').length >= MAX_PROJECT_CHAT_REFERENCES) return references
  return [...references, reference]
}
