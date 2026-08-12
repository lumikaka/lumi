import assert from 'node:assert/strict'
import test from 'node:test'

import { chatComposerMode, chatTurnElapsedMs, groupChatItemsByTurn, isChatSteeringShortcut, shouldShowAssistantPending, suggestedChatThreadTitle } from './chatAreaPresentation.js'

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
