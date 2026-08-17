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

test('persistent sidebar owns projects, conversations, and project detail navigation', () => {
  const topbarSource = readFileSync(new URL('./GlobalTopbar.jsx', import.meta.url), 'utf8')
  const sidebarSource = readFileSync(new URL('./AppSidebar.jsx', import.meta.url), 'utf8')
  const sidebarNavigationSource = readFileSync(new URL('./sidebarNavigation.js', import.meta.url), 'utf8')
  const shellStyles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')
  const tabsSource = readFileSync(new URL('./WorkspaceGroupTabs.jsx', import.meta.url), 'utf8')
  assert.doesNotMatch(topbarSource, /AccountMenu|project-menu-drawer|WORKSPACE_SECTIONS/)
  assert.match(sidebarSource, /listRecentProjects/)
  assert.match(sidebarSource, /listChatThreads\(project\.uuid/)
  assert.match(sidebarSource, /newConversationPath\(projects, activeProjectUuid\)/)
  assert.match(sidebarNavigationSource, /chat_scope=project&chat_new=1/)
  assert.match(sidebarSource, /projects\.project_settings/)
  assert.match(sidebarSource, /sidebar\.project\.open_folder/)
  assert.match(sidebarSource, /sidebar\.project\.archive/)
  assert.match(sidebarSource, /settings\.account_and_settings/)
  assert.match(shellStyles, /\.app-frame[\s\S]*grid-template-columns: \$sidebar-width/)
  assert.match(shellStyles, /\.app-sidebar-project__row[\s\S]*\.app-sidebar-project__actions[\s\S]*opacity: 1/)
  assert.match(shellStyles, /\.app-sidebar-thread[\s\S]*\.app-sidebar-thread__actions[\s\S]*opacity: 1/)
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
