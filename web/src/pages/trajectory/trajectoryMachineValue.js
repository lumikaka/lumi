export function safeMachineJSON(value) {
  if (value == null || value === '') return '—'

  let normalized = value
  if (typeof value === 'string') {
    try { normalized = JSON.parse(value) } catch { return value }
  }

  try {
    return JSON.stringify(normalized, null, 2) ?? String(normalized)
  } catch {
    return String(normalized)
  }
}

export function jsonSyntaxSegments(value) {
  const source = safeMachineJSON(value)
  const pattern = /("(?:\\(?:["\\/bfnrt]|u[0-9a-fA-F]{4})|[^"\\])*")(?=\s*:)|("(?:\\(?:["\\/bfnrt]|u[0-9a-fA-F]{4})|[^"\\])*")|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|\btrue\b|\bfalse\b|\bnull\b/g
  const segments = []
  let cursor = 0
  let match

  while ((match = pattern.exec(source)) !== null) {
    if (match.index > cursor) segments.push({ kind: 'plain', text: source.slice(cursor, match.index), offset: cursor })
    const text = match[0]
    let kind = 'number'
    if (match[1]) kind = 'key'
    else if (match[2]) kind = 'string'
    else if (text === 'true' || text === 'false') kind = 'boolean'
    else if (text === 'null') kind = 'null'
    segments.push({ kind, text, offset: match.index })
    cursor = pattern.lastIndex
  }

  if (cursor < source.length) segments.push({ kind: 'plain', text: source.slice(cursor), offset: cursor })
  return segments.length ? segments : [{ kind: 'plain', text: source, offset: 0 }]
}
