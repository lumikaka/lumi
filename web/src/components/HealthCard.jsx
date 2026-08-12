import { useQuery } from '@tanstack/react-query'

import { getHealth } from '../api/health.js'
import { useI18n } from '../i18n/useI18n.js'

export default function HealthCard({ compact = false }) {
  const { t } = useI18n()
  const health = useQuery({
    queryKey: ['health'],
    queryFn: getHealth,
    retry: 1,
    refetchInterval: 30_000,
  })

  const state = health.isPending ? 'checking' : health.isError ? 'offline' : 'online'
  const label = t(state === 'checking' ? 'common.status.connecting' : state === 'offline' ? 'common.health.unavailable' : 'common.health.connected')

  return (
    <section className={`health-card health-card--${state} ${compact ? 'health-card--compact' : ''}`} aria-live="polite">
      <div className="health-card__status">
        <span className="health-card__dot" aria-hidden="true" />
        <div>
          <p>{t('common.health.title')}</p>
          <strong>{label}</strong>
        </div>
      </div>
      {!compact ? (
        <div className="health-card__details">
          <span>API</span>
          <b>{health.data?.status || '—'}</b>
          <span>SQLite</span>
          <b>{health.data?.database || '—'}</b>
        </div>
      ) : null}
      {health.isError ? (
        <button className="health-card__retry" onClick={() => health.refetch()} type="button">{t('common.health.retry')}</button>
      ) : null}
    </section>
  )
}
