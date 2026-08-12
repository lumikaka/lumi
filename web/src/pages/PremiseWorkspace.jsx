import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery, useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import {
  ChevronDown,
  ChevronRight,
  EyeOff,
  History,
  ImagePlus,
  MessageSquareQuote,
  Plus,
  RotateCcw,
  Sparkles,
  Trash2,
  Upload,
  X,
} from 'lucide-react'

import { createAssetUpload } from '../api/assets.js'
import { getStoryProfile } from '../api/story.js'
import {
  breakdownSettingImage,
  createPremiseAsset,
  createPremiseAssetVariant,
  createPremiseSource,
  generateSettingImage,
  getPremise,
  importSettingImage,
  listPremiseAssets,
  listPremiseAssetVariants,
  listPremiseSources,
  listProductionTasks,
  listSettingImages,
  emptyPremiseAssetTrash,
  permanentlyDeletePremiseAsset,
  restorePremiseAsset,
  selectPremiseAssetVariant,
  selectSettingImage,
  trashPremiseAsset,
  updatePremise,
  updatePremiseAsset,
  updatePremiseSource,
} from '../api/production.js'
import { useProjectRealtime } from '../realtime/useProjectRealtime.js'
import LumiDialog from '../components/LumiDialog.jsx'
import { Notice, ProductionImage, ProductionTaskStrip } from './ProductionWorkspaces.jsx'
import ProjectLLMLogsPanel from './ProjectLLMLogsPanel.jsx'
import { PremisePromptsPanel, PremiseThreadsPanel, usePremiseThreads } from './PremiseSupportPanels.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { sourceTypeLabel } from '../i18n/labels.js'
import {
  activeTaskFor,
  collectPremiseTags,
  normalizedTags,
  premiseAssetTitleFromFile,
  premiseSourceState,
} from './productionWorkspaceState.js'

const newKey = (prefix) => `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2)}`
// i18n-exempt: generation prompt content follows the project generation pipeline, not the interface locale.
const PREMISE_BREAKDOWN_GUIDANCE = '识别主要角色、场景与道具并裁剪为独立资产。'

const SOURCE_STATE_COPY = Object.freeze({
  draft: 'premise.source_state.draft',
  generating: 'premise.source_state.generating',
  ready: 'premise.source_state.ready',
  splitting: 'premise.source_state.splitting',
  completed: 'premise.source_state.completed',
  failed: 'premise.source_state.failed',
  ignored: 'premise.source_state.ignored',
})

const ASSET_TYPE_COPY = Object.freeze({
  character: 'premise.asset_type.character',
  scene: 'premise.asset_type.scene',
  prop: 'premise.asset_type.prop',
  reference: 'premise.asset_type.reference',
})

function isEditableTarget(target) {
  return target instanceof Element && Boolean(target.closest('input, textarea, select, [contenteditable="true"]'))
}

function createUploadDraft(file, untitledLabel) {
  return {
    id: newKey('premise-upload'),
    file,
    previewUrl: URL.createObjectURL(file),
    assetType: 'character',
    title: premiseAssetTitleFromFile(file, untitledLabel),
    summary: '',
    tags: '',
  }
}

function EmptyState({ title, description, actions }) {
  return (
    <div className="premise-empty-state">
      <ImagePlus size={32} aria-hidden="true" />
      <h2>{title}</h2>
      <p>{description}</p>
      {actions ? <div>{actions}</div> : null}
    </div>
  )
}

function LoadingCards() {
  const { t } = useI18n()
  return <div className="premise-card-grid" aria-label={t('premise.loading_cards')}>{Array.from({ length: 8 }, (_, index) => <div className="premise-card-skeleton" key={index} />)}</div>
}

