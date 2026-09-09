import { useId, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'

import { getModelSettings, updateModelSettings, getProjectModelSettings, updateProjectModelSettings } from '../api/ai.js'
import { invalidateModelSettingsQueries } from '../realtime/modelSettingsQueries.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { INHERIT_MODEL_VALUE, modelOptionsForSetting, modelSelectionValue, parseModelSelection, imageThinkingSelection, imagePromptExtendSelection, imageThinkingEnabled } from '../pages/modelSettingsState.js'

const UNAVAILABLE_SOURCE = 'unavailable'

const definitions = [
  ['project_text', 'projects.overview.model.project_text'],
  ['project_image', 'projects.overview.model.project_image'],
  ['chat_area', 'projects.overview.model.chat_area'],
  ['story_text', 'projects.overview.model.story_text'],
  ['section_premise_selection', 'projects.overview.model.section_selection'],
]

function optionLabel(option) {
  return `${option.provider_name} · ${option.model}`
}

function effectiveLabel(settings, selection) {
  if (!selection) return '—'
  const options = [...(settings.options?.text_models || []), ...(settings.options?.image_models || [])]
  const option = options.find((item) => item.provider_uuid === selection.provider_uuid && item.model === selection.model)
  return option ? optionLabel(option) : selection.model
}

export default function ModelSettingsCard({ projectUuid }) {
  const global = !projectUuid
  const helpId = useId()
  const { t } = useI18n()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [error, setError] = useState(null)
  const queryKey = global ? ['model-settings'] : ['project-model-settings', projectUuid]
  const settingsQuery = useQuery({ queryKey, queryFn: () => global ? getModelSettings() : getProjectModelSettings(projectUuid), retry: false })
  const update = useMutation({
    mutationFn: ({ key, selection }) => global
      ? updateModelSettings(settingsQuery.data.revision, { [key]: selection })
      : updateProjectModelSettings(projectUuid, settingsQuery.data.revision, { [key]: selection }),
    onSuccess: (value) => {
      queryClient.setQueryData(queryKey, value)
      if (global) invalidateModelSettingsQueries(queryClient)
      else queryClient.invalidateQueries({ queryKey: ['project-image-generation-preflight', projectUuid] })
      setError(null)
    },
    onError: (requestError) => {
      setError(requestError)
      queryClient.invalidateQueries({ queryKey })
    },
  })

  const settings = settingsQuery.data
  return (
    <section className="overview-card overview-model-card" aria-labelledby={`${helpId}-title`}>
      <header className="overview-card__header">
        <div><h2 id={`${helpId}-title`}>{t(global ? 'settings.models.title' : 'projects.overview.model.title')}</h2><p>{t(global ? 'settings.models.body' : 'projects.overview.model.body')}</p></div>
        {!global ? <Link className="overview-card__action" to="/settings/providers" state={{ from: `${location.pathname}${location.search}` }}>{t('projects.overview.model.manage')}</Link> : null}
      </header>
      <LocalizedErrorMessage error={error || settingsQuery.error} onDismiss={error ? () => setError(null) : undefined} />
      {settingsQuery.isLoading ? <p className="overview-card__loading">{t(global ? 'settings.models.loading' : 'projects.overview.model.loading')}</p> : null}
      {settings ? (
        <div className="overview-model-settings" aria-busy={update.isPending}>
          {definitions.map(([key, labelKey]) => {
            const setting = settings.settings?.[key]
            if (!setting) return null
            const options = modelOptionsForSetting(settings, setting)
            const value = modelSelectionValue(setting.override)
            const source = setting.effective ? setting.source : UNAVAILABLE_SOURCE
            const invalid = setting.override_status === 'invalid'
            const thinkingSelection = imageThinkingSelection(settings, setting)
            const promptExtendSelection = imagePromptExtendSelection(settings, setting)
            const thinkingUnavailable = thinkingSelection?.prompt_extend === false
            return (
              <article className={`overview-model-setting ${invalid ? 'is-invalid' : ''}`} key={key}>
                <div className="overview-model-setting__title">
                  <div><h3>{t(labelKey)}</h3><span>{t(`projects.overview.model.source.${source}`)}</span></div>
                  {setting.override ? <button type="button" className="button-quiet" disabled={update.isPending} onClick={() => update.mutate({ key, selection: null })}>{t('projects.overview.model.restore_inheritance')}</button> : null}
                </div>
                <label>
                  <span>{t(global ? 'settings.models.selection' : 'projects.overview.model.override')}</span>
                  <select aria-label={t('projects.overview.model.select_label', { setting: t(labelKey) })} value={value} disabled={update.isPending} onChange={(event) => update.mutate({ key, selection: parseModelSelection(event.target.value) })}>
                    <option value={INHERIT_MODEL_VALUE}>{t('projects.overview.model.inherit')}</option>
                    {invalid ? <option value={value}>{t('projects.overview.model.invalid_saved', { model: setting.override?.model || '—' })}</option> : null}
                    {options.map((option) => <option value={modelSelectionValue(option)} key={`${option.provider_uuid}:${option.model}`}>{optionLabel(option)}</option>)}
                  </select>
                </label>
                {promptExtendSelection ? (
                  <div className="overview-model-setting__image-option">
                    <label>
                      <input type="checkbox" role="switch" checked={promptExtendSelection.prompt_extend !== false} disabled={update.isPending} aria-describedby={`${helpId}-prompt-extend`} onChange={(event) => update.mutate({ key, selection: { ...promptExtendSelection, prompt_extend: event.target.checked } })} />
                      <span>{t('projects.overview.model.prompt_extend')}</span>
                    </label>
                    <p id={`${helpId}-prompt-extend`}>{t('projects.overview.model.prompt_extend_help')}</p>
                  </div>
                ) : null}
                {thinkingSelection ? (
                  <div className="overview-model-setting__image-option">
                    <label>
                      <input type="checkbox" role="switch" checked={imageThinkingEnabled(thinkingSelection)} disabled={update.isPending || thinkingUnavailable} aria-describedby={`${helpId}-thinking`} onChange={(event) => update.mutate({ key, selection: { ...thinkingSelection, enable_thinking: event.target.checked } })} />
                      <span>{t('projects.overview.model.enable_thinking')}</span>
                    </label>
                    <p id={`${helpId}-thinking`}>{t(thinkingUnavailable ? 'projects.overview.model.thinking_requires_prompt_extend' : global ? 'settings.models.thinking_help' : 'projects.overview.model.thinking_help')}</p>
                  </div>
                ) : null}
                <dl>
                  <div><dt>{t('projects.overview.model.inherited')}</dt><dd>{effectiveLabel(settings, setting.inherited)}</dd></div>
                  <div><dt>{t('projects.overview.model.effective')}</dt><dd>{effectiveLabel(settings, setting.effective)}</dd></div>
                </dl>
                {invalid ? <p className="overview-model-setting__warning">{t('projects.overview.model.invalid_help')}</p> : null}
              </article>
            )
          })}
        </div>
      ) : null}
    </section>
  )
}
