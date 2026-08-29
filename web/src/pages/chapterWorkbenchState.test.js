import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import {
  chapterPromptKey,
  chapterWorkbenchPrompts,
  dirtyChapterPrompts,
  enterTimelineMultiSelect,
  filterTimelineSelection,
  normalizedChapterTab,
  normalizedPreviewTab,
  patchWorkbenchSearch,
  promptSaveFailures,
  reorderedTimelineUuids,
  sectionImageGenerationActive,
  timelineDragScrollDelta,
  timelineDragTransition,
  timelineManageDisabledState,
  timelineSectionDropIntent,
  timelineSelectableSectionUuids,
  timelineSelectionControls,
  toggleTimelineSelection,
} from './chapterWorkbenchState.js'

test('chapter workbench normalizes shareable tab state without dropping an open ChatArea thread', () => {
  assert.equal(normalizedChapterTab('body'), 'body')
  assert.equal(normalizedChapterTab('unknown'), 'storyboard')
  assert.equal(normalizedPreviewTab('reference'), 'reference')
  assert.equal(normalizedPreviewTab('unknown'), 'current')

  const next = patchWorkbenchSearch('?chat_thread_uuid=thread-uuid&preview_tab=reference', {
    workspace_tab: 'prompts',
    preview_tab: null,
  })
  assert.equal(next.get('chat_thread_uuid'), 'thread-uuid')
  assert.equal(next.get('workspace_tab'), 'prompts')
  assert.equal(next.has('preview_tab'), false)
})

test('chapter workbench exposes the configured chapter prompt families', () => {
  const items = ['json_system', 'comic_storyboard', 'section_premise_selection', 'section_image', 'before_image']
    .map((prompt_key) => ({ prompt_key }))
  assert.deepEqual(chapterWorkbenchPrompts(items).map((item) => item.prompt_key), [
    'json_system',
    'comic_storyboard',
    'section_premise_selection',
    'section_image',
  ])
})

test('chapter workbench distinguishes loading, error, and empty section states', () => {
  const source = readFileSync(new URL('./ChapterWorkbenchPage.jsx', import.meta.url), 'utf8')
  assert.match(source, /sectionsQuery\.isLoading \? <ChapterComicSkeleton/)
  assert.match(source, /sectionsQuery\.isError && !sectionsQuery\.data/)
  assert.match(source, /aria-busy="true"/)
})

test('comic readiness warning stays non-blocking and refreshes for premise realtime changes', () => {
  const source = readFileSync(new URL('./ChapterWorkbenchPage.jsx', import.meta.url), 'utf8')
  const realtime = readFileSync(new URL('../realtime/projectRealtimeQueries.js', import.meta.url), 'utf8')
  assert.match(source, /comicStateQuery\.data && !comicStateQuery\.data\.has_premise_assets/)
  assert.ok((source.match(/<PremiseAssetsWarning/g) || []).length >= 2)
  assert.match(realtime, /event\.startsWith\('premise:'\)[\s\S]*addComic\(\)/)
  assert.match(realtime, /const comicKeys = \[[^\]]*'comic-state'/)
  assert.match(source, /const search = searchParams\.toString\(\)/)
  assert.match(source, /pathname: `\/projects\/\$\{encodeURIComponent\(projectUuid\)\}\/premise`/)
  assert.doesNotMatch(source, /disabled=\{[^}]*has_premise_assets/)
})