export default function PremiseWorkspace({ projectUuid }) {
  const { formatDateTime, locale, t } = useI18n()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const addMenuRef = useRef(null)
  const fileInputRef = useRef(null)
  const uploadUrlsRef = useRef([])
  const [error, setError] = useState(null)
  const [decisionNotice, setDecisionNotice] = useState('')
  const [activeTab, setActiveTab] = useState('assets')
  const [addMenuOpen, setAddMenuOpen] = useState(false)
  const [filterTag, setFilterTag] = useState('')
  const [style, setStyle] = useState('')
  const [styleEditorOpen, setStyleEditorOpen] = useState(false)
  const [batchDialogOpen, setBatchDialogOpen] = useState(false)
  const [batchText, setBatchText] = useState('')
  const [batchStyle, setBatchStyle] = useState('')
  const [settingFile, setSettingFile] = useState(null)
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false)
  const [uploadDrafts, setUploadDrafts] = useState([])
  const [uploadProgress, setUploadProgress] = useState(0)
  const [dragActive, setDragActive] = useState(false)
  const [historyAsset, setHistoryAsset] = useState(null)
  const [detailDraft, setDetailDraft] = useState(null)
  const [replacementFile, setReplacementFile] = useState(null)
  const [expandedSourceUuids, setExpandedSourceUuids] = useState(() => new Set())
  const [deleteRequest, setDeleteRequest] = useState(null)

  const premiseQuery = useQuery({ queryKey: ['premise', projectUuid], queryFn: () => getPremise(projectUuid) })
  const sourcesQuery = useInfiniteQuery({
    queryKey: ['premise-sources', projectUuid],
    queryFn: ({ pageParam }) => listPremiseSources(projectUuid, { page: pageParam, perPage: 20 }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) => lastPage.pagination?.current_page < lastPage.pagination?.last_page
      ? lastPage.pagination.current_page + 1
      : undefined,
  })
  const activeAssetsQuery = useQuery({ queryKey: ['premise-assets', projectUuid, '', false], queryFn: () => listPremiseAssets(projectUuid, { state: 'active' }) })
  const trashAssetsQuery = useQuery({ queryKey: ['premise-assets', projectUuid, '', true], queryFn: () => listPremiseAssets(projectUuid, { state: 'trashed' }) })
  const variantsQuery = useQuery({
    queryKey: ['premise-variants', projectUuid, historyAsset?.uuid],
    queryFn: () => listPremiseAssetVariants(projectUuid, historyAsset.uuid),
    enabled: Boolean(historyAsset),
  })
  const tasksQuery = useQuery({
    queryKey: ['production-tasks', projectUuid],
    queryFn: () => listProductionTasks(projectUuid),
    refetchInterval: (query) => query.state.data?.items?.some((task) => ['queued', 'running'].includes(task.status)) ? 1200 : false,
  })
  const storyProfileQuery = useQuery({
    queryKey: ['story-profile', projectUuid],
    queryFn: () => getStoryProfile(projectUuid),
    enabled: batchDialogOpen,
  })
  const premiseThreadsQuery = usePremiseThreads(projectUuid)

  const refresh = useCallback(() => {
    ['premise', 'premise-sources', 'premise-settings', 'premise-assets', 'premise-variants', 'production-tasks', 'story-project', 'asset-scans', 'asset-maintenance-tasks'].forEach((key) => {
      queryClient.invalidateQueries({ queryKey: [key, projectUuid] })
    })
  }, [projectUuid, queryClient])

  useProjectRealtime(projectUuid, useCallback((event) => {
    if (event.startsWith('premise:') || event.startsWith('production_') || event === 'production:resource_changed' || event === 'phx_reconnected') refresh()
  }, [refresh]))

  useEffect(() => {
    if (premiseQuery.data) setStyle(premiseQuery.data.default_style || '')
  }, [premiseQuery.data?.revision])

  useEffect(() => {
    uploadUrlsRef.current = uploadDrafts.map((draft) => draft.previewUrl)
  }, [uploadDrafts])

  useEffect(() => () => {
    uploadUrlsRef.current.forEach((url) => URL.revokeObjectURL(url))
  }, [])

  useEffect(() => {
    if (!addMenuOpen) return undefined
    const closeOnPointerDown = (event) => {
      if (!addMenuRef.current?.contains(event.target)) setAddMenuOpen(false)
    }
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') setAddMenuOpen(false)
    }
    document.addEventListener('pointerdown', closeOnPointerDown)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeOnPointerDown)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [addMenuOpen])

  const styleMutation = useMutation({
    onMutate: () => setError(null),
    mutationFn: () => updatePremise(projectUuid, { default_style: style, expected_revision: premiseQuery.data.revision }),
    onSuccess: () => { setStyleEditorOpen(false); refresh() },
    onError: setError,
  })
  const batchMutation = useMutation({
    onMutate: () => setError(null),
    mutationFn: async () => {
      const source = await createPremiseSource(projectUuid, {
        source_text: batchText.trim(),
        style_snapshot: batchStyle.trim() || style,
        source_type: 'manual',
        parameters: {},
      })
      await generateSettingImage(projectUuid, source.uuid, {
        prompt: source.source_text,
        parameters: source.parameters || {},
        idempotency_key: newKey('premise-setting'),
      })
      return source
    },
    onSuccess: () => {
      setBatchDialogOpen(false)
      setBatchText('')
      setActiveTab('batches')
      refresh()
    },
    onError: setError,
  })
  const generateMutation = useMutation({
    onMutate: () => setError(null),
    mutationFn: (source) => generateSettingImage(projectUuid, source.uuid, {
      prompt: source.source_text,
      parameters: source.parameters || {},
      idempotency_key: newKey('premise-setting'),
    }),
    onSuccess: refresh,
    onError: setError,
  })
  const settingImport = useMutation({
    onMutate: () => setError(null),
    mutationFn: async () => {
      const upload = await createAssetUpload(projectUuid, { purpose: 'premise_setting_image', displayName: settingFile.name, file: settingFile })
      return importSettingImage(projectUuid, { upload_uuid: upload.uuid, source_uuid: premiseQuery.data?.current_source?.uuid || '' })
    },
    onSuccess: () => { setSettingFile(null); refresh() },
    onError: setError,
  })
  const breakdownMutation = useMutation({
    onMutate: () => setError(null),
    mutationFn: (setting) => breakdownSettingImage(projectUuid, setting.uuid, {
      prompt: PREMISE_BREAKDOWN_GUIDANCE,
      parameters: {},
      idempotency_key: newKey('premise-breakdown'),
    }),
    onSuccess: refresh,
    onError: setError,
  })
  const selectSetting = useMutation({ onMutate: () => setError(null), mutationFn: (uuid) => selectSettingImage(projectUuid, uuid), onSuccess: refresh, onError: setError })
  const trashMutation = useMutation({ onMutate: () => setError(null), mutationFn: (asset) => trashPremiseAsset(projectUuid, asset.uuid, asset.revision), onSuccess: refresh, onError: setError })
  const restoreMutation = useMutation({ onMutate: () => setError(null), mutationFn: (asset) => restorePremiseAsset(projectUuid, asset.uuid, asset.revision), onSuccess: refresh, onError: setError })
  const permanentDeleteMutation = useMutation({
    onMutate: () => setError(null),
    mutationFn: (asset) => permanentlyDeletePremiseAsset(projectUuid, asset.uuid, asset.revision),
    onSuccess: () => { setDeleteRequest(null); setDecisionNotice('premise.assets.permanent_deleted'); refresh() },
    onError: setError,
  })
  const emptyTrashMutation = useMutation({
    onMutate: () => setError(null),
    mutationFn: () => emptyPremiseAssetTrash(projectUuid),
    onSuccess: (result) => {
      setDeleteRequest(null)
      setDecisionNotice(result.blocked_items?.length ? 'premise.assets.empty_partial' : 'premise.assets.empty_done')
      refresh()
    },
    onError: setError,
  })
  const sourceIgnoreMutation = useMutation({
    onMutate: () => setError(null),
    mutationFn: ({ source, ignored }) => updatePremiseSource(projectUuid, source.uuid, { ignored, expected_revision: source.revision }),
    onSuccess: refresh,
    onError: setError,
  })
  const uploadMutation = useMutation({
    onMutate: () => setError(null),
    mutationFn: async () => {
      setUploadProgress(0)
      const created = []
      const completedDraftIds = []
      try {
        for (const draft of uploadDrafts) {
          const upload = await createAssetUpload(projectUuid, { purpose: 'premise_asset', displayName: draft.title.trim(), file: draft.file })
          created.push(await createPremiseAsset(projectUuid, {
            upload_uuid: upload.uuid,
            asset_type: draft.assetType,
            title: draft.title.trim(),
            summary: draft.summary.trim(),
            tags: normalizedTags(draft.tags),
            position: {},
            crop: {},
          }))
          completedDraftIds.push(draft.id)
          setUploadProgress(created.length)
        }
      } catch (uploadError) {
        if (completedDraftIds.length) {
          setUploadDrafts((current) => current.filter((draft) => {
            if (completedDraftIds.includes(draft.id)) URL.revokeObjectURL(draft.previewUrl)
            return !completedDraftIds.includes(draft.id)
          }))
          setUploadProgress(0)
          refresh()
        }
        throw uploadError
      }
      return created
    },
    onSuccess: () => {
      uploadDrafts.forEach((draft) => URL.revokeObjectURL(draft.previewUrl))
      setUploadDrafts([])
      setUploadDialogOpen(false)
      setUploadProgress(0)
      setActiveTab('assets')
      refresh()
    },
    onError: setError,
  })
  const selectVariant = useMutation({
    onMutate: () => setError(null),
    mutationFn: (variant) => selectPremiseAssetVariant(projectUuid, historyAsset.uuid, variant.uuid, historyAsset.revision),
    onSuccess: (asset) => { setHistoryAsset(asset); refresh() },
    onError: setError,
  })
  const replaceImage = useMutation({
    onMutate: () => setError(null),
    mutationFn: async () => {
      const upload = await createAssetUpload(projectUuid, { purpose: 'premise_asset', displayName: historyAsset.title, file: replacementFile })
      return createPremiseAssetVariant(projectUuid, historyAsset.uuid, { upload_uuid: upload.uuid, crop: {}, expected_revision: historyAsset.revision })
    },
    onSuccess: (asset) => { setHistoryAsset(asset); setReplacementFile(null); refresh() },
    onError: setError,
  })
  const updateAsset = useMutation({
    onMutate: () => setError(null),
    mutationFn: () => updatePremiseAsset(projectUuid, historyAsset.uuid, {
      asset_type: detailDraft.assetType,
      title: detailDraft.title.trim(),
      summary: detailDraft.summary.trim(),
      tags: normalizedTags(detailDraft.tags),
      expected_revision: historyAsset.revision,
    }),
    onSuccess: (asset) => {
      setHistoryAsset(asset)
      setDetailDraft({ assetType: asset.asset_type, title: asset.title, summary: asset.summary || '', tags: (asset.tags || []).join(', ') })
      refresh()
    },
    onError: setError,
  })

  const activeAssets = activeAssetsQuery.data?.items || []
  const trashedAssets = trashAssetsQuery.data?.items || []
  const sources = useMemo(() => {
    const seen = new Set()
    return (sourcesQuery.data?.pages || []).flatMap((page) => page.items || []).filter((source) => {
      if (seen.has(source.uuid)) return false
      seen.add(source.uuid)
      return true
    })
  }, [sourcesQuery.data?.pages])
  const sourceTotal = sourcesQuery.data?.pages?.[0]?.pagination?.total ?? sources.length
  const settingQueries = useQueries({
    queries: (sourcesQuery.data?.pages || []).map((page) => {
      const sourceUuids = (page.items || []).map((source) => source.uuid)
      return {
        queryKey: ['premise-settings', projectUuid, sourceUuids],
        queryFn: () => listSettingImages(projectUuid, { sourceUuids }),
        enabled: sourceUuids.length > 0,
      }
    }),
  })
  const settings = useMemo(() => {
    const seen = new Set()
    return settingQueries.flatMap((query) => query.data?.items || []).filter((setting) => {
      if (seen.has(setting.uuid)) return false
      seen.add(setting.uuid)
      return true
    })
  }, [settingQueries])
  const settingsError = settingQueries.find((query) => query.error)?.error
  const tasks = tasksQuery.data?.items || []
  const tags = useMemo(() => collectPremiseTags(activeAssets, locale), [activeAssets, locale])
  const displayedAssets = useMemo(() => filterTag ? activeAssets.filter((asset) => (asset.tags || []).includes(filterTag)) : activeAssets, [activeAssets, filterTag])
  const currentSettingUuid = premiseQuery.data?.current_setting_image?.uuid
  const activeTaskCount = tasks.filter((task) => ['queued', 'running'].includes(task.status) && ['premise_setting_generation', 'premise_asset_breakdown', 'premise_asset_generation'].includes(task.kind)).length

  const openBatchDialog = () => {
    setBatchText('')
    setBatchStyle(style)
    setBatchDialogOpen(true)
    setAddMenuOpen(false)
  }

  const addUploadFiles = (files) => {
    const images = Array.from(files || []).filter((file) => file.type.startsWith('image/'))
    if (!images.length) {
      setDecisionNotice('premise.upload.images_only')
      return
    }
    setUploadDrafts((current) => [...current, ...images.map((file) => createUploadDraft(file, t('premise.assets.untitled')))])
    setUploadDialogOpen(true)
    setAddMenuOpen(false)
  }

  const closeUploadDialog = () => {
    if (uploadMutation.isPending) return
    uploadDrafts.forEach((draft) => URL.revokeObjectURL(draft.previewUrl))
    setUploadDrafts([])
    setUploadProgress(0)
    setUploadDialogOpen(false)
  }

  const removeUploadDraft = (id) => {
    setUploadDrafts((current) => current.filter((draft) => {
      if (draft.id === id) URL.revokeObjectURL(draft.previewUrl)
      return draft.id !== id
    }))
  }

  const updateUploadDraft = (id, key, value) => {
    setUploadDrafts((current) => current.map((draft) => draft.id === id ? { ...draft, [key]: value } : draft))
  }

  const appendDraftTag = (id, tag) => {
    setUploadDrafts((current) => current.map((draft) => draft.id === id
      ? { ...draft, tags: [...normalizedTags(draft.tags), tag].join(', ') }
      : draft))
  }

  const openAssetDetail = (asset) => {
    setHistoryAsset(asset)
    setDetailDraft({ assetType: asset.asset_type, title: asset.title, summary: asset.summary || '', tags: (asset.tags || []).join(', ') })
  }

  const updateChatQuery = ({ threadUuid = '', scene = '', subject = null } = {}) => {
    const next = new URLSearchParams(searchParams)
    next.set('chat_scope', 'premise')
    next.delete('workflow_uuid')
    if (threadUuid) {
      next.set('chat_thread_uuid', threadUuid)
      next.delete('chat_new')
    } else {
      next.delete('chat_thread_uuid')
      next.set('chat_new', '1')
    }
    if (scene) next.set('chat_scene', scene)
    else next.delete('chat_scene')
    if (subject?.uuid) {
      next.set('chat_subject_uuid', subject.uuid)
      next.set('chat_subject_title', subject.title || '')
    } else {
      next.delete('chat_subject_uuid')
      next.delete('chat_subject_title')
    }
    setSearchParams(next)
    setAddMenuOpen(false)
  }

  const openChatScene = (scene, subject = null) => {
    updateChatQuery({ scene, subject })
    if (historyAsset) closeAssetDetail()
  }

  const toggleSourceDetails = (sourceUuid) => {
    setExpandedSourceUuids((current) => {
      const next = new Set(current)
      if (next.has(sourceUuid)) next.delete(sourceUuid)
      else next.add(sourceUuid)
      return next
    })
  }

  const closeAssetDetail = () => {
    if (replaceImage.isPending || updateAsset.isPending) return
    setHistoryAsset(null)
    setDetailDraft(null)
    setReplacementFile(null)
  }

  useEffect(() => {
    if (!batchDialogOpen && !uploadDialogOpen && !historyAsset) return undefined
    const closeOnEscape = (event) => {
      if (event.key !== 'Escape') return
      if (historyAsset && !replaceImage.isPending && !updateAsset.isPending) {
        setHistoryAsset(null)
        setDetailDraft(null)
        setReplacementFile(null)
        return
      }
      if (uploadDialogOpen && !uploadMutation.isPending) {
        uploadDrafts.forEach((draft) => URL.revokeObjectURL(draft.previewUrl))
        setUploadDrafts([])
        setUploadProgress(0)
        setUploadDialogOpen(false)
        return
      }
      if (batchDialogOpen && !batchMutation.isPending) setBatchDialogOpen(false)
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [batchDialogOpen, batchMutation.isPending, historyAsset, replaceImage.isPending, updateAsset.isPending, uploadDialogOpen, uploadDrafts, uploadMutation.isPending])

  const tabs = [
    { key: 'assets', labelKey: 'premise.tab.assets', count: activeAssets.length },
    { key: 'trash', labelKey: 'projects.tab.trash', count: trashedAssets.length },
    { key: 'batches', labelKey: 'premise.tab.batches', count: sourceTotal },
    { key: 'threads', labelKey: 'premise.threads.title', count: premiseThreadsQuery.data?.pages?.[0]?.pagination?.total || 0 },
    { key: 'prompts', labelKey: 'projects.tab.prompts' },
    { key: 'llm_logs', labelKey: 'premise.tab.llm_logs' },
  ]

  const queryError = error || premiseQuery.error || sourcesQuery.error || settingsError || activeAssetsQuery.error || trashAssetsQuery.error || tasksQuery.error
  if (premiseQuery.isLoading) return <p className="workspace-loading">{t('premise.loading')}</p>

  return (
    <div
      className={`premise-workspace${dragActive ? ' premise-workspace--dragging' : ''}`}
      onDragEnter={(event) => { if (event.dataTransfer?.types?.includes('Files')) { event.preventDefault(); setDragActive(true) } }}
      onDragOver={(event) => { if (event.dataTransfer?.types?.includes('Files')) event.preventDefault() }}
      onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) setDragActive(false) }}
      onDrop={(event) => { event.preventDefault(); setDragActive(false); addUploadFiles(event.dataTransfer.files) }}
      onPaste={(event) => {
        if (isEditableTarget(event.target)) return
        const files = Array.from(event.clipboardData?.files || []).filter((file) => file.type.startsWith('image/'))
        if (files.length) { event.preventDefault(); addUploadFiles(files) }
      }}
    >
      <header className="premise-toolbar">
        <div className="premise-toolbar__tabs" role="tablist" aria-label={t('premise.toolbar')}>
          {tabs.map((tab) => (
            <button
              type="button"
              className="premise-toolbar__tab"
              role="tab"
              key={tab.key}
              aria-selected={activeTab === tab.key}
              onClick={() => setActiveTab(tab.key)}
            >
              {t(tab.labelKey)}{typeof tab.count === 'number' ? <span>{tab.count}</span> : null}
            </button>
          ))}
        </div>
        <div className="premise-add" ref={addMenuRef}>
          <button type="button" className="premise-add__trigger" aria-haspopup="menu" aria-expanded={addMenuOpen} onClick={() => setAddMenuOpen((value) => !value)}>
            <Plus size={16} aria-hidden="true" />{t('premise.add')}<ChevronDown size={15} aria-hidden="true" />
          </button>
          {addMenuOpen ? (
            <div className="premise-add__menu" role="menu">
              <button type="button" className="premise-add__item" role="menuitem" onClick={openBatchDialog}><Sparkles size={16} aria-hidden="true" /><span><strong>{t('premise.add.batch.title')}</strong><small>{t('premise.add.batch.body')}</small></span></button>
              <button type="button" className="premise-add__item" role="menuitem" onClick={() => openChatScene('premise_asset_generation')}><Plus size={16} aria-hidden="true" /><span><strong>{t('premise.add.single.title')}</strong><small>{t('premise.add.single.body')}</small></span></button>
              <button type="button" className="premise-add__item" role="menuitem" onClick={() => { setUploadDialogOpen(true); setAddMenuOpen(false) }}><Upload size={16} aria-hidden="true" /><span><strong>{t('premise.add.upload.title')}</strong><small>{t('premise.add.upload.body')}</small></span></button>
            </div>
          ) : null}
          <input ref={fileInputRef} className="premise-visually-hidden" type="file" accept="image/*" multiple aria-hidden="true" tabIndex="-1" onChange={(event) => { addUploadFiles(event.target.files); event.target.value = '' }} />
        </div>
      </header>

      {activeTaskCount ? <div className="premise-running-state" role="status"><span />{t('premise.tasks.running', { count: activeTaskCount })}</div> : null}
      <Notice error={queryError} />
      {decisionNotice ? <div className="premise-decision-notice" role="status"><span>{t(decisionNotice)}</span><button type="button" className="premise-decision-notice__close" onClick={() => setDecisionNotice('')} aria-label={t('premise.notice.close')}><X size={14} aria-hidden="true" /></button></div> : null}

      {activeTab === 'assets' ? (
        <section className="premise-content-panel" role="tabpanel" aria-busy={activeAssetsQuery.isLoading}>
          <header className="premise-content-heading premise-content-heading--assets">
            <div><p className="premise-content-eyebrow">{t('premise.assets')}</p><h2>{t('premise.assets.list_title')}</h2></div>
            <span>{displayedAssets.length}</span>
          </header>
          {tags.length ? (
            <div className="premise-asset-filter">
              <span className="premise-asset-filter__label">{t('premise.assets.filter.tags')}</span>
              <div className="premise-tag-filters" aria-label={t('premise.assets.filter.navigation')}>
                <button type="button" className="premise-tag-filter" aria-pressed={!filterTag} onClick={() => setFilterTag('')}>{t('common.label.all')}</button>
                {tags.map((tag) => <button type="button" className="premise-tag-filter" key={tag} aria-pressed={filterTag === tag} onClick={() => setFilterTag(tag)}>{tag}</button>)}
              </div>
            </div>
          ) : null}
          {activeAssetsQuery.isLoading ? <LoadingCards /> : null}
          {!activeAssetsQuery.isLoading && activeAssetsQuery.isError ? (
            <EmptyState title={t('premise.assets.load_failed')} description={t('premise.assets.load_failed_body')} actions={<button type="button" onClick={() => activeAssetsQuery.refetch()}>{t('common.action.reload')}</button>} />
          ) : null}
          {!activeAssetsQuery.isLoading && !activeAssetsQuery.isError && displayedAssets.length === 0 ? (
            filterTag
              ? <EmptyState title={t('premise.assets.no_match')} description={t('premise.assets.no_match_body', { tag: filterTag })} actions={<button type="button" className="button-secondary" onClick={() => setFilterTag('')}>{t('premise.assets.clear_filter')}</button>} />
              : <EmptyState title={t('premise.assets.no_items')} description={t('premise.assets.no_items_body')} actions={<><button type="button" onClick={openBatchDialog}>{t('premise.tab.batches')}</button><button type="button" className="button-secondary" onClick={() => setUploadDialogOpen(true)}>{t('premise.add.upload.title')}</button></>} />
          ) : null}
          {displayedAssets.length ? (
            <div className="premise-card-grid">
              {displayedAssets.map((asset) => (
                <article className="premise-asset-tile" key={asset.uuid}>
                  <div className="premise-asset-tile__image">
                    <button type="button" className="premise-asset-tile__surface" onClick={() => openAssetDetail(asset)} aria-label={t('premise.assets.view_detail', { title: asset.title })}>
                      {asset.current_variant ? <ProductionImage projectUuid={projectUuid} asset={asset.current_variant.asset} alt={asset.title} /> : <span className="asset-card__placeholder">{t('premise.assets.no_image')}</span>}
                    </button>
                    <div className="premise-asset-tile__actions">
                      <button type="button" className="premise-icon-button" onClick={() => openChatScene('asset_reference', asset)} aria-label={t('premise.assets.reference_chat', { title: asset.title })} title={t('premise.assets.reference')}><MessageSquareQuote size={15} aria-hidden="true" /></button>
                      <button type="button" className="premise-icon-button" onClick={() => openAssetDetail(asset)} aria-label={t('premise.assets.history_label', { title: asset.title })} title={t('premise.assets.history')}><History size={15} aria-hidden="true" /></button>
                      <button type="button" className="premise-icon-button is-danger" disabled={trashMutation.isPending && trashMutation.variables?.uuid === asset.uuid} onClick={() => trashMutation.mutate(asset)} aria-label={t('premise.assets.trash_label', { title: asset.title })} title={t('story.chapter.trash')}><Trash2 size={15} aria-hidden="true" /></button>
                    </div>
                  </div>
                  <button type="button" className="premise-asset-tile__title" onClick={() => openAssetDetail(asset)}>
                    <strong>{asset.title}</strong><span>{ASSET_TYPE_COPY[asset.asset_type] ? t(ASSET_TYPE_COPY[asset.asset_type]) : t('common.status.unknown_with_code', { code: asset.asset_type })}</span>
                  </button>
                </article>
              ))}
            </div>
          ) : null}
        </section>
      ) : null}

      {activeTab === 'trash' ? (
        <section className="premise-content-panel" role="tabpanel" aria-busy={trashAssetsQuery.isLoading}>
          <header className="premise-content-heading premise-content-heading--actions"><div><div><h1>{t('projects.tab.trash')}</h1><span>{trashedAssets.length}</span></div><p>{t('premise.assets.trash_body')}</p></div><button type="button" className="button-secondary premise-danger-button" disabled={!trashedAssets.length || emptyTrashMutation.isPending || permanentDeleteMutation.isPending} onClick={() => setDeleteRequest({ mode: 'all' })}><Trash2 size={15} aria-hidden="true" />{t('premise.assets.empty_trash')}</button></header>
          {trashAssetsQuery.isLoading ? <LoadingCards /> : null}
          {!trashAssetsQuery.isLoading && trashAssetsQuery.isError ? <EmptyState title={t('premise.assets.trash_load_failed')} description={t('premise.assets.try_later')} actions={<button type="button" onClick={() => trashAssetsQuery.refetch()}>{t('common.action.reload')}</button>} /> : null}
          {!trashAssetsQuery.isLoading && !trashAssetsQuery.isError && !trashedAssets.length ? <EmptyState title={t('premise.assets.trash_empty')} description={t('premise.assets.trash_empty_body')} /> : null}
          {trashedAssets.length ? (
            <div className="premise-card-grid">
              {trashedAssets.map((asset) => (
                <article className="premise-asset-tile premise-asset-tile--trashed" key={asset.uuid}>
                  <div className="premise-asset-tile__image">
                    <button type="button" className="premise-asset-tile__surface" onClick={() => openAssetDetail(asset)} aria-label={t('premise.assets.view_detail', { title: asset.title })}>
                      {asset.current_variant ? <ProductionImage projectUuid={projectUuid} asset={asset.current_variant.asset} alt={asset.title} /> : <span className="asset-card__placeholder">{t('premise.assets.no_image')}</span>}
                    </button>
                    <div className="premise-asset-tile__actions"><button type="button" className="premise-icon-button" disabled={(restoreMutation.isPending && restoreMutation.variables?.uuid === asset.uuid) || permanentDeleteMutation.isPending || emptyTrashMutation.isPending} onClick={() => restoreMutation.mutate(asset)} aria-label={t('premise.assets.restore_label', { title: asset.title })} title={t('common.action.restore')}><RotateCcw size={15} aria-hidden="true" /></button><button type="button" className="premise-icon-button is-danger" disabled={restoreMutation.isPending || permanentDeleteMutation.isPending || emptyTrashMutation.isPending} onClick={() => setDeleteRequest({ mode: 'asset', asset })} aria-label={t('premise.assets.permanent_delete_label', { title: asset.title })} title={t('premise.assets.permanent_delete')}><Trash2 size={15} aria-hidden="true" /></button></div>
                  </div>
                  <button type="button" className="premise-asset-tile__title" onClick={() => openAssetDetail(asset)}><strong>{asset.title}</strong><span>{t('premise.assets.deleted')}</span></button>
                </article>
              ))}
            </div>
          ) : null}
        </section>
      ) : null}

      {activeTab === 'batches' ? (
        <section className="premise-content-panel premise-batches" role="tabpanel">
          <header className="premise-content-heading premise-content-heading--actions premise-content-heading--batch">
            <div><p className="premise-content-eyebrow">{t('premise.tab.batches')}</p><div><h1>{t('premise.batches.title')}</h1><span>{sourceTotal}</span></div><p>{t('premise.batches.description')}</p></div>
            <div><button type="button" className="button-secondary" aria-expanded={styleEditorOpen} onClick={() => setStyleEditorOpen((value) => !value)}>{t('premise.style.default')}</button><button type="button" onClick={openBatchDialog}><Sparkles size={15} aria-hidden="true" />{t('premise.batches.start')}</button></div>
          </header>
          {styleEditorOpen ? (
            <form className="premise-style-editor" onSubmit={(event) => { event.preventDefault(); styleMutation.mutate() }}>
              <label>{t('premise.style.project')}<textarea rows="4" value={style} onChange={(event) => setStyle(event.target.value)} placeholder={t('premise.style.placeholder')} /></label>
              <div><button type="button" className="button-secondary" onClick={() => setStyleEditorOpen(false)}>{t('common.action.cancel')}</button><button disabled={styleMutation.isPending}>{t(styleMutation.isPending ? 'common.status.saving' : 'premise.style.save')}</button></div>
            </form>
          ) : null}
          {sourcesQuery.isLoading ? <div className="premise-batch-loading">{t('premise.batches.loading')}</div> : null}
          {!sourcesQuery.isLoading && sourcesQuery.isError && !sources.length && !sourcesQuery.isFetchNextPageError ? <EmptyState title={t('premise.batches.load_failed')} description={t('premise.batches.load_failed_body')} actions={<button type="button" onClick={() => sourcesQuery.refetch()}>{t('common.action.retry')}</button>} /> : null}
          {!sourcesQuery.isLoading && !sourcesQuery.isError && !sources.length ? <EmptyState title={t('premise.batches.empty')} description={t('premise.batches.empty_body')} actions={<button type="button" onClick={openBatchDialog}>{t('premise.add.batch.title')}</button>} /> : null}
          <div className="premise-batch-list">
            {sources.map((source, index) => {
              const sourceSettings = settings.filter((setting) => setting.source_uuid === source.uuid)
              const sourceState = premiseSourceState(source, settings, tasks)
              const generating = Boolean(activeTaskFor(tasks, 'premise_setting_generation', source.uuid))
              const splitting = sourceSettings.some((setting) => Boolean(activeTaskFor(tasks, 'premise_asset_breakdown', setting.uuid)))
              const sourceBusy = generating || splitting
              const ignored = Boolean(source.ignored_at)
              const expanded = expandedSourceUuids.has(source.uuid)
              const ignorePending = sourceIgnoreMutation.isPending && sourceIgnoreMutation.variables?.source?.uuid === source.uuid
              return (
                <article className={`premise-batch-card${ignored ? ' is-ignored' : ''}${expanded ? ' is-expanded' : ''}`} key={source.uuid}>
                  <header>
                    <button type="button" className="premise-batch-card__toggle" aria-expanded={expanded} aria-controls={`premise-batch-detail-${source.uuid}`} onClick={() => toggleSourceDetails(source.uuid)}>
                      <ChevronRight size={15} aria-hidden="true" />
                      <span>{t('premise.batches.number', { number: sourceTotal - index })}</span>
                      <strong className={`premise-batch-status premise-batch-status--${sourceState}`}>{SOURCE_STATE_COPY[sourceState] ? t(SOURCE_STATE_COPY[sourceState]) : t('common.status.unknown_with_code', { code: sourceState })}</strong>
                    </button>
                    <time dateTime={source.created_at}>{formatDateTime(source.created_at)}</time>
                  </header>
                  <button type="button" className="premise-batch-card__summary" aria-expanded={expanded} onClick={() => toggleSourceDetails(source.uuid)}>{source.source_text}</button>
                  {source.style_snapshot ? <small>{t('premise.batches.style', { style: source.style_snapshot })}</small> : null}
                  <div className="premise-batch-card__actions">
                    <button type="button" className="button-secondary" disabled={ignored || sourceBusy} onClick={() => generateMutation.mutate(source)}>{t(sourceSettings.length ? 'premise.batches.regenerate' : 'premise.batches.generate')}</button>
                    <button
                      type="button"
                      className="button-quiet premise-batch-ignore"
                      disabled={ignorePending || (!ignored && (!sourceSettings.length || sourceBusy))}
                      title={!ignored && !sourceSettings.length ? t('premise.batches.ignore_requires_image') : !ignored && sourceBusy ? t('premise.batches.ignore_requires_idle') : undefined}
                      onClick={() => sourceIgnoreMutation.mutate({ source, ignored: !ignored })}
                    >
                      {ignored ? <RotateCcw size={14} aria-hidden="true" /> : <EyeOff size={14} aria-hidden="true" />}{t(ignored ? 'premise.batches.restore' : 'premise.batches.ignore')}
                    </button>
                    <button type="button" className="button-quiet" aria-expanded={expanded} onClick={() => toggleSourceDetails(source.uuid)}>{t(expanded ? 'premise.batches.collapse' : 'premise.batches.view')}</button>
                  </div>
                  {expanded ? (
                    <section className="premise-batch-detail" id={`premise-batch-detail-${source.uuid}`} aria-label={t('premise.batches.detail_label', { number: sourceTotal - index })}>
                      <dl>
                        <div><dt>{t('premise.batches.source_uuid')}</dt><dd><code>{source.uuid}</code></dd></div>
                        <div><dt>{t('common.label.source')}</dt><dd>{sourceTypeLabel(t, source.source_type || 'manual')}</dd></div>
                        <div><dt>{t('common.label.revision')}</dt><dd>{source.revision}</dd></div>
                        <div><dt>{t('premise.batches.candidates')}</dt><dd>{sourceSettings.length}</dd></div>
                        <div><dt>{t('settings.provider')}</dt><dd>{source.provider_uuid || t('premise.batches.default_provider')}</dd></div>
                        <div><dt>{t('projects.overview.model.text')}</dt><dd>{source.model || t('premise.batches.default_model')}</dd></div>
                        {source.ignored_at ? <div><dt>{t('premise.batches.ignored_at')}</dt><dd>{formatDateTime(source.ignored_at)}</dd></div> : null}
                      </dl>
                      <div className="premise-batch-detail__text"><strong>{t('premise.batches.full_description')}</strong><p data-user-content>{source.source_text}</p></div>
                      <details><summary>{t('premise.batches.parameters')}</summary><pre data-machine-value>{JSON.stringify(source.parameters || {}, null, 2)}</pre></details>
                      <ProductionTaskStrip projectUuid={projectUuid} tasks={tasks} resourceUuid={source.uuid} kind="premise_setting_generation" refresh={refresh} />
                      {sourceSettings.length ? (
                        <div className="premise-setting-grid">
                          {sourceSettings.map((setting) => (
                            <article className={setting.uuid === currentSettingUuid ? 'premise-setting-card is-current' : 'premise-setting-card'} key={setting.uuid}>
                              <ProductionImage projectUuid={projectUuid} asset={setting.asset} alt={setting.prompt || t('premise.batches.overview_alt')} />
                              <div><span>{t(setting.uuid === currentSettingUuid ? 'premise.batches.current_image' : 'premise.batches.candidate_image')}</span><small>{setting.prompt || t('premise.batches.prompt_missing')}</small><div><button type="button" className="button-secondary" aria-pressed={setting.uuid === currentSettingUuid} disabled={ignored} onClick={() => selectSetting.mutate(setting.uuid)}>{t(setting.uuid === currentSettingUuid ? 'premise.batches.selected' : 'premise.batches.select')}</button><button type="button" disabled={ignored || Boolean(activeTaskFor(tasks, 'premise_asset_breakdown', setting.uuid))} onClick={() => breakdownMutation.mutate(setting)}>{t('premise.batches.breakdown')}</button></div></div>
                              <ProductionTaskStrip projectUuid={projectUuid} tasks={tasks} resourceUuid={setting.uuid} kind="premise_asset_breakdown" refresh={refresh} />
                            </article>
                          ))}
                        </div>
                      ) : <p className="premise-batch-detail__empty">{t('premise.batches.no_image')}</p>}
                    </section>
                  ) : null}
                </article>
              )
            })}
          </div>
          {!sourcesQuery.isLoading && sources.length ? (
            <div className="premise-history-pagination">
              {sourcesQuery.isFetchNextPageError
                ? <button type="button" className="button-quiet" onClick={() => sourcesQuery.fetchNextPage()}>{t('premise.history.retry_more')}</button>
                : sourcesQuery.hasNextPage
                  ? <button type="button" className="button-secondary" disabled={sourcesQuery.isFetchingNextPage} onClick={() => sourcesQuery.fetchNextPage()}>{t(sourcesQuery.isFetchingNextPage ? 'premise.history.loading_more' : 'premise.history.load_more')}</button>
                  : <span>{t('premise.history.end', { count: sourceTotal })}</span>}
            </div>
          ) : null}
          <form className="premise-setting-import" onSubmit={(event) => { event.preventDefault(); settingImport.mutate() }}>
            <div><strong>{t('premise.batches.have_image')}</strong><span>{t('premise.batches.upload_body')}</span></div>
            <input type="file" accept="image/*" onChange={(event) => setSettingFile(event.target.files?.[0] || null)} />
            <button type="submit" className="button-secondary" disabled={!settingFile || settingImport.isPending || !premiseQuery.data?.current_source || premiseQuery.data?.current_source?.ignored_at}>{t(settingImport.isPending ? 'story.chapters.uploading' : 'premise.batches.upload')}</button>
          </form>
        </section>
      ) : null}

      {activeTab === 'threads' ? <PremiseThreadsPanel projectUuid={projectUuid} onOpenThread={(thread) => updateChatQuery({ threadUuid: thread.uuid })} onNewThread={() => updateChatQuery()} /> : null}
      {activeTab === 'prompts' ? <PremisePromptsPanel projectUuid={projectUuid} /> : null}
      {activeTab === 'llm_logs' ? <ProjectLLMLogsPanel projectUuid={projectUuid} scope="premise" title={t('premise.llm_logs.title')} description={t('premise.llm_logs.description')} /> : null}

      {dragActive ? <div className="premise-drop-overlay" aria-hidden="true"><Upload size={30} />{t('premise.drop')}</div> : null}

      {deleteRequest ? (
        <LumiDialog className="premise-delete-dialog" dismissDisabled={permanentDeleteMutation.isPending || emptyTrashMutation.isPending} onClose={() => setDeleteRequest(null)} aria-labelledby="premise-delete-title">
          <header className="lumi-dialog__header"><div><h2 id="premise-delete-title">{t(deleteRequest.mode === 'all' ? 'premise.assets.empty_confirm_title' : 'premise.assets.permanent_confirm_title')}</h2><p>{t(deleteRequest.mode === 'all' ? 'premise.assets.empty_confirm_body' : 'premise.assets.permanent_confirm_body', { title: deleteRequest.asset?.title || '' })}</p></div><button type="button" className="button-quiet" disabled={permanentDeleteMutation.isPending || emptyTrashMutation.isPending} aria-label={t('common.action.close')} onClick={() => setDeleteRequest(null)}><X size={18} aria-hidden="true" /></button></header>
          <div className="lumi-dialog__body"><p className="premise-delete-dialog__warning">{t('premise.assets.permanent_warning')}</p></div>
          <footer className="lumi-dialog__actions"><button type="button" className="button-secondary" disabled={permanentDeleteMutation.isPending || emptyTrashMutation.isPending} onClick={() => setDeleteRequest(null)}>{t('common.action.cancel')}</button><button type="button" className="premise-danger-button" disabled={permanentDeleteMutation.isPending || emptyTrashMutation.isPending} onClick={() => deleteRequest.mode === 'all' ? emptyTrashMutation.mutate() : permanentDeleteMutation.mutate(deleteRequest.asset)}>{t((permanentDeleteMutation.isPending || emptyTrashMutation.isPending) ? 'common.status.processing' : deleteRequest.mode === 'all' ? 'premise.assets.empty_trash' : 'premise.assets.permanent_delete')}</button></footer>
        </LumiDialog>
      ) : null}

      {batchDialogOpen ? (
        <div className="premise-dialog-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !batchMutation.isPending) setBatchDialogOpen(false) }}>
          <section className="premise-dialog premise-batch-dialog" role="dialog" aria-modal="true" aria-labelledby="premise-batch-title">
            <header><div><p>{t('premise.title')}</p><h2 id="premise-batch-title">{t('premise.add.batch.title')}</h2></div><button type="button" className="button-quiet" disabled={batchMutation.isPending} onClick={() => setBatchDialogOpen(false)} aria-label={t('common.action.close')}><X size={18} aria-hidden="true" /></button></header>
            <p className="premise-dialog__intro">{t('premise.batch_dialog.intro')}</p>
            <form onSubmit={(event) => { event.preventDefault(); batchMutation.mutate() }}>
              <label>{t('premise.batch_dialog.description')}<textarea rows="10" autoFocus value={batchText} onChange={(event) => setBatchText(event.target.value)} placeholder={t('premise.batch_dialog.placeholder')} required /></label>
              <div className="premise-story-import"><button type="button" className="button-secondary" disabled={storyProfileQuery.isLoading || !storyProfileQuery.data?.story_md} onClick={() => setBatchText(storyProfileQuery.data.story_md)}>{t(storyProfileQuery.isLoading ? 'story.story_file_loading' : 'premise.batch_dialog.import_story')}</button><span>{t(storyProfileQuery.data?.story_md ? 'premise.batch_dialog.overwrite' : 'premise.batch_dialog.story_empty')}</span></div>
              <details><summary>{t('premise.batch_dialog.style')}</summary><textarea rows="4" value={batchStyle} onChange={(event) => setBatchStyle(event.target.value)} placeholder={t('premise.batch_dialog.style_placeholder')} /></details>
              <footer><button type="button" className="button-secondary" disabled={batchMutation.isPending} onClick={() => setBatchDialogOpen(false)}>{t('common.action.cancel')}</button><button type="submit" disabled={!batchText.trim() || batchMutation.isPending}>{t(batchMutation.isPending ? 'projects.exports.create_task' : 'premise.batches.generate')}</button></footer>
            </form>
          </section>
        </div>
      ) : null}

      {uploadDialogOpen ? (
        <div className="premise-dialog-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) closeUploadDialog() }}>
          <section className="premise-dialog premise-upload-dialog" role="dialog" aria-modal="true" aria-labelledby="premise-upload-title">
            <header><div><p>{t('premise.title')}</p><h2 id="premise-upload-title">{t('premise.add.upload.title')}</h2></div><button type="button" className="button-quiet" disabled={uploadMutation.isPending} onClick={closeUploadDialog} aria-label={t('common.action.close')}><X size={18} aria-hidden="true" /></button></header>
            <button type="button" className="premise-upload-dropzone" disabled={uploadMutation.isPending} onClick={() => fileInputRef.current?.click()}><Upload size={24} aria-hidden="true" /><strong>{t('premise.upload.add_more')}</strong><span>{t('premise.upload.add_more_body')}</span></button>
            <form onSubmit={(event) => { event.preventDefault(); uploadMutation.mutate() }}>
              <div className="premise-upload-list">
                {uploadDrafts.map((draft, index) => (
                  <fieldset key={draft.id} disabled={uploadMutation.isPending}>
                    <legend>{t('premise.upload.item', { number: index + 1 })}</legend>
                    <img src={draft.previewUrl} alt={t('premise.upload.preview')} />
                    <div className="premise-upload-fields">
                      <label>{t('premise.upload.type')}<select value={draft.assetType} onChange={(event) => updateUploadDraft(draft.id, 'assetType', event.target.value)}>{Object.entries(ASSET_TYPE_COPY).map(([value, key]) => <option value={value} key={value}>{t(key)}</option>)}</select></label>
                      <label>{t('common.label.title')}<input value={draft.title} onChange={(event) => updateUploadDraft(draft.id, 'title', event.target.value)} required /></label>
                      <label className="is-wide">{t('premise.upload.summary')}<input value={draft.summary} onChange={(event) => updateUploadDraft(draft.id, 'summary', event.target.value)} placeholder={t('common.label.optional')} /></label>
                      <label className="is-wide">{t('premise.upload.tags')}<input value={draft.tags} onChange={(event) => updateUploadDraft(draft.id, 'tags', event.target.value)} placeholder={t('premise.upload.tags_placeholder')} /></label>
                      {tags.length ? <div className="premise-upload-tags is-wide">{tags.slice(0, 10).map((tag) => <button type="button" className="premise-upload-tag" key={tag} onClick={() => appendDraftTag(draft.id, tag)}>#{tag}</button>)}</div> : null}
                    </div>
                    <button type="button" className="premise-upload-remove" onClick={() => removeUploadDraft(draft.id)} aria-label={t('premise.upload.remove', { number: index + 1 })}><X size={15} aria-hidden="true" /></button>
                  </fieldset>
                ))}
              </div>
              <footer><span>{t(uploadMutation.isPending ? 'premise.upload.progress' : 'premise.upload.total', uploadMutation.isPending ? { current: Math.min(uploadProgress + 1, uploadDrafts.length), total: uploadDrafts.length } : { count: uploadDrafts.length })}</span><div><button type="button" className="button-secondary" disabled={uploadMutation.isPending} onClick={closeUploadDialog}>{t('common.action.cancel')}</button><button type="submit" disabled={!uploadDrafts.length || uploadDrafts.some((draft) => !draft.title.trim()) || uploadMutation.isPending}>{t(uploadMutation.isPending ? 'story.chapters.uploading' : 'premise.upload.submit', { count: uploadDrafts.length })}</button></div></footer>
            </form>
          </section>
        </div>
      ) : null}

      {historyAsset && detailDraft ? (
        <div className="premise-dialog-backdrop premise-dialog-backdrop--detail" onMouseDown={(event) => { if (event.target === event.currentTarget) closeAssetDetail() }}>
          <section className="premise-dialog premise-detail-dialog" role="dialog" aria-modal="true" aria-labelledby="premise-detail-title">
            <header><div><p>{t('premise.detail.title')}</p><h2 id="premise-detail-title">{historyAsset.title}</h2></div><div className="premise-detail-header-actions"><button type="button" className="button-secondary" onClick={() => openChatScene('asset_reference', historyAsset)}><MessageSquareQuote size={15} aria-hidden="true" />{t('premise.assets.reference')}</button><button type="button" className="button-quiet" onClick={closeAssetDetail} aria-label={t('common.action.close')}><X size={18} aria-hidden="true" /></button></div></header>
            <div className="premise-detail-layout">
              <div className="premise-detail-preview">{historyAsset.current_variant ? <ProductionImage projectUuid={projectUuid} asset={historyAsset.current_variant.asset} alt={historyAsset.title} profile="detail_1024" /> : <div className="asset-card__placeholder">{t('premise.assets.no_image')}</div>}</div>
              <form className="premise-detail-form" onSubmit={(event) => { event.preventDefault(); updateAsset.mutate() }}>
                <label>{t('premise.upload.type')}<select value={detailDraft.assetType} onChange={(event) => setDetailDraft((current) => ({ ...current, assetType: event.target.value }))}>{Object.entries(ASSET_TYPE_COPY).map(([value, key]) => <option value={value} key={value}>{t(key)}</option>)}</select></label>
                <label>{t('common.label.title')}<input value={detailDraft.title} onChange={(event) => setDetailDraft((current) => ({ ...current, title: event.target.value }))} required /></label>
                <label>{t('premise.upload.summary')}<textarea rows="4" value={detailDraft.summary} onChange={(event) => setDetailDraft((current) => ({ ...current, summary: event.target.value }))} /></label>
                <label>{t('premise.upload.tags')}<input value={detailDraft.tags} onChange={(event) => setDetailDraft((current) => ({ ...current, tags: event.target.value }))} placeholder={t('premise.upload.tags_placeholder')} /></label>
                <button type="submit" disabled={!detailDraft.title.trim() || updateAsset.isPending}>{t(updateAsset.isPending ? 'common.status.saving' : 'premise.detail.save')}</button>
              </form>
            </div>
            <section className="premise-variant-section">
              <header><div><h3>{t('premise.detail.versions')}</h3><span>{variantsQuery.data?.items?.length || 0}</span></div><label className="button-secondary">{t('premise.detail.upload_version')}<input type="file" accept="image/*" onChange={(event) => setReplacementFile(event.target.files?.[0] || null)} /></label><button type="button" disabled={!replacementFile || replaceImage.isPending} onClick={() => replaceImage.mutate()}>{t(replaceImage.isPending ? 'story.chapters.uploading' : 'premise.detail.make_version')}</button></header>
              {variantsQuery.isLoading ? <p>{t('premise.detail.loading_versions')}</p> : null}
              <div>{variantsQuery.data?.items?.map((variant) => <article key={variant.uuid}><ProductionImage projectUuid={projectUuid} asset={variant.asset} alt={`${historyAsset.title} v${variant.version_no}`} /><strong>v{variant.version_no}</strong><small>{sourceTypeLabel(t, variant.source_type)}</small><button type="button" aria-pressed={historyAsset.current_variant?.uuid === variant.uuid} disabled={historyAsset.current_variant?.uuid === variant.uuid || selectVariant.isPending} onClick={() => selectVariant.mutate(variant)}>{t(historyAsset.current_variant?.uuid === variant.uuid ? 'story.prompts.current' : 'premise.detail.restore_current')}</button></article>)}</div>
            </section>
          </section>
        </div>
      ) : null}
    </div>
  )
}
