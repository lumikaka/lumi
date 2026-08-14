import { useCallback, useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'

import MarkdownPreview from '../components/MarkdownPreview.jsx'
import ImageRatioNotice from '../components/ImageRatioNotice.jsx'
import { createAssetUpload, rebuildAssetThumbnail } from '../api/assets.js'
import { createComicStoryboardGeneration, listTasks } from '../api/ai.js'
import { getStoryProject, listChapters } from '../api/story.js'
import {
  cancelProductionTask, createComicSection, createStoryboard, deleteComicSection, generateSectionImage,
  getComicState, importSectionImage, listComicExports, listComicSections, listComicSnapshots, listImageVariants,
  listPremiseAssets, listProductionTasks, listStoryboards, reorderComicSections,
  retryProductionTask, selectImageVariant, selectStoryboard, updateComicSection, updatePremiseAsset,
} from '../api/production.js'
import ComicExportDialog from '../components/ComicExportDialog.jsx'
import { activeTaskFor, moveSection } from './productionWorkspaceState.js'
import { comicExportDialogRequest } from './comicExportState.js'
import { readImageFileDimensions } from './pictureBookProfile.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { comicStateLabel, sourceTypeLabel, statusLabel, taskKindLabel } from '../i18n/labels.js'

const newKey = (prefix) => `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2)}`

export function Notice({ error, children }) {
  if (!error && !children) return null
  if (error) return <LocalizedErrorMessage error={error} />
  return <div className="workspace-notice" role="status"><div><strong>{children}</strong></div></div>
}

export function ProductionImage({ projectUuid, asset, alt, profile = 'grid_256', renderReady }) {
  const { t } = useI18n()
  const [failed, setFailed] = useState(false)
  const [reload, setReload] = useState(0)
  const rebuild = useMutation({
    mutationFn: () => rebuildAssetThumbnail(projectUuid, asset.uuid, profile),
    onSuccess: () => { setFailed(false); setReload((value) => value + 1) },
  })
  useEffect(() => { setFailed(false); setReload(0) }, [asset?.uuid, asset?.content_url])
  if (!asset || asset.status !== 'ready' || failed) {
    return <div className={`production-asset-image production-asset-image--${asset?.status || 'missing'}`} role="img" aria-label={alt}><strong>{asset?.status === 'ready' ? t('comic.image_unavailable') : statusLabel(t, asset?.status || 'missing')}</strong>{asset?.uuid ? <button type="button" className="button-secondary" disabled={rebuild.isPending} onClick={() => rebuild.mutate()}>{t(rebuild.isPending ? 'comic.image_rebuilding' : 'comic.image_rebuild')}</button> : null}<LocalizedErrorMessage error={rebuild.error} compact /></div>
  }
  const separator = asset.content_url.includes('?') ? '&' : '?'
  const image = <img src={`${asset.content_url}${reload ? `${separator}preview_retry=${reload}` : ''}`} alt={alt} onError={() => setFailed(true)} />
  return renderReady ? renderReady(image) : image
}

export function ProductionTaskStrip({ projectUuid, tasks, resourceUuid, kind, refresh }) {
  const { formatNumber, t } = useI18n()
  const queryClient = useQueryClient()
  const task = (tasks || []).find((item) => item.kind === kind && item.resource_uuid === resourceUuid)
  const [error, setError] = useState(null)
  const cancel = useMutation({ mutationFn: () => cancelProductionTask(projectUuid, task.uuid), onSuccess: refresh, onError: setError })
  const retry = useMutation({ mutationFn: () => retryProductionTask(projectUuid, task.uuid), onSuccess: refresh, onError: setError })
  if (!task) return null
  return <div className="production-task-strip"><div><strong>{statusLabel(t, task.status)}</strong><span>{formatNumber(task.progress)}% · {taskKindLabel(t, task.kind)}</span></div><progress max="100" value={task.progress} />{['queued', 'running'].includes(task.status) ? <button type="button" className="button-quiet" onClick={() => cancel.mutate()}>{t('common.action.cancel')}</button> : null}{['failed', 'interrupted'].includes(task.status) ? <button type="button" className="button-secondary" onClick={() => retry.mutate()}>{t('common.action.retry')}</button> : null}{task.error_message ? <LocalizedErrorMessage error={{ code: task.error_code, message: task.error_message }} compact /> : null}<Notice error={error} />{queryClient.isFetching({ queryKey: ['production-tasks', projectUuid] }) ? <i aria-label={t('comic.task.syncing')} /> : null}</div>
}

export function ComicWorkspace({ projectUuid }) {
  const { formatDateTime, formatNumber, t } = useI18n()
  const { chapterUuid } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [error, setError] = useState(null)
  const [selectedUuid, setSelectedUuid] = useState('')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [storyboard, setStoryboard] = useState('')
  const [preview, setPreview] = useState(false)
  const [imageFile, setImageFile] = useState(null)
  const [imageFileDimensions, setImageFileDimensions] = useState(null)
  const [exportRequest, setExportRequest] = useState(null)
  const projectQuery = useQuery({ queryKey: ['story-project', projectUuid], queryFn: () => getStoryProject(projectUuid) })
  const chaptersQuery = useQuery({ queryKey: ['story-chapters', projectUuid, 'active'], queryFn: () => listChapters(projectUuid, 'active') })
  const sectionsQuery = useQuery({ queryKey: ['comic-sections', projectUuid, chapterUuid], queryFn: () => listComicSections(projectUuid, chapterUuid), enabled: Boolean(chapterUuid) })
  const stateQuery = useQuery({ queryKey: ['comic-state', projectUuid, chapterUuid], queryFn: () => getComicState(projectUuid, chapterUuid), enabled: Boolean(chapterUuid) })
  const storyboardsQuery = useQuery({ queryKey: ['comic-storyboards', projectUuid, chapterUuid, selectedUuid], queryFn: () => listStoryboards(projectUuid, chapterUuid, selectedUuid), enabled: Boolean(chapterUuid && selectedUuid) })
  const imagesQuery = useQuery({ queryKey: ['comic-images', projectUuid, chapterUuid, selectedUuid], queryFn: () => listImageVariants(projectUuid, chapterUuid, selectedUuid), enabled: Boolean(chapterUuid && selectedUuid) })
  const snapshotsQuery = useQuery({ queryKey: ['comic-snapshots', projectUuid, chapterUuid], queryFn: () => listComicSnapshots(projectUuid, chapterUuid), enabled: Boolean(chapterUuid) })
  const exportsQuery = useQuery({ queryKey: ['comic-exports', projectUuid, 'recent'], queryFn: () => listComicExports(projectUuid, { page: 1, perPage: 6 }) })
  const tasksQuery = useQuery({ queryKey: ['production-tasks', projectUuid], queryFn: () => listProductionTasks(projectUuid) })
  const storyTasksQuery = useQuery({ queryKey: ['story-tasks', projectUuid], queryFn: () => listTasks(projectUuid, { limit: 100 }) })
  const premiseAssetsQuery = useQuery({ queryKey: ['premise-assets', projectUuid, '', false], queryFn: () => listPremiseAssets(projectUuid) })
  const sections = sectionsQuery.data?.items || []
  const selected = sections.find((item) => item.uuid === selectedUuid) || sections[0]
  const pictureBook = projectQuery.data?.picture_book
  const pageMode = Boolean(pictureBook?.format && pictureBook.format !== 'vertical_strip')
  useEffect(() => { if (!chapterUuid && chaptersQuery.data?.items?.[0]) navigate(`../comic/${chaptersQuery.data.items[0].uuid}`, { replace: true }) }, [chapterUuid, chaptersQuery.data, navigate])
  useEffect(() => { if (selected && selected.uuid !== selectedUuid) setSelectedUuid(selected.uuid) }, [selected?.uuid, selectedUuid])
  useEffect(() => { if (selected) { setTitle(selected.title || ''); setDescription(selected.description_md || ''); setStoryboard(selected.current_storyboard?.content_md || '') } }, [selected?.uuid, selected?.revision])
  const refresh = useCallback(() => { ['comic-sections', 'comic-state', 'comic-storyboards', 'comic-images', 'comic-snapshots', 'comic-exports', 'production-tasks'].forEach((key) => queryClient.invalidateQueries({ queryKey: [key, projectUuid] })) }, [projectUuid, queryClient])
  const createSection = useMutation({ mutationFn: () => createComicSection(projectUuid, chapterUuid, { title: `${t(pageMode ? 'comic.page' : 'comic.section')} ${sections.length + 1}`, description_md: '', storyboard_md: '' }), onSuccess: (item) => { setSelectedUuid(item.uuid); refresh() }, onError: setError })
  const saveSection = useMutation({ mutationFn: () => updateComicSection(projectUuid, chapterUuid, selected.uuid, { title, description_md: description, expected_revision: selected.revision }), onSuccess: refresh, onError: setError })
  const saveStoryboard = useMutation({ mutationFn: () => createStoryboard(projectUuid, chapterUuid, selected.uuid, { content_md: storyboard, source_type: 'manual', expected_revision: selected.revision }), onSuccess: refresh, onError: setError })
  const removeSection = useMutation({ mutationFn: () => deleteComicSection(projectUuid, chapterUuid, selected.uuid, selected.revision), onSuccess: () => { setSelectedUuid(''); refresh() }, onError: setError })
  const reorder = useMutation({ mutationFn: (uuids) => reorderComicSections(projectUuid, chapterUuid, uuids), onSuccess: refresh, onError: setError })
  const imageImport = useMutation({ mutationFn: async () => { const upload = await createAssetUpload(projectUuid, { purpose: 'comic_section_image', displayName: `${selected.title || (pageMode ? 'page' : 'section')}.png`, file: imageFile }); return importSectionImage(projectUuid, chapterUuid, selected.uuid, { upload_uuid: upload.uuid, expected_revision: selected.revision }) }, onSuccess: () => { setImageFile(null); setImageFileDimensions(null); refresh() }, onError: setError })
	const imageGenerate = useMutation({ mutationFn: () => generateSectionImage(projectUuid, chapterUuid, selected.uuid, { prompt: storyboard, parameters: {}, idempotency_key: newKey('comic-image') }), onSuccess: refresh, onError: setError })
  const selectImageFile = async (file) => {
    setImageFile(file)
    setImageFileDimensions(null)
    if (!file) return
    try { setImageFileDimensions(await readImageFileDimensions(file)) } catch { setImageFileDimensions(null) }
  }
  const storyboardSelect = useMutation({ mutationFn: (variant) => selectStoryboard(projectUuid, chapterUuid, selected.uuid, variant.uuid, selected.revision), onSuccess: refresh, onError: setError })
  const imageSelect = useMutation({ mutationFn: (variant) => selectImageVariant(projectUuid, chapterUuid, selected.uuid, variant.uuid, selected.revision), onSuccess: refresh, onError: setError })
  const comicGenerate = useMutation({ mutationFn: () => createComicStoryboardGeneration(projectUuid, chapterUuid, { prompt: '', parameters: { temperature: 0.7 }, idempotency_key: newKey('comic-storyboard') }), onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] }); queryClient.invalidateQueries({ queryKey: ['chat-threads', projectUuid] }); queryClient.invalidateQueries({ queryKey: ['workflows', projectUuid] }) }, onError: setError })
  const tasks = tasksQuery.data?.items || []
  const comicTask = (storyTasksQuery.data?.items || []).find((task) => task.kind === 'comic_storyboard_generation' && task.resource_uuid === chapterUuid)
  useEffect(() => { if (comicTask?.status === 'completed') refresh() }, [comicTask?.status, refresh])
  const chapterOptions = chaptersQuery.data?.items || []
  const selectedChapter = chapterOptions.find((chapter) => chapter.uuid === chapterUuid)
  const selectedChapterLabel = selectedChapter ? `${selectedChapter.chapter_code} · ${selectedChapter.title || t('comic.unnamed')}` : ''
  if (!chapterUuid && chaptersQuery.isLoading) return <p className="workspace-loading">{t('story.chapter.loading')}</p>
  if (!chapterUuid && !chapterOptions.length) return <div className="workspace-empty"><h2>{t('comic.empty.create_chapter')}</h2><p>{t('comic.empty.chapter_required')}</p></div>
  return <div className="workspace-stack production-workspace comic-workspace">
    <header className="workspace-section-heading"><div><p className="eyebrow">{t('comic.eyebrow')}</p><h1>{t('comic.workspace')}</h1><p>{t('comic.description')}</p></div><div className="comic-header-actions"><select aria-label={t('comic.select_chapter')} value={chapterUuid || ''} onChange={(event) => navigate(`../comic/${event.target.value}`)}>{chapterOptions.map((chapter) => <option key={chapter.uuid} value={chapter.uuid}>{chapter.chapter_code} · {chapter.title || t('comic.unnamed')}</option>)}</select><button type="button" disabled={sections.length > 0 || comicGenerate.isPending || ['queued', 'running'].includes(comicTask?.status)} onClick={() => comicGenerate.mutate()}>{t('comic.generate_storyboard')}</button><span className={`comic-state comic-state--${stateQuery.data?.status || 'empty'}`}>{comicTask ? `${statusLabel(t, comicTask.status)} · ${formatNumber(comicTask.progress)}%` : comicStateLabel(t, stateQuery.data?.status || 'empty')}</span></div></header>
    <Notice error={error || sectionsQuery.error} />
    <section className="comic-layout"><aside className="comic-section-list"><header><h2>{t(pageMode ? 'comic.pages' : 'comic.sections')}</h2><button type="button" onClick={() => createSection.mutate()}>{t(pageMode ? 'comic.page.add' : 'comic.section.add')}</button></header>{sections.map((section, index) => <article key={section.uuid} className={section.uuid === selected?.uuid ? 'comic-section-row comic-section-row--active' : 'comic-section-row'}><button type="button" aria-pressed={section.uuid === selected?.uuid} onClick={() => setSelectedUuid(section.uuid)}><span>{String(section.section_no).padStart(2, '0')}</span><strong>{section.title || t(pageMode ? 'comic.page.untitled' : 'comic.section.untitled')}</strong><small>{t(section.current_image ? 'comic.section.has_image' : section.current_storyboard ? 'comic.section.has_storyboard' : 'comic.section.draft')}</small></button><div><button type="button" className="button-quiet" disabled={index === 0 || reorder.isPending} onClick={() => reorder.mutate(moveSection(sections.map((item) => item.uuid), index, -1))}>↑</button><button type="button" className="button-quiet" disabled={index === sections.length - 1 || reorder.isPending} onClick={() => reorder.mutate(moveSection(sections.map((item) => item.uuid), index, 1))}>↓</button></div></article>)}{sections.length === 0 ? <div className="workspace-empty"><h2>{t(pageMode ? 'comic.page.empty' : 'comic.section.empty')}</h2><p>{t(pageMode ? 'comic.page.empty_body' : 'comic.section.empty_body')}</p></div> : null}</aside>
      <div className="comic-editor">{selected ? <><section className="comic-meta-card"><header><p className="eyebrow">{t(pageMode ? 'comic.page' : 'comic.section')} {selected.section_no}</p><button type="button" className="button-quiet danger-text" onClick={() => removeSection.mutate()}>{t('common.action.delete')}</button></header><input value={title} onChange={(event) => setTitle(event.target.value)} aria-label={t(pageMode ? 'comic.page.title' : 'comic.section.title')} /><textarea rows="4" value={description} onChange={(event) => setDescription(event.target.value)} placeholder={t('comic.section.description_placeholder')} /><button type="button" onClick={() => saveSection.mutate()}>{t(pageMode ? 'comic.page.save' : 'comic.section.save')}</button></section>
        <section className="storyboard-editor-card"><header><div><p className="eyebrow">{t('comic.storyboard')}</p><h2>{t('comic.storyboard.markdown')}</h2></div><button type="button" className="button-secondary" aria-pressed={preview} onClick={() => setPreview((value) => !value)}>{t(preview ? 'comic.storyboard.edit' : 'common.action.preview')}</button></header>{preview ? <MarkdownPreview value={storyboard} /> : <textarea rows="12" value={storyboard} onChange={(event) => setStoryboard(event.target.value)} placeholder={t('comic.storyboard.placeholder')} />}<button type="button" disabled={!storyboard.trim() || saveStoryboard.isPending} onClick={() => saveStoryboard.mutate()}>{t('comic.storyboard.save_candidate')}</button><div className="variant-chip-row">{storyboardsQuery.data?.items?.map((variant) => <button type="button" key={variant.uuid} aria-pressed={selected.current_storyboard?.uuid === variant.uuid} onClick={() => storyboardSelect.mutate(variant)}>v{variant.version_no} · {sourceTypeLabel(t, variant.source_type)}</button>)}</div></section>
        <section className="comic-image-card"><header><div><p className="eyebrow">{t('comic.images.eyebrow')}</p><h2>{t(pageMode ? 'comic.images.page_title' : 'comic.images.title')}</h2><small>{t('comic.images.provider_note')}</small></div>{selected.current_image ? <ProductionImage projectUuid={projectUuid} asset={selected.current_image.asset} alt={selected.title || t(pageMode ? 'comic.page' : 'comic.section')} profile="detail_1024" /> : null}</header>{selected.current_image?.asset ? <ImageRatioNotice pictureBook={pictureBook} width={selected.current_image.asset.width} height={selected.current_image.asset.height} /> : null}<div className="image-action-row"><label>{t('comic.images.replace')}<input type="file" accept="image/*" onChange={(event) => selectImageFile(event.target.files?.[0] || null)} /></label><button type="button" disabled={!imageFile || imageImport.isPending} onClick={() => imageImport.mutate()}>{t('comic.images.import')}</button><button type="button" disabled={!selected.current_storyboard || Boolean(activeTaskFor(tasks, 'comic_image_generation', selected.uuid))} onClick={() => imageGenerate.mutate()}>{t(pageMode ? 'comic.images.generate_page' : 'comic.images.generate')}</button></div>{imageFileDimensions ? <ImageRatioNotice pictureBook={pictureBook} width={imageFileDimensions.width} height={imageFileDimensions.height} beforeImport showCompatible /> : null}<ProductionTaskStrip projectUuid={projectUuid} tasks={tasks} resourceUuid={selected.uuid} kind="comic_image_generation" refresh={refresh} /><div className="image-variant-grid">{imagesQuery.data?.items?.map((variant) => <article key={variant.uuid}><ProductionImage projectUuid={projectUuid} asset={variant.asset} alt={`v${variant.version_no}`} /><ImageRatioNotice pictureBook={pictureBook} width={variant.asset?.width} height={variant.asset?.height} /><span>v{variant.version_no}</span><button type="button" aria-pressed={selected.current_image?.uuid === variant.uuid} onClick={() => imageSelect.mutate(variant)}>{t(selected.current_image?.uuid === variant.uuid ? 'comic.images.current' : 'common.action.restore')}</button></article>)}</div></section></> : <div className="workspace-empty"><h2>{t(pageMode ? 'comic.page.empty' : 'comic.editor.empty')}</h2></div>}</div>
      <aside className="comic-history"><section><header><h2>{t('comic.snapshots')}</h2><span>{snapshotsQuery.data?.items?.length || 0}</span></header>{snapshotsQuery.data?.items?.slice(0, 12).map((snapshot) => <article key={snapshot.uuid}><strong>v{snapshot.version_no}</strong><span data-machine-value>{snapshot.reason}</span><button type="button" className="button-secondary" onClick={() => navigate(`/projects/${encodeURIComponent(projectUuid)}/chapters/${encodeURIComponent(chapterUuid)}?history=snapshots&snapshot_uuid=${encodeURIComponent(snapshot.uuid)}`)}>{t('comic.workbench.snapshot.preview')}</button></article>)}</section><section><h2>{t('projects.tab.exports')}</h2><button type="button" onClick={() => setExportRequest(comicExportDialogRequest('chapter', chapterUuid, selectedChapterLabel))}>{t('comic.exports.chapter')}</button><button type="button" className="button-secondary" onClick={() => setExportRequest(comicExportDialogRequest('project'))}>{t('comic.exports.project')}</button>{exportsQuery.data?.items?.slice(0, 6).map((item) => <article key={item.uuid}><strong>{item.scope === 'chapter' ? t('story.chapter') : t('projects.title')} · {statusLabel(t, item.status)}</strong><small>{item.relative_path || item.snapshot_hash.slice(0, 12)}</small><small>{t('projects.exports.retention_days', { days: item.retention_days || 7 })}{item.expires_at ? ` · ${t('projects.exports.expires')} ${formatDateTime(item.expires_at)}` : ''}</small>{item.download_url ? <a href={item.download_url}>{t('comic.exports.open')}</a> : null}</article>)}</section></aside>
    </section>
    {exportRequest ? <ComicExportDialog projectUuid={projectUuid} request={exportRequest} onClose={() => setExportRequest(null)} /> : null}
  </div>
}

export async function renamePremiseAsset(projectUuid, asset, title) {
  return updatePremiseAsset(projectUuid, asset.uuid, { title, expected_revision: asset.revision })
}
