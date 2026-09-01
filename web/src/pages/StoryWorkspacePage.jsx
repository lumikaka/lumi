import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, Navigate, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, RefreshCw, Save, Trash2, X } from 'lucide-react'

import MarkdownPreview from '../components/MarkdownPreview.jsx'
import DraftProjectWorkspace from '../components/DraftProjectWorkspace.jsx'
import LumiDialog from '../components/LumiDialog.jsx'
import ProjectWorkspaceLayout from '../components/ProjectWorkspaceLayout.jsx'
import { ProjectDashboardModeProvider, useProjectDashboardMode } from '../components/ProjectDashboardModeContext.jsx'
import WorkspaceGroupTabs from '../components/WorkspaceGroupTabs.jsx'
import { workspaceSectionForPath } from '../components/workspaceNavigation.js'
import { cancelTask, createChapterGeneration, createComicStoryboardGeneration, listTasks, retryTask } from '../api/ai.js'
import { createAssetUpload, createIntegrityScan, finalizeAssetUpload, listAssetMaintenanceTasks, listAssets, listIntegrityScans, reconcileAssets, restoreAsset, trashAsset } from '../api/assets.js'
import {
  getChapter,
  emptyChapterTrash,
  getStoryProject,
  listChapters,
  listChapterStories,
  permanentlyDeleteChapter,
  restoreChapter,
  trashChapter,
  updateChapter,
  updateChapterStory,
} from '../api/story.js'
import { saveStateForError } from './storyWorkspaceState.js'
import { ACTIVE_TASK_STATUSES, latestTaskForResource, taskControls } from './aiRuntimeState.js'
import PremiseWorkspace from './PremiseWorkspace.jsx'
import { OverviewExportsPanel, OverviewSummaryPanel } from './ProjectOverviewPanels.jsx'
import { PromptPanel, StoryProfilePanel } from './ProjectOverviewContentPanels.jsx'
import ProjectLLMLogsPanel from './ProjectLLMLogsPanel.jsx'
import ChaptersWorkspace from './ChaptersWorkspace.jsx'
import ChapterComicPreviewPage from './ChapterComicPreviewPage.jsx'
import ChapterWorkbenchPage from './ChapterWorkbenchPage.jsx'
import ThreadTrajectoryPage from './trajectory/ThreadTrajectoryPage.jsx'
import SimpleProjectWorkspace from './SimpleProjectWorkspace.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { localizedErrorPresentation } from '../i18n/errorLocalization.js'
import { useI18n } from '../i18n/useI18n.js'
import { assetKindLabel, sourceTypeLabel, statusLabel as localizedStatusLabel, taskKindLabel } from '../i18n/labels.js'
import { formatTerminologyMessageKey } from './pictureBookProfile.js'
import { canonicalProjectLocation, projectRoute } from '../projectRoutes.js'

const DEFAULT_COMIC_SECTION_COUNT = 6
const MAX_COMIC_SECTION_COUNT = 24

function ErrorNotice({ error, onDismiss }) {
  return <LocalizedErrorMessage error={error} onDismiss={onDismiss} />
}

function SaveState({ state }) {
  const { t } = useI18n()
  const keys = {
    idle: 'story.save.idle',
    saving: 'common.status.saving',
    saved: 'common.status.saved',
    failed: 'story.save.failed',
    conflict: 'story.save.conflict',
  }
  return <span className={`save-state save-state--${state}`} aria-live="polite">{t(keys[state] || keys.idle)}</span>
}

function ViewToggle({ preview, setPreview }) {
  const { t } = useI18n()
  return (
    <div className="view-toggle" aria-label={t('story.editor.view')}>
      <button type="button" className="view-toggle__button" aria-pressed={!preview} onClick={() => setPreview(false)}>{t('common.action.edit')}</button>
      <button type="button" className="view-toggle__button" aria-pressed={preview} onClick={() => setPreview(true)}>{t('common.action.preview')}</button>
    </div>
  )
}

