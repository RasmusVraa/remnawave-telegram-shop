import { type ReactNode, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { CalendarPlus, Check, ChevronRight, Infinity as InfinityIcon, MonitorSmartphone, Zap, type LucideIcon } from 'lucide-react'

import { AddDeviceIcon } from '@/components/AddDeviceIcon'
import { DevicePlatformIcon } from '@/components/DevicePlatformIcon'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { useToast } from '@/components/ui/toast'
import { StatRing, UnboundedRing } from '@/components/StatRing'
import { ConnectDeviceButton, RenewNowButton, RenewWarnButton } from '@/components/SubscriptionActions'
import { SubscriptionStatusBadge } from '@/components/SubscriptionStatusBadge'
import { formatDayMonth } from '@/features/subscription/DeviceSlots'
import type { SubscriptionResponse, TrafficUsageResponse } from '@/lib/api'
import { cn, formatDate } from '@/lib/utils'
import type { ToneLevel } from '@/lib/subscriptionTone'
import { useDeviceManager } from './DeviceManager'
import {
  DEFAULT_HOME_QUICK_ACTIONS,
  HOME_QUICK_ACTIONS,
  resolveHomeQuickActions,
  type HomeQuickActionId,
} from './homeQuickActions'
import { formatGbValue, type HomeData } from './useHomeData'

/** Опорный горизонт для кольца дней: 90 суток закрывают типичные периоды подписки. */
const DAYS_RING_HORIZON = 90

export function subscriptionTariffLabel(sub: SubscriptionResponse | null | undefined, t: TFunction): string {
  if (!sub) return t('dashboard.basicPlan')
  if (sub.is_trial) return t('dashboard.trialTariffLabel')
  const raw = sub.tariff?.name
  const name = typeof raw === 'string' ? raw.trim() : ''
  if (name) return name
  if (sub.tariff?.slug === 'classic') return t('dashboard.classicTariffFallback')
  return t('dashboard.basicPlan')
}

// ─── Заголовок: тариф, дата, чип ───────────────────────────────────────

export function HomeTitle({ data, lang, compact }: { data: HomeData; lang: string; compact?: boolean }) {
  const { t } = useTranslation()
  const { sub, state, isActive, effectiveDays, isTrial } = data
  const expired = state === 'expired'
  const date = sub?.expire_at ? formatDate(sub.expire_at, lang) : ''
  return (
    <div className={cn('flex justify-between gap-3', compact ? 'items-start' : 'items-end')}>
      <div className="min-w-0">
        <h1
          className={cn(
            'font-heading font-extrabold leading-[1.1] tracking-[-0.02em]',
            compact ? 'text-2xl' : 'text-[34px]',
            expired && 'text-muted-foreground',
          )}
        >
          {subscriptionTariffLabel(sub, t)}
        </h1>
        <p className={cn('mt-1 truncate text-muted-foreground', compact ? 'text-sm' : 'text-[15px]')}>
          {date ? (expired ? t('home.expiredSince', { date }) : t('dashboard.untilDate', { date })) : ''}
        </p>
      </div>
      <SubscriptionStatusBadge
        days={effectiveDays}
        isActive={isActive}
        isTrial={isTrial}
        trafficExhausted={state === 'traffic'}
      />
    </div>
  )
}

// ─── Три кольца ─────────────────────────────────────────────────────────

const TONE_TEXT: Record<ToneLevel, string> = {
  calm: 'text-foreground',
  warn: 'text-amber-600 dark:text-amber-400',
  danger: 'text-destructive',
}

function RingCard({
  children,
  label,
  labelClass,
  onClick,
  chevron = true,
  compact,
}: {
  children: ReactNode
  label: ReactNode
  labelClass?: string
  onClick?: () => void
  /** Шеврон у подписи — знак, что карточка открывает подробности. Только при onClick. */
  chevron?: boolean
  compact?: boolean
}) {
  const cls = cn(
    'cabinet-elevated-card flex min-w-0 flex-col items-center gap-2 text-center',
    // На телефоне карточка — контейнер: размер подписи считается от её ширины.
    compact ? 'px-1 pb-3 pt-3.5 [container-type:inline-size]' : 'px-2 pb-4 pt-[18px]',
    onClick && 'cursor-pointer transition-transform active:scale-[0.97] active:duration-100',
  )
  const inner = (
    <>
      {children}
      <span
        className={cn(
          'whitespace-nowrap font-semibold uppercase leading-[1.3] text-muted-foreground',
          // Подпись всегда в одну строку вместе с шевроном: на узком телефоне
          // (карточка уже ~90 px) шрифт уменьшается под ширину карточки.
          compact ? 'text-[length:min(11px,12cqi)] tracking-[0.04em]' : 'text-[11px] tracking-[0.08em]',
          labelClass,
        )}
      >
        {label}
        {onClick && chevron && (
          <ChevronRight aria-hidden className="-mr-1 ml-px inline-block h-3 w-3 align-[-2px]" strokeWidth={2.5} />
        )}
      </span>
    </>
  )
  if (onClick) {
    return (
      <button type="button" onClick={onClick} className={cls}>
        {inner}
      </button>
    )
  }
  return <div className={cls}>{inner}</div>
}

export function HomeRings({
  data,
  compact,
  onTraffic,
  onDevices,
  devicesChevron = true,
}: {
  data: HomeData
  compact?: boolean
  onTraffic?: () => void
  onDevices?: () => void
  /** На ПК «Мои устройства» и так раскрыты ниже — карточка только прокручивает к ним. */
  devicesChevron?: boolean
}) {
  const { t } = useTranslation()
  const { isExpiredByDate, days, dayTone, trafficLimit, trafficUsed, trafficPct, trafficTone, connected, deviceLimit, allSlotsTaken } = data
  const size = compact ? 64 : 76
  // При исчерпанном трафике срок ещё идёт — кольцо дней показывает его как есть.
  const shownDays = !isExpiredByDate && days !== null ? Math.max(0, days) : 0
  const numCls = cn('font-heading font-extrabold', compact ? 'text-lg' : 'text-xl')

  return (
    <div className={cn('grid grid-cols-3', compact ? 'gap-2' : 'gap-3')}>
      <RingCard label={t('home.statDays', { count: shownDays })} compact={compact}>
        <StatRing value={Math.min(100, (shownDays / DAYS_RING_HORIZON) * 100)} tone={dayTone} size={size}>
          <span className={numCls}>{shownDays}</span>
        </StatRing>
      </RingCard>

      <RingCard
        compact={compact}
        onClick={onTraffic}
        label={
          trafficLimit !== null
            ? t('home.trafficUsedOfLimit', { used: formatGbValue(trafficUsed), limit: formatGbValue(trafficLimit) })
            : t('home.statTraffic')
        }
      >
        {trafficPct === null ? (
          <UnboundedRing size={size} />
        ) : (
          <StatRing value={trafficPct} tone={trafficTone} size={size}>
            <span className={cn('font-heading font-extrabold', compact ? 'text-[13px]' : 'text-[15px]', TONE_TEXT[trafficTone])}>
              {Math.round(trafficPct)}%
            </span>
          </StatRing>
        )}
      </RingCard>

      <RingCard
        compact={compact}
        onClick={onDevices}
        chevron={devicesChevron}
        labelClass={allSlotsTaken ? 'font-bold text-amber-600 dark:text-amber-400' : undefined}
        label={
          allSlotsTaken
            ? t(compact ? 'home.devicesAllTakenShort' : 'home.devicesAllTaken')
            : t('home.statDevices', { count: connected })
        }
      >
        <StatRing value={deviceLimit > 0 ? (connected / deviceLimit) * 100 : 0} tone={data.devicesTone} size={size}>
          <span className={numCls}>{connected}</span>
        </StatRing>
      </RingCard>
    </div>
  )
}

// ─── Подсказка по состоянию ─────────────────────────────────────────────

const ALERT_BASE = 'flex rounded-2xl px-3.5 py-3 text-sm leading-[1.35]'
const ALERT_WARN = 'bg-amber-500/[0.12] text-amber-800 dark:text-amber-200'
const ALERT_CRIT = 'bg-destructive/[0.12] text-red-700 dark:text-red-200'

/**
 * Подсказка под кольцами (SPEC §6). На ПК дополнительно предупреждение о
 * трафике ≥70% — на телефоне его роль играет жёлтая карточка «Трафик».
 */
export function HomeAlert({ data, desktop }: { data: HomeData; desktop?: boolean }) {
  const { t } = useTranslation()
  const { state, days, trafficTone, trafficLimit, trafficUsed } = data

  if (state === 'warn') {
    return (
      <div className={cn(ALERT_BASE, ALERT_WARN, 'flex-col items-stretch gap-2.5')}>
        <span>{t('home.alertWarn')}</span>
        <RenewWarnButton className={desktop ? 'sm:w-auto sm:self-start sm:px-6' : undefined} />
      </div>
    )
  }
  if (state === 'crit') {
    return <div className={cn(ALERT_BASE, ALERT_CRIT)}>{t('home.alertCrit', { count: Math.max(1, days ?? 1) })}</div>
  }
  if (state === 'expired') {
    return <div className={cn(ALERT_BASE, ALERT_CRIT)}>{t('home.alertExpired')}</div>
  }
  if (state === 'traffic') {
    return <div className={cn(ALERT_BASE, ALERT_CRIT)}>{t('dashboard.subscriptionExpiredTrafficHint')}</div>
  }
  if (desktop && trafficLimit !== null && trafficTone !== 'calm') {
    const left = Math.max(0, trafficLimit - trafficUsed)
    return (
      <div className={cn(ALERT_BASE, trafficTone === 'danger' ? ALERT_CRIT : ALERT_WARN, 'items-center gap-3')}>
        <span className="min-w-0 flex-1">
          {t('home.alertTrafficWarn', { left: formatGbValue(left), limit: formatGbValue(trafficLimit) })}
        </span>
        <Link
          to="/tariffs"
          className="ml-auto inline-flex h-9 shrink-0 items-center rounded-full bg-amber-500 px-4 text-sm font-bold text-amber-950 transition-[filter] hover:brightness-110"
        >
          {t('home.changeTariff')}
        </Link>
      </div>
    )
  }
  return null
}

// ─── Главная кнопка ─────────────────────────────────────────────────────

export function HomeMainButton({ data }: { data: HomeData }) {
  const { state, effectiveDays, connected, deviceLimit, isTrial } = data
  if (state === 'expired' || state === 'traffic') return <RenewNowButton expired />
  if (state === 'crit') return <RenewNowButton expired={false} />
  return (
    <ConnectDeviceButton
      days={effectiveDays}
      devicesUsed={connected}
      devicesLimit={deviceLimit}
      isTrial={isTrial}
      id="cabinet-onboarding-connect-target"
    />
  )
}

// ─── Быстрые действия ───────────────────────────────────────────────────

type QuickAction = {
  key: string
  icon: LucideIcon
  label: string
  to?: string
  href?: string
  onClick?: () => void
  disabled?: boolean
}

/*
 * Места 1 и 2 зависят от состояния подписки и не настраиваются; места 3 и 4
 * задаёт админ (CABINET_HOME_QUICK_ACTIONS). Когда подписка истекла, первых
 * двух нет — остаются настроенные и «Написать в поддержку».
 */
function useQuickActions(data: HomeData): QuickAction[] {
  const { t } = useTranslation()
  const { openConnect } = useDeviceManager()
  const { data: bootstrap } = useAuthBootstrap()
  const toast = useToast()
  const { state: linkCopyState, copy: copyLink } = useCopyToClipboard()
  const { state, isTrial, hasLink, sub } = data
  const configured = resolveHomeQuickActions(bootstrap)

  // «Ссылка подписки» копирует сразу, без модалки: её ставят на главную для
  // тех, кто вставляет подписку в приложение сам. Кнопка видна и без
  // подписки — иначе сетка кнопок прыгала бы от состояния аккаунта.
  const copySubscriptionLink = async () => {
    const link = sub?.subscription_link?.trim()
    if (!link) {
      toast.info(t('home.qaSubLinkNone'))
      return
    }
    if (await copyLink(link)) toast.success(t('home.qaSubLinkCopied'))
    else toast.error(t('common.copyFailed'))
  }

  const toAction = (id: HomeQuickActionId): QuickAction => {
    const meta = HOME_QUICK_ACTIONS[id]
    if (meta.copiesSubscriptionLink) {
      const done = linkCopyState === 'done'
      return {
        key: id,
        icon: done ? Check : meta.icon,
        label: done ? t('subscriptionPage.copied') : t(meta.labelKey),
        onClick: () => void copySubscriptionLink(),
      }
    }
    return { key: id, icon: meta.icon, label: t(meta.labelKey), to: meta.to, href: meta.href?.(bootstrap) }
  }
  const tail = configured.map(toAction)
  const addExtra: QuickAction = { key: 'extra', icon: AddDeviceIcon, label: t('home.qaAddExtra'), onClick: openConnect, disabled: !hasLink }
  if (state === 'expired' || state === 'traffic') {
    const fallbacks: HomeQuickActionId[] = ['support', ...DEFAULT_HOME_QUICK_ACTIONS]
    const third = fallbacks.find((id) => !configured.includes(id))
    return third ? [...tail, toAction(third)] : tail
  }
  if (state === 'crit') {
    return [addExtra, { key: 'connect', icon: MonitorSmartphone, label: t('home.qaConnect'), to: '/connections' }, ...tail]
  }
  // Календарь с плюсом — «добавить дни к сроку». У пробного продлевать
  // нечего, там покупка, и остаётся молния.
  const renew: QuickAction = isTrial
    ? { key: 'renew', icon: Zap, label: t('home.qaBuy'), to: '/tariffs' }
    : { key: 'renew', icon: CalendarPlus, label: t('home.qaRenew'), to: '/tariffs' }
  return [addExtra, renew, ...tail]
}

function ActionTarget({ action, className, children }: { action: QuickAction; className: string; children: ReactNode }) {
  if (action.href) {
    return (
      <a href={action.href} target="_blank" rel="noopener noreferrer" className={className}>
        {children}
      </a>
    )
  }
  if (action.to) {
    return (
      <Link to={action.to} className={className}>
        {children}
      </Link>
    )
  }
  return (
    <button type="button" onClick={action.onClick} disabled={action.disabled} className={cn(className, 'disabled:opacity-50')}>
      {children}
    </button>
  )
}

export function QuickActionsMobile({ data }: { data: HomeData }) {
  const actions = useQuickActions(data)
  return (
    <div className={cn('-mx-3 grid px-1 pt-1', actions.length === 3 ? 'grid-cols-3' : 'grid-cols-4')}>
      {actions.map((a) => (
        <ActionTarget key={a.key} action={a} className="flex min-w-0 flex-col items-center gap-2 text-foreground/85">
          {/* data-pending-ring: пока грузится страница, по рамке бежит дуга (index.css) */}
          <span data-pending-ring className="inline-flex size-14 items-center justify-center rounded-full border border-border bg-secondary text-foreground">
            <a.icon size={24} strokeWidth={1.9} />
          </span>
          <span className="px-0.5 text-center text-[12.5px] leading-[1.25]">{a.label}</span>
        </ActionTarget>
      ))}
    </div>
  )
}

export function QuickActionsDesktop({ data }: { data: HomeData }) {
  const actions = useQuickActions(data)
  return (
    <div className={cn('grid gap-3', actions.length === 3 ? 'grid-cols-3' : 'grid-cols-4')}>
      {actions.map((a) => (
        <ActionTarget
          key={a.key}
          action={a}
          className="cabinet-elevated-card flex min-w-0 items-center gap-3 rounded-[18px] px-3.5 py-3 text-left text-[15px] font-semibold text-foreground"
        >
          <span data-pending-ring className="inline-flex size-10 shrink-0 items-center justify-center rounded-full bg-secondary text-primary">
            <a.icon size={20} />
          </span>
          <span className="min-w-0 leading-tight">{a.label}</span>
        </ActionTarget>
      ))}
    </div>
  )
}

// ─── Широкие карточки (телефон) ─────────────────────────────────────────

const WIDE_CARD = 'cabinet-elevated-card flex min-h-[84px] w-full items-center gap-3 rounded-[22px] px-[18px] py-4 text-left'

export function DevicesWideCard({ data, onOpen }: { data: HomeData; onOpen: () => void }) {
  const { t } = useTranslation()
  const list = data.devices?.devices ?? []
  const shown = list.length > 3 ? list.slice(0, 2) : list
  return (
    <button type="button" onClick={onOpen} className={WIDE_CARD}>
      <div className="min-w-0 flex-1">
        <p className="text-[17px] font-bold">{t('home.myDevices')}</p>
        <p className="mt-0.5 text-sm text-muted-foreground">{t('home.manageDevices')}</p>
      </div>
      {list.length > 0 ? (
        <div className={cn('flex shrink-0 pl-2.5', data.stopped && 'opacity-40')} aria-hidden>
          {shown.map((d) => (
            <span
              key={d.hwid}
              className="-ml-2.5 inline-flex size-10 items-center justify-center rounded-xl border-2 border-card bg-secondary text-primary"
            >
              <DevicePlatformIcon platform={d.platform ?? d.device_model} className="size-5" />
            </span>
          ))}
          {list.length > 3 && (
            <span className="-ml-2.5 inline-flex size-10 items-center justify-center rounded-xl border-2 border-card bg-secondary text-sm font-bold text-primary">
              +{list.length - 2}
            </span>
          )}
        </div>
      ) : (
        <ChevronRight size={18} className="shrink-0 text-muted-foreground" />
      )}
    </button>
  )
}

/** Сколько дней в графике шторки: две недели читаются на телефоне без сжатия подписей. */
const CHART_DAYS = 14

/**
 * Последние CHART_DAYS дней расчётного периода. Если период короче (тариф
 * куплен вчера), слева дописываем пустые дни: столбики остаются обычной
 * ширины, а не растягиваются на всю карточку.
 */
function chartWindow(usage: TrafficUsageResponse | undefined) {
  if (!usage?.enabled || usage.categories.length === 0) return { points: [] as number[], cats: [] as string[], pad: 0 }
  const points = usage.points.slice(-CHART_DAYS)
  const cats = usage.categories.slice(-CHART_DAYS)
  const real = points.length
  const first = new Date(`${cats[0]}T00:00:00Z`)
  if (Number.isFinite(first.getTime())) {
    for (let i = 1; points.length < CHART_DAYS; i++) {
      points.unshift(0)
      cats.unshift(new Date(first.getTime() - i * 86_400_000).toISOString().slice(0, 10))
    }
  }
  // pad — сколько дней слева дописано до начала периода: про них данных нет
  return { points, cats, pad: points.length - real }
}

/**
 * График расхода по дням. Под каждым столбиком — число месяца; выбранный
 * день (нажатием, на ПК — наведением) подписан над столбиком: «370 МБ · 27 окт».
 * Сразу выбран последний день с расходом — чаще всего это «сегодня» или
 * «вчера», то, что и хотят увидеть. Дни до начала периода серые и не
 * нажимаются: это не «ноль», а «не считалось».
 */
function UsageChart({ points, cats, pad, lang }: { points: number[]; cats: string[]; pad: number; lang: string }) {
  const { t } = useTranslation()
  const lastWithData = points.reduce((acc, p, i) => (p > 0 ? i : acc), points.length - 1)
  const [sel, setSel] = useState(lastWithData)
  // Данные пришли позже первого рендера (или обновились) — выбор переводим на свежий день.
  useEffect(() => setSel(lastWithData), [lastWithData])

  const max = Math.max(...points, 1)
  const n = points.length
  // Столбики занимают до 76% высоты: сверху остаётся место под подпись.
  const h = (p: number) => Math.max(3, (p / max) * 76)
  const align = sel <= 1 ? '-translate-x-[18%]' : sel >= n - 2 ? '-translate-x-[82%]' : '-translate-x-1/2'

  return (
    <div className="mt-1">
      <div className="relative h-[190px] border-b border-border/70">
        {sel >= pad && (
          <div
            className={cn(
              'pointer-events-none absolute z-10 whitespace-nowrap rounded-lg bg-secondary px-2 py-1 text-xs font-bold text-foreground shadow-md transition-[left,bottom] duration-150',
              align,
            )}
            style={{ left: `${((sel + 0.5) / n) * 100}%`, bottom: `calc(${h(points[sel])}% + 8px)` }}
          >
            {formatBytes(points[sel], t)}
            <span className="font-medium text-muted-foreground"> · {shortDate(cats[sel], lang)}</span>
          </div>
        )}
        <div className="absolute inset-0 grid items-end gap-1.5" style={{ gridTemplateColumns: `repeat(${n}, minmax(0, 1fr))` }}>
          {points.map((p, i) =>
            i < pad ? (
              <span key={i} className="block w-full self-end rounded-[5px] bg-muted-foreground/15" style={{ height: '3%' }} />
            ) : (
              <button
                key={i}
                type="button"
                onClick={() => setSel(i)}
                onMouseEnter={() => setSel(i)}
                aria-pressed={i === sel}
                aria-label={`${shortDate(cats[i], lang)}: ${formatBytes(p, t)}`}
                className="flex h-full w-full items-end"
              >
                <span
                  className={cn(
                    'block w-full rounded-[5px] transition-colors',
                    i === sel ? 'bg-primary' : 'bg-primary/40 hover:bg-primary/60',
                  )}
                  style={{ height: `${h(p)}%` }}
                />
              </button>
            ),
          )}
        </div>
      </div>
      <div className="mt-1.5 grid gap-1.5" style={{ gridTemplateColumns: `repeat(${n}, minmax(0, 1fr))` }} aria-hidden>
        {cats.map((c, i) => (
          <span
            key={c}
            className={cn(
              'text-center text-[11px] tabular-nums',
              i === sel ? 'font-bold text-foreground' : i < pad ? 'text-muted-foreground/50' : 'text-muted-foreground',
            )}
          >
            {Number(c.slice(8, 10)) || ''}
          </span>
        ))}
      </div>
    </div>
  )
}

export function TrafficBar({ pct, tone, className }: { pct: number; tone: ToneLevel; className?: string }) {
  return (
    <div className={cn('h-2 overflow-hidden rounded-full bg-muted', className)}>
      <div
        className={cn(
          'h-full rounded-full',
          tone === 'warn' ? 'bg-amber-500' : tone === 'danger' ? 'bg-destructive' : 'bg-primary',
        )}
        style={{ width: `${Math.max(0, Math.min(100, pct))}%` }}
      />
    </div>
  )
}

// ─── Подробности трафика (шторка / модалка) ─────────────────────────────

/** Объём в числе и единице отдельно: в плитках единица мельче числа. */
function bytesParts(bytes: number, t: TFunction): [string, string] {
  const gb = bytes / 1024 ** 3
  if (gb >= 1) return [formatGbValue(gb), t('dashboard.gigabytes')]
  return [String(Math.round(bytes / 1024 ** 2)), t('home.mb')]
}

function formatBytes(bytes: number, t: TFunction): string {
  return bytesParts(bytes, t).join(' ')
}

function shortDate(iso: string, lang: string): string {
  // Дату без времени читаем как местный полдень: «2026-10-30» как полночь UTC
  // западнее Гринвича превращается во вчера.
  const d = new Date(/^\d{4}-\d{2}-\d{2}$/.test(iso) ? `${iso}T12:00:00` : iso)
  if (!Number.isFinite(d.getTime())) return iso
  return d.toLocaleDateString(lang === 'ru' ? 'ru-RU' : 'en-US', { day: 'numeric', month: 'short' }).replace('.', '')
}

/** Подзаголовок шторки: с какого дня считается расход. */
export function trafficSheetSubtitle(data: HomeData, lang: string, t: TFunction): string | undefined {
  const start = data.usage?.enabled ? data.usage.period_start : undefined
  return start ? t('dashboard.usageChartSince', { date: formatDayMonth(start, lang) }) : undefined
}

function UsageTile({ label, value, unit }: { label: string; value: string; unit: string }) {
  return (
    <div className="rounded-2xl border border-border/70 bg-card/60 px-3.5 py-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 text-lg font-extrabold leading-tight">
        {value} <span className="text-xs font-semibold text-muted-foreground">{unit}</span>
      </p>
    </div>
  )
}

/**
 * Шторка «Расход трафика» (SPEC §9, вариант A): сверху сколько потрачено и
 * когда обнулится, ниже график за две недели прямо на фоне шторки и две
 * плитки — в среднем и пик. Без карточек внутри шторки.
 */
export function TrafficDetails({ data, lang }: { data: HomeData; lang: string }) {
  const { t } = useTranslation()
  const { sub, usage, trafficLimit, trafficUsed, trafficTone } = data
  const { points, cats, pad } = chartWindow(usage)
  const hasPoints = points.some((p) => p > 0)
  const totalBytes = usage?.enabled ? usage.total_bytes : trafficUsed * 1024 ** 3
  const peakIdx = hasPoints ? points.indexOf(Math.max(...points)) : -1
  const allPoints = usage?.enabled ? usage.points : []
  const avg = allPoints.length ? allPoints.reduce((a, b) => a + b, 0) / allPoints.length : 0
  const resetsOn =
    sub?.traffic_reset_strategy && sub.traffic_reset_strategy !== 'NO_RESET' && sub.next_traffic_reset_at
      ? t('home.resetsOn', { date: formatDayMonth(sub.next_traffic_reset_at, lang) })
      : ''

  const [avgValue, avgUnit] = bytesParts(avg, t)
  const [peakValue, peakUnit] = peakIdx >= 0 ? bytesParts(points[peakIdx], t) : ['', '']

  return (
    <>
      {trafficLimit !== null ? (
        <div>
          <p className="font-heading text-[34px] font-extrabold leading-[1.1]">
            {t('home.trafficUsedOfLimit', { used: formatGbValue(trafficUsed), limit: formatGbValue(trafficLimit) })}
          </p>
          <TrafficBar pct={(trafficUsed / trafficLimit) * 100} tone={trafficTone} className="mt-3" />
          <p className="mt-2 text-[13px] text-muted-foreground">
            {trafficUsed >= trafficLimit
              ? t('home.trafficEnded')
              : t('home.trafficLeft', { left: formatGbValue(trafficLimit - trafficUsed) })}
            {resetsOn ? ` · ${resetsOn}` : ''}
          </p>
        </div>
      ) : (
        <div>
          <p className="font-heading text-[34px] font-extrabold leading-[1.1]">{formatBytes(totalBytes, t)}</p>
          <p className="mt-2 flex items-center gap-1.5 text-[13px] text-muted-foreground">
            <InfinityIcon size={18} className="text-primary" aria-hidden />
            {t('home.unlimitedTariff')}
          </p>
        </div>
      )}

      {points.length > 0 && <UsageChart points={points} cats={cats} pad={pad} lang={lang} />}

      {hasPoints && (
        <div className="grid grid-cols-2 gap-2.5">
          <UsageTile label={t('home.avgPerDay')} value={avgValue} unit={avgUnit} />
          <UsageTile
            label={t('home.maxDay')}
            value={peakValue}
            unit={`${peakUnit}${cats[peakIdx] ? ` · ${shortDate(cats[peakIdx], lang)}` : ''}`}
          />
        </div>
      )}
    </>
  )
}
