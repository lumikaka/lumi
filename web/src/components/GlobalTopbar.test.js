import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

import { WORKSPACE_GROUP_ITEMS, WORKSPACE_SECTIONS } from './workspaceNavigation.js'

test('project navigation exposes three primary sections and grouped tools', () => {
  assert.deepEqual(WORKSPACE_SECTIONS.map((item) => item.key), ['overview', 'premise', 'chapters'])
  assert.equal(WORKSPACE_SECTIONS[0].route, 'overview/summary')
  assert.deepEqual(WORKSPACE_GROUP_ITEMS.overview.map((item) => item.key), ['summary', 'profile', 'prompts', 'llm-logs', 'exports'])
  assert.deepEqual(WORKSPACE_GROUP_ITEMS.premise.map((item) => item.key), ['premise', 'assets'])
  assert.deepEqual(WORKSPACE_GROUP_ITEMS.chapters.map((item) => item.key), ['chapters', 'comic', 'trash'])
})

test('topbar and group tabs use shared route builder with current search', () => {
  const topbarSource = readFileSync(new URL('./GlobalTopbar.jsx', import.meta.url), 'utf8')
  const shellStyles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')
  const tabsSource = readFileSync(new URL('./WorkspaceGroupTabs.jsx', import.meta.url), 'utf8')
  assert.match(topbarSource, /workspaceRoute\(projectUuid, section\.route, location\.search\)/)
  assert.match(topbarSource, /projects\.recent_used/)
  assert.match(topbarSource, /project-menu-drawer__project-copy/)
  assert.match(shellStyles, /\.project-menu-drawer__project-link[\s\S]*border: 0[\s\S]*background: transparent[\s\S]*text-align: left/)
  assert.match(tabsSource, /workspaceRoute\(projectUuid, item\.route, location\.search\)/)
  assert.doesNotMatch(topbarSource, /workspace-sidebar/)
})

test('overview routes are canonical and legacy story links keep redirect compatibility', () => {
  const workspaceSource = readFileSync(new URL('../pages/StoryWorkspacePage.jsx', import.meta.url), 'utf8')
  for (const route of ['overview/summary', 'overview/profile', 'overview/prompts', 'overview/llm-logs', 'overview/exports']) {
    assert.match(workspaceSource, new RegExp(`path="${route.replace('/', '\\/')}"`))
  }
  assert.match(workspaceSource, /path="story" element={<RouteRedirect to={`\$\{base\}\/overview\/profile`} \/>}/)
  assert.match(workspaceSource, /path="prompts" element={<RouteRedirect to={`\$\{base\}\/overview\/prompts`} \/>}/)
  assert.match(workspaceSource, /to={{ pathname: to, search: location\.search, hash: location\.hash }}/)
  assert.match(workspaceSource, /path="overview\/llm-logs"/)
})
