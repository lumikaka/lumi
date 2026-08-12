import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, History, RotateCcw, Save, Sparkles, X } from 'lucide-react'

import { createPromptVersion, listPromptCatalog, listPromptVersions, restorePromptVersion } from '../api/story.js'
import LumiDialog from './LumiDialog.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { sourceTypeLabel } from '../i18n/labels.js'
import { useI18n } from '../i18n/useI18n.js'
import {
  applyOverallStylePresetDraft,
  normalizedPrompt,
  promptIdentity,
  promptIssues,
  promptServerValues,
  promptUpdatePayload,
  reconcilePromptDrafts,
  resetPromptGroupDrafts,
} from './promptCatalogState.js'

const groupOrder = ['story', 'premise', 'chapter', 'premise_style', 'agent', 'runtime']

function PromptDialog({ children, dismissDisabled = false, onClose }) {
  return (
    <LumiDialog className="prompt-candidates-dialog" dismissDisabled={dismissDisabled} onClose={onClose}>
      {children}
    </LumiDialog>
  )
}

function PromptCandidatesDialog({ projectUuid, definition, draft, onClose, onRestored }) {
  const { formatDateTime, t } = useI18n()
  const [page, setPage] = useState(1)
  const [error, setError] = useState(null)
  const versionsQuery = useQuery({
    queryKey: ['prompt-versions', projectUuid, definition.prompt_group, definition.prompt_key, page],
    queryFn: () => listPromptVersions(projectUuid, { promptGroup: definition.prompt_group, promptKey: definition.prompt_key, page, perPage: 10 }),
  })
  const versions = versionsQuery.data?.items || []
  const pagination = versionsQuery.data?.pagination
  const restoreMutation = useMutation({
    mutationFn: (version) => restorePromptVersion(projectUuid, version.uuid, definition.current_version?.version_no || 0),
    onSuccess: (version) => {
      setError(null)
      onRestored(version.prompt)
    },
    onError: setError,
  })
  const restore = (version) => {
    const dirty = normalizedPrompt(draft) !== normalizedPrompt(definition.effective_value)
    if (dirty && !window.confirm(t('story.prompts.restore_discard_draft'))) return
    restoreMutation.mutate(version)
  }

  return (
    <PromptDialog dismissDisabled={restoreMutation.isPending} onClose={onClose}>
      <header className="lumi-dialog__header">
        <div><h2>{definition.title || definition.prompt_key}</h2><p>{t('story.prompts.candidates')}</p></div>
        <button type="button" className="button-quiet" disabled={restoreMutation.isPending} aria-label={t('common.action.close')} onClick={onClose}><X size={18} aria-hidden="true" /></button>
      </header>
      <div className="lumi-dialog__body prompt-candidates-dialog__body">
        <LocalizedErrorMessage error={error || versionsQuery.error} onDismiss={error ? () => setError(null) : undefined} />
        {versionsQuery.isLoading ? <p className="prompt-candidates-dialog__loading">{t('story.prompts.loading')}</p> : null}
        <div className="prompt-candidate-list">
          {versions.map((version) => {
            const current = version.uuid === definition.current_version?.uuid
            return (
              <article key={version.uuid} className={current ? 'is-current' : ''}>
                <header><div><strong>v{version.version_no}</strong><span>{sourceTypeLabel(t, version.source_type)}</span></div><time dateTime={version.created_at}>{formatDateTime(version.created_at)}</time></header>
                <pre data-user-content>{version.prompt}</pre>
                <footer><code>{version.prompt_hash.slice(0, 12)}</code>{current ? <span>{t('story.prompts.current')}</span> : <button type="button" className="button-secondary" disabled={restoreMutation.isPending} onClick={() => restore(version)}>{t('story.prompts.restore')}</button>}</footer>
              </article>
            )
          })}
        </div>
        {!versionsQuery.isLoading && versions.length === 0 ? <div className="workspace-empty"><h2>{t('story.prompts.candidates_empty')}</h2></div> : null}
        {pagination && pagination.last_page > 1 ? (
          <footer className="prompt-candidates-dialog__pagination">
            <button type="button" className="button-secondary" disabled={page <= 1 || versionsQuery.isFetching} onClick={() => setPage((current) => current - 1)}><ChevronLeft size={15} aria-hidden="true" />{t('common.action.previous_page')}</button>
            <span>{t('story.prompts.page', { current: pagination.current_page, total: pagination.last_page })}</span>
            <button type="button" className="button-secondary" disabled={page >= pagination.last_page || versionsQuery.isFetching} onClick={() => setPage((current) => current + 1)}>{t('common.action.next_page')}<ChevronRight size={15} aria-hidden="true" /></button>
          </footer>
        ) : null}
      </div>
    </PromptDialog>
  )
}

