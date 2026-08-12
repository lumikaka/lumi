import { useCallback, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'

import { getProjectModelSettings, updateProjectModelSettings } from '../api/ai.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { useProjectRealtime } from '../realtime/useProjectRealtime.js'
import { INHERIT_MODEL_VALUE, modelOptionsForSetting, modelSelectionValue, parseModelSelection } from '../pages/modelSettingsState.js'

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

export default function ProjectModelSettingsCard({ projectUuid }) {
  const { t } = useI18n()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [error, setError] = useState(null)
  const queryKey = ['project-model-settings', projectUuid]
  const settingsQuery = useQuery({ queryKey, queryFn: () => getProjectModelSettings(projectUuid), retry: false })
  const refresh = useCallback(() => queryClient.invalidateQueries({ queryKey }), [queryClient, projectUuid])

  useProjectRealtime(projectUuid, useCallback((event) => {
    if (event === 'project:model_settings_changed' || event === 'phx_reconnected') refresh()
  }, [refresh]))

  const update = useMutation({
    mutationFn: ({ key, selection }) => updateProjectModelSettings(projectUuid, settingsQuery.data.revision, { [key]: selection }),
    onSuccess: (value) => {
      queryClient.setQueryData(queryKey, value)
      queryClient.invalidateQueries({ queryKey: ['project-image-generation-preflight', projectUuid] })
      setError(null)
    },
    onError: setError,
  })

  const settings = settingsQuery.data
  return (
    <section className="overview-card overview-model-card">
      <header className="overview-card__header">
        <div><h2>{t('projects.overview.model.title')}</h2><p>{t('projects.overview.model.body')}</p></div>
        <Link className="overview-card__action" to="/settings/providers" state={{ from: `${location.pathname}${location.search}` }}>{t('projects.overview.model.manage')}</Link>
      </header>
      <LocalizedErrorMessage error={error || settingsQuery.error} onDismiss={error ? () => setError(null) : undefined} />
      {settingsQuery.isLoading ? <p className="overview-card__loading">{t('projects.overview.model.loading')}</p> : null}
      {settings ? (
        <div className="overview-model-settings">
          {definitions.map(([key, labelKey]) => {
            const setting = settings.settings?.[key]
            if (!setting) return null
            const options = modelOptionsForSetting(settings, setting)
            const value = modelSelectionValue(setting.override)
            const invalid = setting.override_status === 'invalid'
            return (
              <article className={`overview-model-setting ${invalid ? 'is-invalid' : ''}`} key={key}>
                <div className="overview-model-setting__title">
                  <div><h3>{t(labelKey)}</h3><span>{t(`projects.overview.model.source.${setting.source || 'unavailable'}`)}</span></div>
                  {setting.override ? <button type="button" className="button-quiet" disabled={update.isPending} onClick={() => update.mutate({ key, selection: null })}>{t('projects.overview.model.restore_inheritance')}</button> : null}
                </div>
                <label>
                  <span>{t('projects.overview.model.override')}</span>
                  <select aria-label={t('projects.overview.model.select_label', { setting: t(labelKey) })} value={value} disabled={update.isPending} onChange={(event) => update.mutate({ key, selection: parseModelSelection(event.target.value) })}>
                    <option value={INHERIT_MODEL_VALUE}>{t('projects.overview.model.inherit')}</option>
                    {invalid ? <option value={value}>{t('projects.overview.model.invalid_saved', { model: setting.override?.model || '—' })}</option> : null}
                    {options.map((option) => <option value={modelSelectionValue(option)} key={`${option.provider_uuid}:${option.model}`}>{optionLabel(option)}</option>)}
                  </select>
                </label>
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