function GenerationPanel({ projectUuid, chapterUuid, pictureBook, disabled = false, onCompleted, compact = false }) {
  const { formatNumber, t } = useI18n()
  const term = (key, values) => t(formatTerminologyMessageKey(pictureBook, key), values)
  const queryClient = useQueryClient()
  const defaultPrompt = term('story.generation.default_prompt')
  const [prompt, setPrompt] = useState(defaultPrompt)
  const previousDefaultPromptRef = useRef(defaultPrompt)
  const [error, setError] = useState(null)
  const completedRef = useRef('')
  const tasksQuery = useQuery({
    queryKey: ['story-tasks', projectUuid],
    queryFn: () => listTasks(projectUuid, { limit: 100 }),
  })
  const latest = latestTaskForResource(tasksQuery.data?.items, chapterUuid)

  useEffect(() => {
    setPrompt((current) => current === previousDefaultPromptRef.current ? defaultPrompt : current)
    previousDefaultPromptRef.current = defaultPrompt
  }, [defaultPrompt])

  const refreshTasks = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['chat-threads', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['workflows', projectUuid] })
  }, [projectUuid, queryClient])
  useEffect(() => {
    if (latest?.status === 'completed' && latest.uuid !== completedRef.current) {
      completedRef.current = latest.uuid
      onCompleted?.(latest.uuid)
    }
  }, [latest?.status, latest?.uuid, onCompleted])

  const createMutation = useMutation({
    mutationFn: () => createChapterGeneration(projectUuid, chapterUuid, {
      prompt,
      parameters: { temperature: 0.7 },
      idempotency_key: `chapter-generation-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    }),
    onSuccess: (task) => { queryClient.setQueryData(['story-tasks', projectUuid], (current) => ({ items: [task, ...(current?.items || []).filter((item) => item.uuid !== task.uuid)] })); refreshTasks(); setError(null) },
    onError: setError,
  })
  const cancelMutation = useMutation({ mutationFn: () => cancelTask(projectUuid, latest.uuid), onSuccess: refreshTasks, onError: setError })
  const retryMutation = useMutation({ mutationFn: () => retryTask(projectUuid, latest.uuid), onSuccess: refreshTasks, onError: setError })
  const active = latest && ACTIVE_TASK_STATUSES.has(latest.status)
  const controls = taskControls(latest)
  const statusKeys = { queued: 'story.task.status.queued', running: 'story.task.status.running', waiting_for_input: 'story.task.status.waiting_for_input', completed: 'story.task.status.completed', failed: 'story.task.status.failed', cancelled: 'story.task.status.cancelled', interrupted: 'story.task.status.interrupted' }

  if (compact) {
    return (
      <section className="chapter-body-generation">
        <div><strong>{term('story.generation.title')}</strong><small>{t('story.generation.provider_note')}</small></div>
        {latest ? <span className={`task-status task-status--${latest.status}`}>{statusKeys[latest.status] ? t(statusKeys[latest.status]) : t('common.status.unknown_with_code', { code: latest.status })}</span> : null}
        <details><summary>{t('story.generation.requirements')}</summary><textarea name="chapter_generation_requirements" value={prompt} onChange={(event) => setPrompt(event.target.value)} rows="4" /></details>
        <button type="button" disabled={disabled || active || !prompt.trim() || createMutation.isPending} onClick={() => createMutation.mutate()}>{t(createMutation.isPending ? 'story.generation.enqueuing' : active ? 'story.generation.running' : 'story.generation.new_version')}</button>
        <ErrorNotice error={error || tasksQuery.error} onDismiss={() => setError(null)} />
        {latest && active ? <progress max="100" value={latest.progress} /> : null}
      </section>
    )
  }

  return (
    <section className="generation-panel">
      <header><div><p className="eyebrow">{t('story.generation.eyebrow')}</p><h2>{term('story.generation.title')}</h2></div>{latest ? <span className={`task-status task-status--${latest.status}`}>{statusKeys[latest.status] ? t(statusKeys[latest.status]) : t('common.status.unknown_with_code', { code: latest.status })}</span> : null}</header>
      <ErrorNotice error={error || tasksQuery.error} onDismiss={() => setError(null)} />
        <div className="generation-form">
          <p>{t('story.generation.provider_note')}</p>
          <label className="generation-prompt">{t('story.generation.requirements')}<textarea name="chapter_generation_requirements" value={prompt} onChange={(event) => setPrompt(event.target.value)} rows="4" /></label>
          <button type="button" disabled={disabled || active || !prompt.trim() || createMutation.isPending} onClick={() => createMutation.mutate()}>{t(createMutation.isPending ? 'story.generation.enqueuing' : active ? 'story.generation.running' : 'story.generation.new_version')}</button>
        </div>
      {latest ? <div className="task-runtime"><div><span>{t('story.task.progress', { progress: formatNumber(latest.progress) })}</span><span>{t('story.task.attempt', { attempt: formatNumber(latest.attempt), max: formatNumber(latest.max_attempts) })}</span></div><progress max="100" value={latest.progress} />{latest.error_message ? <LocalizedErrorMessage error={{ code: latest.error_code, message: latest.error_message }} compact /> : null}<div>{controls.canCancel ? <button type="button" className="button-secondary" disabled={cancelMutation.isPending} onClick={() => cancelMutation.mutate()}>{t('story.task.cancel')}</button> : null}{controls.canRetry ? <button type="button" className="button-secondary" disabled={retryMutation.isPending} onClick={() => retryMutation.mutate()}>{t('story.task.retry')}</button> : null}<small>{t('story.task.persisted_note')}</small></div></div> : null}
    </section>
  )
}

function ChapterEditorPanel({ projectUuid, embedded = false }) {
  const { formatDateTime, t } = useI18n()
  const { chapterUuid } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const projectQuery = useQuery({ queryKey: ['story-project', projectUuid], queryFn: () => getStoryProject(projectUuid) })
  const chapterQuery = useQuery({ queryKey: ['story-chapter', projectUuid, chapterUuid], queryFn: () => getChapter(projectUuid, chapterUuid) })
  const historyQuery = useQuery({ queryKey: ['story-chapter-history', projectUuid, chapterUuid], queryFn: () => listChapterStories(projectUuid, chapterUuid) })
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [format, setFormat] = useState('md')
  const [preview, setPreview] = useState(false)
  const [saveState, setSaveState] = useState('idle')
  const [error, setError] = useState(null)
  const [editorOpen, setEditorOpen] = useState(false)
  const [comicGenerationOpen, setComicGenerationOpen] = useState(false)
  const [comicGenerationPrompt, setComicGenerationPrompt] = useState('')
  const [comicMaxSectionCount, setComicMaxSectionCount] = useState(DEFAULT_COMIC_SECTION_COUNT)
  const pictureBook = projectQuery.data?.picture_book
  const term = (key, values) => t(formatTerminologyMessageKey(pictureBook, key), values)
  const pageMode = Boolean(projectQuery.data?.picture_book?.format && projectQuery.data.picture_book.format !== 'vertical_strip')
  const initializedUuid = useRef('')
  const revisionRef = useRef(0)
  const savedContentRef = useRef('')
  const savedFormatRef = useRef('md')
  const contentRef = useRef('')
  contentRef.current = content

  const applyServerChapter = (chapter, force = false) => {
    revisionRef.current = chapter.revision
    queryClient.setQueryData(['story-chapter', projectUuid, chapterUuid], chapter)
    if (force || initializedUuid.current !== chapter.uuid) {
      initializedUuid.current = chapter.uuid
      setTitle(chapter.title || '')
      setContent(chapter.current_story?.content || '')
      setFormat(chapter.current_story?.content_format || 'md')
      savedContentRef.current = chapter.current_story?.content || ''
      savedFormatRef.current = chapter.current_story?.content_format || 'md'
      setSaveState('idle')
    }
  }

  useEffect(() => {
    if (chapterQuery.data) applyServerChapter(chapterQuery.data)
    // Initialization is intentionally keyed to the public resource UUID.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chapterQuery.data?.uuid])

  const saveMutation = useMutation({
    mutationFn: ({ draft, contentFormat, revision }) => updateChapterStory(projectUuid, chapterUuid, { content: draft, content_format: contentFormat, expected_revision: revision }),
    onMutate: () => { setSaveState('saving'); setError(null) },
    onSuccess: (chapter, variables) => {
      revisionRef.current = chapter.revision
      savedContentRef.current = variables.draft
      savedFormatRef.current = variables.contentFormat
      queryClient.setQueryData(['story-chapter', projectUuid, chapterUuid], chapter)
      queryClient.invalidateQueries({ queryKey: ['story-chapter-history', projectUuid, chapterUuid] })
      queryClient.invalidateQueries({ queryKey: ['story-chapters', projectUuid] })
      setSaveState(contentRef.current === variables.draft ? 'saved' : 'idle')
    },
    onError: (mutationError) => { setError(mutationError); setSaveState(saveStateForError(mutationError)) },
  })

  useEffect(() => {
    if (embedded || !initializedUuid.current || saveState === 'conflict' || saveMutation.isPending || (content === savedContentRef.current && format === savedFormatRef.current)) return undefined
    const timer = window.setTimeout(() => saveMutation.mutate({ draft: content, contentFormat: format, revision: revisionRef.current }), 700)
    return () => window.clearTimeout(timer)
  }, [content, embedded, format, saveState, saveMutation.isPending])

  const titleMutation = useMutation({
    mutationFn: () => updateChapter(projectUuid, chapterUuid, { title, expected_revision: revisionRef.current }),
    onSuccess: (chapter) => { revisionRef.current = chapter.revision; queryClient.setQueryData(['story-chapter', projectUuid, chapterUuid], chapter); queryClient.invalidateQueries({ queryKey: ['story-chapters', projectUuid] }) },
    onError: (mutationError) => { setError(mutationError); setSaveState(saveStateForError(mutationError)) },
  })
  const comicGenerationMutation = useMutation({
    mutationFn: () => createComicStoryboardGeneration(projectUuid, chapterUuid, {
      prompt: comicGenerationPrompt.trim(),
      max_section_count: Number(comicMaxSectionCount),
      parameters: { temperature: 0.7 },
      idempotency_key: `comic-storyboard-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] })
      queryClient.invalidateQueries({ queryKey: ['chat-threads', projectUuid] })
      queryClient.invalidateQueries({ queryKey: ['workflows', projectUuid] })
      setComicGenerationOpen(false)
      setComicGenerationPrompt('')
      setComicMaxSectionCount(DEFAULT_COMIC_SECTION_COUNT)
      setError(null)
    },
    onError: setError,
  })
  const trashMutation = useMutation({
    mutationFn: () => trashChapter(projectUuid, chapterUuid, revisionRef.current),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['story-chapters', projectUuid] }); queryClient.invalidateQueries({ queryKey: ['story-project', projectUuid] }); navigate(projectRoute(projectUuid, 'chapters', '?state=trashed')) },
    onError: setError,
  })

  const reload = async () => {
    const result = await chapterQuery.refetch()
    if (result.data) applyServerChapter(result.data, true)
    setError(null)
  }

  const closeEmbeddedEditor = () => {
    if (chapterQuery.data) applyServerChapter(chapterQuery.data, true)
    setEditorOpen(false)
    setError(null)
  }
  const saveEmbeddedEditor = async () => {
    try {
      if (title !== (chapterQuery.data?.title || '')) await titleMutation.mutateAsync()
      if (content !== savedContentRef.current || format !== savedFormatRef.current) {
        await saveMutation.mutateAsync({ draft: content, contentFormat: format, revision: revisionRef.current })
      }
      setEditorOpen(false)
    } catch {
      // The mutations retain the draft and expose localized conflict/error state.
    }
  }

  const applyGenerated = useCallback(async () => {
    const result = await chapterQuery.refetch()
    if (result.data) applyServerChapter(result.data, true)
    queryClient.invalidateQueries({ queryKey: ['story-chapter-history', projectUuid, chapterUuid] })
    queryClient.invalidateQueries({ queryKey: ['story-chapters', projectUuid] })
  // applyServerChapter intentionally works with the current editor refs.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chapterUuid, projectUuid, queryClient])

  if (chapterQuery.isLoading) return <p className="workspace-loading">{term('story.chapter.loading')}</p>
  if (chapterQuery.isError && !chapterQuery.data) return <ErrorNotice error={chapterQuery.error} />
  const chapter = chapterQuery.data

  if (embedded) {
    return (
      <section className="chapter-workbench__body">
        <header className="chapter-workbench__reader-header">
          <div><p className="eyebrow">{chapter.chapter_code}</p><h2>{chapter.title || term('story.chapter.title')}</h2></div>
          <div className="chapter-workbench__reader-actions">
            <button type="button" className="button-secondary" onClick={() => setEditorOpen(true)}><Pencil size={14} aria-hidden="true" />{t('common.action.edit')}</button>
            {chapter.current_story ? <span className="story-status-pill">{sourceTypeLabel(t, chapter.current_story.source_type)}</span> : null}
            <button type="button" disabled={!chapter.current_story || comicGenerationMutation.isPending} onClick={() => setComicGenerationOpen(true)}><RefreshCw size={14} aria-hidden="true" />{t(pageMode ? 'comic.workbench.body.regenerate_pages' : 'comic.workbench.body.regenerate_storyboard')}</button>
          </div>
        </header>
        <ErrorNotice error={error} onDismiss={() => setError(null)} />
        <article className="chapter-workbench__reader-content"><pre data-user-content>{chapter.current_story?.content || t('story.story_file_empty')}</pre></article>

        {editorOpen ? <ChapterBodyEditorDialog
          chapter={chapter}
          title={title}
          setTitle={setTitle}
          content={content}
          onContentChange={(value) => { setContent(value); if (saveState === 'saved') setSaveState('idle') }}
          format={format}
          setFormat={setFormat}
          preview={preview}
          setPreview={setPreview}
          saveState={saveState}
          error={error}
          history={historyQuery.data?.items || []}
          pending={titleMutation.isPending || saveMutation.isPending}
          onSave={saveEmbeddedEditor}
          onClose={closeEmbeddedEditor}
          onReload={reload}
          chapterTitleLabel={term('story.chapter.title')}
          editTitle={t(pageMode ? 'comic.workbench.body.edit_pages' : 'comic.workbench.body.edit')}
        >
          <GenerationPanel projectUuid={projectUuid} chapterUuid={chapterUuid} pictureBook={pictureBook} disabled={saveMutation.isPending || saveState === 'conflict'} onCompleted={applyGenerated} compact />
        </ChapterBodyEditorDialog> : null}

        {comicGenerationOpen ? <ComicStoryboardGenerationDialog
          prompt={comicGenerationPrompt}
          setPrompt={setComicGenerationPrompt}
          maxSectionCount={comicMaxSectionCount}
          setMaxSectionCount={setComicMaxSectionCount}
          pending={comicGenerationMutation.isPending}
          error={comicGenerationMutation.error}
          pageMode={pageMode}
          onClose={() => { setComicGenerationOpen(false); setComicGenerationPrompt(''); setComicMaxSectionCount(DEFAULT_COMIC_SECTION_COUNT) }}
          onSubmit={() => comicGenerationMutation.mutate()}
        /> : null}
      </section>
    )
  }

  return (
    <div className="workspace-stack">
      <header className="editor-header"><div><Link to={projectRoute(projectUuid, 'chapters')}>← {term('story.chapter.back')}</Link><p className="eyebrow">{chapter.chapter_code}</p><input name="chapter_title" className="chapter-title-input" value={title} onChange={(event) => setTitle(event.target.value)} aria-label={term('story.chapter.title')} /></div><div><SaveState state={saveState} /><button type="button" className="button-secondary" disabled={titleMutation.isPending || saveMutation.isPending} onClick={() => titleMutation.mutate()}>{t('story.chapter.save_title')}</button><button type="button" className="button-quiet danger-text" onClick={() => trashMutation.mutate()}>{t('story.chapter.trash')}</button></div></header>
      <ErrorNotice error={error} onDismiss={() => setError(null)} />
      {saveState === 'conflict' ? <div className="workspace-notice"><div><strong>{t('story.chapter.conflict_title')}</strong><span>{t('story.chapter.conflict_body')}</span></div><button type="button" onClick={reload}>{t('story.chapter.reload')}</button></div> : null}
      <GenerationPanel projectUuid={projectUuid} chapterUuid={chapterUuid} pictureBook={pictureBook} disabled={saveMutation.isPending || saveState === 'conflict'} onCompleted={applyGenerated} />
      <section className="chapter-editor-layout">
        <div className="editor-card">
          <div className="editor-toolbar"><ViewToggle preview={preview} setPreview={setPreview} /><div className="format-toggle"><button type="button" className="format-toggle__button" aria-pressed={format === 'md'} onClick={() => setFormat('md')}>Markdown</button><button type="button" className="format-toggle__button" aria-pressed={format === 'txt'} onClick={() => setFormat('txt')}>{t('story.chapter.plain_text')}</button></div></div>
          {preview ? <MarkdownPreview value={content} /> : <textarea name="chapter_content" className="story-editor" value={content} onChange={(event) => { setContent(event.target.value); if (saveState === 'saved') setSaveState('idle') }} placeholder={term('story.chapter.placeholder')} autoFocus />}
        </div>
        <aside className="version-history"><div><h2>{t('story.chapter.versions')}</h2><span>{historyQuery.data?.items?.length || 0}</span></div>{historyQuery.data?.items?.map((version) => <article key={version.uuid}><strong>v{version.version_no}</strong><span>{sourceTypeLabel(t, version.source_type)}</span><small>{formatDateTime(version.created_at)}</small></article>)}</aside>
      </section>
    </div>
  )
}

function ChapterBodyEditorDialog({ chapter, title, setTitle, content, onContentChange, format, setFormat, preview, setPreview, saveState, error, history, pending, onSave, onClose, onReload, chapterTitleLabel, editTitle, children }) {
  const { formatDateTime, t } = useI18n()

  return (
    <LumiDialog className="chapter-body-editor-dialog" dismissDisabled={pending} onClose={onClose}>
      <header className="lumi-dialog__header"><div><p className="eyebrow">{chapter.chapter_code}</p><h2>{editTitle}</h2></div><button type="button" className="button-quiet" disabled={pending} aria-label={t('common.action.close')} onClick={onClose}><X size={18} aria-hidden="true" /></button></header>
      <form className="lumi-dialog__body chapter-body-editor-dialog__body" onSubmit={(event) => { event.preventDefault(); onSave() }}>
        <ErrorNotice error={error} />
        {saveState === 'conflict' ? <div className="workspace-notice"><div><strong>{t('story.chapter.conflict_title')}</strong><span>{t('story.chapter.conflict_body')}</span></div><button type="button" onClick={onReload}>{t('story.chapter.reload')}</button></div> : null}
        <label>{chapterTitleLabel}<input name="chapter_title" value={title} onChange={(event) => setTitle(event.target.value)} /></label>
        <label>{t('comic.workbench.body.format')}<select name="chapter_format" value={format} onChange={(event) => setFormat(event.target.value)}><option value="txt">txt</option><option value="md">md</option></select></label>
        <div className="chapter-body-editor-dialog__toolbar"><ViewToggle preview={preview} setPreview={setPreview} /><SaveState state={saveState} /></div>
        {preview ? <MarkdownPreview value={content} /> : <label className="chapter-body-editor-dialog__content">{t('comic.workbench.body.content')}<textarea name="chapter_content" value={content} onChange={(event) => onContentChange(event.target.value)} /></label>}
        <details className="chapter-body-editor-dialog__secondary"><summary>{t('comic.workbench.body.local_tools')}</summary>{children}<aside className="chapter-body-editor-dialog__history"><h3>{t('story.chapter.versions')}</h3>{history.map((version) => <article key={version.uuid}><strong>v{version.version_no}</strong><span>{sourceTypeLabel(t, version.source_type)}</span><time dateTime={version.created_at}>{formatDateTime(version.created_at)}</time></article>)}</aside></details>
        <footer><button type="button" className="button-secondary" disabled={pending} onClick={onClose}>{t('common.action.cancel')}</button><button type="submit" disabled={pending}><Save size={14} aria-hidden="true" />{t(pending ? 'common.status.saving' : 'common.action.save')}</button></footer>
      </form>
    </LumiDialog>
  )
}

function ComicStoryboardGenerationDialog({ prompt, setPrompt, maxSectionCount, setMaxSectionCount, pending, error, pageMode = false, onClose, onSubmit }) {
  const { t } = useI18n()

  return (
    <LumiDialog className="comic-storyboard-generation-dialog" dismissDisabled={pending} onClose={onClose}>
      <header className="lumi-dialog__header"><div><h2>{t(pageMode ? 'comic.workbench.body.regenerate_pages' : 'comic.workbench.body.regenerate_storyboard')}</h2><p>{t(pageMode ? 'comic.workbench.body.regenerate_pages_body' : 'comic.workbench.body.regenerate_storyboard_body')}</p></div><button type="button" className="button-quiet" disabled={pending} aria-label={t('common.action.close')} onClick={onClose}><X size={18} aria-hidden="true" /></button></header>
      <form className="lumi-dialog__body" onSubmit={(event) => { event.preventDefault(); onSubmit() }}>
        <ErrorNotice error={error} />
        <label>{t('comic.workbench.body.storyboard_requirements')}<textarea name="comic_storyboard_requirements" rows="6" value={prompt} onChange={(event) => setPrompt(event.target.value)} /></label>
        <label>{t(pageMode ? 'comic.workbench.body.max_pages' : 'comic.workbench.body.max_sections')}<input name="max_section_count" type="number" min="1" max={MAX_COMIC_SECTION_COUNT} step="1" value={maxSectionCount} onChange={(event) => setMaxSectionCount(event.target.value)} /><small>{t(pageMode ? 'comic.workbench.body.max_pages_body' : 'comic.workbench.body.max_sections_body')}</small></label>
        <footer className="lumi-dialog__actions"><button type="button" className="button-secondary" disabled={pending} onClick={onClose}>{t('common.action.cancel')}</button><button type="submit" disabled={pending}><RefreshCw size={14} aria-hidden="true" />{t(pending ? 'common.status.processing' : 'comic.workbench.body.confirm_regenerate')}</button></footer>
      </form>
    </LumiDialog>
  )
}

function TrashPanel({ projectUuid, pictureBook }) {
  const { formatCount, formatDateTime, t } = useI18n()
  const term = (key, values) => t(formatTerminologyMessageKey(pictureBook, key), values)
  const queryClient = useQueryClient()
  const [error, setError] = useState(null)
  const [emptyDialogOpen, setEmptyDialogOpen] = useState(false)
  const [emptyResult, setEmptyResult] = useState(null)
  const trashQuery = useQuery({ queryKey: ['story-chapters', projectUuid, 'trashed'], queryFn: () => listChapters(projectUuid, 'trashed') })
  const refresh = () => {
    for (const key of ['story-chapters', 'story-project', 'story-chapter', 'story-chapter-history', 'comic-sections', 'comic-state', 'comic-snapshots', 'comic-exports']) {
      queryClient.invalidateQueries({ queryKey: [key, projectUuid] })
    }
  }
  const restoreMutation = useMutation({ mutationFn: (chapter) => restoreChapter(projectUuid, chapter.uuid, chapter.revision), onSuccess: refresh, onError: setError })
  const deleteMutation = useMutation({ mutationFn: (chapter) => permanentlyDeleteChapter(projectUuid, chapter.uuid, chapter.revision), onSuccess: refresh, onError: setError })
  const emptyMutation = useMutation({
    onMutate: () => { setError(null); setEmptyResult(null) },
    mutationFn: () => emptyChapterTrash(projectUuid),
    onSuccess: (result) => {
      setEmptyDialogOpen(false)
      setEmptyResult(result)
      refresh()
    },
    onError: setError,
  })
  const items = trashQuery.data?.items || []
  const confirmDelete = (chapter) => {
    if (window.confirm(t('story.trash.confirm', { code: chapter.chapter_code }))) deleteMutation.mutate(chapter)
  }
  return (
    <div className="workspace-stack">
      <header className="workspace-section-heading"><div><p className="eyebrow">{t('story.trash.eyebrow')}</p><h1>{term('story.trash.title')}</h1><p>{term('story.trash.description')}</p></div><div><span>{formatCount('common.count.items', items.length)}</span><button type="button" className="button-secondary story-trash-empty-button" disabled={!items.length || emptyMutation.isPending || deleteMutation.isPending} onClick={() => setEmptyDialogOpen(true)}><Trash2 size={15} aria-hidden="true" />{t('story.trash.empty_action')}</button></div></header>
      <ErrorNotice error={error || trashQuery.error} onDismiss={error ? () => setError(null) : undefined} />
      {emptyResult ? <div className="chapter-operation-summary" role="status"><div><strong>{term(emptyResult.blocked_items?.length ? 'story.trash.empty_partial' : 'story.trash.empty_done', { count: emptyResult.deleted_count })}</strong>{emptyResult.blocked_items?.length ? <ul>{emptyResult.blocked_items.map((item) => <li key={item.uuid}><span>{item.chapter_code}</span><small>{localizedErrorPresentation(t, { code: item.error_code }).message} · {item.error_code}</small></li>)}</ul> : null}</div><button type="button" onClick={() => setEmptyResult(null)} aria-label={t('common.action.close')}><X size={14} aria-hidden="true" /></button></div> : null}
      <section className="trash-list">{items.map((chapter) => <article key={chapter.uuid}><div><code>{chapter.chapter_code}</code><strong>{chapter.title || term('projects.unnamed_chapter')}</strong><small>{t('story.trash.trashed_at', { date: formatDateTime(chapter.trashed_at) })}</small></div><button type="button" disabled={emptyMutation.isPending} onClick={() => restoreMutation.mutate(chapter)}>{t('common.action.restore')}</button><button type="button" className="button-danger" disabled={emptyMutation.isPending} onClick={() => confirmDelete(chapter)}>{t('story.trash.permanent_delete')}</button></article>)}{!trashQuery.isLoading && items.length === 0 ? <div className="workspace-empty"><span>✓</span><h2>{t('story.trash.empty')}</h2></div> : null}</section>
      {emptyDialogOpen ? <LumiDialog className="story-trash-empty-dialog" dismissDisabled={emptyMutation.isPending} onClose={() => setEmptyDialogOpen(false)} aria-labelledby="story-trash-empty-title"><header className="lumi-dialog__header"><div><h2 id="story-trash-empty-title">{term('story.trash.empty_confirm_title')}</h2><p>{term('story.trash.empty_confirm_body', { count: items.length })}</p></div><button type="button" className="button-quiet" disabled={emptyMutation.isPending} onClick={() => setEmptyDialogOpen(false)} aria-label={t('common.action.close')}><X size={18} aria-hidden="true" /></button></header><div className="lumi-dialog__body"><p className="story-trash-empty-warning">{t('story.trash.empty_warning')}</p></div><footer className="lumi-dialog__actions"><button type="button" className="button-secondary" disabled={emptyMutation.isPending} onClick={() => setEmptyDialogOpen(false)}>{t('common.action.cancel')}</button><button type="button" className="button-danger" disabled={emptyMutation.isPending} onClick={() => emptyMutation.mutate()}>{t(emptyMutation.isPending ? 'common.status.processing' : 'story.trash.empty_action')}</button></footer></LumiDialog> : null}
    </div>
  )
}

function AssetsPanel({ projectUuid }) {
  const { formatNumber, t } = useI18n()
  const queryClient = useQueryClient()
  const [file, setFile] = useState(null)
  const [purpose, setPurpose] = useState('premise_asset')
  const [displayName, setDisplayName] = useState('')
  const [showTrash, setShowTrash] = useState(false)
  const [error, setError] = useState(null)
  const assetsQuery = useQuery({ queryKey: ['assets', projectUuid, showTrash], queryFn: () => listAssets(projectUuid, { deleted: showTrash }) })
  const scansQuery = useQuery({ queryKey: ['asset-scans', projectUuid], queryFn: () => listIntegrityScans(projectUuid) })
  const tasksQuery = useQuery({ queryKey: ['asset-maintenance-tasks', projectUuid], queryFn: () => listAssetMaintenanceTasks(projectUuid) })
  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['assets', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['asset-scans', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['asset-maintenance-tasks', projectUuid] })
  }, [projectUuid, queryClient])
  const uploadMutation = useMutation({
    mutationFn: async () => {
      const upload = await createAssetUpload(projectUuid, { purpose, displayName, file })
      return finalizeAssetUpload(projectUuid, upload.uuid, purpose)
    },
    onSuccess: () => { setFile(null); setDisplayName(''); setError(null); refresh() },
    onError: setError,
  })
  const scanMutation = useMutation({ mutationFn: () => createIntegrityScan(projectUuid), onSuccess: refresh, onError: setError })
  const reconcileMutation = useMutation({ mutationFn: () => reconcileAssets(projectUuid), onSuccess: refresh, onError: setError })
  const trashMutation = useMutation({ mutationFn: (asset) => trashAsset(projectUuid, asset.uuid), onSuccess: refresh, onError: setError })
  const restoreMutation = useMutation({ mutationFn: (asset) => restoreAsset(projectUuid, asset.uuid), onSuccess: refresh, onError: setError })
  const items = assetsQuery.data?.items || []
  const latestScan = scansQuery.data?.items?.[0]
  const latestTask = tasksQuery.data?.items?.[0]
  const summaries = latestScan?.summary || {}
  return (
    <div className="workspace-stack">
      <header className="workspace-section-heading"><div><p className="eyebrow">{t('premise.assets.eyebrow')}</p><h1>{t('premise.assets.title')}</h1><p>{t('premise.assets.description')}</p></div><button type="button" className="button-secondary" aria-pressed={showTrash} onClick={() => setShowTrash((value) => !value)}>{t(showTrash ? 'premise.assets.view_active' : 'premise.assets.view_trash')}</button></header>
      <ErrorNotice error={error || assetsQuery.error || scansQuery.error} onDismiss={() => setError(null)} />
      <section className="asset-control-grid">
        <form onSubmit={(event) => { event.preventDefault(); uploadMutation.mutate() }}>
          <div><span className="project-action__number">01</span><h2>{t('premise.assets.upload_title')}</h2><p>{t('premise.assets.upload_body')}</p></div>
          <label>{t('common.label.purpose')}<select value={purpose} onChange={(event) => setPurpose(event.target.value)}><option value="premise_asset">{t('premise.assets.purpose.premise_asset')}</option><option value="premise_setting_image">{t('premise.assets.purpose.premise_setting_image')}</option><option value="comic_section_image">{t('premise.assets.purpose.comic_section_image')}</option><option value="story_import">{t('premise.assets.purpose.story_import')}</option></select></label>
          <label>{t('premise.assets.display_name')}<input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder={t('common.label.optional')} /></label>
          <label>{t('premise.assets.select_file')}<input type="file" onChange={(event) => setFile(event.target.files?.[0] || null)} /></label>
          <button type="submit" disabled={!file || uploadMutation.isPending}>{t(uploadMutation.isPending ? 'premise.assets.validating' : 'premise.assets.upload')}</button>
        </form>
        <div className="asset-maintenance-card"><div><span className="project-action__number">02</span><h2>{t('premise.assets.maintenance_title')}</h2><p>{t('premise.assets.maintenance_body')}</p></div>{latestTask && ['queued', 'running'].includes(latestTask.status) ? <div className="task-runtime"><strong>{taskKindLabel(t, latestTask.kind)}</strong><progress max="100" value={latestTask.progress} /><small>{localizedStatusLabel(t, latestTask.status)} · {t('premise.assets.task_progress', { progress: formatNumber(latestTask.progress), uuid: latestTask.uuid.slice(0, 13) })}</small></div> : null}{latestScan ? <div className="scan-summary"><strong>{localizedStatusLabel(t, latestScan.status)} · {formatNumber(latestScan.progress)}%</strong><span>{t('premise.assets.findings', { count: formatNumber(latestScan.finding_count) })}</span><small>{t('premise.assets.scan_summary', { missing: formatNumber(summaries.missing || 0), corrupt: formatNumber(summaries.corrupt || 0), orphan: formatNumber(summaries.orphan || 0) })}</small></div> : <p>{t('premise.assets.scan_empty')}</p>}<div><button type="button" onClick={() => scanMutation.mutate()} disabled={scanMutation.isPending || ['queued', 'running'].includes(latestTask?.status)}>{t(scanMutation.isPending ? 'premise.assets.enqueuing' : 'premise.assets.scan')}</button><button type="button" className="button-secondary" onClick={() => reconcileMutation.mutate()} disabled={reconcileMutation.isPending}>{t('premise.assets.reconcile')}</button></div></div>
      </section>
      <section className="asset-grid" aria-label={t('premise.assets.list')}>
        {items.map((asset) => <article key={asset.uuid} className="asset-card">{asset.kind === 'image' && asset.status === 'ready' && !asset.deleted_at ? <img src={asset.content_url} alt={asset.display_name || asset.original_filename || t('premise.assets.preview')} /> : <div className={`asset-card__placeholder asset-card__placeholder--${asset.status}`}><span>{assetKindLabel(t, asset.kind)}</span><strong>{localizedStatusLabel(t, asset.status)}</strong></div>}<div><span className={`asset-status asset-status--${asset.status}`}>{localizedStatusLabel(t, asset.status)}</span><h2>{asset.display_name || asset.original_filename || t('premise.assets.untitled')}</h2><code>{asset.uuid.slice(0, 18)}…</code><p>{asset.mime_type} · {formatNumber(Math.ceil(asset.byte_size / 1024))} KB{asset.width ? ` · ${formatNumber(asset.width)}×${formatNumber(asset.height)}` : ''}</p>{asset.deleted_at ? <button type="button" onClick={() => restoreMutation.mutate(asset)}>{t('common.action.restore')}</button> : <button type="button" className="button-quiet danger-text" onClick={() => trashMutation.mutate(asset)}>{t('story.chapter.trash')}</button>}</div></article>)}
        {!assetsQuery.isLoading && items.length === 0 ? <div className="workspace-empty"><span>◫</span><h2>{t(showTrash ? 'premise.assets.trash_empty' : 'premise.assets.empty')}</h2><p>{t('premise.assets.empty_body')}</p></div> : null}
      </section>
    </div>
  )
}

