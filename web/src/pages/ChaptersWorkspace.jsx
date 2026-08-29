import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import {
  ArrowDown,
  ArrowRight,
  ArrowUp,
  ChevronDown,
  Files,
  Keyboard,
  MoreHorizontal,
  Plus,
  Sparkles,
  Trash2,
  Upload,
  X,
} from 'lucide-react'

import { createChapterBatch, createChapterGeneration, getActiveProvider, listTasks } from '../api/ai.js'
import {
  createChapter,
  getStoryProfile,
  importChapters,
  listChapters,
  trashChapter,
} from '../api/story.js'
import { ACTIVE_TASK_STATUSES } from './aiRuntimeState.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import {
  CHAPTER_CREATION_ACTIONS,
  chapterCodePattern,
  chapterContinuationContext,
  chapterGenerationPlan,
  isSupportedStoryFile,
  nextChapterCode,
  sortChaptersByDirection,
} from './storyWorkspaceState.js'
import { formatTerminologyMessageKey } from './pictureBookProfile.js'

const TASK_STATUS = {
  queued: 'story.task.status.queued',
  running: 'story.task.status.running',
  waiting_for_input: 'story.task.status.waiting_for_input',
  completed: 'common.status.ready',
  failed: 'story.task.status.failed',
  cancelled: 'common.status.cancelled',
  interrupted: 'common.status.interrupted',
}

const CREATION_ICON = {
  batch: Files,
  next: Sparkles,
  continue: Sparkles,
  manual: Keyboard,
  upload: Upload,
}

const DIALOG_TITLE = {
  batch: 'story.chapters.create.batch',
  next: 'story.chapters.create.next',
  continue: 'story.chapters.create.continue',
  manual: 'story.chapters.create.manual',
  upload: 'story.chapters.create.upload',
}

