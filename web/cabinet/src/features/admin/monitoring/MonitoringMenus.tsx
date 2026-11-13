import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ChevronDown, ListFilter, X } from 'lucide-react'

import { cn } from '@/lib/utils'
import type { AdminMonitoringPeriod } from '@/lib/types/admin'
import { CATEGORY_DOT, EVENT_CATEGORIES, type EventCategory } from './eventView'

const FILTER_KEY = 'admin.monitoring.filter'
// v2: раньше период влиял только на плитки, теперь — на ленту; сохранённый
// «час» спрятал бы почти все события.
const PERIOD_KEY = 'admin.monitoring.period.v2'

export interface FeedFilter {
  /** Пусто — все категории. */
  categories: EventCategory[]
  botActions: boolean
}

export function loadFeedFilter(): FeedFilter {
  try {
    const raw = JSON.parse(localStorage.getItem(FILTER_KEY) ?? 'null') as Partial<FeedFilter> | null
    const cats = Array.isArray(raw?.categories)
      ? EVENT_CATEGORIES.filter((c) => raw!.categories!.includes(c))
      : []
    return { categories: cats, botActions: raw?.botActions === true }
  } catch {
    return { categories: [], botActions: false }
  }
}

function saveFeedFilter(f: FeedFilter) {
  try {
    localStorage.setItem(FILTER_KEY, JSON.stringify(f))
  } catch {
    /* приватный режим — фильтр просто не запомнится */
  }
}

export const PERIODS: AdminMonitoringPeriod[] = ['hour', 'day', 'week', 'month']

export function loadPeriod(): AdminMonitoringPeriod {
  try {
    const v = localStorage.getItem(PERIOD_KEY) as AdminMonitoringPeriod | null
    return v && PERIODS.includes(v) ? v : 'day'
  } catch {
    return 'day'
  }
}

export function savePeriod(p: AdminMonitoringPeriod) {
  try {
    localStorage.setItem(PERIOD_KEY, p)
  } catch {
    /* см. saveFeedFilter */
  }
}

/** Закрыть всплывающий список по клику мимо и Esc. */
function useDismiss(open: boolean, close: () => void) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) close()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, close])
  return ref
}

function Popover({ children, align = 'left' }: { children: ReactNode; align?: 'left' | 'right' }) {
  return (
    <div
      role="menu"
      className={cn(
        'absolute top-12 z-30 w-64 max-w-[calc(100vw-2rem)] rounded-2xl border border-border bg-card p-1.5 shadow-[0_18px_50px_rgba(0,0,0,0.5)]',
        align === 'right' ? 'right-0' : 'left-0',
      )}
    >
      {children}
    </div>
  )
}

/**
 * Фильтр ленты одной кнопкой, как «Статус» у партнёров: галочки по категориям,
 * ниже — нажатия в боте, внизу «Сбросить». Ничего не отмечено — видно всё.
 */
