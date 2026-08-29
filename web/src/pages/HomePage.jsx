import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowUpDown, FolderOpen, ImagePlus, MoreHorizontal, Plus, Search, X } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import AppPageShell from '../components/AppPageShell.jsx'
import LumiDialog from '../components/LumiDialog.jsx'
import PictureBookProfileFields from '../components/PictureBookProfileFields.jsx'
import { ReferenceStrip } from '../components/ChatReferences.jsx'
import { projectStatusCopy } from '../components/RecentProjectsView.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { createYoloWorkflow } from '../api/chat.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import {
  overallStyleForLanguage,
  overallStyleUsesDefault,
  projectCreationErrors,
  projectDefaultOverallStyle,
} from './projectCreationForm.js'
import { projectRowActions, projectRowPrimaryAction } from './projectIndexState.js'
import {
  createProject,
  createProjectCreationSession,
  forgetRecentProject,
  getProjectCreationSession,
  getProjectDefaults,
  listOpenProjects,
  listRecentProjects,
  openProjectPath,
  preflightImageGeneration,
  revealProjectDirectory,
  relocateRecentProject,
  retryProjectCreationSession,
  selectProjectDirectory,
  uploadProjectCreationReference,
} from '../api/projects.js'
import { defaultPictureBookDraft, pictureBookDraftIsValid, pictureBookPayload } from './pictureBookProfile.js'
import {
  MAX_PROJECT_CHAT_REFERENCES,
  projectChatClipboardFiles,
  selectProjectChatClipboardImages,
  selectProjectChatImageFiles,
} from './projectChatAttachments.js'

const SORT_OPTIONS = [
  { value: 'recent', labelKey: 'projects.index.sort.recent' },
  { value: 'oldest', labelKey: 'projects.index.sort.oldest' },
  { value: 'name_asc', labelKey: 'projects.index.sort.name_asc' },
  { value: 'name_desc', labelKey: 'projects.index.sort.name_desc' },
]

const CREATION_CHECKPOINT_KEY = 'lumi.homeProjectCreation'

function loadCreationCheckpoint() {
  if (typeof window === 'undefined') return null
  try {
    const value = JSON.parse(window.sessionStorage.getItem(CREATION_CHECKPOINT_KEY) || 'null')
    return value && typeof value.inputText === 'string' && typeof value.idempotencyKey === 'string' ? value : null
  } catch { return null }
}

function saveCreationCheckpoint(value) {
  if (typeof window === 'undefined') return
  try {
    if (value) window.sessionStorage.setItem(CREATION_CHECKPOINT_KEY, JSON.stringify(value))
    else window.sessionStorage.removeItem(CREATION_CHECKPOINT_KEY)
  } catch { /* restricted browser */ }
}

