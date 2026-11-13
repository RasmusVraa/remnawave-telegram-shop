import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  AlertTriangle,
  ArrowRight,
  Banknote,
  CalendarDays,
  CalendarRange,
  Check,
  CircleCheck,
  CircleSlash,
  Clock,
  Cpu,
  Gauge,
  HardDrive,
  LayoutGrid,
  Loader2,
  Radio,
  Receipt,
  RefreshCw,
  Server,
  ShieldCheck,
  ShoppingCart,
  UserRoundX,
  UsersRound,
  Wallet,
  type LucideIcon,
} from 'lucide-react'

import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { useAdminOverview } from '../hooks/useAdminOverview'
import { incidentLabel, incidentLink } from '../monitoring/IncidentsCard'
import { StatsIconChip } from '../stats/components/StatsPanel'
import { formatRub, statsNumberLocale } from '../stats/utils/statsFormat'
import { STATS_ACCENT } from '../stats/utils/statsPalette'

/** Уровень срочности дела. Цвет плашки — по худшему из них. */
type Severity = 'warn' | 'crit'

/**
 * Статусы пользователей в панели. Ключи — как их отдаёт Remnawave; набор задаёт
 * она, поэтому неизвестный статус просто не попадёт в этот список, а не сломает
 * разбор.
 */
const PANEL_STATUSES = [
  { key: 'ACTIVE', color: STATS_ACCENT.green },
  { key: 'EXPIRED', color: STATS_ACCENT.orange },
  { key: 'LIMITED', color: STATS_ACCENT.amber },
  { key: 'DISABLED', color: STATS_ACCENT.red },
] as const

interface AttentionItem {
  key: string
  count: number
  to: string
  label: string
  severity: Severity
}

export default function AdminDashboardPage() {
  return (
    <AdminLayout>
      <AdminDashboardContent />
    </AdminLayout>
  )
}

/**
 * «Обзор» — оперативный экран: что происходит сейчас и что ждёт действия.
 *
 * Раньше здесь лежали плитки-ссылки на разделы админки, то есть меню внутри
 * бокового меню. Разделение со «Статистикой» — по горизонту: здесь «сейчас и
 * сегодня», там «как шёл период».
 *
 * Вес распределён намеренно неравномерно: одно число-герой сверху, под ним
 * полоса дел во всю ширину, и только потом три колонки подробностей мелким
 * кеглем. Когда все блоки одного веса, экран читается как обои и у глаза нет
 * точки входа.
 *
 * Панель Remnawave — внешняя зависимость. Числа магазина показываются всегда;
 * если панель не отвечает, вместо её блоков стоит честная плашка, а не спиннер
 * и не нули, выдающие себя за данные.
 */
