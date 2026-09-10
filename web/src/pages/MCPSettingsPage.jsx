import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { FolderOpen, Plug } from 'lucide-react'
import { useSearchParams } from 'react-router-dom'

import { getMCPConnection } from '../api/mcp.js'
import { ensureProjectOpen, listOpenProjects, listRecentProjects } from '../api/projects.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import AppPageShell from '../components/AppPageShell.jsx'
import LocalAccountSettingsNav from '../components/LocalAccountSettingsNav.jsx'
import ProjectMCPAccessCard from '../components/ProjectMCPAccessCard.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { useProjectRealtimeSync } from '../realtime/useProjectRealtimeSync.js'

export default function MCPSettingsPage() {
  const { t } = useI18n()
  const connection = useQuery({ queryKey: ['mcp-connection'], queryFn: getMCPConnection })
  const [copied, setCopied] = useState(false)
  const [copyError, setCopyError] = useState(null)
  const [searchParams, setSearchParams] = useSearchParams()
  const projectUuid = searchParams.get('project_uuid') || ''
  const recent = useQuery({ queryKey: projectQueryKeys.recent(), queryFn: listRecentProjects })
  const open = useQuery({ queryKey: projectQueryKeys.openProjects(), queryFn: listOpenProjects })
  const selected = useQuery({ queryKey: projectQueryKeys.open(projectUuid), queryFn: () => ensureProjectOpen(projectUuid), enabled: Boolean(projectUuid), retry: false })
  const project = selected.data
  const projects = useMemo(() => {
    const items = new Map()
    for (const item of open.data?.items || []) items.set(item.uuid, item)
    for (const item of recent.data?.items || []) items.set(item.uuid, item)
    if (project) items.set(project.uuid, project)
    return [...items.values()]
  }, [open.data, recent.data, project])
  useProjectRealtimeSync(project ? projectUuid : '')

  return <AppPageShell title={t('settings.mcp')}>
    <div className="local-account-page">
      <header className="local-account-heading">
        <span className="local-account-heading__icon"><Plug size={22} aria-hidden="true" /></span>
        <div>
          <p className="eyebrow">{t('settings.mcp.eyebrow')}</p>
          <h1>{t('settings.mcp')}</h1>
          <p>{t('settings.mcp.description')}</p>
          <p className="mcp-development-notice">{t('settings.mcp.development_notice')}</p>
        </div>
      </header>
      <div className="local-account-layout">
        <LocalAccountSettingsNav />
        <div className="local-account-logs">
          <section className="overview-card">
            <h2>{t('mcp.http.title')}</h2><p>{t('mcp.http.steps')}</p>
            {connection.data?.endpoint ? <><label>{t('mcp.http.address')}<input readOnly value={connection.data.endpoint} /></label><button type="button" onClick={async () => { try { await navigator.clipboard.writeText(connection.data.endpoint); setCopied(true); setCopyError(null) } catch (error) { setCopyError(error) } }}>{t(copied ? 'mcp.copied' : 'mcp.http.copy')}</button></> : <p>{t('mcp.http.unavailable')}</p>}
            <p>{t('mcp.http.requirements')}</p><LocalizedErrorMessage error={connection.error || copyError} />
          </section>
          <label>{t('settings.mcp.project')}<select value={projectUuid} onChange={(event) => setSearchParams(event.target.value ? { project_uuid: event.target.value } : {})}>
            <option value="">{t('settings.mcp.select_project')}</option>
            {projects.map((item) => <option key={item.uuid} value={item.uuid}>{item.name}</option>)}
          </select></label>
          <LocalizedErrorMessage error={selected.error || recent.error || open.error} />
          {selected.isLoading ? <p className="workspace-loading">{t('projects.loading.project')}</p> : null}
          {!projectUuid ? <section className="local-account-empty"><FolderOpen size={28} aria-hidden="true" /><h2>{t('settings.mcp.empty_title')}</h2><p>{t('settings.mcp.empty_body')}</p></section> : null}
          {project ? <ProjectMCPAccessCard key={project.uuid} projectUuid={project.uuid} /> : null}
        </div>
      </div>
    </div>
  </AppPageShell>
}
