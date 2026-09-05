import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createModelPrice, deleteModelPrice, listModelPrices } from '../api/pricing.js'
import LumiDialog from '../components/LumiDialog.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { blankPrice, blankRates, money, pricingMetrics } from './pricingState.js'
import './pricing.sass'

function PriceEditor({ provider, initial, prices, onClose }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const [form, setForm] = useState(() => initial ? { ...initial, rates: initial.rates.map((rate) => ({ ...rate })), provider_uuid: provider.uuid } : blankPrice(provider))
  const update = (key, value) => setForm((old) => ({ ...old, [key]: value }))
  const updateRate = (index, key, value) => setForm((old) => ({ ...old, rates: old.rates.map((rate, i) => i === index ? { ...rate, [key]: value } : rate) }))
  // Freeze the edit's expected predecessor. Background invalidation must not
  // silently convert a stale form into permission to overwrite a newer version.
  const [versions] = useState(prices)
  const save = useMutation({
    mutationFn: () => {
      const previous = versions.find((p) => p.active && p.source === 'user' && p.provider_uuid === provider.uuid && p.model === form.model && p.region === form.region && p.request_type === form.request_type)
      const { uuid, source, active, created_at, ...rule } = form
      return createModelPrice({ ...rule, verified_at: '' }, previous?.uuid || '')
    },
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ['model-prices'] }); onClose() },
  })
  const changeKind = (kind) => { const mode = kind === 'text' ? 'tokens' : 'images'; setForm((old) => ({ ...old, request_type: kind, mode, rates: blankRates(mode) })) }
  return <LumiDialog className="pricing-dialog" onClose={() => { if (!save.isPending) onClose() }}>
    <header className="lumi-dialog__header"><h2>{t('pricing.edit')}</h2><button type="button" onClick={onClose} disabled={save.isPending}>{t('common.action.close')}</button></header>
    <form onSubmit={(event) => { event.preventDefault(); save.mutate() }}>
      <LocalizedErrorMessage error={save.error} />
      <div className="pricing-fields">
        <label>{t('settings.llm_logs.filter.model')}<input required maxLength={512} value={form.model} onChange={(e) => update('model', e.target.value)} /></label>
        <label>{t('common.label.type')}<select value={form.request_type} onChange={(e) => changeKind(e.target.value)}><option value="text">{t('settings.llm_logs.request_type.text')}</option><option value="image">{t('settings.llm_logs.request_type.image')}</option></select></label>
        <label>{t('pricing.region')}<input maxLength={80} value={form.region} onChange={(e) => update('region', e.target.value)} placeholder={t('pricing.region_hint')} /></label>
        <label>{t('pricing.currency')}<input required pattern="[A-Z]{3}" maxLength={3} value={form.currency} onChange={(e) => update('currency', e.target.value.toUpperCase())} /></label>
        {form.request_type === 'image' ? <label>{t('pricing.mode')}<select value={form.mode} onChange={(e) => setForm((old) => ({ ...old, mode: e.target.value, rates: blankRates(e.target.value) }))}><option value="images">{t('pricing.mode.images')}</option><option value="responses">{t('pricing.mode.responses')}</option></select></label> : null}
        {form.mode === 'responses' ? <label>{t('pricing.image_model')}<input required maxLength={512} value={form.image_model || ''} onChange={(e) => update('image_model', e.target.value)} /></label> : null}
        <label>{t('pricing.source_url')}<input type="url" value={form.source_url || ''} onChange={(e) => update('source_url', e.target.value)} /></label>
      </div>
      <p className="pricing-note">{t('pricing.rule_hint')}</p>
      {form.request_type === 'image' ? <p className="pricing-note">{t(`pricing.hint.${form.mode}`)}</p> : null}
      <div className="pricing-rates">
        {form.rates.map((rate, index) => <fieldset key={index} className="pricing-rate"><legend>{index + 1}</legend>
          <div className="pricing-fields">
            <label>{t('pricing.metric')}<select value={rate.metric} onChange={(e) => updateRate(index, 'metric', e.target.value)}>{pricingMetrics(form.mode).map((m) => <option value={m} key={m}>{t(`pricing.metric.${m}`)}</option>)}</select></label>
            <label>{t(rate.metric.endsWith('_tokens') ? 'pricing.per_million' : 'pricing.per_image')}<input required inputMode="decimal" pattern="(0|[1-9][0-9]{0,9})(\.[0-9]{1,9})?" value={rate.price} onChange={(e) => updateRate(index, 'price', e.target.value)} /></label>
            <label>{t('pricing.input_from')}<input type="number" min="0" max="9007199254740991" step="1" required value={rate.input_from} onChange={(e) => updateRate(index, 'input_from', Number(e.target.value))} /></label>
            <label>{t('pricing.input_to')}<input type="number" min="1" max="9007199254740991" step="1" value={rate.input_to ?? ''} onChange={(e) => updateRate(index, 'input_to', e.target.value ? Number(e.target.value) : null)} /></label>
            {form.request_type === 'image' ? <>
              <label>{t('pricing.size')}<input value={rate.size || ''} onChange={(e) => updateRate(index, 'size', e.target.value)} placeholder={t('pricing.size_example')} /></label>
              <label>{t('pricing.quality')}<input value={rate.quality || ''} onChange={(e) => updateRate(index, 'quality', e.target.value)} placeholder={t('pricing.quality_example')} /></label>
              <label>{t('pricing.resolution')}<select value={rate.resolution || ''} onChange={(e) => updateRate(index, 'resolution', e.target.value)}><option value="">{t('common.label.all')}</option>{['1k', '2k', '4k'].map((v) => <option key={v}>{v}</option>)}</select></label>
            </> : null}
          </div>
          <button type="button" className="button-quiet" onClick={() => update('rates', form.rates.filter((_, i) => i !== index))}>{t('common.action.delete')}</button>
        </fieldset>)}
      </div>
      <div className="pricing-actions"><button type="button" className="button-secondary" disabled={form.rates.length >= 100} onClick={() => update('rates', [...form.rates, { ...blankRates(form.mode)[0] }])}>{t('pricing.add_rate')}</button><button disabled={save.isPending || !form.rates.length} type="submit">{t('pricing.save_version')}</button></div>
    </form>
  </LumiDialog>
}
export default function ModelPricesPanel({ providers }) {
  const { t, formatDateTime } = useI18n()
  const [providerUuid, setProviderUuid] = useState('')
  const [editor, setEditor] = useState(null)
  const queryClient = useQueryClient()
  const query = useQuery({ queryKey: ['model-prices'], queryFn: listModelPrices })
  const prices = query.data?.items || []
  const provider = providers.find((p) => p.uuid === providerUuid) || providers.find((p) => p.active) || providers[0]
  const rows = prices.filter((p) => p.provider_type === provider?.provider_type && (!p.provider_uuid || p.provider_uuid === provider?.uuid))
  const revoke = useMutation({ mutationFn: deleteModelPrice, onSuccess: () => queryClient.invalidateQueries({ queryKey: ['model-prices'] }) })
  return <section className="pricing-panel">
    <header className="pricing-actions"><div><h2>{t('pricing.title')}</h2><p className="pricing-note">{t('pricing.catalog_hint')}</p></div>{provider ? <button type="button" onClick={() => setEditor({})}>{t('pricing.add')}</button> : null}</header>
    <LocalizedErrorMessage error={query.error || revoke.error} />
    <label>{t('settings.provider')}<select value={provider?.uuid || ''} onChange={(e) => setProviderUuid(e.target.value)}>{providers.map((p) => <option key={p.uuid} value={p.uuid}>{p.display_name}</option>)}</select></label>
    {provider ? [provider.default_model, provider.default_image_model].filter(Boolean).filter((model) => !rows.some((p) => p.active && p.model === model && p.region === (provider.region || ''))).map((model) => <p key={model} className="pricing-note">{model} · {t('pricing.missing_price')}</p>) : null}
    <div className="pricing-table-wrap"><table className="pricing-table"><thead><tr><th>{t('settings.llm_logs.filter.model')}</th><th>{t('pricing.region')}</th><th>{t('pricing.source')}</th><th>{t('pricing.rates')}</th><th>{t('common.label.status')}</th><th>{t('pricing.actions')}</th></tr></thead><tbody>
      {rows.filter((p) => p.active).map((p) => <tr key={p.uuid}><td>{p.model}<small>{t(`pricing.mode.${p.mode}`)}</small></td><td>{p.region || '—'}<small>{p.currency}</small></td><td>{t(`pricing.source.${p.source}`)}<small>{p.verified_at || formatDateTime(p.created_at)}</small>{p.source_url ? <a href={p.source_url} target="_blank" rel="noreferrer">{t('pricing.source_url')}</a> : null}</td><td><details><summary>{t('pricing.show_rates')}</summary>{p.rates.map((rate, i) => <p key={i}>{t(`pricing.metric.${rate.metric}`)}: {money(rate.price, p.currency)} / {t(rate.metric.endsWith('_tokens') ? 'pricing.million_tokens' : 'pricing.image')}<small>{t('pricing.tier_range', { from: rate.input_from, to: rate.input_to ?? '∞' })} {rate.size} {rate.quality} {rate.resolution}</small></p>)}</details></td><td>{t(p.source === 'builtin' && rows.some((x) => x.active && x.source === 'user' && x.model === p.model && x.region === p.region && x.request_type === p.request_type) ? 'pricing.overridden' : 'pricing.active')}</td><td><button type="button" className="button-quiet" onClick={() => setEditor(p)}>{t('pricing.edit')}</button>{p.source === 'user' ? <button type="button" className="button-quiet" disabled={revoke.isPending} onClick={() => revoke.mutate(p.uuid)}>{t('pricing.restore')}</button> : null}</td></tr>)}
    </tbody></table></div>
    <details><summary>{t('pricing.history')}</summary>{rows.filter((p) => !p.active).map((p) => <p key={p.uuid}>{p.model} · {p.region} · {formatDateTime(p.created_at)} · {p.uuid}</p>)}</details>
    {editor && provider ? <PriceEditor provider={provider} initial={editor.uuid ? editor : null} prices={prices} onClose={() => setEditor(null)} /> : null}
  </section>
}