export default function ChaptersWorkspace({ projectUuid, pictureBook }) {
  const { t } = useI18n()
  const term = (key, values) => t(formatTerminologyMessageKey(pictureBook, key), values)
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const createMenuRef = useRef(null)
  const createTriggerRef = useRef(null)
  const rowMenuRef = useRef(null)
  const rowMenuTriggerRef = useRef(null)
  const deleteReturnFocusRef = useRef(null)
  const [sortDirection, setSortDirection] = useState('asc')
  const [createMenuOpen, setCreateMenuOpen] = useState(false)
  const [openRowMenuUuid, setOpenRowMenuUuid] = useState('')
  const [activeDialog, setActiveDialog] = useState('')
  const [batchTab, setBatchTab] = useState('ai')
  const [chapterCode, setChapterCode] = useState('')
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [contentFormat, setContentFormat] = useState('txt')
  const [prompt, setPrompt] = useState('')
  const [chapterCount, setChapterCount] = useState(3)
  const [singleFile, setSingleFile] = useState(null)
  const [batchFiles, setBatchFiles] = useState([])
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [error, setError] = useState(null)
  const [summary, setSummary] = useState(null)
  const [nextStepDismissed, setNextStepDismissed] = useState(() => readNextStepDismissed(projectUuid))

  const chaptersQuery = useQuery({
    queryKey: ['story-chapters', projectUuid, 'active'],
    queryFn: () => listChapters(projectUuid, 'active'),
  })
  const profileQuery = useQuery({
    queryKey: ['story-profile', projectUuid],
    queryFn: () => getStoryProfile(projectUuid),
  })
  const tasksQuery = useQuery({
    queryKey: ['story-tasks', projectUuid],
    queryFn: () => listTasks(projectUuid, { limit: 100 }),
  })

  const items = chaptersQuery.data?.items || []
  const sortedItems = useMemo(() => sortChaptersByDirection(items, sortDirection), [items, sortDirection])
  const continuation = useMemo(() => chapterContinuationContext(items), [items])
  const suggestedCode = useMemo(() => nextChapterCode(items), [items])
  const storyTasks = useMemo(
    () => (tasksQuery.data?.items || []).filter((task) => task.kind === 'story_chapter_generation'),
    [tasksQuery.data?.items],
  )
  const taskByChapter = useMemo(() => {
    const byChapter = new Map()
    storyTasks.forEach((task) => {
      if (!byChapter.has(task.resource_uuid)) byChapter.set(task.resource_uuid, task)
    })
    return byChapter
  }, [storyTasks])
  const processing = storyTasks.some((task) => ACTIVE_TASK_STATUSES.has(task.status))
  const singleFileError = singleFile && !isSupportedStoryFile(singleFile) ? term('story.chapters.invalid_file', { name: singleFile.name }) : ''
  const unsupportedBatchFile = batchFiles.find((file) => !isSupportedStoryFile(file))
  const queryError = chaptersQuery.error || tasksQuery.error
  const base = `/projects/${encodeURIComponent(projectUuid || '')}`

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['story-chapters', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['story-project', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['chat-threads', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['workflows', projectUuid] })
  }, [projectUuid, queryClient])

  useEffect(() => {
    setNextStepDismissed(readNextStepDismissed(projectUuid))
  }, [projectUuid])

  useEffect(() => {
    if (!createMenuOpen) return undefined
    const onPointerDown = (event) => {
      if (!createMenuRef.current?.contains(event.target)) setCreateMenuOpen(false)
    }
    const onKeyDown = (event) => {
      if (event.key !== 'Escape') return
      setCreateMenuOpen(false)
      createTriggerRef.current?.focus()
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [createMenuOpen])

  useEffect(() => {
    if (!openRowMenuUuid) return undefined
    const onPointerDown = (event) => {
      if (!rowMenuRef.current?.contains(event.target)) setOpenRowMenuUuid('')
    }
    const onKeyDown = (event) => {
      if (event.key !== 'Escape') return
      setOpenRowMenuUuid('')
      rowMenuTriggerRef.current?.focus()
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [openRowMenuUuid])

  const manualMutation = useMutation({
    mutationFn: () => createChapter(projectUuid, {
      chapter_code: chapterCode.trim().toLowerCase(),
      title: title.trim(),
      content,
      content_format: contentFormat,
    }),
    onSuccess: (chapter) => {
      refresh()
      setError(null)
      setActiveDialog('')
      navigate({ pathname: `${base}/chapters/${chapter.uuid}`, search: location.search })
    },
    onError: setError,
  })

  const importMutation = useMutation({
    mutationFn: ({ mode, files }) => importChapters(projectUuid, {
      mode,
      files,
      chapterCode: mode === 'single' ? chapterCode.trim().toLowerCase() : '',
      title: mode === 'single' ? title.trim() : '',
    }),
    onSuccess: (result, variables) => {
      refresh()
      setError(null)
      setSummary({ type: 'import', result })
      setActiveDialog('')
      if (variables.mode === 'single' && result.items?.length === 1) {
        navigate({ pathname: `${base}/chapters/${result.items[0].uuid}`, search: location.search })
      }
    },
    onError: setError,
  })

  const generationMutation = useMutation({
    mutationFn: async ({ mode, requestedCount }) => {
      await getActiveProvider()
      if (mode === 'batch') {
        const task = await createChapterBatch(projectUuid, {
          prompt: prompt.trim(),
          chapter_count: requestedCount,
          parameters: { temperature: 0.7 },
          idempotency_key: `chapter-batch-${Date.now()}`,
        })
        return { created: [], tasks: [task] }
      }
      const plan = chapterGenerationPlan({
        mode,
        chapters: items,
        prompt,
        count: requestedCount,
        storyMd: profileQuery.data?.story_md || '',
      })
      const created = []
      const tasks = []
      try {
        for (const step of plan) {
          const chapter = await createChapter(projectUuid, {
            chapter_code: step.chapterCode,
            title: step.title,
            content: '',
            content_format: '',
          })
          created.push(chapter)
          const task = await createChapterGeneration(projectUuid, chapter.uuid, {
            prompt_key: mode === 'next' || mode === 'continue' ? 'next_story_chapter' : 'story_chapter',
            prompt: step.prompt,
            parameters: { temperature: 0.7 },
            idempotency_key: `chapter-creation-${mode}-${Date.now()}-${chapter.uuid}`,
          })
          tasks.push(task)
        }
      } catch (cause) {
        const details = created.length
          ? term('story.chapters.partial_generation', { created: created.length, queued: tasks.length })
          : cause?.details
        const wrapped = new Error(term('story.chapters.generation_failed'))
        wrapped.code = cause?.code
        wrapped.details = details
        throw wrapped
      }
      return { created, tasks }
    },
    onSuccess: ({ tasks }) => {
      queryClient.setQueryData(['story-tasks', projectUuid], (current) => ({
        items: [...tasks, ...(current?.items || []).filter((item) => !tasks.some((task) => task.uuid === item.uuid))],
      }))
      refresh()
      setError(null)
      setSummary({ type: 'generation', count: tasks.length })
      setActiveDialog('')
    },
    onError: (mutationError) => { setError(mutationError); refresh() },
  })

  const deleteMutation = useMutation({
    mutationFn: (chapter) => trashChapter(projectUuid, chapter.uuid, chapter.revision),
    onSuccess: () => {
      refresh()
      setDeleteTarget(null)
      setDeleteConfirmation('')
      setError(null)
    },
    onError: setError,
  })

  const busy = manualMutation.isPending || importMutation.isPending || generationMutation.isPending || deleteMutation.isPending

  function openDialog(kind) {
    if (busy || processing) return
    manualMutation.reset()
    importMutation.reset()
    generationMutation.reset()
    setError(null)
    setChapterCode(suggestedCode)
    setTitle('')
    setContent('')
    setContentFormat('txt')
    setPrompt('')
    setChapterCount(3)
    setSingleFile(null)
    setBatchFiles([])
    setBatchTab('ai')
    setCreateMenuOpen(false)
    setActiveDialog(kind)
  }

  function closeDialog() {
    if (!busy) setActiveDialog('')
  }

  function requestDelete(chapter) {
    deleteReturnFocusRef.current = rowMenuTriggerRef.current
    setOpenRowMenuUuid('')
    setDeleteConfirmation('')
    setDeleteTarget(chapter)
  }

  function dismissNextStep() {
    writeNextStepDismissed(projectUuid)
    setNextStepDismissed(true)
  }

  const showNextStep = !nextStepDismissed && items.some((chapter) => Boolean(chapter.current_story))

  return (
    <div className={`chapters-page${showNextStep ? ' chapters-page--with-next-step' : ''}`}>
      <section className="chapters-panel" aria-labelledby="chapters-title">
        <header className="chapters-toolbar">
          <h1 id="chapters-title">{term('story.chapters')}</h1>
          <div className="chapters-toolbar__actions">
            <span className="chapters-count" aria-label={term('story.chapters.count_label', { count: items.length })}>{items.length}</span>
            <button
              type="button"
              className="button-secondary chapters-compact-button"
              aria-label={t(sortDirection === 'asc' ? 'story.chapters.sort_desc' : 'story.chapters.sort_asc')}
              title={t(sortDirection === 'asc' ? 'story.chapters.sort_desc' : 'story.chapters.sort_asc')}
              onClick={() => setSortDirection((current) => current === 'asc' ? 'desc' : 'asc')}
            >
              {sortDirection === 'asc' ? <ArrowUp size={14} aria-hidden="true" /> : <ArrowDown size={14} aria-hidden="true" />}
              {t(sortDirection === 'asc' ? 'story.chapters.order_asc' : 'story.chapters.order_desc')}
            </button>
            <Link className="button-secondary chapters-compact-button" to={{ pathname: `${base}/trash`, search: location.search }}>
              <Trash2 size={14} aria-hidden="true" />
              {t('projects.tab.trash')}
            </Link>
            <div className="chapter-create-menu" ref={createMenuRef}>
              <button
                ref={createTriggerRef}
                type="button"
                className="button-secondary chapters-compact-button chapter-create-menu__trigger"
                aria-haspopup="menu"
                aria-expanded={createMenuOpen}
                disabled={busy || processing}
                title={term(processing ? 'story.chapters.add_wait' : 'story.chapters.add')}
                onClick={() => setCreateMenuOpen((current) => !current)}
              >
                <Plus size={14} aria-hidden="true" />
                {term('story.chapters.add')}
                <ChevronDown size={14} aria-hidden="true" />
              </button>
              {createMenuOpen ? (
                <div className="chapter-create-menu__dropdown" role="menu">
                  {CHAPTER_CREATION_ACTIONS.map((action) => {
                    const Icon = CREATION_ICON[action.key]
                    return <button className="chapter-menu-item" key={action.key} type="button" role="menuitem" onClick={() => openDialog(action.key)}><Icon size={16} aria-hidden="true" /><span>{term(action.labelKey)}</span></button>
                  })}
                </div>
              ) : null}
            </div>
          </div>
        </header>

        {summary ? <OperationSummary summary={summary} onDismiss={() => setSummary(null)} term={term} /> : null}
        {(error || queryError) && !activeDialog && !deleteTarget ? <ErrorNotice error={error || queryError} onDismiss={error ? () => setError(null) : undefined} /> : null}
        {chaptersQuery.isLoading ? <p className="chapters-loading">{term('story.chapter.loading')}</p> : null}
        {!chaptersQuery.isLoading && !chaptersQuery.isError && items.length === 0 ? <p className="chapters-empty">{term('story.chapters.empty')}</p> : null}
        <div className="chapters-list" aria-label={term('story.chapters.list')}>
          {sortedItems.map((chapter) => {
            const task = taskByChapter.get(chapter.uuid)
            const active = task && ACTIVE_TASK_STATUSES.has(task.status)
            const status = task ? t(TASK_STATUS[task.status] || 'common.status.unknown_with_code', { code: task.status }) : t(chapter.current_story ? 'common.status.ready' : 'common.status.blank')
            return (
              <article className="chapters-list-item" key={chapter.uuid}>
                <Link className="chapters-list-item__link" to={{ pathname: `${base}/chapters/${chapter.uuid}`, search: location.search }}>
                  <strong>{chapter.chapter_code}</strong>
                  <span className={`chapter-status chapter-status--${task?.status || (chapter.current_story ? 'ready' : 'empty')}`}>{status}</span>
                </Link>
                <div className="chapters-row-actions" ref={openRowMenuUuid === chapter.uuid ? rowMenuRef : undefined}>
                  <button
                    type="button"
                    className="chapters-more-button"
                    aria-label={t('story.chapters.more', { code: chapter.chapter_code })}
                    aria-haspopup="menu"
                    aria-expanded={openRowMenuUuid === chapter.uuid}
                    disabled={active || deleteMutation.isPending}
                    title={term(active ? 'story.chapters.delete_disabled' : 'common.label.actions')}
                    onClick={(event) => {
                      rowMenuTriggerRef.current = event.currentTarget
                      setOpenRowMenuUuid((current) => current === chapter.uuid ? '' : chapter.uuid)
                    }}
                  >
                    <MoreHorizontal size={18} aria-hidden="true" />
                  </button>
                  {openRowMenuUuid === chapter.uuid ? (
                    <div className="chapters-row-menu" role="menu">
                      <button className="chapter-menu-item" type="button" role="menuitem" onClick={() => requestDelete(chapter)}><Trash2 size={14} aria-hidden="true" /><span>{t('common.action.delete')}</span></button>
                    </div>
                  ) : null}
                </div>
              </article>
            )
          })}
        </div>
      </section>

      {showNextStep ? (
        <div className="chapters-next-step">
          <Link to={{ pathname: `${base}/assets`, search: location.search }}><span>{t('story.chapters.next_step')}</span><ArrowRight size={18} aria-hidden="true" /></Link>
          <button className="chapters-next-step__dismiss" type="button" aria-label={t('story.chapters.next_step_close')} title={t('story.chapters.next_step_close')} onClick={dismissNextStep}><X size={17} aria-hidden="true" /></button>
        </div>
      ) : null}

      {activeDialog ? (
        <AccessibleDialog title={term(DIALOG_TITLE[activeDialog])} eyebrow={term('story.chapter')} onClose={closeDialog} busy={busy} returnFocusRef={createTriggerRef}>
          {activeDialog === 'batch' ? (
            <>
              <div className="chapter-dialog-tabs" role="tablist" aria-label={t('story.chapters.batch_method')}>
                <button className="chapter-dialog-tab" type="button" role="tab" aria-selected={batchTab === 'ai'} onClick={() => { setBatchTab('ai'); setError(null) }} disabled={busy}>{t('story.chapters.ai_generate')}</button>
                <button className="chapter-dialog-tab" type="button" role="tab" aria-selected={batchTab === 'upload'} onClick={() => { setBatchTab('upload'); setError(null) }} disabled={busy}>{t('story.chapters.create.upload')}</button>
              </div>
              {batchTab === 'ai' ? (
                <form className="chapter-dialog-form" onSubmit={(event) => { event.preventDefault(); generationMutation.mutate({ mode: 'batch', requestedCount: Number(chapterCount) }) }}>
                  <label>{t('projects.tab.prompts')}<textarea data-autofocus value={prompt} maxLength={30000} rows="8" required disabled={busy} onChange={(event) => { setPrompt(event.target.value); setError(null) }} /></label>
                  <label>{term('story.chapters.count')}<input type="number" min="1" max="10" value={chapterCount} required disabled={busy} onChange={(event) => setChapterCount(event.target.value)} /><small>{term('story.chapters.count_hint')}</small></label>
                  <ErrorNotice error={error} />
                  <DialogActions busy={busy} submitDisabled={!prompt.trim() || Number(chapterCount) < 1 || Number(chapterCount) > 10} pendingLabel={t('story.chapters.creating_queue')} submitLabel={t('story.chapters.start_generation')} onCancel={closeDialog} />
                </form>
              ) : (
                <form className="chapter-dialog-form" onSubmit={(event) => { event.preventDefault(); importMutation.mutate({ mode: 'batch', files: batchFiles }) }}>
                  <label>{t('story.chapters.files')}<input data-autofocus type="file" accept=".txt,.md,text/plain,text/markdown" multiple required disabled={busy} onChange={(event) => { setBatchFiles(Array.from(event.target.files || [])); setError(null) }} /><small>{term('story.chapters.files_hint')}</small></label>
                  <FileList files={batchFiles} />
                  {unsupportedBatchFile ? <p className="chapter-field-error">{t('story.chapters.invalid_file', { name: unsupportedBatchFile.name })}</p> : null}
                  <ErrorNotice error={error} />
                  <DialogActions busy={busy} submitDisabled={!batchFiles.length || Boolean(unsupportedBatchFile)} pendingLabel={t('story.chapters.uploading')} submitLabel={term('story.chapters.import')} onCancel={closeDialog} />
                </form>
              )}
            </>
          ) : null}

          {activeDialog === 'next' ? (
            <form className="chapter-dialog-form" onSubmit={(event) => { event.preventDefault(); generationMutation.mutate({ mode: 'next', requestedCount: 1 }) }}>
              <p className="chapter-dialog-hint">{term('story.chapters.next_hint', { code: suggestedCode })}</p>
              <label>{t('projects.tab.prompts')}<textarea data-autofocus value={prompt} maxLength={30000} rows="8" required disabled={busy} onChange={(event) => { setPrompt(event.target.value); setError(null) }} /></label>
              <ErrorNotice error={error} />
              <DialogActions busy={busy} submitDisabled={!prompt.trim()} pendingLabel={t('story.chapters.creating_queue')} submitLabel={t('story.chapters.start_generation')} onCancel={closeDialog} />
            </form>
          ) : null}

          {activeDialog === 'continue' ? (
            <form className="chapter-dialog-form" onSubmit={(event) => { event.preventDefault(); generationMutation.mutate({ mode: 'continue', requestedCount: 1 }) }}>
              <label>{term('story.chapters.source_chapter')}<input value={continuation.sourceChapter ? `${continuation.sourceChapter.chapter_code}${continuation.sourceChapter.title ? ` · ${continuation.sourceChapter.title}` : ''}` : term('story.chapters.none')} readOnly /></label>
              <label>{term('story.chapters.target_chapter')}<input value={continuation.targetChapterCode} readOnly /></label>
              <label>{t('story.chapters.continue_requirements')}<textarea data-autofocus value={prompt} maxLength={30000} rows="8" disabled={busy} onChange={(event) => { setPrompt(event.target.value); setError(null) }} /><small>{term('story.chapters.continue_hint')}</small></label>
              {!continuation.hasCurrentStory ? <p className="chapter-field-error">{term('story.chapters.continue_missing')}</p> : null}
              <ErrorNotice error={error} />
              <DialogActions busy={busy} submitDisabled={!continuation.hasCurrentStory} pendingLabel={t('story.chapters.creating_queue')} submitLabel={t('story.chapters.start_continue')} onCancel={closeDialog} />
            </form>
          ) : null}

          {activeDialog === 'manual' ? (
            <form className="chapter-dialog-form" onSubmit={(event) => { event.preventDefault(); manualMutation.mutate() }}>
              <ChapterIdentityFields chapterCode={chapterCode} setChapterCode={setChapterCode} title={title} setTitle={setTitle} busy={busy} codeLabel={term('story.chapters.code')} autoFocus />
              <label>{t('story.chapters.format')}<select value={contentFormat} disabled={busy} onChange={(event) => setContentFormat(event.target.value)}><option value="txt">txt</option><option value="md">md</option></select></label>
              <label>{t('story.chapters.content')}<textarea value={content} rows="12" required disabled={busy} onChange={(event) => { setContent(event.target.value); setError(null) }} /></label>
              <ErrorNotice error={error} />
              <DialogActions busy={busy} submitDisabled={!chapterCodePattern.test(chapterCode.trim()) || !content.trim()} pendingLabel={t('common.status.saving')} submitLabel={t('common.action.save')} onCancel={closeDialog} />
            </form>
          ) : null}

          {activeDialog === 'upload' ? (
            <form className="chapter-dialog-form" onSubmit={(event) => { event.preventDefault(); importMutation.mutate({ mode: 'single', files: singleFile ? [singleFile] : [] }) }}>
              <ChapterIdentityFields chapterCode={chapterCode} setChapterCode={setChapterCode} title={title} setTitle={setTitle} busy={busy} codeLabel={term('story.chapters.code')} autoFocus />
              <label>{t('story.chapters.files')}<input type="file" accept=".txt,.md,text/plain,text/markdown" required disabled={busy} onChange={(event) => { setSingleFile(event.target.files?.[0] || null); setError(null) }} /></label>
              {singleFileError ? <p className="chapter-field-error">{singleFileError}</p> : null}
              <ErrorNotice error={error} />
              <DialogActions busy={busy} submitDisabled={!chapterCodePattern.test(chapterCode.trim()) || !singleFile || Boolean(singleFileError)} pendingLabel={t('story.chapters.uploading')} submitLabel={term('story.chapters.import')} onCancel={closeDialog} />
            </form>
          ) : null}
        </AccessibleDialog>
      ) : null}

      {deleteTarget ? (
        <AccessibleDialog title={term('story.chapters.delete_title')} eyebrow={term('story.chapter')} onClose={() => { if (!deleteMutation.isPending) setDeleteTarget(null) }} busy={deleteMutation.isPending} returnFocusRef={deleteReturnFocusRef} compact>
          <form className="chapter-dialog-form" onSubmit={(event) => { event.preventDefault(); if (deleteConfirmation === t('story.chapters.delete_word')) deleteMutation.mutate(deleteTarget) }}>
            <p className="chapter-dialog-hint">{term('story.chapters.delete_hint', { code: deleteTarget.chapter_code })}</p>
            <label>{t('story.chapters.delete_confirm', { word: t('story.chapters.delete_word') })}<input data-autofocus value={deleteConfirmation} autoComplete="off" disabled={deleteMutation.isPending} onChange={(event) => { setDeleteConfirmation(event.target.value); setError(null) }} /></label>
            <ErrorNotice error={error} />
            <DialogActions danger busy={deleteMutation.isPending} submitDisabled={deleteConfirmation !== t('story.chapters.delete_word')} pendingLabel={t('story.chapters.deleting')} submitLabel={t('common.action.delete')} onCancel={() => setDeleteTarget(null)} />
          </form>
        </AccessibleDialog>
      ) : null}
    </div>
  )
}

function ChapterIdentityFields({ chapterCode, setChapterCode, title, setTitle, busy, codeLabel, autoFocus = false }) {
  const { t } = useI18n()
  return (
    <>
      <label>{codeLabel}<input data-autofocus={autoFocus || undefined} value={chapterCode} pattern="vol[0-9]{2,}\.ch[0-9]{2,}" required disabled={busy} onChange={(event) => setChapterCode(event.target.value.toLowerCase())} /></label>
      <label>{t('common.label.title')}<input value={title} maxLength="255" placeholder={t('common.label.optional')} disabled={busy} onChange={(event) => setTitle(event.target.value)} /></label>
    </>
  )
}

function DialogActions({ busy, submitDisabled, pendingLabel, submitLabel, onCancel, danger = false }) {
  const { t } = useI18n()
  return (
    <div className="chapter-dialog-actions">
      <button type="button" className="button-secondary" disabled={busy} onClick={onCancel}>{t('common.action.cancel')}</button>
      <button type="submit" className={danger ? 'story-button story-button--danger' : 'story-button'} disabled={busy || submitDisabled}>{busy ? pendingLabel : submitLabel}</button>
    </div>
  )
}

function AccessibleDialog({ title, eyebrow, onClose, busy, returnFocusRef, children, compact = false }) {
  const { t } = useI18n()
  const dialogRef = useRef(null)
  const closeRef = useRef(onClose)
  const busyRef = useRef(busy)
  closeRef.current = onClose
  busyRef.current = busy

  useEffect(() => {
    const fallbackFocus = document.activeElement
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const frame = window.requestAnimationFrame(() => {
      const preferred = dialogRef.current?.querySelector('[data-autofocus]')
      const first = focusableElements(dialogRef.current)[0]
      ;(preferred || first)?.focus()
    })
    const onKeyDown = (event) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        if (!busyRef.current) closeRef.current()
        return
      }
      if (event.key !== 'Tab' || !dialogRef.current) return
      const focusable = focusableElements(dialogRef.current)
      if (!focusable.length) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && (document.activeElement === first || !dialogRef.current.contains(document.activeElement))) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      window.cancelAnimationFrame(frame)
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', onKeyDown)
      const target = returnFocusRef?.current || fallbackFocus
      if (target instanceof HTMLElement && target.isConnected) target.focus()
    }
  }, [returnFocusRef])

  return (
    <div className="chapter-dialog-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) onClose() }}>
      <section ref={dialogRef} className={`chapter-dialog${compact ? ' chapter-dialog--compact' : ''}`} role="dialog" aria-modal="true" aria-labelledby="chapter-dialog-title">
        <header><div><p>{eyebrow}</p><h2 id="chapter-dialog-title">{title}</h2></div><button type="button" className="chapter-dialog__close" aria-label={t('common.action.close')} disabled={busy} onClick={onClose}><X size={18} aria-hidden="true" /></button></header>
        <div className="chapter-dialog__body">{children}</div>
      </section>
    </div>
  )
}

