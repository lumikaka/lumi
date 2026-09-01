import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import MarkdownPreview from '../components/MarkdownPreview.jsx'
import LumiDialog from '../components/LumiDialog.jsx'
import PromptCatalogEditor from '../components/PromptCatalogEditor.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { createStoryProfileGeneration, createStoryProfileReconstruction, listTasks } from '../api/ai.js'
import {
  getStoryProfile,
  importExternalStoryMD,
  listStoryProfileVersions,
  regenerateStoryMD,
  updateStoryProfile,
} from '../api/story.js'
import { useI18n } from '../i18n/useI18n.js'
import { projectionStateLabel, sourceTypeLabel, statusLabel as localizedStatusLabel } from '../i18n/labels.js'
import { ACTIVE_TASK_STATUSES } from './aiRuntimeState.js'
import { formatTerminologyMessageKey } from './pictureBookProfile.js'
import { saveStateForError } from './storyWorkspaceState.js'

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

export function StoryConflictNotice({ profile, onImport, onRegenerate, pending = false }) {
  const { t } = useI18n()
  if (!['conflict', 'pending'].includes(profile?.projection_state)) return null
  if (profile.projection_state === 'pending') {
    return (
      <div className="story-conflict" role="status">
        <div><strong>{t('story.projection.pending.title')}</strong><p>{t('story.projection.pending.body')}</p></div>
        <button type="button" onClick={onRegenerate} disabled={pending}>{t('story.projection.pending.action')}</button>
      </div>
    )
  }
  return (
    <div className="story-conflict" role="alert">
      <div><strong>{t('story.projection.conflict.title')}</strong><p>{t('story.projection.conflict.body')}</p></div>
      <button type="button" onClick={onImport} disabled={pending}>{t('story.projection.conflict.import')}</button>
      <button type="button" className="button-secondary" onClick={onRegenerate} disabled={pending}>{t('story.projection.conflict.regenerate')}</button>
    </div>
  )
}

function StoryProfileEditorDialog({ profile, value, setValue, preview, setPreview, saveState, error, pending, onSave, onClose, onReload }) {
  const { t } = useI18n()
  const dirty = value !== profile.story_md
  return (
    <LumiDialog className="overview-profile-dialog" dismissDisabled={pending} onClose={onClose}>
      <form onSubmit={(event) => { event.preventDefault(); onSave() }}>
        <header className="lumi-dialog__header">
          <div><h2>{t('story.profile.edit_title')}</h2><p>{t('story.profile.edit_body')}</p></div>
          <button type="button" className="button-quiet" aria-label={t('common.action.close')} disabled={pending} onClick={onClose}>×</button>
        </header>
        <div className="lumi-dialog__body overview-profile-dialog__body">
          <div className="editor-toolbar"><ViewToggle preview={preview} setPreview={setPreview} /><SaveState state={saveState} /></div>
          <ErrorNotice error={error} />
          {saveState === 'conflict' ? <div className="workspace-notice"><div><strong>{t('story.profile.conflict_title')}</strong><span>{t('story.profile.conflict_body')}</span></div><button type="button" className="button-secondary" onClick={onReload}>{t('story.profile.reload_server')}</button></div> : null}
          {preview ? <MarkdownPreview value={value} /> : <textarea className="story-editor story-editor--profile" value={value} onChange={(event) => setValue(event.target.value)} aria-label="STORY.md" autoFocus />}
        </div>
        <div className="lumi-dialog__actions">
          <button type="button" className="button-secondary" disabled={pending} onClick={onClose}>{t('common.action.cancel')}</button>
          <button type="submit" disabled={pending || !dirty || saveState === 'conflict' || profile.projection_state !== 'synced'}>{t(pending ? 'common.status.saving' : 'common.action.save')}</button>
        </div>
      </form>
    </LumiDialog>
  )
}

