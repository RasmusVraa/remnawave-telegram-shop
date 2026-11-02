import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  AlertTriangle,
  Calendar,
  Check,
  ChevronRight,
  Copy,
  Download,
  FileText,
  Gauge,
  Gem,
  Infinity as InfinityIcon,
  Laptop,
  Minus,
  MonitorSmartphone,
  Newspaper,
  Plus,
  Smartphone,
  Sparkles,
  Star,
  Ticket,
  Trash2,
  Users,
  Wifi,
  Zap,
  type LucideIcon,
} from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { applyTheme } from '@/hooks/useTheme'
import type { AuthBootstrapResponse } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useCardSpotlight } from '@/features/landing/components/LandingMotion'
import './redesign-preview.css'

/**
 * Dev-превью редизайна: /cabinet/dev/redesign.
 *
 * Маршрут регистрируется только при import.meta.env.DEV (см. App.tsx). Данные —
 * моки, сеть и сессия не нужны: страница поднимается одним `npm run dev`.
 */

// ── Моки ────────────────────────────────────────────────────────────────────

const MOCK = {
  tariff: 'Премиум',
  expireAt: '5 октября 2026 г.',
  /** Начало расчётного периода — от него считается расход в карточке графика. */
  periodStart: '5 сентября',
  trafficLimitGb: 500,
  devicesLimit: 10,
  extraDevicesActive: 2,
  extraDevicePriceRub: 49,
  loyaltyLevel: 3,
  loyaltyDiscountPct: 7,
  loyaltyProgressPct: 62,
  loyaltyNextLevel: 4,
  subscriptionLink: 'https://sub.me0ws.store/s/9f3ac1e0b74d2a58',
}

const MOCK_DEVICES = [
  { hwid: 'a1b2c3d4e5f6', title: 'iPhone 15 Pro', subtitle: 'iOS · 18.2', icon: Smartphone },
  { hwid: 'f6e5d4c3b2a1', title: 'MacBook Air M2', subtitle: 'macOS · 15.1', icon: Laptop },
  { hwid: '0099aabbccdd', title: 'Pixel 8', subtitle: 'Android · 15', icon: Smartphone },
  { hwid: '1122334455aa', title: 'iPad Air', subtitle: 'iPadOS · 18.1', icon: Smartphone },
  { hwid: 'bbccddeeff00', title: 'ThinkPad X1', subtitle: 'Windows · 11', icon: Laptop },
  { hwid: 'aa00bb11cc22', title: 'Redmi Note 13', subtitle: 'Android · 14', icon: Smartphone },
  { hwid: 'dd33ee44ff55', title: 'Mac mini', subtitle: 'macOS · 15.2', icon: Laptop },
  { hwid: '6677889900ab', title: 'Galaxy S24', subtitle: 'Android · 15', icon: Smartphone },
  { hwid: 'cdef01234567', title: 'Steam Deck', subtitle: 'Linux · 3.6', icon: Laptop },
  { hwid: '89abcdef0123', title: 'Apple TV 4K', subtitle: 'tvOS · 18', icon: Laptop },
]

const QUICK_LINKS: { icon: LucideIcon; label: string; hint: string; accent: string }[] = [
  { icon: Zap, label: 'Тарифы', hint: 'Продлить или сменить', accent: 'cab-accent-violet' },
  { icon: Users, label: 'Рефералы', hint: 'Дни за приглашения', accent: '' },
  { icon: Ticket, label: 'Промокоды', hint: 'Активировать код', accent: 'cab-accent-amber' },
  { icon: FileText, label: 'Информация', hint: 'Ответы и документы', accent: '' },
  { icon: Newspaper, label: 'Новости', hint: 'Канал проекта', accent: '' },
  { icon: Star, label: 'Отзывы', hint: 'Поделиться мнением', accent: 'cab-accent-amber' },
]

// ── Пороги и уровни тревоги ─────────────────────────────────────────────────

/**
 * Единая шкала на все показатели: спокойно → предупреждение → опасность.
 *
 * Пороги дней те же, что гоняют призыв продлить (<= 7 и <= 3). Пороги трафика
 * взяты из прода — там уже есть trafficBarFillClass с 70% и 90%.
 */
const WARN_DAYS = 7
const DANGER_DAYS = 3
const WARN_TRAFFIC_PCT = 70
const DANGER_TRAFFIC_PCT = 90

type Level = 'calm' | 'warn' | 'danger'

/** Класс акцента: кольца, полосы и иконки внутри наследуют --cab-accent. */
const LEVEL_ACCENT: Record<Level, string> = {
  calm: '',
  warn: 'cab-accent-amber',
  danger: 'cab-accent-rose',
}

function daysLevel(days: number | null): Level {
  if (days === null || days <= 0) return 'danger'
  if (days <= DANGER_DAYS) return 'danger'
  if (days <= WARN_DAYS) return 'warn'
  return 'calm'
}

function trafficLevel(pct: number | null): Level {
  if (pct === null) return 'calm'
  if (pct >= DANGER_TRAFFIC_PCT) return 'danger'
  if (pct >= WARN_TRAFFIC_PCT) return 'warn'
  return 'calm'
}

/**
 * Устройства не эскалируют до красного.
 *
 * Занятые слоты — не поломка, а обычная граница: человек либо освободит слот,
 * либо докупит. Красный тут кричал бы о проблеме, которой нет. Янтарный
 * появляется только когда слотов не осталось совсем — там подсказка уместна.
 */
function devicesLevel(used: number, limit: number): Level {
  return used >= limit ? 'warn' : 'calm'
}

// ── Контекст превью ─────────────────────────────────────────────────────────

type LinksLayout = 'one-card' | 'separate' | 'grouped' | 'two-tier' | 'footer'
type UnlimitedBar = 'none' | 'dashed' | 'fade' | 'full'

interface PreviewOptions {
  days: number | null
  /** true — спокойное состояние срока зелёное (как в проде), false — нейтральное. */
  calmGreen: boolean
  devices: number
  /** null — безлимитный тариф. */
  trafficPct: number | null
  links: LinksLayout
  unlimitedBar: UnlimitedBar
}

const PreviewOptionsContext = createContext<PreviewOptions>({
  days: 40,
  calmGreen: false,
  devices: 3,
  trafficPct: 30,
  links: 'two-tier',
  unlimitedBar: 'none',
})

const usePreviewOptions = () => useContext(PreviewOptionsContext)

const usedGb = (pct: number | null) =>
  pct === null ? 148 : Math.round((pct / 100) * MOCK.trafficLimitGb)

// ── Примитивы ───────────────────────────────────────────────────────────────

function Ring({
  value,
  size = 92,
  children,
  className,
}: {
  value: number
  size?: number
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn('cab-ring shrink-0', className)}
      style={{ width: size, ['--value' as string]: value }}
    >
      <div className="text-center leading-none">{children}</div>
    </div>
  )
}

/** Статичное кольцо с ∞: держит ритм сетки, но не изображает прогресс. */
function InfinityRing({ size = 92 }: { size?: number }) {
  return (
    <div
      className="grid shrink-0 place-items-center rounded-full border-[7px] border-muted"
      style={{ width: size, height: size }}
    >
      <InfinityIcon size={size * 0.32} className="text-[hsl(var(--cab-accent))]" />
    </div>
  )
}

function Pill({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        'cab-pill px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.14em]',
        className,
      )}
    >
      {children}
    </span>
  )
}

