import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  Bot,
  Check,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  GripVertical,
  ImagePlus,
  MoreHorizontal,
  Route as RouteIcon,
  PanelRightClose,
  PanelRightOpen,
  Pencil,
  Plus,
  Send,
  Square,
  Trash2,
  Zap,
  X,
} from 'lucide-react'

import {
  abortChatTurn,
  cancelUserInput,
  cancelWorkflow,
  createChatThread,
  createChatTurn,
  createFollowUp,
  deleteFollowUp,
  getChatThread,
  listChatItems,
  listChatTurns,
  listFollowUps,
  listUserInputRequests,
  listWorkflows,
  listWorkflowEvents,
  listWorkflowLLMLogs,
  listWorkflowRuns,
  moveFollowUp,
  respondUserInput,
  resolveWorkflowConflict,
  retryWorkflow,
  steerChatRun,
  steerFollowUp,
  updateFollowUp,
} from '../api/chat.js'
import { createAssetUpload, finalizeAssetUpload } from '../api/assets.js'
import {
  captureChatScrollAnchor,
  chatComposerMode,
  chatThreadCountLabel,
  chatTurnDurationMs,
  chatTurnElapsedMs,
  groupChatItemsByTurn,
  isChatSteeringShortcut,
  projectChatTurnActivity,
  projectChatUserInput,
  restoreChatScrollAnchor,
  shouldLoadEarlierChatItems,
  shouldShowAssistantPending,
  suggestedChatThreadTitle,
  threadDisplayTitle,
  workflowDisplayTitle,
  workflowProgressPercent,
} from '../pages/chatAreaPresentation.js'
import {
  MAX_PROJECT_CHAT_REFERENCES,
  appendProjectChatReference,
  consumeProjectChatReferenceQuery,
  projectChatClipboardFiles,
  readyProjectChatReferences,
  referenceKey,
  removeProjectChatAttachment,
  selectProjectChatClipboardImages,
  selectProjectChatImageFiles,
} from '../pages/projectChatAttachments.js'
import { ACTIVE_CHAT_STATUSES, ACTIVE_WORKFLOW_STATUSES, agentQueryKeysForEvent, comicStoryboardOverwriteRequest, workflowControls } from '../pages/chatWorkspaceState.js'
import { flattenProjectThreads, useProjectThreads } from '../pages/projectThreads.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { ReferencePicker, ReferenceStrip } from './ChatReferences.jsx'
import SafeMarkdown from './SafeMarkdown.jsx'

const threadStatusCopy = {
  idle: 'chat.status.idle', busy: 'chat.status.busy', waiting_for_input: 'chat.status.waiting_for_input', completed: 'common.status.completed',
  failed: 'common.status.failed', cancelled: 'common.status.cancelled', interrupted: 'common.status.interrupted',
}

const turnStatusCopy = {
  queued: 'chat.status.queued', in_progress: 'chat.status.in_progress', waiting_for_input: 'chat.status.waiting_for_input', completed: 'common.status.completed',
  failed: 'common.status.failed', cancelled: 'common.status.cancelled', interrupted: 'common.status.interrupted',
}

const workflowStatusCopy = {
  pending: 'chat.workflow.status.queued', queued: 'chat.workflow.status.queued', running: 'chat.workflow.status.running', waiting: 'chat.status.waiting_for_input', completed: 'common.status.completed', failed: 'common.status.failed',
  cancelled: 'common.status.cancelled', interrupted: 'common.status.interrupted',
}

const stepCopy = {
  project_initialization: 'chat.workflow.step.project_initialization', story: 'chat.workflow.step.story', story_profile: 'chat.workflow.step.story_profile',
  premise: 'chat.workflow.step.premise', comic_sections: 'chat.workflow.step.comic_sections', first_section_image: 'chat.workflow.step.first_section_image',
  select_reference_assets: 'chat.workflow.step.select_reference_assets', save_section_premise: 'chat.workflow.step.save_section_premise',
  generate_section_image: 'chat.workflow.step.generate_section_image', save_section_image: 'chat.workflow.step.save_section_image',
  comic_storyboard: 'chat.workflow.step.comic_storyboard',
  story_chapter: 'chat.workflow.step.story_chapter',
  chapter_batch_plan: 'chat.workflow.step.chapter_batch_plan',
}

const MESSAGE_PAGE_LIMIT = 30

function threadTrajectoryHref(projectUuid, threadUuid) {
  return `/projects/${encodeURIComponent(projectUuid)}/threads/${encodeURIComponent(threadUuid)}/trajectory`
}

function CollapseButton({ overlay, onToggle }) {
  const { t } = useI18n()
  const Icon = overlay ? X : PanelRightClose
  const label = t(overlay ? 'chat.close' : 'chat.collapse')
  return (
    <button className="chat-collapse-button" type="button" onClick={onToggle} aria-label={label} aria-expanded="true" title={label}>
      <Icon size={16} aria-hidden="true" />
    </button>
  )
}

function ErrorNotice({ error, onDismiss }) {
  return <LocalizedErrorMessage error={error} className="chat-error" onDismiss={onDismiss} />
}

function AttachmentPicker({ disabled, onFiles }) {
  const { t } = useI18n()
  const inputRef = useRef(null)
  return (
    <>
      <button className="chat-attachment-picker" type="button" disabled={disabled} onClick={() => inputRef.current?.click()} title={t('chat.attachment.add')} aria-label={t('chat.attachment.add')}><ImagePlus size={16} /></button>
      <input ref={inputRef} className="chat-attachment-input" type="file" accept="image/png,image/jpeg,image/webp" multiple disabled={disabled} onChange={(event) => { onFiles(event.target.files); event.target.value = '' }} />
    </>
  )
}

function UserInputCard({ request, pending, onRespond, onCancel }) {
  const presentation = projectChatUserInput(request)
  if (presentation.mode !== 'pending') {
    return <UserInputHistory request={request} presentation={presentation} />
  }
  return <PendingUserInputCard request={request} presentation={presentation} pending={pending} onRespond={onRespond} onCancel={onCancel} />
}

function PendingUserInputCard({ request, presentation, pending, onRespond, onCancel }) {
  const { t } = useI18n()
  const initialSelected = presentation.selectedOptionUuids
  const initialOtherText = presentation.otherText
  const [selected, setSelected] = useState(initialSelected)
  const [otherText, setOtherText] = useState(initialOtherText)
  const isMultiple = request.input_type === 'multiple_choice'

  useEffect(() => {
    setSelected(initialSelected)
    setOtherText(initialOtherText)
  }, [request.uuid, initialOtherText, initialSelected.join('\u0000')])

  const toggle = (uuid) => {
    setSelected((current) => isMultiple
      ? current.includes(uuid) ? current.filter((item) => item !== uuid) : [...current, uuid]
      : [uuid])
    if (!isMultiple) setOtherText('')
  }
  const updateOtherText = (value) => {
    setOtherText(value)
    if (!isMultiple && value.trim()) setSelected([])
  }
  return (
    <article className="chat-message chat-message--assistant chat-message--user-input">
      <form className="chat-input-request" onSubmit={(event) => {
        event.preventDefault()
        onRespond(request.uuid, { selected_option_uuids: selected, other_text: otherText.trim() })
      }}>
        <span className="chat-message__type"><Bot size={13} aria-hidden="true" />Lumi Agent</span>
        <strong>{request.question}</strong>
        <div className="chat-input-request__options">
          {request.options.map((option) => (
            <label className="chat-input-request__option" key={option.uuid}>
              <input
                checked={selected.includes(option.uuid)}
                disabled={pending}
                name={`chat-input-${request.uuid}`}
                onChange={() => toggle(option.uuid)}
                type={isMultiple ? 'checkbox' : 'radio'}
                value={option.uuid}
              />
              <span><b>{option.label}</b>{option.description ? <small>{option.description}</small> : null}</span>
            </label>
          ))}
        </div>
        <label className="chat-input-request__other">
          <span>{t('chat.input.other')}</span>
          <input disabled={pending} value={otherText} onChange={(event) => updateOtherText(event.target.value)} placeholder={t('chat.input.other_placeholder')} />
        </label>
        <footer>
          <button type="submit" disabled={pending || (!selected.length && !otherText.trim())}>{t(pending ? 'chat.input.submitting' : 'common.action.submit')}</button>
          <button type="button" className="button-quiet" disabled={pending} onClick={() => onCancel(request.uuid)}>{t('chat.input.cancel_turn')}</button>
        </footer>
      </form>
    </article>
  )
}

