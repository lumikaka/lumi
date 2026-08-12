export const MAX_PROJECT_CHAT_IMAGE_REFERENCES = 4
const PROJECT_CHAT_IMAGE_MIME_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp'])

export function canProjectChatAttachImages(scene) {
  return scene === 'premise_asset_generation' || scene === 'asset_reference'
}

export function selectProjectChatImageFiles(files, currentCount = 0) {
  const candidates = Array.from(files || [])
  const images = candidates.filter((file) => PROJECT_CHAT_IMAGE_MIME_TYPES.has(file?.type?.toLowerCase?.()))
  const available = Math.max(MAX_PROJECT_CHAT_IMAGE_REFERENCES - currentCount, 0)
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

export function readyProjectChatUploadUUIDs(attachments) {
  return attachments
    .filter((attachment) => attachment.status === 'ready' && attachment.upload?.uuid)
    .map((attachment) => attachment.upload.uuid)
}