/** Статус подписки идёт по той же шкале, что и всё остальное. */
function StatusBadge() {
  const { days } = usePreviewOptions()
  const level = daysLevel(days)
  const expired = days === null || days <= 0

  if (expired) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full border border-destructive/45 bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">
        <span className="size-1.5 rounded-full bg-destructive" />
        Истекла
      </span>
    )
  }

  if (level !== 'calm') {
    const danger = level === 'danger'
    return (
      <span
        className={cn(
          'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium',
          danger
            ? 'border-destructive/45 bg-destructive/10 text-destructive'
            : 'border-amber-400/50 bg-amber-500/10 text-amber-700 dark:text-amber-300',
        )}
      >
        <span className={cn('size-1.5 rounded-full', danger ? 'bg-destructive' : 'bg-amber-500')} />
        Заканчивается
      </span>
    )
  }

  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-400/30 bg-emerald-500/15 px-2.5 py-1 text-xs font-medium text-emerald-600 dark:text-emerald-300">
      <span className="size-1.5 rounded-full bg-emerald-500" />
      Активна
    </span>
  )
}

// ── Срок действия ───────────────────────────────────────────────────────────

function ExpireRow() {
  const { days, calmGreen } = usePreviewOptions()
  const level = daysLevel(days)
  const expired = days === null || days <= 0

  const skin = {
    calm: calmGreen
      ? {
          box: 'border-emerald-300/50 bg-emerald-500/10 dark:border-emerald-300/25',
          icon: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300',
          label: 'text-emerald-700/85 dark:text-emerald-300/90',
          note: 'text-emerald-600 dark:text-emerald-300',
        }
      : {
          box: 'border-border/60 bg-background/40',
          icon: 'bg-muted text-muted-foreground',
          label: 'text-muted-foreground',
          note: 'text-muted-foreground',
        },
    warn: {
      box: 'border-amber-400/60 bg-amber-500/10 dark:border-amber-300/30',
      icon: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
      label: 'text-amber-800/90 dark:text-amber-300/90',
      note: 'text-amber-700 dark:text-amber-300',
    },
    danger: {
      box: 'border-destructive/55 bg-destructive/10',
      icon: 'bg-destructive/15 text-destructive',
      label: 'text-destructive/80',
      note: 'text-destructive',
    },
  }[level]

  return (
    <div className={cn('flex items-center gap-3 rounded-xl border px-3.5 py-3', skin.box)}>
      <span className={cn('inline-flex size-9 shrink-0 items-center justify-center rounded-lg', skin.icon)}>
        {level === 'calm' ? <Calendar size={15} /> : <AlertTriangle size={15} />}
      </span>
      <div className="min-w-0 flex-1">
        <p className={cn('text-[11px] uppercase tracking-[0.14em]', skin.label)}>
          {expired ? 'Подписка истекла' : 'Действует до'}
        </p>
        <p className="text-[0.95rem] font-medium">{expired ? '—' : MOCK.expireAt}</p>
        <p className={cn('text-xs', skin.note)}>
          {expired ? 'Продлите, чтобы восстановить доступ' : `Осталось ${days} дн.`}
        </p>
      </div>
    </div>
  )
}

// ── Кнопки ──────────────────────────────────────────────────────────────────

/**
 * Продление. Эскалация в три ступени по тем же порогам, что и тон срока:
 * до 7 дней кнопки нет, 3–7 — вторичная, <= 3 и после истечения — главная,
 * с бликом, привлекающим внимание.
 */
function RenewCta() {
  const { days } = usePreviewOptions()
  const level = daysLevel(days)
  if (level === 'calm') return null

  if (level === 'warn') {
    return (
      <button
        type="button"
        className="cab-btn-warn inline-flex h-11 w-full items-center justify-center gap-2 rounded-xl px-4 text-sm font-semibold"
      >
        <Zap size={15} />
        Продлить подписку
      </button>
    )
  }

  const expired = days === null || days <= 0

  return (
    <span className="cab-attn-sheen">
      <button
        type="button"
        className="cab-cta flex h-12 w-full items-center justify-center gap-2 rounded-xl text-sm font-semibold"
      >
        <Zap size={16} />
        {expired ? 'Возобновить подписку' : 'Продлить сейчас'}
      </button>
    </span>
  )
}

/**
 * Подключение устройства.
 *
 * Блик — только пока не подключено ни одного устройства: подсказка нужна тому,
 * кто ещё не нашёл кнопку, а у остальных вечное движение превращается в шум.
 * При истёкшей подписке кнопка неактивна: подключать нечего.
 */
function ConnectDeviceButton() {
  const { days, devices } = usePreviewOptions()
  const level = daysLevel(days)
  const expired = days === null || days <= 0
  const demoted = level === 'danger'

  if (expired) {
    return (
      <button
        type="button"
        disabled
        className="inline-flex h-11 w-full cursor-not-allowed items-center justify-center gap-2 rounded-xl border border-border bg-muted/40 px-4 text-sm font-medium text-muted-foreground opacity-70"
      >
        <MonitorSmartphone size={16} />
        Подключить устройство
      </button>
    )
  }

  const button = (
    <button
      type="button"
      className={cn(
        'flex h-11 w-full items-center justify-center gap-2 rounded-xl text-sm font-semibold',
        demoted
          ? 'cab-row border border-border bg-background/50 font-medium'
          : 'cab-cta',
      )}
    >
      <MonitorSmartphone size={16} />
      Подключить устройство
      {devices > 0 && (
        <span
          className={cn(
            'ml-1 rounded-md px-1.5 py-0.5 text-[11px] tabular-nums',
            demoted ? 'bg-muted' : 'bg-white/20',
          )}
        >
          {devices}/{MOCK.devicesLimit}
        </span>
      )}
    </button>
  )

  if (devices > 0 || demoted) return button
  return <span className="cab-attn-sheen">{button}</span>
}

/** Пара кнопок: при срочности продление идёт первым. */
function ActionButtons() {
  return (
    <div className="space-y-2">
      <RenewCta />
      <ConnectDeviceButton />
    </div>
  )
}

// ── Трафик ──────────────────────────────────────────────────────────────────

/**
 * Безлимит: полосы прогресса нет.
 *
 * Шкала отвечает на вопрос «сколько из X осталось». Без X любое её состояние
 * врёт — залитая читается как «лимит исчерпан», пустая как «ничего не потрачено».
 * Сейчас в проде рисуется именно залитая на 100%. Вместо неё — кольцо с ∞
 * и абсолютный расход.
 */
function TrafficRingSlot({ size = 92 }: { size?: number }) {
  const { trafficPct } = usePreviewOptions()

  if (trafficPct === null) {
    return (
      <div className="flex flex-col items-center gap-2">
        <InfinityRing size={size} />
        {/* Без «из ∞»: знак уже внутри кольца, повторять его в подписи незачем. */}
        <p className="text-xs text-muted-foreground">{usedGb(null)} ГБ</p>
      </div>
    )
  }

  return (
    <div className={cn('flex flex-col items-center gap-2', LEVEL_ACCENT[trafficLevel(trafficPct)])}>
      <Ring value={trafficPct} size={size}>
        <span className="font-heading text-base font-bold">{trafficPct}%</span>
      </Ring>
      <p className="text-xs text-muted-foreground">
        {usedGb(trafficPct)} / {MOCK.trafficLimitGb} ГБ
      </p>
    </div>
  )
}

/**
 * Чем заменить шкалу при безлимите.
 *
 * Полная заливка читается как «лимит исчерпан», пустой трек — как «ничего не
 * потрачено»; оба варианта сообщают неправду. Поэтому по умолчанию шкалы нет
 * совсем: строка со значением держит ритм с соседними показателями, а её
 * отсутствие и есть честный признак того, что мерить нечего.
 */
