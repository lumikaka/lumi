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

test('project workspace owns realtime presence and query synchronization', () => {
  const layout = readFileSync(new URL('./ProjectWorkspaceLayout.jsx', import.meta.url), 'utf8')
  const realtime = readFileSync(new URL('../realtime/useProjectRealtimeSync.js', import.meta.url), 'utf8')
  assert.match(layout, /useProjectRealtimeSync\(projectUuid\)/)
  assert.match(realtime, /channel\.onMessage\(handleMessage\)/)
  assert.match(realtime, /channel\.on\('phx_joined', resyncProject\)/)
  assert.match(realtime, /channel\.join\(\)/)
  assert.match(realtime, /channel\.leave\(\)/)
  assert.match(realtime, /window\.addEventListener\('focus'/)
})

test('system lifecycle events refresh open and recent project state', () => {
  const source = readFileSync(new URL('../realtime/useSiteSettingsRealtime.js', import.meta.url), 'utf8')
	assert.match(source, /open_project:changed/)
	assert.match(source, /projectQueryKeys\.openProjects\(\)/)
	assert.match(source, /projectQueryKeys\.recent\(\)/)
	assert.match(source, /isProjectBusinessQuery\(query, payload\.project_uuid\)/)
	assert.doesNotMatch(source, /project-activation/)
})
