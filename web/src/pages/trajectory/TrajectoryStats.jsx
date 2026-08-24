import { Fragment, useMemo } from 'react'

import { useI18n } from '../../i18n/useI18n.js'
import { trajectoryStatsGroups } from './trajectoryStats.js'

export default function TrajectoryStats({ overview }) {
  const { t } = useI18n()
  const groups = useMemo(() => trajectoryStatsGroups(overview, t), [overview, t])
  if (!groups.length) return null
  const line = groups.join(' | ')
  return (
    <footer className="trajectory-stats" aria-label={t('trajectory.stats.aria')}>
      <p className="trajectory-stats__line" role="status" title={line}>
        {groups.map((group, index) => (
          <Fragment key={group}>
            {index > 0 ? <span className="trajectory-stats__separator" aria-hidden="true">|</span> : null}
            <span>{group}</span>
          </Fragment>
        ))}
      </p>
    </footer>
  )
}
