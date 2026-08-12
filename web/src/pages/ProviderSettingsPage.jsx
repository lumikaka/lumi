import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, X } from 'lucide-react'
import { Link, useLocation, useNavigate } from 'react-router-dom'

import AppPageShell from '../components/AppPageShell.jsx'
import { checkProvider, getSiteSettings, listProviders, resetSiteSettings, updateSiteSettings } from '../api/ai.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import LumiDialog from '../components/LumiDialog.jsx'
import { useI18n } from '../i18n/useI18n.js'

const PREFIXES = {
  cloudflare_ai_gateway: 'ai_providers.openai_compatible',
  aliyun_bailian: 'ai_providers.aliyun_bailian',
}

function secretItem(settings, providerType) {
  const key = `${PREFIXES[providerType]}.api_key`
  return settings?.items?.find((item) => item.key === key)
}

function providerNameKey(provider) {
  return provider.provider_type === 'aliyun_bailian'
    ? 'settings.provider.bailian_name'
    : 'settings.provider.cloudflare_name'
}

function initialProviderForm(provider, onboarding = false) {
  return provider.provider_type === 'aliyun_bailian'
    ? { workspace_id: provider.workspace_id || '', region: provider.region || 'cn-beijing' }
    : {
        account_id: provider.account_id || '',
        ...(onboarding ? {} : {
          default_model: provider.default_model || '',
          default_image_model: provider.default_image_model || '',
        }),
      }
}

function cloudflareEndpoint(accountId) {
  const normalized = accountId.trim().toLowerCase()
  return normalized ? `https://api.cloudflare.com/client/v4/accounts/${normalized}/ai/v1` : '—'
}

function ProviderListItem({ provider, onOpen }) {
  const { t } = useI18n()
  const isBailian = provider.provider_type === 'aliyun_bailian'
  const statusKey = provider.active
    ? 'settings.provider.current'
    : provider.verified ? 'settings.provider.connected' : provider.configured ? 'settings.provider.needs_verification' : 'settings.provider.not_configured'
  const statusTone = provider.active || provider.verified ? 'ready' : provider.configured ? 'warning' : 'idle'

  return (
    <button
      type="button"
      className={`provider-list-item${provider.active ? ' provider-list-item--active' : ''}`}
      onClick={onOpen}
      aria-label={t('settings.provider.open_configuration', { name: t(providerNameKey(provider)) })}
    >
      <span className="provider-list-item__content">
        <span className="provider-list-item__title">
          <strong>{t(providerNameKey(provider))}</strong>
          {isBailian ? <span className="provider-recommended">{t('settings.provider.recommended')}</span> : null}
        </span>
        <span className="provider-list-item__description">{t(isBailian ? 'settings.provider.bailian_description' : 'settings.provider.cloudflare_description')}</span>
        <span className="provider-list-item__requirements">{t(isBailian ? 'settings.provider.bailian_requirements' : 'settings.provider.cloudflare_requirements')}</span>
      </span>
      <span className="provider-list-item__aside">
        <span className={`provider-status provider-status--${statusTone}`}>{t(statusKey)}</span>
        <ChevronRight size={19} aria-hidden="true" />
      </span>
    </button>
  )
}