function UnlimitedTrack() {
  const { unlimitedBar } = usePreviewOptions()

  if (unlimitedBar === 'none') return null

  if (unlimitedBar === 'dashed') {
    return <div className="cab-bar cab-bar--dashed mt-1.5 h-2" />
  }

  if (unlimitedBar === 'full') {
    return (
      <div className="cab-bar mt-1.5 h-2">
        <span style={{ width: '100%' }} />
      </div>
    )
  }

  return (
    <div className="cab-bar cab-bar--unbounded mt-1.5 h-2">
      <span />
    </div>
  )
}

/** Полосный вид трафика для карточек с горизонтальной раскладкой. */
function TrafficBar() {
  const { trafficPct } = usePreviewOptions()

  if (trafficPct === null) {
    return (
      <div>
        <div className="flex items-baseline justify-between gap-2 text-xs">
          <span className="text-muted-foreground">Трафик</span>
          <span className="flex items-center gap-1 font-semibold tabular-nums">
            {usedGb(null)} ГБ / <InfinityIcon size={14} />
          </span>
        </div>
        <UnlimitedTrack />
      </div>
    )
  }

  return (
    <div className={LEVEL_ACCENT[trafficLevel(trafficPct)]}>
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="text-muted-foreground">Трафик</span>
        <span className="font-semibold tabular-nums">
          {usedGb(trafficPct)} / {MOCK.trafficLimitGb} ГБ
        </span>
      </div>
      <div className="cab-bar mt-1.5 h-2">
        <span style={{ width: `${trafficPct}%` }} />
      </div>
    </div>
  )
}

function DevicesBar() {
  const { devices } = usePreviewOptions()
  const pct = (devices / MOCK.devicesLimit) * 100

  return (
    <div className={LEVEL_ACCENT[devicesLevel(devices, MOCK.devicesLimit)]}>
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="text-muted-foreground">Устройства</span>
        <span className="font-semibold tabular-nums">
          {devices} / {MOCK.devicesLimit}
        </span>
      </div>
      <div className="cab-bar mt-1.5 h-2">
        <span style={{ width: `${pct}%` }} />
      </div>
      {devices >= MOCK.devicesLimit && (
        <p className="mt-1 text-[11px] text-amber-700 dark:text-amber-300">
          Все слоты заняты — освободите один или докупите
        </p>
      )}
    </div>
  )
}

// ── Блоки страницы подписки ─────────────────────────────────────────────────

function DeviceRow({
  device,
  compact = false,
}: {
  device: (typeof MOCK_DEVICES)[number]
  compact?: boolean
}) {
  const Icon = device.icon
  return (
    <li
      className={cn(
        'cab-row flex items-center justify-between gap-3 rounded-xl px-3',
        compact ? 'py-2' : 'py-2.5',
      )}
    >
      <div className="flex min-w-0 items-center gap-3">
        <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[hsl(var(--cab-accent)/0.12)] text-[hsl(var(--cab-accent))]">
          <Icon size={16} />
        </span>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{device.title}</p>
          <p className="truncate text-xs text-muted-foreground">{device.subtitle}</p>
        </div>
      </div>
      <button
        type="button"
        className="shrink-0 rounded-lg p-2 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
        aria-label="Удалить устройство"
      >
        <Trash2 size={15} />
      </button>
    </li>
  )
}

function DevicesCard({ compact = false }: { compact?: boolean }) {
  const { devices } = usePreviewOptions()
  const list = MOCK_DEVICES.slice(0, devices)

  return (
    <div className="cab-card p-4">
      <div className="mb-2 flex items-center justify-between gap-2 px-1">
        <p className="flex items-center gap-2 text-sm font-semibold">
          <Smartphone size={15} className="text-[hsl(var(--cab-accent))]" />
          Мои устройства
        </p>
        <span
          className={cn(
            'text-xs tabular-nums',
            devices >= MOCK.devicesLimit
              ? 'font-medium text-amber-700 dark:text-amber-300'
              : 'text-muted-foreground',
          )}
        >
          {devices} / {MOCK.devicesLimit}
        </span>
      </div>
      {list.length === 0 ? (
        <div className="px-1 py-6 text-center">
          <p className="text-sm font-medium">Пока ни одного устройства</p>
          <p className="mt-1 text-xs text-muted-foreground">
            Нажмите «Подключить устройство» — покажем, как настроить
          </p>
        </div>
      ) : (
        <ul className="space-y-1">
          {list.map((device) => (
            <DeviceRow key={device.hwid} device={device} compact={compact} />
          ))}
        </ul>
      )}
    </div>
  )
}

function LoyaltyBlock() {
  return (
    <div className="cab-card cab-card--interactive cab-accent-violet cursor-pointer p-4">
      <div className="flex items-center gap-3">
        <span className="inline-flex size-10 shrink-0 items-center justify-center rounded-xl bg-[hsl(var(--cab-accent)/0.14)] text-[hsl(var(--cab-accent))]">
          <Gem size={18} />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold">Уровень {MOCK.loyaltyLevel}</p>
          <p className="text-xs text-muted-foreground">
            Скидка {MOCK.loyaltyDiscountPct}% на продление
          </p>
        </div>
        <ChevronRight className="cab-row-chevron size-5 shrink-0 text-muted-foreground" />
      </div>
      <div className="mt-3">
        <div className="cab-bar h-1.5">
          <span style={{ width: `${MOCK.loyaltyProgressPct}%` }} />
        </div>
        <p className="mt-1.5 text-[11px] text-muted-foreground">
          До уровня {MOCK.loyaltyNextLevel} — ещё 4 600 XP
        </p>
      </div>
    </div>
  )
}

function ExtraDevicesBlock() {
  return (
    <div className="cab-card cab-accent-emerald p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-semibold">Дополнительные устройства</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {MOCK.extraDevicePriceRub} ₽ в месяц за слот
          </p>
        </div>
        <span className="shrink-0 rounded-lg bg-[hsl(var(--cab-accent)/0.12)] px-2.5 py-1 text-xs font-semibold text-[hsl(var(--cab-accent))]">
          +{MOCK.extraDevicesActive} активно
        </span>
      </div>
      <div className="mt-3 flex items-center gap-3">
        <div className="flex items-center gap-1 rounded-xl border border-border bg-background/60 p-1">
          <button
            type="button"
            className="rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            aria-label="Убрать слот"
          >
            <Minus size={15} />
          </button>
          <span className="min-w-8 text-center text-sm font-semibold tabular-nums">
            {MOCK.extraDevicesActive}
          </span>
          <button
            type="button"
            className="rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            aria-label="Добавить слот"
          >
            <Plus size={15} />
          </button>
        </div>
        <button type="button" className="cab-cta h-9 flex-1 rounded-xl px-4 text-sm font-semibold">
          Применить
        </button>
      </div>
    </div>
  )
}

function SubscriptionLinkBlock() {
  return (
    <div className="cab-card p-4">
      <p className="flex items-center gap-2 text-sm font-semibold">
        <Wifi size={15} className="text-[hsl(var(--cab-accent))]" />
        Ссылка подписки
      </p>
      <div className="mt-3 flex items-center gap-2">
        <div className="min-w-0 flex-1 truncate rounded-lg bg-muted/70 px-3 py-2 font-mono text-xs text-muted-foreground">
          {MOCK.subscriptionLink}
        </div>
        <button
          type="button"
          className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-border px-3 py-2 text-xs font-medium transition-colors hover:bg-muted"
        >
          <Copy size={13} />
          Копировать
        </button>
      </div>
    </div>
  )
}

/**
 * Главная карточка подписки — общая для обоих вариантов страницы подписки
 * и для главной. Именно её вы отметили как удачную в варианте «две колонки».
 */
