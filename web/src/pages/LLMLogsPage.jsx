import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, FolderOpen } from 'lucide-react'
import { Link, useSearchParams } from 'react-router-dom'

import { ensureProjectOpen, listOpenProjects, listRecentProjects } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import AppPageShell from '../components/AppPageShell.jsx'
import LocalAccountSettingsNav from '../components/LocalAccountSettingsNav.jsx'
import { useI18n } from '../i18n/useI18n.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import ProjectLLMLogsPanel from './ProjectLLMLogsPanel.jsx'
import { useProjectRealtimeSync } from '../realtime/useProjectRealtimeSync.js'

export default function LLMLogsPage() {
	const { t } = useI18n()
	const [searchParams, setSearchParams] = useSearchParams()
	const projectUuid = searchParams.get('project_uuid') || ''
	const recentProjectsQuery = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
	const openProjectsQuery = useQuery({ queryKey: projectQueryKeys.openProjects(), queryFn: listOpenProjects })
	const projectQuery = useQuery({ queryKey: projectQueryKeys.open(projectUuid), queryFn: () => ensureProjectOpen(projectUuid), enabled: Boolean(projectUuid), retry: false })
	const project = projectQuery.data
	const selectableProjects = useMemo(() => {
		const projects = new Map()
		for (const item of openProjectsQuery.data?.items || []) projects.set(item.uuid, item)
		for (const item of recentProjectsQuery.data?.items || []) projects.set(item.uuid, { ...projects.get(item.uuid), ...item })
		if (project) projects.set(project.uuid, project)
		return Array.from(projects.values())
	}, [openProjectsQuery.data, project, recentProjectsQuery.data])
	useProjectRealtimeSync(project ? projectUuid : '')
	const selectProject = (uuid) => {
		const next = new URLSearchParams(searchParams)
		if (uuid) next.set('project_uuid', uuid)
		else next.delete('project_uuid')
		setSearchParams(next)
	}

  return (
    <AppPageShell title={t('settings.llm_logs')}>
      <div className="local-account-page">
        <header className="local-account-heading">
          <span className="local-account-heading__icon"><Activity size={22} aria-hidden="true" /></span>
          <div>
            <p className="eyebrow">{t('settings.llm_logs.eyebrow')}</p>
            <h1>{t('settings.llm_logs.title')}</h1>
            <p>{t('settings.llm_logs.description')}</p>
          </div>
        </header>

        <div className="local-account-layout">
          <LocalAccountSettingsNav />
          <div className="local-account-logs">
			<label>{t('settings.llm_logs.project')}<select value={projectUuid} onChange={(event) => selectProject(event.target.value)}><option value="">{t('settings.llm_logs.select_project')}</option>{selectableProjects.map((item) => <option key={item.uuid} value={item.uuid}>{item.name}</option>)}</select></label>
			{projectQuery.isLoading ? <p className="workspace-loading">{t('projects.loading.project')}</p> : null}
			<LocalizedErrorMessage error={projectQuery.error || recentProjectsQuery.error || openProjectsQuery.error} />
			{!projectUuid && !recentProjectsQuery.isLoading ? (
              <section className="local-account-empty">
                <FolderOpen size={28} aria-hidden="true" />
                <h2>{t('settings.llm_logs.no_project')}</h2>
                <p>{t('settings.llm_logs.no_project_body')}</p>
                <Link className="button-link" to="/">{t('settings.llm_logs.view_projects')}</Link>
              </section>
            ) : null}
            {project ? (
              <>
				<p className="local-account-current-project"><span>{t('projects.open')}</span><strong data-no-i18n>{project.name}</strong></p>
                <ProjectLLMLogsPanel
                  projectUuid={project.uuid}
                  standalone
                  title={t('settings.llm_logs.title')}
                  description={t('settings.llm_logs.panel_description')}
                />
              </>
            ) : null}
          </div>
        </div>
      </div>
    </AppPageShell>
  )
}
