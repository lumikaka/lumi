import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowUpDown, FolderOpen, HardDrive, MoreHorizontal, Plus, Search, X } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import AppPageShell from '../components/AppPageShell.jsx'
import LumiDialog from '../components/LumiDialog.jsx'
import PictureBookProfileFields from '../components/PictureBookProfileFields.jsx'
import { projectStatusCopy } from '../components/RecentProjectsView.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { createYoloWorkflow } from '../api/chat.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import { projectCreationErrors } from './projectCreationForm.js'
import { projectRowActions, projectRowPrimaryAction } from './projectIndexState.js'
import {
  createProject,
  forgetRecentProject,
	getProjectDefaults,
	listOpenProjects,
  listRecentProjects,
  openProjectPath,
  preflightImageGeneration,
  relocateRecentProject,
  selectProjectDirectory,
} from '../api/projects.js'
import { defaultPictureBookDraft, pictureBookDraftIsValid, pictureBookPayload } from './pictureBookProfile.js'

const SORT_OPTIONS = [
  { value: 'recent', labelKey: 'projects.index.sort.recent' },
  { value: 'oldest', labelKey: 'projects.index.sort.oldest' },
  { value: 'name_asc', labelKey: 'projects.index.sort.name_asc' },
  { value: 'name_desc', labelKey: 'projects.index.sort.name_desc' },
]

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
  const [pictureBookDraft, setPictureBookDraft] = useState(defaultPictureBookDraft)
  const [existingPath, setExistingPath] = useState('')
  const [targetProject, setTargetProject] = useState(null)
  const [relocatePath, setRelocatePath] = useState('')
  const [storyPrompt, setStoryPrompt] = useState('')
  const [createValidationAttempted, setCreateValidationAttempted] = useState(false)
  const [openMenuUuid, setOpenMenuUuid] = useState('')
  const [actionError, setActionError] = useState(null)
  const nameInputRef = useRef(null)
  const storyPromptInputRef = useRef(null)
  const parentPathInputRef = useRef(null)
  const createFormRef = useRef(null)
  const openMenuRef = useRef(null)
  const openMenuTriggerRef = useRef(null)

  const recentQuery = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
  const openProjectsQuery = useQuery({ queryKey: projectQueryKeys.openProjects(), queryFn: listOpenProjects })
  const projectDefaultsQuery = useQuery({ queryKey: ['project-defaults'], queryFn: getProjectDefaults })
  const defaultProjectParentPath = projectDefaultsQuery.data?.parent_path || ''
  const pictureBook = useMemo(() => pictureBookPayload(pictureBookDraft), [pictureBookDraft])
  const pictureBookValid = pictureBookDraftIsValid(pictureBookDraft)

  useEffect(() => {
    if (!parentPathDirty && defaultProjectParentPath) setParentPath(defaultProjectParentPath)
  }, [defaultProjectParentPath, parentPathDirty])

  const resetCreationFields = () => {
    setCreateMode('yolo')
    setName('')
    setParentPath(defaultProjectParentPath)
    setParentPathDirty(false)
    setGenerationLanguage('zh-Hans')
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
  const relocateMutation = useProjectMutation(queryClient, setActionError, relocateRecentProject, () => closeDialog())
  const forgetMutation = useProjectMutation(queryClient, setActionError, forgetRecentProject, () => closeDialog())
  const yoloMutation = useMutation({
    mutationFn: async () => {
			await preflightImageGeneration(pictureBook)
			const project = await createProject({ name, parentPath, generationLanguage, pictureBook })
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
  const currentCreateErrors = createValidationAttempted ? projectCreationErrors({ name, parentPath, storyPrompt, createMode, pictureBookValid }) : {}
  const selectCreateMode = (mode) => { setCreateMode(mode); setCreateValidationAttempted(false) }
  const submitCreateForm = (event) => {
    event.preventDefault()
    if (pending) return
    const errors = projectCreationErrors({ name, parentPath, storyPrompt, createMode, pictureBookValid })
    setCreateValidationAttempted(true)
    if (Object.keys(errors).length) {
      if (errors.name) nameInputRef.current?.focus()
      else if (errors.storyPrompt) storyPromptInputRef.current?.focus()
      else if (errors.parentPath) parentPathInputRef.current?.focus()
      else createFormRef.current?.querySelector('.picture-book-custom-ratio input')?.focus()
      return
    }
    if (createMode === 'yolo') yoloMutation.mutate()
    else createMutation.mutate({ name, parentPath, generationLanguage, pictureBook })
  }
  const openCreateDialog = () => { setActionError(null); resetCreationFields(); setCreateMode('yolo'); setDialog('create') }
  const openExistingDialog = () => { setActionError(null); setDialog('open') }

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
          <div className="project-index-table" role="table" aria-label={t('projects.recent')}>
            <div className="project-index-row project-index-row--head" role="row"><span>{t('common.label.name')}</span><span>{t('common.label.status')}</span><span>{t('projects.recent_used')}</span><span>{t('projects.index.column.path')}</span><span>{t('common.action.more')}</span></div>
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
                onRelocate={() => { setTargetProject(project); setRelocatePath(project.root_path || ''); setDialog('relocate'); setOpenMenuUuid('') }}
                onForget={() => { setTargetProject(project); setDialog('forget'); setOpenMenuUuid('') }}
                formatDateTime={formatDateTime}
                t={t}
              />
            ))}
          </div>
          <p className="project-index-count">{t('projects.index.count', { shown: projects.length, total: recentQuery.data?.items?.length || 0 })}</p>
        </section>
		<aside className="project-index-side">
		  <section className="story-panel project-index-current">
			<span className="project-index-current__badge">{t('projects.open')}</span>
			<div><p className="story-eyebrow">{t('projects.open.eyebrow')}</p><h2>{t('projects.open.count', { count: openProjectsQuery.data?.items?.length || 0 })}</h2></div>
			<code>{t('projects.open.body')}</code>
			{openProjectsQuery.data?.items?.length ? <div className="project-index-current__actions"><button type="button" onClick={() => enterProject(openProjectsQuery.data.items[0])}>{t('projects.action.enter_workspace')}</button></div> : <button type="button" onClick={openExistingDialog}>{t('projects.dialog.open.title')}</button>}
          </section>
          <section className="story-panel project-index-local-note"><HardDrive size={20} aria-hidden="true" /><h2>{t('projects.local_first.title')}</h2><p>{t('projects.local_first.body')}</p></section>
        </aside>
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

export function ProjectRow({ project, menuOpen, menuRef, onToggleMenu, onEnter, onRelocate, onForget, formatDateTime, t }) {
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
      className={`project-index-row project-index-row--item ${onActivate ? 'is-activatable' : ''} ${menuOpen ? 'is-menu-open' : ''}`}
      role="row"
      tabIndex={onActivate ? 0 : undefined}
      aria-label={onActivate ? t('projects.row.enter_label', { name: project.name }) : undefined}
      onClick={onActivate}
      onKeyDown={activateOnKeyDown}
    >
      <div className="project-index-name"><strong>{project.name}</strong><small>{project.uuid.slice(0, 13)}</small></div>
      <span className={`project-index-status project-index-status--${project.status}`}>{t(projectStatusCopy[project.status] || 'projects.status.unavailable')}</span>
      <time className="project-index-date" dateTime={project.last_opened_at}>{formatDateTime(project.last_opened_at, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</time>
      <span className="project-index-path" title={project.root_path}>{project.root_path}</span>
      <div className="project-index-more" ref={menuRef} onClick={(event) => event.stopPropagation()}>
        <button className="project-index-more-button" type="button" aria-label={t('projects.row.more_label', { name: project.name })} aria-expanded={menuOpen} onClick={onToggleMenu}><MoreHorizontal size={18} /></button>
        {menuOpen ? <div className="project-index-menu" role="menu">{actions.includes('enter') ? <button type="button" role="menuitem" onClick={onEnter}>{t('projects.action.enter')}</button> : null}{actions.includes('relocate') ? <button type="button" role="menuitem" onClick={onRelocate}>{t('projects.action.relocate')}</button> : null}{actions.includes('forget') ? <button className="danger-text" type="button" role="menuitem" onClick={onForget}>{t('projects.action.forget')}</button> : null}</div> : null}
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