function newCreationIdempotencyKey() {
  return `home-project-${globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(36).slice(2)}`}`
}

function releaseCreationReferencePreview(reference) {
  if (reference?.previewUrl?.startsWith?.('blob:') && typeof URL !== 'undefined' && URL.revokeObjectURL) URL.revokeObjectURL(reference.previewUrl)
}

function creationReferenceFileInput(reference) {
  return {
    original_filename: reference.filename,
    mime_type: reference.mimeType,
    byte_size: reference.byteSize,
  }
}

function creationReferenceManifest(references) {
  return references.map(creationReferenceFileInput)
}

function sameCreationReferenceManifest(left = [], right = []) {
  return JSON.stringify(left) === JSON.stringify(right)
}

function creationReferencesFromCheckpoint(checkpoint) {
  if (!Array.isArray(checkpoint?.referenceFiles)) return []
  return checkpoint.referenceFiles.map((reference, index) => ({
    localId: `checkpoint-reference-${index + 1}`,
    position: index + 1,
    resource_type: 'file', resource_uuid: '', image_file_uuid: '', image_available: false,
    title: reference.original_filename,
    filename: reference.original_filename,
    mimeType: reference.mime_type,
    byteSize: reference.byte_size,
    status: 'missing', error: null, file: null, previewUrl: '',
  }))
}

function mergeCreationSessionReferences(current, session) {
  if (!session?.references?.length) return current
  const byUuid = new Map(current.map((item) => [item.referenceUuid, item]))
  const byPosition = new Map(current.map((item, index) => [item.position || index + 1, item]))
  return session.references.map((reference) => {
    const existing = byUuid.get(reference.uuid) || byPosition.get(reference.position)
    const ready = reference.status === 'ready' && reference.file_uuid
    if (ready && existing?.previewUrl) releaseCreationReferencePreview(existing)
    return {
      ...existing,
      localId: reference.uuid,
      referenceUuid: reference.uuid,
      position: reference.position,
      resource_type: 'file',
      resource_uuid: ready ? reference.file_uuid : '',
      image_file_uuid: ready ? reference.file_uuid : '',
      image_available: Boolean(ready),
      title: reference.original_filename,
      filename: reference.original_filename,
      mimeType: reference.mime_type,
      byteSize: reference.byte_size,
      status: ready ? 'ready' : reference.status === 'failed' ? 'error' : existing?.file ? existing.status === 'uploading' ? 'uploading' : 'selected' : 'missing',
      previewUrl: ready ? '' : existing?.previewUrl || '',
      error: reference.error_code ? { code: reference.error_code } : existing?.error || null,
    }
  })
}

function useProjectMutation(queryClient, setActionError, mutationFn, afterSuccess) {
  return useMutation({
    mutationFn,
    onSuccess: (data) => {
      afterSuccess?.(data)
      setActionError(null)
      queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
      queryClient.invalidateQueries({ queryKey: projectQueryKeys.openProjects() })
    },
    onError: setActionError,
  })
}

export default function HomePage() {
  const { formatDateTime, locale, t } = useI18n()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState('recent')
  const [dialog, setDialog] = useState('')
  const [createMode, setCreateMode] = useState('yolo')
  const [name, setName] = useState('')
  const [parentPath, setParentPath] = useState('')
  const [parentPathDirty, setParentPathDirty] = useState(false)
  const [generationLanguage, setGenerationLanguage] = useState('zh-Hans')
  const [overallStyle, setOverallStyle] = useState('')
  const [overallStyleDirty, setOverallStyleDirty] = useState(false)
  const [pictureBookDraft, setPictureBookDraft] = useState(defaultPictureBookDraft)
  const [existingPath, setExistingPath] = useState('')
  const [targetProject, setTargetProject] = useState(null)
  const [relocatePath, setRelocatePath] = useState('')
  const [storyPrompt, setStoryPrompt] = useState('')
  const [createValidationAttempted, setCreateValidationAttempted] = useState(false)
  const [openMenuUuid, setOpenMenuUuid] = useState('')
  const [actionError, setActionError] = useState(null)
  const [creationCheckpoint, setCreationCheckpoint] = useState(loadCreationCheckpoint)
  const [creationInput, setCreationInput] = useState(() => loadCreationCheckpoint()?.inputText || '')
  const [creationSession, setCreationSession] = useState(null)
  const [creationReferences, setCreationReferences] = useState(() => creationReferencesFromCheckpoint(loadCreationCheckpoint()))
  const [creationReferenceError, setCreationReferenceError] = useState(null)
  const [creationValidationAttempted, setCreationValidationAttempted] = useState(false)
  const nameInputRef = useRef(null)
  const storyPromptInputRef = useRef(null)
  const parentPathInputRef = useRef(null)
  const overallStyleDetailsRef = useRef(null)
  const overallStyleInputRef = useRef(null)
  const createFormRef = useRef(null)
  const openMenuRef = useRef(null)
  const openMenuTriggerRef = useRef(null)
  const creationReferenceInputRef = useRef(null)
  const creationReferencesRef = useRef([])

  const recentQuery = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
  const openProjectsQuery = useQuery({ queryKey: projectQueryKeys.openProjects(), queryFn: listOpenProjects })
  const projectDefaultsQuery = useQuery({ queryKey: ['project-defaults'], queryFn: getProjectDefaults })
  const creationSessionQuery = useQuery({
    queryKey: ['project-creation-session', creationCheckpoint?.sessionUuid || ''],
    queryFn: () => getProjectCreationSession(creationCheckpoint.sessionUuid),
    enabled: Boolean(creationCheckpoint?.sessionUuid),
    retry: false,
  })
  const defaultProjectParentPath = projectDefaultsQuery.data?.parent_path || ''
  const defaultOverallStyles = projectDefaultsQuery.data?.default_overall_styles || null
  const defaultOverallStyle = projectDefaultOverallStyle(defaultOverallStyles, generationLanguage)
  const usingDefaultOverallStyle = overallStyleUsesDefault(overallStyle, defaultOverallStyle)
  const pictureBook = useMemo(() => pictureBookPayload(pictureBookDraft), [pictureBookDraft])
  const pictureBookValid = pictureBookDraftIsValid(pictureBookDraft)

  useEffect(() => {
    creationReferencesRef.current = creationReferences
  }, [creationReferences])

  useEffect(() => () => {
    creationReferencesRef.current.forEach(releaseCreationReferencePreview)
  }, [])

  useEffect(() => {
    if (!parentPathDirty && defaultProjectParentPath) setParentPath(defaultProjectParentPath)
  }, [defaultProjectParentPath, parentPathDirty])

  useEffect(() => {
    setOverallStyle((currentStyle) => overallStyleForLanguage({ currentStyle, dirty: overallStyleDirty, defaultOverallStyles, generationLanguage }))
  }, [defaultOverallStyles, generationLanguage, overallStyleDirty])

  const resetCreationFields = () => {
    const resetLanguage = 'zh-Hans'
    setCreateMode('yolo')
    setName('')
    setParentPath(defaultProjectParentPath)
    setParentPathDirty(false)
    setGenerationLanguage(resetLanguage)
    setOverallStyle(projectDefaultOverallStyle(defaultOverallStyles, resetLanguage))
    setOverallStyleDirty(false)
    setStoryPrompt('')
    setCreateValidationAttempted(false)
    setPictureBookDraft(defaultPictureBookDraft())
  }
  const closeDialog = () => {
		if (dialog === 'create') resetCreationFields()
    setDialog('')
    setTargetProject(null)
    setRelocatePath('')
  }
  const enterProject = (project) => navigate(`/projects/${encodeURIComponent(project.uuid)}`)
  const createMutation = useProjectMutation(queryClient, setActionError, createProject, (project) => { resetCreationFields(); closeDialog(); enterProject(project) })
  const openPathMutation = useProjectMutation(queryClient, setActionError, openProjectPath, (project) => { setExistingPath(''); closeDialog(); enterProject(project) })
  const selectDirectoryMutation = useMutation({
    mutationFn: selectProjectDirectory,
    onSuccess: (selection) => {
      if (selection?.root_path) setExistingPath(selection.root_path)
      setActionError(null)
    },
    onError: setActionError,
  })
  const revealDirectoryMutation = useMutation({
    mutationFn: revealProjectDirectory,
    onSuccess: () => setActionError(null),
    onError: setActionError,
  })
  const relocateMutation = useProjectMutation(queryClient, setActionError, relocateRecentProject, () => closeDialog())
  const forgetMutation = useProjectMutation(queryClient, setActionError, forgetRecentProject, () => closeDialog())
  const yoloMutation = useMutation({
    mutationFn: async () => {
			await preflightImageGeneration(pictureBook)
			const project = await createProject({ name, parentPath, generationLanguage, pictureBook, overallStyle })
      const workflow = await createYoloWorkflow(project.uuid, {
        title: name,
        story_prompt: storyPrompt,
        idempotency_key: `yolo-${Date.now()}-${Math.random().toString(36).slice(2)}`,
      })
      return { project, workflow }
    },
    onSuccess: ({ project, workflow }) => {
      setActionError(null)
			resetCreationFields()
      closeDialog()
      queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
      queryClient.invalidateQueries({ queryKey: projectQueryKeys.openProjects() })
      navigate(`/projects/${encodeURIComponent(project.uuid)}?chat_thread_uuid=${encodeURIComponent(workflow.thread_uuid)}&workflow_uuid=${encodeURIComponent(workflow.uuid)}`)
    },
    onError: setActionError,
  })

  const acceptCreationSession = (session, checkpoint) => {
    const nextCheckpoint = { ...(checkpoint || creationCheckpoint || {}), sessionUuid: session.uuid }
    setCreationCheckpoint(nextCheckpoint)
    setCreationSession(session)
    setCreationReferences((current) => mergeCreationSessionReferences(current, session))
    saveCreationCheckpoint(nextCheckpoint)
    if (session.status !== 'active' || !session.project_uuid || !session.thread_uuid) return
    creationReferencesRef.current.forEach(releaseCreationReferencePreview)
    saveCreationCheckpoint(null)
    queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
    queryClient.invalidateQueries({ queryKey: projectQueryKeys.openProjects() })
    navigate(`/projects/${encodeURIComponent(session.project_uuid)}?chat_thread_uuid=${encodeURIComponent(session.thread_uuid)}`)
  }
  const updateCreationReference = (position, patch) => {
    setCreationReferences((current) => current.map((item, index) => (item.position || index + 1) === position ? { ...item, ...patch } : item))
  }
  const creationMutation = useMutation({
    mutationFn: async (variables) => {
      let session = variables.sessionUuid
        ? await retryProjectCreationSession(variables.sessionUuid)
        : await createProjectCreationSession({ inputText: variables.inputText, idempotencyKey: variables.idempotencyKey, referenceFiles: variables.referenceFiles })
      if (session.status !== 'active') acceptCreationSession(session, variables.checkpoint)
      if (session.status === 'active' || session.status === 'failed') return session
      const localByPosition = new Map(variables.references.map((item, index) => [item.position || index + 1, item]))
      let firstUploadError = null
      for (const reference of session.references || []) {
        if (reference.status === 'ready') continue
        const local = localByPosition.get(reference.position)
        if (!local?.file) continue
        updateCreationReference(reference.position, { status: 'uploading', error: null, referenceUuid: reference.uuid })
        try {
          session = await uploadProjectCreationReference(session.uuid, reference.uuid, local.file)
          if (session.status !== 'active') acceptCreationSession(session, variables.checkpoint)
        } catch (uploadError) {
          firstUploadError ||= uploadError
          updateCreationReference(reference.position, { status: 'error', error: uploadError, referenceUuid: reference.uuid })
        }
      }
      if (session.status === 'active') return session
      if (firstUploadError) {
        try {
          session = await getProjectCreationSession(session.uuid)
          if (session.status === 'active' || session.status === 'failed') return session
          acceptCreationSession(session, variables.checkpoint)
        } catch { /* preserve the original upload failure */ }
        throw firstUploadError
      }
      if (session.status !== 'active' && (session.references || []).every((reference) => reference.status === 'ready')) {
        session = await retryProjectCreationSession(session.uuid)
      }
      return session
    },
    onSuccess: (session, variables) => acceptCreationSession(session, variables.checkpoint),
  })

  useEffect(() => {
    if (creationSessionQuery.data) acceptCreationSession(creationSessionQuery.data, creationCheckpoint)
  }, [creationSessionQuery.data])

  const projects = useMemo(() => {
    const needle = search.trim().toLocaleLowerCase()
    const items = [...(recentQuery.data?.items || [])].filter((project) => !needle || project.name.toLocaleLowerCase().includes(needle) || project.root_path.toLocaleLowerCase().includes(needle))
    return items.sort((left, right) => {
      if (sort === 'oldest') return dateValue(left.last_opened_at) - dateValue(right.last_opened_at)
      if (sort === 'name_asc') return left.name.localeCompare(right.name, locale)
      if (sort === 'name_desc') return right.name.localeCompare(left.name, locale)
      return dateValue(right.last_opened_at) - dateValue(left.last_opened_at)
    })
  }, [locale, recentQuery.data, search, sort])

	const pageError = actionError || projectDefaultsQuery.error || recentQuery.error || openProjectsQuery.error
  const pending = createMutation.isPending || yoloMutation.isPending || openPathMutation.isPending || selectDirectoryMutation.isPending || relocateMutation.isPending || forgetMutation.isPending
  const currentCreateErrors = createValidationAttempted ? projectCreationErrors({ name, parentPath, storyPrompt, createMode, pictureBookValid, overallStyle }) : {}
  const selectCreateMode = (mode) => { setCreateMode(mode); setCreateValidationAttempted(false) }
  const submitCreateForm = (event) => {
    event.preventDefault()
    if (pending) return
    const errors = projectCreationErrors({ name, parentPath, storyPrompt, createMode, pictureBookValid, overallStyle })
    setCreateValidationAttempted(true)
    if (Object.keys(errors).length) {
      if (errors.name) nameInputRef.current?.focus()
      else if (errors.storyPrompt) storyPromptInputRef.current?.focus()
      else if (errors.parentPath) parentPathInputRef.current?.focus()
      else if (errors.overallStyle) {
        if (overallStyleDetailsRef.current) overallStyleDetailsRef.current.open = true
        overallStyleInputRef.current?.focus()
      } else createFormRef.current?.querySelector('.picture-book-custom-ratio input')?.focus()
      return
    }
    if (createMode === 'yolo') yoloMutation.mutate()
    else createMutation.mutate({ name, parentPath, generationLanguage, pictureBook, overallStyle })
  }
  const openCreateDialog = () => { setActionError(null); resetCreationFields(); setCreateMode('yolo'); setDialog('create') }
  const openExistingDialog = () => { setActionError(null); setDialog('open') }
  const creationPending = creationMutation.isPending
  const creationError = creationMutation.error || creationSessionQuery.error
  const unresolvedCreationReferences = creationReferences.filter((reference) => reference.status !== 'ready')
  const missingCreationReferenceFiles = unresolvedCreationReferences.filter((reference) => !reference.file).length
  const submitCreationComposer = (event) => {
    event.preventDefault()
    if (creationPending) return
    setCreationValidationAttempted(true)
    if (!creationInput.trim()) return
    const referenceFiles = creationReferenceManifest(creationReferences)
    const checkpoint = creationCheckpoint?.inputText === creationInput && sameCreationReferenceManifest(creationCheckpoint.referenceFiles, referenceFiles)
      ? creationCheckpoint
      : { inputText: creationInput, idempotencyKey: newCreationIdempotencyKey(), referenceFiles }
    setCreationCheckpoint(checkpoint)
    if (!checkpoint.sessionUuid) setCreationSession(null)
    saveCreationCheckpoint(checkpoint)
    creationMutation.mutate({ inputText: creationInput, idempotencyKey: checkpoint.idempotencyKey, referenceFiles: checkpoint.referenceFiles || referenceFiles, references: creationReferences, sessionUuid: checkpoint.sessionUuid, checkpoint })
  }
  const retryCreation = () => {
    if (creationPending || !creationCheckpoint) return
    creationMutation.mutate({
      inputText: creationCheckpoint.inputText,
      idempotencyKey: creationCheckpoint.idempotencyKey,
      referenceFiles: creationCheckpoint.referenceFiles || creationReferenceManifest(creationReferences),
      references: creationReferences,
      sessionUuid: creationCheckpoint.sessionUuid,
      checkpoint: creationCheckpoint,
    })
  }
  const changeCreationInput = (value) => {
    setCreationInput(value)
    setCreationValidationAttempted(false)
    if (value !== creationCheckpoint?.inputText) {
      setCreationSession(null)
      setCreationCheckpoint(null)
      setCreationReferences((current) => current.filter((reference) => reference.file).map((reference, index) => ({ ...reference, position: index + 1 })))
      saveCreationCheckpoint(null)
    }
  }
  const addCreationReferenceFiles = (files) => {
    setCreationReferenceError(null)
    const missingSlots = creationReferences.filter((reference) => reference.status !== 'ready' && !reference.file).length
    const refillLockedManifest = Array.isArray(creationCheckpoint?.referenceFiles) && missingSlots > 0
    const availableSlots = refillLockedManifest ? missingSlots : MAX_PROJECT_CHAT_REFERENCES - creationReferences.length
    const selection = selectProjectChatImageFiles(files, MAX_PROJECT_CHAT_REFERENCES - availableSlots)
    if (selection.rejectedNonImages) setCreationReferenceError({ code: 'chat_reference_invalid_mime' })
    else if (selection.exceededLimit) setCreationReferenceError({ code: 'chat_reference_limit_exceeded' })
    if (!selection.acceptedFiles.length) return
    if (refillLockedManifest) {
      const selected = [...selection.acceptedFiles]
      setCreationReferences((current) => current.map((reference) => {
        if (reference.status === 'ready' || reference.file || !selected.length) return reference
        const file = selected.shift()
        return { ...reference, file, status: 'selected', error: null, previewUrl: typeof URL !== 'undefined' && URL.createObjectURL ? URL.createObjectURL(file) : '' }
      }))
      return
    }
    const additions = selection.acceptedFiles.map((file, index) => {
      const filename = file.name?.trim() || t('chat.attachment.image')
      return {
        localId: globalThis.crypto?.randomUUID?.() || `${Date.now()}-${index}-${filename}`,
        position: creationReferences.length + index + 1,
        resource_type: 'file', resource_uuid: '', image_file_uuid: '', image_available: false,
        title: filename, filename, mimeType: file.type.toLowerCase(), byteSize: file.size,
        status: 'selected', error: null, file,
        previewUrl: typeof URL !== 'undefined' && URL.createObjectURL ? URL.createObjectURL(file) : '',
      }
    })
    setCreationReferences((current) => [...current, ...additions])
  }
  const removeCreationReference = (localId) => {
    const serverManifestLocked = Boolean(creationCheckpoint?.sessionUuid)
    if (!serverManifestLocked && Array.isArray(creationCheckpoint?.referenceFiles)) {
      setCreationCheckpoint(null)
      saveCreationCheckpoint(null)
    }
    setCreationReferences((current) => {
      const target = current.find((reference) => reference.localId === localId)
      if (!target || target.status === 'ready') return current
      releaseCreationReferencePreview(target)
      if (serverManifestLocked) return current.map((reference) => reference.localId === localId ? { ...reference, file: null, previewUrl: '', status: 'missing', error: null } : reference)
      return current.filter((reference) => reference.localId !== localId).map((reference, index) => ({ ...reference, position: index + 1 }))
    })
  }
  const handleCreationReferencePaste = (event) => {
    const selected = selectProjectChatClipboardImages(event.clipboardData, creationReferences.filter((reference) => reference.status === 'ready' || reference.file).length)
    if (!selected.hasImages) return
    event.preventDefault()
    addCreationReferenceFiles(projectChatClipboardFiles(event.clipboardData))
  }

  useEffect(() => {
    if (!openMenuUuid) return undefined
    const closeOpenMenuOnPointerDown = (event) => {
      if (!openMenuRef.current?.contains(event.target)) setOpenMenuUuid('')
    }
    const closeOpenMenuOnEscape = (event) => {
      if (event.key !== 'Escape') return
      setOpenMenuUuid('')
      openMenuTriggerRef.current?.focus()
    }
    document.addEventListener('pointerdown', closeOpenMenuOnPointerDown)
    document.addEventListener('keydown', closeOpenMenuOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeOpenMenuOnPointerDown)
      document.removeEventListener('keydown', closeOpenMenuOnEscape)
    }
  }, [openMenuUuid])

  return (
    <AppPageShell
      title={t('projects.title')}
      actions={<><button className="project-topbar__action" type="button" aria-label={t('projects.action.open_existing')} title={t('projects.action.open_existing')} onClick={openExistingDialog}><FolderOpen size={15} aria-hidden="true" /><span>{t('projects.action.open_existing')}</span></button><button className="project-topbar__action" type="button" aria-label={t('projects.action.new')} title={t('projects.action.new')} onClick={openCreateDialog}><Plus size={15} aria-hidden="true" /><span>{t('projects.action.new')}</span></button></>}
    >
      <div className="project-index-content">
        <section className="project-creation-composer" aria-labelledby="project-creation-composer-title">
          <div className="project-creation-composer__copy">
            <p className="story-eyebrow">{t('projects.conversation.eyebrow')}</p>
            <h2 id="project-creation-composer-title">{t('projects.conversation.title')}</h2>
            <p>{t('projects.conversation.body')}</p>
          </div>
          <form onSubmit={submitCreationComposer}>
            <label htmlFor="project-creation-input">{t('projects.conversation.label')}</label>
            <textarea id="project-creation-input" rows="4" value={creationInput} onChange={(event) => changeCreationInput(event.target.value)} onPaste={handleCreationReferencePaste} placeholder={t('projects.conversation.placeholder')} disabled={creationPending || Boolean(creationCheckpoint?.sessionUuid)} aria-invalid={creationValidationAttempted && !creationInput.trim() ? 'true' : undefined} aria-describedby="project-creation-hint" />
            {creationValidationAttempted && !creationInput.trim() ? <p className="project-field-error" role="alert">{t('projects.conversation.required')}</p> : null}
            <div className="project-creation-composer__attachments">
              <button type="button" className="button-secondary" disabled={creationPending || (Boolean(creationCheckpoint?.sessionUuid) && missingCreationReferenceFiles === 0)} onClick={() => creationReferenceInputRef.current?.click()}><ImagePlus size={16} aria-hidden="true" />{t('projects.conversation.reference.add')}</button>
              <input ref={creationReferenceInputRef} className="project-creation-composer__file-input" type="file" accept="image/png,image/jpeg,image/webp" multiple disabled={creationPending} onChange={(event) => { addCreationReferenceFiles(event.target.files); event.target.value = '' }} />
              <span>{t('projects.conversation.reference.count', { count: creationReferences.length, max: MAX_PROJECT_CHAT_REFERENCES })}</span>
            </div>
            <ReferenceStrip projectUuid={creationSession?.project_uuid || ''} references={creationReferences} onRemove={removeCreationReference} canRemove={(reference) => !creationPending && reference.status !== 'ready' && (!creationCheckpoint?.sessionUuid || Boolean(reference.file))} compact />
            <LocalizedErrorMessage error={creationReferenceError} className="project-creation-composer__reference-error" compact onDismiss={() => setCreationReferenceError(null)} />
            {creationSession?.status === 'awaiting_references' && missingCreationReferenceFiles > 0 ? <p className="project-creation-composer__reference-notice" role="status">{t('projects.conversation.reference.reselect', { count: missingCreationReferenceFiles })}</p> : null}
            <div className="project-creation-composer__footer">
              <p id="project-creation-hint">{t('projects.conversation.path_hint')}</p>
              <button type="submit" disabled={creationPending || !creationInput.trim() || (creationSession?.status === 'awaiting_references' && missingCreationReferenceFiles > 0)}><Plus size={16} aria-hidden="true" />{t(creationPending ? 'projects.conversation.creating' : creationSession?.status === 'awaiting_references' ? 'projects.conversation.reference.continue' : 'projects.conversation.submit')}</button>
            </div>
          </form>
          {creationError ? <div className="project-creation-composer__failure" role="alert"><LocalizedErrorMessage error={creationError} className="project-alert project-creation-composer__error" titleKey="projects.conversation.failed" /><button type="button" className="button-secondary" disabled={creationPending || !creationCheckpoint} onClick={retryCreation}>{t(creationPending ? 'projects.conversation.retrying' : 'common.action.retry')}</button></div> : null}
          {creationSession?.status === 'failed' ? <div className="project-creation-composer__failure" role="alert"><div><strong>{t('projects.conversation.failed')}</strong><p>{creationSession.error_message || t('errors.generic')}</p>{creationSession.error_code ? <code>{creationSession.error_code}</code> : null}</div><button type="button" className="button-secondary" disabled={creationPending} onClick={retryCreation}>{t(creationPending ? 'projects.conversation.retrying' : 'common.action.retry')}</button></div> : null}
        </section>
        <LocalizedErrorMessage error={pageError} className="project-alert project-index-alert" titleKey="projects.error.action_title" onDismiss={actionError ? () => setActionError(null) : undefined} />
        <section className="project-index-main">
          <div className="project-index-toolbar">
            <div><p className="story-eyebrow">{t('projects.index.eyebrow')}</p><h2>{t('projects.index.local')}</h2></div>
            <div className="project-index-toolbar__controls">
              <label className="project-index-search"><Search size={16} aria-hidden="true" /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t('projects.index.search_placeholder')} aria-label={t('projects.index.search_label')} /></label>
              <label className="project-index-sort"><ArrowUpDown size={16} aria-hidden="true" /><select value={sort} onChange={(event) => setSort(event.target.value)} aria-label={t('projects.index.sort_label')}>{SORT_OPTIONS.map((option) => <option key={option.value} value={option.value}>{t(option.labelKey)}</option>)}</select></label>
              <button className="story-button" type="button" onClick={openCreateDialog}><Plus size={16} />{t('projects.action.new')}</button>
            </div>
          </div>
          <div className="project-card-grid" role="list" aria-label={t('projects.recent')}>
            {recentQuery.isLoading ? <p className="story-muted project-index-loading">{t('projects.loading.index')}</p> : null}
            {!recentQuery.isLoading && projects.length === 0 ? <div className="story-empty project-index-empty">{t(search.trim() ? 'projects.index.empty_search' : 'projects.index.empty')}</div> : null}
            {projects.map((project) => (
              <ProjectRow
                key={project.uuid}
                project={project}
                menuOpen={openMenuUuid === project.uuid}
                menuRef={openMenuUuid === project.uuid ? openMenuRef : undefined}
                onToggleMenu={(event) => {
                  openMenuTriggerRef.current = event.currentTarget
                  setOpenMenuUuid((uuid) => uuid === project.uuid ? '' : project.uuid)
                }}
                onEnter={() => { setOpenMenuUuid(''); enterProject(project) }}
                onReveal={() => { setOpenMenuUuid(''); revealDirectoryMutation.mutate(project.root_path) }}
                onRelocate={() => { setTargetProject(project); setRelocatePath(project.root_path || ''); setDialog('relocate'); setOpenMenuUuid('') }}
                onForget={() => { setTargetProject(project); setDialog('forget'); setOpenMenuUuid('') }}
                formatDateTime={formatDateTime}
                t={t}
              />
            ))}
          </div>
          <p className="project-index-count">{t('projects.index.count', { shown: projects.length, total: recentQuery.data?.items?.length || 0 })}</p>
        </section>
      </div>

      {dialog === 'create' ? <Modal className="project-create-dialog" title={t('projects.dialog.create.title')} description={t('projects.dialog.create.description')} dismissDisabled={pending} onClose={closeDialog}>
        <div className="project-create-tabs"><button type="button" aria-pressed={createMode === 'manual'} onClick={() => selectCreateMode('manual')}>{t('projects.create.manual')}</button><button type="button" aria-pressed={createMode === 'yolo'} onClick={() => selectCreateMode('yolo')}>{t('projects.create.yolo')}</button></div>
		<form ref={createFormRef} className="project-dialog-form" noValidate onSubmit={submitCreateForm}>
          <div className="project-create-priority-fields">
            <div className="project-dialog-field">
              <label htmlFor="new-project-name">{t('projects.field.name')}<span className="project-field-required" aria-hidden="true"> *</span></label>
              <input ref={nameInputRef} id="new-project-name" value={name} onChange={(event) => setName(event.target.value)} placeholder={t('projects.field.name_placeholder')} required autoFocus aria-invalid={currentCreateErrors.name ? 'true' : undefined} aria-describedby={currentCreateErrors.name ? 'new-project-name-error' : undefined} />
              {currentCreateErrors.name ? <p className="project-field-error" id="new-project-name-error" role="alert">{t(currentCreateErrors.name)}</p> : null}
            </div>
            {createMode === 'yolo' ? <div className="project-dialog-field">
              <label htmlFor="new-project-story-idea">{t('projects.field.story_idea')}<span className="project-field-required" aria-hidden="true"> *</span></label>
              <textarea ref={storyPromptInputRef} id="new-project-story-idea" rows="5" value={storyPrompt} onChange={(event) => setStoryPrompt(event.target.value)} placeholder={t('projects.field.story_idea_placeholder')} required aria-invalid={currentCreateErrors.storyPrompt ? 'true' : undefined} aria-describedby={currentCreateErrors.storyPrompt ? 'new-project-story-idea-error' : undefined} />
              {currentCreateErrors.storyPrompt ? <p className="project-field-error" id="new-project-story-idea-error" role="alert">{t(currentCreateErrors.storyPrompt)}</p> : null}
            </div> : null}
          </div>
          <div className="project-dialog-field">
            <label htmlFor="new-project-parent-path">{t('projects.field.parent_path')}<span className="project-field-required" aria-hidden="true"> *</span></label>
            <input ref={parentPathInputRef} id="new-project-parent-path" value={parentPath} onChange={(event) => { setParentPath(event.target.value); setParentPathDirty(true) }} placeholder={t('projects.field.parent_path_placeholder')} required aria-invalid={currentCreateErrors.parentPath ? 'true' : undefined} aria-describedby={currentCreateErrors.parentPath ? 'new-project-parent-path-error' : undefined} />
            {currentCreateErrors.parentPath ? <p className="project-field-error" id="new-project-parent-path-error" role="alert">{t(currentCreateErrors.parentPath)}</p> : null}
          </div>
		  <label>{t('projects.field.generation_language')}<select value={generationLanguage} onChange={(event) => setGenerationLanguage(event.target.value)}><option value="zh-Hans">{t('common.language.zh_hans')}</option><option value="en">{t('common.language.en')}</option></select></label>
		  <details ref={overallStyleDetailsRef} className="project-overall-style">
		    <summary><span>{t('projects.field.overall_style')}</span><small>{t(usingDefaultOverallStyle ? 'projects.field.overall_style_default' : 'projects.field.overall_style_custom')}</small></summary>
		    <div className="project-overall-style__editor">
		      <p>{t('projects.field.overall_style_hint')}</p>
		      <label htmlFor="new-project-overall-style">{t('projects.field.overall_style')}</label>
		      <textarea ref={overallStyleInputRef} id="new-project-overall-style" rows="8" value={overallStyle} onChange={(event) => { setOverallStyle(event.target.value); setOverallStyleDirty(true) }} placeholder={t('projects.field.overall_style_placeholder')} aria-invalid={currentCreateErrors.overallStyle ? 'true' : undefined} aria-describedby={currentCreateErrors.overallStyle ? 'new-project-overall-style-error' : undefined} />
		      {currentCreateErrors.overallStyle ? <p className="project-field-error" id="new-project-overall-style-error" role="alert">{t(currentCreateErrors.overallStyle)}</p> : null}
		      <div><button className="button-secondary" type="button" onClick={() => { setOverallStyle(defaultOverallStyle); setOverallStyleDirty(false) }}>{t('projects.field.overall_style_restore')}</button></div>
		    </div>
		  </details>
		  <PictureBookProfileFields value={pictureBookDraft} onChange={setPictureBookDraft} />
          <p className="project-dialog-hint">{createMode === 'yolo' ? t('projects.create.yolo_hint') : t('projects.create.manual_hint', { path: defaultProjectParentPath || t('projects.create.default_path_loading') })}</p>
		  <div className="lumi-dialog__actions"><button className="button-secondary" type="button" disabled={pending} onClick={closeDialog}>{t('common.action.cancel')}</button><button type="submit" disabled={pending}>{t(pending ? 'projects.create.creating' : createMode === 'yolo' ? 'projects.create.start' : 'projects.create.enter')}</button></div>
        </form>
      </Modal> : null}

      {dialog === 'open' ? <Modal title={t('projects.dialog.open.title')} description={t('projects.dialog.open.description')} dismissDisabled={pending} onClose={closeDialog}><form className="project-dialog-form" onSubmit={(event) => { event.preventDefault(); openPathMutation.mutate(existingPath) }}><div className="project-dialog-field"><label htmlFor="existing-project-root">{t('projects.field.root_path')}</label><div className="project-path-picker"><input id="existing-project-root" value={existingPath} onChange={(event) => setExistingPath(event.target.value)} placeholder={t('projects.field.root_path_placeholder')} required autoFocus /><button className="button-secondary" type="button" disabled={pending} onClick={() => selectDirectoryMutation.mutate(existingPath)}><FolderOpen size={16} aria-hidden="true" />{t(selectDirectoryMutation.isPending ? 'projects.open.choosing_folder' : 'projects.open.choose_folder')}</button></div></div><p className="project-dialog-hint">{t('projects.open.path_hint')}</p><div className="lumi-dialog__actions"><button className="button-secondary" type="button" disabled={pending} onClick={closeDialog}>{t('common.action.cancel')}</button><button type="submit" disabled={pending || !existingPath.trim()}>{t(openPathMutation.isPending ? 'projects.open.validating' : 'projects.open.validate_enter')}</button></div></form></Modal> : null}

      {dialog === 'relocate' && targetProject ? <Modal title={t('projects.dialog.relocate.title')} description={targetProject.name} dismissDisabled={relocateMutation.isPending} onClose={closeDialog}><form className="project-dialog-form" onSubmit={(event) => { event.preventDefault(); relocateMutation.mutate({ uuid: targetProject.uuid, rootPath: relocatePath }) }}><label>{t('projects.field.new_root_path')}<input value={relocatePath} onChange={(event) => setRelocatePath(event.target.value)} placeholder={t('projects.field.root_path_placeholder')} required autoFocus /></label><p className="project-dialog-hint">{t('projects.relocate.hint')}</p><div className="lumi-dialog__actions"><button className="button-secondary" type="button" disabled={relocateMutation.isPending} onClick={closeDialog}>{t('common.action.cancel')}</button><button type="submit" disabled={relocateMutation.isPending || !relocatePath.trim()}>{t(relocateMutation.isPending ? 'projects.open.validating' : 'projects.relocate.validate_update')}</button></div></form></Modal> : null}

      {dialog === 'forget' && targetProject ? <Modal title={t('projects.dialog.forget.title')} description={targetProject.name} dismissDisabled={forgetMutation.isPending} onClose={closeDialog}><p className="project-dialog-hint">{t('projects.forget.hint')}</p><div className="lumi-dialog__actions"><button className="button-secondary" type="button" disabled={forgetMutation.isPending} onClick={closeDialog}>{t('common.action.cancel')}</button><button className="button-danger" type="button" disabled={forgetMutation.isPending} onClick={() => forgetMutation.mutate(targetProject.uuid)}>{t(forgetMutation.isPending ? 'projects.forget.removing' : 'projects.forget.confirm')}</button></div></Modal> : null}
    </AppPageShell>
  )
}

export function ProjectRow({ project, menuOpen, menuRef, onToggleMenu, onEnter, onReveal, onRelocate, onForget, formatDateTime, t }) {
  const actions = projectRowActions(project)
  const primaryAction = projectRowPrimaryAction(project)
  const onActivate = primaryAction === 'enter' ? onEnter : undefined
  const activateOnKeyDown = (event) => {
    if (event.target !== event.currentTarget || (event.key !== 'Enter' && event.key !== ' ')) return
    event.preventDefault()
    onActivate?.()
  }
  return (
    <article
      className={`project-card ${onActivate ? 'is-activatable' : ''} ${menuOpen ? 'is-menu-open' : ''}`}
      role="listitem"
      tabIndex={onActivate ? 0 : undefined}
      aria-label={onActivate ? t('projects.row.enter_label', { name: project.name }) : undefined}
      onClick={onActivate}
      onKeyDown={activateOnKeyDown}
    >
      <div className="project-card__heading">
        <div className="project-index-name"><strong>{project.name}</strong></div>
        <span className={`project-index-status project-index-status--${project.status}`}>{t(projectStatusCopy[project.status] || 'projects.status.unavailable')}</span>
      </div>
      <div className="project-card__meta">
        <time className="project-index-date" dateTime={project.last_opened_at}>{formatDateTime(project.last_opened_at, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</time>
      </div>
      <div className="project-index-more" ref={menuRef} onClick={(event) => event.stopPropagation()}>
        <button className="project-index-more-button" type="button" aria-label={t('projects.row.more_label', { name: project.name })} aria-expanded={menuOpen} onClick={onToggleMenu}><MoreHorizontal size={18} /></button>
        {menuOpen ? <div className="project-index-menu" role="menu">
          <div className="project-index-menu__path"><span>{t('projects.index.column.path')}</span><code data-no-i18n>{project.root_path}</code></div>
          {actions.includes('enter') ? <button className="project-index-menu__item" type="button" role="menuitem" onClick={onEnter}>{t('projects.action.enter')}</button> : null}
          {actions.includes('reveal') ? <button className="project-index-menu__item" type="button" role="menuitem" onClick={onReveal}>{t('projects.action.reveal')}</button> : null}
          {actions.includes('relocate') ? <button className="project-index-menu__item" type="button" role="menuitem" onClick={onRelocate}>{t('projects.action.relocate')}</button> : null}
          {actions.includes('forget') ? <><span className="project-index-menu__separator" role="separator" /><button className="project-index-menu__item project-index-menu__item--danger" type="button" role="menuitem" onClick={onForget}>{t('projects.action.forget')}</button></> : null}
        </div> : null}
      </div>
    </article>
  )
}

function Modal({ title, description, className = '', dismissDisabled = false, onClose, children }) {
  const { t } = useI18n()
  return <LumiDialog className={className} dismissDisabled={dismissDisabled} onClose={onClose}><header className="lumi-dialog__header"><div><h2>{title}</h2>{description ? <p>{description}</p> : null}</div><button className="button-quiet" type="button" disabled={dismissDisabled} aria-label={t('common.action.close')} onClick={onClose}><X size={17} /></button></header><div className="lumi-dialog__body">{children}</div></LumiDialog>
}

function dateValue(value) {
  const number = Date.parse(value || '')
  return Number.isFinite(number) ? number : 0
}
