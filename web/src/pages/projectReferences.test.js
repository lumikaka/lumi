import assert from 'node:assert/strict'
import test from 'node:test'

import {
  isCanonicalUUIDv7,
	isPlainProjectReferenceClick,
  parseProjectReference,
  projectChatControlKey,
  resolveProjectReference,
} from './projectReferences.js'

const projectUuid = '01900000-0000-7000-8000-000000000001'
const chapterUuid = '01900000-0000-7000-8000-000000000002'
const sectionUuid = '01900000-0000-7000-8000-000000000003'
const assetUuid = '01900000-0000-7000-8000-000000000004'
const workflowUuid = '01900000-0000-7000-8000-000000000005'
const letteredUuid = '0190abcd-efab-7abc-8abc-abcdefabcdef'

test('project reference parser accepts exactly the eight registered resource shapes', () => {
  const references = [
    ['@project/story-profile', { kind: 'story-profile' }],
    ['@project/premise', { kind: 'premise' }],
    [`@project/premise/assets/${assetUuid}`, { kind: 'premise-asset', assetUuid }],
    [`@project/workflows/${workflowUuid}`, { kind: 'workflow', workflowUuid }],
    [`@project/chapters/${chapterUuid}`, { kind: 'chapter', chapterUuid }],
    [`@project/chapters/${chapterUuid}/body`, { kind: 'chapter-body', chapterUuid }],
    [`@project/chapters/${chapterUuid}/sections/${sectionUuid}`, { kind: 'section', chapterUuid, sectionUuid }],
    ['@project/exports', { kind: 'exports' }],
  ]
  references.forEach(([value, expected]) => assert.deepEqual(parseProjectReference(value), expected))
})

test('project reference parser rejects traversal, encoding, noncanonical UUIDs, and unknown paths', () => {
  const invalid = [
    '@project/Story-profile',
    '@project/story-profile/',
    '@project//premise',
    '@project/../exports',
    '@project/%2e%2e/exports',
    '@project/premise\\assets\\01900000-0000-7000-8000-000000000004',
    '@project/premise?premise_asset_uuid=x',
    '@project/premise#asset',
	`@project/chapters/${letteredUuid.toUpperCase()}`,
	`@project/workflows/${letteredUuid.toUpperCase()}`,
    '@project/chapters/550e8400-e29b-41d4-a716-446655440000',
    `@project/chapters/${chapterUuid}/sections/550e8400-e29b-41d4-a716-446655440000`,
    '@project/assets/01900000-0000-7000-8000-000000000004',
    '@project/workflows/550e8400-e29b-41d4-a716-446655440000',
    `@project/workflows/${workflowUuid}/../events`,
    `@project/chapters/${chapterUuid}/body\u0000`,
  ]
  invalid.forEach((value) => assert.equal(parseProjectReference(value), null, value))
  assert.equal(isCanonicalUUIDv7(projectUuid), true)
	assert.equal(isCanonicalUUIDv7(letteredUuid.toUpperCase()), false)
})

test('project references keep cross-route chat context and an explicit workspace-mode override', () => {
  const sourceSearch = `?chat_thread_uuid=${chapterUuid}&workflow_uuid=${sectionUuid}&workspace_mode=expert&workspace_tab=comic&section_uuid=old&premise_asset_uuid=old&chat_new=1&chat_reference_uuid=old&unrelated=drop`
  const preserved = `?chat_thread_uuid=${chapterUuid}&workflow_uuid=${sectionUuid}&workspace_mode=expert`
  const cases = [
    ['@project/story-profile', `/projects/${projectUuid}/story${preserved}`],
    ['@project/premise', `/projects/${projectUuid}/premise${preserved}`],
    [`@project/premise/assets/${assetUuid}`, `/projects/${projectUuid}/premise/assets/${assetUuid}${preserved}`],
    [`@project/workflows/${workflowUuid}`, `/projects/${projectUuid}?chat_thread_uuid=${chapterUuid}&workflow_uuid=${workflowUuid}&workspace_mode=expert`],
    [`@project/chapters/${chapterUuid}`, `/projects/${projectUuid}/chapters/${chapterUuid}${preserved}`],
    [`@project/chapters/${chapterUuid}/body`, `/projects/${projectUuid}/chapters/${chapterUuid}${preserved}&workspace_tab=body`],
    [`@project/chapters/${chapterUuid}/sections/${sectionUuid}`, `/projects/${projectUuid}/chapters/${chapterUuid}/sections/${sectionUuid}${preserved}`],
    ['@project/exports', `/projects/${projectUuid}/exports${preserved}`],
  ]
  cases.forEach(([reference, expected]) => {
    const target = resolveProjectReference(reference, { projectUuid, search: sourceSearch })
    assert.equal(`${target.pathname}${target.search}`, expected)
  })
})

test('project reference resolver rejects invalid current context and forged parsed values', () => {
  assert.equal(resolveProjectReference('@project/premise', { projectUuid: 'not-a-project' }), null)
  assert.equal(resolveProjectReference({ kind: 'chapter', chapterUuid: '../outside' }, { projectUuid }), null)
  assert.equal(resolveProjectReference({ kind: 'workflow', workflowUuid: '../outside' }, { projectUuid }), null)
  assert.equal(resolveProjectReference('@project/unknown', { projectUuid }), null)
})

test('chat control key ignores resource navigation query changes', () => {
  const first = projectChatControlKey(`?chat_thread_uuid=${chapterUuid}&workspace_tab=body`)
  const second = projectChatControlKey(`?section_uuid=${sectionUuid}&chat_thread_uuid=${chapterUuid}`)
  assert.equal(first, second)
  assert.notEqual(first, projectChatControlKey(`?chat_thread_uuid=${assetUuid}`))
})

test('only an unmodified primary click closes the compact chat overlay', () => {
	assert.equal(isPlainProjectReferenceClick({ button: 0 }), true)
	assert.equal(isPlainProjectReferenceClick({ button: 0, metaKey: true }), false)
	assert.equal(isPlainProjectReferenceClick({ button: 0, ctrlKey: true }), false)
	assert.equal(isPlainProjectReferenceClick({ button: 0, shiftKey: true }), false)
	assert.equal(isPlainProjectReferenceClick({ button: 0, altKey: true }), false)
	assert.equal(isPlainProjectReferenceClick({ button: 1 }), false)
})
