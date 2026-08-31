import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'

import { workspaceRoute } from '../components/workspaceNavigation.js'
import { getPremise, listComicExports, updatePremise } from '../api/production.js'
import { preflightProjectImageGeneration } from '../api/projects.js'
import { getStoryProfile, getStoryProject, listChapters, updateStoryProject } from '../api/story.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import { projectionStateLabel } from '../i18n/labels.js'
import ComicExportDialog from '../components/ComicExportDialog.jsx'
import ProjectDashboardModeSetting from '../components/ProjectDashboardModeSetting.jsx'
import ProjectModelSettingsCard from '../components/ProjectModelSettingsCard.jsx'
import { comicExportDialogRequest } from './comicExportState.js'
import { formatTerminologyMessageKey, pictureBookProfileDetails, pictureBookRatio } from './pictureBookProfile.js'

const pendingExportStatuses = new Set(['queued', 'running'])
const exportPageSize = 10

function destination(projectUuid, route, search) {
  return workspaceRoute(projectUuid, route, search)
}

function storyExcerpt(markdown = '') {
  const copy = markdown
    .replace(/<!--[\s\S]*?-->/g, '')
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => !/^#{1,6}\s+/.test(line))
    .map((line) => line.replace(/^[-*>]\s*/, ''))
    .filter(Boolean)
    .join(' ')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/[*_`~]/g, '')
  if (!copy) return ''
  return copy.length > 420 ? `${copy.slice(0, 420).trimEnd()}…` : copy
}

function generationLanguageLabel(t, value) {
  if (value === 'en') return t('common.language.en')
  if (value === 'zh-Hans' || value === 'zh-CN' || value === 'zh') return t('common.language.zh_hans')
  return t('common.status.unknown_with_code', { code: value || '—' })
}

export function OverviewSummaryPanel({ projectUuid, projectQuery }) {
  const { formatDateTime, t } = useI18n()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [editingProject, setEditingProject] = useState(false)
  const [editingStyle, setEditingStyle] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [generationLanguage, setGenerationLanguage] = useState('zh-Hans')
  const [style, setStyle] = useState('')
  const [error, setError] = useState(null)
  const profileQuery = useQuery({ queryKey: ['story-profile', projectUuid], queryFn: () => getStoryProfile(projectUuid) })
  const premiseQuery = useQuery({ queryKey: ['premise', projectUuid], queryFn: () => getPremise(projectUuid) })
  const imagePreflightQuery = useQuery({
    queryKey: ['project-image-generation-preflight', projectUuid],
    queryFn: () => preflightProjectImageGeneration(projectUuid),
    enabled: Boolean(projectQuery.data),
    retry: false,
  })

  useEffect(() => {
    if (!projectQuery.data || editingProject) return
    setName(projectQuery.data.name || '')
    setDescription(projectQuery.data.description || '')
  }, [editingProject, projectQuery.data])

  useEffect(() => {
    if (projectQuery.data) setGenerationLanguage(projectQuery.data.generation_language || 'zh-Hans')
  }, [projectQuery.data])

  useEffect(() => {
    if (!premiseQuery.data || editingStyle) return
    setStyle(premiseQuery.data.default_style || '')
  }, [editingStyle, premiseQuery.data])

  const updateProject = useMutation({
    mutationFn: () => updateStoryProject(projectUuid, {
      name,
      description,
      generation_language: projectQuery.data.generation_language,
      expected_revision: projectQuery.data.revision,
    }),
    onSuccess: (project) => {
      queryClient.setQueryData(['story-project', projectUuid], project)
		queryClient.invalidateQueries({ queryKey: projectQueryKeys.open(projectUuid) })
		queryClient.invalidateQueries({ queryKey: projectQueryKeys.openProjects() })
		queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
      setEditingProject(false)
      setError(null)
    },
    onError: setError,
  })

  const updateLanguage = useMutation({
    mutationFn: () => updateStoryProject(projectUuid, {
      name: projectQuery.data.name,
      description: projectQuery.data.description,
      generation_language: generationLanguage,
      expected_revision: projectQuery.data.revision,
    }),
    onSuccess: (project) => {
      queryClient.setQueryData(['story-project', projectUuid], project)
      setGenerationLanguage(project.generation_language)
      setError(null)
    },
    onError: setError,
  })

  const updateStyle = useMutation({
    mutationFn: () => updatePremise(projectUuid, {
      default_style: style,
      expected_revision: premiseQuery.data.revision,
    }),
    onSuccess: (premise) => {
      queryClient.setQueryData(['premise', projectUuid], premise)
      setEditingStyle(false)
      setError(null)
    },
    onError: setError,
  })

  if (projectQuery.isLoading) return <p className="workspace-loading">{t('projects.loading.project')}</p>
  if (projectQuery.isError) return <LocalizedErrorMessage error={projectQuery.error} />

  const project = projectQuery.data
  const term = (key, values) => t(formatTerminologyMessageKey(project.picture_book, key), values)
  const profile = profileQuery.data
  const premise = premiseQuery.data
  const excerpt = storyExcerpt(profile?.story_md)
  const route = (suffix) => destination(projectUuid, suffix, location.search)

  const cancelProjectEdit = () => {
    setName(project.name || '')
    setDescription(project.description || '')
    setEditingProject(false)
  }
  const cancelStyleEdit = () => {
    setStyle(premise?.default_style || '')
    setEditingStyle(false)
  }
  const configurationDirty = Boolean(project) && (
    name !== (project.name || '')
    || description !== (project.description || '')
  )

  return (
    <div className="project-overview project-overview--summary" role="tabpanel" id="overview-panel-summary" aria-labelledby="overview-tab-summary">
      <LocalizedErrorMessage error={error || profileQuery.error || premiseQuery.error} onDismiss={error ? () => setError(null) : undefined} />
      <div className="project-overview-grid">
        <div className="project-overview-main">
          <section className="overview-card overview-card--project">
            <header className="overview-card__header">
              <div><h1>{t('projects.overview.summary')}</h1></div>
              {!editingProject ? <button type="button" className="button-quiet overview-card__action" onClick={() => setEditingProject(true)}>{t('projects.configuration')}</button> : null}
            </header>
            {editingProject ? (
              <form className="overview-edit-form" onSubmit={(event) => { event.preventDefault(); updateProject.mutate() }}>
                <label>{t('projects.field.name')}<input value={name} onChange={(event) => setName(event.target.value)} required maxLength="120" /></label>
                <label>{t('projects.overview.description')}<textarea value={description} onChange={(event) => setDescription(event.target.value)} rows="4" maxLength="2000" /></label>
                <ProjectDashboardModeSetting projectUuid={projectUuid} dirty={configurationDirty} disabled={updateProject.isPending} />
                <div className="overview-form-actions"><button type="submit" disabled={!name.trim() || updateProject.isPending}>{t(updateProject.isPending ? 'common.status.saving' : 'common.action.save')}</button><button type="button" className="button-secondary" disabled={updateProject.isPending} onClick={cancelProjectEdit}>{t('common.action.cancel')}</button></div>
              </form>
            ) : (
              <>
                {project.description ? <p className="overview-project-description">{project.description}</p> : null}
                <dl className="overview-project-facts">
                  <div><dt>{term('projects.overview.active_chapters')}</dt><dd>{project.chapter_count}</dd></div>
                  <div><dt>{t('projects.tab.trash')}</dt><dd>{project.trash_count}</dd></div>
                  <div><dt>{t('projects.overview.revision')}</dt><dd>r{project.revision}</dd></div>
                  <div><dt>{t('projects.overview.generation_language')}</dt><dd>{generationLanguageLabel(t, project.generation_language)}</dd></div>
                  <div><dt>{t('projects.overview.updated')}</dt><dd>{formatDateTime(project.updated_at, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</dd></div>
                </dl>
              </>
            )}
          </section>

          <PictureBookOverviewCard project={project} preflightQuery={imagePreflightQuery} />

          <section className="overview-card">
            <header className="overview-card__header">
              <div><h2>{t('story.profile')}</h2></div>
              <Link className="overview-card__action" to={route('story')}>{t(excerpt ? 'projects.overview.view_edit' : 'projects.overview.start_writing')}</Link>
            </header>
            {profileQuery.isLoading ? <p className="overview-card__loading">{t('story.story_file_loading')}</p> : <p className={`overview-story-copy ${excerpt ? '' : 'is-empty'}`}>{excerpt || t('story.story_file_empty')}</p>}
            {profile ? <footer className="overview-card__meta"><span>v{profile.version_no}</span><span>{profile.projection_state === 'synced' ? t('story.file_synced') : t('story.file_state', { state: projectionStateLabel(t, profile.projection_state) })}</span></footer> : null}
          </section>

          <section className="overview-card overview-language-card">
            <form onSubmit={(event) => { event.preventDefault(); updateLanguage.mutate() }}>
              <header className="overview-card__header"><div><h2>{t('projects.overview.language.title')}</h2><p>{t('projects.overview.language.body')}</p></div><button type="submit" disabled={updateLanguage.isPending || generationLanguage === project.generation_language}>{t(updateLanguage.isPending ? 'common.status.saving' : 'common.action.save')}</button></header>
              <label>{t('projects.field.generation_language')}<select value={generationLanguage} onChange={(event) => setGenerationLanguage(event.target.value)} disabled={updateLanguage.isPending}><option value="zh-Hans">{t('common.language.zh_hans')}</option><option value="en">{t('common.language.en')}</option></select></label>
            </form>
          </section>

          <ProjectModelSettingsCard projectUuid={projectUuid} />

          <section className="overview-card">
            <header className="overview-card__header">
              <div><h2>{t('projects.overview.style.title')}</h2></div>
              {!editingStyle && premise ? <button type="button" className="button-quiet overview-card__action" onClick={() => setEditingStyle(true)}>{t('common.action.edit')}</button> : null}
            </header>
            {editingStyle ? (
              <form className="overview-edit-form" onSubmit={(event) => { event.preventDefault(); updateStyle.mutate() }}>
                <label>{t('projects.overview.style.default')}<textarea value={style} onChange={(event) => setStyle(event.target.value)} rows="5" placeholder={t('projects.overview.style.placeholder')} /></label>
                <div className="overview-form-actions"><button type="submit" disabled={updateStyle.isPending}>{t(updateStyle.isPending ? 'common.status.saving' : 'projects.overview.style.save')}</button><button type="button" className="button-secondary" disabled={updateStyle.isPending} onClick={cancelStyleEdit}>{t('common.action.cancel')}</button></div>
              </form>
            ) : premiseQuery.isLoading ? <p className="overview-card__loading">{t('projects.overview.style.loading')}</p> : <p className={`overview-style-copy ${premise?.default_style ? '' : 'is-empty'}`}>{premise?.default_style || t('projects.overview.style.empty')}</p>}
            <footer className="overview-card__meta"><Link to={route('premise')}>{t('projects.overview.premise_link')} <span aria-hidden="true">→</span></Link></footer>
          </section>

          <section className="overview-card">
            <header className="overview-card__header"><div><h2>{t('projects.overview.continue')}</h2></div></header>
            <div className="overview-work-links">
              <Link to={route('story')}><span>01</span><strong>{t('projects.overview.work.profile.title')}</strong><small>{t('projects.overview.work.profile.body')}</small></Link>
              <Link to={route('premise')}><span>02</span><strong>{t('projects.overview.work.premise.title')}</strong><small>{t('projects.overview.work.premise.body')}</small></Link>
              <Link to={route('chapters')}><span>03</span><strong>{term('projects.overview.work.chapters.title')}</strong><small>{term('projects.overview.work.chapters.body')}</small></Link>
            </div>
          </section>
        </div>

        <aside className="overview-progress-card" aria-label={t('projects.overview.progress')}>
          <header><h2>{t('projects.overview.progress')}</h2></header>
          <div className="overview-progress-grid">
            <article><strong>{project.chapter_count}</strong><span>{term('projects.overview.active_chapters')}</span></article>
            <article><strong>{project.trash_count}</strong><span>{t('projects.tab.trash')}</span></article>
            <article><strong>{profile ? `v${profile.version_no}` : '—'}</strong><span>STORY.md</span></article>
            <article><strong>r{project.revision}</strong><span>{t('projects.overview.revision')}</span></article>
          </div>
        </aside>
      </div>
    </div>
  )
}

function PictureBookOverviewCard({ project, preflightQuery }) {
  const { t } = useI18n()
  const profile = project.picture_book
  const details = pictureBookProfileDetails(t, profile)
  const ratio = pictureBookRatio(profile)?.value || '—'
  const unsupported = preflightQuery.error?.code === 'image_aspect_ratio_unsupported'
  return (
    <section className="overview-card overview-picture-book-card">
      <header className="overview-card__header"><div><h2>{t('projects.picture_book.overview.title')}</h2><p>{t('projects.picture_book.immutable_hint')}</p></div></header>
      <dl className="overview-project-facts">
        {details.map((item) => <div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}
      </dl>
      <div className={`overview-picture-book-compatibility ${unsupported ? 'is-unsupported' : preflightQuery.data ? 'is-compatible' : ''}`}>
        <strong>{t('projects.picture_book.overview.compatibility')}</strong>
        {preflightQuery.isLoading ? <span>{t('common.loading')}</span> : null}
        {preflightQuery.data ? <span>{t('projects.picture_book.overview.compatible', { model: preflightQuery.data.model, size: preflightQuery.data.output_size?.value || '—' })}</span> : null}
        {unsupported ? <><span>{t('projects.picture_book.overview.unsupported', { ratio })}</span><small>{t('projects.picture_book.overview.switch_model')}</small></> : null}
        {preflightQuery.isError && !unsupported ? <span>{t('projects.picture_book.overview.unavailable')}</span> : null}
      </div>
    </section>
  )
}

function exportStatusLabel(t, status) {
  const key = { queued: 'projects.exports.status.queued', running: 'projects.exports.status.running', ready: 'projects.exports.status.ready', failed: 'projects.exports.status.failed', cancelled: 'common.status.cancelled' }[status]
  return key ? t(key) : t('common.status.unknown_with_code', { code: status || '—' })
}

function OverviewExportTable({ items, loading, emptyTitle, chapterTitles, chapterFallbackLabel, projectFallbackLabel }) {
  const { formatDateTime, t } = useI18n()
  if (loading) return <p className="overview-export-empty">{t('projects.exports.loading')}</p>
  if (!items.length) return <p className="overview-export-empty">{emptyTitle}</p>
  return (
    <div className="overview-export-table">
      <div className="overview-export-table__head" aria-hidden="true"><span>{t('common.label.file')}</span><span>{t('common.label.status')}</span><span>{t('common.label.details')}</span><span>{t('common.label.actions')}</span></div>
      <ul>
        {items.map((item) => {
          const label = item.scope === 'chapter' ? chapterTitles.get(item.chapter_uuid) || chapterFallbackLabel : projectFallbackLabel
          // i18n-exempt: export formats and fallback filenames are machine identifiers, not interface copy.
          const extension = item.format === 'pdf' ? 'pdf' : 'zip'
          // i18n-exempt: fallback export filenames are machine identifiers, not interface copy.
          const filename = item.filename || item.relative_path?.split('/').filter(Boolean).at(-1) || `${item.scope === 'chapter' ? 'chapter' : 'project'}-comic.${extension}`
          return (
            <li key={item.uuid}>
              <div className="overview-export-file"><strong>{filename}</strong><span>{label}</span></div>
              <span className={`overview-export-status overview-export-status--${item.status}`}>{exportStatusLabel(t, item.status)}</span>
              <dl className="overview-export-metrics">
                <div><dt>{t('common.label.format')}</dt><dd>{item.format?.toUpperCase() || 'ZIP'}</dd></div>
                <div><dt>{t('projects.exports.created')}</dt><dd>{formatDateTime(item.created_at, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</dd></div>
                <div><dt>{t('projects.exports.retention')}</dt><dd>{t('projects.exports.retention_days', { days: item.retention_days || 7 })}</dd></div>
                <div><dt>{t('projects.exports.expires')}</dt><dd>{item.expires_at ? formatDateTime(item.expires_at, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '—'}</dd></div>
                <div><dt>{t('projects.exports.snapshot')}</dt><dd title={item.snapshot_hash}>{item.snapshot_hash?.slice(0, 10) || '—'}</dd></div>
              </dl>
              <div className="overview-export-row-action">{item.download_url ? <a className="button-secondary" href={item.download_url}>{t('common.action.download')}</a> : <span>{item.error_code ? t('common.status.unknown_with_code', { code: item.error_code }) : pendingExportStatuses.has(item.status) ? t('projects.exports.processing') : exportStatusLabel(t, item.status)}</span>}</div>
              {item.error_code ? <p className="overview-export-error" data-machine-value>{item.error_code}</p> : null}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function OverviewExportPagination({ page, pagination, fetching, onPageChange, label }) {
  const { t } = useI18n()
  if ((pagination?.last_page || 1) <= 1) return null
  return <nav className="overview-export-pagination" aria-label={label}><button type="button" className="button-secondary" disabled={page <= 1 || fetching} onClick={() => onPageChange(Math.max(1, page - 1))}>{t('common.action.previous_page')}</button><span>{pagination.current_page} / {pagination.last_page}</span><button type="button" className="button-secondary" disabled={page >= pagination.last_page || fetching} onClick={() => onPageChange(page + 1)}>{t('common.action.next_page')}</button></nav>
}

export function OverviewExportsPanel({ projectUuid, standalone = false }) {
  const { formatCount, t } = useI18n()
  const [chapterUuid, setChapterUuid] = useState('')
  const [projectPage, setProjectPage] = useState(1)
  const [chapterPage, setChapterPage] = useState(1)
  const [exportRequest, setExportRequest] = useState(null)
  const projectQuery = useQuery({ queryKey: ['story-project', projectUuid], queryFn: () => getStoryProject(projectUuid) })
  const chaptersQuery = useQuery({ queryKey: ['story-chapters', projectUuid, 'active'], queryFn: () => listChapters(projectUuid, 'active') })
  const projectExportsQuery = useQuery({
    queryKey: ['comic-exports', projectUuid, 'project', projectPage],
    queryFn: () => listComicExports(projectUuid, { page: projectPage, perPage: exportPageSize, scope: 'project' }),
  })
  const chapterExportsQuery = useQuery({
    queryKey: ['comic-exports', projectUuid, 'chapter', chapterPage],
    queryFn: () => listComicExports(projectUuid, { page: chapterPage, perPage: exportPageSize, scope: 'chapter' }),
  })
  const chapters = chaptersQuery.data?.items || []
  const term = (key, values) => t(formatTerminologyMessageKey(projectQuery.data?.picture_book, key), values)
  const chapterTitles = new Map(chapters.map((chapter) => [chapter.uuid, `${chapter.chapter_code} · ${chapter.title || term('projects.unnamed_chapter')}`]))

  useEffect(() => {
    if (!chapterUuid && chapters[0]) setChapterUuid(chapters[0].uuid)
  }, [chapterUuid, chapters])

  const projectItems = projectExportsQuery.data?.items || []
  const chapterItems = chapterExportsQuery.data?.items || []
  const projectPagination = projectExportsQuery.data?.pagination || { current_page: projectPage, last_page: 1, total: 0 }
  const chapterPagination = chapterExportsQuery.data?.pagination || { current_page: chapterPage, last_page: 1, total: 0 }
  const total = projectPagination.total + chapterPagination.total

  return (
    <div
      className={`project-overview project-overview--exports ${standalone ? 'project-overview--standalone' : ''}`}
      role={standalone ? undefined : 'tabpanel'}
      id={standalone ? undefined : 'overview-panel-exports'}
      aria-labelledby={standalone ? undefined : 'overview-tab-exports'}
    >
      <LocalizedErrorMessage error={projectQuery.error || projectExportsQuery.error || chapterExportsQuery.error || chaptersQuery.error} />
      <section className="overview-card overview-exports-panel">
        <header className="overview-card__header"><div><h1>{t('projects.tab.exports')}</h1><p>{term('projects.exports.description')}</p></div><span>{formatCount('common.count.items', total)}</span></header>
        <div className="overview-export-toolbar">
          <div><h2>{t('projects.exports.project_records')}</h2><span>{projectPagination.total}</span></div>
          <button type="button" className="button-secondary" onClick={() => setExportRequest(comicExportDialogRequest('project'))}>{t('projects.exports.new_project')}</button>
        </div>
        <OverviewExportTable items={projectItems} loading={projectExportsQuery.isLoading} emptyTitle={t('projects.exports.empty_project')} chapterTitles={chapterTitles} chapterFallbackLabel={term('projects.exports.chapter_label')} projectFallbackLabel={term('projects.exports.project_label')} />
        <OverviewExportPagination page={projectPage} pagination={projectPagination} fetching={projectExportsQuery.isFetching} onPageChange={setProjectPage} label={term('projects.exports.pagination')} />
        <div className="overview-export-toolbar overview-export-toolbar--chapter">
          <div><h2>{term('projects.exports.chapter_records')}</h2><span>{chapterPagination.total}</span></div>
          <form onSubmit={(event) => { event.preventDefault(); setExportRequest(comicExportDialogRequest('chapter', chapterUuid, chapterTitles.get(chapterUuid) || '')) }}><label>{term('story.chapter')}<select aria-label={term('projects.exports.chapter_select')} value={chapterUuid} onChange={(event) => setChapterUuid(event.target.value)} disabled={!chapters.length}>{chapters.map((chapter) => <option value={chapter.uuid} key={chapter.uuid}>{chapter.chapter_code} · {chapter.title || term('projects.unnamed_chapter')}</option>)}</select></label><button type="submit" className="button-secondary" disabled={!chapterUuid}>{term('projects.exports.new_chapter')}</button></form>
        </div>
        <OverviewExportTable items={chapterItems} loading={chapterExportsQuery.isLoading} emptyTitle={term('projects.exports.empty_chapter')} chapterTitles={chapterTitles} chapterFallbackLabel={term('projects.exports.chapter_label')} projectFallbackLabel={term('projects.exports.project_label')} />
        <OverviewExportPagination page={chapterPage} pagination={chapterPagination} fetching={chapterExportsQuery.isFetching} onPageChange={setChapterPage} label={term('projects.exports.pagination')} />
      </section>
      {exportRequest ? <ComicExportDialog projectUuid={projectUuid} request={exportRequest} onClose={() => setExportRequest(null)} /> : null}
    </div>
  )
}
