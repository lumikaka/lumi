import assert from 'node:assert/strict'
import test from 'node:test'

import {
  PROJECT_DASHBOARD_MODE_EXPERT,
  PROJECT_DASHBOARD_MODE_SIMPLE,
  normalizeProjectDashboardMode,
  projectDashboardModeStorageKey,
  readProjectDashboardMode,
  writeProjectDashboardMode,
} from './projectDashboardMode.js'

const projectUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff810'

function memoryStorage() {
  const values = new Map()
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    values,
  }
}

test('dashboard mode persists independently for each public project UUID and safely defaults to simple', () => {
  const storage = memoryStorage()
  const secondProjectUuid = '01990c73-4ca2-7aa1-8f4b-0555633ff814'

  assert.equal(normalizeProjectDashboardMode('unexpected'), PROJECT_DASHBOARD_MODE_SIMPLE)
  assert.equal(readProjectDashboardMode(storage, projectUuid), PROJECT_DASHBOARD_MODE_SIMPLE)
  assert.equal(writeProjectDashboardMode(storage, projectUuid, PROJECT_DASHBOARD_MODE_SIMPLE), PROJECT_DASHBOARD_MODE_SIMPLE)
  assert.equal(readProjectDashboardMode(storage, projectUuid), PROJECT_DASHBOARD_MODE_SIMPLE)
  assert.equal(readProjectDashboardMode(storage, secondProjectUuid), PROJECT_DASHBOARD_MODE_SIMPLE)
  assert.equal(writeProjectDashboardMode(storage, secondProjectUuid, PROJECT_DASHBOARD_MODE_EXPERT), PROJECT_DASHBOARD_MODE_EXPERT)
  assert.equal(readProjectDashboardMode(storage, secondProjectUuid), PROJECT_DASHBOARD_MODE_EXPERT)
  assert.equal(storage.values.get(projectDashboardModeStorageKey(projectUuid)), PROJECT_DASHBOARD_MODE_SIMPLE)

  const blockedStorage = {
    getItem() { throw new Error('blocked') },
    setItem() { throw new Error('blocked') },
  }
  assert.equal(readProjectDashboardMode(blockedStorage, projectUuid), PROJECT_DASHBOARD_MODE_SIMPLE)
  assert.equal(writeProjectDashboardMode(blockedStorage, projectUuid, PROJECT_DASHBOARD_MODE_SIMPLE), PROJECT_DASHBOARD_MODE_SIMPLE)
})