function SubscriptionHeroCard({ title }: { title?: string }) {
  const spotlight = useCardSpotlight()

  return (
    <div className="cab-card p-5" onMouseMove={spotlight}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <Pill>{title ?? 'Ваша подписка'}</Pill>
          <p className="mt-2 font-heading text-2xl font-bold">{MOCK.tariff}</p>
        </div>
        <StatusBadge />
      </div>

      {/* Кольца здесь нет и при безлимите: показатели идут полосами, кольцо
          съедало высоту и выбивалось из ряда. */}
      <div className="mt-4 space-y-3">
        <TrafficBar />
        <DevicesBar />
      </div>

      <div className="mt-4">
        <ExpireRow />
      </div>

      <div className="mt-4">
        <ActionButtons />
      </div>
    </div>
  )
}

/** Одна строка-раздел. Внешний вид общий, меняется только обёртка вокруг группы. */
function QuickLinkRow({
  item,
  boxed,
}: {
  item: (typeof QUICK_LINKS)[number]
  /** true — строка сама себе карточка, false — лежит внутри общей. */
  boxed: boolean
}) {
  const Icon = item.icon
  return (
    <button
      type="button"
      className={cn(
        'cab-row flex w-full items-center gap-3 rounded-xl px-3 py-3 text-left',
        item.accent,
        boxed && 'cab-card cab-card--interactive',
      )}
    >
      <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[hsl(var(--cab-accent)/0.12)] text-[hsl(var(--cab-accent))]">
        <Icon size={16} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">{item.label}</span>
        <span className="block text-xs text-muted-foreground">{item.hint}</span>
      </span>
      <ChevronRight className="cab-row-chevron size-4 shrink-0 text-muted-foreground" />
    </button>
  )
}

/**
 * Разделы кабинета.
 *
 * Шесть пунктов не равны по важности: «Тарифы» — это деньги, «Отзывы» —
 * внешняя ссылка в Telegram. Общая обёртка уравнивала их, отсюда и ощущение
 * свалки. Варианты ниже отличаются тем, насколько сильно они это неравенство
 * проявляют.
 */
function QuickLinks() {
  const { links } = usePreviewOptions()

  if (links === 'one-card') {
    return (
      <div className="cab-card p-2">
        <div className="cab-grid-2 gap-1">
          {QUICK_LINKS.map((item) => (
            <QuickLinkRow key={item.label} item={item} boxed={false} />
          ))}
        </div>
      </div>
    )
  }

  if (links === 'separate') {
    return (
      <div className="cab-grid-2">
        {QUICK_LINKS.map((item) => (
          <QuickLinkRow key={item.label} item={item} boxed />
        ))}
      </div>
    )
  }

  if (links === 'grouped') {
    const groups: { title: string; items: typeof QUICK_LINKS }[] = [
      { title: 'Подписка и оплата', items: QUICK_LINKS.filter((l) => ['Тарифы', 'Промокоды'].includes(l.label)) },
      { title: 'Бонусы', items: QUICK_LINKS.filter((l) => l.label === 'Рефералы') },
      {
        title: 'Помощь и сообщество',
        items: QUICK_LINKS.filter((l) => ['Информация', 'Новости', 'Отзывы'].includes(l.label)),
      },
    ]

    return (
      <div className="space-y-4">
        {groups.map((group) => (
          <div key={group.title}>
            <p className="mb-1.5 px-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
              {group.title}
            </p>
            <div className="cab-card p-2">
              <div className="cab-grid-2 gap-1">
                {group.items.map((item) => (
                  <QuickLinkRow key={item.label} item={item} boxed={false} />
                ))}
              </div>
            </div>
          </div>
        ))}
      </div>
    )
  }

  // two-tier / footer: то, ради чего сюда заходят — карточками; редкое — ниже.
  const primary = QUICK_LINKS.filter((l) => ['Тарифы', 'Промокоды', 'Рефералы'].includes(l.label))

  return (
    <div className="space-y-3">
      <div className="cab-grid-2">
        {primary.map((item) => (
          <QuickLinkRow key={item.label} item={item} boxed />
        ))}
      </div>
      {/* В режиме footer второй уровень уезжает в подвал страницы (CabinetFooter). */}
      {links === 'two-tier' && <SecondaryLinksRow />}
    </div>
  )
}

const SECONDARY_LINKS = QUICK_LINKS.filter((l) =>
  ['Информация', 'Новости', 'Отзывы'].includes(l.label),
)

function SecondaryLinksRow() {
  return (
    <div className="flex flex-wrap items-center justify-center gap-x-5 gap-y-2 px-2 py-1">
      {SECONDARY_LINKS.map(({ icon: Icon, label }) => (
        <button
          key={label}
          type="button"
          className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          <Icon size={13} />
          {label}
        </button>
      ))}
    </div>
  )
}

/**
 * Подвал кабинета — задел под идею вынести редкие ссылки со всех страниц сразу.
 *
 * Плюс перед вторым уровнем на главной: ссылки перестают дублироваться на
 * каждой странице и не занимают первый экран. Минус: подвал длинной страницы
 * видят редко — но для справки и внешних каналов это приемлемо, туда идут
 * осознанно, а не наткнувшись глазом.
 */
function CabinetFooter() {
  return (
    <footer className="mt-8 border-t border-border/60 pt-5">
      <div className="flex flex-wrap items-center justify-center gap-x-6 gap-y-3">
        {SECONDARY_LINKS.map(({ icon: Icon, label }) => (
          <button
            key={label}
            type="button"
            className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
          >
            <Icon size={13} />
            {label}
          </button>
        ))}
        <button
          type="button"
          className="text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          Условия и оферта
        </button>
      </div>
      <p className="mt-3 text-center text-[11px] text-muted-foreground/70">Meows VPN</p>
    </footer>
  )
}

// ── Главная: вариант A «Фокус» ──────────────────────────────────────────────

function DashboardFocus() {
  const { days } = usePreviewOptions()
  const expired = days === null || days <= 0

  return (
    <div className="space-y-5">
      <div>
        <h1 className="font-heading text-3xl font-bold tracking-tight">Добро пожаловать!</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {expired ? 'Подписка истекла' : `Подписка активна ещё ${days} дн.`}
        </p>
      </div>

      <SubscriptionHeroCard />
      <QuickLinks />
    </div>
  )
}

// ── Главная: вариант B «Приборная панель» ───────────────────────────────────