function ProviderDialog({ provider, settings, onboarding, onClose, onActivated }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const prefix = PREFIXES[provider.provider_type]
  const isBailian = provider.provider_type === 'aliyun_bailian'
  const secret = secretItem(settings, provider.provider_type)
  const [form, setForm] = useState(() => initialProviderForm(provider, onboarding))
  const [apiKey, setApiKey] = useState('')
  const [error, setError] = useState(null)

  useEffect(() => {
    setForm(initialProviderForm(provider, onboarding))
  }, [onboarding, provider.account_id, provider.default_image_model, provider.default_model, provider.provider_type, provider.region, provider.uuid, provider.workspace_id])

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['site-settings'] }),
      queryClient.invalidateQueries({ queryKey: ['providers'] }),
      queryClient.invalidateQueries({ queryKey: ['active-provider'] }),
    ])
  }
  const saveAndCheck = useMutation({
    mutationFn: async () => {
      const next = Object.fromEntries(Object.entries(form).map(([key, value]) => [`${prefix}.${key}`, value]))
      if (apiKey.trim()) next[`${prefix}.api_key`] = apiKey.trim()
      await updateSiteSettings(next)
      await checkProvider(provider.uuid)
      if (onboarding) await updateSiteSettings({ 'ai_provider.active': provider.provider_type })
    },
    onMutate: () => setError(null),
    onSuccess: async () => {
      setApiKey('')
      await refresh()
      onClose()
      if (onboarding) onActivated()
    },
    onError: async (requestError) => {
      setError(requestError)
      await refresh()
    },
  })
  const activate = useMutation({
    mutationFn: () => updateSiteSettings({ 'ai_provider.active': provider.provider_type }),
    onMutate: () => setError(null),
    onSuccess: async () => {
      await refresh()
      onClose()
      onActivated()
    },
    onError: setError,
  })
  const resetSecret = useMutation({
    mutationFn: () => resetSiteSettings([`${prefix}.api_key`]),
    onSuccess: async () => {
      setApiKey('')
      setError(null)
      await refresh()
    },
    onError: setError,
  })
  const field = (name) => ({
    value: form[name],
    onChange: (event) => setForm((current) => ({ ...current, [name]: event.target.value })),
  })
  const initialForm = initialProviderForm(provider, onboarding)
  const hasUnsavedChanges = apiKey.trim() || Object.keys(initialForm).some((key) => form[key] !== initialForm[key])
  const busy = saveAndCheck.isPending || activate.isPending || resetSecret.isPending
  const providerName = t(providerNameKey(provider))
  const secretCopy = secret?.secret_state === 'unavailable'
    ? t('settings.provider.secret_unavailable')
    : secret?.has_value ? t('settings.provider.secret_saved', { masked: secret.masked_value || '****' }) : t(isBailian ? 'settings.provider.api_key_required' : 'settings.provider.cloudflare_api_token_required')
  const close = () => { if (!busy) onClose() }

  return (
    <LumiDialog
      className="provider-dialog"
      aria-labelledby="provider-dialog-title"
      dismissDisabled={busy}
      onClose={close}
    >
      <header className="lumi-dialog__header">
        <div>
          <h2 id="provider-dialog-title">{t('settings.provider.dialog_title', { name: providerName })}</h2>
          <p>{t('settings.provider.dialog_security')}</p>
        </div>
        <button type="button" className="button-quiet" disabled={busy} aria-label={t('common.action.close')} onClick={close}><X size={18} aria-hidden="true" /></button>
      </header>
      <form aria-busy={busy} onSubmit={(event) => { event.preventDefault(); saveAndCheck.mutate() }}>
        <div className="lumi-dialog__body provider-dialog__body">
          <div className="provider-config-grid">
            {isBailian ? <>
              <label>{t('settings.provider.workspace_id')}<input {...field('workspace_id')} required autoFocus placeholder={t('settings.provider.workspace_id_placeholder')} /></label>
              <label>{t('settings.provider.region')}<select {...field('region')}><option value="cn-beijing">{t('settings.provider.region.beijing')}</option><option value="ap-southeast-1">{t('settings.provider.region.singapore')}</option><option value="eu-central-1">{t('settings.provider.region.frankfurt')}</option><option value="ap-northeast-1">{t('settings.provider.region.tokyo')}</option></select></label>
              <p>{t('settings.provider.fixed_models', { text_model: provider.default_model, image_model: provider.default_image_model })}</p>
            </> : <>
              <label>{t('settings.provider.cloudflare_account_id')}<input {...field('account_id')} required autoFocus minLength={32} maxLength={32} pattern="[A-Fa-f0-9]{32}" placeholder={t('settings.provider.cloudflare_account_id_placeholder')} /></label>
              <p>{t('settings.provider.cloudflare_endpoint', { endpoint: cloudflareEndpoint(form.account_id) })}</p>
              {!onboarding ? <>
                <label>{t('settings.provider.default_text_model')}<input {...field('default_model')} required placeholder="deepseek/deepseek-v4-pro" /></label>
                <label>{t('settings.provider.default_image_model')}<input {...field('default_image_model')} required placeholder="openai/gpt-5.5" /></label>
              </> : <p>{t('settings.provider.cloudflare_setup_defaults')}</p>}
            </>}
            <label className="provider-api-key-field">
              {t(isBailian ? 'settings.provider.api_key' : 'settings.provider.cloudflare_api_token')}
              <input type="password" value={apiKey} onChange={(event) => setApiKey(event.target.value)} required={!secret?.has_value} autoComplete="new-password" placeholder={t(secret?.has_value ? 'settings.provider.api_key_keep' : (isBailian ? 'settings.provider.api_key_enter' : 'settings.provider.cloudflare_api_token_enter'))} />
              <span className={secret?.secret_state === 'unavailable' ? 'provider-field-hint provider-field-hint--danger' : 'provider-field-hint'}>{secretCopy}</span>
            </label>
          </div>
          <LocalizedErrorMessage error={error} className="provider-error" />
        </div>
        <footer className="lumi-dialog__actions provider-dialog__actions">
          {secret?.has_value && !onboarding ? <button type="button" className="button-quiet danger-text provider-dialog__reset" disabled={busy} onClick={() => window.confirm(t('settings.provider.api_key_confirm')) && resetSecret.mutate()}>{t('settings.provider.api_key_reset')}</button> : null}
          {!onboarding && provider.verified && !provider.active ? <button type="button" className="button-secondary" disabled={busy || Boolean(hasUnsavedChanges)} onClick={() => activate.mutate()}>{t('settings.provider.set_current')}</button> : null}
          <button type="button" className="button-secondary" disabled={busy} onClick={close}>{t('common.action.cancel')}</button>
          <button type="submit" disabled={busy}>{t(saveAndCheck.isPending ? (onboarding ? 'settings.provider.connecting_start' : 'settings.provider.saving_checking') : (onboarding ? 'settings.provider.connect_start' : 'settings.provider.save_check'))}</button>
        </footer>
      </form>
    </LumiDialog>
  )
}