function AdminDashboardContent() {
  const { t, i18n } = useTranslation()
  const numberLocale = statsNumberLocale(i18n.language)
  const { data, isLoading, error, refetch, isFetching } = useAdminOverview()
  const [refreshDone, setRefreshDone] = useState(false)

  const handleRefresh = async () => {
    setRefreshDone(false)
    const result = await refetch()
    if (!result.isError) {
      setRefreshDone(true)
      setTimeout(() => setRefreshDone(false), 2000)
    }
  }

  const panel = data?.panel
  const a = data?.attention

  // Уровень задаёт не размер числа, а цена промедления: неоплаченный сервер
  // выключится, заявка на выплату — это чужие деньги, которые уже ждут.
  // Заявка в партнёры или висящий счёт подождут до завтра.
  // Сначала то, что горит сейчас (из «Мониторинга»: нода лежит, поддержка
  // ждёт), потом дела на дни. Список инцидентов общий — один источник на бэке.
  const incidentItems: AttentionItem[] = (data?.incidents ?? []).map((it, i) => ({
    key: `incident-${it.key}-${i}`,
    count: it.count || 1,
    to: incidentLink(it),
    severity: it.severity,
    label: incidentLabel(it, t),
  }))

  const taskItems: AttentionItem[] = [
    {
      key: 'billingOverdue',
      count: a?.billing_overdue ?? 0,
      to: '/admin/infra',
      severity: 'crit' as const,
      label: t('admin.overview.attentionBillingOverdue', { count: a?.billing_overdue ?? 0 }),
    },
    {
      key: 'billingDueUrgent',
      count: a?.billing_due_urgent ?? 0,
      to: '/admin/infra',
      severity: 'crit' as const,
      label: t('admin.overview.attentionBillingUrgent', { count: a?.billing_due_urgent ?? 0 }),
    },
    {
      key: 'partnerPayouts',
      count: a?.partner_payouts ?? 0,
      to: '/admin/partners?tab=payouts',
      severity: 'crit' as const,
      label: t('admin.overview.attentionPartnerPayouts', { count: a?.partner_payouts ?? 0 }),
    },
    {
      key: 'billingDueSoon',
      count: a?.billing_due_soon ?? 0,
      to: '/admin/infra',
      severity: 'warn' as const,
      label: t('admin.overview.attentionBillingDueSoon', { count: a?.billing_due_soon ?? 0 }),
    },
    {
      key: 'fortuneLoss',
      count: a?.fortune_net_loss_days ?? 0,
      to: '/admin/stats?tab=mechanics',
      severity: 'warn' as const,
      label: t('admin.overview.attentionFortuneLoss', {
        count: a?.fortune_net_loss_days ?? 0,
      }),
    },
    {
      key: 'partnerApplications',
      count: a?.partner_applications ?? 0,
      to: '/admin/partners?tab=applications',
      severity: 'warn' as const,
      label: t('admin.overview.attentionPartnerApplications', {
        count: a?.partner_applications ?? 0,
      }),
    },
  ].filter((item) => item.count > 0)
  const attentionItems = [...incidentItems, ...taskItems]

  const worst: Severity | null = attentionItems.some((i) => i.severity === 'crit')
    ? 'crit'
    : attentionItems.length > 0
      ? 'warn'
      : null

  return (
    <div className="space-y-4">
      <AdminPageHeader
        icon={LayoutGrid}
        title={t('admin.overview.title')}
        subtitle={t('admin.overview.subtitle')}
        accent="blue"
        actions={
          <button
            type="button"
            onClick={() => void handleRefresh()}
            disabled={isFetching}
            aria-label={refreshDone ? t('admin.stats.refreshDone') : t('admin.stats.refresh')}
            title={refreshDone ? t('admin.stats.refreshDone') : t('admin.stats.refresh')}
            className={cn('inline-flex size-11 shrink-0 items-center justify-center rounded-lg border border-border/60 bg-card transition-colors hover:bg-accent disabled:opacity-50', isFetching && '!opacity-100')}
          >
            {refreshDone ? (
              <Check className="size-4 animate-fade-in text-emerald-500 dark:text-emerald-400" />
            ) : (
              <RefreshCw className={cn('size-4', isFetching && 'animate-spin')} />
            )}
          </button>
        }
      />

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="size-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <Card className="border-destructive/50 p-6 text-center text-sm text-destructive">
          {t('admin.overview.error')}
        </Card>
      )}

      {data && (
        <>
          <HeroBlock data={data} numberLocale={numberLocale} t={t} />

          <AttentionBar items={attentionItems} worst={worst} t={t} />

          {!panel?.available && (
            <Card className="flex items-center gap-3 border-amber-500/40 p-4 text-sm sm:px-5">
              <CircleSlash className="size-4 shrink-0 text-amber-500" />
              <span className="text-muted-foreground">
                {panel?.reason === 'not_configured'
                  ? t('admin.overview.panelNotConfigured')
                  : t('admin.overview.panelUnreachable')}
              </span>
            </Card>
          )}

          <div className="grid gap-4 lg:grid-cols-3">
            {panel?.available && (
              <DetailCard
                title={t('admin.overview.trafficTitle')}
                icon={Gauge}
                color={STATS_ACCENT.cyan}
                rows={[
                  {
                    icon: CalendarRange,
                    label: t('admin.overview.periodWeek'),
                    value: panel.traffic.last_seven_days.current || '—',
                  },
                  {
                    icon: CalendarRange,
                    label: t('admin.overview.periodThirty'),
                    value: panel.traffic.last_thirty_days.current || '—',
                  },
                  {
                    icon: CalendarDays,
                    label: t('admin.overview.periodCalendarMonth'),
                    value: panel.traffic.calendar_month.current || '—',
                  },
                  {
                    icon: CalendarRange,
                    label: t('admin.overview.periodYear'),
                    value: panel.traffic.current_year.current || '—',
                  },
                ]}
              />
            )}

            {panel?.available && (
              <DetailCard
                title={t('admin.overview.systemTitle')}
                icon={Server}
                color={STATS_ACCENT.blue}
                rows={[
                  {
                    icon: HardDrive,
                    label: t('admin.overview.totalTraffic'),
                    value: formatBytes(panel.system.total_bytes_lifetime, numberLocale),
                  },
                  {
                    icon: Cpu,
                    label: t('admin.overview.memory'),
                    value: `${formatGiB(panel.system.memory_used)} / ${formatGiB(panel.system.memory_total)}`,
                  },
                  {
                    icon: Clock,
                    label: t('admin.overview.uptime'),
                    value: formatUptime(panel.system.uptime_seconds, t),
                  },
                  {
                    icon: UsersRound,
                    label: t('admin.overview.inPanel'),
                    value: panel.panel_users.total.toLocaleString(numberLocale),
                  },
                ]}
              />
            )}

            <DetailCard
              title={t('admin.overview.moneyTitle')}
              icon={Wallet}
              color={STATS_ACCENT.green}
              rows={[
                {
                  icon: Banknote,
                  label: t('admin.overview.today'),
                  value: formatRub(data.shop.revenue_today_rub, numberLocale),
                },
                {
                  icon: Banknote,
                  label: t('admin.overview.thisMonth'),
                  value: formatRub(data.shop.revenue_month_rub, numberLocale),
                },
                {
                  icon: ShoppingCart,
                  label: t('admin.overview.salesTodayRow'),
                  value: data.shop.sales_today.toLocaleString(numberLocale),
                },
                {
                  icon: Receipt,
                  label: t('admin.overview.payersTodayRow'),
                  value: data.shop.payers_today.toLocaleString(numberLocale),
                },
              ]}
              footer={
                <Link
                  to="/admin/stats"
                  className="mt-3 inline-flex items-center gap-1.5 text-[13px] font-medium text-primary hover:underline"
                >
                  {t('admin.overview.moreStatsLink')}
                  <ArrowRight className="size-3.5 shrink-0" />
                </Link>
              }
            />
          </div>
        </>
      )}
    </div>
  )
}

