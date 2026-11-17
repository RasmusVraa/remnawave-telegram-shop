import { useTranslation } from 'react-i18next'
import { Hourglass, ShieldCheck, UserPlus, Users } from 'lucide-react'

import type { AdminStatsInsightsDTO, AdminStatsTimeSeriesDTO } from '@/lib/types/admin'
import type { AdminStatsResponse } from '../../hooks/useAdminStats'

import { StatsBaseComposition } from '../components/StatsBaseComposition'
import { shouldShowChannelGate, StatsChannelGateBlock } from '../components/StatsChannelGateBlock'
import { StatsDelta, StatsKpiCard } from '../components/StatsKpiCard'
import { StatsMainChart } from '../components/StatsMainChart'
import { TopReferrersTable } from '../components/TopReferrersTable'
import {
  formatDecimal,
  formatGrowthPct,
  formatPct,
  growthTrend,
  statsNumberLocale,
} from '../utils/statsFormat'
import { STATS_ACCENT } from '../utils/statsPalette'
import {
  resolveStatsPeriodSlice,
  type StatsCustomRange,
  type StatsPeriod,
} from '../utils/statsPeriod'

interface StatsUsersTabProps {
  data: AdminStatsResponse
  insights?: AdminStatsInsightsDTO | null
  timeseries?: AdminStatsTimeSeriesDTO | null
  period: StatsPeriod
  customRange?: StatsCustomRange | null
}

export function StatsUsersTab({
  data,
  insights,
  timeseries,
  period,
  customRange,
}: StatsUsersTabProps) {
  const { t, i18n } = useTranslation()
  const numberLocale = statsNumberLocale(i18n.language)

  const slice = resolveStatsPeriodSlice(data, period, timeseries)
  const newUsers = insights?.current.new_users ?? slice.newUsers
  const previous = insights?.previous

  const lifetime = insights?.lifetime
  const lifetimeMonths = lifetime ? lifetime.avg_lifetime_days / 30.44 : null

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatsKpiCard
          icon={Users}
          color={STATS_ACCENT.blue}
          label={t('admin.stats.totalCustomers')}
          value={data.total_customers.toLocaleString(numberLocale)}
          hint={
            <span className="text-muted-foreground">{t('admin.stats.totalCustomersHint')}</span>
          }
        />
        <StatsKpiCard
          icon={UserPlus}
          color={STATS_ACCENT.green}
          label={t('admin.stats.newInPeriodShort')}
          value={newUsers.toLocaleString(numberLocale)}
          hint={
            previous === undefined ? undefined : (
              <StatsDelta
                pct={formatGrowthPct(newUsers, previous.new_users)}
                trend={growthTrend(newUsers, previous.new_users)}
                note={t('admin.stats.deltaNote')}
              />
            )
          }
        />
        <StatsKpiCard
          icon={ShieldCheck}
          color={STATS_ACCENT.cyan}
          label={t('admin.stats.activeSubscriptions')}
          value={data.active_subscriptions.toLocaleString(numberLocale)}
          hint={
            <span className="text-muted-foreground">
              {t('admin.stats.activeSubsHint', {
                base: formatPct(data.active_subscriptions, data.total_customers, numberLocale),
                paid: formatPct(data.paid_active, data.trial_active + data.paid_active, numberLocale),
              })}
            </span>
          }
        />
        <StatsKpiCard
          icon={Hourglass}
          color={STATS_ACCENT.amber}
          label={t('admin.stats.avgLifetime')}
          value={
            lifetimeMonths === null
              ? '—'
              : t('admin.stats.monthsValue', { value: formatDecimal(lifetimeMonths, numberLocale) })
          }
          hint={
            <span className="text-muted-foreground">
              {lifetime
                ? t('admin.stats.avgLifetimeHint', {
                    months: formatDecimal(lifetime.avg_paid_months, numberLocale),
                    purchases: formatDecimal(lifetime.avg_purchases, numberLocale),
                  })
                : t('admin.stats.lifetimeHint')}
            </span>
          }
        />
      </div>

      <StatsMainChart
        timeseries={timeseries}
        period={period}
        customRange={customRange}
        only="new_users"
        title={t('admin.stats.newRegistrations')}
      />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.25fr)]">
        <StatsBaseComposition data={data} />
        <TopReferrersTable
          rows={data.top_referrers}
          distinctReferrers={data.distinct_referrers}
          activeReferrers={data.active_referrers}
          bonusDaysAll={data.ref_bonus_days_all}
        />
      </div>

      {shouldShowChannelGate(insights?.channel_gate) && <StatsChannelGateBlock gate={insights.channel_gate} />}
    </div>
  )
}
