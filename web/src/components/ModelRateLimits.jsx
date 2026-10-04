import { useId, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'

import { updateModelRateLimits } from '../api/ai.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { invalidateModelSettingsQueries } from '../realtime/modelSettingsQueries.js'

function uniqueOptions(settings) {
  const seen = new Set()
  return [...(settings.options?.text_models || []), ...(settings.options?.image_models || [])].filter((option) => {
    const key = JSON.stringify([option.provider_uuid, option.model])
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function optionKey(option, scope) {
  return scope === 'provider' ? option.provider_uuid : JSON.stringify([option.provider_uuid, option.model])
}

function LimitEditor({ providerOptions, modelOptions, initial, pending, onCancel, onSave, t }) {
  const [scope, setScope] = useState(initial?.scope || (providerOptions.length ? 'provider' : 'model'))
  const [selected, setSelected] = useState('')
  const [rpm, setRpm] = useState(initial?.rpm == null ? '' : String(initial.rpm))
  const [rps, setRps] = useState(initial?.rps == null ? '' : String(initial.rps))
  const [validation, setValidation] = useState('')
  const options = scope === 'provider' ? providerOptions : modelOptions
  const option = initial || options.find((item) => optionKey(item, scope) === selected) || options[0]

  const submit = (event) => {
    event.preventDefault()
    const parsedRpm = rpm === '' ? null : Number(rpm)
    const parsedRps = rps === '' ? null : Number(rps)
    if (!option || (parsedRpm == null && parsedRps == null) || [parsedRpm, parsedRps].some((value) => value != null && (!Number.isSafeInteger(value) || value <= 0))) {
      setValidation(t('settings.models.rate_limits.validation'))
      return
    }
    setValidation('')
    onSave({ scope, provider_uuid: option.provider_uuid, ...(scope === 'model' ? { model: option.model } : {}), rpm: parsedRpm, rps: parsedRps })
  }

  return <form className="overview-model-limits__editor" onSubmit={submit} aria-busy={pending}>
    {initial ? <strong>{initial.provider_name} · {initial.scope === 'provider' ? t('settings.models.rate_limits.provider_pool') : initial.model}</strong> : <>
      <label>{t('settings.models.rate_limits.scope')}<select value={scope} onChange={(event) => { setScope(event.target.value); setSelected('') }}>
        <option value="provider" disabled={!providerOptions.length}>{t('settings.models.rate_limits.provider_pool')}</option>
        <option value="model" disabled={!modelOptions.length}>{t('settings.models.rate_limits.model_pool')}</option>
      </select></label>
      <label>{t(scope === 'provider' ? 'settings.models.rate_limits.provider' : 'settings.models.rate_limits.model')}<select value={option ? optionKey(option, scope) : ''} onChange={(event) => setSelected(event.target.value)}>{options.map((item) => <option value={optionKey(item, scope)} key={optionKey(item, scope)}>{item.provider_name}{scope === 'model' ? ` · ${item.model}` : ''}</option>)}</select></label>
    </>}
    <label>{t('settings.models.rate_limits.rpm')}<input type="number" min="1" max="9007199254740991" step="1" value={rpm} onChange={(event) => setRpm(event.target.value)} /></label>
    <label>{t('settings.models.rate_limits.rps')}<input type="number" min="1" max="9007199254740991" step="1" value={rps} onChange={(event) => setRps(event.target.value)} /></label>
    {validation ? <p className="overview-model-limits__validation" role="alert">{validation}</p> : null}
    <div className="overview-model-limits__actions"><button type="button" className="button-secondary" disabled={pending} onClick={onCancel}>{t('common.action.cancel')}</button><button type="submit" disabled={pending || !option}>{t('common.action.save')}</button></div>
  </form>
}

export default function ModelRateLimits({ settings }) {
  const titleId = useId()
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [editor, setEditor] = useState(null)
  const [error, setError] = useState(null)
  const options = uniqueOptions(settings)
  const providers = options.filter((option, index) => options.findIndex((item) => item.provider_uuid === option.provider_uuid) === index)
  const rules = (settings.rate_limits || []).map((rule) => ({ ...rule, scope: rule.scope || 'model' }))
  const availableProviders = providers.filter((option) => !rules.some((rule) => rule.scope === 'provider' && rule.provider_uuid === option.provider_uuid))
  const availableModels = options.filter((option) => !rules.some((rule) => rule.scope === 'model' && rule.provider_uuid === option.provider_uuid && rule.model === option.model))
  const namedRules = rules.map((rule) => ({ ...rule, provider_name: providers.find((option) => option.provider_uuid === rule.provider_uuid)?.provider_name || rule.provider_uuid }))
  const update = useMutation({
    mutationFn: (rule) => updateModelRateLimits(settings.revision, [rule]),
    onSuccess: (value) => {
      queryClient.setQueryData(['model-settings'], value)
      invalidateModelSettingsQueries(queryClient)
      setError(null)
      setEditor(null)
    },
    onError: (requestError) => {
      setError(requestError)
      queryClient.invalidateQueries({ queryKey: ['model-settings'] })
    },
  })
  const editRule = editor?.type === 'edit' ? namedRules.find((rule) => rule.scope === editor.scope && rule.provider_uuid === editor.provider_uuid && rule.model === editor.model) : null

  return <section id="provider-rate-limits" className="overview-card overview-model-limits" aria-labelledby={titleId}>
    <header className="overview-card__header overview-model-limits__header"><div><h2 id={titleId}>{t('settings.models.rate_limits.title')}</h2><p>{t('settings.models.rate_limits.body')}</p></div><button type="button" disabled={(!availableProviders.length && !availableModels.length) || update.isPending} onClick={() => { setError(null); setEditor({ type: 'add' }) }}>{t('settings.models.rate_limits.add')}</button></header>
    <LocalizedErrorMessage error={error} onDismiss={() => setError(null)} />
    {rules.length ? <div className="overview-model-limits__list">{namedRules.map((rule) => <div className="overview-model-limits__rule" key={`${rule.scope}:${rule.provider_uuid}:${rule.model || ''}`}>
      <div><strong>{rule.provider_name} · {rule.scope === 'provider' ? t('settings.models.rate_limits.provider_pool') : rule.model}</strong><span>{t('settings.models.rate_limits.rpm')}: {rule.rpm ?? '—'} · {t('settings.models.rate_limits.rps')}: {rule.rps ?? '—'}</span></div>
      <div className="overview-model-limits__actions"><button type="button" className="button-quiet" disabled={update.isPending} onClick={() => { setError(null); setEditor({ type: 'edit', scope: rule.scope, provider_uuid: rule.provider_uuid, model: rule.model }) }}>{t('common.action.edit')}</button><button type="button" className="button-quiet danger-text" disabled={update.isPending} onClick={() => update.mutate({ scope: rule.scope, provider_uuid: rule.provider_uuid, ...(rule.scope === 'model' ? { model: rule.model } : {}), rpm: null, rps: null })}>{t('common.action.delete')}</button></div>
    </div>)}</div> : <p className="overview-model-limits__empty">{t('settings.models.rate_limits.empty')}</p>}
    {editor && (editor.type === 'add' ? availableProviders.length || availableModels.length : editRule) ? <LimitEditor key={`${editor.type}:${editor.scope || ''}:${editor.provider_uuid || ''}:${editor.model || ''}`} providerOptions={availableProviders} modelOptions={availableModels} initial={editRule} pending={update.isPending} onCancel={() => setEditor(null)} onSave={(rule) => update.mutate(rule)} t={t} /> : null}
  </section>
}
