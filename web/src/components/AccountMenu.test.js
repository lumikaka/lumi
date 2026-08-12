import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

test('the person icon is the final topbar action and opens local account resources', () => {
  const topbarSource = readFileSync(new URL('./GlobalTopbar.jsx', import.meta.url), 'utf8')
  const menuSource = readFileSync(new URL('./AccountMenu.jsx', import.meta.url), 'utf8')
  const shellStyles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')

  assert.match(topbarSource, /\{actions\}<AccountMenu \/>/)
  assert.match(menuSource, /className="account-menu__icon"/)
  assert.match(menuSource, /to="\/settings\/account"/)
  assert.match(menuSource, /to="\/settings\/providers"[\s\S]*settings\.llm_management/)
  assert.match(menuSource, /to="\/settings\/llm-logs"/)
  assert.match(shellStyles, /\.account-menu__trigger[\s\S]*\[aria-expanded="true"\]:hover/)
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
