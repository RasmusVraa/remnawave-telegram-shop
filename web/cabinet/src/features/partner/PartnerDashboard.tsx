import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  TrendingUp,
  Briefcase,
  LayoutGrid,
  Link2,
  Users,
  Receipt,
  Banknote,
  CreditCard,
  Wallet,
  Clock,
  HandCoins,
  PiggyBank,
  type LucideIcon,
} from 'lucide-react'

import { RevealItem } from '@/components/PageReveal'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { hapticSelection } from '@/lib/haptic'
import type { PartnerAccountDTO, PartnerStateResponse } from '@/lib/api'

import { PartnerLinksTab } from './PartnerLinksTab'
import { PartnerCustomersTab, PartnerEarningsTab } from './PartnerListTab'
import { PartnerPayoutsTab } from './PartnerPayoutsTab'
import { formatMoney, formatMonthShort, formatMonthLong, formatDayMonth, formatPercent } from './format'

type TabId = 'overview' | 'links' | 'customers' | 'earnings' | 'payouts'

const TABS: { id: TabId; labelKey: string; icon: LucideIcon }[] = [
  { id: 'overview', labelKey: 'partnerPage.tabs.overview', icon: LayoutGrid },
  { id: 'links', labelKey: 'partnerPage.tabs.links', icon: Link2 },
  { id: 'customers', labelKey: 'partnerPage.tabs.customers', icon: Users },
  { id: 'earnings', labelKey: 'partnerPage.tabs.earnings', icon: Receipt },
  { id: 'payouts', labelKey: 'partnerPage.tabs.payouts', icon: Banknote },
]

