import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

import { projectRowActions, projectRowPrimaryAction } from './projectIndexState.js'

test('unavailable local projects stay recoverable without an open action', () => {
	assert.deepEqual(projectRowActions({ open: false, available: false }), ['relocate', 'forget'])
})

test('active and recent project rows expose the correct local actions', () => {
	assert.deepEqual(projectRowActions({ open: true, available: true }), ['enter', 'reveal', 'forget'])
	assert.deepEqual(projectRowActions({ open: false, available: true, status: 'recent' }), ['enter', 'reveal', 'relocate', 'forget'])
})

test('project rows use the available action that enters the workspace', () => {
	assert.equal(projectRowPrimaryAction({ open: true, available: true }), 'enter')
	assert.equal(projectRowPrimaryAction({ open: false, available: true }), 'enter')
	assert.equal(projectRowPrimaryAction({ open: false, available: false }), null)
})

test('project page presents creation, open, relocation and forget dialogs', () => {
  const source = readFileSync(new URL('./HomePage.jsx', import.meta.url), 'utf8')
  for (const dialog of ["dialog === 'create'", "dialog === 'open'", "dialog === 'relocate'", "dialog === 'forget'"]) {
    assert.match(source, new RegExp(dialog.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))
  }
  assert.match(source, /projects\.forget\.hint/)
  assert.match(source, /queryKey: \['project-defaults'\], queryFn: getProjectDefaults/)
  assert.match(source, /if \(!parentPathDirty && defaultProjectParentPath\) setParentPath\(defaultProjectParentPath\)/)
  assert.match(source, /setParentPath\(defaultProjectParentPath\)[\s\S]*setParentPathDirty\(false\)/)
  assert.doesNotMatch(source, /disabled=\{[^}]*projectDefaultsQuery\.isError/)
  assert.doesNotMatch(source, /~\/Documents\/Lumi|\/Users\/me\//)
  assert.match(source, /projects\.dialog\.open\.title/)
  assert.match(source, /selectDirectoryMutation\.mutate\(existingPath\)/)
  assert.match(source, /projects\.open\.choose_folder/)
  assert.match(source, /revealDirectoryMutation\.mutate\(project\.root_path\)/)
  assert.match(source, /projects\.action\.reveal/)
  assert.doesNotMatch(source, /安全关闭|closeCurrentProject/)
})

test('new project dialogs prefer YOLO creation every time they open', () => {
  const source = readFileSync(new URL('./HomePage.jsx', import.meta.url), 'utf8')
  const styles = readFileSync(new URL('../styles/projects.sass', import.meta.url), 'utf8')
  assert.match(source, /const \[createMode, setCreateMode\] = useState\('yolo'\)/)
  assert.match(source, /const openCreateDialog = \(\) => \{ setActionError\(null\); resetCreationFields\(\); setCreateMode\('yolo'\); setDialog\('create'\) \}/)
  assert.match(source, /<Modal className="project-create-dialog" title=\{t\('projects\.dialog\.create\.title'\)\}/)
  assert.match(source, /<LumiDialog className=\{className\}/)
  assert.match(styles, /\.lumi-dialog\.project-create-dialog\n\s+width: min\(820px, calc\(100vw - 32px\)\)/)
  assert.match(source, /<PictureBookProfileFields value=\{pictureBookDraft\}/)
  assert.match(source, /await preflightImageGeneration\(pictureBook\)[\s\S]*await createProject/)
})

test('YOLO creation prioritizes its name and story idea and validates on submit', () => {
  const source = readFileSync(new URL('./HomePage.jsx', import.meta.url), 'utf8')
  const priorityFields = source.match(/<div className="project-create-priority-fields">([\s\S]*?)<\/div>\n          <div className="project-dialog-field">/)
  assert.ok(priorityFields)
  assert.match(priorityFields[1], /projects\.field\.name/)
  assert.match(priorityFields[1], /projects\.field\.story_idea/)
  assert.match(source, /className="project-dialog-form" noValidate onSubmit=\{submitCreateForm\}/)
  assert.match(source, /const errors = projectCreationErrors/)
  assert.match(source, /<button type="submit" disabled=\{pending\}>/)
  assert.doesNotMatch(source, /disabled=\{pending \|\| projectDefaultsQuery\.isPending/)
})

test('both creation modes use a collapsed editable overall style initialized from server defaults', () => {
  const source = readFileSync(new URL('./HomePage.jsx', import.meta.url), 'utf8')
  assert.match(source, /default_overall_styles/)
  assert.match(source, /<details ref=\{overallStyleDetailsRef\} className="project-overall-style">/)
  assert.doesNotMatch(source, /<details[^>]*project-overall-style[^>]*\sopen(?:=|\s|>)/)
  assert.match(source, /projects\.field\.overall_style_default/)
  assert.match(source, /projects\.field\.overall_style_custom/)
  assert.match(source, /setOverallStyleDirty\(true\)/)
  assert.match(source, /setOverallStyle\(defaultOverallStyle\); setOverallStyleDirty\(false\)/)
  assert.match(source, /createProject\(\{ name, parentPath, generationLanguage, pictureBook, overallStyle \}\)/)
  assert.match(source, /createMutation\.mutate\(\{ name, parentPath, generationLanguage, pictureBook, overallStyle \}\)/)
})

test('home conversation composer preserves input, retry identity, and public navigation targets', () => {
  const source = readFileSync(new URL('./HomePage.jsx', import.meta.url), 'utf8')
  const api = readFileSync(new URL('../api/projects.js', import.meta.url), 'utf8')
  assert.match(source, /<textarea id="project-creation-input"[^>]*value=\{creationInput\}/)
  assert.match(source, /if \(!creationInput\.trim\(\)\) return/)
  assert.match(source, /creationMutation\.mutate\(\{ inputText: creationInput, idempotencyKey: checkpoint\.idempotencyKey \}\)/)
  assert.match(source, /creationCheckpoint\?\.inputText === creationInput/)
  assert.match(source, /window\.sessionStorage\.setItem\(CREATION_CHECKPOINT_KEY/)
  assert.match(source, /disabled=\{creationPending \|\| !creationInput\.trim\(\)\}/)
  assert.match(source, /const creationPending = creationMutation\.isPending \|\| creationRetryMutation\.isPending/)
  assert.match(source, /creationSession\?\.status === 'failed'/)
  assert.match(source, /creationError \? <div className="project-creation-composer__failure"/)
  assert.match(source, /disabled=\{creationPending \|\| !creationCheckpoint\} onClick=\{retryCreation\}/)
  assert.match(source, /navigate\(`\/projects\/\$\{encodeURIComponent\(session\.project_uuid\)\}\?chat_thread_uuid=\$\{encodeURIComponent\(session\.thread_uuid\)\}`\)/)
  assert.match(source, /retryProjectCreationSession/)
  assert.match(api, /\/api\/v1\/project-creation-sessions/)
  assert.match(api, /input_text: inputText, idempotency_key: idempotencyKey/)
  assert.doesNotMatch(source, /projects\.conversation[\s\S]{0,600}parentPath/)
})
