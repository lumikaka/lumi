import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  BookOpenText,
  Bot,
  Check,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  ChevronDown,
  GripVertical,
  History,
  Images,
  Maximize2,
  Minimize2,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Shapes,
  Sparkles,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import { Link, Navigate, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'

import { createAssetUpload } from '../api/assets.js'
import {
  createStoryProfileGeneration,
  createStoryProfileReconstruction,
  listTasks,
} from '../api/ai.js'
import {
  createComicSection,
  createPremiseAsset,
  createPremiseAssetVariant,
  createStoryboard,
  deleteComicSection,
  generateChapterImagesBatch,
  generatePremiseAssetVariant,
  generateSectionImage,
  getPremise,
  getPremiseAsset,
  importSectionImage,
  listComicSections,
  listImageVariants,
  listPremiseAssets,
  listPremiseAssetVariants,
  listProductionTasks,
  listStoryboards,
  reorderComicSections,
  restorePremiseAsset,
  selectImageVariant,
  selectPremiseAssetVariant,
  selectStoryboard,
  setComicSectionPremiseAssets,
  trashPremiseAsset,
  updateComicSection,
  updatePremiseAsset,
} from '../api/production.js'
import {
  createChapter,
  getChapter,
  getStoryProfile,
  importExternalStoryMD,
  listChapters,
  listStoryProfileVersions,
  regenerateStoryMD,
  reorderChapters,
  restoreChapter,
  restoreStoryProfileVersion,
  trashChapter,
  updateChapter,
  updateStoryProfile,
  updateStoryProject,
} from '../api/story.js'
import { useI18n } from '../i18n/useI18n.js'
import { sourceTypeLabel } from '../i18n/labels.js'
import ProjectDashboardModeSetting from '../components/ProjectDashboardModeSetting.jsx'
import { projectRoute } from '../projectRoutes.js'
import {
  COMIC_PAGE_ROLES,
  comicBodyReorderUuids,
  comicPageFallbackTitle,
  comicPageLabel,
  comicPageRole,
  comicPageRoleOptionDisabled,
  reorderedComicBodyUuids,
} from './comicPageRoles.js'
import { pictureBookFormatKey, pictureBookRatio } from './pictureBookProfile.js'
import {
  firstReadySimpleImage,
  orderedSimplePages,
  simpleStoryExcerpt,
  storyDocumentBlocks,
} from './simpleProjectState.js'

const ACTIVE_TASK_STATUSES = new Set(['queued', 'running', 'waiting_for_input'])
const PREMISE_ASSET_TYPES = ['character', 'scene', 'prop']

export function SimpleHomePage({ project, projectUuid, projectQuery }) {
  const { t } = useI18n()
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(project?.name || '')
  const [description, setDescription] = useState(project?.description || '')
  const [language, setLanguage] = useState(project?.generation_language || 'zh-Hans')
  const [feedback, setFeedback] = useState(null)
  const profileQuery = useQuery({ queryKey: ['story-profile', projectUuid], queryFn: () => getStoryProfile(projectUuid) })
  const premiseQuery = useQuery({ queryKey: ['premise', projectUuid], queryFn: () => getPremise(projectUuid) })
  const assetsQuery = useQuery({ queryKey: ['premise-assets', projectUuid, '', false], queryFn: () => listPremiseAssets(projectUuid, { state: 'active' }) })
  const chaptersQuery = useQuery({ queryKey: ['story-chapters', projectUuid, 'active'], queryFn: () => listChapters(projectUuid, 'active') })
  const chapters = chaptersQuery.data?.items || []
  const sectionQueries = useQueries({ queries: chapters.map((chapter) => ({ queryKey: ['comic-sections', projectUuid, chapter.uuid], queryFn: () => listComicSections(projectUuid, chapter.uuid) })) })
  const sections = sectionQueries.flatMap((query) => query.data?.items || [])
  const cover = firstReadySimpleImage(sections)
  const assets = assetsQuery.data?.items || []
  const ratio = pictureBookRatio(project?.picture_book)
  const story = profileQuery.data?.story_md || ''
  const displayDescription = project?.description || simpleStoryExcerpt(story) || t('simple.home.description_fallback')
  const loading = projectQuery.isLoading || profileQuery.isLoading || premiseQuery.isLoading || assetsQuery.isLoading || chaptersQuery.isLoading
  const error = projectQuery.error || profileQuery.error || premiseQuery.error || assetsQuery.error || chaptersQuery.error || sectionQueries.find((query) => query.error)?.error
  const saveProject = useMutation({
    mutationFn: () => updateStoryProject(projectUuid, {
      name: name.trim(),
      description: description.trim(),
      generation_language: language,
      expected_revision: project.revision,
    }),
    onMutate: () => setFeedback(null),
    onSuccess: (updated) => {
      queryClient.setQueryData(['story-project', projectUuid], updated)
      void queryClient.invalidateQueries({ queryKey: ['recent-projects'] })
      setFeedback({ kind: 'success', message: t('simple.feedback.saved') })
      setEditing(false)
    },
    onError: (mutationError) => {
      setFeedback({ kind: 'error', error: mutationError })
      void projectQuery.refetch()
    },
  })
  const openEditor = () => {
    setName(project?.name || '')
    setDescription(project?.description || '')
    setLanguage(project?.generation_language || 'zh-Hans')
    setFeedback(null)
    setEditing(true)
  }
  const configurationDirty = Boolean(project) && (
    name !== (project.name || '')
    || description !== (project.description || '')
    || language !== (project.generation_language || 'zh-Hans')
  )
  useEffect(() => {
    if (!location.state?.openProjectConfiguration || !project) return
    openEditor()
    navigate({ pathname: location.pathname, search: location.search }, { replace: true, state: null })
  }, [location.pathname, location.search, location.state, navigate, project]) // eslint-disable-line react-hooks/exhaustive-deps
  if (loading && !project) return <SimpleLoading message={t('simple.loading.project')} />
  return (
    <div className="simple-project-page simple-project-home">
      <SimpleError error={error} onRetry={() => { projectQuery.refetch(); profileQuery.refetch(); premiseQuery.refetch(); assetsQuery.refetch(); chaptersQuery.refetch(); sectionQueries.forEach((query) => query.refetch()) }} />
      <SimpleFeedback feedback={feedback} onDismiss={() => setFeedback(null)} />
      <section className="simple-home-hero">
        <div className="simple-home-cover" style={{ '--simple-page-ratio': ratio ? `${ratio.width} / ${ratio.height}` : '4 / 3' }}>
          <SimpleImage asset={cover} alt={project?.name || t('projects.fallback_name')} fallbackIcon={<BookOpenText size={28} aria-hidden="true" />} />
        </div>
        <div className="simple-home-hero__copy">
          <header><h1 data-no-i18n>{project?.name || t('projects.fallback_name')}</h1><div><Link className="simple-button" to={projectRoute(projectUuid, 'story', location.search)}><BookOpenText size={15} aria-hidden="true" />{t('simple.home.story_action')}</Link><button className="simple-button simple-button--secondary" type="button" onClick={openEditor}><Pencil size={15} aria-hidden="true" />{t('projects.configuration')}</button></div></header>
          <p data-user-content>{displayDescription}</p>
          <dl>
            <div><dt>{t('simple.home.style')}</dt><dd data-user-content>{premiseQuery.data?.default_style || '—'}</dd></div>
            <div><dt>{t('simple.home.language')}</dt><dd>{generationLanguageLabel(t, project?.generation_language)}</dd></div>
            <div><dt>{t('simple.home.format')}</dt><dd>{project?.picture_book ? t(pictureBookFormatKey(project.picture_book.format)) : '—'}</dd></div>
            <div><dt>{t('simple.home.ratio')}</dt><dd>{ratio?.value || '—'}</dd></div>
          </dl>
        </div>
      </section>
      <section className="simple-home-section">
        <header><div><h2>{t('simple.home.settings_title')}</h2><p>{t('simple.home.settings_body')}</p></div><Link className="simple-button simple-button--secondary" to={projectRoute(projectUuid, 'premise', location.search)}>{t('simple.home.settings_action')}<ChevronRight size={15} aria-hidden="true" /></Link></header>
        {assets.length ? <div className="simple-setting-grid simple-setting-grid--preview">{assets.slice(0, 3).map((asset) => <SimpleSettingCard asset={asset} key={asset.uuid} projectUuid={projectUuid} />)}</div> : <p className="simple-empty-copy">{t('simple.home.no_settings')}</p>}
      </section>
      <section className="simple-home-section">
        <header><div><h2>{t('simple.home.books_title')}</h2><p>{t('simple.home.books_progress', { count: chapters.length })}</p></div><Link className="simple-button simple-button--secondary" to={projectRoute(projectUuid, 'chapters', location.search)}>{t('simple.home.books_action')}<ChevronRight size={15} aria-hidden="true" /></Link></header>
        {chapters.length ? <SimpleBookPreviewGrid projectUuid={projectUuid} chapters={chapters} sectionQueries={sectionQueries} /> : <p className="simple-empty-copy">{t('simple.home.no_books')}</p>}
      </section>
      {editing ? (
        <SimpleDialog title={t('projects.configuration')} onClose={() => !saveProject.isPending && setEditing(false)}>
          <form className="simple-form" onSubmit={(event) => { event.preventDefault(); saveProject.mutate() }}>
            <label>{t('common.label.name')}<input autoFocus maxLength={120} value={name} onChange={(event) => setName(event.target.value)} /></label>
            <label>{t('common.label.description')}<textarea maxLength={2000} value={description} onChange={(event) => setDescription(event.target.value)} /></label>
            <label>{t('simple.home.language')}<select value={language} onChange={(event) => setLanguage(event.target.value)}><option value="zh-Hans">{t('common.language.zh_hans')}</option><option value="en">{t('common.language.en')}</option></select></label>
            <label>{t('simple.home.format')}<input value={project?.picture_book ? t(pictureBookFormatKey(project.picture_book.format)) : '—'} disabled /></label>
            <p className="simple-form__hint">{t('simple.project.immutable_hint')}</p>
            <ProjectDashboardModeSetting projectUuid={projectUuid} dirty={configurationDirty} disabled={saveProject.isPending} />
            <SimpleDialogActions pending={saveProject.isPending} submitDisabled={!name.trim()} onCancel={() => setEditing(false)} />
          </form>
        </SimpleDialog>
      ) : null}
    </div>
  )
}

export function SimpleStoryPage({ project, projectUuid }) {
  const { formatDateTime, t } = useI18n()
  const location = useLocation()
  const queryClient = useQueryClient()
  const profileQuery = useQuery({ queryKey: ['story-profile', projectUuid], queryFn: () => getStoryProfile(projectUuid), refetchOnWindowFocus: true })
  const historyQuery = useQuery({ queryKey: ['story-profile-history', projectUuid], queryFn: () => listStoryProfileVersions(projectUuid) })
  const tasksQuery = useQuery({ queryKey: ['story-tasks', projectUuid], queryFn: () => listTasks(projectUuid, { limit: 100 }) })
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [generationPrompt, setGenerationPrompt] = useState('')
  const [restoreVersion, setRestoreVersion] = useState(null)
  const [feedback, setFeedback] = useState(null)
  const completedTask = useRef('')
  const profile = profileQuery.data
  const versions = historyQuery.data?.items || []
  const task = (tasksQuery.data?.items || []).find((item) => ['story_profile_generation', 'story_profile_from_chapters'].includes(item.kind))
  const taskActive = Boolean(task && ACTIVE_TASK_STATUSES.has(task.status))

  useEffect(() => {
    if (!editing && profile) setDraft(profile.story_md || '')
  }, [editing, profile?.revision, profile?.story_md])

  useEffect(() => {
    if (task?.status !== 'completed' || completedTask.current === task.uuid) return
    completedTask.current = task.uuid
    void queryClient.invalidateQueries({ queryKey: ['story-profile', projectUuid] })
    void queryClient.invalidateQueries({ queryKey: ['story-profile-history', projectUuid] })
    void queryClient.invalidateQueries({ queryKey: ['story-chapters', projectUuid] })
    setFeedback({ kind: 'success', message: t('simple.story.generation_complete') })
  }, [projectUuid, queryClient, task?.status, task?.uuid, t])

  const applyProfile = (updated, message = t('simple.feedback.saved')) => {
    queryClient.setQueryData(['story-profile', projectUuid], updated)
    void queryClient.invalidateQueries({ queryKey: ['story-profile-history', projectUuid] })
    setDraft(updated.story_md || '')
    setEditing(false)
    setRestoreVersion(null)
    setFeedback({ kind: 'success', message })
  }
  const mutationError = (error) => {
    setFeedback({ kind: 'error', error })
    void profileQuery.refetch()
  }
  const save = useMutation({ mutationFn: () => updateStoryProfile(projectUuid, { story_md: draft, expected_revision: profile.revision }), onSuccess: (updated) => applyProfile(updated), onError: mutationError })
  const restore = useMutation({ mutationFn: (version) => restoreStoryProfileVersion(projectUuid, version.uuid, profile.revision), onSuccess: (updated) => applyProfile(updated, t('simple.story.restored')), onError: mutationError })
  const regenerate = useMutation({ mutationFn: () => regenerateStoryMD(projectUuid, profile.revision), onSuccess: (updated) => applyProfile(updated, t('simple.story.projection_regenerated')), onError: mutationError })
  const importExternal = useMutation({ mutationFn: () => importExternalStoryMD(projectUuid, profile.revision), onSuccess: (updated) => applyProfile(updated, t('simple.story.external_imported')), onError: mutationError })
  const generate = useMutation({
    mutationFn: () => createStoryProfileGeneration(projectUuid, { prompt: generationPrompt.trim(), chapter_count: 1, parameters: { temperature: 0.7 }, idempotency_key: `simple-story-${Date.now()}` }),
    onSuccess: () => { setGenerationPrompt(''); setFeedback({ kind: 'success', message: t('simple.story.generation_started') }); void queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] }) },
    onError: mutationError,
  })
  const reconstruct = useMutation({
    mutationFn: () => createStoryProfileReconstruction(projectUuid, { parameters: { temperature: 0.4 }, idempotency_key: `simple-story-chapters-${Date.now()}` }),
    onSuccess: () => { setFeedback({ kind: 'success', message: t('simple.story.generation_started') }); void queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] }) },
    onError: mutationError,
  })

  if (profileQuery.isLoading) return <SimpleLoading message={t('simple.loading.story')} />
  if (profileQuery.isError && !profile) return <SimpleError error={profileQuery.error} onRetry={() => profileQuery.refetch()} />
  const blocks = storyDocumentBlocks(profile?.story_md)
  return (
    <div className="simple-project-page simple-story-page">
      <SimplePageHeading title={t('simple.story.title')} description={t('simple.story.description_editable')} backTo={projectRoute(projectUuid, '', location.search)} actions={<button className="simple-button" type="button" disabled={profile?.projection_state !== 'synced'} onClick={() => { setDraft(profile.story_md); setEditing(true); setFeedback(null) }}><Pencil size={15} aria-hidden="true" />{t('common.action.edit')}</button>} />
      <SimpleFeedback feedback={feedback} onDismiss={() => setFeedback(null)} />
      <SimpleError error={historyQuery.error || tasksQuery.error} onRetry={() => { historyQuery.refetch(); tasksQuery.refetch() }} />
      {profile?.projection_state === 'conflict' ? <section className="simple-conflict" role="alert"><div><strong>{t('simple.story.conflict_title')}</strong><p>{t('simple.story.conflict_body')}</p></div><button type="button" disabled={importExternal.isPending} onClick={() => importExternal.mutate()}>{t('simple.story.import_external')}</button><button type="button" className="simple-button--secondary" disabled={regenerate.isPending} onClick={() => regenerate.mutate()}>{t('simple.story.use_database')}</button></section> : null}
      <div className="simple-story-layout">
        <article className="simple-story-document">
          {blocks.length ? blocks.map((block, index) => block.type === 'heading' ? <h2 key={index} data-user-content>{block.text}</h2> : <p key={index} data-user-content>{block.text}</p>) : <p className="simple-empty-copy">{t('simple.story.empty_editable')}</p>}
        </article>
        <aside className="simple-story-meta">
          <span>{t('simple.story.current_version')}</span><strong>v{profile?.version_no || '—'}</strong><small>{project?.name}</small>
          <dl><div><dt>{t('common.label.revision')}</dt><dd>r{profile?.revision}</dd></div><div><dt>{t('common.label.source')}</dt><dd>{sourceTypeLabel(t, profile?.source_type)}</dd></div><div><dt>{t('simple.story.file_status')}</dt><dd>{profile?.projection_state}</dd></div></dl>
          <button className="simple-button simple-button--secondary" type="button" disabled={regenerate.isPending} onClick={() => regenerate.mutate()}><RefreshCw size={15} aria-hidden="true" />{t('simple.story.regenerate_file')}</button>
        </aside>
      </div>
      <section className="simple-story-generation">
        <header><div><h2>{t('simple.story.generate_title')}</h2><p>{t('simple.story.generate_body')}</p></div>{task ? <SimpleTaskStatus task={task} /> : null}</header>
        <div><input value={generationPrompt} onChange={(event) => setGenerationPrompt(event.target.value)} placeholder={t('simple.story.generate_placeholder')} /><button type="button" disabled={!generationPrompt.trim() || taskActive || generate.isPending} onClick={() => generate.mutate()}><Sparkles size={15} aria-hidden="true" />{t('simple.story.generate')}</button><button type="button" className="simple-button--secondary" disabled={taskActive || reconstruct.isPending} onClick={() => reconstruct.mutate()}>{t('simple.story.reconstruct')}</button></div>
      </section>
      <section className="simple-version-list">
        <header><div><h2>{t('simple.story.history')}</h2><p>{t('simple.story.history_body')}</p></div><span>{versions.length}</span></header>
        <div>{versions.map((version) => <article className={version.uuid === profile?.uuid ? 'is-current' : ''} key={version.uuid}><div><strong>v{version.version_no}</strong><span>{sourceTypeLabel(t, version.source_type)}</span><time dateTime={version.created_at}>{formatDateTime(version.created_at)}</time></div><button type="button" disabled={version.uuid === profile?.uuid || restore.isPending || profile?.projection_state !== 'synced'} onClick={() => setRestoreVersion(version)}>{version.uuid === profile?.uuid ? t('simple.version.current') : t('common.action.restore')}</button></article>)}</div>
      </section>
      {editing ? <SimpleDialog wide title={t('simple.story.edit_title')} onClose={() => !save.isPending && setEditing(false)}><form className="simple-form" onSubmit={(event) => { event.preventDefault(); save.mutate() }}><label>{t('simple.story.markdown')}<textarea className="simple-story-editor" autoFocus value={draft} onChange={(event) => setDraft(event.target.value)} /></label><p className="simple-form__hint">{t('simple.story.save_hint')}</p><SimpleDialogActions pending={save.isPending} submitDisabled={!draft.trim() || draft === profile.story_md || profile.projection_state !== 'synced'} onCancel={() => setEditing(false)} /></form></SimpleDialog> : null}
      {restoreVersion ? <SimpleConfirm title={t('simple.story.restore_title')} body={t('simple.story.restore_body', { version: restoreVersion.version_no })} pending={restore.isPending} onCancel={() => setRestoreVersion(null)} onConfirm={() => restore.mutate(restoreVersion)} /> : null}
    </div>
  )
}

