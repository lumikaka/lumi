import assert from 'node:assert/strict'
import test from 'node:test'

import { mergeSidebarProjectOrder, orderSidebarProjects } from './sidebarProjectOrder.js'

const projects = (...uuids) => uuids.map((uuid) => ({ uuid, name: uuid.toUpperCase() }))

test('sidebar project order does not follow recent-project reordering after activation', () => {
  const initialOrder = mergeSidebarProjectOrder([], projects('a', 'b', 'c'))
  const orderAfterActivation = mergeSidebarProjectOrder(initialOrder, projects('b', 'a', 'c'))

  assert.strictEqual(orderAfterActivation, initialOrder)
  assert.deepEqual(orderSidebarProjects(projects('b', 'a', 'c'), orderAfterActivation).map((project) => project.uuid), ['a', 'b', 'c'])
})

test('sidebar project order removes missing projects and appends new projects', () => {
  const nextOrder = mergeSidebarProjectOrder(['a', 'b', 'c'], projects('d', 'c', 'b'))

  assert.deepEqual(nextOrder, ['b', 'c', 'd'])
})

test('sidebar project order survives the empty loading state', () => {
  const previousOrder = ['a', 'b']

  assert.strictEqual(mergeSidebarProjectOrder(previousOrder, []), previousOrder)
  assert.deepEqual(orderSidebarProjects([], previousOrder), [])
})
