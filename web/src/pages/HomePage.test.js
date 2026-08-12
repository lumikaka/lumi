import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

import { projectRowActions, projectRowPrimaryAction } from './projectIndexState.js'

test('unavailable local projects stay recoverable without an open action', () => {
	assert.deepEqual(projectRowActions({ open: false, available: false }), ['relocate', 'forget'])
})

test('active and recent project rows expose the correct local actions', () => {
	assert.deepEqual(projectRowActions({ open: true, available: true }), ['enter', 'forget'])
	assert.deepEqual(projectRowActions({ open: false, available: true, status: 'recent' }), ['enter', 'relocate', 'forget'])
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
  assert.match(source, /projectDefaultsQuery\.isError/)
  assert.doesNotMatch(source, /~\/Documents\/Lumi|\/Users\/me\//)
  assert.match(source, /projects\.dialog\.open\.title/)
  assert.match(source, /selectDirectoryMutation\.mutate\(existingPath\)/)
  assert.match(source, /projects\.open\.choose_folder/)
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
