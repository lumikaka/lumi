import assert from 'node:assert/strict'
import test from 'node:test'

import {
  captureChatScrollAnchor,
  chatComposerMode,
  chatThreadCountLabel,
  chatTurnDurationMs,
  chatTurnElapsedMs,
	dedicatedWorkflowForThread,
  groupChatItemsByTurn,
	groupInlineWorkflowsByTurn,
  isChatSteeringShortcut,
  projectChatTurnActivity,
  projectChatUserInput,
  restoreChatScrollAnchor,
  shouldLoadEarlierChatItems,
  shouldShowAssistantPending,
  suggestedChatThreadTitle,
  threadContextCopyKey,
  threadDisplayTitle,
  workflowDisplayTitle,
  workflowProgressPercent,
} from './chatAreaPresentation.js'

test('composer supports send, queue, stop and steering behavior', () => {
  assert.equal(chatComposerMode(), 'disabled')
  assert.equal(chatComposerMode({ draft: '继续写' }), 'send')
  assert.equal(chatComposerMode({ activeTurn: { status: 'in_progress' } }), 'stop')
  assert.equal(chatComposerMode({ activeTurn: { status: 'in_progress' }, draft: '换个方向' }), 'queue')
  assert.equal(isChatSteeringShortcut({ key: 'Enter', shiftKey: true, metaKey: true }), true)
  assert.equal(isChatSteeringShortcut({ key: 'Enter', shiftKey: true, ctrlKey: true }), true)
  assert.equal(isChatSteeringShortcut({ key: 'Enter', shiftKey: false, metaKey: true }), false)
})

test('new thread titles use the first-message suggestion', () => {
  assert.equal(suggestedChatThreadTitle('  月光邮局\n 需要一位新店员  '), '月光邮局 需要一位新店员')
  assert.equal(suggestedChatThreadTitle('星'.repeat(61)), '星'.repeat(60))
})

test('thread pagination count shows progress until every thread is loaded', () => {
  assert.equal(chatThreadCountLabel(20, 45), '20 / 45')
  assert.equal(chatThreadCountLabel(45, 45), '45')
  assert.equal(chatThreadCountLabel(0, 0), '0')
})

test('mixed project threads retain localized workflow titles and use generic project context copy', () => {
  const t = (key) => `translated:${key}`
	const workflow = { kind: 'comic_storyboard_generation', title: 'internal', presentation_mode: 'dedicated_thread' }
  assert.equal(workflowDisplayTitle(workflow, t), 'translated:chat.workflow.kind.comic_storyboard_generation')
  assert.equal(threadDisplayTitle({ title: 'internal' }, workflow, t), 'translated:chat.workflow.kind.comic_storyboard_generation')
	assert.equal(threadDisplayTitle({ title: '正常对话' }, { ...workflow, presentation_mode: 'inline' }, t), '正常对话')
  assert.equal(threadContextCopyKey({}, null), 'chat.thread.project_context')
  assert.equal(threadContextCopyKey({ scope: 'premise', scene: 'asset_reference' }, null), 'chat.thread.project_context')
})

test('chapter workflows use prompt-aware localized titles with chapter context', () => {
  const t = (key, values = {}) => `${key}:${values.code || values.count || ''}`
  assert.equal(workflowDisplayTitle({ kind: 'story_chapter_generation', input_snapshot: { prompt_key: 'story_chapter', chapter_code: 'vol01.ch02' } }, t), 'chat.workflow.kind.story_chapter_with_code:vol01.ch02')
  assert.equal(workflowDisplayTitle({ kind: 'story_chapter_generation', input_snapshot: JSON.stringify({ prompt_key: 'next_story_chapter', chapter_code: 'vol01.ch03' }) }, t), 'chat.workflow.kind.next_story_chapter_with_code:vol01.ch03')
  assert.equal(workflowDisplayTitle({ kind: 'story_chapter_batch_plan', input_snapshot: { chapter_count: 4 } }, t), 'chat.workflow.kind.chapter_batch_plan_with_count:4')
	assert.equal(threadDisplayTitle({ title: 'internal' }, { kind: 'story_chapter_generation', presentation_mode: 'dedicated_thread', input_snapshot: { prompt_key: 'next_story_chapter', chapter_code: 'vol01.ch03' } }, t), 'chat.workflow.kind.next_story_chapter_with_code:vol01.ch03')
})