function DashboardCockpit() {
  const spotlight = useCardSpotlight()
  const { days, devices, trafficPct } = usePreviewOptions()
  const expired = days === null || days <= 0
  const dLevel = daysLevel(days)
  const daysRingPct = expired ? 100 : Math.min(100, Math.round(((days ?? 0) / 90) * 100))

  return (
    <div className="space-y-5">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h1 className="font-heading text-3xl font-bold tracking-tight">{MOCK.tariff}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {expired ? 'Подписка истекла' : `до ${MOCK.expireAt}`}
          </p>
        </div>
        <StatusBadge />
      </div>

      {/* Три показателя. Трафик остаётся в ряду и при безлимите — там кольцо ∞. */}
      <div className="grid grid-cols-3 gap-3">
        <div className={cn('cab-card flex flex-col items-center gap-2 p-4', LEVEL_ACCENT[dLevel])} onMouseMove={spotlight}>
          <Ring value={daysRingPct} size={72}>
            <span className="font-heading text-base font-bold">{expired ? 0 : days}</span>
          </Ring>
          <span className="text-[11px] uppercase tracking-wide text-muted-foreground">дней</span>
        </div>

        <div
          className={cn(
            'cab-card flex flex-col items-center gap-2 p-4',
            trafficPct === null ? 'cab-accent-violet' : LEVEL_ACCENT[trafficLevel(trafficPct)],
          )}
          onMouseMove={spotlight}
        >
          {trafficPct === null ? (
            <InfinityRing size={72} />
          ) : (
            <Ring value={trafficPct} size={72}>
              <span className="font-heading text-base font-bold">{trafficPct}%</span>
            </Ring>
          )}
          <span className="text-[11px] uppercase tracking-wide text-muted-foreground">трафик</span>
        </div>

        <div
          className={cn(
            'cab-card flex flex-col items-center gap-2 p-4',
            devicesLevel(devices, MOCK.devicesLimit) === 'calm'
              ? 'cab-accent-emerald'
              : LEVEL_ACCENT.warn,
          )}
          onMouseMove={spotlight}
        >
          <Ring value={(devices / MOCK.devicesLimit) * 100} size={72}>
            <span className="font-heading text-base font-bold">{devices}</span>
          </Ring>
          <span className="text-[11px] uppercase tracking-wide text-muted-foreground">устройств</span>
        </div>
      </div>

      <ActionButtons />

      {/*
        Расход за расчётный период подписки, а не за календарный месяц: так
        цифра сходится с лимитом тарифа. Панель отдаёт готовый sparklineData
        через GET /api/bandwidth-stats/users/{userId}?start&end.

        Карточка одинакова для лимитного и безлимитного тарифа: график
        показывает динамику по дням, а не долю от лимита, поэтому знаменатель
        ему не нужен. Для безлимита это вообще единственная информация о
        трафике — полосы у него нет.
      */}
      <div className="cab-card p-4" onMouseMove={spotlight}>
        <div className="flex items-start justify-between gap-2">
          <div>
            <p className="text-sm font-semibold">Расход за период</p>
            <p className="text-xs text-muted-foreground">с {MOCK.periodStart}</p>
          </div>
          <span className="text-sm font-semibold tabular-nums">
            {usedGb(trafficPct)} ГБ
          </span>
        </div>
        <Sparkline className="mt-3" />
      </div>

      <QuickLinks />
    </div>
  )
}

/** Мини-график расхода: чистый SVG, без recharts. */
function Sparkline({ className }: { className?: string }) {
  const points = [12, 18, 15, 26, 22, 31, 28, 36, 34, 44, 41, 52]
  const max = Math.max(...points)
  const step = 100 / (points.length - 1)
  const line = points.map((p, i) => `${i * step},${40 - (p / max) * 34}`).join(' ')

  return (
    <svg viewBox="0 0 100 40" preserveAspectRatio="none" className={cn('h-16 w-full', className)}>
      <defs>
        <linearGradient id="cab-spark" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="hsl(var(--cab-accent))" stopOpacity="0.35" />
          <stop offset="100%" stopColor="hsl(var(--cab-accent))" stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon points={`0,40 ${line} 100,40`} fill="url(#cab-spark)" />
      <polyline
        points={line}
        fill="none"
        stroke="hsl(var(--cab-accent))"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  )
}

// ── Подписка: вариант A «Плотный» ───────────────────────────────────────────

function SubscriptionDense() {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="font-heading text-2xl font-bold tracking-tight">Подписка</h1>
      </div>

      <SubscriptionHeroCard title="Тариф" />
      <SubscriptionLinkBlock />
      <LoyaltyBlock />
      <ExtraDevicesBlock />
      <DevicesCard compact />
    </div>
  )
}


// ── Новый пользователь: гибрид по фазам ─────────────────────────────────────

/*
 * Один герой на этапе, а не два онбординга разом.
 *
 * Сейчас новый пользователь видит карточку триала и поверх неё тур из трёх
 * шагов, причём шаг «нажмите эту кнопку, чтобы получить инструкции по
 * подключению» подсвечивает кнопку активации. После нажатия экран сразу
 * становится обычной панелью, хотя приложение ещё не установлено.
 *
 * Здесь герой меняется по фазе: оффер → дорожка подключения → обычная главная.
 */

type OnbPhase = 'offer' | 'blocked' | 'activated' | 'done'

/** Плитка выгоды: цифра крупно, единица под ней. Заменяет строку текста. */
function StatTile({ icon: Icon, value, unit }: { icon: LucideIcon; value: ReactNode; unit: string }) {
  return (
    <div className="cab-stat-tile px-1.5 py-3">
      <span className="mx-auto mb-1.5 inline-flex size-7 items-center justify-center rounded-lg bg-[hsl(var(--cab-accent)/0.14)] text-[hsl(var(--cab-accent))]">
        <Icon size={14} />
      </span>
      <div className="font-heading text-2xl font-bold leading-none tabular-nums">{value}</div>
      <div className="mt-1 text-[10px] uppercase leading-tight tracking-wide text-muted-foreground">
        {unit}
      </div>
    </div>
  )
}

/** Уход к тарифам мимо пробного: не акцент, но и не потерянная подпись. */
function SkipToTariffs({ label }: { label: string }) {
  return (
    <div className="mt-5 border-t border-border/70 pt-4">
      <button
        type="button"
        className="cab-btn-quiet cab-row flex h-10 w-full items-center justify-center gap-1.5 rounded-xl text-xs font-medium"
      >
        {label}
        <ChevronRight className="cab-row-chevron size-4 text-muted-foreground" />
      </button>
    </div>
  )
}

/*
 * Четыре подачи одного и того же предложения.
 *
 * Содержание везде одинаковое — меняется только оформление, чтобы можно было
 * сравнить, а не спорить на словах. Оффер триала и заглушка «триал недоступен»
 * идут через один и тот же рендер: это одна карточка с разным наполнением,
 * и выглядеть они должны как родственники.
 */
type OfferStyle = 'plain' | 'tiles' | 'path' | 'hero'

type OfferStat = { icon: LucideIcon; value: ReactNode; unit: string }

type OfferContent = {
  icon: LucideIcon
  pill: string
  /** Приставка перед выделенной частью: «От» у цены, у триала не нужна. */
  prefix?: string
  /** Выделенная часть заголовка: цифра или цена. */
  lead: string
  rest: string
  subtitle: string
  stats: OfferStat[]
  /** Для варианта D: одна цифра крупно и всё остальное строкой под ней. */
  hero: { value: string; caption: string }
  cta: string
  skip?: string
  accent?: string
}

const TRIAL_OFFER: OfferContent = {
  icon: Sparkles,
  pill: 'Пробный период',
  lead: '7 дней',
  rest: 'бесплатно',
  subtitle: 'Попробуйте бесплатно — без обязательств',
  stats: [
    { icon: Calendar, value: '7', unit: 'дней' },
    { icon: Gauge, value: '3', unit: 'ГБ' },
    { icon: Smartphone, value: '1', unit: 'устройство' },
  ],
  hero: { value: '7', caption: 'дней бесплатно' },
  cta: 'Активировать бесплатно',
  skip: 'Не нужен пробный — сразу к тарифам',
}

/*
 * Заглушка «триал недоступен»: уже использован или выключен в настройках.
 *
 * Всё выводится из GET /tariffs, ничего не зашито: цена — минимальный
 * monthly_base_rub по витрине, устройства — максимальный device_limit,
 * трафик — ∞, если хоть у одного тарифа traffic_gb === null, иначе «до N ГБ»,
 * выгода — разница цены самого длинного и самого короткого периода. Витрина
 * пустая или тариф один — плитку просто не выводим.
 */
const TARIFFS_OFFER: OfferContent = {
  icon: Zap,
  pill: 'Тарифы',
  prefix: 'От',
  lead: '110 ₽',
  rest: 'в месяц',
  subtitle: 'Пробный период уже использован — дальше платные тарифы',
  stats: [
    { icon: MonitorSmartphone, value: '5', unit: 'до устройств' },
    { icon: Gauge, value: <InfinityIcon size={22} className="mx-auto" />, unit: 'трафик' },
    { icon: Calendar, value: '−31%', unit: 'на год' },
  ],
  hero: { value: '110 ₽', caption: 'в месяц' },
  cta: 'Смотреть тарифы',
  accent: 'cab-accent-violet',
}