test('current and candidate comic images use the card and dialog structures', () => {
  const source = readFileSync(new URL('./ChapterWorkbenchPage.jsx', import.meta.url), 'utf8')
  const productionImageSource = readFileSync(new URL('./ProductionWorkspaces.jsx', import.meta.url), 'utf8')
  const styles = readFileSync(new URL('../styles/workspaces.sass', import.meta.url), 'utf8')
  assert.match(source, /mode: 'preview'/)
  assert.match(source, /mode: 'candidate'/)
  assert.match(source, /<CurrentImagePreviewDialog/)
  assert.match(source, /<ImageVariantDetailDialog/)
  assert.match(source, /className="comic-section-visual comic-section-visual--image"/)
  assert.match(source, /className="comic-section-visual__media comic-section-visual__media-button"/)
  assert.match(source, /className="section-image-variant-item__details"/)
  assert.doesNotMatch(source, /section-preview__detail-surface/)
  assert.match(productionImageSource, /renderReady \? renderReady\(image\)/)
  assert.match(source, /profile="detail_1024"/)
  assert.doesNotMatch(source, /imageDetail\.input_snapshot|variant\.input_snapshot/)
  assert.match(source, /disabled=\{pending \|\| current\}/)
  assert.match(styles, /\.comic-section-visual__media-button[\s\S]*background: \$color-surface/)
  assert.match(styles, /\.section-image-variant-dialog__body[\s\S]*grid-template-columns: minmax\(0, 1fr\) 360px/)
  assert.match(styles, /\.lumi-dialog\.section-image-dialog[\s\S]*width: min\(920px/)
})

test('snapshot history loads public detail before enabling a confirmed restore', () => {
  const source = readFileSync(new URL('./ChapterWorkbenchPage.jsx', import.meta.url), 'utf8')
  assert.match(source, /getComicSnapshot\(projectUuid, chapterUuid, selectedSnapshot\.uuid\)/)
  assert.match(source, /enabled: Boolean\(historyOpen && selectedSnapshot\?\.uuid\)/)
  assert.match(source, /!snapshotDetailQuery\.isSuccess \|\| snapshotDetailQuery\.data\?\.uuid !== selectedSnapshot\?\.uuid/)
  assert.match(source, /restore_confirm[\s\S]*count: detail\.sections\?\.length \|\| 0/)
  assert.match(source, /dismissDisabled=\{restoreMutation\.isPending\}/)
  assert.match(source, /setSelectedSnapshotUuid\(snapshot\.uuid\)/)
  assert.match(source, /<SnapshotMedia[\s\S]*premise_reference/)
  assert.doesNotMatch(source, /snapshot\.snapshot\?\.sections/)
})

test('snapshot preview uses a two-column desktop dialog and a stacked narrow layout', () => {
  const styles = readFileSync(new URL('../styles/workspaces.sass', import.meta.url), 'utf8')
  const desktopStart = styles.indexOf('.chapter-history-dialog\n')
  const desktop = styles.slice(desktopStart, styles.indexOf('.storyboard-candidates-dialog\n', desktopStart))
  assert.match(desktop, /grid-template-columns: 270px minmax\(0, 1fr\)/)
  assert.match(desktop, /\[aria-pressed="true"\]:hover/)
  const narrow = styles.slice(styles.lastIndexOf('@media (max-width: 760px)'))
  assert.match(narrow, /\.chapter-history-dialog__body[\s\S]*grid-template-columns: 1fr/)
  assert.match(narrow, /\.chapter-snapshot-section__media[\s\S]*grid-template-columns: 1fr/)
})

test('chapter workbench follows the 1100px single-column breakpoint', () => {
  const styles = readFileSync(new URL('../styles/workspaces.sass', import.meta.url), 'utf8')
  const responsive = styles.slice(styles.lastIndexOf('@media (max-width: 1100px)'))
  assert.match(responsive, /\.chapter-comic-workbench,\s+\.chapter-comic-skeleton\s+height: auto/)
  assert.match(responsive, /grid-template-columns: minmax\(0, 1fr\)/)
  assert.match(responsive, /\.storyboard-workbench,[\s\S]*min-height: 520px/)
  assert.match(responsive, /\.section-preview,[\s\S]*grid-row: 2/)
})

test('chapter prompt save planning keeps only dirty non-empty drafts and preserves partial failures', () => {
  const prompts = [
    { prompt_group: 'chapter', prompt_key: 'comic_storyboard', effective_value: 'old', current_version: { version_no: 3 } },
    { prompt_group: 'chapter', prompt_key: 'section_image', effective_value: 'same', current_version: { version_no: 5 } },
    { prompt_group: 'chapter', prompt_key: 'json_system', effective_value: 'json', current_version: null },
  ]
  const drafts = {
    [chapterPromptKey(prompts[0])]: 'new',
    [chapterPromptKey(prompts[1])]: 'same',
    [chapterPromptKey(prompts[2])]: '',
  }
  assert.deepEqual(dirtyChapterPrompts(prompts, drafts).map((item) => item.prompt_key), ['comic_storyboard'])

  const conflict = new Error('version conflict')
  assert.deepEqual(promptSaveFailures([
    { key: 'chapter/comic_storyboard', version: { version_no: 4 } },
    { key: 'chapter/section_image', error: conflict },
  ]), { 'chapter/section_image': conflict })
})

test('timeline multi-select starts from the current selectable Section and keeps counts actionable', () => {
  const sections = [
    { uuid: 'section-a', current_storyboard: { uuid: 'storyboard-a' } },
    { uuid: 'section-b', current_storyboard: { uuid: 'storyboard-b' } },
    { uuid: 'section-c', current_storyboard: null },
  ]
  const tasks = [
    { kind: 'comic_image_generation', resource_uuid: 'section-b', status: 'running' },
    { kind: 'comic_image_generation', resource_uuid: 'section-a', status: 'completed' },
  ]
  const selectable = timelineSelectableSectionUuids(sections, tasks)
  assert.deepEqual(selectable, ['section-a'])
  assert.equal(sectionImageGenerationActive(tasks, 'section-b'), true)
  assert.equal(sectionImageGenerationActive(tasks, 'section-a'), false)

  const initial = enterTimelineMultiSelect('section-a', selectable)
  assert.deepEqual([...initial], ['section-a'])
  assert.deepEqual([...enterTimelineMultiSelect('section-b', selectable)], [])

  const cleared = toggleTimelineSelection(initial, 'section-a', selectable)
  assert.deepEqual([...cleared], [])
  assert.deepEqual([...toggleTimelineSelection(cleared, 'section-b', selectable)], [])
  assert.deepEqual([...filterTimelineSelection(new Set(['section-a', 'section-b']), selectable)], ['section-a'])

  assert.deepEqual(timelineSelectionControls(cleared, selectable), {
    selection: new Set(),
    selectedCount: 0,
    selectableCount: 1,
    selectAllDisabled: false,
    clearDisabled: true,
    generateDisabled: true,
  })
  assert.equal(timelineSelectionControls(initial, selectable).selectAllDisabled, true)
})

test('chapter workbench submits selected images through one batch request', () => {
  const source = readFileSync(new URL('./ChapterWorkbenchPage.jsx', import.meta.url), 'utf8')
  assert.match(source, /generateChapterImagesBatch\(projectUuid, chapterUuid, \{[\s\S]*section_uuids:/)
  assert.doesNotMatch(source, /for \(const section of targets\)/)
})

test('timeline management disables destructive controls during generation and every mutation', () => {
  assert.deepEqual(timelineManageDisabledState({ index: 0, total: 3 }), {
    pending: false,
    createDisabled: false,
    doneDisabled: false,
    dragDisabled: false,
    deleteDisabled: false,
    moveBeforeDisabled: true,
    moveAfterDisabled: false,
  })
  assert.equal(timelineManageDisabledState({ imageGenerationActive: true, index: 1, total: 3 }).deleteDisabled, true)
  for (const pendingKey of ['createPending', 'deletePending', 'reorderPending']) {
    const state = timelineManageDisabledState({ [pendingKey]: true, index: 1, total: 3 })
    assert.equal(state.pending, true)
    assert.equal(state.dragDisabled, true)
    assert.equal(state.deleteDisabled, true)
    assert.equal(state.doneDisabled, true)
  }
  const last = timelineManageDisabledState({ index: 2, total: 3 })
  assert.equal(last.moveBeforeDisabled, false)
  assert.equal(last.moveAfterDisabled, true)
})

test('timeline drag intent supports start, cancel, before/after completion, and edge scrolling', () => {
  const timelineRect = { left: 10, right: 310, top: 100, bottom: 220 }
  const sectionRects = [
    { uuid: 'a', left: 10, width: 88, height: 118 },
    { uuid: 'b', left: 106, width: 88, height: 118 },
    { uuid: 'c', left: 202, width: 88, height: 118 },
  ]
  assert.deepEqual(timelineSectionDropIntent({ timelineRect, sectionRects, clientX: 110, clientY: 140, draggingUuid: 'a' }), { targetUuid: 'b', placement: 'before' })
  assert.deepEqual(timelineSectionDropIntent({ timelineRect, sectionRects, clientX: 280, clientY: 140, draggingUuid: 'a' }), { targetUuid: 'c', placement: 'after' })
  assert.equal(timelineSectionDropIntent({ timelineRect, sectionRects, clientX: 110, clientY: 260, draggingUuid: 'a' }), null)
  assert.deepEqual(reorderedTimelineUuids(['a', 'b', 'c'], 'a', 'c', 'after'), ['b', 'c', 'a'])
  assert.deepEqual(reorderedTimelineUuids(['a', 'b', 'c'], 'c', 'a', 'before'), ['c', 'a', 'b'])
  assert.equal(reorderedTimelineUuids(['a', 'b', 'c'], 'a', 'b', 'before'), null)
  assert.equal(reorderedTimelineUuids(['a', 'b', 'c'], 'a', '', 'before'), null)
  assert.equal(timelineDragScrollDelta(timelineRect, 20), -22)
  assert.equal(timelineDragScrollDelta(timelineRect, 300), 22)
  assert.equal(timelineDragScrollDelta(timelineRect, 160), 0)

  const started = timelineDragTransition(null, { type: 'start', sectionUuid: 'a', preview: { left: 10, top: 100 } })
  assert.deepEqual(started, { sectionUuid: 'a', targetUuid: null, placement: null, preview: { left: 10, top: 100 } })
  const moved = timelineDragTransition(started, { type: 'move', targetUuid: 'c', placement: 'after', preview: { left: 210 } })
  assert.deepEqual(moved, { sectionUuid: 'a', targetUuid: 'c', placement: 'after', preview: { left: 210, top: 100 } })
  assert.equal(timelineDragTransition(moved, { type: 'cancel' }), null)
  assert.equal(timelineDragTransition(moved, { type: 'complete' }), null)
})

test('chapter workbench uses CodeMirror and the shared single-prompt editor', () => {
  const source = readFileSync(new URL('./ChapterWorkbenchPage.jsx', import.meta.url), 'utf8')
  const promptEditor = readFileSync(new URL('../components/PromptCatalogEditor.jsx', import.meta.url), 'utf8')
  assert.match(source, /<MarkdownEditor[\s\S]*enableSearch/)
  assert.match(source, /storyboardRef\.current\?\.openSearchPanel\(\)/)
  assert.match(source, /<PromptCatalogEditor projectUuid=\{projectUuid\} groups=\{\['chapter'\]\}/)
  assert.match(promptEditor, /createPromptVersion\(projectUuid, promptUpdatePayload\(definition, draft\)\)/)
  assert.match(promptEditor, /disabled=\{!dirty \|\| invalid \|\| saveMutation\.isPending\}/)
})

test('storyboard AI draft routes through project ChatArea with a preselected Section Reference', () => {
  const workbench = readFileSync(new URL('./ChapterWorkbenchPage.jsx', import.meta.url), 'utf8')
  const chatArea = readFileSync(new URL('../components/ChatArea.jsx', import.meta.url), 'utf8')
  assert.match(workbench, /chat_reference_type:\s*'comic_section'/)
  assert.match(workbench, /chat_reference_uuid:\s*selected\.uuid/)
  assert.match(chatArea, /requestedReferenceType = searchParams\.get\('chat_reference_type'\)/)
  assert.match(chatArea, /resource_type: requestedReferenceType, resource_uuid: requestedReferenceUuid/)
  assert.doesNotMatch(chatArea, /chat_scene|chat_subject_uuid|subject_uuid/)
})

test('manual Section image workflows appear in ChatArea without changing the selected thread', () => {
  const chatArea = readFileSync(new URL('../components/ChatArea.jsx', import.meta.url), 'utf8')
  const presentation = readFileSync(new URL('./chatAreaPresentation.js', import.meta.url), 'utf8')
  const realtime = readFileSync(new URL('../realtime/projectRealtimeQueries.js', import.meta.url), 'utf8')
  const messages = readFileSync(new URL('../i18n/messages/chat.js', import.meta.url), 'utf8')

  for (const step of ['select_reference_assets', 'save_section_premise', 'generate_section_image', 'save_section_image']) {
    assert.match(chatArea, new RegExp(`${step}: 'chat\\.workflow\\.step\\.${step}'`))
  }
  assert.match(chatArea, /pending: 'chat\.workflow\.status\.queued'/)
  assert.match(presentation, /comic_section_image_generation: 'chat\.workflow\.kind\.comic_section_image_generation'/)
  assert.match(chatArea, /threadDisplayTitle\(thread, workflowByThread\.get\(thread\.uuid\), t\)/)
  assert.match(messages, /'chat\.workflow\.kind\.comic_section_image_generation': \['漫画片段图片生成', 'Comic section image generation'\]/)
  assert.match(realtime, /event\.startsWith\('production_task:'\)/)
  assert.doesNotMatch(chatArea, /production_task:queued[\s\S]*setSelectedThreadUuid/)
})