test('dedicated and inline workflows are separated and inline cards sort stably within their origin turn', () => {
	const workflows = [
		{ uuid: 'workflow-b', thread_uuid: 'thread-1', origin_turn_uuid: 'turn-1', presentation_mode: 'inline', created_at: '2026-08-26T00:00:02Z' },
		{ uuid: 'dedicated', thread_uuid: 'thread-1', presentation_mode: 'dedicated_thread', created_at: '2026-08-26T00:00:00Z' },
		{ uuid: 'workflow-c', thread_uuid: 'thread-2', origin_turn_uuid: 'turn-1', presentation_mode: 'inline', created_at: '2026-08-26T00:00:00Z' },
		{ uuid: 'workflow-a', thread_uuid: 'thread-1', origin_turn_uuid: 'turn-1', presentation_mode: 'inline', created_at: '2026-08-26T00:00:01Z' },
		{ uuid: 'workflow-d', thread_uuid: 'thread-1', origin_turn_uuid: 'turn-2', presentation_mode: 'inline', created_at: '2026-08-26T00:00:03Z' },
	]

	assert.equal(dedicatedWorkflowForThread(workflows, 'thread-1')?.uuid, 'dedicated')
	const groups = groupInlineWorkflowsByTurn(workflows, 'thread-1')
	assert.deepEqual([...groups.keys()], ['turn-1', 'turn-2'])
	assert.deepEqual(groups.get('turn-1').map((workflow) => workflow.uuid), ['workflow-a', 'workflow-b'])
})

test('workflow progress aggregates persisted step percentages', () => {
  assert.equal(workflowProgressPercent({ steps: [{ status: 'running', progress: 37 }] }), 37)
  assert.equal(workflowProgressPercent({ steps: [{ status: 'completed', progress: 0 }, { status: 'running', progress: 50 }] }), 75)
  assert.equal(workflowProgressPercent({ status: 'completed', steps: [] }), 100)
  assert.equal(workflowProgressPercent({ steps: [{ status: 'running', progress: 140 }, { status: 'queued', progress: -5 }] }), 50)
})

test('chat items are grouped and ordered by public turn UUID and queue sequence', () => {
  const groups = groupChatItemsByTurn([
    { uuid: 'item-2', turn_uuid: 'turn-2', sequence: 4 },
    { uuid: 'item-1', turn_uuid: 'turn-1', sequence: 2 },
    { uuid: 'item-3', turn_uuid: 'turn-1', sequence: 1 },
  ], [
    { uuid: 'turn-2', queue_sequence: 2, status: 'completed' },
    { uuid: 'turn-1', queue_sequence: 1, status: 'completed' },
  ])

  assert.deepEqual(groups.map((group) => group.uuid), ['turn-1', 'turn-2'])
  assert.deepEqual(groups[0].items.map((item) => item.uuid), ['item-3', 'item-1'])
})

test('active turns remain visible before the first persisted item arrives', () => {
  const groups = groupChatItemsByTurn([], [
    { uuid: 'turn-complete', queue_sequence: 1, status: 'completed' },
    { uuid: 'turn-running', queue_sequence: 2, status: 'in_progress' },
  ])

  assert.deepEqual(groups.map((group) => group.uuid), ['turn-running'])
})

test('workflow-waiting turns stay visible and expose a distinct activity mode', () => {
	const groups = groupChatItemsByTurn([], [
		{ uuid: 'turn-waiting', queue_sequence: 1, status: 'waiting_for_workflow' },
	])
	assert.deepEqual(groups.map((group) => group.uuid), ['turn-waiting'])
	assert.equal(projectChatTurnActivity(groups[0].turn, []).mode, 'waiting_for_workflow')
})

