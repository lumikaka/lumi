export function chatComposerMode({ activeTurn = null, draft = '' } = {}) {
  const hasDraft = String(draft).trim().length > 0
  if (activeTurn) return hasDraft ? 'queue' : 'stop'
  return hasDraft ? 'send' : 'disabled'
}

export function isChatSteeringShortcut(event) {
  if (!event) return false
  return event.key === 'Enter' && event.shiftKey === true && (event.metaKey === true || event.ctrlKey === true)
}

export function suggestedChatThreadTitle(input = '') {
  const normalized = String(input).replace(/\s+/gu, ' ').trim()
  return [...normalized].slice(0, 60).join('')
}

export function groupChatItemsByTurn(items = [], turns = []) {
  const groups = new Map()

  turns.forEach((turn) => {
    groups.set(turn.uuid, { uuid: turn.uuid, turn, items: [] })
  })

  items.forEach((item) => {
    const key = item.turn_uuid || 'unassigned'
    const group = groups.get(key) || { uuid: key, turn: null, items: [] }
    group.items.push(item)
    groups.set(key, group)
  })

  return [...groups.values()]
    .filter((group) => group.items.length > 0 || isVisibleTurn(group.turn))
    .map((group) => ({
      ...group,
      items: [...group.items].sort((left, right) => Number(left.sequence || 0) - Number(right.sequence || 0)),
    }))
    .sort(compareTurnGroups)
}

export function shouldShowAssistantPending(turn, items = []) {
  if (turn?.status !== 'in_progress') return false
  return !items.some((item) => item.role === 'assistant' || ['tool_call', 'tool_result', 'error', 'user_input_request'].includes(item.item_type))
}

export function chatTurnElapsedMs(turn, now = Date.now()) {
  const started = Date.parse(turn?.started_at || turn?.updated_at || turn?.created_at || '')
  return Number.isFinite(started) ? Math.max(0, now - started) : 0
}

function isVisibleTurn(turn) {
  return Boolean(turn && ['queued', 'in_progress', 'waiting_for_input', 'failed'].includes(turn.status))
}

function compareTurnGroups(left, right) {
  if (!left.turn && right.turn) return 1
  if (left.turn && !right.turn) return -1

  const leftSequence = Number(left.turn?.queue_sequence)
  const rightSequence = Number(right.turn?.queue_sequence)
  if (Number.isFinite(leftSequence) && Number.isFinite(rightSequence) && leftSequence !== rightSequence) {
    return leftSequence - rightSequence
  }

  const leftTime = Date.parse(left.turn?.created_at || left.items[0]?.created_at || '')
  const rightTime = Date.parse(right.turn?.created_at || right.items[0]?.created_at || '')
  if (Number.isFinite(leftTime) && Number.isFinite(rightTime) && leftTime !== rightTime) return leftTime - rightTime

  return String(left.uuid).localeCompare(String(right.uuid))
}
