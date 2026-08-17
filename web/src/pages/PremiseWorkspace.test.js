import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./PremiseWorkspace.jsx', import.meta.url), 'utf8')
const supportSource = readFileSync(new URL('./PremiseSupportPanels.jsx', import.meta.url), 'utf8')
const chatSource = readFileSync(new URL('../components/ChatArea.jsx', import.meta.url), 'utf8')
const projectThreadsSource = readFileSync(new URL('./projectThreads.js', import.meta.url), 'utf8')
const layoutSource = readFileSync(new URL('../components/ProjectWorkspaceLayout.jsx', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../styles/workspaces.sass', import.meta.url), 'utf8')
const chatStyles = readFileSync(new URL('../styles/chat.sass', import.meta.url), 'utf8')
const shellStyles = readFileSync(new URL('../styles/shell.sass', import.meta.url), 'utf8')

test('project outputs remove low-frequency tool navigation and expose prompt-based creation', () => {
  assert.doesNotMatch(source, /const tabs = \[/)
  assert.doesNotMatch(source, /premise-toolbar__tabs/)
  assert.match(source, /projects\.canvas\.add_setting_prompt/)
  assert.match(source, /projects\.canvas\.add_scene_prompt/)
  assert.match(source, /projects\.canvas\.add_prop_prompt/)
  assert.match(source, /projects\.canvas\.add_chapter_prompt/)
  assert.match(source, /onCreatePrompt\?\.\(t\(promptKey\)\)/)
  assert.match(source, /projects\.canvas\.import/)
  assert.match(source, /fileInputRef\.current\?\.click\(\)/)
  assert.match(source, /EMPTY_PROMPT_BY_FILTER\[filterType\]/)
  assert.doesNotMatch(source, /setFilterType\(''\).*premise\.assets\.clear_filter/)
  assert.match(source, /output-add-horizontal\.svg/)
  assert.match(source, /output-add-vertical\.svg/)
})

test('project thread lists are shared while premise scenes keep scoped ChatArea routing', () => {
  assert.match(source, /next\.delete\('chat_scope'\)/)
  assert.match(source, /openChatScene\('asset_reference', asset\)/)
  assert.match(source, /<PremiseThreadsPanel/)
  assert.match(source, /<PremisePromptsPanel/)
  assert.match(source, /scope="premise" title=\{t\('premise\.llm_logs\.title'\)\}/)
  assert.match(supportSource, /useProjectThreads\(projectUuid\)/)
  assert.match(projectThreadsSource, /listChatThreads\(projectUuid, \{ page: pageParam, perPage: PROJECT_THREADS_PAGE_SIZE \}\)/)
  assert.match(supportSource, /pagination\?\.total/)
  assert.match(projectThreadsSource, /seen\.has\(thread\.uuid\)/)
  assert.match(supportSource, /fetchNextPage/)
  assert.match(supportSource, /isFetchNextPageError[\s\S]*premise\.history\.retry_more/)
  assert.match(supportSource, /threadsQuery\.refetch\(\)/)
  assert.match(projectThreadsSource, /return \['chat-threads', projectUuid, 'pages'\]/)
  assert.match(chatSource, /useProjectThreads\(projectUuid, expanded\)/)
  assert.doesNotMatch(chatSource, /requestedScope/)
  assert.match(supportSource, /<PromptCatalogEditor projectUuid=\{projectUuid\} groups=\{\['premise', 'premise_style'\]\}/)
  assert.match(chatSource, /scene: requestedScene/)
  assert.match(chatSource, /subject_uuid: requestedSubjectUuid/)
  assert.match(chatSource, /createChatTurn\(projectUuid, thread\.uuid/)
  assert.match(chatStyles, /\.chat-scene-card/)
})

test('premise batches expose full detail and optimistic Ignore or restore', () => {
  assert.match(source, /useInfiniteQuery/)
  assert.match(source, /useQueries/)
  assert.match(source, /listSettingImages\(projectUuid, \{ sourceUuids \}\)/)
  assert.match(source, /pagination\?\.total/)
  assert.match(source, /sourcesQuery\.isFetchNextPageError[\s\S]*premise\.history\.retry_more/)
  assert.match(source, /sourcesQuery\.refetch\(\)/)
  assert.match(source, /updatePremiseSource\(projectUuid, source\.uuid, \{ ignored, expected_revision: source\.revision \}\)/)
  assert.match(source, /premise\.batches\.source_uuid/)
  assert.match(source, /premise\.batches\.full_description/)
  assert.match(source, /JSON\.stringify\(source\.parameters/)
  assert.match(source, /ignored \? 'premise\.batches\.restore' : 'premise\.batches\.ignore'/)
})

test('premise batch dialog composes the existing Source to setting image workflow', () => {
  assert.match(source, /createPremiseSource\(projectUuid/)
  assert.match(source, /generateSettingImage\(projectUuid, source\.uuid/)
  assert.match(source, /premise\.batch_dialog\.intro/)
  assert.match(source, /storyProfileQuery\.data\.story_md/)
  assert.match(source, /breakdownSettingImage\(projectUuid, setting\.uuid/)
})

test('premise upload supports multiple files, drag, paste, previews and safe partial retry', () => {
  assert.match(source, /type="file" accept="image\/\*" multiple/)
  assert.match(source, /onDrop=\{\(event\) =>/)
  assert.match(source, /onPaste=\{\(event\) =>/)
  assert.match(source, /previewUrl: URL\.createObjectURL\(file\)/)
  assert.match(source, /completedDraftIds\.push\(draft\.id\)/)
  assert.match(source, /return !completedDraftIds\.includes\(draft\.id\)/)
  assert.match(source, /createAssetUpload\(projectUuid, \{ purpose: 'premise_asset'/)
  assert.match(source, /createPremiseAsset\(projectUuid/)
})

test('pasting an image into blank workspace selects its generated title for immediate editing', () => {
  assert.match(source, /addUploadFiles\(files, \{ editTitle: true \}\)/)
  assert.match(source, /uploadTitleRefs\.current\.get\(uploadTitleFocusId\)/)
  assert.match(source, /input\.focus\(\)[\s\S]*input\.select\(\)/)
  assert.match(source, /ref=\{\(element\) => \{ if \(element\) uploadTitleRefs\.current\.set\(draft\.id, element\)/)
})

test('premise selected states retain explicit combined hover feedback', () => {
  assert.match(styles, /\.premise-type-filters[\s\S]*?&:hover,[\s\S]*?color: \$color-text/)
  for (const selector of [
    '.premise-toolbar__tabs button[aria-selected="true"]:hover',
    '.premise-tag-filters button[aria-pressed="true"]:hover',
    '.premise-setting-card button[aria-pressed="true"]:hover',
    '.premise-variant-section button[aria-pressed="true"]:hover',
    '.premise-batch-card__toggle[aria-expanded="true"]:hover',
    '.premise-prompt-layout > nav button[aria-pressed="true"]:hover',
  ]) {
    assert.ok(styles.includes(selector), `missing combined hover selector: ${selector}`)
  }
})

test('premise narrow layout keeps controls operable and moves ChatArea into an overlay', () => {
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.project-canvas[\s\S]*?\.premise-toolbar[\s\S]*?flex-direction: row/)
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.project-canvas[\s\S]*?\.premise-add,[\s\S]*?\.premise-add__trigger[\s\S]*?width: auto/)
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.premise-card-grid[\s\S]*?repeat\(2, minmax\(0, 1fr\)\)/)
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.premise-detail-layout[\s\S]*?grid-template-columns: 1fr/)
  assert.match(shellStyles, /@media \(max-width: 1179px\)[\s\S]*?grid-template-columns: minmax\(0, 1fr\)/)
  assert.match(layoutSource, /const COMPACT_CHAT_QUERY = '\(max-width: 1179px\)'/)
  assert.match(layoutSource, /compact && overlayOpen/)
  assert.match(layoutSource, /role=\{compact && overlayOpen \? 'dialog' : undefined\}/)
  assert.match(layoutSource, /aria-modal=\{compact && overlayOpen \? 'true' : undefined\}/)
  assert.match(layoutSource, /aria-label=\{compact && overlayOpen \? t\('chat\.project'\) : undefined\}/)
})
