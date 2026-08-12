function normalizeReadableText(value) {
  if (typeof value !== 'string') return ''

  const text = value.replace(/\r\n?/g, '\n')
  return text.trim() ? text : ''
}

export function extractReadablePrompt(requestPayload) {
  return normalizeReadableText(requestPayload?.prompt)
}

export function extractReadableResponse(response) {
  return normalizeReadableText(
    typeof response?.content === 'string' ? response.content : response?.message?.content,
  )
}
