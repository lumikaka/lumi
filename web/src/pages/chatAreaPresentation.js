import { isVerticalStripPictureBook } from './pictureBookProfile.js'

export function chatComposerMode({ activeTurn = null, draft = '' } = {}) {
  const hasDraft = String(draft).trim().length > 0
  if (activeTurn) return hasDraft ? 'queue' : 'stop'
  return hasDraft ? 'send' : 'disabled'
}

export function workflowFirstImageStepCopyKey(workflow, pictureBook) {
  const snapshot = workflowSnapshot(workflow)
  const frozenPictureBook = snapshot.picture_book && typeof snapshot.picture_book === 'object'
    ? snapshot.picture_book
    : pictureBook
  const version = Number(snapshot.version || 0)
  return version >= 5 && !isVerticalStripPictureBook(frozenPictureBook)
    ? 'chat.workflow.step.cover_and_first_page_image'
    : 'chat.workflow.step.first_section_image'
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
  premise_asset_generation: 'chat.workflow.kind.premise_asset_generation',
  comic_section_image_generation: 'chat.workflow.kind.comic_section_image_generation',
  comic_storyboard_generation: 'chat.workflow.kind.comic_storyboard_generation',
  story_chapter_generation: 'chat.workflow.kind.story_chapter_generation',
  story_chapter_batch_plan: 'chat.workflow.kind.story_chapter_batch_plan',
}

export function workflowDisplayTitle(workflow, t) {
  const snapshot = workflowSnapshot(workflow)
  if (workflow?.kind === 'premise_asset_generation') {
    return snapshot.asset_title
      ? t('chat.workflow.kind.premise_asset_generation_with_title', { title: snapshot.asset_title })
      : t('chat.workflow.kind.premise_asset_generation')
  }
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
	if (workflow?.presentation_mode === 'dedicated_thread') return workflowDisplayTitle(workflow, t)
  const copyKey = workflowKindCopy[thread?.title]
  if (copyKey) return t(copyKey)
  return thread?.title || t('chat.threads')
}

export function threadContextCopyKey(thread, workflow) {
	if (workflow?.presentation_mode === 'dedicated_thread' && workflow?.kind === 'story_chapter_generation') return 'chat.workflow.kind.story_chapter_generation'
	if (workflow?.presentation_mode === 'dedicated_thread' && workflow?.kind === 'story_chapter_batch_plan') return 'chat.workflow.kind.story_chapter_batch_plan'
	if (workflow?.presentation_mode === 'dedicated_thread' && workflow?.kind === 'comic_storyboard_generation') return 'chat.workflow.kind.comic_storyboard_generation'
	if (workflow?.presentation_mode === 'dedicated_thread' && workflow?.kind === 'comic_section_image_generation') return 'chat.workflow.kind.comic_section_image_generation'
	if (workflow?.presentation_mode === 'dedicated_thread' && workflow?.kind === 'premise_asset_generation') return 'chat.workflow.kind.premise_asset_generation'
  return 'chat.thread.project_context'
}

export function dedicatedWorkflowForThread(workflows = [], threadUuid = '') {
	return workflows
		.filter((workflow) => workflow?.thread_uuid === threadUuid && workflow?.presentation_mode === 'dedicated_thread')
		.sort(compareWorkflows)[0] || null
}

export function groupInlineWorkflowsByTurn(workflows = [], threadUuid = '') {
	const groups = new Map()
	workflows
		.filter((workflow) => (
			workflow?.thread_uuid === threadUuid
			&& workflow?.presentation_mode === 'inline'
			&& workflow?.origin_turn_uuid
		))
		.sort(compareWorkflows)
		.forEach((workflow) => {
			const current = groups.get(workflow.origin_turn_uuid) || []
			current.push(workflow)
			groups.set(workflow.origin_turn_uuid, current)
		})
	return groups
}

