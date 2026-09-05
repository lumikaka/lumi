import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { applyCostBackfill, getCostSummary, listCostBackfills, listModelPrices, previewCostBackfill } from '../api/pricing.js'
import LumiDialog from '../components/LumiDialog.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { money } from './pricingState.js'
import './pricing.sass'

export function CostValue({ log }) {
  const { t } = useI18n()
  return <span>{log.cost_status === 'calculated' ? money(log.cost_amount, log.cost_currency) : t(log.cost_status === 'pending' || log.status === 'pending' ? 'pricing.pending' : 'pricing.unpriced')}{log.cost_origin === 'backfill' ? <small>{t('pricing.historical')}</small> : null}</span>
}
export function CostDetails({ log }) {
  const { t } = useI18n()
  const details = log.cost_details
  const price = details?.price_snapshot?.price
  return <section className="pricing-panel pricing-detail"><h3>{t('pricing.estimate')}</h3><strong><CostValue log={log} /></strong>
    {log.cost_status === 'unpriced' ? <p>{t(`pricing.reason.${log.cost_reason || 'missing_price'}`)}</p> : null}
    {price ? <p className="pricing-note">{price.model} · {price.region || '—'} · {t(`pricing.source.${price.source}`)} · {price.uuid}</p> : null}
    {details?.lines?.length ? <div className="pricing-table-wrap"><table className="pricing-table"><thead><tr><th>{t('pricing.metric')}</th><th>{t('pricing.quantity')}</th><th>{t('pricing.unit_price')}</th><th>{t('pricing.estimate')}</th></tr></thead><tbody>{details.lines.map((line) => <tr key={line.metric}><td>{t(`pricing.metric.${line.metric}`)}</td><td>{line.quantity}</td><td>{money(line.price, log.cost_currency)} / {line.per === 1000000 ? t('pricing.million_tokens') : t('pricing.image')}</td><td>{money(line.amount, log.cost_currency)}</td></tr>)}</tbody></table></div> : null}
  </section>
}
function BackfillDialog({ projectUuid, filters, onClose }) {
  const { t, formatDateTime } = useI18n()
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState([])
  const [preview, setPreview] = useState(null)
  const alive = useRef(true)
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const pricesQuery = useQuery({ queryKey: ['model-prices'], queryFn: listModelPrices })
  const plansQuery = useQuery({ queryKey: ['project-llm-cost-backfills', projectUuid], queryFn: () => listCostBackfills(projectUuid) })
  const prices = (pricesQuery.data?.items || []).filter((p) => (!filters.providerType || p.provider_type === filters.providerType) && (!filters.providerUuid || !p.provider_uuid || p.provider_uuid === filters.providerUuid) && (!filters.model || p.model === filters.model) && (!filters.requestType || p.request_type === filters.requestType))
  const create = useMutation({ mutationFn: () => previewCostBackfill(projectUuid, filters, selected), onSuccess: (result) => { setPreview(result); queryClient.invalidateQueries({ queryKey: ['project-llm-cost-backfills', projectUuid] }) } })
  const apply = useMutation({ mutationFn: async () => {
    let result = preview
    // Each request applies one durable batch. These are explicit writes, not
    // background HTTP polling. Navigating away stops dispatching further batches.
    while (result?.has_more && alive.current) { result = await applyCostBackfill(projectUuid, result.uuid); if (alive.current) setPreview(result) }
    return result
  }, onSettled: () => { queryClient.invalidateQueries({ queryKey: ['project-llm-cost-backfills', projectUuid] }); queryClient.invalidateQueries({ queryKey: ['project-llm-cost-summary', projectUuid] }); queryClient.invalidateQueries({ queryKey: ['project-llm-logs', projectUuid] }) } })
  const toggle = (price) => {
    setPreview(null)
    setSelected((old) => old.includes(price.uuid) ? old.filter((id) => id !== price.uuid) : [...old.filter((id) => { const p = prices.find((x) => x.uuid === id); return p && (p.model !== price.model || p.provider_type !== price.provider_type || p.request_type !== price.request_type) }), price.uuid])
  }
  return <LumiDialog className="pricing-dialog" onClose={() => { if (!apply.isPending) onClose() }}>
    <header className="lumi-dialog__header"><h2>{t('pricing.backfill')}</h2><button type="button" disabled={apply.isPending} onClick={onClose}>{t('common.action.close')}</button></header>
    <p>{t('pricing.backfill_hint')}</p>
    <LocalizedErrorMessage error={pricesQuery.error || plansQuery.error || create.error || apply.error} />
    <div className="pricing-price-options">{prices.map((p) => (
      <label className="pricing-price-option" key={p.uuid}>
        <input type="checkbox" checked={selected.includes(p.uuid)} disabled={apply.isPending || create.isPending} onChange={() => toggle(p)} />
        <span className="pricing-price-option__content">
          <span className="pricing-price-option__heading"><strong>{p.model}</strong><span>{p.region || '—'} · {p.currency}</span></span>
          <small>{p.provider_type} · {t(`pricing.source.${p.source}`)} · {formatDateTime(p.created_at)} · {p.uuid}</small>
        </span>
      </label>
    ))}</div>
    {!pricesQuery.isLoading && !prices.length ? <p>{t('pricing.missing_price')}</p> : null}
    <div className="pricing-actions"><button type="button" disabled={!selected.length || create.isPending || apply.isPending} onClick={() => create.mutate()}>{t('pricing.preview')}</button></div>
    {preview ? <div className="pricing-preview" aria-live="polite"><p>{t('pricing.backfill_counts', { total: preview.total, calculated: preview.calculated, skipped: preview.skipped, processed: preview.processed })}</p>{preview.totals.map((row) => <p key={row.currency}>{money(row.amount, row.currency)}</p>)}<button type="button" disabled={!preview.has_more || apply.isPending || create.isPending} onClick={() => apply.mutate()}>{t(preview.has_more ? 'pricing.apply' : 'pricing.applied')}</button></div> : null}
    <details><summary>{t('pricing.recent_previews')}</summary>{(plansQuery.data?.items || []).map((plan) => <p key={plan.uuid}><button type="button" className="button-quiet" disabled={apply.isPending || create.isPending} onClick={() => setPreview(plan)}>{formatDateTime(plan.created_at)} · {plan.processed} / {plan.total}</button></p>)}</details>
  </LumiDialog>
}
export default function LLMCostPanel({ projectUuid, filters }) {
  const { t } = useI18n()
  const [backfillOpen, setBackfillOpen] = useState(false)
  const query = useQuery({ queryKey: ['project-llm-cost-summary', projectUuid, filters], queryFn: () => getCostSummary(projectUuid, filters) })
  const data = query.data
  return <section className="pricing-summary">
    <header className="pricing-actions"><h2>{t('pricing.filtered_total')}</h2><button type="button" className="button-secondary" onClick={() => setBackfillOpen(true)}>{t('pricing.backfill')}</button></header>
    <LocalizedErrorMessage error={query.error} />
    {data ? <><div className="pricing-totals">{data.totals.map((row) => <strong key={row.currency}>{money(row.amount, row.currency)}</strong>)}{!data.totals.length ? <span>{t('pricing.no_estimates')}</span> : null}</div><p className="pricing-note">{t('pricing.counts', { calculated: data.calculated, unpriced: data.unpriced, pending: data.pending })}</p>
      <details><summary>{t('pricing.group_totals')}</summary>{[['by_model', 'pricing.by_model'], ['by_scenario', 'pricing.by_scenario']].map(([key, label]) => <div key={key}><h3>{t(label)}</h3><div className="pricing-table-wrap"><table className="pricing-table"><tbody>{data[key].map((row) => <tr key={`${row.currency}-${row.model || row.scenario}-${row.request_type || ''}`}><td>{row.model || row.scenario}</td><td>{row.count}</td><td>{money(row.amount, row.currency)}</td></tr>)}</tbody></table></div></div>)}</details>
    </> : null}
    {backfillOpen ? <BackfillDialog key={projectUuid} projectUuid={projectUuid} filters={filters} onClose={() => setBackfillOpen(false)} /> : null}
  </section>
}
