import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { agentQueryKeysForEvent, comicStoryboardOverwriteRequest, workflowControls } from './chatWorkspaceState.js'

test('local mutations invalidate persistent thread and core workflow recovery queries', () => {
  assert.deepEqual(agentQueryKeysForEvent('project-uuid', { thread_uuid: 'thread-uuid', workflow_uuid: 'workflow-uuid' }), [
    ['chat-threads', 'project-uuid'],
    ['chat-items', 'project-uuid', 'thread-uuid'],
    ['chat-turns', 'project-uuid', 'thread-uuid'],
    ['chat-follow-ups', 'project-uuid', 'thread-uuid'],
    ['chat-input-requests', 'project-uuid', 'thread-uuid'],
    ['chat-events', 'project-uuid', 'thread-uuid'],
    ['chat-thread', 'project-uuid', 'thread-uuid'],
    ['workflows', 'project-uuid'],
    ['workflow', 'project-uuid', 'workflow-uuid'],
    ['workflow-runs', 'project-uuid', 'workflow-uuid'],
    ['workflow-events', 'project-uuid', 'workflow-uuid'],
  ])
  assert.deepEqual(agentQueryKeysForEvent('project-uuid', { thread_uuid: 'thread-uuid' }).map((key) => key[0]), ['chat-threads', 'chat-items', 'chat-turns', 'chat-follow-ups', 'chat-input-requests', 'chat-events', 'chat-thread'])
  assert.deepEqual(agentQueryKeysForEvent('project-uuid', { workflow_uuid: 'workflow-uuid' }).map((key) => key[0]), ['workflows', 'workflow', 'workflow-runs', 'workflow-events'])
  assert.deepEqual(agentQueryKeysForEvent('project-uuid', {}), [])
})

test('workflow controls cover retry and cancellation states', () => {
  assert.deepEqual(workflowControls({ status: 'failed' }), { canCancel: false, canRetry: true })
  assert.deepEqual(workflowControls({ status: 'running' }), { canCancel: true, canRetry: false })
})

test('comic storyboard conflict exposes only a valid overwrite confirmation', () => {
  const workflow = {
    kind: 'comic_storyboard_generation',
    status: 'running',
    steps: [{
      step_key: 'comic_storyboard',
      status: 'waiting',
      output: {
        action_required: 'confirm_comic_storyboard_overwrite',
        existing_section_count: 10,
        generated_section_count: 12,
        expected_comic_state_revision: 7,
      },
    }],
  }
  assert.deepEqual(comicStoryboardOverwriteRequest(workflow), {
    existingSectionCount: 10,
    generatedSectionCount: 12,
    expectedComicStateRevision: 7,
  })
  assert.deepEqual(workflowControls(workflow), { canCancel: false, canRetry: false })
  assert.equal(comicStoryboardOverwriteRequest({ ...workflow, steps: [{ ...workflow.steps[0], status: 'running' }] }), null)
  assert.equal(comicStoryboardOverwriteRequest({ ...workflow, steps: [{ ...workflow.steps[0], output: '{bad json' }] }), null)
})

test('ChatArea presents comic storyboard workflows with localized kind and step copy', () => {
  const chatArea = readFileSync(new URL('../components/ChatArea.jsx', import.meta.url), 'utf8')
  const presentation = readFileSync(new URL('./chatAreaPresentation.js', import.meta.url), 'utf8')
  const messages = readFileSync(new URL('../i18n/messages/chat.js', import.meta.url), 'utf8')
  assert.match(presentation, /comic_storyboard_generation: 'chat\.workflow\.kind\.comic_storyboard_generation'/)
  assert.match(chatArea, /comic_storyboard: 'chat\.workflow\.step\.comic_storyboard'/)
  assert.match(messages, /'chat\.workflow\.kind\.comic_storyboard_generation': \['漫画分镜生成', 'Comic storyboard generation'\]/)
  assert.match(messages, /'chat\.workflow\.step\.comic_storyboard': \['生成漫画分镜', 'Generate comic storyboard'\]/)
  assert.match(chatArea, /resolveWorkflowConflict/)
  assert.match(messages, /'chat\.workflow\.conflict\.overwrite': \['覆盖现有分镜', 'Overwrite'\]/)
})

test('ChatArea presents chapter workflows and persisted task progress', () => {
  const chatArea = readFileSync(new URL('../components/ChatArea.jsx', import.meta.url), 'utf8')
  const presentation = readFileSync(new URL('./chatAreaPresentation.js', import.meta.url), 'utf8')
  const messages = readFileSync(new URL('../i18n/messages/chat.js', import.meta.url), 'utf8')
  assert.match(presentation, /story_chapter_generation: 'chat\.workflow\.kind\.story_chapter_generation'/)
  assert.match(presentation, /story_chapter_batch_plan: 'chat\.workflow\.kind\.story_chapter_batch_plan'/)
  assert.match(chatArea, /story_chapter: 'chat\.workflow\.step\.story_chapter'/)
  assert.match(chatArea, /chapter_batch_plan: 'chat\.workflow\.step\.chapter_batch_plan'/)
  assert.match(chatArea, /workflowProgressPercent\(workflow\)/)
  assert.match(messages, /'chat\.workflow\.kind\.next_story_chapter': \['AI 续写', 'AI continuation'\]/)
})

test('ChatArea presents premise image workflows created by direct generation requests', () => {
  const chatArea = readFileSync(new URL('../components/ChatArea.jsx', import.meta.url), 'utf8')
  const presentation = readFileSync(new URL('./chatAreaPresentation.js', import.meta.url), 'utf8')
  const messages = readFileSync(new URL('../i18n/messages/chat.js', import.meta.url), 'utf8')
  assert.match(presentation, /premise_asset_generation: 'chat\.workflow\.kind\.premise_asset_generation'/)
  assert.match(chatArea, /generate_premise_asset: 'chat\.workflow\.step\.generate_premise_asset'/)
  assert.match(messages, /'chat\.workflow\.kind\.premise_asset_generation': \['设定项图片生成', 'Premise-image generation'\]/)
  assert.match(messages, /'chat\.workflow\.step\.generate_premise_asset': \['生成设定项图片', 'Generate premise image'\]/)
})