function PromptCard({ projectUuid, definition, draft, setDraft, applyPreset, openCandidates }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [error, setError] = useState(null)
  const issues = promptIssues(definition, draft)
  const dirty = normalizedPrompt(draft) !== normalizedPrompt(definition.effective_value)
  const invalid = !normalizedPrompt(draft) || issues.unknown.length > 0
  const isDefault = normalizedPrompt(draft) === normalizedPrompt(definition.default_value)
  const isPreset = definition.prompt_type === 'preset'
  const saveMutation = useMutation({
    mutationFn: () => createPromptVersion(projectUuid, promptUpdatePayload(definition, draft)),
    onSuccess: () => {
      setError(null)
      queryClient.invalidateQueries({ queryKey: ['prompt-catalog', projectUuid] })
      queryClient.invalidateQueries({ queryKey: ['prompt-versions', projectUuid, definition.prompt_group, definition.prompt_key] })
      if (definition.prompt_group === 'premise_style' && definition.prompt_key === 'project_overall_style') queryClient.invalidateQueries({ queryKey: ['premise', projectUuid] })
    },
    onError: setError,
  })

  return (
    <article className="prompt-card" data-prompt-key={definition.prompt_key}>
      <header className="prompt-card__header">
        <div className="prompt-card__identity">
          <div><h3>{definition.title || definition.prompt_key}</h3><span className={isDefault ? 'prompt-status-pill' : 'prompt-status-pill prompt-status-pill--modified'}>{t(isDefault ? 'story.prompts.default' : 'story.prompts.modified')}</span><span className={`prompt-type-pill prompt-type-pill--${definition.prompt_type || 'template'}`}>{t(`story.prompts.type.${definition.prompt_type || 'template'}`)}</span></div>
          <code>{definition.prompt_group}/{definition.prompt_key}</code>
        </div>
        <div className="prompt-card__actions">
          {isPreset ? <button type="button" className="button-secondary" onClick={() => applyPreset(definition)}><Sparkles size={14} aria-hidden="true" />{t('story.prompts.apply_style')}</button> : null}
          <button type="button" className="button-secondary" onClick={() => openCandidates(definition)}><History size={14} aria-hidden="true" />{t('story.prompts.candidates')}</button>
          <button type="button" className="button-secondary" disabled={isDefault || saveMutation.isPending} onClick={() => setDraft(definition, definition.default_value || '')}><RotateCcw size={14} aria-hidden="true" />{t('story.prompts.restore_default')}</button>
          <button type="button" disabled={!dirty || invalid || saveMutation.isPending} onClick={() => saveMutation.mutate()}><Save size={14} aria-hidden="true" />{t(saveMutation.isPending ? 'common.status.saving' : 'common.action.save')}</button>
        </div>
      </header>
      <LocalizedErrorMessage error={error} onDismiss={error ? () => setError(null) : undefined} />
      <p className="prompt-card__description">{definition.description}</p>
      <textarea rows={definition.prompt_key === 'json_system' ? 5 : 12} value={draft} onChange={(event) => setDraft(definition, event.target.value)} aria-label={`${definition.title || definition.prompt_key} ${t('projects.tab.prompts')}`} />
      <footer className="prompt-card__footer">
        <div className="prompt-card__meta"><span>{t('story.prompts.current_version', { version: definition.current_version?.version_no || 0 })}</span><span>{t('story.prompts.placeholder_note_short', { placeholders: definition.placeholders?.length ? definition.placeholders.map((item) => `{{${item}}}`).join(', ') : t('common.label.none') })}</span></div>
        {issues.missing.length ? <p className="prompt-card__warning">{t('story.prompts.missing_placeholders', { placeholders: issues.missing.map((item) => `{{${item}}}`).join(', ') })}</p> : null}
        {issues.unknown.length ? <p className="prompt-card__error">{t('story.prompts.unknown_placeholders', { placeholders: issues.unknown.map((item) => `{{${item}}}`).join(', ') })}</p> : null}
        {definition.prompt_group === 'chapter' && definition.prompt_key === 'section_image' && issues.missing.includes('before_image_prompt') ? <p className="prompt-card__warning">{t('story.prompts.before_image_detached')}</p> : null}
      </footer>
    </article>
  )
}