/**
 * Герой: одно число во всю ширину.
 *
 * Справа — не график, а разбивка онлайна. Почасового ряда панель не отдаёт
 * (только «сейчас», «за сутки», «за неделю» и «ни разу»), и рисовать линию по
 * четырём точкам, растянув их на сутки, значило бы выдумать данные.
 */
function HeroBlock({
  data,
  numberLocale,
  t,
}: {
  data: NonNullable<ReturnType<typeof useAdminOverview>['data']>
  numberLocale: string
  t: TFunction
}) {
  const panel = data.panel
  const online = panel.available ? panel.online : null
  const scaleBase = Math.max(online?.now ?? 0, online?.today ?? 0, online?.week ?? 0, 1)

  // Состав пользователей панели. Слева в герое — сколько людей на связи,
  // справа — что у них с подпиской: это разные вопросы, а не одно и то же
  // число в другой обёртке. Данные уже приходят в ответе, но нигде не видны.
  const statusCounts = panel.available ? panel.panel_users.status_counts : {}
  const statuses = PANEL_STATUSES.map((st) => ({
    ...st,
    label: t(`admin.overview.panelStatus.${st.key}`),
    value: statusCounts[st.key] ?? 0,
  }))
  const statusTotal = statuses.reduce((sum, st) => sum + st.value, 0)

  return (
    <Card className="cabinet-elevated-card stats-ring p-5 sm:p-6">
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)] lg:items-center">
        <div>
          <div className="flex items-center gap-2">
            <span className="relative flex size-2.5 shrink-0">
              <span
                className="absolute inline-flex size-full rounded-full opacity-60 motion-safe:animate-ping"
                style={{ backgroundColor: STATS_ACCENT.green }}
              />
              <span
                className="relative inline-flex size-2.5 rounded-full"
                style={{ backgroundColor: STATS_ACCENT.green }}
              />
            </span>
            <span className="text-sm text-muted-foreground">{t('admin.overview.onlineNow')}</span>
          </div>

          <div className="mt-2 font-heading text-6xl font-extrabold leading-none tracking-tight tabular-nums sm:text-7xl">
            {panel.available ? (online?.now ?? 0).toLocaleString(numberLocale) : '—'}
          </div>

          <div className="mt-3.5 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-[13px] text-muted-foreground">
            <span className="inline-flex items-center gap-1.5">
              <Clock className="size-3.5 shrink-0" aria-hidden />
              {t('admin.overview.heroToday', { count: online?.today ?? 0 })}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <UsersRound className="size-3.5 shrink-0" aria-hidden />
              {t('admin.overview.heroWeek', { count: online?.week ?? 0 })}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <UserRoundX className="size-3.5 shrink-0" aria-hidden />
              {t('admin.overview.heroNever', { count: online?.never_online ?? 0 })}
            </span>
          </div>

          <div className="mt-5 grid grid-cols-2 gap-4 sm:grid-cols-3">
            <HeroStat
              icon={Gauge}
              color={STATS_ACCENT.amber}
              label={t('admin.overview.trafficToday')}
              value={panel.available ? panel.traffic.today.current || '—' : '—'}
            />
            <HeroStat
              icon={ShieldCheck}
              color={STATS_ACCENT.cyan}
              label={t('admin.overview.activeSubscriptions')}
              value={data.shop.active_subscriptions.toLocaleString(numberLocale)}
            />
            <HeroStat
              icon={UsersRound}
              color={STATS_ACCENT.blue}
              label={t('admin.overview.totalCustomers')}
              value={data.shop.total_customers.toLocaleString(numberLocale)}
            />
          </div>
        </div>

        <div className="rounded-2xl border border-border/50 bg-muted/20 p-4 sm:p-5">
          <div className="mb-3.5 flex items-baseline justify-between gap-3">
            <p className="flex items-center gap-2 text-[13px] text-muted-foreground">
              <UsersRound className="size-3.5 shrink-0" aria-hidden />
              {t('admin.overview.panelUsersTitle')}
            </p>
            <p className="shrink-0 text-[13px] font-semibold tabular-nums">
              {panel.available ? statusTotal.toLocaleString(numberLocale) : '—'}
            </p>
          </div>

          {panel.available ? (
            <>
              <div className="mb-4 flex h-2.5 gap-0.5 overflow-hidden rounded-full">
                {statuses.map((st) => (
                  <div
                    key={st.key}
                    style={{
                      width: `${statusTotal > 0 ? (st.value * 100) / statusTotal : 0}%`,
                      backgroundColor: st.color,
                    }}
                  />
                ))}
              </div>
              <div className="grid grid-cols-2 gap-x-4 gap-y-2.5">
                {statuses.map((st) => (
                  <div key={st.key} className="flex items-center gap-2 text-[13px]">
                    <span
                      className="size-2.5 shrink-0 rounded-[3px]"
                      style={{ backgroundColor: st.color }}
                      aria-hidden
                    />
                    <span className="min-w-0 flex-1 truncate text-muted-foreground">
                      {st.label}
                    </span>
                    <span className="shrink-0 font-semibold tabular-nums">
                      {st.value.toLocaleString(numberLocale)}
                    </span>
                  </div>
                ))}
              </div>
              <p className="mt-4 flex items-center gap-2 border-t border-border/50 pt-3 text-xs text-muted-foreground">
                <UserRoundX className="size-3.5 shrink-0" aria-hidden />
                {t('admin.overview.neverOnlineNote', {
                  count: online?.never_online ?? 0,
                  total: statusTotal.toLocaleString(numberLocale),
                })}
              </p>
            </>
          ) : (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t('admin.overview.panelUnreachableShort')}
            </p>
          )}
        </div>
      </div>
    </Card>
  )
}

