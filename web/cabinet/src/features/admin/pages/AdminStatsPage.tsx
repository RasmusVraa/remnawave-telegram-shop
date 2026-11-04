import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { BarChart3, Check, Gift, LayoutGrid, Loader2, RefreshCw, Users, Wallet } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { useAdminShell } from '../layout/AdminShellContext'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { useAdminStats } from '../hooks/useAdminStats'
import { useAdminStatsTimeSeries } from '../hooks/useAdminStatsTimeSeries'
import { useAdminStatsInsights } from '../hooks/useAdminStatsInsights'
import { useAdminFortuneStats } from '../hooks/useAdminFortuneStats'
import { useAdminLoyaltyStats } from '../hooks/useAdminLoyaltyStats'
import { useAdminPromoStats } from '../hooks/useAdminPromoStats'
import { StatsCustomRangePicker } from '../stats/components/StatsCustomRangePicker'
import { StatsPeriodSelector } from '../stats/components/StatsPeriodSelector'
import { StatsTabs, type StatsTabItem } from '../stats/components/StatsTabs'
import { StatsMechanicsTab } from '../stats/tabs/StatsMechanicsTab'
import { StatsMoneyTab } from '../stats/tabs/StatsMoneyTab'
import { StatsOverviewTab } from '../stats/tabs/StatsOverviewTab'
import { StatsUsersTab } from '../stats/tabs/StatsUsersTab'
import type { StatsCustomRange, StatsPeriod } from '../stats/utils/statsPeriod'
import { statsNumberLocale } from '../stats/utils/statsFormat'
import { useAdminMobileHeaderAutoHide } from '../hooks/useAdminMobileHeaderAutoHide'

type StatsTabKey = 'overview' | 'money' | 'users' | 'mechanics'

const STATS_TAB_KEYS: StatsTabKey[] = ['overview', 'money', 'users', 'mechanics']

function isStatsTab(value: string | null): value is StatsTabKey {
  return value !== null && (STATS_TAB_KEYS as string[]).includes(value)
}

export default function AdminStatsPage() {
  return (
    <AdminLayout>
      <AdminStatsPageContent />
    </AdminLayout>
  )
}

