import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Calendar, Clock, Loader2, type LucideIcon } from 'lucide-react'

import { AdminModal } from './AdminModal'
import { cn } from '@/lib/utils'
import { surface } from './Surface'
import type { AdminSectionIconAccent } from '../utils/adminSectionIconAccents'
import {
  AdminDatePicker,
  dateToExpireIso,
  defaultExpireDate,
  parseIsoToLocalDateTime,
} from './AdminDatePicker'
import { formatAdminDateTime } from '../utils/datetime'

const QUICK_DAYS = [7, 30, 90, 365]

const chipClass =
  'rounded-full border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent'

interface AdminSetExpireModalProps {
  open: boolean
  onClose: () => void
  title: string
  minDate?: Date
  isPending?: boolean
  onApply: (iso: string) => void
  error?: string | null
  onClearError?: () => void
  icon?: LucideIcon
  iconAccent?: AdminSectionIconAccent
  /** Текущий срок подписки пользователя (ISO). */
  currentExpireAt?: string | null
}

/**
 * Окно срока действия.
 *
 * Раньше здесь был только календарь: чтобы продлить на месяц, админ листал
 * месяцы и целился в число. Продление на 7/30/90/365 дней — это один клик, а
 * календарь остаётся для точной даты. Считается всё от текущего срока, а если
 * подписка уже истекла — от сегодняшнего дня, иначе «+30» выдало бы дату в
 * прошлом.
 */
export function AdminSetExpireModal({
  open,
  onClose,
  title,
  minDate,
  isPending,
  onApply,
  error,
  onClearError,
  icon = Calendar,
  iconAccent = 'amber',
  currentExpireAt,
}: AdminSetExpireModalProps) {
  const { t, i18n } = useTranslation()
  const dateLocale = i18n.language?.startsWith('en') ? 'en-GB' : 'ru-RU'
  const [date, setDate] = useState<Date | null>(null)

  const current = useMemo(() => parseIsoToLocalDateTime(currentExpireAt), [currentExpireAt])

  useEffect(() => {
    if (open) {
      // Открываемся на текущем сроке: сразу видно, откуда считается продление.
      setDate(current ?? defaultExpireDate())
    }
  }, [open, currentExpireAt])

  const handleClose = () => {
    onClearError?.()
    onClose()
  }

  const addDays = (days: number) => {
    const base = current && current.getTime() > Date.now() ? current : new Date()
    const next = new Date(base)
    next.setDate(next.getDate() + days)
    setDate(next)
  }

  const deltaDays = useMemo(() => {
    if (!date) return null
    const base = current && current.getTime() > Date.now() ? current : new Date()
    return Math.round((date.getTime() - base.getTime()) / 86_400_000)
  }, [date, current])

  return (
    <AdminModal
      open={open}
      onClose={handleClose}
      title={title}
      description={
        current
          ? t('admin.users.currentExpireHint', {
              date: formatAdminDateTime(currentExpireAt ?? null, dateLocale),
            })
          : undefined
      }
      size="md"
      icon={icon}
      iconAccent={iconAccent}
      footer={
        <div className="flex items-center justify-end gap-2">
          <button
            type="button"
            onClick={handleClose}
            disabled={isPending}
            className="rounded-btn border px-4 py-2 text-sm hover:bg-accent disabled:opacity-50"
          >
            {t('admin.cancel')}
          </button>
          <button
            type="button"
            onClick={() => {
              if (!date) return
              onApply(dateToExpireIso(date))
            }}
            disabled={isPending || !date}
            className={cn(
              'inline-flex items-center gap-2 rounded-btn bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50',
              isPending && '!opacity-100',
            )}
          >
            {isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            {t('admin.users.extendApply')}
          </button>
        </div>
      }
    >
      <div className="space-y-4">
        {error && (
          <p className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        )}

        <div>
          <p className="mb-2 text-sm font-medium">{t('admin.users.extendQuick')}</p>
          <div className="flex flex-wrap gap-2">
            {QUICK_DAYS.map((days) => (
              <button
                key={days}
                type="button"
                onClick={() => addDays(days)}
                className={cn(chipClass, deltaDays === days && 'border-primary bg-primary/10 text-primary')}
              >
                {days === 365
                  ? t('admin.users.extendQuickYear')
                  : t('admin.users.extendQuickDays', { count: days })}
              </button>
            ))}
          </div>
        </div>

        <div>
          <p className="mb-2 text-sm font-medium">{t('admin.users.extendPickDateShort')}</p>
          <AdminDatePicker
            value={date}
            onChange={setDate}
            minDate={minDate}
            currentExpireAt={currentExpireAt}
            className="w-full"
          />
        </div>

        {date && (
          <div className={surface('raised', 'flex flex-wrap items-center gap-2 rounded-xl px-3 py-2.5 text-sm')}>
            <Clock className="size-4 shrink-0 text-emerald-600 dark:text-emerald-400" aria-hidden />
            <span className="text-muted-foreground">{t('admin.users.extendResult')}</span>
            <span className="font-semibold tabular-nums">
              {formatAdminDateTime(dateToExpireIso(date), dateLocale)}
            </span>
            {deltaDays != null && deltaDays !== 0 && (
              <span
                className={cn(
                  'tabular-nums',
                  deltaDays > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive',
                )}
              >
                {deltaDays > 0
                  ? t('admin.users.extendDelta', { count: deltaDays })
                  : t('admin.users.extendDeltaMinus', { count: Math.abs(deltaDays) })}
              </span>
            )}
          </div>
        )}
      </div>
    </AdminModal>
  )
}

export { parseIsoToLocalDateTime, defaultExpireDate, dateToExpireIso }