function HeroStat({
  icon: Icon,
  color,
  label,
  value,
}: {
  icon: LucideIcon
  color: string
  label: string
  value: string
}) {
  return (
    <div className="min-w-0">
      <div className="flex min-h-[2.1rem] items-start gap-1.5">
        <Icon className="mt-0.5 size-3.5 shrink-0" style={{ color }} aria-hidden />
        <span className="text-xs leading-tight text-muted-foreground">{label}</span>
      </div>
      <p className="mt-1 truncate font-heading text-xl font-bold tabular-nums sm:text-[22px]">
        {value}
      </p>
    </div>
  )
}

/**
 * Полоса дел во всю ширину, сразу под героем.
 *
 * Цвет плашки — по худшему делу в списке, а не по их количеству: одна
 * просроченная оплата сервера важнее пяти висящих счетов. Когда чинить нечего,
 * полоса не исчезает, а становится зелёной: «дел нет» — это тоже ответ, и его
 * лучше увидеть, чем гадать, загрузилось ли вообще.
 */
function AttentionBar({
  items,
  worst,
  t,
}: {
  items: AttentionItem[]
  worst: Severity | null
  t: TFunction
}) {
  if (worst === null) {
    return (
      <div className="flex items-center gap-3 rounded-[--radius] border border-emerald-500/40 bg-emerald-500/[0.07] p-4 sm:px-5">
        <CircleCheck className="size-5 shrink-0 text-emerald-500" />
        <span className="text-sm font-medium">{t('admin.overview.attentionClear')}</span>
      </div>
    )
  }

  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-x-4 gap-y-2.5 rounded-[--radius] border p-4 sm:px-5',
        worst === 'crit'
          ? 'border-rose-500/45 bg-gradient-to-r from-rose-500/[0.16] via-rose-500/[0.07] to-transparent'
          : 'border-amber-500/45 bg-gradient-to-r from-amber-500/[0.16] via-amber-500/[0.07] to-transparent',
      )}
    >
      <span className="flex items-center gap-2 text-sm font-semibold">
        <AlertTriangle
          className={cn('size-4 shrink-0', worst === 'crit' ? 'text-rose-500' : 'text-amber-500')}
        />
        {t('admin.overview.attentionTitle')}
      </span>
      {items.map((item) => (
        <Link
          key={item.key}
          to={item.to}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-[13px] transition-colors',
            item.severity === 'crit' &&
              'border-rose-500/50 bg-rose-500/10 text-rose-500 hover:border-rose-500',
            item.severity === 'warn' &&
              'border-amber-500/50 bg-amber-500/10 text-amber-600 hover:border-amber-500 dark:text-amber-400',
          )}
        >
          {item.label}
          <ArrowRight className="size-3.5 shrink-0" />
        </Link>
      ))}
    </div>
  )
}