function PromptGroupSection({ projectUuid, group, definitions, drafts, setDraft, resetGroup, applyPreset, openCandidates }) {
  const { t } = useI18n()

  return (
    <section className="prompt-group" data-prompt-group={group}>
      <header className="prompt-group__header">
        <div><div><h2>{t(`story.prompts.group.${group}`)}</h2><span>{definitions.length}</span></div><p>{t(`story.prompts.group.${group}.description`)}</p></div>
        <div className="prompt-group__actions">
          <button type="button" className="button-secondary" onClick={() => resetGroup(definitions)}><RotateCcw size={14} aria-hidden="true" />{t('story.prompts.restore_group_default')}</button>
        </div>
      </header>
      <div className="prompt-group__list">
        {definitions.map((definition) => {
          const identity = promptIdentity(definition)
          const draft = drafts[identity] ?? definition.effective_value ?? ''
          return (
            <PromptCard key={identity} projectUuid={projectUuid} definition={definition} draft={draft} setDraft={setDraft} applyPreset={applyPreset} openCandidates={openCandidates} />
          )
        })}
      </div>
    </section>
  )
}

export default function PromptCatalogEditor({ projectUuid, groups = groupOrder, showHeader = true, className = '' }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [drafts, setDrafts] = useState({})
  const [candidate, setCandidate] = useState(null)
  const serverValues = useRef({})
  const catalogQuery = useQuery({ queryKey: ['prompt-catalog', projectUuid], queryFn: () => listPromptCatalog(projectUuid) })
  const catalog = catalogQuery.data?.items || []
  const visibleGroups = groupOrder.filter((group) => groups.includes(group))
  const visibleCatalog = catalog.filter((definition) => visibleGroups.includes(definition.prompt_group))

  useEffect(() => {
    if (!catalog.length) return
    const previousServerValues = serverValues.current
    const nextServerValues = promptServerValues(catalog)
    setDrafts((current) => reconcilePromptDrafts(current, previousServerValues, catalog))
    serverValues.current = nextServerValues
  }, [catalogQuery.data])

  const grouped = useMemo(() => Object.fromEntries(visibleGroups.map((group) => [group, visibleCatalog.filter((definition) => definition.prompt_group === group)])), [visibleCatalog, visibleGroups])
  const setDraft = (definition, value) => setDrafts((current) => ({ ...current, [promptIdentity(definition)]: value }))
  const resetGroup = (definitions) => setDrafts((current) => resetPromptGroupDrafts(current, definitions))
  const applyPreset = (definition) => {
    const target = catalog.find((item) => item.prompt_group === 'premise_style' && item.prompt_key === 'project_overall_style')
    if (!target) return
    setDrafts((current) => applyOverallStylePresetDraft(current, definition, target))
  }
  const restored = (value) => {
    if (!candidate) return
    setDraft(candidate, value)
    queryClient.invalidateQueries({ queryKey: ['prompt-catalog', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['prompt-versions', projectUuid, candidate.prompt_group, candidate.prompt_key] })
    if (candidate.prompt_group === 'premise_style' && candidate.prompt_key === 'project_overall_style') queryClient.invalidateQueries({ queryKey: ['premise', projectUuid] })
    setCandidate(null)
  }

  return (
    <div className={`prompt-catalog-editor ${className}`.trim()}>
      {showHeader ? <header className="prompt-catalog-editor__intro"><div><h1>{t('projects.tab.prompts')}</h1><p>{t('story.prompts.description')}</p></div><span>{visibleCatalog.length}</span></header> : null}
      <LocalizedErrorMessage error={catalogQuery.error} />
      {catalogQuery.isLoading ? <div className="prompt-catalog-editor__loading" aria-busy="true">{t('story.prompts.loading_catalog')}</div> : null}
      {!catalogQuery.isLoading && visibleGroups.map((group) => grouped[group]?.length ? <PromptGroupSection key={group} projectUuid={projectUuid} group={group} definitions={grouped[group]} drafts={drafts} setDraft={setDraft} resetGroup={resetGroup} applyPreset={applyPreset} openCandidates={setCandidate} /> : null)}
      {candidate ? <PromptCandidatesDialog projectUuid={projectUuid} definition={catalog.find((item) => promptIdentity(item) === promptIdentity(candidate)) || candidate} draft={drafts[promptIdentity(candidate)]} onClose={() => setCandidate(null)} onRestored={restored} /> : null}
    </div>
  )
}
