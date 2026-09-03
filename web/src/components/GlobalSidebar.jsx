import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  Cpu,
  House,
  Languages,
  MessageSquareWarning,
  PanelLeftClose,
  Plus,
  Search,
  Settings,
  User,
  X,
} from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'

import { projectQueryKeys } from '../api/projectQueryKeys.js'
import { forgetRecentProject, relocateRecentProject, revealProjectDirectory } from '../api/projects.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { projectRowActions } from '../pages/projectIndexState.js'
import LumiDialog from './LumiDialog.jsx'
import ProjectActionsMenu from './ProjectActionsMenu.jsx'
import ProjectSearchDialog from './ProjectSearchDialog.jsx'
import SidebarProjectCreator from './SidebarProjectCreator.jsx'
import { projectContextMenuPosition } from './projectContextMenuPosition.js'
import { mergeSidebarProjectOrder, orderSidebarProjects, reorderSidebarProjectOrder } from './sidebarProjectOrder.js'

export const GLOBAL_SIDEBAR_COLLAPSED_KEY = 'lumi.globalSidebarCollapsed'
export const GLOBAL_SIDEBAR_PROJECT_ORDER_KEY = 'lumi.globalSidebarProjectOrder'

const SETTINGS_ITEMS = [
  { labelKey: 'settings.user_account', to: '/settings/account', icon: User },
  { labelKey: 'settings.language_preference', to: '/settings/account#language', icon: Languages },
  { labelKey: 'settings.model_configuration', to: '/settings/providers', icon: Cpu },
  { labelKey: 'settings.llm_calls', to: '/settings/llm-logs', icon: Activity },
  { labelKey: 'settings.feedback', to: '/about', icon: MessageSquareWarning },
]

const PROJECT_TONES = ['warm', 'accent', 'muted', 'warm', 'accent', 'muted']

export function useGlobalSidebarState() {
  const [collapsed, setCollapsed] = useState(readCollapsed)

  useEffect(() => {
    try {
      window.localStorage.setItem(GLOBAL_SIDEBAR_COLLAPSED_KEY, collapsed ? 'true' : 'false')
    } catch {
      // Storage may be unavailable in restricted browsers; the visual state still works.
    }
  }, [collapsed])

  return [collapsed, setCollapsed]
}

