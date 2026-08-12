import assert from 'node:assert/strict'
import test from 'node:test'

import { cancelTask, checkProvider, createChapterBatch, createChapterGeneration, createComicStoryboardGeneration, createStoryProfileGeneration, createStoryProfileReconstruction, getProjectLLMLog, getProjectModelSettings, listProjectLLMLogs, listTaskEvents, retryTask, updateProjectModelSettings, updateSiteSettings } from './ai.js'

function envelope(data) {
  return new Response(JSON.stringify({ success: true, data }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

test('provider configuration uses site settings and connection-check resources', async () => {
  const calls = []
  const original = global.fetch
  global.fetch = async (url, options = {}) => { calls.push([url, options]); return envelope({ uuid: 'provider-uuid', has_secret: true }) }
  try {
    await updateSiteSettings({ 'ai_providers.openai_compatible.api_key': 'secret' })
    await checkProvider('provider-uuid')
  } finally { global.fetch = original }
  assert.equal(calls[0][0], '/api/v1/site-settings')
  assert.equal(calls[0][1].method, 'PATCH')
	assert.deepEqual(JSON.parse(calls[0][1].body), { settings: { 'ai_providers.openai_compatible.api_key': 'secret' } })
  assert.equal(calls[1][0], '/api/v1/providers/provider-uuid/connection-checks')
})

test('story generation, cancellation, retry and event cursor use public UUID resources', async () => {
  const calls = []
  const original = global.fetch
  global.fetch = async (url, options = {}) => { calls.push([url, options]); return envelope({ uuid: 'task-uuid' }) }
  try {
    await createChapterGeneration('project-uuid', 'chapter-uuid', { prompt: 'write', idempotency_key: 'key' })
    await cancelTask('project-uuid', 'task-uuid')
    await retryTask('project-uuid', 'task-uuid')
    await listTaskEvents('project-uuid', 'task-uuid', { after: 12, limit: 25 })
  } finally { global.fetch = original }
  assert.equal(calls[0][0], '/api/v1/projects/project-uuid/chapters/chapter-uuid/generations')
  assert.equal(Object.hasOwn(JSON.parse(calls[0][1].body), 'provider_uuid'), false)
  assert.equal(calls[1][0], '/api/v1/projects/project-uuid/tasks/task-uuid/cancellations')
  assert.equal(calls[2][0], '/api/v1/projects/project-uuid/tasks/task-uuid/retries')
  assert.equal(calls[3][0], '/api/v1/projects/project-uuid/tasks/task-uuid/events?after=12&limit=25')
  assert.ok(calls.slice(0, 3).every(([, options]) => options.method === 'POST'))
})

test('project LLM logs use project-scoped page pagination', async () => {
  const calls = []
  const original = global.fetch
  global.fetch = async (url, options = {}) => { calls.push([url, options]); return envelope({ items: [], pagination: {} }) }
  try {
    await listProjectLLMLogs('project uuid', { page: 2, perPage: 12, scope: 'premise', providerType: 'openai', model: 'gpt-test', scenario: 'project_chat', status: 'completed', requestType: 'text', keyword: 'hello' })
    await getProjectLLMLog('project uuid', 'log uuid')
  } finally { global.fetch = original }
  assert.equal(calls[0][0], '/api/v1/projects/project%20uuid/llm-logs?page=2&per_page=12&scope=premise&provider_type=openai&model=gpt-test&scenario=project_chat&status=completed&request_type=text&keyword=hello')
  assert.equal(calls[0][1].method, undefined)
  assert.equal(calls[1][0], '/api/v1/projects/project%20uuid/llm-logs/log%20uuid')
  assert.equal(calls[1][1].method, undefined)
})

test('project model settings use a revisioned resource and null clears inheritance overrides', async () => {
  const calls = []
  const original = global.fetch
  global.fetch = async (url, options = {}) => { calls.push([url, options]); return envelope({ revision: 4 }) }
  try {
    await getProjectModelSettings('project uuid')
    await updateProjectModelSettings('project uuid', 3, { story_text: null })
  } finally { global.fetch = original }
  assert.equal(calls[0][0], '/api/v1/projects/project%20uuid/model-settings')
  assert.equal(calls[1][0], '/api/v1/projects/project%20uuid/model-settings')
  assert.equal(calls[1][1].method, 'PATCH')
  assert.deepEqual(JSON.parse(calls[1][1].body), { expected_revision: 3, overrides: { story_text: null } })
})

test('planning steps use project-scoped resource endpoints', async () => {
  const calls = []
  const original = global.fetch
  global.fetch = async (url, options = {}) => { calls.push([url, options]); return envelope({ uuid: 'task-uuid' }) }
  try {
    await createStoryProfileGeneration('project', { prompt: 'idea', idempotency_key: 'profile' })
    await createStoryProfileReconstruction('project', { idempotency_key: 'reconstruct' })
    await createChapterBatch('project', { prompt: 'plan', chapter_count: 2, idempotency_key: 'batch' })
    await createComicStoryboardGeneration('project', 'chapter', { idempotency_key: 'comic' })
  } finally { global.fetch = original }
  assert.deepEqual(calls.map(([url]) => url), [
    '/api/v1/projects/project/story-profile/generations',
    '/api/v1/projects/project/story-profile/reconstructions',
    '/api/v1/projects/project/chapter-batches',
    '/api/v1/projects/project/chapters/chapter/comic-storyboard-generations',
  ])
  assert.ok(calls.every(([, options]) => options.method === 'POST'))
})