function DetailCard({
  title,
  icon,
  color,
  rows,
  footer,
}: {
  title: string
  icon: LucideIcon
  color: string
  rows: { icon: LucideIcon; label: string; value: string }[]
  footer?: React.ReactNode
}) {
  return (
    <Card className="cabinet-elevated-card stats-ring flex flex-col p-4 sm:px-5">
      <div className="mb-1 flex items-center gap-2.5">
        <StatsIconChip icon={icon} color={color} size="sm" />
        <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {title}
        </h2>
      </div>
      {rows.map((row, i) => {
        const RowIcon = row.icon
        return (
          <div
            key={row.label}
            className={cn(
              'flex items-baseline justify-between gap-3 py-2.5',
              i < rows.length - 1 && 'border-b border-border/40',
            )}
          >
            <span className="inline-flex min-w-0 items-center gap-2 text-[13px] text-muted-foreground">
              <RowIcon className="size-3.5 shrink-0" aria-hidden />
              <span className="truncate">{row.label}</span>
            </span>
            <span className="shrink-0 text-[15px] font-semibold tabular-nums">{row.value}</span>
          </div>
        )
      })}
      {footer}
    </Card>
  )
}

/**
 * totalBytesLifetime приходит строкой с сырыми байтами — в отличие от блоков
 * трафика, которые панель форматирует сама. Из-за этого на экране висело
 * «132114059095594» вместо «120.12 TiB».
 */
function formatBytes(raw: string, locale: string): string {
  const bytes = Number(raw)
  if (!Number.isFinite(bytes) || bytes <= 0) return '—'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  const digits = value >= 100 ? 1 : 2
  return `${value.toLocaleString(locale, { minimumFractionDigits: digits, maximumFractionDigits: digits })} ${units[unit]}`
}

function formatGiB(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '—'
  return `${(bytes / 1024 ** 3).toFixed(1)} GiB`
}

function formatUptime(seconds: number, t: TFunction): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  if (days > 0) return t('admin.overview.uptimeDays', { days, hours })
  return t('admin.overview.uptimeHours', { hours })
}
