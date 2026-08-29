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

test('premise toolbar keeps workspace navigation and the exact three creation choices', () => {
  for (const labelKey of ['premise.tab.assets', 'projects.tab.trash', 'premise.tab.batches', 'premise.threads.title', 'projects.tab.prompts', 'premise.tab.llm_logs']) {
    assert.match(source, new RegExp(`labelKey: '${labelKey.replaceAll('.', '\\.')}'`))
  }
  for (const menuItemKey of ['premise.add.batch.title', 'premise.add.single.title', 'premise.add.upload.title']) {
    assert.match(source, new RegExp(`t\\('${menuItemKey.replaceAll('.', '\\.')}'\\)`))
  }
  assert.match(source, /onClick=\{\(\) => openChat\(\)\}/)
  assert.doesNotMatch(source, /openChatScene/)
  assert.doesNotMatch(source, /pending: true|showPendingCapability/)
})

test('project thread lists are shared while premise entries preselect a Reference', () => {
  assert.match(source, /openChat\(asset\)/)
  assert.match(source, /next\.set\('chat_reference_type', 'premise_asset'\)/)
  assert.match(source, /next\.set\('chat_reference_uuid', reference\.uuid\)/)
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
  assert.doesNotMatch(chatSource, /requestedScope|requestedScene|requestedSubjectUuid/)
  assert.match(supportSource, /<PromptCatalogEditor projectUuid=\{projectUuid\} groups=\{\['premise', 'premise_style'\]\}/)
  assert.match(chatSource, /requestedReferenceType = searchParams\.get\('chat_reference_type'\)/)
  assert.match(chatSource, /appendProjectChatReference\(current, requestedReference\)/)
  assert.match(chatSource, /createChatTurn\(projectUuid, thread\.uuid/)
  assert.match(chatStyles, /\.chat-reference-picker/)
  assert.doesNotMatch(chatStyles, /\.chat-scene-card/)
})

test('premise asset project references load the exact active or trashed detail and clear on close', () => {
	assert.match(source, /linkedAssetUuid = searchParams\.get\('premise_asset_uuid'\)/)
	assert.match(source, /getPremiseAsset\(projectUuid, linkedAssetUuid\)/)
	assert.match(source, /setActiveTab\(asset\.deleted_at \? 'trash' : 'assets'\)/)
	assert.match(source, /asset\.uuid !== linkedAssetUuid/)
	assert.match(source, /asset\.uuid !== linkedAssetUuid\)[\s\S]*setHistoryAsset\(null\)[\s\S]*setDetailDraft\(null\)/)
	assert.match(source, /linkedAssetQuery\.error/)
	assert.match(source, /next\.delete\('premise_asset_uuid'\)[\s\S]*setSearchParams\(next, \{ replace: true \}\)/)
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
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.premise-toolbar[\s\S]*?flex-direction: column/)
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.premise-add,[\s\S]*?\.premise-add__trigger[\s\S]*?width: 100%/)
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.premise-card-grid[\s\S]*?minmax\(140px, 1fr\)/)
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.premise-detail-layout[\s\S]*?grid-template-columns: 1fr/)
  assert.match(shellStyles, /@media \(max-width: 980px\)[\s\S]*?grid-template-columns: minmax\(0, 1fr\)/)
  assert.match(layoutSource, /const COMPACT_CHAT_QUERY = '\(max-width: 980px\)'/)
  assert.match(layoutSource, /compact && overlayOpen/)
  assert.match(layoutSource, /role="dialog" aria-modal="true" aria-label=\{t\('chat\.project'\)\}/)
})