test('completed turn activity pairs tool calls and results without polluting conversation items', () => {
  const activity = projectChatTurnActivity({ status: 'completed' }, [
    { uuid: 'user', sequence: 1, item_type: 'user_message', role: 'user' },
    { uuid: 'call', sequence: 2, item_type: 'tool_call', tool_call_uuid: 'tool-1', tool_name: 'image_gen', status: 'completed', content: '{"prompt":"moon"}' },
    { uuid: 'result', sequence: 3, item_type: 'tool_result', tool_call_uuid: 'tool-1', tool_name: 'image_gen', status: 'completed', content: '{"success":true}' },
    { uuid: 'assistant', sequence: 4, item_type: 'assistant_message', role: 'assistant' },
  ])

  assert.equal(activity.mode, 'terminal')
  assert.deepEqual(activity.conversationItems.map((item) => item.uuid), ['user', 'assistant'])
  assert.equal(activity.tools.length, 1)
  assert.equal(activity.tools[0].call.uuid, 'call')
  assert.equal(activity.tools[0].result.uuid, 'result')
  assert.equal(activity.tools[0].status, 'completed')
  assert.equal(activity.summaryIndex, 1)
})

test('turn activity derives failures, interruptions and partial-history state', () => {
  const activity = projectChatTurnActivity({ status: 'failed' }, [
    { uuid: 'failed-call', sequence: 1, item_type: 'tool_call', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'failed' },
    { uuid: 'failed-result', sequence: 2, item_type: 'tool_result', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'completed', content: '{"success":false,"error":{"code":"bad"}}' },
    { uuid: 'unfinished-call', sequence: 3, item_type: 'tool_call', tool_call_uuid: 'tool-2', tool_name: 'image_gen', status: 'in_progress' },
    { uuid: 'orphan-result', sequence: 4, item_type: 'tool_result', tool_call_uuid: 'tool-3', tool_name: 'read_agent_doc', status: 'completed', content: '{"success":true}' },
    { uuid: 'error', sequence: 5, item_type: 'error', role: 'system' },
  ], { historyMayBePartial: true })

  assert.deepEqual(activity.tools.map((tool) => tool.status), ['failed', 'interrupted', 'completed'])
  assert.equal(activity.issueCount, 2)
  assert.equal(activity.historyMayBePartial, true)
  assert.equal(activity.summaryIndex, 0)
})

test('completed turns hide safely recovered validation failures from tool activity', () => {
  const activity = projectChatTurnActivity({ status: 'completed' }, [
    { uuid: 'failed-call', sequence: 1, item_type: 'tool_call', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'failed' },
    { uuid: 'failed-result', sequence: 2, item_type: 'tool_result', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'completed', content: '{"success":false,"error":{"code":"agent_tool_validation_failed"}}' },
    { uuid: 'recovery-call', sequence: 3, item_type: 'tool_call', tool_call_uuid: 'tool-2', tool_name: 'read_agent_doc', status: 'completed' },
    { uuid: 'recovery-result', sequence: 4, item_type: 'tool_result', tool_call_uuid: 'tool-2', tool_name: 'read_agent_doc', status: 'completed', content: '{"success":true}' },
    { uuid: 'assistant', sequence: 5, item_type: 'assistant_message', role: 'assistant' },
  ])

  assert.deepEqual(activity.tools.map((tool) => tool.key), ['tool-2'])
  assert.equal(activity.issueCount, 0)
})

test('completed turns retain failures that are unsafe or have no observed recovery', () => {
  const unsafe = projectChatTurnActivity({ status: 'completed' }, [
    { uuid: 'failed-call', sequence: 1, item_type: 'tool_call', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'failed' },
    { uuid: 'failed-result', sequence: 2, item_type: 'tool_result', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'completed', content: '{"success":false,"error":{"code":"production_state_conflict"}}' },
    { uuid: 'later-call', sequence: 3, item_type: 'tool_call', tool_call_uuid: 'tool-2', tool_name: 'request_api', status: 'completed' },
    { uuid: 'later-result', sequence: 4, item_type: 'tool_result', tool_call_uuid: 'tool-2', tool_name: 'request_api', status: 'completed', content: '{"success":true}' },
  ])
  const notRecovered = projectChatTurnActivity({ status: 'completed' }, [
    { uuid: 'failed-call', sequence: 1, item_type: 'tool_call', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'failed' },
    { uuid: 'failed-result', sequence: 2, item_type: 'tool_result', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'completed', content: '{"success":false,"error":{"code":"agent_tool_validation_failed"}}' },
    { uuid: 'assistant', sequence: 3, item_type: 'assistant_message', role: 'assistant' },
  ])

  assert.equal(unsafe.tools.length, 2)
  assert.equal(unsafe.issueCount, 1)
  assert.equal(notRecovered.tools.length, 1)
  assert.equal(notRecovered.issueCount, 1)
})