function compareWorkflows(left, right) {
	const leftTime = Date.parse(left?.created_at || '')
	const rightTime = Date.parse(right?.created_at || '')
	if (Number.isFinite(leftTime) && Number.isFinite(rightTime) && leftTime !== rightTime) return leftTime - rightTime
	if (Number.isFinite(leftTime) !== Number.isFinite(rightTime)) return Number.isFinite(leftTime) ? -1 : 1
	return String(left?.uuid || '').localeCompare(String(right?.uuid || ''))
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

export function shouldLoadEarlierChatItems({ scrollTop, hasEarlierPage, isFetchingEarlierPage } = {}) {
  return Boolean(hasEarlierPage && !isFetchingEarlierPage && Number(scrollTop) < 72)
}

export function shouldAutofillEarlierChatItems({ scrollHeight, clientHeight, hasEarlierPage, isFetchingEarlierPage } = {}) {
  return Boolean(
    hasEarlierPage
    && !isFetchingEarlierPage
    && Number(clientHeight) > 0
    && Number(scrollHeight) <= Number(clientHeight)
  )
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

const terminalChatTurnStatuses = new Set(['completed', 'failed', 'cancelled', 'interrupted'])
const toolItemTypes = new Set(['tool_call', 'tool_result'])
const safelyRecoverableToolErrorCodes = new Set(['agent_tool_validation_failed'])

export function projectChatTurnActivity(turn, items = [], { historyMayBePartial = false } = {}) {
  const conversationItems = []
  const executions = new Map()

  items.forEach((item, index) => {
    if (!toolItemTypes.has(item?.item_type)) {
      conversationItems.push(item)
      return
    }
    if (item.tool_name === 'request_user_input') return

    const toolCallUuid = String(item.tool_call_uuid || '')
    const key = toolCallUuid || `${item.item_type}:${item.uuid || item.sequence || index}`
    const sequence = finiteSequence(item.sequence, index)
    const execution = executions.get(key) || {
      key,
      toolCallUuid,
      toolName: item.tool_name || 'controlled_tool',
      call: null,
      result: null,
      sequence,
      lastSequence: sequence,
    }

    execution.toolName = execution.toolName === 'controlled_tool' && item.tool_name ? item.tool_name : execution.toolName
    execution.sequence = Math.min(execution.sequence, sequence)
    execution.lastSequence = Math.max(execution.lastSequence, sequence)
    if (item.item_type === 'tool_call') {
      execution.call ||= item
    } else {
      execution.result = item
    }
    executions.set(key, execution)
  })

  const turnStatus = turn?.status || ''
  const terminal = terminalChatTurnStatuses.has(turnStatus)
  const projectedTools = [...executions.values()]
    .sort((left, right) => left.sequence - right.sequence || left.key.localeCompare(right.key))
    .map((execution) => ({ ...execution, status: projectedToolStatus(execution, terminal) }))
  const tools = projectedTools.filter((tool) => !isRecoveredToolFailure(tool, turnStatus, projectedTools, conversationItems))
  const inferredActive = !turn && tools.some((tool) => tool.status === 'running' || tool.status === 'pending')
  const mode = turnStatus === 'in_progress' || inferredActive
    ? 'active'
    : turnStatus === 'waiting_for_workflow'
      ? 'waiting_for_workflow'
    : turnStatus === 'waiting_for_input'
      ? 'waiting_for_input'
      : terminal || (!turn && tools.length > 0)
        ? 'terminal'
        : 'idle'
  const activeTool = [...tools].reverse().find((tool) => tool.status === 'running' || tool.status === 'pending') || tools.at(-1) || null

  return {
    activeTool,
    conversationItems,
    historyMayBePartial: Boolean(historyMayBePartial),
    issueCount: tools.filter((tool) => tool.status === 'failed' || tool.status === 'interrupted').length,
    mode,
    summaryIndex: terminalSummaryIndex(conversationItems),
    tools,
  }
}

function finiteSequence(value, fallback) {
  const sequence = Number(value)
  return Number.isFinite(sequence) ? sequence : 1_000_000_000_000_000 + fallback
}

function projectedToolStatus(execution, terminal) {
  if (execution.call?.status === 'failed' || execution.result?.status === 'failed' || toolResultFailed(execution.result?.content)) return 'failed'
  if (execution.result) return 'completed'
  if (terminal) return execution.call?.status === 'completed' ? 'completed' : 'interrupted'
  if (execution.call?.status === 'in_progress') return 'running'
  if (execution.call?.status === 'completed') return 'completed'
  return 'pending'
}

function toolResultFailed(content) {
  if (typeof content !== 'string' || !content.trim()) return false
  try {
    return JSON.parse(content)?.success === false
  } catch {
    return false
  }
}

function isRecoveredToolFailure(tool, turnStatus, tools, conversationItems) {
  if (turnStatus !== 'completed' || tool.status !== 'failed') return false
  if (!safelyRecoverableToolErrorCodes.has(toolResultErrorCode(tool.result?.content))) return false

  const laterCompletedTool = tools.some((candidate) => (
    candidate.key !== tool.key
    && candidate.sequence > tool.lastSequence
    && candidate.status === 'completed'
  ))
  const laterInputRequest = conversationItems.some((item, index) => (
    item?.item_type === 'user_input_request'
    && finiteSequence(item.sequence, index) > tool.lastSequence
  ))
  return laterCompletedTool || laterInputRequest
}

function toolResultErrorCode(content) {
  if (typeof content !== 'string' || !content.trim()) return ''
  try {
    return String(JSON.parse(content)?.error?.code || '')
  } catch {
    return ''
  }
}

export function projectChatUserInput(request = {}) {
  const response = parseObject(request.response)
  const codexQuestions = Array.isArray(request.questions) ? request.questions : []
  const questions = codexQuestions.length > 0
    ? codexQuestions.map((question) => projectCodexUserInputQuestion(question, response.answers?.[question.id]))
    : [projectLegacyUserInputQuestion(request, response)]
  const answers = questions.flatMap((question) => question.answers)
  const selectedOptionUuids = questions.flatMap((question) => question.selectedOptionUuids)
  const otherText = questions.map((question) => question.otherText).filter(Boolean).join('\n')

  return {
    answers,
    mode: request.status === 'pending' ? 'pending' : answers.length > 0 ? 'answered' : 'incomplete',
    otherText,
    questions,
    selectedOptionUuids,
  }
}

export function activeProjectChatUserInputRequest(requests = [], activeTurn = null) {
  const pending = requests.filter((request) => request?.status === 'pending')
  if (!pending.length) return null
  if (activeTurn?.uuid) {
    const activeRequest = pending.find((request) => request.turn_uuid === activeTurn.uuid)
    if (activeRequest) return activeRequest
  }
  return pending.at(-1) || null
}

function projectCodexUserInputQuestion(question = {}, answerValue) {
  const answer = parseObject(answerValue)
  const options = Array.isArray(question.options) ? question.options : []
  const selectedOptionUuid = String(answer.selected_option_uuid || '')
  const otherText = String(answer.other_text || '').trim()
  const selected = options.find((option) => String(option?.uuid) === selectedOptionUuid)
  const answers = selectedOptionUuid ? [selected?.label || selectedOptionUuid] : otherText ? [otherText] : []
  return {
    answers,
    header: String(question.header || ''),
    id: String(question.id || ''),
    options,
    otherText,
    question: String(question.question || ''),
    selectedOptionUuid,
    selectedOptionUuids: selectedOptionUuid ? [selectedOptionUuid] : [],
  }
}

function projectLegacyUserInputQuestion(request, response) {
  const responseOptions = new Map(
    (Array.isArray(response.selected_options) ? response.selected_options : [])
      .filter((option) => option?.uuid)
      .map((option) => [option.uuid, option]),
  )
  const options = Array.isArray(request.options) ? request.options : []
  const requestedOptions = new Map(options.filter((option) => option?.uuid).map((option) => [option.uuid, option]))
  const selectedOptionUuids = Array.isArray(response.selected_option_uuids)
    ? response.selected_option_uuids.map(String)
    : [...responseOptions.keys()]
  const otherText = String(response.other_text || '').trim()
  const answers = selectedOptionUuids.map((uuid) => requestedOptions.get(uuid)?.label || responseOptions.get(uuid)?.label || uuid)
  if (otherText) answers.push(otherText)
  return {
    answers,
    header: '',
    id: 'legacy_question',
    inputType: request.input_type || 'single_choice',
    options,
    otherText,
    question: String(request.question || ''),
    selectedOptionUuid: selectedOptionUuids[0] || '',
    selectedOptionUuids,
  }
}

function parseObject(value) {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value
  if (typeof value !== 'string' || !value.trim()) return {}
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {}
  } catch {
    return {}
  }
}

function terminalSummaryIndex(items) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (items[index]?.item_type === 'assistant_message') return index
  }
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (items[index]?.item_type === 'error') return index
  }
  return items.length
}

export function shouldShowAssistantPending(turn, items = []) {
  if (turn?.status !== 'in_progress') return false
  return !items.some((item) => item.role === 'assistant' || ['tool_call', 'tool_result', 'error', 'user_input_request'].includes(item.item_type))
}

export function chatTurnElapsedMs(turn, now = Date.now()) {
  const started = Date.parse(turn?.started_at || turn?.updated_at || turn?.created_at || '')
  return Number.isFinite(started) ? Math.max(0, now - started) : 0
}

export function chatTurnDurationMs(turn) {
  const started = Date.parse(turn?.started_at || turn?.created_at || '')
  const completed = Date.parse(turn?.completed_at || turn?.updated_at || '')
  if (!Number.isFinite(started) || !Number.isFinite(completed)) return null
  return Math.max(0, completed - started)
}

function isVisibleTurn(turn) {
  return Boolean(turn && ['queued', 'in_progress', 'waiting_for_input', 'waiting_for_workflow', 'failed'].includes(turn.status))
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
