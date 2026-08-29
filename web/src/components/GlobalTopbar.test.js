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

test('topbar, persistent sidebar and group tabs preserve the existing routes', () => {
  const topbarSource = readFileSync(new URL('./GlobalTopbar.jsx', import.meta.url), 'utf8')
  const sidebarSource = readFileSync(new URL('./GlobalSidebar.jsx', import.meta.url), 'utf8')
  const shellStyles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')
  const tabsSource = readFileSync(new URL('./WorkspaceGroupTabs.jsx', import.meta.url), 'utf8')
  assert.match(topbarSource, /workspaceRoute\(projectUuid, section\.route, location\.search\)/)
  assert.match(topbarSource, /projects\.recent_used/)
  assert.match(sidebarSource, /lumi\.globalSidebarCollapsed/)
  assert.match(sidebarSource, /global-sidebar__logo-button[\s\S]*onClick=\{onToggleCollapsed\}/)
  assert.match(sidebarSource, /recentProjects\.slice\(0, 6\)/)
  assert.match(sidebarSource, /icon=\{House\}/)
  assert.match(sidebarSource, /global-sidebar__search[\s\S]*aria-disabled="true"/)
  assert.match(sidebarSource, /to: '\/settings\/account'/)
  assert.match(sidebarSource, /to: '\/settings\/account#language'/)
  assert.match(sidebarSource, /to: '\/settings\/providers'/)
  assert.match(sidebarSource, /to: '\/settings\/llm-logs'/)
  assert.match(sidebarSource, /to: '\/about'/)
  assert.match(sidebarSource, /role="menu"/)
  assert.match(shellStyles, /grid-template-columns: \$global-sidebar-width minmax\(0, 1fr\)/)
  assert.match(shellStyles, /@media \(max-width: 760px\)[\s\S]*?\.global-sidebar[\s\S]*?transform: translateX\(-100%\)/)
  assert.match(tabsSource, /workspaceRoute\(projectUuid, item\.route, location\.search\)/)
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