export default function StoryWorkspacePage() {
  const { projectUuid } = useParams()
  return (
    <ProjectDashboardModeProvider key={projectUuid} projectUuid={projectUuid}>
      <StoryWorkspaceContent projectUuid={projectUuid} />
    </ProjectDashboardModeProvider>
  )
}

function StoryWorkspaceContent({ projectUuid }) {
  const location = useLocation()
  const { forcedExpert, simple } = useProjectDashboardMode()
  const projectQuery = useQuery({ queryKey: ['story-project', projectUuid], queryFn: () => getStoryProject(projectUuid) })
  const canonical = canonicalProjectLocation({
    projectUuid,
    pathname: location.pathname,
    search: location.search,
    hash: location.hash,
  })
  if (canonical) return <Navigate replace to={canonical} />

  if (simple) {
    return <SimpleProjectWorkspace project={projectQuery.data} projectQuery={projectQuery} projectUuid={projectUuid} />
  }

  const activeSection = workspaceSectionForPath(location.pathname)
  const chapterPreview = /\/chapters\/[^/]+\/preview$/.test(location.pathname)
  const trajectoryView = /\/threads\/[^/]+\/trajectory$/.test(location.pathname)
  const draftProject = projectQuery.data?.setup_status === 'draft'
  return (
    <ProjectWorkspaceLayout project={projectQuery.data} projectUuid={projectUuid} activeSection={activeSection} forcedExpert={forcedExpert} hideChat={chapterPreview || trajectoryView}>
      {!trajectoryView ? <WorkspaceGroupTabs projectUuid={projectUuid} activeSection={activeSection} pictureBook={projectQuery.data?.picture_book} hidden={draftProject} /> : null}
      <main className={`workspace-main ${trajectoryView ? 'workspace-main--trajectory' : ''}`}>
        {draftProject ? <DraftProjectWorkspace /> : <Routes>
          <Route index element={<OverviewSummaryPanel projectUuid={projectUuid} projectQuery={projectQuery} />} />
          <Route path="settings" element={<Navigate replace to={projectRoute(projectUuid, '', location.search)} />} />
          <Route path="story" element={<StoryProfilePanel projectUuid={projectUuid} pictureBook={projectQuery.data?.picture_book} />} />
          <Route path="prompts" element={<PromptPanel projectUuid={projectUuid} pictureBook={projectQuery.data?.picture_book} />} />
          <Route path="llm-logs" element={<ProjectLLMLogsPanel projectUuid={projectUuid} />} />
          <Route path="exports" element={<OverviewExportsPanel projectUuid={projectUuid} />} />
          <Route path="chapters" element={<ChapterCollectionRoute projectUuid={projectUuid} pictureBook={projectQuery.data?.picture_book} />} />
          <Route path="chapters/:chapterUuid/preview" element={<ChapterComicPreviewPage projectUuid={projectUuid} />} />
          <Route path="chapters/:chapterUuid/sections/:sectionUuid" element={<ChapterWorkbenchPage projectUuid={projectUuid} renderBody={() => <ChapterEditorPanel projectUuid={projectUuid} embedded />} />} />
          <Route path="chapters/:chapterUuid" element={<ChapterWorkbenchPage projectUuid={projectUuid} renderBody={() => <ChapterEditorPanel projectUuid={projectUuid} embedded />} />} />
          <Route path="premise" element={<PremiseWorkspace projectUuid={projectUuid} pictureBook={projectQuery.data?.picture_book} />} />
          <Route path="premise/assets/:assetUuid" element={<PremiseWorkspace projectUuid={projectUuid} pictureBook={projectQuery.data?.picture_book} />} />
          <Route path="assets" element={<AssetsPanel projectUuid={projectUuid} />} />
          <Route path="threads/:threadUuid/trajectory" element={<ThreadTrajectoryPage projectUuid={projectUuid} />} />
          <Route path="*" element={<ProjectWorkspaceNotFound projectUuid={projectUuid} />} />
        </Routes>}
      </main>
    </ProjectWorkspaceLayout>
  )
}

function ChapterCollectionRoute({ pictureBook, projectUuid }) {
  const location = useLocation()
  return new URLSearchParams(location.search).get('state') === 'trashed'
    ? <TrashPanel projectUuid={projectUuid} pictureBook={pictureBook} />
    : <ChaptersWorkspace projectUuid={projectUuid} pictureBook={pictureBook} />
}

function ProjectWorkspaceNotFound({ projectUuid }) {
  const { t } = useI18n()
  return (
    <div className="workspace-empty">
      <h1>{t('common.not_found.page_title')}</h1>
      <p>{t('errors.not_found')}</p>
      <Link className="button-link" to={projectRoute(projectUuid)}>{t('simple.not_found.body')}</Link>
    </div>
  )
}