export default function GlobalSidebar({
  collapsed,
  mobileOpen,
  onClose,
  onToggleCollapsed,
  recentProjects = [],
  recentProjectsLoading = false,
  onSwitchProject,
}) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const location = useLocation()
  const titleId = useId()
  const searchDialogId = useId()
  const projectCreatorId = useId()
  const projectActionDialogTitleId = useId()
  const projectReorderHintId = useId()
  const settingsMenuId = useId()
  const closeRef = useRef(null)
  const projectDragClickSuppressedRef = useRef('')
  const projectPointerDragRef = useRef(null)
  const projectContextMenuRef = useRef(null)
  const projectContextTriggerRef = useRef(null)
  const projectCreatorTriggerRef = useRef(null)
  const searchTriggerRef = useRef(null)
  const settingsRef = useRef(null)
  const settingsTriggerRef = useRef(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [projectCreatorOpen, setProjectCreatorOpen] = useState(false)
  const [projectContextMenu, setProjectContextMenu] = useState(null)
  const [projectActionDialog, setProjectActionDialog] = useState('')
  const [projectActionTarget, setProjectActionTarget] = useState(null)
  const [projectActionError, setProjectActionError] = useState(null)
  const [projectDragState, setProjectDragState] = useState(null)
  const [projectReorderAnnouncement, setProjectReorderAnnouncement] = useState('')
  const [relocatePath, setRelocatePath] = useState('')
  const [stableRecentProjects, reorderProject] = useStableRecentProjects(recentProjects)

  useEffect(() => {
    setSettingsOpen(false)
    setSearchOpen(false)
    setProjectCreatorOpen(false)
    setProjectContextMenu(null)
    setProjectActionDialog('')
    setProjectActionTarget(null)
    setProjectActionError(null)
    projectDragClickSuppressedRef.current = ''
    projectPointerDragRef.current = null
    setProjectDragState(null)
    setRelocatePath('')
    onClose?.()
  }, [location.hash, location.pathname, location.search]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!collapsed) return
    setProjectCreatorOpen(false)
    setProjectContextMenu(null)
  }, [collapsed])

  useEffect(() => {
    if (!settingsOpen) return undefined
    const handlePointerDown = (event) => {
      if (!settingsRef.current?.contains(event.target)) setSettingsOpen(false)
    }
    const handleKeyDown = (event) => {
      if (event.key !== 'Escape') return
      setSettingsOpen(false)
      settingsTriggerRef.current?.focus()
    }
    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [settingsOpen])

  useLayoutEffect(() => {
    if (!projectContextMenu || projectContextMenu.positioned || !projectContextMenuRef.current) return
    const rect = projectContextMenuRef.current.getBoundingClientRect()
    const position = projectContextMenuPosition({
      x: projectContextMenu.left,
      y: projectContextMenu.top,
      width: rect.width,
      height: rect.height,
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
    })
    setProjectContextMenu((current) => current ? { ...current, ...position, positioned: true } : null)
  }, [projectContextMenu])

  useEffect(() => {
    if (!projectContextMenu?.positioned) return undefined
    projectContextMenuRef.current?.querySelector('[role="menuitem"]')?.focus()
    const handlePointerDown = (event) => {
      if (!projectContextMenuRef.current?.contains(event.target)) setProjectContextMenu(null)
    }
    const handleKeyDown = (event) => {
      if (event.key !== 'Escape') return
      setProjectContextMenu(null)
      projectContextTriggerRef.current?.focus()
    }
    const handleViewportChange = () => setProjectContextMenu(null)
    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    window.addEventListener('resize', handleViewportChange)
    window.addEventListener('scroll', handleViewportChange, true)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('resize', handleViewportChange)
      window.removeEventListener('scroll', handleViewportChange, true)
    }
  }, [projectContextMenu?.positioned])

  useEffect(() => {
    if (!mobileOpen) return undefined
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    closeRef.current?.focus()
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') onClose?.()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [mobileOpen, onClose])

  const invalidateProjectQueries = () => {
    queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
    queryClient.invalidateQueries({ queryKey: projectQueryKeys.openProjects() })
  }

  const revealDirectoryMutation = useMutation({
    mutationFn: revealProjectDirectory,
    onMutate: () => setProjectActionError(null),
    onSuccess: () => setProjectActionError(null),
    onError: setProjectActionError,
  })
  const relocateMutation = useMutation({
    mutationFn: relocateRecentProject,
    onMutate: () => setProjectActionError(null),
    onSuccess: () => {
      invalidateProjectQueries()
      closeProjectActionDialog()
    },
    onError: setProjectActionError,
  })
  const forgetMutation = useMutation({
    mutationFn: forgetRecentProject,
    onMutate: () => setProjectActionError(null),
    onSuccess: () => {
      invalidateProjectQueries()
      closeProjectActionDialog()
    },
    onError: setProjectActionError,
  })

  const closeProjectActionDialog = () => {
    setProjectActionDialog('')
    setProjectActionTarget(null)
    setProjectActionError(null)
    setRelocatePath('')
  }

  const openProjectContextMenu = (event, project) => {
    event.preventDefault()
    event.stopPropagation()
    const triggerRect = event.currentTarget.getBoundingClientRect()
    projectContextTriggerRef.current = event.currentTarget
    setSettingsOpen(false)
    setSearchOpen(false)
    setProjectCreatorOpen(false)
    setProjectActionError(null)
    setProjectContextMenu({
      project,
      left: event.clientX || triggerRect.right,
      top: event.clientY || triggerRect.top,
      positioned: false,
    })
  }

  const openProjectActionDialog = (dialog, project) => {
    setProjectContextMenu(null)
    setProjectActionError(null)
    setProjectActionTarget(project)
    setRelocatePath(project.root_path || '')
    setProjectActionDialog(dialog)
  }

  const settingsActive = location.pathname.startsWith('/settings/') || location.pathname === '/about'
  const currentProjectUuid = stableRecentProjects.find((project) => isProjectActive(location.pathname, project.uuid))?.uuid || ''

  const closeSearch = () => {
    setSearchOpen(false)
    window.requestAnimationFrame(() => searchTriggerRef.current?.focus())
  }

  const openSearch = () => {
    setSettingsOpen(false)
    setProjectCreatorOpen(false)
    setSearchOpen(true)
    onClose?.()
  }

  const closeProjectCreator = () => {
    setProjectCreatorOpen(false)
    window.requestAnimationFrame(() => projectCreatorTriggerRef.current?.focus())
  }

  const toggleProjectCreator = () => {
    setSettingsOpen(false)
    setSearchOpen(false)
    setProjectCreatorOpen((current) => !current)
  }

  const clearProjectDrag = () => {
    projectPointerDragRef.current = null
    setProjectDragState(null)
  }

  const moveProject = (draggedUuid, targetUuid, placement) => {
    const currentOrder = stableRecentProjects.map((project) => project.uuid)
    const nextOrder = reorderSidebarProjectOrder(currentOrder, draggedUuid, targetUuid, placement)
    if (nextOrder === currentOrder) return

    reorderProject(draggedUuid, targetUuid, placement)
    const project = stableRecentProjects.find((item) => item.uuid === draggedUuid)
    setProjectReorderAnnouncement(t('projects.sidebar.reordered', {
      name: project?.name || '',
      position: nextOrder.indexOf(draggedUuid) + 1,
      total: nextOrder.length,
    }))
  }

  const startProjectDrag = (event, projectUuid) => {
    if (event.button !== 0 || !event.isPrimary) return
    projectPointerDragRef.current = {
      draggedUuid: projectUuid,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      dragging: false,
      targetUuid: '',
      placement: '',
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
  }

  const updateProjectDrag = (event) => {
    const drag = projectPointerDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    if (!drag.dragging) {
      if (Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) < 6) return
      drag.dragging = true
      setProjectContextMenu(null)
      setProjectDragState({ draggedUuid: drag.draggedUuid, targetUuid: '', placement: '' })
    }
    event.preventDefault()
    const target = document.elementFromPoint(event.clientX, event.clientY)?.closest('[data-sidebar-project-uuid]')
    const targetUuid = target?.dataset.sidebarProjectUuid || ''
    if (!targetUuid || targetUuid === drag.draggedUuid) {
      drag.targetUuid = ''
      drag.placement = ''
      setProjectDragState({ draggedUuid: drag.draggedUuid, targetUuid: '', placement: '' })
      return
    }
    const rect = target.getBoundingClientRect()
    const placement = event.clientY < rect.top + (rect.height / 2) ? 'before' : 'after'
    drag.targetUuid = targetUuid
    drag.placement = placement
    setProjectDragState((current) => (
      current?.targetUuid === targetUuid && current.placement === placement
        ? current
        : { draggedUuid: drag.draggedUuid, targetUuid, placement }
    ))
  }

  const finishProjectDrag = (event) => {
    const drag = projectPointerDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    if (!drag.dragging) {
      projectPointerDragRef.current = null
      return
    }
    projectDragClickSuppressedRef.current = drag.draggedUuid
    if (drag.targetUuid) moveProject(drag.draggedUuid, drag.targetUuid, drag.placement)
    clearProjectDrag()
    window.setTimeout(() => {
      if (projectDragClickSuppressedRef.current === drag.draggedUuid) projectDragClickSuppressedRef.current = ''
    }, 0)
  }

  const cancelProjectDrag = (event) => {
    if (projectPointerDragRef.current?.pointerId !== event.pointerId) return
    clearProjectDrag()
  }

  const handleProjectClick = (event, projectUuid) => {
    if (projectDragClickSuppressedRef.current !== projectUuid) return true
    projectDragClickSuppressedRef.current = ''
    event.preventDefault()
    event.stopPropagation()
    return false
  }

  const moveProjectWithKeyboard = (event, project, index) => {
    if (!event.altKey || !['ArrowUp', 'ArrowDown'].includes(event.key)) return
    event.preventDefault()
    const direction = event.key === 'ArrowUp' ? -1 : 1
    const target = stableRecentProjects[index + direction]
    if (!target) return
    moveProject(project.uuid, target.uuid, direction < 0 ? 'before' : 'after')
  }

  return (
    <>
      {mobileOpen ? (
        <button
          className="global-sidebar__backdrop"
          type="button"
          aria-label={t('common.navigation.close')}
          onClick={onClose}
        />
      ) : null}
      <aside
        className={`global-sidebar ${collapsed ? 'is-collapsed' : ''} ${mobileOpen ? 'is-mobile-open' : ''}`}
        aria-labelledby={titleId}
      >
        <header className="global-sidebar__header">
          <button
            className="global-sidebar__brand"
            type="button"
            aria-label={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
            aria-pressed={collapsed}
            title={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
            onClick={onToggleCollapsed}
          >
            <img className="global-sidebar__logo" src="/favicon.png" alt="" aria-hidden="true" />
            <span className="global-sidebar__brand-name" id={titleId}>Lumi</span>
          </button>
          <button
            ref={closeRef}
            className="global-sidebar__mobile-close"
            type="button"
            aria-label={t('common.navigation.close')}
            onClick={onClose}
          >
            <X size={18} aria-hidden="true" />
          </button>
        </header>

        <nav className="global-sidebar__nav" aria-label={t('common.navigation.main')}>
          <SidebarLink
            to="/"
            label={t('common.navigation.home')}
            icon={House}
            active={location.pathname === '/'}
          />
          <button
            ref={searchTriggerRef}
            className={`global-sidebar__link global-sidebar__search ${searchOpen ? 'is-active' : ''}`}
            type="button"
            aria-haspopup="dialog"
            aria-expanded={searchOpen}
            aria-controls={searchOpen ? searchDialogId : undefined}
            title={t('common.action.search')}
            onClick={openSearch}
          >
            <Search size={16} aria-hidden="true" />
            <span>{t('common.action.search')}</span>
          </button>
        </nav>

        <section className="global-sidebar__recent" aria-labelledby={`${titleId}-projects`}>
          <header className="global-sidebar__section-header">
            <h2 id={`${titleId}-projects`}>{t('projects.title')}</h2>
            <div className="global-sidebar__project-creator-anchor">
              <button
                ref={projectCreatorTriggerRef}
                className="global-sidebar__add-project"
                type="button"
                aria-label={t('projects.action.new')}
                aria-haspopup="dialog"
                aria-expanded={projectCreatorOpen}
                aria-controls={projectCreatorOpen ? projectCreatorId : undefined}
                onClick={toggleProjectCreator}
              >
                <Plus size={16} aria-hidden="true" />
              </button>
              {projectCreatorOpen ? (
                <SidebarProjectCreator
                  id={projectCreatorId}
                  onClose={closeProjectCreator}
                  onComplete={() => { setProjectCreatorOpen(false); onClose?.() }}
                />
              ) : null}
            </div>
          </header>
          <nav aria-label={t('projects.title')}>
            <p className="global-sidebar__reorder-assistive" id={projectReorderHintId}>{t('projects.sidebar.reorder_hint')}</p>
            <p className="global-sidebar__reorder-assistive" role="status" aria-live="polite" aria-atomic="true">{projectReorderAnnouncement}</p>
            {stableRecentProjects.map((project, index) => (
              <RecentProjectLink
                key={project.uuid}
                active={isProjectActive(location.pathname, project.uuid)}
                dragState={projectDragState}
                index={index}
                project={project}
                reorderHintId={projectReorderHintId}
                tone={PROJECT_TONES[index % PROJECT_TONES.length]}
                onClick={(event) => handleProjectClick(event, project.uuid)}
                onContextMenu={(event) => openProjectContextMenu(event, project)}
                onKeyDown={(event) => moveProjectWithKeyboard(event, project, index)}
                onPointerCancel={cancelProjectDrag}
                onPointerDown={(event) => startProjectDrag(event, project.uuid)}
                onPointerMove={updateProjectDrag}
                onPointerUp={finishProjectDrag}
                onSwitchProject={onSwitchProject}
              />
            ))}
          </nav>
        </section>

        <footer className="global-sidebar__footer" ref={settingsRef}>
          <button
            ref={settingsTriggerRef}
            className={`global-sidebar__link global-sidebar__settings ${settingsActive ? 'is-active' : ''}`}
            type="button"
            aria-haspopup="menu"
            aria-expanded={settingsOpen}
            aria-controls={settingsOpen ? settingsMenuId : undefined}
            title={t('settings.title')}
            onClick={() => { setProjectCreatorOpen(false); setSettingsOpen((current) => !current) }}
          >
            <Settings size={16} aria-hidden="true" />
            <span>{t('settings.title')}</span>
          </button>

          {settingsOpen ? (
            <nav className="global-sidebar__settings-menu" id={settingsMenuId} role="menu" aria-label={t('settings.menu')}>
              {SETTINGS_ITEMS.map((item) => {
                const Icon = item.icon
                return (
                  <Link className="global-sidebar__menu-item" key={`${item.to}-${item.labelKey}`} role="menuitem" to={item.to}>
                    <Icon size={16} aria-hidden="true" />
                    <span>{t(item.labelKey)}</span>
                  </Link>
                )
              })}
            </nav>
          ) : null}

          <button
            className="global-sidebar__collapse"
            type="button"
            aria-label={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
            aria-pressed={collapsed}
            title={collapsed ? t('common.navigation.open_lumi') : t('common.navigation.close')}
            onClick={onToggleCollapsed}
          >
            <PanelLeftClose size={16} aria-hidden="true" />
            <span>{t('common.navigation.collapse')}</span>
          </button>
        </footer>
      </aside>
      {searchOpen ? (
        <ProjectSearchDialog
          id={searchDialogId}
          currentProjectUuid={currentProjectUuid}
          loading={recentProjectsLoading}
          projects={stableRecentProjects}
          onClose={closeSearch}
          onSwitchProject={onSwitchProject}
        />
      ) : null}
      {projectContextMenu && typeof document !== 'undefined' ? createPortal(
        <ProjectActionsMenu
          actions={projectRowActions(projectContextMenu.project)}
          className="project-index-menu--context"
          menuRef={projectContextMenuRef}
          project={projectContextMenu.project}
          style={{
            left: projectContextMenu.left,
            top: projectContextMenu.top,
            visibility: projectContextMenu.positioned ? 'visible' : 'hidden',
          }}
          t={t}
          onEnter={() => {
            setProjectContextMenu(null)
            onSwitchProject?.(projectContextMenu.project.uuid)
          }}
          onReveal={() => {
            setProjectContextMenu(null)
            revealDirectoryMutation.mutate(projectContextMenu.project.root_path)
          }}
          onRelocate={() => openProjectActionDialog('relocate', projectContextMenu.project)}
          onForget={() => openProjectActionDialog('forget', projectContextMenu.project)}
        />,
        document.body,
      ) : null}
      {projectActionDialog === 'relocate' && projectActionTarget ? (
        <LumiDialog aria-labelledby={projectActionDialogTitleId} dismissDisabled={relocateMutation.isPending} onClose={closeProjectActionDialog}>
          <header className="lumi-dialog__header">
            <div>
              <h2 id={projectActionDialogTitleId}>{t('projects.dialog.relocate.title')}</h2>
              <p data-no-i18n>{projectActionTarget.name}</p>
            </div>
            <button className="button-quiet" type="button" disabled={relocateMutation.isPending} aria-label={t('common.action.close')} onClick={closeProjectActionDialog}><X size={17} aria-hidden="true" /></button>
          </header>
          <div className="lumi-dialog__body">
            <form className="project-dialog-form" onSubmit={(event) => { event.preventDefault(); relocateMutation.mutate({ uuid: projectActionTarget.uuid, rootPath: relocatePath }) }}>
              <label>{t('projects.field.new_root_path')}<input value={relocatePath} onChange={(event) => { setRelocatePath(event.target.value); setProjectActionError(null) }} placeholder={t('projects.field.root_path_placeholder')} required autoFocus /></label>
              <p className="project-dialog-hint">{t('projects.relocate.hint')}</p>
              <LocalizedErrorMessage error={projectActionError} compact onDismiss={() => setProjectActionError(null)} />
              <div className="lumi-dialog__actions"><button className="button-secondary" type="button" disabled={relocateMutation.isPending} onClick={closeProjectActionDialog}>{t('common.action.cancel')}</button><button type="submit" disabled={relocateMutation.isPending || !relocatePath.trim()}>{t(relocateMutation.isPending ? 'projects.open.validating' : 'projects.relocate.validate_update')}</button></div>
            </form>
          </div>
        </LumiDialog>
      ) : null}
      {projectActionDialog === 'forget' && projectActionTarget ? (
        <LumiDialog aria-labelledby={projectActionDialogTitleId} dismissDisabled={forgetMutation.isPending} onClose={closeProjectActionDialog}>
          <header className="lumi-dialog__header">
            <div>
              <h2 id={projectActionDialogTitleId}>{t('projects.dialog.forget.title')}</h2>
              <p data-no-i18n>{projectActionTarget.name}</p>
            </div>
            <button className="button-quiet" type="button" disabled={forgetMutation.isPending} aria-label={t('common.action.close')} onClick={closeProjectActionDialog}><X size={17} aria-hidden="true" /></button>
          </header>
          <div className="lumi-dialog__body">
            <p className="project-dialog-hint">{t('projects.forget.hint')}</p>
            <LocalizedErrorMessage error={projectActionError} compact onDismiss={() => setProjectActionError(null)} />
            <div className="lumi-dialog__actions"><button className="button-secondary" type="button" disabled={forgetMutation.isPending} onClick={closeProjectActionDialog}>{t('common.action.cancel')}</button><button className="button-danger" type="button" disabled={forgetMutation.isPending} onClick={() => forgetMutation.mutate(projectActionTarget.uuid)}>{t(forgetMutation.isPending ? 'projects.forget.removing' : 'projects.forget.confirm')}</button></div>
          </div>
        </LumiDialog>
      ) : null}
      {projectActionError && !projectActionDialog && typeof document !== 'undefined' ? createPortal(
        <LocalizedErrorMessage error={projectActionError} className="workspace-notice workspace-notice--error global-sidebar__project-action-error" compact onDismiss={() => setProjectActionError(null)} />,
        document.body,
      ) : null}
    </>
  )
}

function SidebarLink({ active, icon: Icon, label, to }) {
  return (
    <Link className={`global-sidebar__link ${active ? 'is-active' : ''}`} to={to} title={label} aria-current={active ? 'page' : undefined}>
      <Icon size={16} aria-hidden="true" />
      <span>{label}</span>
    </Link>
  )
}

function RecentProjectLink({ active, dragState, project, reorderHintId, onClick, onContextMenu, onKeyDown, onPointerCancel, onPointerDown, onPointerMove, onPointerUp, onSwitchProject, tone }) {
  const content = (
    <>
      <span className={`global-sidebar__project-mark is-${tone}`} aria-hidden="true">{project.name?.trim().charAt(0) || '·'}</span>
      <span className="global-sidebar__project-copy" data-no-i18n>{project.name}</span>
    </>
  )
  const dragging = dragState?.draggedUuid === project.uuid
  const dropPlacement = dragState?.targetUuid === project.uuid ? dragState.placement : ''
  const className = [
    'global-sidebar__link global-sidebar__project',
    active ? 'is-active' : '',
    dragging ? 'is-dragging' : '',
    dropPlacement ? `is-drop-${dropPlacement}` : '',
  ].filter(Boolean).join(' ')
  const dragProps = {
    draggable: false,
    'data-sidebar-project-uuid': project.uuid,
    'aria-describedby': reorderHintId,
    'aria-grabbed': dragging,
    'aria-keyshortcuts': 'Alt+ArrowUp Alt+ArrowDown',
    onKeyDown,
    onPointerCancel,
    onPointerDown,
    onPointerMove,
    onPointerUp,
  }
  if (!project.available) {
    return <Link {...dragProps} className={className} to="/" title={project.name} aria-current={active ? 'page' : undefined} aria-haspopup="menu" onClick={onClick} onContextMenu={onContextMenu}>{content}</Link>
  }
  return (
    <button
      {...dragProps}
      className={className}
      type="button"
      title={project.name}
      aria-current={active ? 'page' : undefined}
      aria-haspopup="menu"
      onClick={(event) => { if (onClick?.(event) !== false) onSwitchProject?.(project.uuid) }}
      onContextMenu={onContextMenu}
    >
      {content}
    </button>
  )
}

function isProjectActive(pathname, projectUuid) {
  return pathname === `/projects/${encodeURIComponent(projectUuid)}` || pathname.startsWith(`/projects/${encodeURIComponent(projectUuid)}/`)
}

function useStableRecentProjects(recentProjects) {
  const [projectOrder, setProjectOrder] = useState(readProjectOrder)
  const nextProjectOrder = mergeSidebarProjectOrder(projectOrder, recentProjects)

  useEffect(() => {
    if (nextProjectOrder === projectOrder) return
    setProjectOrder(nextProjectOrder)
  }, [nextProjectOrder, projectOrder])

  useEffect(() => {
    try {
      window.localStorage.setItem(GLOBAL_SIDEBAR_PROJECT_ORDER_KEY, JSON.stringify(projectOrder))
    } catch {
      // Storage may be unavailable; the order still remains stable until unmount.
    }
  }, [projectOrder])

  return [
    orderSidebarProjects(recentProjects, nextProjectOrder),
    (draggedUuid, targetUuid, placement) => {
      setProjectOrder((currentOrder) => reorderSidebarProjectOrder(
        mergeSidebarProjectOrder(currentOrder, recentProjects),
        draggedUuid,
        targetUuid,
        placement,
      ))
    },
  ]
}

function readProjectOrder() {
  if (typeof window === 'undefined') return []
  try {
    const storedValue = window.localStorage.getItem(GLOBAL_SIDEBAR_PROJECT_ORDER_KEY)
      || window.sessionStorage.getItem(GLOBAL_SIDEBAR_PROJECT_ORDER_KEY)
      || '[]'
    const value = JSON.parse(storedValue)
    if (!Array.isArray(value)) return []
    return [...new Set(value.filter((uuid) => typeof uuid === 'string' && uuid))]
  } catch {
    return []
  }
}

function readCollapsed() {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(GLOBAL_SIDEBAR_COLLAPSED_KEY) === 'true'
  } catch {
    return false
  }
}