export default function ProviderSettingsPage({ onboarding = false }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const [selectedProviderType, setSelectedProviderType] = useState(null)
  const providersQuery = useQuery({ queryKey: ['providers'], queryFn: listProviders })
  const settingsQuery = useQuery({ queryKey: ['site-settings'], queryFn: getSiteSettings })
  const items = providersQuery.data?.items || []
  const sortedItems = [...items].sort((left, right) => Number(right.provider_type === 'aliyun_bailian') - Number(left.provider_type === 'aliyun_bailian'))
  const selectedProvider = items.find((item) => item.provider_type === selectedProviderType)
  const returnTarget = typeof location.state?.from === 'string' && !location.state.from.startsWith('/settings/providers') && !location.state.from.startsWith('/setup') ? location.state.from : '/'
  const onActivated = () => navigate(returnTarget, { replace: true })

  return (
    <AppPageShell title={t('settings.ai')} actions={!onboarding ? <Link className="project-topbar__action" to="/">{t('common.not_found.action')}</Link> : null}>
      <div className="provider-settings">
        <header className="provider-heading">
          <div><p className="eyebrow">{t('settings.provider.eyebrow')}</p><h1>{t(onboarding ? 'settings.provider.onboarding_title' : 'settings.provider.title')}</h1><p>{t(onboarding ? 'settings.provider.onboarding_body' : 'settings.provider.security_body')}</p></div>
          <span className={items.some((item) => item.active) ? '' : 'provider-heading__inactive'}>{items.find((item) => item.active)?.display_name || t('settings.provider.not_active')}</span>
        </header>
        <LocalizedErrorMessage error={providersQuery.error || settingsQuery.error} className="provider-error" />
        <section className="provider-list-section" aria-labelledby="provider-list-title">
          <header className="provider-list-heading"><h2 id="provider-list-title">{t('settings.provider.choose_title')}</h2><p>{t('settings.provider.choose_body')}</p></header>
          <div className="provider-list">
            {providersQuery.isLoading || settingsQuery.isLoading ? <p className="workspace-loading">{t('settings.provider.loading')}</p> : null}
            {sortedItems.map((item) => <ProviderListItem key={item.uuid} provider={item} onOpen={() => setSelectedProviderType(item.provider_type)} />)}
          </div>
        </section>
      </div>
      {selectedProvider && settingsQuery.data ? <ProviderDialog provider={selectedProvider} settings={settingsQuery.data} onboarding={onboarding} onClose={() => setSelectedProviderType(null)} onActivated={onActivated} /> : null}
    </AppPageShell>
  )
}