export function PartnerDashboard({
  state,
  partner,
}: {
  state: PartnerStateResponse
  partner: PartnerAccountDTO
}) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<TabId>('overview')

  return (
    <>
      <RevealItem>
        <div className="flex items-center justify-between gap-2">
          <div
            role="tablist"
            aria-label={t('partnerPage.title')}
            className="flex w-full gap-1 overflow-x-auto rounded-xl bg-muted p-1"
          >
            {TABS.map((item) => {
              const Icon = item.icon
              return (
                <button
                  key={item.id}
                  role="tab"
                  type="button"
                  aria-selected={tab === item.id}
                  onClick={() => {
                    if (tab !== item.id) hapticSelection()
                    setTab(item.id)
                  }}
                  className={cn(
                    'flex flex-1 items-center justify-center gap-1.5 whitespace-nowrap rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors',
                    tab === item.id
                      ? 'bg-card text-foreground shadow-sm'
                      : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  <Icon size={14} className={cn('shrink-0', tab === item.id && 'text-primary')} />
                  {t(item.labelKey)}
                </button>
              )
            })}
          </div>
        </div>
      </RevealItem>

      {state.status === 'suspended' ? (
        <RevealItem>
          <Card className="border-amber-500/30 bg-amber-500/5">
            <CardContent className="pt-5 text-sm text-amber-700 dark:text-amber-400">
              {t('partnerPage.suspendedNotice')}
            </CardContent>
          </Card>
        </RevealItem>
      ) : null}

      {tab === 'overview' ? <PartnerOverview state={state} partner={partner} onWithdraw={() => setTab('payouts')} /> : null}
      {tab === 'links' ? <PartnerLinksTab partner={partner} /> : null}
      {tab === 'customers' ? <PartnerCustomersTab /> : null}
      {tab === 'earnings' ? <PartnerEarningsTab /> : null}
      {tab === 'payouts' ? <PartnerPayoutsTab partner={partner} /> : null}
    </>
  )
}

function PartnerOverview({
  state,
  partner,
  onWithdraw,
}: {
  state: PartnerStateResponse
  partner: PartnerAccountDTO
  onWithdraw: () => void
}) {
  const { t } = useTranslation()
  const summary = partner.summary
  const belowMinimum = partner.balance < state.terms.min_payout

  return (
    <>
      <RevealItem>
        <Card className="border-primary/15 bg-gradient-to-br from-card via-card to-primary/5">
          <CardContent className="pt-4 sm:pt-6">
            <p className="flex items-center gap-1.5 text-xs uppercase tracking-wide text-muted-foreground">
              <Wallet size={13} className="shrink-0 text-primary" />
              {t('partnerPage.overview.available')}
            </p>
            <p className="mt-1 text-3xl font-semibold text-primary">{formatMoney(partner.balance)}</p>

            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <BalanceCell
                icon={Clock}
                label={t('partnerPage.overview.onHold')}
                value={formatMoney(partner.hold_balance)}
                sub={
                  partner.next_hold_release_at
                    ? t('partnerPage.overview.opensAt', { date: formatDayMonth(partner.next_hold_release_at) })
                    : undefined
                }
              />
              <BalanceCell
                icon={PiggyBank}
                label={t('partnerPage.overview.paidOut')}
                value={formatMoney(partner.total_paid)}
                sub={
                  partner.reserved_balance > 0
                    ? t('partnerPage.overview.reserved', { amount: formatMoney(partner.reserved_balance) })
                    : undefined
                }
              />
            </div>

            <Button
              className="mt-4 w-full gap-2"
              onClick={onWithdraw}
              disabled={!partner.can_withdraw || belowMinimum || partner.has_open_payout}
            >
              <HandCoins size={16} className="shrink-0" />
              {partner.has_open_payout
                ? t('partnerPage.overview.payoutPending')
                : belowMinimum
                  ? t('partnerPage.overview.belowMinimum', { min: formatMoney(state.terms.min_payout) })
                  : t('partnerPage.overview.withdraw', { amount: formatMoney(partner.balance) })}
            </Button>
          </CardContent>
        </Card>
      </RevealItem>

      <RevealItem className="grid gap-3 sm:grid-cols-3">
        <StatCard
          icon={Users}
          label={t('partnerPage.overview.customers')}
          value={String(summary.customers)}
          sub={t('partnerPage.overview.customersWeek', { n: summary.customers_last_week })}
        />
        <StatCard
          icon={CreditCard}
          label={t('partnerPage.overview.paying')}
          value={String(summary.paying)}
          sub={t('partnerPage.overview.conversion', { n: summary.conversion_pct })}
        />
        <StatCard
          icon={Wallet}
          label={t('partnerPage.overview.earned')}
          value={formatMoney(summary.earned_total)}
          sub={t('partnerPage.overview.earnedMonth', { amount: formatMoney(summary.earned_last_month) })}
        />
      </RevealItem>

      {partner.months.length > 0 ? (
        <RevealItem>
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="flex items-center gap-2 text-base font-medium">
                <TrendingUp size={18} className="text-primary" />
                {t('partnerPage.overview.chartTitle')}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <MonthsChart months={partner.months} />
            </CardContent>
          </Card>
        </RevealItem>
      ) : null}

      <RevealItem>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base font-medium">
              <Briefcase size={18} className="text-primary" />
              {t('partnerPage.terms.title')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="divide-y divide-border rounded-lg border border-border text-sm">
              <TermRow
                label={t('partnerPage.terms.firstPayment')}
                value={<Badge>{formatPercent(partner.first_percent)}</Badge>}
              />
              <TermRow
                label={t('partnerPage.terms.renewals')}
                value={<Badge>{formatPercent(partner.renewal_percent)}</Badge>}
              />
              <TermRow
                label={t('partnerPage.terms.hold')}
                value={t('partnerPage.terms.holdValue', { days: state.terms.hold_days })}
              />
              <TermRow label={t('partnerPage.terms.minPayout')} value={formatMoney(state.terms.min_payout)} />
              <TermRow
                label={t('partnerPage.terms.cooldown')}
                value={t('partnerPage.terms.cooldownValue', { days: state.terms.payout_cooldown_days })}
              />
            </dl>
          </CardContent>
        </Card>
      </RevealItem>
    </>
  )
}

/**
 * Столбцы начислений по месяцам.
 *
 * Inline SVG вместо библиотеки графиков: шесть значений не стоят лишней
 * зависимости в бандле кабинета.
 */
function MonthsChart({ months }: { months: { month: string; amount: number }[] }) {
  const { t } = useTranslation()
  // По умолчанию подсвечен последний месяц — он же обычно и самый интересный.
  const [active, setActive] = useState(months.length - 1)
  const current = months[Math.min(Math.max(active, 0), months.length - 1)]
  const max = Math.max(...months.map((m) => m.amount), 1)
  const width = 300
  const height = 100
  const gap = 8
  const barWidth = Math.max((width - gap * (months.length - 1)) / months.length, 4)

  return (
    <div>
      {/* Значение выбранного месяца — подписью над графиком, а не всплывающей
          подсказкой: на телефоне hover'а нет, и подсказка была бы недоступна
          ровно там, где кабинет открывают чаще всего. */}
      <div className="mb-2 flex items-baseline justify-between gap-2">
        <span className="text-sm font-semibold tabular-nums">{formatMoney(current?.amount ?? 0)}</span>
        <span className="text-xs text-muted-foreground">{formatMonthLong(current?.month ?? '')}</span>
      </div>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        width="100%"
        height={height}
        role="img"
        aria-label={t('partnerPage.overview.chartAlt', {
          from: formatMoney(months[0]?.amount ?? 0),
          to: formatMoney(months[months.length - 1]?.amount ?? 0),
        })}
      >
        {months.map((m, i) => {
          const barHeight = Math.max((m.amount / max) * (height - 20), 2)
          const cx = i * (barWidth + gap) + barWidth / 2
          return (
            <g key={m.month}>
              {i === active ? (
                <text
                  x={cx}
                  y={Math.max(height - barHeight - 4, 9)}
                  textAnchor="middle"
                  className="fill-muted-foreground"
                  fontSize={9}
                >
                  {formatMoney(m.amount)}
                </text>
              ) : null}
            <rect
              x={i * (barWidth + gap)}
              y={height - barHeight}
              width={barWidth}
              height={barHeight}
              rx={4}
              tabIndex={0}
              role="button"
              aria-label={`${formatMonthLong(m.month)}: ${formatMoney(m.amount)}`}
              className={cn('cursor-pointer outline-none', i === active ? 'fill-primary' : 'fill-primary/25')}
              onMouseEnter={() => setActive(i)}
              onFocus={() => setActive(i)}
              onClick={() => setActive(i)}
            />
            </g>
          )
        })}
      </svg>
      <div className="mt-1 flex justify-between text-xs">
        {months.map((m, i) => (
          <span key={m.month} className={i === active ? 'font-medium text-foreground' : 'text-muted-foreground'}>
            {formatMonthShort(m.month)}
          </span>
        ))}
      </div>
    </div>
  )
}

/** Ячейка внутри карточки баланса: холд и выплаченное с подписями. */
function BalanceCell({
  icon: Icon,
  label,
  value,
  sub,
}: {
  icon: LucideIcon
  label: string
  value: string
  sub?: string
}) {
  return (
    <div>
      <p className="flex items-center gap-1.5 text-xs uppercase tracking-wide text-muted-foreground">
        <Icon size={13} className="shrink-0 text-primary" />
        {label}
      </p>
      <p className="mt-0.5 font-semibold tabular-nums">{value}</p>
      {sub ? <p className="text-xs text-muted-foreground">{sub}</p> : null}
    </div>
  )
}

function StatCard({
  icon: Icon,
  label,
  value,
  sub,
}: {
  icon: LucideIcon
  label: string
  value: string
  sub: string
}) {
  return (
    <Card>
      <CardContent className="pt-4">
        <p className="flex items-center gap-1.5 text-xs uppercase tracking-wide text-muted-foreground">
          <Icon size={13} className="shrink-0 text-primary" />
          {label}
        </p>
        <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
        <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>
      </CardContent>
    </Card>
  )
}

function TermRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2.5">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-medium">{value}</dd>
    </div>
  )
}
