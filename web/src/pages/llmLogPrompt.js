function normalizeReadableText(value) {
  if (typeof value !== 'string') return ''

  const text = value.replace(/\r\n?/g, '\n')
  return text.trim() ? text : ''
}

function extractMessageContent(content) {
  if (typeof content === 'string') return normalizeReadableText(content)
  if (!Array.isArray(content)) return ''

  return content
    .map((part) => {
      if (typeof part === 'string') return normalizeReadableText(part)
      if (!['text', 'input_text', 'output_text'].includes(part?.type)) return ''
      return normalizeReadableText(part.text)
    })
    .filter(Boolean)
    .join('\n')
}

function formatToolCall(toolCall) {
  const name = toolCall?.name || toolCall?.function?.name
  const args = toolCall?.arguments ?? toolCall?.function?.arguments
  const argsText = typeof args === 'string'
    ? normalizeReadableText(args)
    : args == null
      ? ''
      : JSON.stringify(args, null, 2)

  if (!name && !argsText) return ''

  const heading = name ? `[tool_call: ${name}]` : '[tool_call]'
  return argsText ? `${heading}\n${argsText}` : heading
}

function formatMessage(message) {
  const content = extractMessageContent(message?.content)
  const toolCalls = Array.isArray(message?.tool_calls)
    ? message.tool_calls.map(formatToolCall).filter(Boolean).join('\n\n')
    : ''
  const body = [content, toolCalls].filter(Boolean).join('\n\n')
  if (!body) return ''

  const role = normalizeReadableText(message?.role) || 'message'
  return `[${role}]\n${body}`
}

export function extractReadablePrompt(requestPayload) {
  const prompt = normalizeReadableText(requestPayload?.prompt)
  if (prompt) return prompt
  if (!Array.isArray(requestPayload?.messages)) return ''

  return requestPayload.messages
    .map(formatMessage)
    .filter(Boolean)
    .join('\n\n')
}

export function extractReadableResponse(response) {
  return normalizeReadableText(
    typeof response?.content === 'string' ? response.content : response?.message?.content,
  )
}
