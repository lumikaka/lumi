import { useEffect, useId, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, FolderOpen, X } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import {
  createProject,
  getProjectDefaults,
  openProjectPath,
  selectProjectDirectory,
} from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import LumiDialog from './LumiDialog.jsx'
import { sidebarProjectCreateInput } from './sidebarProjectCreatorState.js'

export default function SidebarProjectCreator({ id, onClose, onComplete }) {
  const { locale, t } = useI18n()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const titleId = useId()
  const nameInputRef = useRef(null)
  const [name, setName] = useState('')
  const [sourcePath, setSourcePath] = useState('')
  const [actionError, setActionError] = useState(null)
  const defaultsQuery = useQuery({
    queryKey: ['project-defaults'],
    queryFn: getProjectDefaults,
  })

  useEffect(() => {
    nameInputRef.current?.focus()
  }, [])

  const finish = (project) => {
    setActionError(null)
    queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
    queryClient.invalidateQueries({ queryKey: projectQueryKeys.openProjects() })
    onComplete?.()
    navigate(`/projects/${encodeURIComponent(project.uuid)}`)
  }

  const createMutation = useMutation({
    mutationFn: async (projectName) => {
      const defaults = defaultsQuery.data || await queryClient.fetchQuery({
        queryKey: ['project-defaults'],
        queryFn: getProjectDefaults,
      })
      return createProject(sidebarProjectCreateInput(projectName, defaults, locale))
    },
    onMutate: () => setActionError(null),
    onSuccess: finish,
    onError: setActionError,
  })
  const openMutation = useMutation({
    mutationFn: openProjectPath,
    onMutate: () => setActionError(null),
    onSuccess: finish,
    onError: setActionError,
  })
  const selectDirectoryMutation = useMutation({
    mutationFn: selectProjectDirectory,
    onMutate: () => setActionError(null),
    onSuccess: (selection) => {
      if (selection?.root_path) setSourcePath(selection.root_path)
    },
    onError: setActionError,
  })
  const pending = createMutation.isPending || openMutation.isPending || selectDirectoryMutation.isPending
  const error = actionError || defaultsQuery.error

  const submitCreate = (event) => {
    event.preventDefault()
    const projectName = name.trim()
    if (!projectName || pending) return
    createMutation.mutate(projectName)
  }

  const submitOpen = (event) => {
    event.preventDefault()
    const rootPath = sourcePath.trim()
    if (!rootPath || pending) return
    openMutation.mutate(rootPath)
  }

  return (
    <LumiDialog className="sidebar-project-creator-dialog" id={id} aria-labelledby={titleId} dismissDisabled={pending} onClose={onClose}>
      <header className="lumi-dialog__header">
        <div>
          <h2 id={titleId}>{t('projects.sidebar_creator.title')}</h2>
          <p>{t('projects.sidebar_creator.description')}</p>
        </div>
        <button className="button-quiet" type="button" aria-label={t('common.action.close')} disabled={pending} onClick={onClose}>
          <X size={16} aria-hidden="true" />
        </button>
      </header>

      <div className="lumi-dialog__body sidebar-project-creator-dialog__body">
        <form className="sidebar-project-creator-dialog__form" onSubmit={submitCreate}>
          <label htmlFor={`${id}-name`}>{t('projects.field.name')}</label>
          <input
            ref={nameInputRef}
            id={`${id}-name`}
            value={name}
            placeholder={t('projects.field.name_placeholder')}
            disabled={pending}
            onChange={(event) => { setName(event.target.value); setActionError(null) }}
          />
          <button type="submit" disabled={pending || !name.trim()}>
            <span>{t(createMutation.isPending ? 'projects.create.creating' : 'projects.sidebar_creator.create')}</span>
            <ArrowRight size={15} aria-hidden="true" />
          </button>
          <small>
            {defaultsQuery.data?.parent_path
              ? t('projects.sidebar_creator.create_path', { path: defaultsQuery.data.parent_path })
              : t('projects.sidebar_creator.create_hint')}
          </small>
        </form>

        <div className="sidebar-project-creator-dialog__divider"><span>{t('projects.sidebar_creator.or')}</span></div>

        <form className="sidebar-project-creator-dialog__form" onSubmit={submitOpen}>
          <label htmlFor={`${id}-source-path`}>{t('projects.sidebar_creator.source_folder')}</label>
          <input
            id={`${id}-source-path`}
            value={sourcePath}
            placeholder={t('projects.field.root_path_placeholder')}
            disabled={pending}
            onChange={(event) => { setSourcePath(event.target.value); setActionError(null) }}
          />
          <div className="sidebar-project-creator-dialog__actions">
            <button className="button-secondary" type="button" disabled={pending} onClick={() => selectDirectoryMutation.mutate(sourcePath)}>
              <FolderOpen size={15} aria-hidden="true" />
              <span>{t(selectDirectoryMutation.isPending ? 'projects.open.choosing_folder' : 'projects.sidebar_creator.choose_folder')}</span>
            </button>
            <button type="submit" disabled={pending || !sourcePath.trim()}>
              <span>{t(openMutation.isPending ? 'projects.open.validating' : 'projects.sidebar_creator.open')}</span>
            </button>
          </div>
        </form>

        {error ? <LocalizedErrorMessage error={error} className="sidebar-project-creator-dialog__error" compact onDismiss={actionError ? () => setActionError(null) : undefined} /> : null}
      </div>
    </LumiDialog>
  )
}