export function StoryProfilePanel({ projectUuid, pictureBook, panelId = 'overview-panel-profile', labelledBy = 'overview-tab-profile' }) {
  const { formatCount, formatDateTime, t } = useI18n()
  const term = (key, values) => t(formatTerminologyMessageKey(pictureBook, key), values)
  const queryClient = useQueryClient()
  const profileQuery = useQuery({ queryKey: ['story-profile', projectUuid], queryFn: () => getStoryProfile(projectUuid), refetchOnWindowFocus: true })
  const historyQuery = useQuery({ queryKey: ['story-profile-history', projectUuid], queryFn: () => listStoryProfileVersions(projectUuid) })
  const tasksQuery = useQuery({
    queryKey: ['story-tasks', projectUuid],
    queryFn: () => listTasks(projectUuid, { limit: 100 }),
  })
  const [storyMD, setStoryMD] = useState('')
  const [editing, setEditing] = useState(false)
  const [preview, setPreview] = useState(false)
  const [saveState, setSaveState] = useState('idle')
  const [error, setError] = useState(null)
  const [generationPrompt, setGenerationPrompt] = useState('')
  const loadedRevision = useRef(0)
  const completedTaskRef = useRef('')
  const profileTask = (tasksQuery.data?.items || []).find((task) => task.kind === 'story_profile_generation' || task.kind === 'story_profile_from_chapters')
  const profileTaskActive = Boolean(profileTask && ACTIVE_TASK_STATUSES.has(profileTask.status))

  useEffect(() => {
    if (profileTask?.status !== 'completed' || completedTaskRef.current === profileTask.uuid) return
    completedTaskRef.current = profileTask.uuid
    queryClient.invalidateQueries({ queryKey: ['story-profile', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['story-profile-history', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['story-chapters', projectUuid] })
  }, [profileTask?.status, profileTask?.uuid, projectUuid, queryClient])

  useEffect(() => {
    if (!editing && profileQuery.data && loadedRevision.current !== profileQuery.data.revision) {
      loadedRevision.current = profileQuery.data.revision
      setStoryMD(profileQuery.data.story_md)
      setSaveState('idle')
    }
  }, [editing, profileQuery.data])

  const applyProfile = (profile) => {
    queryClient.setQueryData(['story-profile', projectUuid], profile)
    queryClient.invalidateQueries({ queryKey: ['story-profile-history', projectUuid] })
    loadedRevision.current = profile.revision
    setStoryMD(profile.story_md)
    setSaveState('saved')
    setEditing(false)
    setError(null)
  }
  const saveMutation = useMutation({
    mutationFn: () => updateStoryProfile(projectUuid, { story_md: storyMD, expected_revision: profileQuery.data.revision }),
    onMutate: () => setSaveState('saving'),
    onSuccess: applyProfile,
    onError: (mutationError) => { setError(mutationError); setSaveState(saveStateForError(mutationError)); profileQuery.refetch() },
  })
  const importMutation = useMutation({ mutationFn: () => importExternalStoryMD(projectUuid, profileQuery.data.revision), onSuccess: applyProfile, onError: setError })
  const regenerateMutation = useMutation({ mutationFn: () => regenerateStoryMD(projectUuid, profileQuery.data.revision), onSuccess: applyProfile, onError: setError })
  const aiGenerateMutation = useMutation({
    mutationFn: () => createStoryProfileGeneration(projectUuid, { prompt: generationPrompt.trim(), chapter_count: 1, parameters: { temperature: 0.7 }, idempotency_key: `story-profile-${Date.now()}` }),
    onSuccess: () => { setGenerationPrompt(''); queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] }); setError(null) },
    onError: setError,
  })
  const aiReconstructMutation = useMutation({
    mutationFn: () => createStoryProfileReconstruction(projectUuid, { parameters: { temperature: 0.4 }, idempotency_key: `story-profile-from-chapters-${Date.now()}` }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] }); setError(null) },
    onError: setError,
  })
  const profile = profileQuery.data

  if (profileQuery.isLoading) return <p className="workspace-loading">{t('story.profile.checking')}</p>
  if (profileQuery.isError && !profile) return <ErrorNotice error={profileQuery.error} />
  const openEditor = () => {
    loadedRevision.current = profile.revision
    setStoryMD(profile.story_md)
    setPreview(false)
    setSaveState('idle')
    setError(null)
    setEditing(true)
  }
  const closeEditor = () => {
    if (saveMutation.isPending) return
    loadedRevision.current = profile.revision
    setStoryMD(profile.story_md)
    setSaveState('idle')
    setError(null)
    setEditing(false)
  }
  const reloadEditor = () => {
    loadedRevision.current = profile.revision
    setStoryMD(profile.story_md)
    setSaveState('idle')
    setError(null)
  }
  const versions = historyQuery.data?.items || []
  return (
    <div className="workspace-stack project-overview" role="tabpanel" id={panelId} aria-labelledby={labelledBy}>
      {!editing ? <ErrorNotice error={error || tasksQuery.error} onDismiss={() => setError(null)} /> : null}
      <StoryConflictNotice profile={profile} pending={importMutation.isPending || regenerateMutation.isPending} onImport={() => importMutation.mutate()} onRegenerate={() => regenerateMutation.mutate()} />
      <section className="overview-card overview-profile-card">
        <header className="overview-card__header">
          <div><h1>{t('story.profile')}</h1><p>{term('story.profile.context_body')}</p></div>
          <button type="button" className="button-secondary" onClick={openEditor} disabled={profile.projection_state !== 'synced'}>{t('common.action.edit')}</button>
        </header>
        <pre className="overview-profile-source" data-user-content>{profile.story_md || t('story.profile.empty')}</pre>
        <div className="image-action-row">
          <input value={generationPrompt} onChange={(event) => setGenerationPrompt(event.target.value)} placeholder={t('story.profile.generate_placeholder')} />
          <button type="button" disabled={!generationPrompt.trim() || profileTaskActive || aiGenerateMutation.isPending} onClick={() => aiGenerateMutation.mutate()}>{t('story.profile.generate')}</button>
          <button type="button" className="button-secondary" disabled={profileTaskActive || aiReconstructMutation.isPending} onClick={() => aiReconstructMutation.mutate()}>{term('story.profile.reconstruct')}</button>
        </div>
        {profileTask ? <p className={`task-status task-status--${profileTask.status}`}>{t('story.profile.task', { status: localizedStatusLabel(t, profileTask.status), progress: profileTask.progress })}</p> : null}
        <dl className="overview-profile-facts">
          <div><dt>STORY.md</dt><dd>v{profile.version_no}</dd></div>
          <div><dt>{t('story.profile.file_status')}</dt><dd>{projectionStateLabel(t, profile.projection_state)}</dd></div>
          <div><dt>{t('common.label.source')}</dt><dd>{sourceTypeLabel(t, profile.source_type)}</dd></div>
          <div><dt>{t('story.profile.revision')}</dt><dd>r{profile.revision}</dd></div>
        </dl>
      </section>
      <section className="overview-card overview-profile-history">
        <header className="overview-card__header"><div><h2>{t('story.profile.history')}</h2></div><span>{formatCount('common.count.items', versions.length)}</span></header>
        <div>{versions.map((version) => <article key={version.uuid}><strong>v{version.version_no}</strong><span>{sourceTypeLabel(t, version.source_type)}</span><small>r{version.revision} · {projectionStateLabel(t, version.projection_state)}</small><time dateTime={version.created_at}>{formatDateTime(version.created_at)}</time></article>)}{!historyQuery.isLoading && versions.length === 0 ? <p className="workspace-empty">{t('story.profile.history_empty')}</p> : null}</div>
      </section>
      {editing ? <StoryProfileEditorDialog profile={profile} value={storyMD} setValue={(value) => { setStoryMD(value); if (saveState === 'saved') setSaveState('idle') }} preview={preview} setPreview={setPreview} saveState={saveState} error={error} pending={saveMutation.isPending} onSave={() => saveMutation.mutate()} onClose={closeEditor} onReload={reloadEditor} /> : null}
    </div>
  )
}

export function PromptPanel({ projectUuid, pictureBook, panelId = 'overview-panel-prompts', labelledBy = 'overview-tab-prompts' }) {
  return (
    <div className="workspace-stack project-overview" role="tabpanel" id={panelId} aria-labelledby={labelledBy}>
      <PromptCatalogEditor projectUuid={projectUuid} pictureBook={pictureBook} />
    </div>
  )
}
