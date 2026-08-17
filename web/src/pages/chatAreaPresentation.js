export function chatComposerMode({ activeTurn = null, draft = '' } = {}) {
  const hasDraft = String(draft).trim().length > 0
  if (activeTurn) return hasDraft ? 'queue' : 'stop'
  return hasDraft ? 'send' : 'disabled'
}

export function chatComposerVisualState({ activeTurn = null, draft = '', pending = false, abortPending = false, attachments = [] } = {}) {
  if (abortPending) return 'stopping'
  if (pending) return 'sending'
  if (attachments.some((attachment) => attachment.status === 'error')) return 'attachment_error'
  if (attachments.some((attachment) => attachment.status === 'uploading')) return 'attachment_uploading'
  if (attachments.length) return 'attachment_ready'
  if (activeTurn?.status === 'waiting_for_input') return 'waiting_input'
  if (activeTurn) return String(draft).trim() ? 'running_queue' : 'running_stop'
  if (String(draft).includes('\n')) return 'multiline'
  return String(draft).trim() ? 'draft' : 'idle'
}

export function isChatSteeringShortcut(event) {
  if (!event) return false
  return event.key === 'Enter' && event.shiftKey === true && (event.metaKey === true || event.ctrlKey === true)
}

export function suggestedChatThreadTitle(input = '') {
  const normalized = String(input).replace(/\s+/gu, ' ').trim()
  return [...normalized].slice(0, 60).join('')
}

export function chatThreadCountLabel(loaded, total) {
  return total > loaded ? `${loaded} / ${total}` : `${loaded}`
}

const workflowKindCopy = {
  comic_section_image_generation: 'chat.workflow.kind.comic_section_image_generation',
  comic_storyboard_generation: 'chat.workflow.kind.comic_storyboard_generation',
  story_chapter_generation: 'chat.workflow.kind.story_chapter_generation',
  story_chapter_batch_plan: 'chat.workflow.kind.story_chapter_batch_plan',
}

export function workflowDisplayTitle(workflow, t) {
  const snapshot = workflowSnapshot(workflow)
  if (workflow?.kind === 'story_chapter_generation') {
    const key = snapshot.prompt_key === 'next_story_chapter'
      ? 'chat.workflow.kind.next_story_chapter'
      : 'chat.workflow.kind.story_chapter'
    return snapshot.chapter_code ? t(`${key}_with_code`, { code: snapshot.chapter_code }) : t(key)
  }
  if (workflow?.kind === 'story_chapter_batch_plan') {
    return snapshot.chapter_count
      ? t('chat.workflow.kind.chapter_batch_plan_with_count', { count: snapshot.chapter_count })
      : t('chat.workflow.kind.chapter_batch_plan')
  }
  const copyKey = workflowKindCopy[workflow?.kind]
  return copyKey ? t(copyKey) : workflow?.title || t('chat.workflow.title')
}

export function threadDisplayTitle(thread, workflow, t) {
  if (workflow) return workflowDisplayTitle(workflow, t)
  const copyKey = workflowKindCopy[thread?.title]
  if (copyKey) return t(copyKey)
  return thread?.title || t('chat.threads')
}

export function threadContextCopyKey(thread, workflow) {
  if (workflow?.kind === 'story_chapter_generation') return 'chat.workflow.kind.story_chapter_generation'
  if (workflow?.kind === 'story_chapter_batch_plan') return 'chat.workflow.kind.story_chapter_batch_plan'
  if (workflow?.kind === 'comic_storyboard_generation') return 'chat.workflow.kind.comic_storyboard_generation'
  if (workflow?.kind === 'comic_section_image_generation') return 'chat.workflow.kind.comic_section_image_generation'
  if (thread?.scene === 'premise_asset_generation') return 'premise.threads.scene.generate'
  if (thread?.scene === 'asset_reference') return 'premise.threads.scene.reference'
  if (thread?.scene === 'storyboard_reference') return 'chat.scene.storyboard.title'
  return thread?.scope === 'premise' ? 'premise.threads.scene.chat' : 'premise.threads.scene.project'
}

export function workflowProgressPercent(workflow) {
  const steps = Array.isArray(workflow?.steps) ? workflow.steps : []
  if (!steps.length) return workflow?.status === 'completed' ? 100 : 0
  const total = steps.reduce((sum, step) => {
    if (step?.status === 'completed') return sum + 100
    const progress = Number(step?.progress)
    return sum + (Number.isFinite(progress) ? Math.min(100, Math.max(0, progress)) : 0)
  }, 0)
  return Math.round(total / steps.length)
}

function workflowSnapshot(workflow) {
  const snapshot = workflow?.input_snapshot
  if (snapshot && typeof snapshot === 'object') return snapshot
  if (typeof snapshot !== 'string') return {}
  try {
    const parsed = JSON.parse(snapshot)
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

export function projectChatSearchWithoutLegacyScope(search = '') {
  const next = new URLSearchParams(search)
  next.delete('chat_scope')
  return next
}

export function shouldLoadEarlierChatItems({ scrollTop, hasPreviousPage, isFetchingPreviousPage } = {}) {
  return Boolean(hasPreviousPage && !isFetchingPreviousPage && Number(scrollTop) < 72)
}

export function captureChatScrollAnchor(container) {
  if (!container) return null
  const containerRect = container.getBoundingClientRect()
  const turns = Array.from(container.querySelectorAll('[data-turn-uuid]'))
  const anchorTurn = turns.find((turn) => turn.getBoundingClientRect().bottom >= containerRect.top + 1)

  if (!anchorTurn) return { scrollHeight: container.scrollHeight }

  return {
    turnUuid: anchorTurn.dataset.turnUuid,
    offset: anchorTurn.getBoundingClientRect().top - containerRect.top,
    scrollHeight: container.scrollHeight,
  }
}

export function restoreChatScrollAnchor(container, anchor) {
  if (!container || !anchor) return

  if (anchor.turnUuid) {
    const anchorTurn = Array.from(container.querySelectorAll('[data-turn-uuid]'))
      .find((turn) => turn.dataset.turnUuid === anchor.turnUuid)
    if (anchorTurn) {
      const containerRect = container.getBoundingClientRect()
      const nextOffset = anchorTurn.getBoundingClientRect().top - containerRect.top
      container.scrollTop += nextOffset - anchor.offset
      return
    }
  }

  if (anchor.scrollHeight) container.scrollTop += container.scrollHeight - anchor.scrollHeight
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