/** Кнопка предложения: блик всегда, это единственное действие на экране. */
function OfferCta({ label }: { label: string }) {
  return (
    <span className="cab-attn-sheen mt-5 block">
      <button type="button" className="cab-cta h-12 w-full rounded-xl text-sm font-semibold">
        {label}
      </button>
    </span>
  )
}

/** A · как на проде: по центру, без рамок и градиентов. Меньше всего нового. */
function OfferPlain({ content }: { content: OfferContent }) {
  const Icon = content.icon
  return (
    <div className={cn('cab-card p-6 text-center', content.accent)}>
      <span className="mx-auto flex size-12 items-center justify-center rounded-xl border border-[hsl(var(--cab-accent)/0.3)] bg-[hsl(var(--cab-accent)/0.1)] text-[hsl(var(--cab-accent))]">
        <Icon size={20} />
      </span>

      <h1 className="mt-4 font-heading text-2xl font-bold leading-tight">
        {content.prefix ? `${content.prefix} ` : ''}
        {content.lead} {content.rest}
      </h1>
      <p className="mt-1 text-sm text-muted-foreground">{content.subtitle}</p>

      <div className="mt-5 grid grid-cols-3 gap-3">
        {content.stats.map((s) => (
          <div key={s.unit}>
            <div className="font-heading text-3xl font-bold leading-none tabular-nums">{s.value}</div>
            <div className="mt-1 text-[10px] uppercase leading-tight tracking-wide text-muted-foreground">
              {s.unit}
            </div>
          </div>
        ))}
      </div>

      <OfferCta label={content.cta} />
      {content.skip && <SkipToTariffs label={content.skip} />}
    </div>
  )
}

/** B · плитки: сияние по верху, градиент в цифре, обводка у каждой цифры. */
function OfferTiles({ content }: { content: OfferContent }) {
  const Icon = content.icon
  return (
    <div className={cn('cab-card relative overflow-hidden p-5 sm:p-6', content.accent)}>
      <span className="cab-onb-aurora" aria-hidden />

      <div className="relative">
        <div className="flex items-center gap-3">
          <span className="inline-flex size-11 items-center justify-center rounded-2xl border border-[hsl(var(--cab-accent)/0.35)] bg-[hsl(var(--cab-accent)/0.12)] text-[hsl(var(--cab-accent))]">
            <Icon size={20} />
          </span>
          <Pill>{content.pill}</Pill>
        </div>

        <h1 className="mt-4 font-heading text-3xl font-bold leading-[1.1] sm:text-4xl">
          {content.prefix ? `${content.prefix} ` : ''}
          <span className="cab-gradient-text">{content.lead}</span> {content.rest}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">{content.subtitle}</p>

        <div className="mt-5 grid grid-cols-3 gap-2">
          {content.stats.map((s) => (
            <StatTile key={s.unit} icon={s.icon} value={s.value} unit={s.unit} />
          ))}
        </div>

        <OfferCta label={content.cta} />
        {content.skip && <SkipToTariffs label={content.skip} />}
      </div>
    </div>
  )
}

/*
 * C · путь виден сразу.
 *
 * Человек с первого экрана понимает, что шагов три, но инструкцию получает
 * только на текущем. Сюрприза после активации нет: карточка не подменяется,
 * а просто переезжает на следующий шаг.
 */
function OfferPath({ content }: { content: OfferContent }) {
  return (
    <div className="cab-card p-5 sm:p-6">
      <Pill>{content.pill}</Pill>
      <h1 className="mt-3 font-heading text-2xl font-bold leading-tight sm:text-3xl">
        {content.prefix ? `${content.prefix} ` : ''}
        <span className="cab-gradient-text">{content.lead}</span> {content.rest}
      </h1>
      <p className="mt-1.5 text-sm text-muted-foreground">{content.subtitle}</p>

      <div className="mt-5 space-y-1">
        <div className="rounded-xl bg-[hsl(var(--cab-accent)/0.07)] px-3 py-3">
          <div className="flex items-center gap-3">
            <span className="inline-flex size-7 shrink-0 items-center justify-center rounded-full bg-[hsl(var(--cab-accent))] text-xs font-semibold text-white">
              1
            </span>
            <span className="text-sm font-medium">Активировать пробный период</span>
          </div>
          <p className="mt-1.5 pl-10 text-xs text-muted-foreground">
            7 дней · 3 ГБ · 1 устройство
          </p>
          <div className="pl-10">
            <OfferCta label={content.cta} />
          </div>
        </div>

        {['Установить приложение', 'Подключиться'].map((title, i) => (
          <div key={title} className="flex items-center gap-3 px-3 py-3">
            <span className="inline-flex size-7 shrink-0 items-center justify-center rounded-full border border-border text-xs font-semibold text-muted-foreground">
              {i + 2}
            </span>
            <span className="text-sm font-medium text-muted-foreground">{title}</span>
          </div>
        ))}
      </div>

      {content.skip && <SkipToTariffs label={content.skip} />}
    </div>
  )
}

/** D · одна цифра: всё остальное уходит в строку под ней. Минимум элементов. */
function OfferHero({ content }: { content: OfferContent }) {
  return (
    <div className={cn('cab-card relative overflow-hidden p-6 text-center', content.accent)}>
      <span className="cab-onb-aurora" aria-hidden />

      <div className="relative">
        <Pill>{content.pill}</Pill>

        {/* «от» отдельной строкой: внутри крупной цифры оно ломало ритм. */}
        {content.prefix && (
          <div className="mt-4 text-sm font-medium lowercase text-muted-foreground">
            {content.prefix}
          </div>
        )}
        <div
          className={cn(
            'font-heading text-6xl font-bold leading-none sm:text-7xl',
            content.prefix ? 'mt-1' : 'mt-4',
          )}
        >
          <span className="cab-gradient-text">{content.hero.value}</span>
        </div>
        <div className="mt-2 font-heading text-xl font-bold">{content.hero.caption}</div>
        <p className="mt-1.5 text-sm text-muted-foreground">{content.subtitle}</p>

        {/* Остальные цифры — строкой с иконками: без них строка читалась как подпись. */}
        <div className="mt-4 flex items-center justify-center gap-4">
          {content.stats.slice(1).map((s) => {
            const Icon = s.icon
            return (
              <span key={s.unit} className="inline-flex items-center gap-1.5 text-sm font-medium">
                <span className="inline-flex size-6 items-center justify-center rounded-md bg-[hsl(var(--cab-accent)/0.14)] text-[hsl(var(--cab-accent))]">
                  <Icon size={13} />
                </span>
                {s.value} {s.unit}
              </span>
            )
          })}
        </div>

        <OfferCta label={content.cta} />
        {content.skip && <SkipToTariffs label={content.skip} />}
      </div>
    </div>
  )
}

function OfferCard({ style, content }: { style: OfferStyle; content: OfferContent }) {
  // Путь применим только к триалу: у тарифов шагов нет, там он вырождается в B.
  if (style === 'path' && !content.skip) return <OfferTiles content={content} />
  if (style === 'plain') return <OfferPlain content={content} />
  if (style === 'path') return <OfferPath content={content} />
  if (style === 'hero') return <OfferHero content={content} />
  return <OfferTiles content={content} />
}

/*
 * Мок тура сразу после активации.
 *
 * Текст не тот, что в проде: там первый шаг здоровается, а второй обещает
 * инструкции по подключению, подсвечивая кнопку активации. Здесь тур
 * появляется уже после активации и говорит ровно то, ради чего нужен —
 * куда нажать, чтобы подключить.
 */
