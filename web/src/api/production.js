import { readSummaryPages } from './summary.js'
import { apiRequest as webRequest } from './client.js'

// One endpoint contract; callers inject only the transport.
export function createProductionAPI(apiRequest = webRequest) {

const json = (method, body = {}) => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
const root = (projectUuid, suffix = '') => `/api/v1/projects/${encodeURIComponent(projectUuid)}${suffix}`
const chapterRoot = (projectUuid, chapterUuid, suffix = '') => root(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}${suffix}`)
const sectionRoot = (projectUuid, chapterUuid, sectionUuid, suffix = '') => chapterRoot(projectUuid, chapterUuid, `/comic-sections/${encodeURIComponent(sectionUuid)}${suffix}`)

const getPremise = (projectUuid) => apiRequest(root(projectUuid, '/premise'))
const updatePremise = (projectUuid, payload) => apiRequest(root(projectUuid, '/premise'), json('PATCH', payload))
const listPremiseSources = (projectUuid, { page = 1, perPage = 20 } = {}) => apiRequest(root(projectUuid, `/premise-sources?${new URLSearchParams({ page: String(page), per_page: String(perPage) })}`))
const createPremiseSource = (projectUuid, payload) => apiRequest(root(projectUuid, '/premise-sources'), json('POST', payload))
const updatePremiseSource = (projectUuid, sourceUuid, payload) => apiRequest(root(projectUuid, `/premise-sources/${encodeURIComponent(sourceUuid)}`), json('PATCH', payload))
const generateSettingImage = (projectUuid, sourceUuid, payload) => apiRequest(root(projectUuid, `/premise-sources/${encodeURIComponent(sourceUuid)}/setting-generations`), json('POST', payload))
const listSettingImages = (projectUuid, { sourceUuids = [] } = {}) => {
  const search = new URLSearchParams()
  sourceUuids.forEach((sourceUuid) => search.append('source_uuid', sourceUuid))
  const suffix = search.size ? `?${search}` : ''
  return apiRequest(root(projectUuid, `/premise-setting-images${suffix}`))
}
const importSettingImage = (projectUuid, payload) => apiRequest(root(projectUuid, '/premise-setting-images'), json('POST', payload))
const selectSettingImage = (projectUuid, settingUuid) => apiRequest(root(projectUuid, `/premise-setting-images/${encodeURIComponent(settingUuid)}/selections`), json('POST'))
const breakdownSettingImage = (projectUuid, settingUuid, payload) => apiRequest(root(projectUuid, `/premise-setting-images/${encodeURIComponent(settingUuid)}/breakdowns`), json('POST', payload))
const listPremiseAssets = (projectUuid, { tag = '', state = 'active' } = {}) => apiRequest(root(projectUuid, `/premise-assets?${new URLSearchParams({ tag, state })}`))
const getPremiseAsset = (projectUuid, assetUuid) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}`))
const createPremiseAsset = (projectUuid, payload) => apiRequest(root(projectUuid, '/premise-assets'), json('POST', payload))
const updatePremiseAsset = (projectUuid, assetUuid, payload) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}`), json('PATCH', payload))
const trashPremiseAsset = (projectUuid, assetUuid, revision) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}?expected_revision=${encodeURIComponent(revision)}`), { method: 'DELETE' })
const restorePremiseAsset = (projectUuid, assetUuid, revision) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}/restorations`), json('POST', { expected_revision: revision }))
const permanentlyDeletePremiseAsset = (projectUuid, assetUuid, revision) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}/permanent?expected_revision=${encodeURIComponent(revision)}`), { method: 'DELETE' })
const emptyPremiseAssetTrash = (projectUuid) => apiRequest(root(projectUuid, '/premise-assets/trash'), { method: 'DELETE' })
const listPremiseAssetVariants = (projectUuid, assetUuid) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}/variants`))
const createPremiseAssetVariant = (projectUuid, assetUuid, payload) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}/variants`), json('POST', payload))
const selectPremiseAssetVariant = (projectUuid, assetUuid, variantUuid, revision) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}/variants/${encodeURIComponent(variantUuid)}/selections`), json('POST', { expected_revision: revision }))
const generatePremiseAssetVariant = (projectUuid, assetUuid, payload) => apiRequest(root(projectUuid, `/premise-assets/${encodeURIComponent(assetUuid)}/generations`), json('POST', payload))

const getComicState = (projectUuid, chapterUuid) => apiRequest(chapterRoot(projectUuid, chapterUuid, '/comic'))
const listComicSections = (projectUuid, chapterUuid, { state = '' } = {}) => {
  const search = new URLSearchParams()
  if (state) search.set('state', state)
  return apiRequest(chapterRoot(projectUuid, chapterUuid, `/comic-sections${search.size ? `?${search}` : ''}`))
}
const createComicSection = (projectUuid, chapterUuid, payload) => apiRequest(chapterRoot(projectUuid, chapterUuid, '/comic-sections'), json('POST', payload))
const updateComicSection = (projectUuid, chapterUuid, sectionUuid, payload) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid), json('PATCH', payload))
const deleteComicSection = (projectUuid, chapterUuid, sectionUuid, revision) => apiRequest(`${sectionRoot(projectUuid, chapterUuid, sectionUuid)}?expected_revision=${encodeURIComponent(revision)}`, { method: 'DELETE' })
const restoreComicSection = (projectUuid, chapterUuid, sectionUuid, revision) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, '/restorations'), json('POST', { expected_revision: revision }))
const reorderComicSections = (projectUuid, chapterUuid, sectionUuids) => apiRequest(chapterRoot(projectUuid, chapterUuid, '/comic-section-order'), json('PUT', { section_uuids: sectionUuids }))
const setComicSectionPremiseAssets = (projectUuid, chapterUuid, sectionUuid, premiseAssetUuids, revision) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, '/premise-assets'), json('PUT', { premise_asset_uuids: premiseAssetUuids, expected_revision: revision }))
const listStoryboards = (projectUuid, chapterUuid, sectionUuid) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, '/storyboard-variants'))
const createStoryboard = (projectUuid, chapterUuid, sectionUuid, payload) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, '/storyboard-variants'), json('POST', payload))
const selectStoryboard = (projectUuid, chapterUuid, sectionUuid, variantUuid, revision) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, `/storyboard-variants/${encodeURIComponent(variantUuid)}/selections`), json('POST', { expected_revision: revision }))
const importSectionImage = (projectUuid, chapterUuid, sectionUuid, payload) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, '/images'), json('POST', payload))
const listImageVariants = (projectUuid, chapterUuid, sectionUuid) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, '/image-variants'))
const selectImageVariant = (projectUuid, chapterUuid, sectionUuid, variantUuid, revision) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, `/image-variants/${encodeURIComponent(variantUuid)}/selections`), json('POST', { expected_revision: revision }))
const generateSectionImage = (projectUuid, chapterUuid, sectionUuid, payload) => apiRequest(sectionRoot(projectUuid, chapterUuid, sectionUuid, '/image-generations'), json('POST', payload))
const generateChapterImagesBatch = (projectUuid, chapterUuid, payload) => apiRequest(chapterRoot(projectUuid, chapterUuid, '/comic-image-generation-batches'), json('POST', payload))
const listComicSnapshots = (projectUuid, chapterUuid) => apiRequest(chapterRoot(projectUuid, chapterUuid, '/comic-snapshots'))
const getComicSnapshot = (projectUuid, chapterUuid, snapshotUuid) => apiRequest(chapterRoot(projectUuid, chapterUuid, `/comic-snapshots/${encodeURIComponent(snapshotUuid)}`))
const restoreComicSnapshot = (projectUuid, chapterUuid, snapshotUuid) => apiRequest(chapterRoot(projectUuid, chapterUuid, `/comic-snapshots/${encodeURIComponent(snapshotUuid)}/restorations`), json('POST'))
const listComicExports = (projectUuid, { page = 1, perPage = 20, scope = '', chapterUuid = '', taskUuid = '', snapshotHash = '', format = '', status = '' } = {}) => {
  const search = new URLSearchParams({ page: String(page), per_page: String(perPage) })
  if (scope) search.set('scope', scope)
  if (chapterUuid) search.set('chapter_uuid', chapterUuid)
  if (taskUuid) search.set('task_uuid', taskUuid)
  if (snapshotHash) search.set('snapshot_hash', snapshotHash)
  if (format) search.set('format', format)
  if (status) search.set('status', status)
  return apiRequest(root(projectUuid, `/comic-exports?${search}`))
}
const getComicExportReadiness = (projectUuid, { scope, chapterUuid = '' }) => apiRequest(root(projectUuid, `/comic-exports/readiness?${new URLSearchParams({ scope, chapter_uuid: chapterUuid })}`))
const createComicExport = (projectUuid, payload) => apiRequest(root(projectUuid, '/comic-exports'), json('POST', payload))
const listProductionTasks = (projectUuid, limit = 100) => apiRequest(root(projectUuid, `/production-tasks?limit=${encodeURIComponent(limit)}`))
const getProductionTask = (projectUuid, taskUuid) => apiRequest(root(projectUuid, `/production-tasks/${encodeURIComponent(taskUuid)}`))
const listProductionTaskEvents = (projectUuid, taskUuid, query = {}) => apiRequest(root(projectUuid, `/production-tasks/${encodeURIComponent(taskUuid)}/events?${new URLSearchParams(query)}`))
const cancelProductionTask = (projectUuid, taskUuid) => apiRequest(root(projectUuid, `/production-tasks/${encodeURIComponent(taskUuid)}/cancellations`), json('POST'))
const retryProductionTask = (projectUuid, taskUuid) => apiRequest(root(projectUuid, `/production-tasks/${encodeURIComponent(taskUuid)}/retries`), json('POST'))

const getProjectWebsite = (projectUuid) => apiRequest(root(projectUuid, '/website'))
const generateProjectWebsite = (projectUuid, payload) => apiRequest(root(projectUuid, '/website/generations'), json('POST', payload))
const openProjectWebsiteDirectory = (projectUuid) => apiRequest(root(projectUuid, '/website/directory-openings'), json('POST'))

const listComicSectionSummaries = (projectUuid,chapterUuid,{state='active',...options}={}) => readSummaryPages(apiRequest,chapterRoot(projectUuid,chapterUuid,'/comic-sections'),{state},options)
const getComicSection = (projectUuid,chapterUuid,sectionUuid,options={}) => apiRequest(sectionRoot(projectUuid,chapterUuid,sectionUuid),options)

  return { listComicSectionSummaries, getComicSection, getPremise, updatePremise, listPremiseSources, createPremiseSource, updatePremiseSource, generateSettingImage, listSettingImages, importSettingImage, selectSettingImage, breakdownSettingImage, listPremiseAssets, getPremiseAsset, createPremiseAsset, updatePremiseAsset, trashPremiseAsset, restorePremiseAsset, permanentlyDeletePremiseAsset, emptyPremiseAssetTrash, listPremiseAssetVariants, createPremiseAssetVariant, selectPremiseAssetVariant, generatePremiseAssetVariant, getComicState, listComicSections, createComicSection, updateComicSection, deleteComicSection, restoreComicSection, reorderComicSections, setComicSectionPremiseAssets, listStoryboards, createStoryboard, selectStoryboard, importSectionImage, listImageVariants, selectImageVariant, generateSectionImage, generateChapterImagesBatch, listComicSnapshots, getComicSnapshot, restoreComicSnapshot, listComicExports, getComicExportReadiness, createComicExport, listProductionTasks, getProductionTask, listProductionTaskEvents, cancelProductionTask, retryProductionTask, getProjectWebsite, generateProjectWebsite, openProjectWebsiteDirectory }
}

export const { listComicSectionSummaries, getComicSection, getPremise, updatePremise, listPremiseSources, createPremiseSource, updatePremiseSource, generateSettingImage, listSettingImages, importSettingImage, selectSettingImage, breakdownSettingImage, listPremiseAssets, getPremiseAsset, createPremiseAsset, updatePremiseAsset, trashPremiseAsset, restorePremiseAsset, permanentlyDeletePremiseAsset, emptyPremiseAssetTrash, listPremiseAssetVariants, createPremiseAssetVariant, selectPremiseAssetVariant, generatePremiseAssetVariant, getComicState, listComicSections, createComicSection, updateComicSection, deleteComicSection, restoreComicSection, reorderComicSections, setComicSectionPremiseAssets, listStoryboards, createStoryboard, selectStoryboard, importSectionImage, listImageVariants, selectImageVariant, generateSectionImage, generateChapterImagesBatch, listComicSnapshots, getComicSnapshot, restoreComicSnapshot, listComicExports, getComicExportReadiness, createComicExport, listProductionTasks, getProductionTask, listProductionTaskEvents, cancelProductionTask, retryProductionTask, getProjectWebsite, generateProjectWebsite, openProjectWebsiteDirectory } = createProductionAPI()
