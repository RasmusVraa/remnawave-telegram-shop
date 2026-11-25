import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronLeft, ChevronRight } from 'lucide-react'

import { cn } from '@/lib/utils'
import { surface } from './Surface'

type ViewMode = 'days' | 'months' | 'years'

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

function mergeDateTime(datePart: Date, timeSource: Date): Date {
  const d = new Date(datePart)
  d.setHours(timeSource.getHours(), timeSource.getMinutes(), 0, 0)
  return d
}

function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  )
}

function startOfDay(d: Date): Date {
  const x = new Date(d)
  x.setHours(0, 0, 0, 0)
  return x
}

interface AdminDatePickerProps {
  value: Date | null
  onChange: (date: Date) => void
  minDate?: Date
  showTime?: boolean
  className?: string
  /** Текущий срок подписки (ISO) — подсвечивается в календаре, если ещё не истёк. */
  currentExpireAt?: string | null
}

export function AdminDatePicker({
  value,
  onChange,
  minDate,
  showTime = true,
  className,
  currentExpireAt,
}: AdminDatePickerProps) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language?.startsWith('en') ? 'en-GB' : 'ru-RU'
  const today = useMemo(() => startOfDay(new Date()), [])

  const currentExpireDay = useMemo(() => {
    if (!currentExpireAt) return null
    const d = new Date(currentExpireAt)
    if (Number.isNaN(d.getTime()) || d.getTime() <= Date.now()) return null
    return startOfDay(d)
  }, [currentExpireAt])

  const initialView = value ?? today
  const [viewMode, setViewMode] = useState<ViewMode>('days')
  const [viewYear, setViewYear] = useState(initialView.getFullYear())
  const [viewMonth, setViewMonth] = useState(initialView.getMonth())
  const [yearPageStart, setYearPageStart] = useState(() => initialView.getFullYear() - 5)

  useEffect(() => {
    if (!value) return
    setViewYear(value.getFullYear())
    setViewMonth(value.getMonth())
    setYearPageStart(value.getFullYear() - 5)
  }, [value?.getTime()])

  const monthName = useMemo(
    () => new Date(viewYear, viewMonth, 1).toLocaleDateString(locale, { month: 'long' }),
    [viewMonth, viewYear, locale],
  )

  const monthLabels = useMemo(() => {
    return Array.from({ length: 12 }, (_, i) =>
      new Date(viewYear, i, 1).toLocaleDateString(locale, { month: 'short' }),
    )
  }, [viewYear, locale])

  const weekdayLabels = useMemo(() => {
    const base = new Date(2024, 0, 1)
    return Array.from({ length: 7 }, (_, i) => {
      const d = new Date(base)
      d.setDate(base.getDate() + i)
      return d.toLocaleDateString(locale, { weekday: 'short' })
    })
  }, [locale])

  const cells = useMemo(() => {
    const first = new Date(viewYear, viewMonth, 1)
    const startOffset = (first.getDay() + 6) % 7
    const daysInMonth = new Date(viewYear, viewMonth + 1, 0).getDate()
    const result: { date: Date; inMonth: boolean }[] = []

    for (let i = 0; i < startOffset; i++) {
      const d = new Date(viewYear, viewMonth, 1 - (startOffset - i))
      result.push({ date: d, inMonth: false })
    }
    for (let day = 1; day <= daysInMonth; day++) {
      result.push({ date: new Date(viewYear, viewMonth, day), inMonth: true })
    }
    while (result.length % 7 !== 0) {
      const last = result[result.length - 1].date
      const d = new Date(last)
      d.setDate(d.getDate() + 1)
      result.push({ date: d, inMonth: false })
    }
    return result
  }, [viewMonth, viewYear])

  const minDay = minDate ? startOfDay(minDate) : null
  const timeSource = value ?? new Date()

  const shiftMonth = (delta: number) => {
    const d = new Date(viewYear, viewMonth + delta, 1)
    setViewYear(d.getFullYear())
    setViewMonth(d.getMonth())
  }

  const pickDay = (date: Date) => {
    onChange(mergeDateTime(date, timeSource))
  }

  const pickMonth = (month: number) => {
    setViewMonth(month)
    setViewMode('days')
  }

  const pickYear = (year: number) => {
    setViewYear(year)
    setViewMode('months')
  }

  const timeValue = `${pad2(timeSource.getHours())}:${pad2(timeSource.getMinutes())}`

  const onTimeChange = (raw: string) => {
    const [h, m] = raw.split(':').map(Number)
    if (Number.isNaN(h) || Number.isNaN(m)) return
    const base = value ?? new Date(viewYear, viewMonth, today.getDate())
    const next = new Date(base)
    next.setHours(h, m, 0, 0)
    onChange(next)
  }

  const yearGrid = useMemo(() => {
    return Array.from({ length: 12 }, (_, i) => yearPageStart + i)
  }, [yearPageStart])

  // Календарь живёт внутри модалки, а `cabinet-elevated-card` красил его ровно
  // как её панель — берём поднятый уровень шкалы.
  return (
    <div className={cn(surface('raised', 'rounded-xl p-2.5'), className)}>
      <div className="mb-2 flex items-center justify-between gap-2">
        <button
          type="button"
          onClick={() => {
            if (viewMode === 'years') setYearPageStart((y) => y - 12)
            else if (viewMode === 'months') setViewYear((y) => y - 1)
            else shiftMonth(-1)
          }}
          className={surface('raised', 'inline-flex size-8 shrink-0 items-center justify-center rounded-lg hover:bg-accent')}
          aria-label={t('admin.prev')}
        >
          <ChevronLeft className="size-4" />
        </button>

        <div className="flex min-w-0 flex-1 items-center justify-center gap-1.5">
          {viewMode === 'years' ? (
            <span className="text-sm font-semibold tabular-nums">
              {yearPageStart} – {yearPageStart + 11}
            </span>
          ) : (
            <>
              <button
                type="button"
                onClick={() => setViewMode('months')}
                className="rounded-md px-2 py-1 text-sm font-semibold capitalize hover:bg-accent"
              >
                {monthName}
              </button>
              <button
                type="button"
                onClick={() => {
                  setYearPageStart(viewYear - 5)
                  setViewMode('years')
                }}
                className="rounded-md px-2 py-1 text-sm font-semibold tabular-nums hover:bg-accent"
              >
                {viewYear}
              </button>
            </>
          )}
        </div>

        <button
          type="button"
          onClick={() => {
            if (viewMode === 'years') setYearPageStart((y) => y + 12)
            else if (viewMode === 'months') setViewYear((y) => y + 1)
            else shiftMonth(1)
          }}
          className={surface('raised', 'inline-flex size-8 shrink-0 items-center justify-center rounded-lg hover:bg-accent')}
          aria-label={t('admin.next')}
        >
          <ChevronRight className="size-4" />
        </button>
      </div>

      {viewMode === 'months' && (
        <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
          {monthLabels.map((label, i) => (
            <button
              key={label}
              type="button"
              onClick={() => pickMonth(i)}
              className={cn(
                'rounded-lg px-2 py-2.5 text-sm capitalize transition-colors hover:bg-accent',
                viewMonth === i && viewYear === (value?.getFullYear() ?? viewYear) && 'bg-primary text-primary-foreground hover:bg-primary/90',
              )}
            >
              {label}
            </button>
          ))}
        </div>
      )}

      {viewMode === 'years' && (
        <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
          {yearGrid.map((year) => (
            <button
              key={year}
              type="button"
              onClick={() => pickYear(year)}
              className={cn(
                'rounded-lg px-2 py-2.5 text-sm tabular-nums transition-colors hover:bg-accent',
                viewYear === year && 'bg-primary text-primary-foreground hover:bg-primary/90',
              )}
            >
              {year}
            </button>
          ))}
        </div>
      )}

      {viewMode === 'days' && (
        <>
          <div className="mb-0.5 grid grid-cols-7 gap-0.5">
            {weekdayLabels.map((w) => (
              <div key={w} className="py-0.5 text-center text-[10px] font-medium uppercase text-muted-foreground">
                {w}
              </div>
            ))}
          </div>

          <div className="grid grid-cols-7 gap-0.5">
            {cells.map(({ date, inMonth }) => {
              const disabled = minDay != null && date < minDay
              const selected = value != null && isSameDay(date, value)
              const isToday = isSameDay(date, today)
              const isCurrentExpire =
                currentExpireDay != null && isSameDay(date, currentExpireDay)

              return (
                <button
                  key={date.toISOString()}
                  type="button"
                  disabled={disabled}
                  onClick={() => pickDay(date)}
                  title={
                    isCurrentExpire && !selected
                      ? t('admin.users.currentExpireDay')
                      : undefined
                  }
                  className={cn(
                    'h-8 rounded-lg text-[13px] tabular-nums transition-colors',
                    !inMonth && 'text-muted-foreground/40',
                    inMonth && !selected && !disabled && 'hover:bg-accent',
                    isToday && !selected && !isCurrentExpire && 'ring-1 ring-primary/40',
                    isCurrentExpire &&
                      !selected &&
                      'bg-emerald-500/15 font-semibold text-emerald-700 ring-2 ring-emerald-500/55 dark:text-emerald-300',
                    selected && 'bg-primary text-primary-foreground hover:bg-primary/90',
                    disabled && 'pointer-events-none opacity-30',
                  )}
                >
                  {date.getDate()}
                </button>
              )
            })}
          </div>
        </>
      )}

      {showTime && viewMode === 'days' && (
        <div className="mt-2 flex items-center justify-between gap-3 border-t border-border/50 pt-2">
          <span className="text-sm text-muted-foreground">{t('admin.users.expireTime')}</span>
          <input
            type="time"
            value={timeValue}
            onChange={(e) => onTimeChange(e.target.value)}
            className="admin-input rounded-lg border border-border bg-background px-2.5 py-1 text-sm tabular-nums"
          />
        </div>
      )}
    </div>
  )
}

/** Selected local date-time → ISO UTC for expire API. */
export function dateToExpireIso(date: Date): string {
  return date.toISOString()
}

export function parseIsoToLocalDateTime(iso?: string | null): Date | null {
  if (!iso) return null
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? null : d
}

/** @deprecated use parseIsoToLocalDateTime */
export function parseIsoToLocalDate(iso?: string | null): Date | null {
  const d = parseIsoToLocalDateTime(iso)
  return d ? startOfDay(d) : null
}

export function defaultExpireDate(): Date {
  return new Date()
}