test('active turn activity exposes only the latest running logical tool', () => {
  const activity = projectChatTurnActivity({ status: 'in_progress' }, [
    { uuid: 'old-call', sequence: 1, item_type: 'tool_call', tool_call_uuid: 'tool-1', tool_name: 'read_agent_doc', status: 'completed' },
    { uuid: 'old-result', sequence: 2, item_type: 'tool_result', tool_call_uuid: 'tool-1', tool_name: 'read_agent_doc', status: 'completed', content: '{}' },
    { uuid: 'active-call', sequence: 3, item_type: 'tool_call', tool_call_uuid: 'tool-2', tool_name: 'unknown_tool', status: 'in_progress' },
  ])

  assert.equal(activity.mode, 'active')
  assert.equal(activity.activeTool.toolName, 'unknown_tool')
  assert.equal(activity.activeTool.status, 'running')
  assert.deepEqual(activity.conversationItems, [])
})

test('request user input stays in the conversation and outside tool summaries', () => {
  const activity = projectChatTurnActivity({ status: 'waiting_for_input' }, [
    { uuid: 'call', sequence: 1, item_type: 'tool_call', tool_call_uuid: 'tool-1', tool_name: 'request_user_input', status: 'completed' },
    { uuid: 'request', sequence: 2, item_type: 'user_input_request', role: 'assistant' },
    { uuid: 'result', sequence: 3, item_type: 'tool_result', tool_call_uuid: 'tool-1', tool_name: 'request_user_input', status: 'completed' },
  ])

  assert.equal(activity.mode, 'waiting_for_input')
  assert.deepEqual(activity.conversationItems.map((item) => item.uuid), ['request'])
  assert.deepEqual(activity.tools, [])
})

test('user input projection keeps pending requests interactive and resolves historical answers', () => {
  const request = {
    status: 'resumed',
    options: [
      { uuid: 'option-1', label: '角色' },
      { uuid: 'option-2', label: '场景' },
    ],
    response: JSON.stringify({ selected_option_uuids: ['option-1'], other_text: '需要成长弧光' }),
  }

  assert.deepEqual(projectChatUserInput(request), {
    answers: ['角色', '需要成长弧光'],
    mode: 'answered',
    otherText: '需要成长弧光',
    questions: [{
      answers: ['角色', '需要成长弧光'],
      header: '',
      id: 'legacy_question',
      inputType: 'single_choice',
      options: request.options,
      otherText: '需要成长弧光',
      question: '',
      selectedOptionUuid: 'option-1',
      selectedOptionUuids: ['option-1'],
    }],
    selectedOptionUuids: ['option-1'],
  })
  assert.equal(projectChatUserInput({ status: 'pending', options: [] }).mode, 'pending')
  assert.equal(projectChatUserInput({ status: 'cancelled', options: [] }).mode, 'incomplete')
})

test('Codex-style user input projection keeps per-question selected and Other answers', () => {
  const request = {
    schema_version: 'codex_questions_v1',
    status: 'resumed',
    questions: [
      { id: 'style', header: '风格', question: '选择风格？', options: [{ uuid: 'style-1', label: '手绘 (Recommended)' }, { uuid: 'style-2', label: '写实' }] },
      { id: 'pages', header: '页数', question: '选择页数？', options: [{ uuid: 'pages-1', label: '八页 (Recommended)' }, { uuid: 'pages-2', label: '十六页' }] },
    ],
    response: { answers: { style: { selected_option_uuid: 'style-2', other_text: '' }, pages: { selected_option_uuid: '', other_text: '12 页' } } },
  }
  const projected = projectChatUserInput(request)
  assert.equal(projected.mode, 'answered')
  assert.deepEqual(projected.answers, ['写实', '12 页'])
  assert.deepEqual(projected.selectedOptionUuids, ['style-2'])
  assert.equal(projected.questions[0].selectedOptionUuid, 'style-2')
  assert.equal(projected.questions[1].otherText, '12 页')
})

