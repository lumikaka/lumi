import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

test('account and settings resources live in the sidebar footer', () => {
  const topbarSource = readFileSync(new URL('./GlobalTopbar.jsx', import.meta.url), 'utf8')
  const sidebarSource = readFileSync(new URL('./AppSidebar.jsx', import.meta.url), 'utf8')
  const shellStyles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')

  assert.doesNotMatch(topbarSource, /AccountMenu/)
  assert.match(sidebarSource, /to: '\/settings\/account'/)
  assert.match(sidebarSource, /settings\.llm_management'[\s\S]*to: '\/settings\/providers'/)
  assert.match(sidebarSource, /to: '\/settings\/llm-logs'/)
  assert.match(shellStyles, /\.app-sidebar__account[\s\S]*\[aria-expanded="true"\]:hover/)
})

test('local account routes do not depend on a users resource', () => {
  const appSource = readFileSync(new URL('../App.jsx', import.meta.url), 'utf8')
  const settingsSource = readFileSync(new URL('../pages/AccountSettingsPage.jsx', import.meta.url), 'utf8')
  const logsSource = readFileSync(new URL('../pages/LLMLogsPage.jsx', import.meta.url), 'utf8')

  assert.match(appSource, /path="\/settings\/account"/)
  assert.match(appSource, /path="\/settings\/llm-logs"/)
  assert.match(settingsSource, /setLocale\(selectedLocale\)/)
  assert.match(logsSource, /listRecentProjects/)
  assert.match(logsSource, /project_uuid/)
  assert.match(logsSource, /ProjectLLMLogsPanel/)
  assert.doesNotMatch(`${settingsSource}\n${logsSource}`, /\/api\/v1\/users/)
})

test('sidebar exposes search, global new conversation, project actions, and thread actions', () => {
  const source = readFileSync(new URL('./AppSidebar.jsx', import.meta.url), 'utf8')
  const rows = readFileSync(new URL('./SidebarRows.jsx', import.meta.url), 'utf8')
  const preferences = readFileSync(new URL('./sidebarPreferences.js', import.meta.url), 'utf8')
  const styles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')

  assert.match(source, /type="search"/)
  assert.match(source, /searchOpen \? \(/)
  assert.match(source, /sidebar\.new_conversation/)
  assert.match(source, /newConversationPath\(projects, activeProjectUuid\)/)
  assert.match(source, /projectAddIcon/)
  assert.match(source, /moreIcon/)
  assert.match(source, /archiveChatThread/)
  assert.match(rows, /aria-pressed=\{pinned\}/)
  assert.match(preferences, /in_progress[\s\S]*read[\s\S]*unread/)
  assert.match(styles, /button\[aria-pressed="true"\]:hover/)
})

test('project more menu exposes the four product-level actions only', () => {
  const source = readFileSync(new URL('./AppSidebar.jsx', import.meta.url), 'utf8')
  assert.match(source, /projects\.project_settings/)
  assert.match(source, /sidebar\.project\.open_folder/)
  assert.match(source, /sidebar\.project\.(?:un)?pin/)
  assert.match(source, /sidebar\.project\.archive/)
  assert.match(source, /openRecentProjectFolder\(project\.uuid\)/)
  assert.doesNotMatch(source, /PROJECT_DETAIL_ITEMS/)
})

test('sidebar uses Figma folder assets and a fixed right status slot', () => {
  const source = readFileSync(new URL('./AppSidebar.jsx', import.meta.url), 'utf8')
  const rows = readFileSync(new URL('./SidebarRows.jsx', import.meta.url), 'utf8')
  const styles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')
  const messages = readFileSync(new URL('../i18n/messages/common.js', import.meta.url), 'utf8')

  assert.match(source, /folderIcon from '\.\.\/assets\/figma\/workspace\/folder\.svg'/)
  assert.doesNotMatch(source, /FolderKanban|FolderOpen|<svg|<path/)
  assert.match(source, /<FigmaIcon src=\{folderIcon\} size=\{16\}/)
  assert.doesNotMatch(`${source}\n${rows}`, /figma\/workspace\/[^'"\n]+\.png/)
  assert.match(rows, /if \(status === 'read'\).*status-slot/s)
  assert.match(styles, /\.app-sidebar-thread__status-slot[\s\S]*right: 12px/)
  assert.match(rows, /thread-in-progress\.svg/)
  assert.match(rows, /thread-unread\.svg/)
  assert.match(rows, /<FigmaIcon className=\{`app-sidebar-thread__status/)
  assert.match(styles, /\.app-sidebar-thread__status--in_progress[\s\S]*animation: app-sidebar-thread-spin/)
  assert.match(styles, /\.app-sidebar-project__threads[\s\S]*?width: 100%[\s\S]*?max-width: 100%/)
  assert.match(styles, /\.app-sidebar-thread__main[\s\S]*?width: 0[\s\S]*?overflow: hidden/)
  assert.match(messages, /'sidebar\.threads\.expand': \['展开', 'Show more'\]/)
  assert.doesNotMatch(messages, /展开其余/)
  assert.match(styles, /\.app-sidebar__body\s+padding: 0 16px/)
  assert.match(styles, /\.app-sidebar-project__main[\s\S]*?> \.figma-icon\s+flex: 0 0 16px/)
})

test('sidebar account and provider copy follow available local contracts', () => {
  const source = readFileSync(new URL('./AppSidebar.jsx', import.meta.url), 'utf8')
  const rows = readFileSync(new URL('./SidebarRows.jsx', import.meta.url), 'utf8')

  assert.match(source, /queryKey: \['providers'\]/)
  assert.match(rows, /state === 'ready'/)
  assert.match(rows, /settings\.local_account/)
  assert.match(source, /settings\.local_account/)
  assert.doesNotMatch(`${source}\n${rows}`, /settings\.account\.unregistered/)
  assert.doesNotMatch(`${source}\n${rows}`, /@zettlab\.com|masked.*email|\/api\/v1\/users/)
})