export function SimpleSettingsPage({ projectUuid }) {
  const { t } = useI18n()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [showTrash, setShowTrash] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteAsset, setDeleteAsset] = useState(null)
  const [feedback, setFeedback] = useState(null)
  const [draft, setDraft] = useState(emptyAssetDraft)
  const activeQuery = useQuery({ queryKey: ['premise-assets', projectUuid, '', false], queryFn: () => listPremiseAssets(projectUuid, { state: 'active' }) })
  const trashQuery = useQuery({ queryKey: ['premise-assets', projectUuid, '', true], queryFn: () => listPremiseAssets(projectUuid, { state: 'trashed' }) })
  const selectedQuery = showTrash ? trashQuery : activeQuery
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['premise-assets', projectUuid] })
    void queryClient.invalidateQueries({ queryKey: ['premise-asset', projectUuid] })
    void queryClient.invalidateQueries({ queryKey: ['story-project', projectUuid] })
  }
  const create = useMutation({
    mutationFn: async () => {
      const upload = await createAssetUpload(projectUuid, { purpose: 'premise_asset', displayName: draft.title.trim(), file: draft.file })
      return createPremiseAsset(projectUuid, { upload_uuid: upload.uuid, asset_type: draft.assetType, title: draft.title.trim(), summary: draft.summary.trim(), tags: splitTags(draft.tags), position: {}, crop: {} })
    },
    onSuccess: () => { refresh(); setCreating(false); setDraft(emptyAssetDraft()); setFeedback({ kind: 'success', message: t('simple.settings.created') }) },
    onError: (error) => setFeedback({ kind: 'error', error }),
  })
  const trash = useMutation({ mutationFn: (asset) => trashPremiseAsset(projectUuid, asset.uuid, asset.revision), onSuccess: () => { refresh(); setDeleteAsset(null); setFeedback({ kind: 'success', message: t('simple.settings.trashed') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const restore = useMutation({ mutationFn: (asset) => restorePremiseAsset(projectUuid, asset.uuid, asset.revision), onSuccess: () => { refresh(); setFeedback({ kind: 'success', message: t('simple.settings.restored') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  if (selectedQuery.isLoading) return <SimpleLoading message={t('simple.loading.settings')} />
  const assets = selectedQuery.data?.items || []
  return (
    <div className="simple-project-page simple-settings-page">
      <SimplePageHeading title={t('simple.settings.title')} description={t('simple.settings.description_manage')} backTo={projectRoute(projectUuid, '', location.search)} actions={<button className="simple-button" type="button" onClick={() => { setDraft(emptyAssetDraft()); setCreating(true); setFeedback(null) }}><Plus size={15} aria-hidden="true" />{t('simple.settings.add')}</button>} />
      <div className="simple-segmented" role="group" aria-label={t('simple.settings.filter')}><button type="button" aria-pressed={!showTrash} onClick={() => setShowTrash(false)}>{t('simple.settings.active')}</button><button type="button" aria-pressed={showTrash} onClick={() => setShowTrash(true)}>{t('simple.settings.trash')}</button></div>
      <SimpleFeedback feedback={feedback} onDismiss={() => setFeedback(null)} />
      <SimpleError error={selectedQuery.error} onRetry={() => selectedQuery.refetch()} />
      {assets.length ? <div className="simple-setting-grid">{assets.map((asset) => <article className="simple-setting-card-wrap" key={asset.uuid}><SimpleSettingCard asset={asset} projectUuid={projectUuid} /><footer>{showTrash ? <button type="button" disabled={restore.isPending} onClick={() => restore.mutate(asset)}>{t('common.action.restore')}</button> : <button className="simple-danger-action" type="button" disabled={trash.isPending} onClick={() => setDeleteAsset(asset)}><Trash2 size={14} aria-hidden="true" />{t('simple.action.trash')}</button>}</footer></article>)}</div> : <SimpleEmptyState icon={<Shapes size={25} aria-hidden="true" />} title={t(showTrash ? 'simple.settings.trash_empty' : 'simple.settings.empty_manage')} actions={!showTrash ? <button className="simple-button" type="button" onClick={() => setCreating(true)}>{t('simple.settings.add')}</button> : null} />}
      {creating ? <SimpleDialog title={t('simple.settings.add_title')} onClose={() => !create.isPending && setCreating(false)}><form className="simple-form" onSubmit={(event) => { event.preventDefault(); create.mutate() }}><label>{t('common.label.type')}<select value={draft.assetType} onChange={(event) => setDraft((value) => ({ ...value, assetType: event.target.value }))}>{PREMISE_ASSET_TYPES.map((type) => <option key={type} value={type}>{assetTypeLabel(t, type)}</option>)}</select></label><label>{t('common.label.name')}<input autoFocus maxLength={160} value={draft.title} onChange={(event) => setDraft((value) => ({ ...value, title: event.target.value }))} /></label><label>{t('simple.setting.summary')}<textarea maxLength={12000} value={draft.summary} onChange={(event) => setDraft((value) => ({ ...value, summary: event.target.value }))} /></label><label>{t('simple.setting.tags')}<input value={draft.tags} onChange={(event) => setDraft((value) => ({ ...value, tags: event.target.value }))} placeholder={t('simple.setting.tags_hint')} /></label><label>{t('simple.setting.image')}<input type="file" accept="image/*" onChange={(event) => setDraft((value) => ({ ...value, file: event.target.files?.[0] || null }))} /></label><SimpleDialogActions pending={create.isPending} submitDisabled={!draft.title.trim() || !draft.file} onCancel={() => setCreating(false)} /></form></SimpleDialog> : null}
      {deleteAsset ? <SimpleConfirm danger title={t('simple.settings.trash_title')} body={t('simple.settings.trash_body', { title: deleteAsset.title })} pending={trash.isPending} onCancel={() => setDeleteAsset(null)} onConfirm={() => trash.mutate(deleteAsset)} /> : null}
    </div>
  )
}

export function SimpleSettingDetailPage({ projectUuid }) {
  const { formatDateTime, t } = useI18n()
  const { assetUuid } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const assetQuery = useQuery({ queryKey: ['premise-asset', projectUuid, assetUuid], queryFn: () => getPremiseAsset(projectUuid, assetUuid) })
  const variantsQuery = useQuery({ queryKey: ['premise-variants', projectUuid, assetUuid], queryFn: () => listPremiseAssetVariants(projectUuid, assetUuid) })
  const tasksQuery = useQuery({ queryKey: ['production-tasks', projectUuid], queryFn: () => listProductionTasks(projectUuid) })
  const [editing, setEditing] = useState(false)
  const [title, setTitle] = useState('')
  const [summary, setSummary] = useState('')
  const [tags, setTags] = useState('')
  const [assetType, setAssetType] = useState('character')
  const [replacementFile, setReplacementFile] = useState(null)
  const [prompt, setPrompt] = useState('')
  const [confirmTrash, setConfirmTrash] = useState(false)
  const [feedback, setFeedback] = useState(null)
  const completedTask = useRef('')
  const asset = assetQuery.data
  const variants = variantsQuery.data?.items || []
  const task = (tasksQuery.data?.items || []).find((item) => item.kind === 'premise_asset_generation' && item.resource_uuid === assetUuid)
  const taskActive = Boolean(task && ACTIVE_TASK_STATUSES.has(task.status))

  useEffect(() => {
    if (!editing && asset) {
      setTitle(asset.title || '')
      setSummary(asset.summary || '')
      setTags((asset.tags || []).join(', '))
      setAssetType(asset.asset_type || 'character')
    }
  }, [asset?.revision, editing])

  useEffect(() => {
    if (task?.status !== 'completed' || completedTask.current === task.uuid) return
    completedTask.current = task.uuid
    void queryClient.invalidateQueries({ queryKey: ['premise-asset', projectUuid, assetUuid] })
    void queryClient.invalidateQueries({ queryKey: ['premise-variants', projectUuid, assetUuid] })
    void queryClient.invalidateQueries({ queryKey: ['premise-assets', projectUuid] })
    setFeedback({ kind: 'success', message: t('simple.setting.generation_complete') })
  }, [assetUuid, projectUuid, queryClient, task?.status, task?.uuid, t])

  const refresh = (updated) => {
    if (updated) queryClient.setQueryData(['premise-asset', projectUuid, assetUuid], updated)
    void queryClient.invalidateQueries({ queryKey: ['premise-variants', projectUuid, assetUuid] })
    void queryClient.invalidateQueries({ queryKey: ['premise-assets', projectUuid] })
  }
  const update = useMutation({ mutationFn: () => updatePremiseAsset(projectUuid, assetUuid, { asset_type: assetType, title: title.trim(), summary: summary.trim(), tags: splitTags(tags), expected_revision: asset.revision }), onSuccess: (updated) => { refresh(updated); setEditing(false); setFeedback({ kind: 'success', message: t('simple.feedback.saved') }) }, onError: (error) => { setFeedback({ kind: 'error', error }); void assetQuery.refetch() } })
  const trash = useMutation({ mutationFn: () => trashPremiseAsset(projectUuid, assetUuid, asset.revision), onSuccess: () => { refresh(); navigate(projectRoute(projectUuid, 'premise', location.search)); }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const restore = useMutation({ mutationFn: () => restorePremiseAsset(projectUuid, assetUuid, asset.revision), onSuccess: (updated) => { refresh(updated); setFeedback({ kind: 'success', message: t('simple.settings.restored') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const importVariant = useMutation({ mutationFn: async () => { const upload = await createAssetUpload(projectUuid, { purpose: 'premise_asset', displayName: asset.title, file: replacementFile }); return createPremiseAssetVariant(projectUuid, assetUuid, { upload_uuid: upload.uuid, crop: {}, expected_revision: asset.revision }) }, onSuccess: (updated) => { setReplacementFile(null); refresh(updated); setFeedback({ kind: 'success', message: t('simple.setting.imported') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const selectVariant = useMutation({ mutationFn: (variant) => selectPremiseAssetVariant(projectUuid, assetUuid, variant.uuid, asset.revision), onSuccess: (updated) => { refresh(updated); setFeedback({ kind: 'success', message: t('simple.setting.version_selected') }) }, onError: (error) => { setFeedback({ kind: 'error', error }); void assetQuery.refetch() } })
  const generate = useMutation({ mutationFn: () => generatePremiseAssetVariant(projectUuid, assetUuid, { prompt: prompt.trim() || asset.summary || asset.title, idempotency_key: `simple-premise-variant-${Date.now()}` }), onSuccess: () => { setPrompt(''); void queryClient.invalidateQueries({ queryKey: ['production-tasks', projectUuid] }); setFeedback({ kind: 'success', message: t('simple.setting.generation_started') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })

  if (assetQuery.isLoading) return <SimpleLoading message={t('simple.loading.settings')} />
  if (assetQuery.isError && !asset) return <SimpleError error={assetQuery.error} onRetry={() => assetQuery.refetch()} />
  if (!asset) return <SimpleNotFound projectUuid={projectUuid} />
  const chatSearch = withChatReference(location.search, 'premise_asset', asset.uuid, asset.title)
  return (
    <div className="simple-project-page simple-setting-detail">
      <SimplePageHeading title={asset.title} eyebrow={assetTypeLabel(t, asset.asset_type)} backLabel={t('simple.setting.back')} backTo={projectRoute(projectUuid, 'premise', location.search)} actions={<><Link className="simple-button simple-button--secondary" to={{ pathname: location.pathname, search: chatSearch }}><Bot size={15} aria-hidden="true" />{t('simple.setting.ask_agent')}</Link>{asset.deleted_at ? <button className="simple-button" type="button" disabled={restore.isPending} onClick={() => restore.mutate()}>{t('common.action.restore')}</button> : <button className="simple-button" type="button" onClick={() => setEditing(true)}><Pencil size={15} aria-hidden="true" />{t('common.action.edit')}</button>}</>} />
      <SimpleFeedback feedback={feedback} onDismiss={() => setFeedback(null)} />
      <SimpleError error={variantsQuery.error || tasksQuery.error} onRetry={() => { variantsQuery.refetch(); tasksQuery.refetch() }} />
      <div className="simple-setting-detail__layout">
        <div className="simple-setting-detail__image"><SimpleImage asset={asset.current_variant?.asset} alt={asset.title} fallbackText={t('simple.setting.no_image')} /></div>
        <article className="simple-setting-detail__copy">
          <section><span>{t('simple.setting.summary')}</span><p data-user-content>{asset.summary || t('simple.setting.no_summary')}</p></section>
          <section><span>{t('simple.setting.tags')}</span>{asset.tags?.length ? <div className="simple-tag-list">{asset.tags.map((tag) => <i key={tag} data-user-content>{tag}</i>)}</div> : <p>{t('common.label.none')}</p>}</section>
          <dl><div><dt>{t('common.label.type')}</dt><dd>{assetTypeLabel(t, asset.asset_type)}</dd></div><div><dt>{t('simple.story.current_version')}</dt><dd>v{asset.current_variant?.version_no || '—'}</dd></div><div><dt>{t('common.label.source')}</dt><dd>{asset.current_variant ? sourceTypeLabel(t, asset.current_variant.source_type) : '—'}</dd></div><div><dt>{t('common.label.revision')}</dt><dd>r{asset.revision}</dd></div></dl>
          {!asset.deleted_at ? <button className="simple-danger-action" type="button" onClick={() => setConfirmTrash(true)}><Trash2 size={14} aria-hidden="true" />{t('simple.action.trash')}</button> : null}
        </article>
      </div>
      {!asset.deleted_at ? <section className="simple-generation-card"><header><div><h2>{t('simple.setting.generate_title')}</h2><p>{t('simple.setting.generate_body')}</p></div>{task ? <SimpleTaskStatus task={task} /> : null}</header><div><input value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder={t('simple.setting.generate_placeholder')} /><button type="button" disabled={!asset.current_variant || taskActive || generate.isPending} onClick={() => generate.mutate()}><Sparkles size={15} aria-hidden="true" />{t('simple.setting.generate')}</button></div></section> : null}
      <section className="simple-version-gallery">
        <header><div><h2>{t('simple.setting.versions')}</h2><p>{t('simple.setting.versions_body')}</p></div>{!asset.deleted_at ? <label className="simple-button simple-button--secondary"><Upload size={15} aria-hidden="true" />{t('simple.setting.import_image')}<input type="file" accept="image/*" onChange={(event) => setReplacementFile(event.target.files?.[0] || null)} /></label> : null}</header>
        {replacementFile ? <div className="simple-upload-confirm"><span data-user-content>{replacementFile.name}</span><button type="button" disabled={importVariant.isPending} onClick={() => importVariant.mutate()}>{t('simple.setting.import_now')}</button><button type="button" className="simple-button--secondary" onClick={() => setReplacementFile(null)}>{t('common.action.cancel')}</button></div> : null}
        <div>{variants.map((variant) => <article className={variant.uuid === asset.current_variant?.uuid ? 'is-current' : ''} key={variant.uuid}><SimpleImage asset={variant.asset} alt={`${asset.title} v${variant.version_no}`} /><footer><div><strong>v{variant.version_no}</strong><span>{sourceTypeLabel(t, variant.source_type)}</span><time dateTime={variant.created_at}>{formatDateTime(variant.created_at)}</time></div><button type="button" disabled={asset.deleted_at || variant.uuid === asset.current_variant?.uuid || selectVariant.isPending} onClick={() => selectVariant.mutate(variant)}>{variant.uuid === asset.current_variant?.uuid ? t('simple.version.current') : t('simple.version.use')}</button></footer></article>)}</div>
      </section>
      {editing ? <SimpleDialog title={t('simple.setting.edit_title')} onClose={() => !update.isPending && setEditing(false)}><form className="simple-form" onSubmit={(event) => { event.preventDefault(); update.mutate() }}><label>{t('common.label.type')}<select value={assetType} onChange={(event) => setAssetType(event.target.value)}>{PREMISE_ASSET_TYPES.map((type) => <option key={type} value={type}>{assetTypeLabel(t, type)}</option>)}</select></label><label>{t('common.label.name')}<input autoFocus maxLength={160} value={title} onChange={(event) => setTitle(event.target.value)} /></label><label>{t('simple.setting.summary')}<textarea maxLength={12000} value={summary} onChange={(event) => setSummary(event.target.value)} /></label><label>{t('simple.setting.tags')}<input value={tags} onChange={(event) => setTags(event.target.value)} /></label><SimpleDialogActions pending={update.isPending} submitDisabled={!title.trim()} onCancel={() => setEditing(false)} /></form></SimpleDialog> : null}
      {confirmTrash ? <SimpleConfirm danger title={t('simple.settings.trash_title')} body={t('simple.settings.trash_body', { title: asset.title })} pending={trash.isPending} onCancel={() => setConfirmTrash(false)} onConfirm={() => trash.mutate()} /> : null}
    </div>
  )
}

export function SimpleBooksPage({ projectUuid }) {
  const { t } = useI18n()
  const location = useLocation()
  const [searchParams, setSearchParams] = useSearchParams()
  const queryClient = useQueryClient()
  const showTrash = searchParams.get('state') === 'trashed'
  const homeSearch = new URLSearchParams(searchParams)
  homeSearch.delete('state')
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState(null)
  const [deleting, setDeleting] = useState(null)
  const [chapterCode, setChapterCode] = useState('')
  const [title, setTitle] = useState('')
  const [feedback, setFeedback] = useState(null)
  const activeQuery = useQuery({ queryKey: ['story-chapters', projectUuid, 'active'], queryFn: () => listChapters(projectUuid, 'active') })
  const trashQuery = useQuery({ queryKey: ['story-chapters', projectUuid, 'trashed'], queryFn: () => listChapters(projectUuid, 'trashed') })
  const chapters = activeQuery.data?.items || []
  const selected = showTrash ? (trashQuery.data?.items || []) : chapters
  const sectionQueries = useQueries({ queries: selected.map((chapter) => ({ queryKey: ['comic-sections', projectUuid, chapter.uuid], queryFn: () => listComicSections(projectUuid, chapter.uuid), enabled: !showTrash })) })
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['story-chapters', projectUuid] })
    void queryClient.invalidateQueries({ queryKey: ['story-project', projectUuid] })
  }
  const create = useMutation({ mutationFn: () => createChapter(projectUuid, { chapter_code: chapterCode.trim(), title: title.trim(), content: '', content_format: 'txt' }), onSuccess: () => { refresh(); setCreating(false); setTitle(''); setFeedback({ kind: 'success', message: t('simple.books.created') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const update = useMutation({ mutationFn: () => updateChapter(projectUuid, editing.uuid, { title: title.trim(), expected_revision: editing.revision }), onSuccess: () => { refresh(); setEditing(null); setFeedback({ kind: 'success', message: t('simple.feedback.saved') }) }, onError: (error) => { setFeedback({ kind: 'error', error }); void activeQuery.refetch() } })
  const trash = useMutation({ mutationFn: (chapter) => trashChapter(projectUuid, chapter.uuid, chapter.revision), onSuccess: () => { refresh(); setDeleting(null); setFeedback({ kind: 'success', message: t('simple.books.trashed') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const restore = useMutation({ mutationFn: (chapter) => restoreChapter(projectUuid, chapter.uuid, chapter.revision), onSuccess: () => { refresh(); setFeedback({ kind: 'success', message: t('simple.books.restored') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const reorder = useMutation({ mutationFn: (uuids) => reorderChapters(projectUuid, uuids), onSuccess: (data) => { queryClient.setQueryData(['story-chapters', projectUuid, 'active'], data); setFeedback({ kind: 'success', message: t('simple.books.reordered') }) }, onError: (error) => { setFeedback({ kind: 'error', error }); void activeQuery.refetch() } })
  const move = (chapter, direction) => {
    const uuids = chapters.map((item) => item.uuid)
    const index = uuids.indexOf(chapter.uuid)
    const target = index + direction
    if (index < 0 || target < 0 || target >= uuids.length) return
    ;[uuids[index], uuids[target]] = [uuids[target], uuids[index]]
    reorder.mutate(uuids)
  }
  const openCreate = () => { setChapterCode(nextChapterCode(chapters)); setTitle(''); setCreating(true); setFeedback(null) }
  const selectTrash = (trashed) => {
    const next = new URLSearchParams(searchParams)
    if (trashed) next.set('state', 'trashed')
    else next.delete('state')
    setSearchParams(next)
  }
  if ((showTrash ? trashQuery : activeQuery).isLoading) return <SimpleLoading message={t('simple.loading.books')} />
  return (
    <div className="simple-project-page simple-books-page">
      <SimplePageHeading title={t('simple.books.title')} description={t('simple.books.description_manage')} backTo={projectRoute(projectUuid, '', homeSearch)} actions={<button className="simple-button" type="button" onClick={openCreate}><Plus size={15} aria-hidden="true" />{t('simple.books.add')}</button>} />
      <div className="simple-segmented" role="group" aria-label={t('simple.books.filter')}><button type="button" aria-pressed={!showTrash} onClick={() => selectTrash(false)}>{t('simple.settings.active')}</button><button type="button" aria-pressed={showTrash} onClick={() => selectTrash(true)}>{t('simple.settings.trash')}</button></div>
      <SimpleFeedback feedback={feedback} onDismiss={() => setFeedback(null)} />
      <SimpleError error={(showTrash ? trashQuery : activeQuery).error || sectionQueries.find((query) => query.error)?.error} onRetry={() => { activeQuery.refetch(); trashQuery.refetch(); sectionQueries.forEach((query) => query.refetch()) }} />
      {selected.length ? <div className="simple-book-list">{selected.map((chapter, index) => {
        const sections = sectionQueries[index]?.data?.items || []
        const cover = firstReadySimpleImage(sections)
        const chatSearch = withChatReference(location.search, 'chapter', chapter.uuid, [chapter.chapter_code, chapter.title].filter(Boolean).join(' · '))
        return <article key={chapter.uuid}><Link className="simple-book-list__main" to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapter.uuid)}`, location.search)}><SimpleImage asset={cover} alt={chapter.title || t('projects.unnamed_picture_book')} fallbackIcon={<BookOpenText size={27} aria-hidden="true" />} /><span><small data-machine-value>{chapter.chapter_code}</small><strong data-user-content>{chapter.title || t('projects.unnamed_picture_book')}</strong><em>{t('simple.books.page_count', { count: sections.length })}</em><b>{t('simple.books.open_pages')}<ChevronRight size={14} aria-hidden="true" /></b></span></Link><footer>{showTrash ? <button type="button" disabled={restore.isPending} onClick={() => restore.mutate(chapter)}>{t('common.action.restore')}</button> : <><Link to={{ pathname: projectRoute(projectUuid, `chapters/${encodeURIComponent(chapter.uuid)}`).pathname, search: chatSearch }}><Bot size={14} aria-hidden="true" />{t('simple.setting.ask_agent')}</Link><button type="button" onClick={() => { setEditing(chapter); setTitle(chapter.title || '') }}><Pencil size={14} aria-hidden="true" />{t('common.action.edit')}</button><button type="button" aria-label={t('simple.books.move_up')} disabled={index === 0 || reorder.isPending} onClick={() => move(chapter, -1)}><ChevronUp size={14} aria-hidden="true" /></button><button type="button" aria-label={t('simple.books.move_down')} disabled={index === chapters.length - 1 || reorder.isPending} onClick={() => move(chapter, 1)}><ChevronDown size={14} aria-hidden="true" /></button><button className="simple-danger-action" type="button" onClick={() => setDeleting(chapter)}><Trash2 size={14} aria-hidden="true" />{t('simple.action.trash')}</button></>}</footer></article>
      })}</div> : <SimpleEmptyState icon={<BookOpenText size={25} aria-hidden="true" />} title={t(showTrash ? 'simple.books.trash_empty' : 'simple.books.empty_manage')} actions={!showTrash ? <button className="simple-button" type="button" onClick={openCreate}>{t('simple.books.add')}</button> : null} />}
      {creating ? <SimpleDialog title={t('simple.books.add_title')} onClose={() => !create.isPending && setCreating(false)}><form className="simple-form" onSubmit={(event) => { event.preventDefault(); create.mutate() }}><label>{t('simple.books.code')}<input autoFocus value={chapterCode} onChange={(event) => setChapterCode(event.target.value)} placeholder="vol01.ch01" /></label><label>{t('common.label.name')}<input maxLength={255} value={title} onChange={(event) => setTitle(event.target.value)} /></label><SimpleDialogActions pending={create.isPending} submitDisabled={!/^vol\d{2,}\.ch\d{2,}$/i.test(chapterCode.trim())} onCancel={() => setCreating(false)} /></form></SimpleDialog> : null}
      {editing ? <SimpleDialog title={t('simple.books.edit_title')} onClose={() => !update.isPending && setEditing(null)}><form className="simple-form" onSubmit={(event) => { event.preventDefault(); update.mutate() }}><label>{t('simple.books.code')}<input value={editing.chapter_code} disabled /></label><label>{t('common.label.name')}<input autoFocus maxLength={255} value={title} onChange={(event) => setTitle(event.target.value)} /></label><SimpleDialogActions pending={update.isPending} submitDisabled={title.trim() === (editing.title || '')} onCancel={() => setEditing(null)} /></form></SimpleDialog> : null}
      {deleting ? <SimpleConfirm danger title={t('simple.books.trash_title')} body={t('simple.books.trash_body', { title: deleting.title || deleting.chapter_code })} pending={trash.isPending} onCancel={() => setDeleting(null)} onConfirm={() => trash.mutate(deleting)} /> : null}
    </div>
  )
}

export function SimpleChapterRedirect({ projectUuid }) {
  const { chapterUuid } = useParams()
  const location = useLocation()
  return <Navigate replace to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}`, location.search)} />
}

export function SimplePagesPage({ project, projectUuid }) {
  const { t } = useI18n()
  const { chapterUuid } = useParams()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState(null)
  const [newPage, setNewPage] = useState(emptyPageDraft)
  const [feedback, setFeedback] = useState(null)
  const chaptersQuery = useQuery({ queryKey: ['story-chapters', projectUuid, 'active'], queryFn: () => listChapters(projectUuid, 'active') })
  const chapterQuery = useQuery({ queryKey: ['story-chapter', projectUuid, chapterUuid], queryFn: () => getChapter(projectUuid, chapterUuid) })
  const sectionsQuery = useQuery({ queryKey: ['comic-sections', projectUuid, chapterUuid], queryFn: () => listComicSections(projectUuid, chapterUuid) })
  const tasksQuery = useQuery({ queryKey: ['production-tasks', projectUuid], queryFn: () => listProductionTasks(projectUuid) })
  const sections = orderedSimplePages(sectionsQuery.data?.items || [])
  const multipleChapters = (chaptersQuery.data?.items || []).length > 1
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['comic-sections', projectUuid, chapterUuid] })
    void queryClient.invalidateQueries({ queryKey: ['comic-state', projectUuid, chapterUuid] })
  }
  const create = useMutation({ mutationFn: () => createComicSection(projectUuid, chapterUuid, { title: newPage.title.trim(), description_md: newPage.direction.trim(), storyboard_md: newPage.text.trim(), page_role: newPage.role }), onSuccess: (created) => { refresh(); setCreating(false); setNewPage(emptyPageDraft()); setFeedback({ kind: 'success', message: t('simple.pages.created') }); navigateToPage(created) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const remove = useMutation({ mutationFn: (section) => deleteComicSection(projectUuid, chapterUuid, section.uuid, section.revision), onSuccess: () => { refresh(); setDeleting(null); setFeedback({ kind: 'success', message: t('simple.pages.deleted') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const reorder = useMutation({ mutationFn: (uuids) => reorderComicSections(projectUuid, chapterUuid, uuids), onSuccess: (data) => { queryClient.setQueryData(['comic-sections', projectUuid, chapterUuid], data); setFeedback({ kind: 'success', message: t('simple.pages.reordered') }) }, onError: (error) => { setFeedback({ kind: 'error', error }); void sectionsQuery.refetch() } })
  const batch = useMutation({ mutationFn: () => generateChapterImagesBatch(projectUuid, chapterUuid, { section_uuids: sections.filter((section) => !section.current_image && section.current_storyboard).map((section) => section.uuid), idempotency_key: `simple-book-missing-${Date.now()}` }), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['production-tasks', projectUuid] }); setFeedback({ kind: 'success', message: t('simple.pages.batch_started') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const navigate = useNavigate()
  const navigateToPage = (section) => navigate(projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/sections/${encodeURIComponent(section.uuid)}`, location.search))
  const move = (section, direction) => {
    const uuids = comicBodyReorderUuids(sections, section.uuid, direction)
    if (uuids) reorder.mutate(uuids)
  }
  const missingCount = sections.filter((section) => !section.current_image && section.current_storyboard).length
  const activeTaskCount = (tasksQuery.data?.items || []).filter((task) => task.kind === 'comic_image_generation' && ACTIVE_TASK_STATUSES.has(task.status) && sections.some((section) => section.uuid === task.resource_uuid)).length
  if (chaptersQuery.isLoading || chapterQuery.isLoading || sectionsQuery.isLoading) return <SimpleLoading message={t('simple.loading.pages')} />
  return (
    <div className="simple-project-page simple-pages-page">
      <SimplePageHeading title={chapterQuery.data?.title || t('simple.pages.title')} eyebrow={chapterQuery.data?.chapter_code} description={t('simple.pages.description_manage')} backLabel={t(multipleChapters ? 'simple.pages.back' : 'simple.shell.page.home')} backTo={projectRoute(projectUuid, multipleChapters ? 'chapters' : '', location.search)} actions={<><Link className="simple-button simple-button--secondary" to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/preview`, location.search)}><Images size={15} aria-hidden="true" />{t('simple.pages.book_view')}</Link><button className="simple-button simple-button--secondary" type="button" disabled={!missingCount || activeTaskCount > 0 || batch.isPending} onClick={() => batch.mutate()}><Sparkles size={15} aria-hidden="true" />{t('simple.pages.batch_missing', { count: missingCount })}</button><button className="simple-button" type="button" onClick={() => { setNewPage(emptyPageDraft()); setCreating(true) }}><Plus size={15} aria-hidden="true" />{t('simple.pages.add')}</button></>} />
      <SimpleFeedback feedback={feedback} onDismiss={() => setFeedback(null)} />
      <SimpleError error={chaptersQuery.error || chapterQuery.error || sectionsQuery.error || tasksQuery.error} onRetry={() => { chaptersQuery.refetch(); chapterQuery.refetch(); sectionsQuery.refetch(); tasksQuery.refetch() }} />
      {activeTaskCount ? <p className="simple-inline-status" role="status">{t('simple.pages.generating_count', { count: activeTaskCount })}</p> : null}
      {sections.length ? <div className="simple-page-grid">{sections.map((section, index) => <SimpleManagedPageCard key={section.uuid} projectUuid={projectUuid} chapterUuid={chapterUuid} section={section} sections={sections} index={index} pending={remove.isPending || reorder.isPending} onDelete={() => setDeleting(section)} onMove={move} />)}</div> : <SimpleEmptyState icon={<Images size={25} aria-hidden="true" />} title={t('simple.pages.empty_manage')} actions={<button className="simple-button" type="button" onClick={() => setCreating(true)}>{t('simple.pages.add')}</button>} />}
      {creating ? <SimpleDialog title={t('simple.pages.add_title')} onClose={() => !create.isPending && setCreating(false)}><form className="simple-form" onSubmit={(event) => { event.preventDefault(); create.mutate() }}><label>{t('simple.page.role')}<select value={newPage.role} onChange={(event) => setNewPage((value) => ({ ...value, role: event.target.value }))}><option value="body">{t('simple.page.role_body')}</option>{project?.picture_book?.format !== 'vertical_strip' ? <><option value="front_cover" disabled={comicPageRoleOptionDisabled(sections, 'front_cover')}>{t('simple.page.role_front')}</option><option value="back_cover" disabled={comicPageRoleOptionDisabled(sections, 'back_cover')}>{t('simple.page.role_back')}</option></> : null}</select></label><label>{t('simple.page.title_label')}<input autoFocus maxLength={160} value={newPage.title} onChange={(event) => setNewPage((value) => ({ ...value, title: event.target.value }))} /></label><label>{t('simple.page.text')}<textarea value={newPage.text} onChange={(event) => setNewPage((value) => ({ ...value, text: event.target.value }))} /></label><label>{t('simple.page.visual_direction')}<textarea value={newPage.direction} onChange={(event) => setNewPage((value) => ({ ...value, direction: event.target.value }))} /></label><SimpleDialogActions pending={create.isPending} submitDisabled={false} onCancel={() => setCreating(false)} /></form></SimpleDialog> : null}
      {deleting ? <SimpleConfirm danger title={t('simple.pages.delete_title')} body={t('simple.pages.delete_body', { page: comicPageLabel(t, sections, deleting) })} pending={remove.isPending} onCancel={() => setDeleting(null)} onConfirm={() => remove.mutate(deleting)} /> : null}
    </div>
  )
}

export function SimplePageView({ project, projectUuid }) {
  const { formatDateTime, t } = useI18n()
  const { chapterUuid, sectionUuid: routeSectionUuid } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const createMenuRef = useRef(null)
  const createTriggerRef = useRef(null)
  const pageDragRef = useRef(null)
  const pageDragClickSuppressed = useRef(false)
  const chapterQuery = useQuery({ queryKey: ['story-chapter', projectUuid, chapterUuid], queryFn: () => getChapter(projectUuid, chapterUuid) })
  const sectionsQuery = useQuery({ queryKey: ['comic-sections', projectUuid, chapterUuid], queryFn: () => listComicSections(projectUuid, chapterUuid) })
  const assetsQuery = useQuery({ queryKey: ['premise-assets', projectUuid, '', false], queryFn: () => listPremiseAssets(projectUuid, { state: 'active' }) })
  const sections = orderedSimplePages(sectionsQuery.data?.items || [])
  const sectionUuid = routeSectionUuid || sections[0]?.uuid || ''
  const storyboardsQuery = useQuery({ queryKey: ['comic-storyboards', projectUuid, chapterUuid, sectionUuid], queryFn: () => listStoryboards(projectUuid, chapterUuid, sectionUuid), enabled: Boolean(sectionUuid) })
  const imagesQuery = useQuery({ queryKey: ['comic-images', projectUuid, chapterUuid, sectionUuid], queryFn: () => listImageVariants(projectUuid, chapterUuid, sectionUuid), enabled: Boolean(sectionUuid) })
  const tasksQuery = useQuery({ queryKey: ['production-tasks', projectUuid], queryFn: () => listProductionTasks(projectUuid) })
  const index = sections.findIndex((item) => item.uuid === sectionUuid)
  const section = sections[index]
  const [title, setTitle] = useState('')
  const [text, setText] = useState('')
  const [direction, setDirection] = useState('')
  const [role, setRole] = useState('body')
  const [selectedRefs, setSelectedRefs] = useState([])
  const [imageFile, setImageFile] = useState(null)
  const [feedback, setFeedback] = useState(null)
  const [createMenuOpen, setCreateMenuOpen] = useState(false)
  const [imageCandidatesOpen, setImageCandidatesOpen] = useState(false)
  const [pageDrag, setPageDrag] = useState(null)
  const completedTask = useRef('')
  const task = (tasksQuery.data?.items || []).find((item) => item.kind === 'comic_image_generation' && item.resource_uuid === sectionUuid)
  const taskActive = Boolean(task && ACTIVE_TASK_STATUSES.has(task.status))
  const dirty = Boolean(section) && (title.trim() !== (section.title || '') || text.trim() !== (section.current_storyboard?.content_md || '') || direction.trim() !== (section.description_md || '') || role !== comicPageRole(section))
  const refsDirty = Boolean(section) && JSON.stringify(selectedRefs) !== JSON.stringify((section.premise_assets || []).map((item) => item.asset_uuid))
  const invalidEmptyText = Boolean(section?.current_storyboard?.content_md) && !text.trim()
  const createRoles = project?.picture_book?.format === 'vertical_strip' ? ['body'] : COMIC_PAGE_ROLES

  useEffect(() => {
    if (!section) return
    setTitle(section.title || '')
    setText(section.current_storyboard?.content_md || '')
    setDirection(section.description_md || '')
    setRole(comicPageRole(section))
    setSelectedRefs((section.premise_assets || []).map((item) => item.asset_uuid))
  }, [section?.revision, section?.current_storyboard?.uuid])

  useEffect(() => {
    if (task?.status !== 'completed' || completedTask.current === task.uuid) return
    completedTask.current = task.uuid
    void queryClient.invalidateQueries({ queryKey: ['comic-sections', projectUuid, chapterUuid] })
    void queryClient.invalidateQueries({ queryKey: ['comic-images', projectUuid, chapterUuid, sectionUuid] })
    setFeedback({ kind: 'success', message: t('simple.page.generation_complete') })
  }, [chapterUuid, projectUuid, queryClient, sectionUuid, task?.status, task?.uuid, t])

  useEffect(() => {
    if (!createMenuOpen) return undefined
    const focusFrame = window.requestAnimationFrame(() => {
      const items = [...(createMenuRef.current?.querySelectorAll('[role="menuitem"]:not(:disabled)') || [])]
      items[0]?.focus()
    })
    const closeAndReturnFocus = () => {
      setCreateMenuOpen(false)
      createTriggerRef.current?.focus()
    }
    const handlePointerDown = (event) => {
      if (!createMenuRef.current?.contains(event.target)) setCreateMenuOpen(false)
    }
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        closeAndReturnFocus()
        return
      }
      if (event.key === 'Tab') {
        setCreateMenuOpen(false)
        return
      }
      if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) || !createMenuRef.current?.contains(document.activeElement)) return
      const items = [...(createMenuRef.current.querySelectorAll('[role="menuitem"]:not(:disabled)') || [])]
      if (!items.length) return
      event.preventDefault()
      const currentIndex = items.indexOf(document.activeElement)
      if (event.key === 'Home') items[0].focus()
      else if (event.key === 'End') items.at(-1).focus()
      else if (event.key === 'ArrowDown') items[(currentIndex + 1 + items.length) % items.length].focus()
      else items[(currentIndex - 1 + items.length) % items.length].focus()
    }
    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      window.cancelAnimationFrame(focusFrame)
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [createMenuOpen])

  const updateSectionCache = (updated) => {
    queryClient.setQueryData(['comic-sections', projectUuid, chapterUuid], (current) => ({ ...current, items: (current?.items || []).map((item) => item.uuid === updated.uuid ? updated : item) }))
  }
  const refreshPage = () => {
    void queryClient.invalidateQueries({ queryKey: ['comic-sections', projectUuid, chapterUuid] })
    void queryClient.invalidateQueries({ queryKey: ['comic-storyboards', projectUuid, chapterUuid, sectionUuid] })
    void queryClient.invalidateQueries({ queryKey: ['comic-images', projectUuid, chapterUuid, sectionUuid] })
  }
  const save = useMutation({
    mutationFn: async () => {
      let current = section
      const metadataChanged = title.trim() !== section.title || direction.trim() !== (section.description_md || '') || role !== comicPageRole(section)
      if (metadataChanged) current = await updateComicSection(projectUuid, chapterUuid, sectionUuid, { title: title.trim(), description_md: direction.trim(), page_role: role, expected_revision: current.revision })
      if (text.trim() !== (section.current_storyboard?.content_md || '')) current = await createStoryboard(projectUuid, chapterUuid, sectionUuid, { content_md: text.trim(), source_type: 'manual', expected_revision: current.revision })
      const persistedReferences = (section.premise_assets || []).map((item) => item.asset_uuid)
      if (JSON.stringify(selectedRefs) !== JSON.stringify(persistedReferences)) current = await setComicSectionPremiseAssets(projectUuid, chapterUuid, sectionUuid, selectedRefs, current.revision)
      return current
    },
    onSuccess: (updated) => { updateSectionCache(updated); refreshPage(); setFeedback({ kind: 'success', message: t('simple.feedback.saved') }) },
    onError: (error) => { setFeedback({ kind: 'error', error }); void sectionsQuery.refetch() },
  })
  const generate = useMutation({ mutationFn: () => generateSectionImage(projectUuid, chapterUuid, sectionUuid, { prompt: '', premise_asset_uuids: (section.premise_assets || []).map((item) => item.asset_uuid), idempotency_key: `simple-page-image-${Date.now()}` }), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['production-tasks', projectUuid] }); setFeedback({ kind: 'success', message: t('simple.page.generation_started') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const importImage = useMutation({ mutationFn: async () => { const upload = await createAssetUpload(projectUuid, { purpose: 'comic_section_image', displayName: section.title || sectionUuid, file: imageFile }); return importSectionImage(projectUuid, chapterUuid, sectionUuid, { upload_uuid: upload.uuid, expected_revision: section.revision }) }, onSuccess: (updated) => { setImageFile(null); updateSectionCache(updated); refreshPage(); setFeedback({ kind: 'success', message: t('simple.page.image_imported') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  const chooseImage = useMutation({ mutationFn: (variant) => selectImageVariant(projectUuid, chapterUuid, sectionUuid, variant.uuid, section.revision), onSuccess: (updated) => { setImageCandidatesOpen(false); updateSectionCache(updated); refreshPage(); setFeedback({ kind: 'success', message: t('simple.page.image_selected') }) }, onError: (error) => { setFeedback({ kind: 'error', error }); void sectionsQuery.refetch() } })
  const chooseStoryboard = useMutation({ mutationFn: (variant) => selectStoryboard(projectUuid, chapterUuid, sectionUuid, variant.uuid, section.revision), onSuccess: (updated) => { updateSectionCache(updated); refreshPage(); setFeedback({ kind: 'success', message: t('simple.page.text_restored') }) }, onError: (error) => { setFeedback({ kind: 'error', error }); void sectionsQuery.refetch() } })
  const createPage = useMutation({
    mutationFn: async (pageRole) => {
      if (dirty || refsDirty) await save.mutateAsync()
      return createComicSection(projectUuid, chapterUuid, { title: '', description_md: '', storyboard_md: '', page_role: pageRole })
    },
    onSuccess: (created) => {
      queryClient.setQueryData(['comic-sections', projectUuid, chapterUuid], (current) => ({ ...current, items: [...(current?.items || []), created] }))
      void queryClient.invalidateQueries({ queryKey: ['comic-sections', projectUuid, chapterUuid] })
      void queryClient.invalidateQueries({ queryKey: ['comic-state', projectUuid, chapterUuid] })
      setCreateMenuOpen(false)
      setFeedback({ kind: 'success', message: t('simple.pages.created') })
      navigate(projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/sections/${encodeURIComponent(created.uuid)}`, location.search))
    },
    onError: (error) => { setCreateMenuOpen(false); setFeedback({ kind: 'error', error }) },
  })
  const reorderPages = useMutation({
    mutationFn: (uuids) => reorderComicSections(projectUuid, chapterUuid, uuids),
    onSuccess: (data) => {
      queryClient.setQueryData(['comic-sections', projectUuid, chapterUuid], data)
      void queryClient.invalidateQueries({ queryKey: ['comic-state', projectUuid, chapterUuid] })
      setFeedback({ kind: 'success', message: t('simple.pages.reordered') })
    },
    onError: (error) => {
      setFeedback({ kind: 'error', error })
      void sectionsQuery.refetch()
    },
  })

  const pageDropIntentForPointer = (list, clientX, clientY, draggingUuid) => {
    if (!list) return null
    const listRect = list.getBoundingClientRect()
    if (clientX < listRect.left - 24 || clientX > listRect.right + 24 || clientY < listRect.top - 24 || clientY > listRect.bottom + 24) return null
    const horizontal = window.getComputedStyle(list).display === 'flex'
    const targets = [...list.querySelectorAll('[data-reorderable="true"]')]
      .filter((element) => element.dataset.pageUuid !== draggingUuid)
      .map((element) => {
        const rect = element.getBoundingClientRect()
        return { uuid: element.dataset.pageUuid, left: rect.left, top: rect.top, width: rect.width, height: rect.height }
      })
      .filter((item) => item.uuid && item.width > 0 && item.height > 0)
    if (!targets.length) return null
    const beforeTarget = targets.find((item) => horizontal ? clientX < item.left + item.width / 2 : clientY < item.top + item.height / 2)
    return beforeTarget ? { targetUuid: beforeTarget.uuid, placement: 'before' } : { targetUuid: targets.at(-1).uuid, placement: 'after' }
  }
  const clearPageDrag = () => {
    pageDragRef.current = null
    setPageDrag(null)
  }
  const startPageDrag = (event, item) => {
    if ((event.pointerType === 'mouse' && event.button !== 0) || comicPageRole(item) !== 'body' || reorderPages.isPending) return
    const list = event.currentTarget.closest('.simple-page-rail__list')
    pageDragRef.current = {
      sectionUuid: item.uuid,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      active: false,
      list,
      targetUuid: '',
      placement: '',
    }
    list?.setPointerCapture?.(event.pointerId)
  }
  const updatePageDrag = (event) => {
    const drag = pageDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    if (!drag.active && Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) < 5) return
    event.preventDefault()
    event.stopPropagation()
    if (!drag.active) {
      drag.active = true
      setPageDrag({ sectionUuid: drag.sectionUuid, targetUuid: '', placement: '' })
    }
    const listRect = drag.list?.getBoundingClientRect()
    if (drag.list && listRect) {
      if (window.getComputedStyle(drag.list).display === 'flex') {
        if (event.clientX < listRect.left + 36) drag.list.scrollLeft -= 18
        else if (event.clientX > listRect.right - 36) drag.list.scrollLeft += 18
      } else if (event.clientY < listRect.top + 36) drag.list.scrollTop -= 18
      else if (event.clientY > listRect.bottom - 36) drag.list.scrollTop += 18
    }
    const intent = pageDropIntentForPointer(drag.list, event.clientX, event.clientY, drag.sectionUuid)
    drag.targetUuid = intent?.targetUuid || ''
    drag.placement = intent?.placement || ''
    setPageDrag({ sectionUuid: drag.sectionUuid, targetUuid: drag.targetUuid, placement: drag.placement })
  }
  const finishPageDrag = (event) => {
    const drag = pageDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    if (!drag.active) {
      clearPageDrag()
      return
    }
    event.preventDefault()
    event.stopPropagation()
    pageDragClickSuppressed.current = true
    window.setTimeout(() => { pageDragClickSuppressed.current = false }, 0)
    const intent = pageDropIntentForPointer(drag.list, event.clientX, event.clientY, drag.sectionUuid)
    const targetUuid = intent?.targetUuid || drag.targetUuid
    const placement = intent?.placement || drag.placement
    clearPageDrag()
    const uuids = reorderedComicBodyUuids(sections, drag.sectionUuid, targetUuid, placement)
    if (uuids && !reorderPages.isPending) reorderPages.mutate(uuids)
  }
  const cancelPageDrag = (event) => {
    const drag = pageDragRef.current
    if (drag && drag.pointerId !== event.pointerId) return
    clearPageDrag()
  }

  if (chapterQuery.isLoading || sectionsQuery.isLoading) return <SimpleLoading message={t('simple.loading.pages')} />
  if (!section) return routeSectionUuid ? <SimpleNotFound projectUuid={projectUuid} /> : <SimplePagesPage project={project} projectUuid={projectUuid} />
  const label = comicPageLabel(t, sections, section)
  const chatSearch = withChatReference(location.search, 'comic_section', section.uuid, section.title || label)
  const previous = sections[index - 1]
  const next = sections[index + 1]
  const imageVariants = imagesQuery.data?.items || []
  return (
    <div className="simple-project-page simple-page-view">
      <div className="simple-page-view__toolbar"><Link to={projectRoute(projectUuid, '', location.search)}><ArrowLeft size={15} aria-hidden="true" />{t('simple.shell.page.home')}</Link><div><Link className="simple-button simple-button--secondary" to={{ pathname: location.pathname, search: chatSearch }}><Bot size={15} aria-hidden="true" />{t('simple.setting.ask_agent')}</Link><button className="simple-button" type="button" disabled={(!dirty && !refsDirty) || save.isPending || invalidEmptyText} onClick={() => save.mutate()}><Save size={15} aria-hidden="true" />{t(save.isPending ? 'common.status.saving' : 'common.action.save')}</button></div></div>
      <SimpleFeedback feedback={feedback} onDismiss={() => setFeedback(null)} />
      <SimpleError error={chapterQuery.error || sectionsQuery.error || assetsQuery.error || storyboardsQuery.error || imagesQuery.error || tasksQuery.error} onRetry={() => { chapterQuery.refetch(); sectionsQuery.refetch(); assetsQuery.refetch(); storyboardsQuery.refetch(); imagesQuery.refetch(); tasksQuery.refetch() }} />
      <div className="simple-page-editor-layout">
        <aside className="simple-page-rail" aria-label={t('simple.pages.title')}>
          <header className="simple-page-rail__header">
            <h2>{t('simple.shell.page.pages')}</h2>
            <div className="simple-page-rail__create" ref={createMenuRef}>
              <button
                ref={createTriggerRef}
                type="button"
                aria-label={t('simple.pages.add')}
                title={t('simple.pages.add')}
                aria-haspopup="menu"
                aria-expanded={createMenuOpen}
                disabled={createPage.isPending || save.isPending}
                onClick={() => setCreateMenuOpen((current) => !current)}
                onKeyDown={(event) => {
                  if (!['ArrowDown', 'ArrowUp'].includes(event.key)) return
                  event.preventDefault()
                  setCreateMenuOpen(true)
                }}
              >
                <Plus size={18} strokeWidth={1.7} aria-hidden="true" />
              </button>
              {createMenuOpen ? (
                <div className="simple-page-rail__menu" role="menu" aria-label={t('comic.page_role.create_label')}>
                  {createRoles.map((pageRole) => (
                    <button
                      key={pageRole}
                      type="button"
                      role="menuitem"
                      disabled={invalidEmptyText || createPage.isPending || save.isPending || comicPageRoleOptionDisabled(sections, pageRole)}
                      onClick={() => createPage.mutate(pageRole)}
                    >
                      {simplePageRoleLabel(t, pageRole)}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
          </header>
          <div className="simple-page-rail__list" aria-busy={reorderPages.isPending} onPointerMove={updatePageDrag} onPointerUp={finishPageDrag} onPointerCancel={cancelPageDrag}>{sections.map((item) => {
            const itemLabel = comicPageLabel(t, sections, item)
            const reorderable = comicPageRole(item) === 'body'
            const itemClasses = [
              'simple-page-rail__item',
              reorderable ? 'is-reorderable' : 'is-fixed',
              pageDrag?.sectionUuid === item.uuid ? 'is-dragging' : '',
              pageDrag?.targetUuid === item.uuid && pageDrag.placement === 'before' ? 'is-drop-before' : '',
              pageDrag?.targetUuid === item.uuid && pageDrag.placement === 'after' ? 'is-drop-after' : '',
            ].filter(Boolean).join(' ')
            return (
              <div
                className={itemClasses}
                key={item.uuid}
                data-page-uuid={item.uuid}
                data-reorderable={reorderable}
                draggable={false}
                title={reorderable ? t('comic.workbench.pages.drag_role', { label: itemLabel }) : t('comic.workbench.pages.fixed_title')}
                onPointerDown={(event) => startPageDrag(event, item)}
              >
                <Link draggable={false} className={item.uuid === section.uuid ? 'is-active' : ''} aria-current={item.uuid === section.uuid ? 'page' : undefined} to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/sections/${encodeURIComponent(item.uuid)}`, location.search)} onClick={(event) => { if (pageDragClickSuppressed.current) { event.preventDefault(); event.stopPropagation(); pageDragClickSuppressed.current = false } }}><SimpleImage asset={item.current_image?.asset} alt="" fallbackText={itemLabel} /><span>{itemLabel}</span></Link>
                {reorderable ? <span className="simple-page-rail__drag-handle" aria-hidden="true"><GripVertical size={15} strokeWidth={1.8} /></span> : null}
              </div>
            )
          })}</div>
        </aside>
        <main className="simple-page-editor">
          <header><div><span data-user-content>{chapterQuery.data?.title}</span><h1>{label}</h1></div><small role="status">{dirty || refsDirty ? t('simple.page.unsaved') : t('common.status.saved')}</small></header>
          <section className="simple-page-meta-form"><label>{t('simple.page.role')}<select value={role} onChange={(event) => setRole(event.target.value)}><option value="body">{t('simple.page.role_body')}</option>{project?.picture_book?.format !== 'vertical_strip' ? <><option value="front_cover" disabled={comicPageRoleOptionDisabled(sections, 'front_cover', section.uuid)}>{t('simple.page.role_front')}</option><option value="back_cover" disabled={comicPageRoleOptionDisabled(sections, 'back_cover', section.uuid)}>{t('simple.page.role_back')}</option></> : null}</select></label><label>{t('simple.page.title_label')}<input maxLength={160} value={title} onChange={(event) => setTitle(event.target.value)} /></label></section>
          <div className="simple-page-current-image"><SimpleImage asset={section.current_image?.asset} alt={section.title || label} fallbackText={t('simple.pages.no_image')} />{section.current_image ? <span>{t('simple.page.current_image', { version: section.current_image.version_no })}</span> : null}</div>
          <div className="simple-illustration-actions">
            <button type="button" className="simple-illustration-actions__drafts" aria-haspopup="dialog" aria-expanded={imageCandidatesOpen} onClick={() => setImageCandidatesOpen(true)}><Images size={16} strokeWidth={1.6} aria-hidden="true" /><span>{t('simple.page.image_drafts_count', { count: imageVariants.length })}</span></button>
            {taskActive ? <SimpleTaskStatus task={task} /> : null}
            <button className="simple-button simple-illustration-actions__generate" type="button" disabled={!section.current_storyboard || taskActive || generate.isPending || refsDirty} onClick={() => generate.mutate()}><Sparkles size={16} strokeWidth={1.6} aria-hidden="true" />{t(section.current_image ? 'simple.setting.generate_title' : 'simple.page.generate')}</button>
          </div>
          <section className="simple-page-content-card"><h2>{t('simple.page.content')}</h2><label><span>{t('simple.page.text')}</span><textarea value={text} onChange={(event) => setText(event.target.value)} /></label><label><span>{t('simple.page.visual_direction')}</span><textarea value={direction} onChange={(event) => setDirection(event.target.value)} /></label></section>
          <section className="simple-page-references"><header><div><h2>{t('simple.page.references')}</h2><p>{t('simple.page.references_body')}</p></div><button type="button" disabled={!refsDirty || save.isPending || invalidEmptyText} onClick={() => save.mutate()}>{t(save.isPending ? 'common.status.saving' : 'common.action.save')}</button></header><div>{(assetsQuery.data?.items || []).filter((asset) => asset.current_variant).map((asset) => <label className={selectedRefs.includes(asset.uuid) ? 'is-selected' : ''} key={asset.uuid}><input type="checkbox" checked={selectedRefs.includes(asset.uuid)} disabled={!selectedRefs.includes(asset.uuid) && selectedRefs.length >= 12} onChange={(event) => setSelectedRefs((current) => event.target.checked ? [...current, asset.uuid] : current.filter((uuid) => uuid !== asset.uuid))} /><SimpleImage asset={asset.current_variant?.asset} alt="" /><span><strong data-user-content>{asset.title}</strong><small>{assetTypeLabel(t, asset.asset_type)}</small></span></label>)}</div></section>
          <section className="simple-version-list"><header><div><h2>{t('simple.page.text_versions')}</h2></div><span>{storyboardsQuery.data?.items?.length || 0}</span></header><div>{(storyboardsQuery.data?.items || []).map((variant) => <article className={variant.uuid === section.current_storyboard?.uuid ? 'is-current' : ''} key={variant.uuid}><div><strong>v{variant.version_no}</strong><span data-user-content>{simpleStoryExcerpt(variant.content_md, 90)}</span><time dateTime={variant.created_at}>{formatDateTime(variant.created_at)}</time></div><button type="button" disabled={variant.uuid === section.current_storyboard?.uuid || chooseStoryboard.isPending} onClick={() => chooseStoryboard.mutate(variant)}>{variant.uuid === section.current_storyboard?.uuid ? t('simple.version.current') : t('common.action.restore')}</button></article>)}</div></section>
          <nav className="simple-page-stepper" aria-label={t('simple.shell.page.pages')}>{previous ? <Link to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/sections/${encodeURIComponent(previous.uuid)}`, location.search)}><ChevronLeft size={15} aria-hidden="true" />{t('simple.page.previous')}</Link> : <span />}{next ? <Link to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/sections/${encodeURIComponent(next.uuid)}`, location.search)}>{t('simple.page.next')}<ChevronRight size={15} aria-hidden="true" /></Link> : null}</nav>
        </main>
      </div>
      {imageCandidatesOpen ? <SimpleDialog title={t('simple.page.image_candidates')} onClose={() => !importImage.isPending && !chooseImage.isPending && setImageCandidatesOpen(false)}><div className="simple-page-candidates-dialog"><header><p>{t('simple.page.image_candidates_body')}</p><label className="simple-button simple-button--secondary"><Upload size={15} aria-hidden="true" />{t('simple.page.import_image')}<input type="file" accept="image/*" onChange={(event) => setImageFile(event.target.files?.[0] || null)} /></label></header>{imageFile ? <div className="simple-upload-confirm"><span data-user-content>{imageFile.name}</span><button type="button" disabled={importImage.isPending} onClick={() => importImage.mutate()}>{t('simple.setting.import_now')}</button><button type="button" className="simple-button--secondary" disabled={importImage.isPending} onClick={() => setImageFile(null)}>{t('common.action.cancel')}</button></div> : null}<div className="simple-candidate-grid">{imageVariants.map((variant) => <article className={variant.uuid === section.current_image?.uuid ? 'is-current' : ''} key={variant.uuid}><SimpleImage asset={variant.asset} alt={`${label} v${variant.version_no}`} /><footer><div><strong>v{variant.version_no}</strong><time dateTime={variant.created_at}>{formatDateTime(variant.created_at)}</time></div><button type="button" disabled={variant.uuid === section.current_image?.uuid || chooseImage.isPending || importImage.isPending} onClick={() => chooseImage.mutate(variant)}>{variant.uuid === section.current_image?.uuid ? t('simple.version.current') : t('simple.version.use')}</button></footer></article>)}</div></div></SimpleDialog> : null}
    </div>
  )
}

export function SimpleBookView({ projectUuid }) {
  const { t } = useI18n()
  const { chapterUuid } = useParams()
  const location = useLocation()
  const queryClient = useQueryClient()
  const readerRef = useRef(null)
  const [fullscreen, setFullscreen] = useState(false)
  const [feedback, setFeedback] = useState(null)
  const chapterQuery = useQuery({ queryKey: ['story-chapter', projectUuid, chapterUuid], queryFn: () => getChapter(projectUuid, chapterUuid) })
  const sectionsQuery = useQuery({ queryKey: ['comic-sections', projectUuid, chapterUuid], queryFn: () => listComicSections(projectUuid, chapterUuid) })
  const tasksQuery = useQuery({ queryKey: ['production-tasks', projectUuid], queryFn: () => listProductionTasks(projectUuid) })
  const sections = orderedSimplePages(sectionsQuery.data?.items || [])
  const missing = sections.filter((section) => !section.current_image && section.current_storyboard)
  const activeTasks = (tasksQuery.data?.items || []).filter((task) => task.kind === 'comic_image_generation' && ACTIVE_TASK_STATUSES.has(task.status) && sections.some((section) => section.uuid === task.resource_uuid))
  const batch = useMutation({ mutationFn: () => generateChapterImagesBatch(projectUuid, chapterUuid, { section_uuids: missing.map((section) => section.uuid), idempotency_key: `simple-book-view-missing-${Date.now()}` }), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['production-tasks', projectUuid] }); setFeedback({ kind: 'success', message: t('simple.pages.batch_started') }) }, onError: (error) => setFeedback({ kind: 'error', error }) })
  useEffect(() => {
    const handle = () => setFullscreen(document.fullscreenElement === readerRef.current)
    document.addEventListener('fullscreenchange', handle)
    return () => document.removeEventListener('fullscreenchange', handle)
  }, [])
  const toggleFullscreen = async () => {
    if (fullscreen && !document.fullscreenElement) {
      setFullscreen(false)
      return
    }
    try {
      if (document.fullscreenElement) await document.exitFullscreen()
      else await readerRef.current?.requestFullscreen()
    } catch {
      // Browsers embedded in desktop shells can deny the Fullscreen API even
      // after a user gesture. Keep the reading experience available with the
      // same viewport-filling layout instead of surfacing a dead-end error.
      setFullscreen(true)
    }
  }
  if (chapterQuery.isLoading || sectionsQuery.isLoading) return <SimpleLoading message={t('simple.loading.pages')} />
  return (
    <div className="simple-project-page simple-book-view">
      <SimplePageHeading title={t('simple.book.title')} eyebrow={chapterQuery.data?.title} description={t('simple.book.description_manage')} backLabel={t('simple.book.back')} backTo={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}`, location.search)} actions={<><button className="simple-button simple-button--secondary" type="button" disabled={!missing.length || activeTasks.length > 0 || batch.isPending} onClick={() => batch.mutate()}><Sparkles size={15} aria-hidden="true" />{t('simple.pages.batch_missing', { count: missing.length })}</button><button className="simple-button" type="button" onClick={toggleFullscreen}>{fullscreen ? <Minimize2 size={15} aria-hidden="true" /> : <Maximize2 size={15} aria-hidden="true" />}{t(fullscreen ? 'simple.book.exit_fullscreen' : 'simple.book.fullscreen')}</button></>} />
      <SimpleFeedback feedback={feedback} onDismiss={() => setFeedback(null)} />
      <SimpleError error={chapterQuery.error || sectionsQuery.error || tasksQuery.error} onRetry={() => { chapterQuery.refetch(); sectionsQuery.refetch(); tasksQuery.refetch() }} />
      {activeTasks.length ? <p className="simple-inline-status" role="status">{t('simple.pages.generating_count', { count: activeTasks.length })}</p> : null}
      {sections.length ? <div className={`simple-book-reader${fullscreen ? ' is-fullscreen' : ''}`} ref={readerRef} aria-label={t('simple.book.reader')}><header><strong data-user-content>{chapterQuery.data?.title}</strong>{fullscreen ? <button type="button" onClick={toggleFullscreen}><Minimize2 size={18} aria-hidden="true" />{t('simple.book.exit_fullscreen')}</button> : null}</header><div className="simple-book-grid">{sections.map((section) => { const label = comicPageLabel(t, sections, section); const chatSearch = withChatReference(location.search, 'comic_section', section.uuid, section.title || label); return <article key={section.uuid}><Link to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/sections/${encodeURIComponent(section.uuid)}`, location.search)} aria-label={t('simple.book.open_page', { page: label })}><SimpleImage asset={section.current_image?.asset} alt={section.title || label} fallbackText={t('simple.pages.no_image')} /><span>{label}</span></Link><Link className="simple-book-grid__agent" to={{ pathname: projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/sections/${encodeURIComponent(section.uuid)}`).pathname, search: chatSearch }}><Bot size={13} aria-hidden="true" />{t('simple.setting.ask_agent')}</Link></article> })}</div></div> : <SimpleEmptyState icon={<Images size={25} aria-hidden="true" />} title={t('simple.pages.empty_manage')} />}
    </div>
  )
}

function SimpleManagedPageCard({ projectUuid, chapterUuid, section, sections, index, pending, onDelete, onMove }) {
  const { t } = useI18n()
  const location = useLocation()
  const label = comicPageLabel(t, sections, section)
  const role = comicPageRole(section)
  const bodyIndex = sections.filter((item) => comicPageRole(item) === 'body').findIndex((item) => item.uuid === section.uuid)
  const bodyCount = sections.filter((item) => comicPageRole(item) === 'body').length
  return (
    <article className="simple-page-card-wrap">
      <Link className="simple-page-card" to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid)}/sections/${encodeURIComponent(section.uuid)}`, location.search)} aria-label={label}><div><SimpleImage asset={section.current_image?.asset} alt={section.title || label} fallbackText={t('simple.pages.no_image')} /><span>{label}</span></div><strong data-user-content>{section.title || comicPageFallbackTitle(t, section)}</strong><small>{t(section.current_image ? 'simple.pages.has_image' : 'simple.pages.no_image')}</small></Link>
      <footer><button type="button" aria-label={t('simple.pages.move_up')} disabled={pending || role !== 'body' || bodyIndex <= 0} onClick={() => onMove(section, -1)}><ChevronUp size={14} aria-hidden="true" /></button><button type="button" aria-label={t('simple.pages.move_down')} disabled={pending || role !== 'body' || bodyIndex < 0 || bodyIndex >= bodyCount - 1} onClick={() => onMove(section, 1)}><ChevronDown size={14} aria-hidden="true" /></button><button className="simple-danger-action" type="button" aria-label={t('simple.pages.delete_page', { page: label })} disabled={pending || (role === 'body' && bodyCount <= 1)} onClick={onDelete}><Trash2 size={14} aria-hidden="true" /></button><span>{index + 1}</span></footer>
    </article>
  )
}

function SimpleSettingCard({ asset, projectUuid }) {
  const { t } = useI18n()
  const location = useLocation()
  return <Link className="simple-setting-card" to={projectRoute(projectUuid, `premise/assets/${encodeURIComponent(asset.uuid)}`, location.search)}><SimpleImage asset={asset.current_variant?.asset} alt={asset.title} fallbackIcon={<Shapes size={24} aria-hidden="true" />} /><span><strong data-user-content>{asset.title}</strong><small>{assetTypeLabel(t, asset.asset_type)} · {t('simple.setting.version', { version: asset.current_variant?.version_no || '—' })}</small></span></Link>
}

function SimpleBookPreviewGrid({ projectUuid, chapters, sectionQueries }) {
  const { t } = useI18n()
  const location = useLocation()
  return <div className="simple-book-list simple-book-list--preview">{chapters.slice(0, 3).map((chapter, index) => { const sections = sectionQueries[index]?.data?.items || []; const cover = firstReadySimpleImage(sections); return <Link key={chapter.uuid} to={projectRoute(projectUuid, `chapters/${encodeURIComponent(chapter.uuid)}`, location.search)}><SimpleImage asset={cover} alt={chapter.title || t('projects.unnamed_picture_book')} fallbackIcon={<BookOpenText size={27} aria-hidden="true" />} /><span><small data-machine-value>{chapter.chapter_code}</small><strong data-user-content>{chapter.title || t('projects.unnamed_picture_book')}</strong><em>{t('simple.books.page_count', { count: sections.length })}</em><b>{t('simple.books.open_pages')}<ChevronRight size={14} aria-hidden="true" /></b></span></Link> })}</div>
}

function SimpleTaskStatus({ task }) {
  const { t } = useI18n()
  return <span className={`simple-task-status is-${task.status}`} role="status">{t('simple.task.status', { status: task.status, progress: task.progress || 0 })}</span>
}

function SimpleFeedback({ feedback, onDismiss }) {
  const { t } = useI18n()
  if (!feedback) return null
  if (feedback.kind === 'success') return <div className="simple-feedback is-success" role="status"><Check size={16} aria-hidden="true" /><span>{feedback.message}</span>{onDismiss ? <button type="button" aria-label={t('common.action.close')} onClick={onDismiss}><X size={14} aria-hidden="true" /></button> : null}</div>
  return <div className="simple-feedback is-error" role="alert"><div><strong>{feedback.error?.message || t('simple.error.title')}</strong>{feedback.error?.details ? <p>{feedback.error.details}</p> : null}{feedback.error?.code ? <code data-machine-value>{feedback.error.code}</code> : null}</div>{onDismiss ? <button type="button" aria-label={t('common.action.close')} onClick={onDismiss}><X size={14} aria-hidden="true" /></button> : null}</div>
}

function SimpleDialog({ title, wide = false, onClose, children }) {
  const { t } = useI18n()
  const titleId = useId()
  const dialogRef = useRef(null)
  const closeRef = useRef(onClose)
  closeRef.current = onClose
  useEffect(() => {
    const previousFocus = document.activeElement
    const frame = window.requestAnimationFrame(() => {
      const focusable = dialogRef.current?.querySelector('input:not([disabled]), select:not([disabled]), textarea:not([disabled]), button:not([disabled]), a[href]')
      ;(focusable || dialogRef.current)?.focus()
    })
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        closeRef.current()
        return
      }
      if (event.key !== 'Tab') return
      const focusable = [...(dialogRef.current?.querySelectorAll('input:not([disabled]), select:not([disabled]), textarea:not([disabled]), button:not([disabled]), a[href]') || [])]
      if (!focusable.length) {
        event.preventDefault()
        dialogRef.current?.focus()
        return
      }
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.cancelAnimationFrame(frame)
      window.removeEventListener('keydown', handleKeyDown)
      previousFocus?.focus?.()
    }
  }, [])
  return <div className="simple-dialog" role="dialog" aria-modal="true" aria-labelledby={titleId}><button className="simple-dialog__backdrop" type="button" tabIndex={-1} aria-label={t('common.action.close')} onClick={onClose} /><section ref={dialogRef} tabIndex={-1} className={wide ? 'is-wide' : ''}><header><h2 id={titleId}>{title}</h2><button type="button" aria-label={t('common.action.close')} onClick={onClose}><X size={18} aria-hidden="true" /></button></header>{children}</section></div>
}

function SimpleConfirm({ title, body, pending, danger = false, onCancel, onConfirm }) {
  const { t } = useI18n()
  return <SimpleDialog title={title} onClose={() => !pending && onCancel()}><div className="simple-confirm"><p data-user-content>{body}</p><div><button className="simple-button simple-button--secondary" type="button" disabled={pending} onClick={onCancel}>{t('common.action.cancel')}</button><button className={danger ? 'simple-button simple-button--danger' : 'simple-button'} type="button" disabled={pending} onClick={onConfirm}>{t(pending ? 'common.status.saving' : 'common.action.confirm')}</button></div></div></SimpleDialog>
}

function SimpleDialogActions({ pending, submitDisabled, onCancel }) {
  const { t } = useI18n()
  return <div className="simple-form__actions"><button className="simple-button simple-button--secondary" type="button" disabled={pending} onClick={onCancel}>{t('common.action.cancel')}</button><button className="simple-button" type="submit" disabled={pending || submitDisabled}>{t(pending ? 'common.status.saving' : 'common.action.save')}</button></div>
}

export function SimplePageHeading({ title, eyebrow, description, backLabel, backTo, actions }) {
  const { t } = useI18n()
  return <header className="simple-page-heading"><div>{backTo ? <Link to={backTo}><ArrowLeft size={15} aria-hidden="true" />{backLabel || t('simple.shell.page.home')}</Link> : null}<span>{eyebrow}</span><h1 data-user-content>{title}</h1>{description ? <p>{description}</p> : null}</div>{actions ? <div className="simple-page-heading__actions">{actions}</div> : null}</header>
}

export function SimpleImage({ asset, alt, fallbackIcon, fallbackText }) {
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [asset?.uuid, asset?.content_url])
  if (!asset || asset.status !== 'ready' || !asset.content_url || failed) return <div className="simple-image-placeholder" role="img" aria-label={alt || fallbackText}>{fallbackIcon || <Sparkles size={22} aria-hidden="true" />}{fallbackText ? <span>{fallbackText}</span> : null}</div>
  return <img src={asset.content_url} alt={alt || ''} loading="lazy" onError={() => setFailed(true)} />
}

export function SimpleError({ error, onRetry }) {
  const { t } = useI18n()
  if (!error) return null
  return <div className="simple-error" role="alert"><div><strong>{error.message || t('simple.error.title')}</strong><p>{error.details || t('simple.error.body_manage')}</p>{error.code ? <code data-machine-value>{error.code}</code> : null}</div>{onRetry ? <button type="button" className="simple-button simple-button--secondary" onClick={onRetry}>{t('common.action.reload')}</button> : null}</div>
}

export function SimpleLoading({ message }) {
  return <div className="simple-loading" role="status"><span /><p>{message}</p></div>
}

export function SimpleEmptyState({ icon, title, actions }) {
  return <div className="simple-empty-state">{icon}<h2>{title}</h2>{actions ? <div>{actions}</div> : null}</div>
}

export function SimpleNotFound({ projectUuid }) {
  const { t } = useI18n()
  return <div className="simple-project-page"><SimpleEmptyState icon={<Images size={25} aria-hidden="true" />} title={t('simple.not_found.title')} actions={<Link className="simple-button" to={projectRoute(projectUuid)}>{t('simple.not_found.body')}</Link>} /></div>
}

function assetTypeLabel(t, assetType) {
  const key = ['character', 'scene', 'prop', 'reference'].includes(assetType) ? `simple.asset_type.${assetType}` : ''
  return key ? t(key) : t('common.status.unknown_with_code', { code: assetType || '—' })
}

function simplePageRoleLabel(t, role) {
  if (role === 'front_cover') return t('simple.page.role_front')
  if (role === 'back_cover') return t('simple.page.role_back')
  return t('simple.page.role_body')
}

function generationLanguageLabel(t, value) {
  if (value === 'en') return t('common.language.en')
  if (['zh-Hans', 'zh-CN', 'zh'].includes(value)) return t('common.language.zh_hans')
  return t('common.status.unknown_with_code', { code: value || '—' })
}

export function withChatReference(search, resourceType, resourceUuid, title) {
  const params = new URLSearchParams(search)
  params.set('chat_new', '1')
  params.set('chat_reference_type', resourceType)
  params.set('chat_reference_uuid', resourceUuid)
  params.set('chat_reference_title', title || '')
  params.delete('chat_thread_uuid')
  params.delete('workflow_uuid')
  return params.toString() ? `?${params}` : ''
}

function splitTags(value) {
  return [...new Set(String(value || '').split(/[,，\n]/).map((item) => item.trim().toLowerCase()).filter(Boolean))]
}

function emptyAssetDraft() {
  return { assetType: 'character', title: '', summary: '', tags: '', file: null }
}

function emptyPageDraft() {
  return { role: 'body', title: '', text: '', direction: '' }
}

function nextChapterCode(chapters) {
  const volume = Math.max(1, ...chapters.map((chapter) => Number(chapter.volume_no) || 1))
  const number = Math.max(0, ...chapters.filter((chapter) => Number(chapter.volume_no) === volume).map((chapter) => Number(chapter.chapter_no) || 0)) + 1
  return `vol${String(volume).padStart(2, '0')}.ch${String(number).padStart(2, '0')}`
}
