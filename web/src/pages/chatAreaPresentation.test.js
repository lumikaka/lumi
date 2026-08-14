import assert from 'node:assert/strict'
import test from 'node:test'

import {
  captureChatScrollAnchor,
  chatComposerMode,
  chatThreadCountLabel,
  chatTurnElapsedMs,
  groupChatItemsByTurn,
  isChatSteeringShortcut,
  projectChatSearchWithoutLegacyScope,
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

test('mixed project threads retain localized workflow titles and existing context copy', () => {
  const t = (key) => `translated:${key}`
  const workflow = { kind: 'comic_storyboard_generation', title: 'internal' }
  assert.equal(workflowDisplayTitle(workflow, t), 'translated:chat.workflow.kind.comic_storyboard_generation')
  assert.equal(threadDisplayTitle({ title: 'internal' }, workflow, t), 'translated:chat.workflow.kind.comic_storyboard_generation')
  assert.equal(threadContextCopyKey({ scope: 'project' }, null), 'premise.threads.scene.project')
  assert.equal(threadContextCopyKey({ scope: 'premise', scene: 'asset_reference' }, null), 'premise.threads.scene.reference')
  assert.equal(threadContextCopyKey({ scope: 'project', scene: 'storyboard_reference' }, null), 'chat.scene.storyboard.title')
})

test('chapter workflows use prompt-aware localized titles with chapter context', () => {
  const t = (key, values = {}) => `${key}:${values.code || values.count || ''}`
  assert.equal(workflowDisplayTitle({ kind: 'story_chapter_generation', input_snapshot: { prompt_key: 'story_chapter', chapter_code: 'vol01.ch02' } }, t), 'chat.workflow.kind.story_chapter_with_code:vol01.ch02')
  assert.equal(workflowDisplayTitle({ kind: 'story_chapter_generation', input_snapshot: JSON.stringify({ prompt_key: 'next_story_chapter', chapter_code: 'vol01.ch03' }) }, t), 'chat.workflow.kind.next_story_chapter_with_code:vol01.ch03')
  assert.equal(workflowDisplayTitle({ kind: 'story_chapter_batch_plan', input_snapshot: { chapter_count: 4 } }, t), 'chat.workflow.kind.chapter_batch_plan_with_count:4')
  assert.equal(threadDisplayTitle({ title: 'internal' }, { kind: 'story_chapter_generation', input_snapshot: { prompt_key: 'next_story_chapter', chapter_code: 'vol01.ch03' } }, t), 'chat.workflow.kind.next_story_chapter_with_code:vol01.ch03')
})

test('workflow progress aggregates persisted step percentages', () => {
  assert.equal(workflowProgressPercent({ steps: [{ status: 'running', progress: 37 }] }), 37)
  assert.equal(workflowProgressPercent({ steps: [{ status: 'completed', progress: 0 }, { status: 'running', progress: 50 }] }), 75)
  assert.equal(workflowProgressPercent({ status: 'completed', steps: [] }), 100)
  assert.equal(workflowProgressPercent({ steps: [{ status: 'running', progress: 140 }, { status: 'queued', progress: -5 }] }), 50)
})

test('legacy chat scope is removed without dropping active thread or workspace state', () => {
  const next = projectChatSearchWithoutLegacyScope('?chat_scope=premise&chat_thread_uuid=thread-uuid&workspace_tab=body')
  assert.equal(next.has('chat_scope'), false)
  assert.equal(next.get('chat_thread_uuid'), 'thread-uuid')
  assert.equal(next.get('workspace_tab'), 'body')
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

test('assistant pending only exists before real runtime output and exposes long waits', () => {
  const turn = { status: 'in_progress', started_at: '2026-08-11T00:00:00.000Z' }
  assert.equal(shouldShowAssistantPending(turn, [{ role: 'user', item_type: 'user_message' }]), true)
  assert.equal(shouldShowAssistantPending(turn, [{ role: 'assistant', item_type: 'assistant_message' }]), false)
  assert.equal(shouldShowAssistantPending(turn, [{ role: 'tool', item_type: 'tool_call' }]), false)
  assert.equal(chatTurnElapsedMs(turn, Date.parse('2026-08-11T00:00:11.000Z')), 11_000)
})

test('chat history autoloads only near the top when an earlier page is available', () => {
  assert.equal(shouldLoadEarlierChatItems({ scrollTop: 71, hasPreviousPage: true, isFetchingPreviousPage: false }), true)
  assert.equal(shouldLoadEarlierChatItems({ scrollTop: 72, hasPreviousPage: true, isFetchingPreviousPage: false }), false)
  assert.equal(shouldLoadEarlierChatItems({ scrollTop: 20, hasPreviousPage: false, isFetchingPreviousPage: false }), false)
  assert.equal(shouldLoadEarlierChatItems({ scrollTop: 20, hasPreviousPage: true, isFetchingPreviousPage: true }), false)
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
