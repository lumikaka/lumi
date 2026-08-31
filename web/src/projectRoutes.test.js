import assert from 'node:assert/strict'
import test from 'node:test'

import {
  canonicalProjectLocation,
  projectModeOverride,
  projectPremiseAssetRoute,
  projectRoute,
  projectRouteRequiresExpert,
  projectSectionRoute,
  withoutProjectModeOverride,
} from './projectRoutes.js'

const projectUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff810'
const chapterUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff811'
const sectionUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff812'
const assetUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff813'

test('canonical project builders identify resources without encoding the workspace mode', () => {
  assert.deepEqual(projectRoute(projectUuid, '', '?chat_thread_uuid=thread'), {
    pathname: `/projects/${projectUuid}`,
    search: '?chat_thread_uuid=thread',
  })
  assert.equal(projectPremiseAssetRoute(projectUuid, assetUuid).pathname, `/projects/${projectUuid}/premise/assets/${assetUuid}`)
  assert.equal(projectSectionRoute(projectUuid, chapterUuid, sectionUuid).pathname, `/projects/${projectUuid}/chapters/${chapterUuid}/sections/${sectionUuid}`)
})

test('legacy simple and expert locations normalize to one resource URL and preserve compatible state', () => {
  const cases = [
    [`/projects/${projectUuid}/simple/home`, '', `/projects/${projectUuid}`, ''],
    [`/projects/${projectUuid}/overview/profile`, '?chat_thread_uuid=thread', `/projects/${projectUuid}/story`, '?chat_thread_uuid=thread'],
    [`/projects/${projectUuid}/simple/settings`, '', `/projects/${projectUuid}/premise`, ''],
    [`/projects/${projectUuid}/simple/settings/${assetUuid}`, '', `/projects/${projectUuid}/premise/assets/${assetUuid}`, ''],
    [`/projects/${projectUuid}/simple/books/${chapterUuid}/pages/${sectionUuid}`, '?chat_new=1', `/projects/${projectUuid}/chapters/${chapterUuid}/sections/${sectionUuid}`, '?chat_new=1'],
    [`/projects/${projectUuid}/simple/books/${chapterUuid}/book`, '', `/projects/${projectUuid}/chapters/${chapterUuid}/preview`, ''],
    [`/projects/${projectUuid}/overview/exports`, '', `/projects/${projectUuid}/exports`, ''],
    [`/projects/${projectUuid}/trash`, '?chat_thread_uuid=thread', `/projects/${projectUuid}/chapters`, '?chat_thread_uuid=thread&state=trashed'],
  ]
  cases.forEach(([pathname, search, expectedPath, expectedSearch]) => {
    const result = canonicalProjectLocation({ projectUuid, pathname, search })
    assert.equal(result.pathname, expectedPath)
    assert.equal(result.search, expectedSearch)
  })
})

test('resource selection moves from expert query state into the canonical path', () => {
  const asset = canonicalProjectLocation({
    projectUuid,
    pathname: `/projects/${projectUuid}/premise`,
    search: `?premise_asset_uuid=${assetUuid}&chat_thread_uuid=thread`,
  })
  assert.deepEqual(asset, {
    pathname: `/projects/${projectUuid}/premise/assets/${assetUuid}`,
    search: '?chat_thread_uuid=thread',
    hash: '',
  })

  const section = canonicalProjectLocation({
    projectUuid,
    pathname: `/projects/${projectUuid}/chapters/${chapterUuid}`,
    search: `?section_uuid=${sectionUuid}&workspace_tab=body`,
    hash: '#editor',
  })
  assert.deepEqual(section, {
    pathname: `/projects/${projectUuid}/chapters/${chapterUuid}/sections/${sectionUuid}`,
    search: '?workspace_tab=body',
    hash: '#editor',
  })
})

test('expert-only routes force an expert surface while shared utilities honor the selected mode', () => {
  for (const route of ['prompts', 'assets', 'threads/thread/trajectory']) {
    assert.equal(projectRouteRequiresExpert(`/projects/${projectUuid}/${route}`, projectUuid), true, route)
  }
  assert.equal(projectRouteRequiresExpert(`/projects/${projectUuid}/llm-logs`, projectUuid), false)
  assert.equal(projectRouteRequiresExpert(`/projects/${projectUuid}/exports`, projectUuid), false)
  assert.equal(projectRouteRequiresExpert(`/projects/${projectUuid}/settings`, projectUuid), false)
  assert.equal(projectRouteRequiresExpert(`/projects/${projectUuid}/story`, projectUuid), false)
  assert.equal(projectModeOverride('?workspace_mode=expert&chat_new=1'), 'expert')
  assert.equal(withoutProjectModeOverride('?workspace_mode=expert&chat_new=1'), '?chat_new=1')
})