function UserInputHistory({ request, presentation }) {
  const { t } = useI18n()
  const selected = new Set(presentation.selectedOptionUuids)
  const answer = presentation.answers.join(t('chat.input.answer_separator'))
  const incomplete = presentation.mode === 'incomplete'
  return (
    <article className={`chat-message chat-message--user-input chat-message--user-input-history${incomplete ? ' chat-message--user-input-incomplete' : ''}`}>
      <details className="chat-input-history">
        <summary>
          {incomplete ? <X size={14} aria-hidden="true" /> : <Check size={14} aria-hidden="true" />}
          <span>
            <strong>{incomplete ? t('chat.input.incomplete_summary') : t('chat.input.answered_summary', { answer })}</strong>
            <small>{request.question}</small>
          </span>
          <small>{t('chat.input.history_hint')}</small>
          <ChevronDown className="chat-input-history__chevron" size={14} aria-hidden="true" />
        </summary>
        <div className="chat-input-history__content">
          <section>
            <b>{t('chat.input.question')}</b>
            <p>{request.question}</p>
          </section>
          {request.options?.length ? (
            <section>
              <b>{t('chat.input.options')}</b>
              <ul>
                {request.options.map((option) => (
                  <li className={selected.has(option.uuid) ? 'is-selected' : ''} key={option.uuid}>
                    <span>{selected.has(option.uuid) ? <Check size={12} aria-hidden="true" /> : null}</span>
                    <span><strong>{option.label}</strong>{option.description ? <small>{option.description}</small> : null}</span>
                    {selected.has(option.uuid) ? <em>{t('chat.input.selected')}</em> : null}
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
          <section>
            <b>{t('chat.input.final_answer')}</b>
            <p>{incomplete ? t('chat.input.not_answered') : answer}</p>
          </section>
        </div>
      </details>
    </article>
  )
}

function ChatItem({ item, projectUuid }) {
  const { t } = useI18n()
  if (item.item_type === 'error') {
    return <article className="chat-message chat-message--error"><div>{t('chat.item.error')}</div></article>
  }

  if (item.role === 'user') {
    return <article className="chat-message chat-message--user"><div className="chat-message__user-bubble"><p>{item.content}</p><ReferenceStrip projectUuid={projectUuid} references={item.references || []} compact /></div></article>
  }

  return (
    <article className={`chat-message chat-message--${item.role || 'assistant'}`}>
      <div className="chat-message__assistant-body">
        <span className="chat-message__type"><Bot size={13} aria-hidden="true" />{item.role === 'system' ? t('chat.item.system') : 'Lumi Agent'}</span>
        <SafeMarkdown value={item.content} />
      </div>
    </article>
  )
}

function ToolActivitySummary({ activity, turn }) {
  const { formatNumber, t } = useI18n()
  if (activity.mode !== 'terminal' || !activity.tools.length) return null
  const abnormalTurn = ['failed', 'cancelled', 'interrupted'].includes(turn?.status)
  const issueSummary = activity.issueCount > 0
    ? t('chat.tool.summary.issues', { issue_count: activity.issueCount })
    : abnormalTurn
      ? t('chat.tool.summary.turn_issue')
      : ''
  const duration = chatTurnDurationLabel(turn, t, formatNumber)

  return (
    <article className={`chat-message chat-message--tool-summary${abnormalTurn || activity.issueCount > 0 ? ' chat-message--tool-summary-issue' : ''}`}>
      <details className="chat-tool-summary">
        <summary>
          <span>{duration || t('chat.tool.summary.title')}</span>
          {issueSummary ? <em>{issueSummary}</em> : null}
          <ChevronRight className="chat-tool-summary__chevron" size={12} aria-hidden="true" />
        </summary>
        <ol>
          {activity.tools.map((tool) => (
            <li className={`chat-tool-execution chat-tool-execution--${statusClass(tool.status)}`} key={tool.key}>
              <details>
                <summary>
                  <strong>{tool.toolName}</strong>
                  <span>{toolStatusLabel(tool.status, t)}</span>
                  <ChevronDown className="chat-tool-execution__chevron" size={13} aria-hidden="true" />
                </summary>
                <div className="chat-tool-execution__content">
                  {tool.call ? <ToolActivityPayload label={t('chat.tool.arguments')} item={tool.call} /> : <small>{t('chat.tool.arguments_unavailable')}</small>}
                  {tool.result ? <ToolActivityPayload label={t('chat.tool.result')} item={tool.result} /> : <small>{t('chat.tool.result_unavailable')}</small>}
                </div>
              </details>
            </li>
          ))}
        </ol>
      </details>
    </article>
  )
}

function chatTurnDurationLabel(turn, t, formatNumber) {
  const durationMs = chatTurnDurationMs(turn)
  if (durationMs === null) return ''
  if (durationMs < 1000) return t('chat.turn.duration.less_than_second')

  const totalSeconds = Math.max(1, Math.round(durationMs / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) {
    return t('chat.turn.duration.hours_minutes_seconds', {
      hours: formatNumber(hours),
      minutes: formatNumber(minutes),
      seconds: formatNumber(seconds),
    })
  }
  if (minutes > 0) {
    return t('chat.turn.duration.minutes_seconds', {
      minutes: formatNumber(minutes),
      seconds: formatNumber(seconds),
    })
  }
  return t('chat.turn.duration.seconds', { seconds: formatNumber(seconds) })
}

function ToolActivityPayload({ label, item }) {
  const { t } = useI18n()
  const content = item.content || (item.target_uuid ? `target_uuid: ${item.target_uuid}` : t('chat.tool.project_only'))
  return <section><b>{label}</b><pre data-machine-value>{content}</pre></section>
}

function toolStatusLabel(status, t) {
  return {
    completed: t('common.status.completed'),
    failed: t('common.status.failed'),
    interrupted: t('common.status.interrupted'),
    running: t('chat.tool.running'),
    pending: t('common.status.pending'),
  }[status] || t('common.status.unknown_with_code', { code: status })
}

function TurnGroup({ group, projectUuid, historyMayBePartial, requestByItemUuid, inputPending, onRespond, onCancel }) {
  const { formatDateTime, t } = useI18n()
  const turn = group.turn
  const activity = projectChatTurnActivity(turn, group.items, { historyMayBePartial })
  const summary = <ToolActivitySummary activity={activity} turn={turn} />
  return (
    <section className={`chat-turn chat-turn--${statusClass(turn?.status)}`} data-turn-uuid={group.uuid}>
      {turn ? (
        <div className="chat-turn__meta">
          <span>{t('chat.turn.number', { number: turn.queue_sequence || '—' })}</span>
          <span className={`chat-turn__status chat-turn__status--${statusClass(turn.status)}`}>{turnStatusCopy[turn.status] ? t(turnStatusCopy[turn.status]) : t('common.status.unknown_with_code', { code: turn.status })}</span>
          <time>{formatDateTime(turn.created_at, { hour: '2-digit', minute: '2-digit' })}</time>
        </div>
      ) : null}
      <div className="chat-turn__items">
        {activity.conversationItems.map((item, index) => (
          <Fragment key={item.uuid}>
            {index === activity.summaryIndex ? summary : null}
            {item.item_type === 'user_input_request' && requestByItemUuid.get(item.uuid)
              ? <UserInputCard request={requestByItemUuid.get(item.uuid)} pending={inputPending} onRespond={onRespond} onCancel={onCancel} />
              : <ChatItem item={item} projectUuid={projectUuid} />}
          </Fragment>
        ))}
        {activity.summaryIndex === activity.conversationItems.length ? summary : null}
        {turn?.status === 'failed' && turn.error_code ? <LocalizedErrorMessage error={{ code: turn.error_code }} compact /> : null}
        <TurnActivity turn={turn} items={group.items} activity={activity} />
      </div>
    </section>
  )
}

function TurnActivity({ turn, items, activity }) {
  const { t } = useI18n()
  const active = turn?.status === 'in_progress'
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return undefined
    setNow(Date.now())
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [active, turn?.uuid])
  if (!active) return null
  const pending = shouldShowAssistantPending(turn, items)
  const copy = pending ? t('chat.activity.thinking') : turnActivityCopy(activity, t)
  if (!copy) return null
  const longRunning = pending && chatTurnElapsedMs(turn, now) >= 10_000
  return <div className="chat-turn__activity" role="status" aria-live="polite"><i aria-hidden="true" /><span>{copy}{longRunning ? <small>{t('chat.activity.long_running')}</small> : null}</span></div>
}

function WorkflowProgress({ projectUuid, workflow, pending, onCancel, onRetry, onResolveConflict }) {
  const { t } = useI18n()
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false)
  const [diagnosticStepUuid, setDiagnosticStepUuid] = useState('')
  useEffect(() => {
    setDiagnosticsOpen(false)
    setDiagnosticStepUuid('')
  }, [workflow?.uuid])
  if (!workflow) return null
  const controls = workflowControls(workflow)
  const overwriteRequest = comicStoryboardOverwriteRequest(workflow)
  const progress = workflowProgressPercent(workflow)
  const openStepDiagnostics = (stepUuid) => {
    setDiagnosticStepUuid(stepUuid)
    setDiagnosticsOpen(true)
  }
  return (
    <section className="workflow-progress">
      <header><div><span>{t('chat.workflow.title')}</span><strong>{workflowDisplayTitle(workflow, t)}</strong></div><b className={`workflow-status workflow-status--${workflow.status}`}>{workflowStatusCopy[workflow.status] ? t(workflowStatusCopy[workflow.status]) : t('common.status.unknown_with_code', { code: workflow.status })}</b></header>
      <div className="workflow-progress__meter"><progress max="100" value={progress} aria-label={t('chat.workflow.progress', { progress })} /><small>{progress}%</small></div>
      <ol>{workflow.steps?.map((step) => {
        const title = stepCopy[step.step_key] ? t(stepCopy[step.step_key]) : t('common.status.unknown_with_code', { code: step.step_key })
        return (
          <li key={step.uuid} className={`workflow-step workflow-step--${step.status}`}>
            <button type="button" aria-pressed={diagnosticStepUuid === step.uuid} aria-label={t('chat.workflow.open_step_details', { title })} onClick={() => openStepDiagnostics(step.uuid)}>
              <span aria-hidden="true">{step.status === 'completed' ? '✓' : step.status === 'running' || step.status === 'waiting' ? '●' : step.status === 'failed' ? '!' : '○'}</span>
              <div><strong>{title}</strong><small>{workflowStatusCopy[step.status] ? t(workflowStatusCopy[step.status]) : t('common.status.unknown_with_code', { code: step.status })} · {Math.min(100, Math.max(0, Number(step.progress) || 0))}%</small></div>
              {step.resource_uuid ? <code>{step.resource_uuid.slice(0, 10)}</code> : null}
            </button>
          </li>
        )
      })}</ol>
      {overwriteRequest ? (
        <section className="workflow-conflict-confirmation" role="alert">
          <div>
            <strong>{t('chat.workflow.conflict.title')}</strong>
            <p>{t('chat.workflow.conflict.body', { existing: overwriteRequest.existingSectionCount, generated: overwriteRequest.generatedSectionCount })}</p>
            <small>{t('chat.workflow.conflict.snapshot_notice')}</small>
          </div>
          <footer>
            <button type="button" className="button-secondary" disabled={pending} onClick={() => onResolveConflict(workflow.uuid, 'keep_existing', overwriteRequest.expectedComicStateRevision)}>{t(pending ? 'chat.workflow.conflict.processing' : 'chat.workflow.conflict.keep_existing')}</button>
            <button type="button" className="button-danger" disabled={pending} onClick={() => onResolveConflict(workflow.uuid, 'overwrite', overwriteRequest.expectedComicStateRevision)}>{t(pending ? 'chat.workflow.conflict.processing' : 'chat.workflow.conflict.overwrite')}</button>
          </footer>
        </section>
      ) : null}
      {workflow.error_code ? <LocalizedErrorMessage error={{ code: workflow.error_code }} compact /> : null}
      <footer>{controls.canCancel ? <button type="button" className="button-secondary" disabled={pending} onClick={() => onCancel(workflow.uuid)}>{t('chat.workflow.cancel')}</button> : null}{controls.canRetry ? <button type="button" disabled={pending} onClick={() => onRetry(workflow.uuid)}>{t('chat.workflow.retry')}</button> : null}<small>{t('chat.workflow.persisted')}</small></footer>
      <WorkflowDiagnostics projectUuid={projectUuid} workflow={workflow} open={diagnosticsOpen} onOpenChange={setDiagnosticsOpen} focusStepUuid={diagnosticStepUuid} onFocusStep={setDiagnosticStepUuid} />
    </section>
  )
}

function WorkflowDiagnostics({ projectUuid, workflow, open, onOpenChange, focusStepUuid, onFocusStep }) {
  const { formatDateTime, formatNumber, t } = useI18n()
  const [selectedLog, setSelectedLog] = useState(null)
  useEffect(() => setSelectedLog(null), [workflow.uuid, focusStepUuid])
  const runsQuery = useInfiniteQuery({
    queryKey: ['workflow-runs', projectUuid, workflow.uuid],
    queryFn: ({ pageParam }) => listWorkflowRuns(projectUuid, workflow.uuid, { before: pageParam, limit: 20 }),
    initialPageParam: '',
    getPreviousPageParam: (page) => page.cursor_pagination?.has_more ? page.cursor_pagination.prev_cursor : undefined,
    getNextPageParam: () => undefined,
    enabled: open,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const eventsQuery = useInfiniteQuery({
    queryKey: ['workflow-events', projectUuid, workflow.uuid],
    queryFn: ({ pageParam }) => listWorkflowEvents(projectUuid, workflow.uuid, { before: pageParam, limit: 40 }),
    initialPageParam: '',
    getPreviousPageParam: (page) => page.cursor_pagination?.has_more ? page.cursor_pagination.prev_cursor : undefined,
    getNextPageParam: () => undefined,
    enabled: open,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const logsQuery = useInfiniteQuery({
    queryKey: ['workflow-llm-logs', projectUuid, workflow.uuid, focusStepUuid],
    queryFn: ({ pageParam }) => listWorkflowLLMLogs(projectUuid, workflow.uuid, { page: pageParam, perPage: 10, stepUuid: focusStepUuid }),
    initialPageParam: 1,
    getNextPageParam: (page) => page.pagination.current_page < page.pagination.last_page ? page.pagination.current_page + 1 : undefined,
    enabled: open,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const runs = useMemo(() => uniqueByUUID(runsQuery.data?.pages?.flatMap((page) => page.items || []) || []), [runsQuery.data])
  const events = useMemo(() => uniqueByUUID(eventsQuery.data?.pages?.flatMap((page) => page.items || []) || []), [eventsQuery.data])
  const logs = useMemo(() => uniqueByUUID(logsQuery.data?.pages?.flatMap((page) => page.items || []) || []), [logsQuery.data])
  const focusedStep = workflow.steps?.find((step) => step.uuid === focusStepUuid)
  const focusedRun = runs.find((run) => run.step_uuid === focusStepUuid)
  const queryError = runsQuery.error || eventsQuery.error || logsQuery.error
  useEffect(() => {
    if (open && focusStepUuid && !logsQuery.isFetching && logs.length > 0 && !selectedLog) setSelectedLog(logs[0])
  }, [focusStepUuid, logs, logsQuery.isFetching, open, selectedLog])

  return (
    <details className="workflow-diagnostics" open={open} onToggle={(event) => onOpenChange(event.currentTarget.open)}>
      <summary>{t('chat.workflow.diagnostics')}</summary>
      <div className="workflow-diagnostics__content">
        {queryError ? <LocalizedErrorMessage error={queryError} compact /> : null}
        <section>
          <h4>{t('chat.workflow.runs')}</h4>
          {runsQuery.isLoading ? <p>{t('chat.loading')}</p> : null}
          {!runsQuery.isLoading && !runsQuery.error && runs.length === 0 ? <p>{t('chat.workflow.no_runs')}</p> : null}
          <ol>{runs.map((run) => <li key={run.uuid}><button type="button" aria-pressed={focusStepUuid === run.step_uuid} onClick={() => onFocusStep(run.step_uuid)}><div><strong>{stepCopy[run.step_key] ? t(stepCopy[run.step_key]) : t('common.status.unknown_with_code', { code: run.step_key })}</strong><small>{t('chat.workflow.attempt', { number: run.attempt })} · {Math.min(100, Math.max(0, Number(run.progress) || 0))}% · {formatDateTime(run.updated_at)}</small></div><span className={`workflow-status workflow-status--${run.status}`}>{workflowStatusCopy[run.status] ? t(workflowStatusCopy[run.status]) : t('common.status.unknown_with_code', { code: run.status })}</span>{run.error_code ? <code>{run.error_code}</code> : null}</button></li>)}</ol>
          {focusedStep ? (
            <article className="workflow-diagnostics__step-detail">
              <header><strong>{stepCopy[focusedStep.step_key] ? t(stepCopy[focusedStep.step_key]) : t('common.status.unknown_with_code', { code: focusedStep.step_key })}</strong><button type="button" className="button-quiet" onClick={() => onFocusStep('')}>{t('chat.workflow.show_all_logs')}</button></header>
              <dl>
                <div><dt>{t('chat.workflow.step_uuid')}</dt><dd><code>{focusedStep.uuid}</code></dd></div>
                <div><dt>{t('chat.workflow.workflow_uuid')}</dt><dd><code>{workflow.uuid}</code></dd></div>
                <div><dt>{t('chat.workflow.status')}</dt><dd>{workflowStatusCopy[focusedStep.status] ? t(workflowStatusCopy[focusedStep.status]) : t('common.status.unknown_with_code', { code: focusedStep.status })}</dd></div>
                {focusedRun ? <div><dt>{t('chat.workflow.attempt_label')}</dt><dd>{formatNumber(focusedRun.attempt)}</dd></div> : null}
                {focusedStep.task_uuid ? <div><dt>{t('chat.workflow.task_uuid')}</dt><dd><code>{focusedStep.task_uuid}</code></dd></div> : null}
                {focusedStep.resource_uuid ? <div><dt>{t('chat.workflow.resource_uuid')}</dt><dd><code>{focusedStep.resource_uuid}</code></dd></div> : null}
                <div><dt>{t('chat.workflow.created_at')}</dt><dd>{formatDateTime(focusedStep.created_at)}</dd></div>
                {focusedStep.started_at ? <div><dt>{t('chat.workflow.started_at')}</dt><dd>{formatDateTime(focusedStep.started_at)}</dd></div> : null}
                {focusedStep.completed_at ? <div><dt>{t('chat.workflow.completed_at')}</dt><dd>{formatDateTime(focusedStep.completed_at)}</dd></div> : null}
                {focusedStep.error_code ? <div><dt>{t('chat.workflow.error_code')}</dt><dd><code>{focusedStep.error_code}</code></dd></div> : null}
                {focusedStep.error_code ? <div><dt>{t('chat.workflow.error_summary')}</dt><dd><LocalizedErrorMessage error={{ code: focusedStep.error_code }} compact /></dd></div> : null}
              </dl>
              <details><summary>{t('chat.workflow.input')}</summary><pre data-machine-value>{prettyDiagnosticJSON(focusedStep.input)}</pre></details>
              <details><summary>{t('chat.workflow.output')}</summary><pre data-machine-value>{prettyDiagnosticJSON(focusedStep.output)}</pre></details>
            </article>
          ) : null}
          {runsQuery.hasPreviousPage ? <button type="button" className="button-quiet" disabled={runsQuery.isFetchingPreviousPage} onClick={() => runsQuery.fetchPreviousPage()}>{t('chat.workflow.load_older')}</button> : null}
        </section>
        <section>
          <h4>{t('chat.workflow.events')}</h4>
          {eventsQuery.isLoading ? <p>{t('chat.loading')}</p> : null}
          {!eventsQuery.isLoading && !eventsQuery.error && events.length === 0 ? <p>{t('chat.workflow.no_events')}</p> : null}
          <div className="workflow-diagnostics__events">{events.map((event) => <details key={event.uuid}><summary><span>{event.event_type}</span><time>{formatDateTime(event.created_at)}</time></summary><dl><div><dt>{t('chat.workflow.event_uuid')}</dt><dd><code>{event.uuid}</code></dd></div>{event.step_uuid ? <div><dt>{t('chat.workflow.step_uuid')}</dt><dd><code>{event.step_uuid}</code></dd></div> : null}<div><dt>{t('chat.workflow.sequence')}</dt><dd>{formatNumber(event.sequence)}</dd></div><div><dt>{t('chat.workflow.created_at')}</dt><dd>{formatDateTime(event.created_at)}</dd></div></dl><pre data-machine-value>{prettyDiagnosticJSON(event.payload)}</pre></details>)}</div>
          {eventsQuery.hasPreviousPage ? <button type="button" className="button-quiet" disabled={eventsQuery.isFetchingPreviousPage} onClick={() => eventsQuery.fetchPreviousPage()}>{t('chat.workflow.load_older')}</button> : null}
        </section>
        <section>
          <div className="workflow-diagnostics__section-heading"><h4>{t('chat.workflow.llm_logs')}</h4>{focusStepUuid ? <button type="button" className="button-quiet" onClick={() => onFocusStep('')}>{t('chat.workflow.show_all_logs')}</button> : null}</div>
          {focusedStep ? <p>{t('chat.workflow.filtered_step', { title: stepCopy[focusedStep.step_key] ? t(stepCopy[focusedStep.step_key]) : t('common.status.unknown_with_code', { code: focusedStep.step_key }) })}</p> : null}
          {logsQuery.isLoading ? <p>{t('chat.loading')}</p> : null}
          {!logsQuery.isLoading && !logsQuery.error && logs.length === 0 ? <p>{t(focusStepUuid ? 'chat.workflow.no_llm_logs_for_step' : 'chat.workflow.no_llm_logs')}</p> : null}
          <div className="workflow-diagnostics__logs">{logs.map((log) => <button type="button" aria-pressed={selectedLog?.uuid === log.uuid} key={log.uuid} onClick={() => setSelectedLog((current) => !focusStepUuid && current?.uuid === log.uuid ? null : log)}><span><strong>{log.scenario}</strong><small>{log.model} · {formatDateTime(log.created_at)}</small></span><em>{log.status}</em></button>)}</div>
          {selectedLog ? <dl className="workflow-diagnostics__log-detail"><div><dt>UUID</dt><dd><code>{selectedLog.uuid}</code></dd></div><div><dt>{t('chat.workflow.step_uuid')}</dt><dd><code>{selectedLog.workflow_step_uuid}</code></dd></div><div><dt>{t('chat.workflow.attempt_label')}</dt><dd>{formatNumber(selectedLog.attempt)}</dd></div><div><dt>{t('chat.workflow.status')}</dt><dd>{selectedLog.status}</dd></div><div><dt>{t('chat.workflow.request_type')}</dt><dd>{selectedLog.request_type}</dd></div><div><dt>{t('chat.workflow.tokens')}</dt><dd>{formatNumber(selectedLog.input_tokens + selectedLog.output_tokens)}</dd></div><div><dt>{t('chat.workflow.duration')}</dt><dd>{t('chat.workflow.duration_ms', { number: formatNumber(selectedLog.duration_ms) })}</dd></div><div><dt>{t('chat.workflow.created_at')}</dt><dd>{formatDateTime(selectedLog.created_at)}</dd></div>{selectedLog.completed_at ? <div><dt>{t('chat.workflow.completed_at')}</dt><dd>{formatDateTime(selectedLog.completed_at)}</dd></div> : null}{selectedLog.error_code ? <div><dt>{t('chat.workflow.error_code')}</dt><dd><code>{selectedLog.error_code}</code></dd></div> : null}</dl> : null}
          {logsQuery.hasNextPage ? <button type="button" className="button-quiet" disabled={logsQuery.isFetchingNextPage} onClick={() => logsQuery.fetchNextPage()}>{t('chat.thread.load_more')}</button> : null}
        </section>
      </div>
    </details>
  )
}

function FollowUpQueue({ projectUuid, items, pending, canSteer, notice, onMove, onDelete, onEdit, onSteer }) {
  const { formatDateTime, t } = useI18n()
  const [editingUuid, setEditingUuid] = useState('')
  const [editingText, setEditingText] = useState('')
  const [draggedUuid, setDraggedUuid] = useState('')
  if (!items.length) return null
  const startEditing = (item) => {
    setEditingUuid(item.uuid)
    setEditingText(item.input_text)
  }
  const saveEditing = () => {
    const text = editingText.trim()
    if (!text) return
    onEdit(editingUuid, text)
    setEditingUuid('')
  }
  const keyboardMove = (event, item, index) => {
    if (!event.altKey || !['ArrowUp', 'ArrowDown'].includes(event.key)) return
    event.preventDefault()
    const position = event.key === 'ArrowUp' ? index : index + 2
    if (position < 1 || position > items.length) return
    onMove(item.uuid, position)
  }
  return (
    <section className="chat-queue" aria-label={t('chat.queue.label')}>
      {notice ? <p className="chat-queue__notice" role="status">{notice}</p> : null}
      <ol>
        {items.map((item, index) => (
          <li key={item.uuid} draggable={!pending && editingUuid !== item.uuid} onDragStart={(event) => { setDraggedUuid(item.uuid); event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', item.uuid) }} onDragEnd={() => setDraggedUuid('')} onDragOver={(event) => { if (draggedUuid && draggedUuid !== item.uuid) event.preventDefault() }} onDrop={(event) => { event.preventDefault(); if (draggedUuid && draggedUuid !== item.uuid) onMove(draggedUuid, index + 1); setDraggedUuid('') }} className={draggedUuid === item.uuid ? 'is-dragging' : ''}>
            <button className="chat-queue__handle" type="button" disabled={pending} aria-grabbed={draggedUuid === item.uuid} aria-label={t('chat.queue.reorder', { number: index + 1 })} title={t('chat.queue.reorder_hint')} onKeyDown={(event) => keyboardMove(event, item, index)}><GripVertical size={15} aria-hidden="true" /></button>
            <div className="chat-queue__body">
              {editingUuid === item.uuid ? <form className="chat-queue__edit" onSubmit={(event) => { event.preventDefault(); saveEditing() }}><input autoFocus value={editingText} onChange={(event) => setEditingText(event.target.value)} maxLength="262144" aria-label={t('chat.queue.edit_label')} /><button type="submit" disabled={pending || !editingText.trim()} aria-label={t('common.action.save')}><Check size={14} /></button><button type="button" onClick={() => setEditingUuid('')} aria-label={t('common.action.cancel')}><X size={14} /></button></form> : <p title={item.input_text}>{item.input_text}</p>}
              <small>{t('chat.queue.item', { time: formatDateTime(item.created_at, { hour: '2-digit', minute: '2-digit' }) })}{item.references?.length ? ` · ${t('chat.reference.count_short', { count: item.references.length })}` : ''}</small>
              <ReferenceStrip projectUuid={projectUuid} references={item.references || []} compact />
            </div>
            <div className="chat-queue__actions">
              <button type="button" disabled={pending || editingUuid === item.uuid} onClick={() => startEditing(item)} aria-label={t('chat.queue.edit')}><Pencil size={14} /></button>
              {canSteer ? <button type="button" className="is-steer" disabled={pending} onClick={() => onSteer(item.uuid)} aria-label={t('chat.queue.steer_now')}><Zap size={14} /></button> : null}
              <button type="button" disabled={pending || index === 0} onClick={() => onMove(item.uuid, index)} aria-label={t('chat.queue.move_up')}><ChevronUp size={14} /></button>
              <button type="button" disabled={pending || index === items.length - 1} onClick={() => onMove(item.uuid, index + 2)} aria-label={t('chat.queue.move_down')}><ChevronDown size={14} /></button>
              <button type="button" className="is-danger" disabled={pending} onClick={() => onDelete(item.uuid)} aria-label={t('common.action.delete')}><Trash2 size={14} /></button>
            </div>
          </li>
        ))}
      </ol>
    </section>
  )
}

function ChatComposer({ projectUuid, activeTurn, draft, pending, abortPending, autoFocus = false, references = [], referenceBlocked = false, onDraftChange, onSend, onAbort, onAddFiles, onRemoveReference, onToggleReference, onPaste }) {
  const { t } = useI18n()
  const mode = chatComposerMode({ activeTurn, draft })
  const canSteer = activeTurn?.status === 'in_progress' && draft.trim() && !pending && !referenceBlocked
  const actionTitle = mode === 'stop'
    ? t(abortPending ? 'chat.composer.stopping' : 'chat.composer.stop')
    : t(mode === 'queue' ? 'chat.composer.queue' : 'chat.composer.send')

  const submit = (event) => {
    event?.preventDefault()
    if (mode === 'stop') onAbort()
    else if (mode === 'queue') onSend('follow_up')
    else if (mode === 'send') onSend('turn')
  }

  const handleKeyDown = (event) => {
    if (isChatSteeringShortcut(event)) {
      if (!canSteer) return
      event.preventDefault()
      onSend('steering')
      return
    }
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent?.isComposing) {
      event.preventDefault()
      submit()
    }
  }

  return (
    <form className="chat-composer" onSubmit={submit}>
      {activeTurn ? <p className="chat-composer__status">{activeTurn.status === 'waiting_for_input' ? t('chat.composer.waiting') : t('chat.composer.turn_running', { number: activeTurn.queue_sequence || '—' })}</p> : null}
      <ReferenceStrip projectUuid={projectUuid} references={references} onRemove={onRemoveReference} />
      <textarea
        value={draft}
        onChange={(event) => onDraftChange(event.target.value)}
        onKeyDown={handleKeyDown}
        onPaste={onPaste}
        rows="6"
        maxLength="262144"
        placeholder={t(activeTurn ? 'chat.composer.follow_up_placeholder' : 'chat.composer.placeholder')}
        aria-label={t('chat.composer.label')}
        autoFocus={autoFocus}
        disabled={pending}
      />
      <footer>
        <div className="chat-composer__left"><ReferencePicker projectUuid={projectUuid} references={references} disabled={pending} onToggle={onToggleReference} /><AttachmentPicker disabled={pending || references.filter((item) => item.status !== 'error').length >= MAX_PROJECT_CHAT_REFERENCES} onFiles={onAddFiles} /><small className="chat-composer__hint">{t('chat.composer.hint')}</small></div>
        <button
          className={`chat-composer__send chat-composer__send--${mode}`}
          type="submit"
          title={actionTitle}
          aria-label={actionTitle}
          disabled={mode === 'disabled' || pending || (mode !== 'stop' && referenceBlocked) || (mode === 'stop' && abortPending)}
        >
          {mode === 'stop' ? <Square size={15} fill="currentColor" aria-hidden="true" /> : <Send size={17} aria-hidden="true" />}
        </button>
      </footer>
    </form>
  )
}

function ThreadList({ projectUuid, threads, workflows, total, loading, loadingMore, hasMore, error, overlay, onToggle, onNewThread, onOpenThread, onLoadMore }) {
  const { t } = useI18n()
  const [openMenuUuid, setOpenMenuUuid] = useState('')
  const listRef = useRef(null)
  const sentinelRef = useRef(null)
  const isAutoLoadingRef = useRef(false)
  const workflowByThread = useMemo(() => new Map(workflows.map((workflow) => [workflow.thread_uuid, workflow])), [workflows])
  const threadCount = chatThreadCountLabel(threads.length, total)

  useEffect(() => {
    if (!loadingMore) isAutoLoadingRef.current = false
  }, [loadingMore])

  useEffect(() => {
    if (!openMenuUuid) return undefined
    const closeOutside = (event) => { if (!listRef.current?.contains(event.target)) setOpenMenuUuid('') }
    const closeOnEscape = (event) => { if (event.key === 'Escape') setOpenMenuUuid('') }
    document.addEventListener('pointerdown', closeOutside)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeOutside)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [openMenuUuid])

  useEffect(() => {
    if (!hasMore || loadingMore || loading || typeof IntersectionObserver === 'undefined') return undefined
    const root = listRef.current
    const target = sentinelRef.current
    if (!root || !target) return undefined
    const observer = new IntersectionObserver(([entry]) => {
      if (!entry?.isIntersecting || isAutoLoadingRef.current) return
      isAutoLoadingRef.current = true
      onLoadMore()
    }, { root, rootMargin: '48px 0px', threshold: 0.01 })
    observer.observe(target)
    return () => observer.disconnect()
  }, [hasMore, loading, loadingMore, onLoadMore])

  return (
    <div className="chat-panel">
      <header className="chat-panel__header">
        <div><p>{t('chat.title')}</p><h2>{t('chat.threads')}</h2></div>
        <div className="chat-panel__header-actions"><span>{threadCount}</span><CollapseButton overlay={overlay} onToggle={onToggle} /></div>
      </header>
      <div className="chat-thread-list" ref={listRef}>
        <div className="chat-thread-row chat-thread-row--new">
          <button className="chat-thread chat-thread--new" type="button" onClick={onNewThread}><span><Plus size={14} aria-hidden="true" />{t('chat.thread.new')}</span></button>
        </div>
        {loading ? <p className="chat-muted">{t('chat.thread.loading')}</p> : null}
        {error ? <LocalizedErrorMessage error={error} compact /> : null}
        {!loading && !error && !threads.length ? <p className="chat-empty">{t('chat.thread.empty')}</p> : null}
        {threads.map((thread) => (
          <div className={`chat-thread-row ${openMenuUuid === thread.uuid ? 'is-menu-open' : ''}`} key={thread.uuid}>
            <button className="chat-thread" type="button" onClick={() => onOpenThread(thread.uuid)}>
              <span className="chat-thread__title">{threadDisplayTitle(thread, workflowByThread.get(thread.uuid), t)}</span>
              <span className="chat-thread__meta">{ACTIVE_CHAT_STATUSES.has(thread.status) ? <i aria-hidden="true" /> : null}<span>{threadStatusCopy[thread.status] ? t(threadStatusCopy[thread.status]) : t('common.status.unknown_with_code', { code: thread.status })}</span></span>
            </button>
            <a
              className="chat-thread__trajectory-link"
              href={threadTrajectoryHref(projectUuid, thread.uuid)}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={t('trajectory.thread.open', { title: threadDisplayTitle(thread, workflowByThread.get(thread.uuid), t) })}
              title={t('trajectory.thread.open', { title: threadDisplayTitle(thread, workflowByThread.get(thread.uuid), t) })}
              onClick={(event) => event.stopPropagation()}
            ><RouteIcon size={15} aria-hidden="true" /></a>
            <button className="chat-thread__menu-button" type="button" aria-label={t('chat.thread.actions', { title: threadDisplayTitle(thread, workflowByThread.get(thread.uuid), t) })} aria-expanded={openMenuUuid === thread.uuid} onClick={(event) => { event.stopPropagation(); setOpenMenuUuid((current) => current === thread.uuid ? '' : thread.uuid) }}><MoreHorizontal size={16} /></button>
            {openMenuUuid === thread.uuid ? <div className="chat-thread__menu" role="menu"><button type="button" role="menuitem" onClick={() => { setOpenMenuUuid(''); onOpenThread(thread.uuid) }}>{t('chat.thread.open')}</button><button type="button" role="menuitem" onClick={() => { setOpenMenuUuid(''); copyText(thread.uuid) }}>{t('chat.thread.copy_uuid')}</button></div> : null}
          </div>
        ))}
        {hasMore || loadingMore ? <div className="chat-thread-list__sentinel" ref={sentinelRef}><button type="button" className="button-quiet" disabled={!hasMore || loadingMore} onClick={onLoadMore}>{t(loadingMore ? 'chat.thread.loading_more' : 'chat.thread.load_more')}</button></div> : null}
      </div>
    </div>
  )
}

function NewThreadDraft({ projectUuid, draft, pending, error, overlay, references, referenceBlocked, onDraftChange, onSubmit, onBack, onToggle, onDismissError, onAddFiles, onRemoveReference, onToggleReference, onPaste }) {
  const { t } = useI18n()
  return (
    <div className="chat-panel chat-panel--detail">
      <header className="chat-detail-header">
        <button className="chat-back" type="button" onClick={onBack} aria-label={t('chat.thread.back')}><ArrowLeft size={17} /></button>
        <div><p>{t('chat.title')}</p><h2>{t('chat.thread.new')}</h2></div>
        <div className="chat-detail-actions"><span className="chat-status chat-status--idle">{t('chat.thread.draft')}</span><CollapseButton overlay={overlay} onToggle={onToggle} /></div>
      </header>
      <div className="chat-detail-body">
        <div className="chat-messages">
          <ErrorNotice error={error} onDismiss={onDismissError} />
          <div className="chat-empty-state"><strong>{t('chat.thread.new_title')}</strong><span>{t('chat.thread.new_body')}</span></div>
        </div>
        <div className="chat-composer-shell">
          <ChatComposer projectUuid={projectUuid} activeTurn={null} draft={draft} pending={pending} abortPending={false} autoFocus references={references} referenceBlocked={referenceBlocked} onDraftChange={onDraftChange} onSend={onSubmit} onAbort={() => {}} onAddFiles={onAddFiles} onRemoveReference={onRemoveReference} onToggleReference={onToggleReference} onPaste={onPaste} />
        </div>
      </div>
    </div>
  )
}

export default function ChatArea({ projectUuid, expanded: controlledExpanded, onToggle, overlay = false }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedReferenceType = searchParams.get('chat_reference_type') || ''
  const requestedReferenceUuid = searchParams.get('chat_reference_uuid') || ''
  const requestedReferenceTitle = searchParams.get('chat_reference_title') || ''
  const requestedReference = ['premise_asset', 'comic_section', 'file'].includes(requestedReferenceType) && requestedReferenceUuid
    ? { localId: `${requestedReferenceType}:${requestedReferenceUuid}`, resource_type: requestedReferenceType, resource_uuid: requestedReferenceUuid, title: requestedReferenceTitle, status: 'ready' }
    : null
  const requestedNewThread = searchParams.get('chat_new') === '1' || Boolean(requestedReference)
  const [uncontrolledExpanded, setUncontrolledExpanded] = useState(Boolean(searchParams.get('chat_thread_uuid') || searchParams.get('workflow_uuid') || requestedNewThread))
  const expanded = controlledExpanded ?? uncontrolledExpanded
  const [selectedThreadUuid, setSelectedThreadUuid] = useState(searchParams.get('chat_thread_uuid') || '')
  const [showCreate, setShowCreate] = useState(requestedNewThread)
  const [inputText, setInputText] = useState('')
  const [references, setReferences] = useState([])
  const referencesRef = useRef([])
  const [error, setError] = useState(null)
  const [queueNotice, setQueueNotice] = useState('')
  useEffect(() => setQueueNotice(''), [selectedThreadUuid])

  const threadsQuery = useProjectThreads(projectUuid, expanded)
  const workflowsQuery = useQuery({
    queryKey: ['workflows', projectUuid],
    queryFn: () => listWorkflows(projectUuid),
    enabled: expanded,
  })
  const threads = useMemo(() => flattenProjectThreads(threadsQuery.data?.pages), [threadsQuery.data])
  const threadTotal = threadsQuery.data?.pages?.[0]?.pagination?.total || threads.length
  const workflows = workflowsQuery.data?.items || []

  useEffect(() => {
    const requested = searchParams.get('chat_thread_uuid') || ''
    setSelectedThreadUuid((current) => requested === current ? current : requested)
    if (requested) {
      setShowCreate(false)
      return
    }
    if (requestedNewThread) {
      setShowCreate(true)
    }
  }, [requestedNewThread, searchParams])

  useEffect(() => {
    if (!requestedReference) return
    setSearchParams(consumeProjectChatReferenceQuery(searchParams), { replace: true })
    if (!selectedThreadUuid) {
      setShowCreate(true)
      setReferences((current) => appendProjectChatReference(current, requestedReference))
    }
  }, [requestedReferenceType, requestedReferenceUuid, requestedReferenceTitle, selectedThreadUuid])

  const selectedThreadQuery = useQuery({ queryKey: ['chat-thread', projectUuid, selectedThreadUuid], queryFn: () => getChatThread(projectUuid, selectedThreadUuid), enabled: expanded && Boolean(selectedThreadUuid) && !threads.some((item) => item.uuid === selectedThreadUuid) })
  const selectedThread = threads.find((item) => item.uuid === selectedThreadUuid) || selectedThreadQuery.data
  const referenceBlocked = references.some((item) => item.status === 'uploading' || item.status === 'error')
  const requestedWorkflow = searchParams.get('workflow_uuid')
  const selectedWorkflow = workflows.find((item) => item.uuid === requestedWorkflow)
    || workflows.find((item) => item.thread_uuid === selectedThreadUuid)
    || null

  useEffect(() => { referencesRef.current = references }, [references])
  useEffect(() => () => releaseAttachmentPreviews(referencesRef.current), [])

  const clearReferences = useCallback(() => {
    setReferences((current) => {
      releaseAttachmentPreviews(current)
      return []
    })
  }, [])

  const removeReference = useCallback((localId) => {
    setReferences((current) => {
      const removed = current.find((item) => item.localId === localId)
      releaseAttachmentPreview(removed)
      return removeProjectChatAttachment(current, localId)
    })
  }, [])

  const toggleReference = useCallback((reference) => {
    setReferences((current) => {
      const key = referenceKey(reference)
      const existing = current.find((item) => referenceKey(item) === key)
      if (existing) {
        releaseAttachmentPreview(existing)
        return current.filter((item) => referenceKey(item) !== key)
      }
      if (current.filter((item) => item.status !== 'error').length >= MAX_PROJECT_CHAT_REFERENCES) {
        setError({ code: 'chat_reference_limit_exceeded' })
        return current
      }
      return appendProjectChatReference(current, reference)
    })
  }, [])

  const uploadAttachmentDraft = useCallback((draft) => {
    createAssetUpload(projectUuid, {
      purpose: 'project_chatbot_reference',
      displayName: draft.filename,
      file: draft.file,
    }).then((upload) => finalizeAssetUpload(projectUuid, upload.uuid, 'project_chatbot_reference')).then((asset) => {
      setReferences((current) => current.map((item) => item.localId === draft.localId ? { ...item, status: 'ready', resource_type: 'file', resource_uuid: asset.uuid, image_file_uuid: asset.uuid, image_available: true, asset, error: null } : item))
    }).catch((uploadError) => {
      setReferences((current) => current.map((item) => item.localId === draft.localId ? { ...item, status: 'error', error: uploadError } : item))
      setError(uploadError)
    })
  }, [projectUuid])

  const addAttachmentFiles = useCallback((files) => {
    const currentCount = references.filter((item) => item.status !== 'error').length
    const selection = selectProjectChatImageFiles(files, currentCount)
    if (selection.rejectedNonImages) {
      setError({ code: 'chat_reference_invalid_mime' })
    } else if (selection.exceededLimit) {
      setError({ code: 'chat_reference_limit_exceeded' })
    }
    const drafts = selection.acceptedFiles.map((file, index) => ({
      localId: globalThis.crypto?.randomUUID?.() || `${Date.now()}-${index}-${file.name}`,
      resource_type: 'file',
      resource_uuid: '',
      title: file.name || t('chat.attachment.image'),
      filename: file.name || t('chat.attachment.image'),
      status: 'uploading',
      upload: null,
      file,
      previewUrl: typeof URL !== 'undefined' && URL.createObjectURL ? URL.createObjectURL(file) : '',
    }))
    if (!drafts.length) return
    setReferences((current) => [...current, ...drafts])
    drafts.forEach(uploadAttachmentDraft)
  }, [references, t, uploadAttachmentDraft])

  const handleAttachmentPaste = useCallback((event) => {
    const selected = selectProjectChatClipboardImages(event.clipboardData, references.filter((item) => item.status !== 'error').length)
    if (!selected.hasImages) return
    event.preventDefault()
    addAttachmentFiles(projectChatClipboardFiles(event.clipboardData))
  }, [addAttachmentFiles, references])
  const itemsQuery = useInfiniteQuery({
    queryKey: ['chat-items', projectUuid, selectedThreadUuid, 'pages'],
    queryFn: ({ pageParam }) => listChatItems(projectUuid, selectedThreadUuid, { before: pageParam, limit: MESSAGE_PAGE_LIMIT }),
    initialPageParam: '',
    getPreviousPageParam: () => undefined,
    getNextPageParam: (lastPage) => lastPage.cursor_pagination?.has_more ? lastPage.cursor_pagination.prev_cursor : undefined,
    enabled: expanded && Boolean(selectedThreadUuid),
  })
  const turnsQuery = useQuery({ queryKey: ['chat-turns', projectUuid, selectedThreadUuid], queryFn: () => listChatTurns(projectUuid, selectedThreadUuid), enabled: expanded && Boolean(selectedThreadUuid) })
  const followUpsQuery = useQuery({ queryKey: ['chat-follow-ups', projectUuid, selectedThreadUuid], queryFn: () => listFollowUps(projectUuid, selectedThreadUuid), enabled: expanded && Boolean(selectedThreadUuid) })
  const requestsQuery = useQuery({ queryKey: ['chat-input-requests', projectUuid, selectedThreadUuid], queryFn: () => listUserInputRequests(projectUuid, selectedThreadUuid), enabled: expanded && Boolean(selectedThreadUuid) })
  const invalidate = useCallback((payload = { thread_uuid: selectedThreadUuid }) => {
    agentQueryKeysForEvent(projectUuid, payload).forEach((queryKey) => queryClient.invalidateQueries({ queryKey }))
  }, [projectUuid, queryClient, selectedThreadUuid])
  const createThreadMutation = useMutation({
    mutationFn: async () => {
      const text = inputText.trim()
      const thread = await createChatThread(projectUuid, {
        title: suggestedChatThreadTitle(text),
      })
      try {
        await createChatTurn(projectUuid, thread.uuid, { input_text: text, references: readyProjectChatReferences(references) })
        return { thread, turnError: null }
      } catch (turnError) {
        return { thread, turnError }
      }
    },
    onSuccess: ({ thread, turnError }) => {
      queryClient.setQueryData(['chat-thread', projectUuid, thread.uuid], thread)
      queryClient.invalidateQueries({ queryKey: ['chat-threads', projectUuid] })
      setSelectedThreadUuid(thread.uuid)
      const next = new URLSearchParams(searchParams)
      next.set('chat_thread_uuid', thread.uuid)
      next.delete('workflow_uuid')
      next.delete('chat_new')
      next.delete('chat_reference_type')
      next.delete('chat_reference_uuid')
      next.delete('chat_reference_title')
      setSearchParams(next, { replace: true })
      setShowCreate(false)
      if (!turnError) {
        setInputText('')
        clearReferences()
      }
      setError(turnError)
    },
    onError: setError,
  })
  const composerMutation = useMutation({
    mutationFn: ({ mode, text, referenceInputs }) => {
      if (mode === 'follow_up') return createFollowUp(projectUuid, selectedThreadUuid, text, referenceInputs)
      if (mode === 'steering') return steerChatRun(projectUuid, selectedThreadUuid, text, referenceInputs)
      return createChatTurn(projectUuid, selectedThreadUuid, { input_text: text, references: referenceInputs })
    },
    onSuccess: () => { setInputText(''); clearReferences(); setError(null); invalidate() },
    onError: setError,
  })
  const abortMutation = useMutation({ mutationFn: () => abortChatTurn(projectUuid, selectedThreadUuid), onSuccess: () => invalidate(), onError: setError })
  const followMutation = useMutation({
    mutationFn: ({ action, uuid, position, text }) => {
      if (action === 'delete') return deleteFollowUp(projectUuid, selectedThreadUuid, uuid)
      if (action === 'edit') return updateFollowUp(projectUuid, selectedThreadUuid, uuid, text)
      if (action === 'steer') return steerFollowUp(projectUuid, selectedThreadUuid, uuid)
      return moveFollowUp(projectUuid, selectedThreadUuid, uuid, position)
    },
    onSuccess: (result) => {
      setQueueNotice(result?.delivery_mode === 'follow_up' ? t('chat.queue.steer_missed') : '')
      invalidate()
    }, onError: setError,
  })
  const inputMutation = useMutation({
    mutationFn: ({ requestUuid, payload, cancel }) => cancel ? cancelUserInput(projectUuid, selectedThreadUuid, requestUuid) : respondUserInput(projectUuid, selectedThreadUuid, requestUuid, payload),
    onSuccess: () => invalidate(), onError: setError,
  })
  const workflowMutation = useMutation({
    mutationFn: ({ workflowUuid, action }) => action === 'cancel' ? cancelWorkflow(projectUuid, workflowUuid) : retryWorkflow(projectUuid, workflowUuid),
    onSuccess: (workflow) => invalidate({ thread_uuid: workflow.thread_uuid, workflow_uuid: workflow.uuid }), onError: setError,
  })
  const workflowConflictMutation = useMutation({
    mutationFn: ({ workflowUuid, action, expectedRevision }) => resolveWorkflowConflict(projectUuid, workflowUuid, {
      action,
      expected_comic_state_revision: expectedRevision,
    }),
    onSuccess: (resolution) => {
      setError(null)
      invalidate({ thread_uuid: resolution.thread_uuid, workflow_uuid: resolution.workflow_uuid })
    },
    onError: (mutationError, variables) => {
      setError(mutationError)
      invalidate({ thread_uuid: selectedThreadUuid, workflow_uuid: variables.workflowUuid })
    },
  })

  const items = useMemo(() => uniqueByUUID(itemsQuery.data?.pages?.flatMap((page) => page.items || []) || []).sort((left, right) => Number(left.sequence || 0) - Number(right.sequence || 0)), [itemsQuery.data])
  const turns = turnsQuery.data?.items || []
  const requests = requestsQuery.data?.items || []
  const followUps = followUpsQuery.data?.items || []
  const turnGroups = useMemo(() => groupChatItemsByTurn(items, turns), [items, turns])
  const requestByItemUuid = useMemo(() => new Map(requests.map((request) => [request.item_uuid, request])), [requests])
  const inlineRequestUuids = useMemo(() => new Set(items.map((item) => requestByItemUuid.get(item.uuid)?.uuid).filter(Boolean)), [items, requestByItemUuid])
  const activeTurn = turns.find((item) => ['queued', 'in_progress', 'waiting_for_input'].includes(item.status)) || null
  const messagesRef = useRef(null)
  const scrollAnchorRef = useRef(null)
  const isLoadingEarlierRef = useRef(false)
  const initializedMessageThreadRef = useRef('')
  useEffect(() => {
    if (!selectedThreadUuid || !items.length || initializedMessageThreadRef.current === selectedThreadUuid) return
    initializedMessageThreadRef.current = selectedThreadUuid
    requestAnimationFrame(() => {
      const container = messagesRef.current
      if (container) container.scrollTop = container.scrollHeight
    })
  }, [items.length, selectedThreadUuid])

  useEffect(() => {
    if (!scrollAnchorRef.current || !messagesRef.current) return
    const anchor = scrollAnchorRef.current
    scrollAnchorRef.current = null
    requestAnimationFrame(() => restoreChatScrollAnchor(messagesRef.current, anchor))
  }, [items.length])

  const loadEarlierMessages = useCallback(async () => {
    if (!itemsQuery.hasNextPage || itemsQuery.isFetchingNextPage || isLoadingEarlierRef.current) return
    isLoadingEarlierRef.current = true
    scrollAnchorRef.current = captureChatScrollAnchor(messagesRef.current)
    try {
      await itemsQuery.fetchNextPage()
    } finally {
      isLoadingEarlierRef.current = false
    }
  }, [itemsQuery])

  const handleMessagesScroll = useCallback(() => {
    if (!shouldLoadEarlierChatItems({
      scrollTop: messagesRef.current?.scrollTop,
      hasEarlierPage: itemsQuery.hasNextPage,
      isFetchingEarlierPage: itemsQuery.isFetchingNextPage,
    })) return
    void loadEarlierMessages()
  }, [itemsQuery.hasNextPage, itemsQuery.isFetchingNextPage, loadEarlierMessages])

  const toggleExpanded = () => {
    if (onToggle) onToggle()
    else setUncontrolledExpanded((value) => !value)
  }
  const showThreadList = () => {
    setShowCreate(false)
    setSelectedThreadUuid('')
    setInputText('')
    clearReferences()
    const next = new URLSearchParams(searchParams)
    next.delete('chat_thread_uuid')
    next.delete('workflow_uuid')
    next.delete('chat_new')
    next.delete('chat_reference_type')
    next.delete('chat_reference_uuid')
    next.delete('chat_reference_title')
    setSearchParams(next, { replace: true })
  }
  const startNewThread = () => {
    setSelectedThreadUuid('')
    setShowCreate(true)
    setInputText('')
    clearReferences()
    setError(null)
    const next = new URLSearchParams(searchParams)
    next.delete('chat_thread_uuid')
    next.delete('workflow_uuid')
    next.set('chat_new', '1')
    next.delete('chat_reference_type')
    next.delete('chat_reference_uuid')
    next.delete('chat_reference_title')
    setSearchParams(next, { replace: true })
  }
  const chooseThread = (uuid) => {
    setSelectedThreadUuid(uuid)
    setShowCreate(false)
    setInputText('')
    clearReferences()
    const next = new URLSearchParams(searchParams)
    next.set('chat_thread_uuid', uuid)
    next.delete('workflow_uuid')
    next.delete('chat_new')
    next.delete('chat_reference_type')
    next.delete('chat_reference_uuid')
    next.delete('chat_reference_title')
    setSearchParams(next, { replace: true })
  }
  const send = (mode) => {
    const text = inputText.trim()
    if (!text || composerMutation.isPending || referenceBlocked) return
    composerMutation.mutate({ mode, text, referenceInputs: readyProjectChatReferences(references) })
  }

  if (!expanded) {
    return (
      <aside className="chat-area chat-area--collapsed" aria-label={t('chat.project')}>
        <button className="chat-collapsed-rail" type="button" onClick={toggleExpanded} aria-label={t('chat.expand')} aria-expanded="false" title={t('chat.expand')}><PanelRightOpen size={18} aria-hidden="true" /><span>{t('chat.title')}</span></button>
      </aside>
    )
  }

  if (showCreate) {
    return <aside className={`chat-area chat-area--expanded ${overlay ? 'chat-area--overlay' : ''}`} aria-label={t('chat.project')}><NewThreadDraft projectUuid={projectUuid} draft={inputText} pending={createThreadMutation.isPending} error={error || createThreadMutation.error} overlay={overlay} references={references} referenceBlocked={referenceBlocked} onDraftChange={setInputText} onSubmit={() => createThreadMutation.mutate()} onBack={showThreadList} onToggle={toggleExpanded} onDismissError={() => setError(null)} onAddFiles={addAttachmentFiles} onRemoveReference={removeReference} onToggleReference={toggleReference} onPaste={handleAttachmentPaste} /></aside>
  }

  if (!selectedThreadUuid) {
    return <aside className={`chat-area chat-area--expanded ${overlay ? 'chat-area--overlay' : ''}`} aria-label={t('chat.project')}><ThreadList projectUuid={projectUuid} threads={threads} workflows={workflows} total={threadTotal} loading={threadsQuery.isLoading} loadingMore={threadsQuery.isFetchingNextPage} hasMore={threadsQuery.hasNextPage} error={threadsQuery.error} overlay={overlay} onToggle={toggleExpanded} onNewThread={startNewThread} onOpenThread={chooseThread} onLoadMore={() => threadsQuery.fetchNextPage()} /></aside>
  }

  return (
    <aside className={`chat-area chat-area--expanded ${overlay ? 'chat-area--overlay' : ''}`} aria-label={t('chat.project')}>
      <div className="chat-panel chat-panel--detail">
        <header className="chat-detail-header">
          <button className="chat-back" type="button" onClick={showThreadList} aria-label={t('chat.thread.back')}><ArrowLeft size={17} /></button>
          <div><p>{t('chat.title')}</p><h2>{selectedThread ? threadDisplayTitle(selectedThread, selectedWorkflow, t) : t('chat.threads')}</h2></div>
          <div className="chat-detail-actions">
            <span className={`chat-status chat-status--${statusClass(selectedThread?.status)}`}>{threadStatusCopy[selectedThread?.status] ? t(threadStatusCopy[selectedThread.status]) : selectedThread?.status ? t('common.status.unknown_with_code', { code: selectedThread.status }) : t('chat.loading')}</span>
            {selectedThread ? (
              <a
                className="chat-detail__trajectory-link"
                href={threadTrajectoryHref(projectUuid, selectedThread.uuid)}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={t('trajectory.thread.open', { title: threadDisplayTitle(selectedThread, selectedWorkflow, t) })}
                title={t('trajectory.thread.open', { title: threadDisplayTitle(selectedThread, selectedWorkflow, t) })}
              ><RouteIcon size={15} aria-hidden="true" /></a>
            ) : null}
            <CollapseButton overlay={overlay} onToggle={toggleExpanded} />
          </div>
        </header>
        <div className="chat-detail-body">
          <div className="chat-messages" ref={messagesRef} onScroll={handleMessagesScroll}>
            {itemsQuery.hasNextPage || itemsQuery.isFetchingNextPage ? <div className="chat-history-loader"><button type="button" className="button-quiet" disabled={!itemsQuery.hasNextPage || itemsQuery.isFetchingNextPage} onClick={loadEarlierMessages}>{t(itemsQuery.isFetchingNextPage ? 'chat.messages.loading_earlier' : 'chat.messages.load_earlier')}</button></div> : null}
            <ErrorNotice error={error || itemsQuery.error || turnsQuery.error || workflowsQuery.error} onDismiss={() => setError(null)} />
            <WorkflowProgress projectUuid={projectUuid} workflow={selectedWorkflow} pending={workflowMutation.isPending || workflowConflictMutation.isPending} onCancel={(uuid) => workflowMutation.mutate({ workflowUuid: uuid, action: 'cancel' })} onRetry={(uuid) => workflowMutation.mutate({ workflowUuid: uuid, action: 'retry' })} onResolveConflict={(uuid, action, expectedRevision) => workflowConflictMutation.mutate({ workflowUuid: uuid, action, expectedRevision })} />
            {itemsQuery.isLoading || turnsQuery.isLoading ? <p className="chat-muted">{t('chat.messages.loading')}</p> : null}
            {!itemsQuery.isLoading && !turnsQuery.isLoading && !turnGroups.length ? <div className="chat-empty-state"><strong>{t('chat.messages.empty')}</strong><span>{t('chat.messages.empty_body')}</span></div> : null}
            {turnGroups.map((group, index) => <TurnGroup key={group.uuid} group={group} projectUuid={projectUuid} historyMayBePartial={Boolean(index === 0 && itemsQuery.hasNextPage && !group.items.some((item) => item.item_type === 'user_message'))} requestByItemUuid={requestByItemUuid} inputPending={inputMutation.isPending} onRespond={(requestUuid, payload) => inputMutation.mutate({ requestUuid, payload })} onCancel={(requestUuid) => inputMutation.mutate({ requestUuid, cancel: true })} />)}
            {requests.filter((request) => request.status === 'pending' && !inlineRequestUuids.has(request.uuid)).map((request) => <UserInputCard key={request.uuid} request={request} pending={inputMutation.isPending} onRespond={(requestUuid, payload) => inputMutation.mutate({ requestUuid, payload })} onCancel={(requestUuid) => inputMutation.mutate({ requestUuid, cancel: true })} />)}
          </div>
          <div className="chat-composer-shell">
            <FollowUpQueue projectUuid={projectUuid} items={followUps} pending={followMutation.isPending} canSteer={activeTurn?.status === 'in_progress'} notice={queueNotice} onMove={(uuid, position) => followMutation.mutate({ uuid, position })} onDelete={(uuid) => followMutation.mutate({ uuid, action: 'delete' })} onEdit={(uuid, text) => followMutation.mutate({ uuid, text, action: 'edit' })} onSteer={(uuid) => followMutation.mutate({ uuid, action: 'steer' })} />
            <ChatComposer projectUuid={projectUuid} activeTurn={activeTurn} draft={inputText} pending={composerMutation.isPending} abortPending={abortMutation.isPending} references={references} referenceBlocked={referenceBlocked} onDraftChange={setInputText} onSend={send} onAbort={() => abortMutation.mutate()} onAddFiles={addAttachmentFiles} onRemoveReference={removeReference} onToggleReference={toggleReference} onPaste={handleAttachmentPaste} />
          </div>
        </div>
      </div>
    </aside>
  )
}

function statusClass(status) {
  return String(status || 'unknown').replace(/[^a-z0-9_-]/gi, '_')
}

function turnActivityCopy(activity, t) {
  const runningTool = activity.activeTool
  const toolVariables = { tool_name: runningTool?.toolName || 'controlled_tool' }
  if (runningTool?.status === 'running' && runningTool.toolName === 'image_gen') return t('chat.activity.image_gen', toolVariables)
  if (runningTool?.status === 'running' && ['request_api', 'request_current_project_api'].includes(runningTool.toolName)) {
    const activityKey = currentProjectAPIActivityKey(runningTool.call?.content)
    if (activityKey) return t(activityKey, toolVariables)
  }
  if (runningTool?.status === 'running' && ['create_premise_asset', 'update_premise_asset'].includes(runningTool.toolName)) return t('chat.activity.writeback', toolVariables)
  if (runningTool?.status === 'running' || runningTool?.status === 'pending') return t('chat.activity.tool_running', toolVariables)
  if (runningTool) return t('chat.activity.finalizing')
  return t('chat.activity.thinking')
}

function currentProjectAPIActivityKey(content) {
  try {
    const method = JSON.parse(content)?.method
    return {
      GET: 'chat.activity.asset_read',
      POST: 'chat.activity.asset_create',
      PATCH: 'chat.activity.asset_update',
      DELETE: 'chat.activity.asset_trash',
    }[method] || null
  } catch {
    return null
  }
}

function copyText(value) {
  if (!value || typeof navigator === 'undefined' || !navigator.clipboard?.writeText) return
  navigator.clipboard.writeText(value).catch(() => {})
}

function releaseAttachmentPreview(attachment) {
  if (attachment?.previewUrl && typeof URL !== 'undefined' && URL.revokeObjectURL) URL.revokeObjectURL(attachment.previewUrl)
}

function releaseAttachmentPreviews(attachments) {
  attachments.forEach(releaseAttachmentPreview)
}

function uniqueByUUID(items) {
  const seen = new Set()
  return items.filter((item) => item?.uuid && !seen.has(item.uuid) && seen.add(item.uuid))
}

function prettyDiagnosticJSON(value) {
  if (typeof value === 'string') {
    try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
  }
  try { return JSON.stringify(value || {}, null, 2) } catch { return '{}' }
}