export function EventFilterMenu({
  value,
  onChange,
  botActionsAvailable,
  compact,
  align,
  botActionsOff,
}: {
  value: FeedFilter
  onChange: (next: FeedFilter) => void
  botActionsAvailable: boolean
  /** На телефоне — только значок и счётчик. */
  compact?: boolean
  align?: 'left' | 'right'
  /** Запись нажатий выключена в настройках — показать подсказку, где включить. */
  botActionsOff?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useDismiss(open, () => setOpen(false))

  const apply = (next: FeedFilter) => {
    saveFeedFilter(next)
    onChange(next)
  }
  const toggle = (c: EventCategory) =>
    apply({
      ...value,
      categories: value.categories.includes(c)
        ? value.categories.filter((x) => x !== c)
        : EVENT_CATEGORIES.filter((x) => x === c || value.categories.includes(x)),
    })

  const bot = botActionsAvailable && value.botActions
  const active = value.categories.length > 0 || bot
  const first = value.categories[0]
  let label =
    value.categories.length === 0
      ? t('admin.monitoring.filter.all')
      : value.categories.length === 1
        ? t(`admin.monitoring.category.${first}`)
        : `${t(`admin.monitoring.category.${first}`)} +${value.categories.length - 1}`
  if (bot) label += ` ${t('admin.monitoring.filter.plusBot')}`

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t('admin.monitoring.filter.title')}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'inline-flex h-10 items-center justify-center gap-2 rounded-xl border text-sm font-medium transition-colors',
          compact ? 'w-10' : 'px-3.5',
          open || active
            ? 'border-primary/50 bg-primary/10 text-primary'
            : 'border-border bg-card hover:bg-secondary/80',
        )}
      >
        <ListFilter className="size-4 shrink-0" aria-hidden />
        {!compact && <span className="max-w-[14rem] truncate">{label}</span>}
        {value.categories.length > 1 && !compact && (
          <span className="rounded-full bg-primary/20 px-1.5 text-[11px] tabular-nums">{value.categories.length}</span>
        )}
        {!compact && <ChevronDown className="size-3.5 shrink-0" aria-hidden />}
        {compact && active && (
          <span className="absolute -right-1 -top-1 min-w-[18px] rounded-full bg-primary px-1 text-center text-[10px] font-semibold leading-[18px] text-primary-foreground">
            {value.categories.length || '•'}
          </span>
        )}
      </button>
      {open && (
        <Popover align={align ?? (compact ? 'right' : 'left')}>
          {EVENT_CATEGORIES.map((c) => {
            const checked = value.categories.includes(c)
            return (
              <button
                key={c}
                type="button"
                role="menuitemcheckbox"
                aria-checked={checked}
                onClick={() => toggle(c)}
                className="flex min-h-10 w-full items-center gap-2.5 rounded-xl px-2.5 text-left text-sm hover:bg-secondary/70"
              >
                <span
                  className={cn(
                    'grid size-4 shrink-0 place-items-center rounded border',
                    checked ? 'border-primary bg-primary text-primary-foreground' : 'border-border',
                  )}
                >
                  {checked && <Check className="size-3" strokeWidth={3} />}
                </span>
                <span className={cn('size-2 shrink-0 rounded-full', CATEGORY_DOT[c])} aria-hidden />
                <span className="truncate">{t(`admin.monitoring.category.${c}`)}</span>
              </button>
            )
          })}
          {!botActionsAvailable && botActionsOff && (
            <>
              <span className="my-1 block h-px bg-border" aria-hidden />
              <div className="px-2.5 py-2 text-[12.5px] leading-snug text-muted-foreground">
                <p className="text-sm text-foreground/80">{t('admin.monitoring.filter.botActions')}</p>
                <p className="mt-0.5">{t('admin.monitoring.filter.botActionsOff')}</p>
              </div>
            </>
          )}
          {botActionsAvailable && (
            <>
              <span className="my-1 block h-px bg-border" aria-hidden />
              <button
                type="button"
                role="menuitemcheckbox"
                aria-checked={value.botActions}
                onClick={() => apply({ ...value, botActions: !value.botActions })}
                className="flex min-h-10 w-full items-center justify-between gap-3 rounded-xl px-2.5 text-left text-sm hover:bg-secondary/70"
              >
                <span>{t('admin.monitoring.filter.botActions')}</span>
                <span
                  className={cn(
                    'relative h-5 w-9 shrink-0 rounded-full transition-colors',
                    value.botActions ? 'bg-primary' : 'bg-muted-foreground/30',
                  )}
                  aria-hidden
                >
                  <span
                    className={cn(
                      'absolute top-0.5 size-4 rounded-full bg-white transition-all',
                      value.botActions ? 'left-[18px]' : 'left-0.5',
                    )}
                  />
                </span>
              </button>
            </>
          )}
          {active && (
            <>
              <span className="my-1 block h-px bg-border" aria-hidden />
              <button
                type="button"
                onClick={() => apply({ categories: [], botActions: false })}
                className="flex w-full items-center gap-2 rounded-xl px-2.5 py-2 text-left text-[13px] text-muted-foreground hover:bg-secondary/70 hover:text-foreground"
              >
                <X className="size-3.5" aria-hidden />
                {t('admin.monitoring.filter.reset')}
              </button>
            </>
          )}
        </Popover>
      )}
    </div>
  )
}

/** Период счётчиков: час, сутки, неделя, месяц. */
export function PeriodMenu({
  value,
  onChange,
  short,
  className,
}: {
  value: AdminMonitoringPeriod
  onChange: (p: AdminMonitoringPeriod) => void
  /** «Час» вместо «За час» — в шапке телефона. */
  short?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useDismiss(open, () => setOpen(false))
  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'inline-flex items-center gap-1.5 rounded-xl border border-border bg-card font-medium transition-colors hover:bg-secondary/80',
          open && 'border-primary/50',
          className,
        )}
      >
        {t(`admin.monitoring.period.${short ? 'short.' : ''}${value}`)}
        <ChevronDown className="size-3.5 text-muted-foreground" aria-hidden />
      </button>
      {open && (
        <Popover align={short ? 'right' : 'left'}>
          {PERIODS.map((p) => (
            <button
              key={p}
              type="button"
              role="menuitemradio"
              aria-checked={p === value}
              onClick={() => {
                savePeriod(p)
                onChange(p)
                setOpen(false)
              }}
              className={cn(
                'flex min-h-10 w-full items-center justify-between rounded-xl px-2.5 text-left text-sm hover:bg-secondary/70',
                p === value && 'font-medium text-primary',
              )}
            >
              {t(`admin.monitoring.period.${p}`)}
              {p === value && <Check className="size-4" aria-hidden />}
            </button>
          ))}
        </Popover>
      )}
    </div>
  )
}