function TourMock() {
  return (
    <div className="pointer-events-none absolute inset-0 z-30 rounded-2xl bg-black/35">
      <div className="absolute left-1/2 top-32 w-[min(20rem,calc(100%-2rem))] -translate-x-1/2 rounded-2xl border border-border bg-card p-4 shadow-2xl">
        <div className="flex items-center gap-2">
          <span className="inline-flex size-7 items-center justify-center rounded-lg bg-[hsl(var(--cab-accent)/0.14)] text-[hsl(var(--cab-accent))]">
            <Download size={14} />
          </span>
          <p className="text-sm font-semibold">Подключите устройство</p>
        </div>
        <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
          Нажмите эту кнопку — подберём приложение под вашу систему и добавим подписку.
        </p>
        <div className="mt-3 flex items-center justify-between gap-3">
          <span className="text-xs text-muted-foreground">Пропустить</span>
          <span className="cab-cta rounded-lg px-3 py-1.5 text-xs font-semibold">Понятно</span>
        </div>
      </div>
    </div>
  )
}

/** Главная сразу после активации: 7 дней, ноль трафика, ноль устройств. */
function FreshDashboard({ children }: { children?: ReactNode }) {
  const base = usePreviewOptions()
  return (
    <PreviewOptionsContext.Provider value={{ ...base, days: 7, trafficPct: 0, devices: 0 }}>
      <div className="relative">
        <DashboardCockpit />
        {children}
      </div>
    </PreviewOptionsContext.Provider>
  )
}

/** Тихая альтернатива туру: одна строка внизу, ничего не перекрывает. */
function QuietReminder() {
  return (
    <div className="cab-card cab-row cab-accent-emerald flex items-center gap-3 px-4 py-3">
      <span className="inline-flex size-8 shrink-0 items-center justify-center rounded-lg bg-[hsl(var(--cab-accent)/0.12)] text-[hsl(var(--cab-accent))]">
        <Check size={15} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">Добавьте резервный способ входа</span>
        <span className="block text-xs text-muted-foreground">
          Чтобы не потерять аккаунт — минута в профиле
        </span>
      </span>
      <ChevronRight className="cab-row-chevron size-4 shrink-0 text-muted-foreground" />
    </div>
  )
}

/*
 * Разделов на этапе настройки нет намеренно.
 *
 * Тарифы, рефералы и промокоды уводят от единственной задачи — довести
 * человека до рабочего VPN. Кто пришёл покупать, уходит по «сразу к тарифам»
 * из оффера; остальным они понадобятся уже на обычной главной.
 */
function OnboardingPreview({
  phase,
  offerStyle,
  blockedStyle,
}: {
  phase: OnbPhase
  offerStyle: OfferStyle
  blockedStyle: OfferStyle
}) {
  if (phase === 'activated') {
    return (
      <FreshDashboard>
        <TourMock />
      </FreshDashboard>
    )
  }

  if (phase === 'done') {
    return (
      <div className="space-y-2.5">
        <DashboardCockpit />
        <QuietReminder />
      </div>
    )
  }

  return (
    <div className="space-y-2.5">
      {phase === 'offer' && <OfferCard style={offerStyle} content={TRIAL_OFFER} />}
      {phase === 'blocked' && <OfferCard style={blockedStyle} content={TARIFFS_OFFER} />}
    </div>
  )
}

// ── Оболочка превью ─────────────────────────────────────────────────────────

type Page = 'dashboard' | 'subscription' | 'onboarding'
type Variant = 'a' | 'b'
type Frame = 'mobile' | 'desktop'
type DaysKey = '40' | '7' | '3' | '1' | 'expired'
type CalmKey = 'neutral' | 'green'
type TrafficKey = '30' | '75' | '95' | 'unlim'
type DevicesKey = '0' | '3' | '10'

const DAYS_VALUE: Record<DaysKey, number | null> = { '40': 40, '7': 7, '3': 3, '1': 1, expired: 0 }
const TRAFFIC_VALUE: Record<TrafficKey, number | null> = {
  '30': 30,
  '75': 75,
  '95': 95,
  unlim: null,
}

const VARIANT_LABEL: Record<Page, Record<Variant, string>> = {
  dashboard: { a: 'A · Фокус', b: 'B · Приборная панель' },
  subscription: { a: 'A · Плотный', b: 'B · Устройства выше' },
  onboarding: { a: '', b: '' },
}

const VARIANT_NOTE: Record<Page, Record<Variant, string>> = {
  dashboard: {
    a: 'Одна крупная карточка держит внимание, разделы уходят в лёгкий список.',
    b: 'Состояние читается за секунду по трём кольцам, есть график расхода, разделы сжаты в пилюли.',
  },
  subscription: {
    a: 'Вариант выбран: подписка → ссылка → лояльность → докупка → устройства.',
    b: '',
  },
  onboarding: { a: '', b: '' },
}

const ONB_LABEL: Record<OnbPhase, string> = {
  offer: '1 · Оффер',
  blocked: '1б · Триал недоступен',
  activated: '2 · Сразу после активации',
  done: '3 · Подключено',
}

const ONB_NOTE: Record<OnbPhase, string> = {
  offer:
    'Подписки нет, триал доступен. Одна цифра, две рядом строкой, одно действие. Ничего не объясняем заранее.',
  blocked:
    'Триал уже использован или выключен. Сейчас здесь тупик с погасшей кнопкой — экран ведёт к тарифам. Всё выводится из витрины тарифов, ничего не зашито.',
  activated:
    'Дорожки нет: человек попадает на обычную главную с нулями, и путь показывает тур. Он же закрывает дыру «что теперь нажать» — без него после активации подсказки не остаётся.',
  done: 'Устройство подключено, тур пройден. Напоминание про резервный вход живёт тихой строкой внизу и никого не останавливает.',
}

