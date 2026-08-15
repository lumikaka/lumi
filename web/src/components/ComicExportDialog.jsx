import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, Download, FileText, X } from 'lucide-react'

import {
  cancelProductionTask,
  createComicExport,
  getComicExportReadiness,
  getProductionTask,
  listComicExports,
  retryProductionTask,
} from '../api/production.js'
import { getStoryProject } from '../api/story.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { statusLabel } from '../i18n/labels.js'
import { useI18n } from '../i18n/useI18n.js'
import {
  activeComicExportStatuses,
  comicExportOperationState,
  comicExportReadinessDecision,
  comicExportSnapshotMetrics,
  retryableComicExportStatuses,
} from '../pages/comicExportState.js'
import LumiDialog from './LumiDialog.jsx'

export default function ComicExportDialog({ projectUuid, request, onClose }) {
  const { formatDateTime, formatNumber, t } = useI18n()
  const queryClient = useQueryClient()
  const projectQuery = useQuery({ queryKey: ['story-project', projectUuid], queryFn: () => getStoryProject(projectUuid) })
  const [stage, setStage] = useState('checking')
  const [readiness, setReadiness] = useState(null)
  const [operation, setOperation] = useState(null)
  const [selectedFormat, setSelectedFormat] = useState('')
  const [error, setError] = useState(null)
  const allowMissingRef = useRef(false)
  const selectedFormatRef = useRef('')
  const scope = request.scope
  const pageMode = Boolean(projectQuery.data?.picture_book?.format && projectQuery.data.picture_book.format !== 'vertical_strip')
  const chapterUuid = scope === 'chapter' ? request.chapterUuid : ''
  const taskUuid = operation?.task?.uuid || operation?.export?.task_uuid || ''
  const snapshotHash = operation?.export?.snapshot_hash || ''
  const taskQueryKey = useMemo(() => ['production-task', projectUuid, taskUuid], [projectUuid, taskUuid])
  const exportQueryKey = useMemo(() => ['comic-export-operation', projectUuid, taskUuid, snapshotHash], [projectUuid, snapshotHash, taskUuid])

  const refreshHistory = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['comic-exports', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['production-tasks', projectUuid] })
  }, [projectUuid, queryClient])

  const adoptOperation = useCallback((next) => {
    setOperation(next)
    const format = next?.export?.format || 'zip'
    setSelectedFormat(format)
    selectedFormatRef.current = format
    setStage('operation')
    setError(null)
    if (next?.task?.uuid) queryClient.setQueryData(['production-task', projectUuid, next.task.uuid], next.task)
    if (next?.task?.uuid && next?.export?.snapshot_hash) {
      queryClient.setQueryData(['comic-export-operation', projectUuid, next.task.uuid, next.export.snapshot_hash], next.export)
    }
    refreshHistory()
  }, [projectUuid, queryClient, refreshHistory])

  const loadActiveOperation = useCallback(async () => {
    const latest = await listComicExports(projectUuid, { page: 1, perPage: 1, scope, chapterUuid })
    const activeExport = latest.items?.find((item) => activeComicExportStatuses.has(item.status))
    if (!activeExport) return null
    const task = await getProductionTask(projectUuid, activeExport.task_uuid)
    return { export: activeExport, task }
  }, [chapterUuid, projectUuid, scope])

  const startExport = useCallback(async (allowMissingImages) => {
    const format = selectedFormatRef.current
    if (!format) return
    allowMissingRef.current = allowMissingImages
    setStage('creating')
    setError(null)
    try {
      const next = await createComicExport(projectUuid, {
        scope,
        chapter_uuid: chapterUuid,
        format,
        allow_missing_images: allowMissingImages,
        idempotency_key: request.idempotencyKey,
      })
      adoptOperation(next)
    } catch (nextError) {
      if (nextError?.code === 'task_conflict') {
        try {
          const activeOperation = await loadActiveOperation()
          if (activeOperation) {
            adoptOperation(activeOperation)
            return
          }
        } catch {
          // Keep the original creation error when active-task recovery fails.
        }
      }
      setError(nextError)
      setStage('create_failed')
    }
  }, [adoptOperation, chapterUuid, loadActiveOperation, projectUuid, request.idempotencyKey, scope])

  useEffect(() => {
    let ignored = false
    const initialize = async () => {
      try {
        const activeOperation = await loadActiveOperation()
        if (ignored) return
        if (activeOperation) {
          adoptOperation(activeOperation)
          return
        }
        const nextReadiness = await getComicExportReadiness(projectUuid, { scope, chapterUuid })
        if (ignored) return
        setReadiness(nextReadiness)
        const decision = comicExportReadinessDecision(nextReadiness)
        if (decision === 'blocked') setStage('blocked')
        else if (decision === 'confirm') setStage('confirm')
        else setStage('select')
      } catch (nextError) {
        if (!ignored) {
          setError(nextError)
          setStage('check_failed')
        }
      }
    }
    void initialize()
    return () => { ignored = true }
  }, [adoptOperation, chapterUuid, loadActiveOperation, projectUuid, scope])

  const taskQuery = useQuery({
    queryKey: taskQueryKey,
    queryFn: () => getProductionTask(projectUuid, taskUuid),
    enabled: Boolean(taskUuid),
    initialData: operation?.task,
  })
  const task = taskQuery.data || operation?.task

  const exportQuery = useQuery({
    queryKey: exportQueryKey,
    queryFn: async () => {
      const operationFormat = operation?.export?.format || selectedFormatRef.current
      const exact = await listComicExports(projectUuid, { page: 1, perPage: 1, taskUuid, format: operationFormat })
      if (exact.items?.[0]) return exact.items[0]
      if (snapshotHash) {
        const canonical = await listComicExports(projectUuid, { page: 1, perPage: 1, snapshotHash, format: operationFormat, status: 'ready' })
        if (canonical.items?.[0]) return canonical.items[0]
      }
      return operation?.export || null
    },
    enabled: Boolean(taskUuid && snapshotHash),
    initialData: operation?.export,
  })
  const exportRecord = exportQuery.data || operation?.export
  const operationState = comicExportOperationState(task, exportRecord)
  const displayState = operationState === 'finalizing' ? 'processing' : operationState
  const metrics = comicExportSnapshotMetrics(readiness, exportRecord)
  const progress = operationState === 'ready' ? 100 : Number(task?.progress || 0)
  const operationError = task?.error_message ? { code: task.error_code, message: task.error_message } : null

  useEffect(() => {
    if (!taskUuid || !['completed', 'failed', 'interrupted', 'cancelled'].includes(task?.status)) return
    queryClient.invalidateQueries({ queryKey: exportQueryKey })
    refreshHistory()
  }, [exportQueryKey, queryClient, refreshHistory, task?.status, taskUuid])

  const cancelMutation = useMutation({
    mutationFn: () => cancelProductionTask(projectUuid, taskUuid),
    onSuccess: (nextTask) => {
      queryClient.setQueryData(taskQueryKey, nextTask)
      queryClient.invalidateQueries({ queryKey: exportQueryKey })
      refreshHistory()
      setError(null)
    },
    onError: setError,
  })
  const retryMutation = useMutation({
    mutationFn: () => retryProductionTask(projectUuid, taskUuid),
    onSuccess: (nextTask) => {
      queryClient.setQueryData(taskQueryKey, nextTask)
      queryClient.invalidateQueries({ queryKey: exportQueryKey })
      refreshHistory()
      setError(null)
    },
    onError: setError,
  })

  const title = t(pageMode
    ? (scope === 'chapter' ? 'projects.exports.dialog.chapter_title_pages' : 'projects.exports.dialog.project_title_pages')
    : (scope === 'chapter' ? 'projects.exports.dialog.chapter_title' : 'projects.exports.dialog.project_title'))
  const scopeLabel = scope === 'chapter' ? request.chapterLabel || t(pageMode ? 'projects.exports.chapter_label_pages' : 'projects.exports.chapter_label') : t(pageMode ? 'projects.exports.project_label_pages' : 'projects.exports.project_label')
  const filename = exportRecord?.filename || t('projects.exports.dialog.pending_filename')
  const busy = cancelMutation.isPending || retryMutation.isPending
  const active = activeComicExportStatuses.has(operationState)
  const retryable = retryableComicExportStatuses.has(operationState)
  const ready = operationState === 'ready' && exportRecord?.download_url
  const chooseFormat = (format) => {
    setSelectedFormat(format)
    selectedFormatRef.current = format
  }
  const formatSelector = (
    <fieldset className="comic-export-dialog__formats">
      <legend>{t('projects.exports.dialog.format_title')}</legend>
      <p>{t('projects.exports.dialog.format_hint')}</p>
      <div role="radiogroup" aria-label={t('projects.exports.dialog.format_title')}>
        <button type="button" role="radio" className="comic-export-dialog__format-option" aria-checked={selectedFormat === 'zip'} aria-pressed={selectedFormat === 'zip'} onClick={() => chooseFormat('zip')}>
          <Archive size={22} aria-hidden="true" />
          <span><strong>{t('projects.exports.dialog.format_zip')}</strong><small>{t('projects.exports.dialog.format_zip_body')}</small></span>
        </button>
        <button type="button" role="radio" className="comic-export-dialog__format-option" aria-checked={selectedFormat === 'pdf'} aria-pressed={selectedFormat === 'pdf'} onClick={() => chooseFormat('pdf')}>
          <FileText size={22} aria-hidden="true" />
          <span><strong>{t('projects.exports.dialog.format_pdf')}</strong><small>{t('projects.exports.dialog.format_pdf_body')}</small></span>
        </button>
      </div>
    </fieldset>
  )

  return (
    <LumiDialog className="comic-export-dialog" onClose={onClose} aria-labelledby="comic-export-dialog-title">
      <header className="lumi-dialog__header">
        <div><p className="eyebrow">{t('projects.exports.dialog.eyebrow')}</p><h2 id="comic-export-dialog-title">{title}</h2><p>{scopeLabel}</p></div>
        <button type="button" className="button-quiet" aria-label={t('common.action.close')} onClick={onClose}><X size={18} aria-hidden="true" /></button>
      </header>
      <div className="lumi-dialog__body comic-export-dialog__body">
        {stage === 'checking' || stage === 'creating' ? <div className="comic-export-dialog__pending" role="status"><strong>{t(stage === 'checking' ? 'projects.exports.preflight' : 'projects.exports.create_task')}</strong><progress aria-label={t('projects.exports.dialog.progress')} /></div> : null}
        {stage === 'blocked' ? <div className="comic-export-dialog__empty" role="alert"><strong>{t('projects.exports.dialog.empty_title')}</strong><p>{t(pageMode ? 'projects.exports.empty_pages' : 'projects.exports.empty')}</p></div> : null}
        {stage === 'select' ? formatSelector : null}
        {stage === 'confirm' ? <>
          {formatSelector}
          <div className="comic-export-dialog__summary"><strong>{t(pageMode ? 'projects.exports.incomplete_title_pages' : 'projects.exports.incomplete_title')}</strong><p>{t(pageMode ? 'projects.exports.incomplete_body_pages' : 'projects.exports.incomplete_body', { ready: readiness.image_section_count, missing: readiness.missing_section_count })}</p></div>
          <ul className="comic-export-dialog__missing">{(readiness.missing_sections || []).map((section) => <li key={section.uuid}><strong>{t(pageMode ? 'comic.workbench.page_label' : 'comic.workbench.section_label', { number: section.section_no })}</strong><span>{section.title || t(pageMode ? 'comic.page.untitled' : 'comic.section.untitled')}</span></li>)}</ul>
        </> : null}
        {stage === 'check_failed' || stage === 'create_failed' ? <LocalizedErrorMessage error={error} /> : null}
        {stage === 'operation' ? <>
          <dl className="comic-export-dialog__facts">
            <div><dt>{t('projects.exports.dialog.scope')}</dt><dd>{scopeLabel}</dd></div>
            <div><dt>{t('common.label.file')}</dt><dd title={filename}>{filename}</dd></div>
            <div><dt>{t('common.label.format')}</dt><dd>{exportRecord?.format?.toUpperCase() || 'ZIP'}</dd></div>
            <div><dt>{t('projects.exports.dialog.snapshot_version')}</dt><dd>{metrics.version ? `v${formatNumber(metrics.version)}` : '—'}</dd></div>
            <div><dt>{t('common.label.status')}</dt><dd>{statusLabel(t, displayState)}</dd></div>
            <div><dt>{t(pageMode ? 'projects.exports.dialog.ready_pages' : 'projects.exports.dialog.ready_sections')}</dt><dd>{formatNumber(metrics.ready)} / {formatNumber(metrics.total)}</dd></div>
            <div><dt>{t(pageMode ? 'projects.exports.dialog.missing_pages' : 'projects.exports.dialog.missing_sections')}</dt><dd>{formatNumber(metrics.missing)}</dd></div>
            <div><dt>{t('projects.exports.created')}</dt><dd>{exportRecord?.created_at ? formatDateTime(exportRecord.created_at) : '—'}</dd></div>
            <div><dt>{t('projects.exports.retention')}</dt><dd>{t('projects.exports.retention_days', { days: exportRecord?.retention_days || 7 })}</dd></div>
            <div><dt>{t('projects.exports.expires')}</dt><dd>{exportRecord?.expires_at ? formatDateTime(exportRecord.expires_at) : '—'}</dd></div>
            <div className="comic-export-dialog__hash"><dt>{t('projects.exports.snapshot')}</dt><dd><code>{exportRecord?.snapshot_hash || '—'}</code></dd></div>
          </dl>
          {!ready ? <div className="comic-export-dialog__progress" aria-live="polite"><div><strong>{statusLabel(t, displayState)}</strong><span>{formatNumber(progress)}%</span></div><progress max="100" value={progress} aria-label={t('projects.exports.dialog.progress')} /></div> : null}
          {active ? <p className="comic-export-dialog__hint">{t('projects.exports.dialog.background_hint')}</p> : null}
          {operationError ? <LocalizedErrorMessage error={operationError} compact /> : null}
          {error ? <LocalizedErrorMessage error={error} compact /> : null}
        </> : null}
      </div>
      <footer className="lumi-dialog__actions">
        {stage === 'select' || stage === 'confirm' ? <><button type="button" className="button-secondary" onClick={onClose}>{t('common.action.cancel')}</button><button type="button" disabled={!selectedFormat} onClick={() => startExport(stage === 'confirm')}>{t(stage === 'confirm' ? 'projects.exports.continue_partial' : 'projects.exports.dialog.start')}</button></> : null}
        {stage === 'blocked' || stage === 'check_failed' ? <button type="button" className="button-secondary" onClick={onClose}>{t('common.action.close')}</button> : null}
        {stage === 'create_failed' ? <><button type="button" className="button-secondary" onClick={onClose}>{t('common.action.close')}</button><button type="button" onClick={() => startExport(allowMissingRef.current)}>{t('common.action.retry')}</button></> : null}
        {stage === 'checking' || stage === 'creating' ? <button type="button" className="button-secondary" onClick={onClose}>{t('projects.exports.dialog.background')}</button> : null}
        {stage === 'operation' ? <>
          <button type="button" className="button-secondary" disabled={busy} onClick={onClose}>{t(active ? 'projects.exports.dialog.background' : 'common.action.close')}</button>
          {active ? <button type="button" disabled={busy} onClick={() => cancelMutation.mutate()}>{t(cancelMutation.isPending ? 'projects.exports.dialog.cancelling' : 'common.action.cancel')}</button> : null}
          {retryable ? <button type="button" disabled={busy} onClick={() => retryMutation.mutate()}>{t(retryMutation.isPending ? 'projects.exports.dialog.retrying' : 'common.action.retry')}</button> : null}
          {ready ? <a className="button-link" href={exportRecord.download_url} download={filename}><Download size={15} aria-hidden="true" />{t('projects.exports.dialog.download_format', { format: exportRecord.format?.toUpperCase() || 'ZIP' })}</a> : null}
        </> : null}
      </footer>
    </LumiDialog>
  )
}
