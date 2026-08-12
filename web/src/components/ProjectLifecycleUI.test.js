import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

test('project routes activate before rendering their workspace', () => {
  const app = readFileSync(new URL('../App.jsx', import.meta.url), 'utf8')
  const gate = readFileSync(new URL('./ProjectActivationGate.jsx', import.meta.url), 'utf8')
  assert.match(app, /<ProjectActivationGate><StoryWorkspacePage \/><\/ProjectActivationGate>/)
	assert.match(gate, /ensureProjectOpen\(projectUuid\)/)
  assert.match(gate, /projects\.loading\.entering/)
  assert.match(gate, /projects\.all/)
})

test('project workspace owns a realtime presence lease', () => {
  const layout = readFileSync(new URL('./ProjectWorkspaceLayout.jsx', import.meta.url), 'utf8')
  const presence = readFileSync(new URL('../realtime/useProjectPresence.js', import.meta.url), 'utf8')
  assert.match(layout, /useProjectPresence\(projectUuid\)/)
  assert.match(presence, /channel\.join\(\)/)
  assert.match(presence, /channel\.leave\(\)/)
})

test('system lifecycle events refresh open and recent project state', () => {
  const source = readFileSync(new URL('../realtime/useSiteSettingsRealtime.js', import.meta.url), 'utf8')
	assert.match(source, /open_project:changed/)
	assert.match(source, /projectQueryKeys\.openProjects\(\)/)
	assert.match(source, /projectQueryKeys\.recent\(\)/)
	assert.match(source, /isProjectBusinessQuery\(query, payload\.project_uuid\)/)
	assert.doesNotMatch(source, /project-activation/)
})