function AdminStatsPageContent() {
  const { t, i18n } = useTranslation()
  const { mobileHeaderVisible } = useAdminShell()
  useAdminMobileHeaderAutoHide(true)
  // Вкладка живёт в адресе: на неё ссылается дашборд («колесо в минусе» ведёт
  // прямо в «Механики»), и её можно сохранить в закладки.
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const tab: StatsTabKey = isStatsTab(tabParam) ? tabParam : 'overview'
  const setTab = (next: StatsTabKey) => {
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev)
        params.set('tab', next)
        return params
      },
      { replace: true },
    )
  }
  // Галочка вместо иконки на пару секунд после успешного обновления — иначе
  // по спиннеру, который гаснет за долю секунды, непонятно, случилось ли что-то.
  const [refreshDone, setRefreshDone] = useState(false)
  const [period, setPeriod] = useState<StatsPeriod>('month')
  const [customRange, setCustomRange] = useState<StatsCustomRange | null>(null)

  const { data, isLoading, error, refetch, isFetching } = useAdminStats()
  const {
    data: timeseries,
    refetch: refetchTimeseries,
    isFetching: timeseriesFetching,
  } = useAdminStatsTimeSeries(period, customRange)
  const {
    data: insights,
    refetch: refetchInsights,
    isFetching: insightsFetching,
  } = useAdminStatsInsights(period, customRange)
  const {
    data: fortuneData,
    refetch: refetchFortune,
    isFetching: fortuneFetching,
  } = useAdminFortuneStats()
  const {
    data: loyaltyData,
    refetch: refetchLoyalty,
    isFetching: loyaltyFetching,
  } = useAdminLoyaltyStats()
  const {
    data: promoData,
    refetch: refetchPromo,
    isFetching: promoFetching,
  } = useAdminPromoStats()

  const refreshing =
    isFetching ||
    fortuneFetching ||
    timeseriesFetching ||
    insightsFetching ||
    loyaltyFetching ||
    promoFetching
  const numberLocale = statsNumberLocale(i18n.language)

  const handlePeriodChange = (next: StatsPeriod) => {
    setCustomRange(null)
    setPeriod(next)
  }

  const handleCustomRange = (range: StatsCustomRange) => {
    setCustomRange(range)
    setPeriod('custom')
  }

  const handleRefresh = async () => {
    setRefreshDone(false)
    const results = await Promise.all([
      refetch(),
      refetchTimeseries(),
      refetchInsights(),
      refetchFortune(),
      refetchLoyalty(),
      refetchPromo(),
    ])
    if (results.every((r) => !r.isError)) {
      setRefreshDone(true)
      setTimeout(() => setRefreshDone(false), 2000)
    }
  }

  const updatedLabel = useMemo(() => {
    if (!data?.captured_at) return null
    try {
      return new Date(data.captured_at).toLocaleString(numberLocale, {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    } catch {
      return null
    }
  }, [data?.captured_at, numberLocale])

  const tabs: StatsTabItem<StatsTabKey>[] = [
    { key: 'overview', label: t('admin.stats.tabOverview'), icon: LayoutGrid },
    { key: 'money', label: t('admin.stats.tabMoney'), icon: Wallet },
    { key: 'users', label: t('admin.stats.tabUsers'), icon: Users },
    { key: 'mechanics', label: t('admin.stats.tabMechanics'), icon: Gift },
  ]

  // Кнопка «Обновить» — только иконка, ростом ровно с селектором периода и
  // кнопкой календаря: три разновысоких контрола в углу выглядели случайной
  // россыпью.
  const refreshLabel = refreshDone ? t('admin.stats.refreshDone') : t('admin.stats.refresh')
  const refreshButton = (
    <button
      type="button"
      onClick={() => void handleRefresh()}
      disabled={refreshing}
      aria-label={refreshLabel}
      title={refreshLabel}
      className={cn('inline-flex size-11 shrink-0 items-center justify-center rounded-lg border border-border/60 bg-card transition-colors hover:bg-accent disabled:opacity-50', refreshing && '!opacity-100')}
    >
      {refreshDone ? (
        <Check className="size-4 animate-fade-in text-emerald-500 dark:text-emerald-400" />
      ) : (
        <RefreshCw className={cn('size-4', refreshing && 'animate-spin')} />
      )}
    </button>
  )

  const periodControls = (compact: boolean) => (
    <div className={cn('flex items-center gap-2', compact && 'min-w-0 flex-1')}>
      <StatsPeriodSelector
        value={period}
        onChange={handlePeriodChange}
        customRange={customRange}
        className={compact ? 'min-w-0 flex-1' : undefined}
      />
      <StatsCustomRangePicker
        active={period === 'custom'}
        value={customRange}
        onApply={handleCustomRange}
      />
    </div>
  )

  return (
    <div className="space-y-4">
      <div
        className={cn(
          'sticky z-40 -mx-3 border-b border-border/80 bg-card/92 px-3 py-2 backdrop-blur-xl transition-[top] duration-200 ease-out sm:-mx-4 sm:px-4 dark:border-primary/12 md:hidden',
          mobileHeaderVisible
            ? 'top-[calc(3.5rem+var(--cabinet-tg-safe-top))]'
            : 'top-[var(--cabinet-tg-safe-top)]',
        )}
      >
        <div className="flex items-center gap-2">
          {periodControls(true)}
          {refreshButton}
        </div>
      </div>

      <AdminPageHeader
        icon={BarChart3}
        title={t('admin.stats.title')}
        subtitle={
          updatedLabel ? t('admin.stats.capturedAt', { date: updatedLabel }) : t('admin.stats.subtitle')
        }
        accent="blue"
        actions={
          <div className="hidden flex-wrap items-center gap-2 md:flex">
            {periodControls(false)}
            {refreshButton}
          </div>
        }
      />

      <StatsTabs items={tabs} value={tab} onChange={setTab} />

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="size-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <Card className="border-destructive/50 p-6 text-center text-sm text-destructive">
          {t('admin.stats.error')}
        </Card>
      )}

      {data && tab === 'overview' && (
        <StatsOverviewTab
          data={data}
          insights={insights}
          timeseries={timeseries}
          fortune={fortuneData}
          period={period}
          customRange={customRange}
        />
      )}

      {data && tab === 'money' && (
        <StatsMoneyTab
          data={data}
          insights={insights}
          timeseries={timeseries}
          period={period}
          customRange={customRange}
        />
      )}

      {data && tab === 'users' && (
        <StatsUsersTab
          data={data}
          insights={insights}
          timeseries={timeseries}
          period={period}
          customRange={customRange}
        />
      )}

      {data && tab === 'mechanics' && (
        <StatsMechanicsTab
          data={data}
          insights={insights}
          fortune={fortuneData}
          loyalty={loyaltyData}
          promo={promoData}
          period={period}
        />
      )}
    </div>
  )
}