export default function RedesignPreviewPage() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark')
  const [page, setPage] = useState<Page>('dashboard')
  const [variant, setVariant] = useState<Variant>('a')
  const [frame, setFrame] = useState<Frame>('mobile')
  const [daysKey, setDaysKey] = useState<DaysKey>('40')
  const [calmKey, setCalmKey] = useState<CalmKey>('neutral')
  const [trafficKey, setTrafficKey] = useState<TrafficKey>('30')
  const [devicesKey, setDevicesKey] = useState<DevicesKey>('3')
  const [links, setLinks] = useState<LinksLayout>('two-tier')
  const [unlimitedBar, setUnlimitedBar] = useState<UnlimitedBar>('none')
  const [shell, setShell] = useState<'prod' | 'bare'>('prod')
  const [onbPhase, setOnbPhase] = useState<OnbPhase>('offer')
  /* Утверждено: оффер — одна цифра, заглушка — плитки. Переключатели оставлены,
     чтобы можно было вернуться к сравнению, не пересобирая превью. */
  const [offerStyle, setOfferStyle] = useState<OfferStyle>('hero')
  const [blockedStyle, setBlockedStyle] = useState<OfferStyle>('tiles')
  const [panelOpen, setPanelOpen] = useState(true)
  const queryClient = useQueryClient()

  /*
   * Тему подсовываем в сам ответ bootstrap, а не ставим data-атрибут руками:
   * сетку и пятна рисует CabinetDecorScene по теме из bootstrap, а глобальный
   * CabinetDecorThemeSync всё равно перетёр бы атрибут фоллбэком.
   */
  useState(() => {
    queryClient.setQueryData(['auth-bootstrap'], (prev: AuthBootstrapResponse | undefined) => ({
      ...(prev ?? {}),
      decor_theme: 'nebula' as const,
    }))
  })

  useEffect(() => {
    queryClient.setQueryData(['auth-bootstrap'], (prev: AuthBootstrapResponse | undefined) => ({
      ...(prev ?? {}),
      decor_theme: shell === 'prod' ? ('nebula' as const) : ('off' as const),
    }))
  }, [queryClient, shell])

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  const pageContent =
    page === 'onboarding' ? (
      <OnboardingPreview phase={onbPhase} offerStyle={offerStyle} blockedStyle={blockedStyle} />
    ) : page === 'dashboard' ? (
      { a: <DashboardFocus />, b: <DashboardCockpit /> }[variant]
    ) : (
      // Страница подписки: вариант выбран, сравнивать больше нечего.
      <SubscriptionDense />
    )

  const content = (
    <>
      {pageContent}
      {links === 'footer' && <CabinetFooter />}
    </>
  )

  const panel = (
    <div className="fixed right-3 top-3 z-[3000] max-w-[min(calc(100vw-1.5rem),46rem)]">
      {panelOpen ? (
        <div className="rounded-2xl border border-border bg-card/95 p-3 shadow-2xl backdrop-blur-xl">
          <div className="mb-2 flex items-center justify-between gap-3">
            <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Превью редизайна
            </span>
            <button
              type="button"
              onClick={() => setPanelOpen(false)}
              className="rounded-lg border border-border px-2 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
            >
              Свернуть
            </button>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Segmented
              value={page}
              onChange={(next) => {
                setPage(next)
                setVariant('a')
              }}
              options={[
                { value: 'dashboard', label: 'Главная' },
                { value: 'subscription', label: 'Подписка' },
                { value: 'onboarding', label: 'Новый юзер' },
              ]}
            />
            {page === 'dashboard' && (
              <Segmented
                value={variant}
                onChange={setVariant}
                options={(['a', 'b'] as Variant[]).map((v) => ({
                  value: v,
                  label: VARIANT_LABEL.dashboard[v],
                }))}
              />
            )}
            {page === 'onboarding' && (
              <Segmented
                value={onbPhase}
                onChange={setOnbPhase}
                options={(['offer', 'blocked', 'setup', 'done'] as OnbPhase[]).map((p) => ({
                  value: p,
                  label: ONB_LABEL[p],
                }))}
              />
            )}
            {page === 'onboarding' && (onbPhase === 'offer' || onbPhase === 'blocked') && (
              <Segmented
                value={onbPhase === 'offer' ? offerStyle : blockedStyle}
                onChange={onbPhase === 'offer' ? setOfferStyle : setBlockedStyle}
                options={[
                  { value: 'plain', label: 'A · Как на проде' },
                  { value: 'tiles', label: 'B · Плитки' },
                  { value: 'path', label: 'C · Путь' },
                  { value: 'hero', label: 'D · Одна цифра' },
                ]}
              />
            )}
            <Segmented
              value={theme}
              onChange={setTheme}
              options={[
                { value: 'dark', label: 'Тёмная' },
                { value: 'light', label: 'Светлая' },
              ]}
            />
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">Оболочка:</span>
            <Segmented
              value={shell}
              onChange={setShell}
              options={[
                { value: 'prod', label: 'Как в проде' },
                { value: 'bare', label: 'Только блоки' },
              ]}
            />
            {shell === 'bare' && (
              <Segmented
                value={frame}
                onChange={setFrame}
                options={[
                  { value: 'mobile', label: 'Мобильный' },
                  { value: 'desktop', label: 'Десктоп' },
                ]}
              />
            )}
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">Дней:</span>
            <Segmented
              value={daysKey}
              onChange={setDaysKey}
              options={[
                { value: '40', label: '40' },
                { value: '7', label: '7' },
                { value: '3', label: '3' },
                { value: '1', label: '1' },
                { value: 'expired', label: 'Истекла' },
              ]}
            />
            <span className="text-xs text-muted-foreground">Спокойное:</span>
            <Segmented
              value={calmKey}
              onChange={setCalmKey}
              options={[
                { value: 'neutral', label: 'Нейтр.' },
                { value: 'green', label: 'Зелёное' },
              ]}
            />
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">Трафик:</span>
            <Segmented
              value={trafficKey}
              onChange={setTrafficKey}
              options={[
                { value: '30', label: '30%' },
                { value: '75', label: '75%' },
                { value: '95', label: '95%' },
                { value: 'unlim', label: 'Безлимит' },
              ]}
            />
            <span className="text-xs text-muted-foreground">Устройств:</span>
            <Segmented
              value={devicesKey}
              onChange={setDevicesKey}
              options={[
                { value: '0', label: '0' },
                { value: '3', label: '3' },
                { value: '10', label: '10 (лимит)' },
              ]}
            />
          </div>

          {page === 'dashboard' && (
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <span className="text-xs text-muted-foreground">Разделы:</span>
              <Segmented
                value={links}
                onChange={setLinks}
                options={[
                  { value: 'one-card', label: 'Одной карточкой' },
                  { value: 'separate', label: 'Отдельными' },
                  { value: 'grouped', label: 'Группами' },
                  { value: 'two-tier', label: 'Два уровня' },
                  { value: 'footer', label: 'Второй уровень в футер' },
                ]}
              />
            </div>
          )}

          {trafficKey === 'unlim' && (
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <span className="text-xs text-muted-foreground">Полоса при безлимите:</span>
              <Segmented
                value={unlimitedBar}
                onChange={setUnlimitedBar}
                options={[
                  { value: 'none', label: 'Нет' },
                  { value: 'dashed', label: 'Пунктир' },
                  { value: 'fade', label: 'Растворяется' },
                  { value: 'full', label: 'Залита' },
                ]}
              />
            </div>
          )}

          <p className="mt-2 max-w-2xl text-xs leading-relaxed text-muted-foreground">
            {page === 'onboarding' ? ONB_NOTE[onbPhase] : VARIANT_NOTE[page][variant]}
            {shell === 'prod' && ' Оболочка настоящая — для мобильного вида включите эмуляцию устройства в devtools.'}
          </p>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setPanelOpen(true)}
          className="rounded-xl border border-border bg-card/95 px-3 py-2 text-xs font-medium shadow-xl backdrop-blur-xl"
        >
          Настройки превью
        </button>
      )}
    </div>
  )

  return (
    <PreviewOptionsContext.Provider
      value={{
        days: DAYS_VALUE[daysKey],
        calmGreen: calmKey === 'green',
        devices: Number(devicesKey),
        trafficPct: TRAFFIC_VALUE[trafficKey],
        links,
        unlimitedBar,
      }}
    >
      {panel}

      {shell === 'prod' ? (
        <AppLayout>
          <div className="cab-rd cab-rd-frame">{content}</div>
        </AppLayout>
      ) : (
        <div className="cab-rd min-h-dvh bg-background text-foreground">
          <div className="flex justify-center px-4 pb-10 pt-28">
            <div
              className={cn(
                'cab-rd-frame w-full',
                frame === 'mobile'
                  ? 'max-w-[390px] rounded-[2rem] border border-border bg-background p-4 shadow-2xl'
                  : 'max-w-5xl',
              )}
            >
              {content}
            </div>
          </div>
        </div>
      )}
    </PreviewOptionsContext.Provider>
  )
}

function Segmented<T extends string>({
  value,
  onChange,
  options,
}: {
  value: T
  onChange: (next: T) => void
  options: { value: T; label: string }[]
}) {
  return (
    <div className="inline-flex gap-0.5 rounded-lg border border-border/80 bg-muted/40 p-0.5">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => onChange(option.value)}
          className={cn(
            'rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors',
            value === option.value
              ? 'bg-card text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