function focusableElements(container) {
  if (!container) return []
  return Array.from(container.querySelectorAll('button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'))
}

function FileList({ files }) {
  const { formatNumber } = useI18n()
  if (!files.length) return null
  return <ul className="chapter-file-list">{files.map((file, index) => <li key={`${file.name}-${file.size}-${index}`}><span>{file.name}</span><small>{formatNumber(Math.max(1, Math.ceil(file.size / 1024)))} KB</small></li>)}</ul>
}

function ErrorNotice({ error, onDismiss }) {
  return <LocalizedErrorMessage error={error} className="chapter-error" onDismiss={onDismiss} />
}

function OperationSummary({ summary, onDismiss, term }) {
  const { t } = useI18n()
  const result = summary.result
  const created = result?.items?.length || 0
  const skipped = result?.skipped?.length || 0
  return (
    <div className="chapter-operation-summary" role="status">
      <div>
        <strong>{summary.type === 'generation' ? term('story.chapters.operation_generation', { count: summary.count }) : term('story.chapters.operation_import', { created, skipped })}</strong>
        {summary.type === 'import' && skipped ? <ul>{result.skipped.map((item, index) => <li key={`${item.filename}-${index}`}><span>{item.chapter_code || item.filename}</span><small>{t(item.reason === 'duplicate_code' ? 'story.chapters.skip_duplicate' : 'story.chapters.skip_exists')}</small></li>)}</ul> : null}
      </div>
      <button className="chapter-notice-dismiss" type="button" aria-label={t('story.chapters.close_result')} onClick={onDismiss}><X size={15} aria-hidden="true" /></button>
    </div>
  )
}

function nextStepStorageKey(projectUuid) {
  return `lumi.project.${projectUuid}.chaptersNextStepDismissed`
}

function readNextStepDismissed(projectUuid) {
  if (typeof window === 'undefined' || !projectUuid) return false
  try { return window.localStorage.getItem(nextStepStorageKey(projectUuid)) === 'true' } catch { return false }
}

function writeNextStepDismissed(projectUuid) {
  if (typeof window === 'undefined' || !projectUuid) return
  try { window.localStorage.setItem(nextStepStorageKey(projectUuid), 'true') } catch { /* restricted browser */ }
}