test('terminal tool summary is placed before the final response after steering messages', () => {
  const activity = projectChatTurnActivity({ status: 'completed' }, [
    { uuid: 'first-user', sequence: 1, item_type: 'user_message', role: 'user' },
    { uuid: 'call', sequence: 2, item_type: 'tool_call', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'completed' },
    { uuid: 'steering-user', sequence: 3, item_type: 'user_message', role: 'user' },
    { uuid: 'result', sequence: 4, item_type: 'tool_result', tool_call_uuid: 'tool-1', tool_name: 'request_api', status: 'completed', content: '{}' },
    { uuid: 'assistant', sequence: 5, item_type: 'assistant_message', role: 'assistant' },
  ])

  assert.deepEqual(activity.conversationItems.map((item) => item.uuid), ['first-user', 'steering-user', 'assistant'])
  assert.equal(activity.summaryIndex, 2)
})

test('assistant pending only exists before real runtime output and exposes long waits', () => {
  const turn = { status: 'in_progress', started_at: '2026-08-11T00:00:00.000Z' }
  assert.equal(shouldShowAssistantPending(turn, [{ role: 'user', item_type: 'user_message' }]), true)
  assert.equal(shouldShowAssistantPending(turn, [{ role: 'assistant', item_type: 'assistant_message' }]), false)
  assert.equal(shouldShowAssistantPending(turn, [{ role: 'tool', item_type: 'tool_call' }]), false)
  assert.equal(chatTurnElapsedMs(turn, Date.parse('2026-08-11T00:00:11.000Z')), 11_000)
})

test('terminal turn duration prefers execution timestamps and safely falls back to persisted bounds', () => {
  assert.equal(chatTurnDurationMs({
    started_at: '2026-08-11T00:00:00.000Z',
    completed_at: '2026-08-11T00:07:49.000Z',
  }), 469_000)
  assert.equal(chatTurnDurationMs({
    created_at: '2026-08-11T00:00:00.000Z',
    updated_at: '2026-08-11T00:00:02.500Z',
  }), 2_500)
  assert.equal(chatTurnDurationMs({ completed_at: '2026-08-11T00:00:02.500Z' }), null)
  assert.equal(chatTurnDurationMs({
    started_at: '2026-08-11T00:00:03.000Z',
    completed_at: '2026-08-11T00:00:02.500Z',
  }), 0)
})

test('chat history autoloads only near the top when an earlier page is available', () => {
  assert.equal(shouldLoadEarlierChatItems({ scrollTop: 71, hasEarlierPage: true, isFetchingEarlierPage: false }), true)
  assert.equal(shouldLoadEarlierChatItems({ scrollTop: 72, hasEarlierPage: true, isFetchingEarlierPage: false }), false)
  assert.equal(shouldLoadEarlierChatItems({ scrollTop: 20, hasEarlierPage: false, isFetchingEarlierPage: false }), false)
  assert.equal(shouldLoadEarlierChatItems({ scrollTop: 20, hasEarlierPage: true, isFetchingEarlierPage: true }), false)
})

test('chat history restores the visible turn after prepending an earlier page', () => {
  const firstTurn = { dataset: { turnUuid: 'turn-1' }, getBoundingClientRect: () => ({ top: 20, bottom: 90 }) }
  const visibleTurn = { dataset: { turnUuid: 'turn-2' }, getBoundingClientRect: () => ({ top: 120, bottom: 190 }) }
  const container = {
    scrollHeight: 500,
    scrollTop: 40,
    getBoundingClientRect: () => ({ top: 100 }),
    querySelectorAll: () => [firstTurn, visibleTurn],
  }

  const anchor = captureChatScrollAnchor(container)
  assert.deepEqual(anchor, { turnUuid: 'turn-2', offset: 20, scrollHeight: 500 })

  visibleTurn.getBoundingClientRect = () => ({ top: 150, bottom: 220 })
  restoreChatScrollAnchor(container, anchor)
  assert.equal(container.scrollTop, 70)
})

test('chat history falls back to the scroll height delta when its turn anchor is unavailable', () => {
  const container = {
    scrollHeight: 700,
    scrollTop: 40,
    getBoundingClientRect: () => ({ top: 100 }),
    querySelectorAll: () => [],
  }
  const anchor = captureChatScrollAnchor({ ...container, scrollHeight: 500 })
  restoreChatScrollAnchor(container, anchor)
  assert.equal(container.scrollTop, 240)
  assert.equal(captureChatScrollAnchor(null), null)
  restoreChatScrollAnchor(null, anchor)
})
