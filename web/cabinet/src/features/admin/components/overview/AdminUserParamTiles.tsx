import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CalendarDays, Gauge, Infinity as InfinityIcon, Layers, Pencil, Smartphone } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import type { TFunction } from 'i18next'

import { cn, timeLeft } from '@/lib/utils'
import { formatDecimals } from '@/lib/format'
import { surface } from '../Surface'
import type { UserCardMetrics } from './userCardMetrics'
import type { UserEditModalKey } from '../user-modals/types'

/** Короткая дата для плитки: «4 окт 2026». */
function shortDate(iso: string, locale: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString(locale, { day: 'numeric', month: 'short', year: 'numeric' })
}

function Bar({ percent, tone }: { percent: number; tone: 'ok' | 'warn' | 'danger' }) {
  return (
    <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
      <div
        className={cn(
          'h-full rounded-full transition-all',
          tone === 'danger' ? 'bg-red-500' : tone === 'warn' ? 'bg-amber-500' : 'bg-primary',
        )}
        style={{ width: `${Math.max(2, Math.min(100, percent))}%` }}
      />
    </div>
  )
}

/**
 * Плитка параметра: значение и подсказка «Изменить» внутри рамки.
 *
 * Карандаш раньше жил у правого края всей карточки и не был связан со строкой,
 * которую правит. Здесь он внутри той самой плитки, по которой и нужно
 * нажать, а на ховере появляется подпись — кликабельное выглядит кликабельным.
 */
function Tile({
  icon: Icon,
  label,
  value,
  hint,
  children,
  onClick,
  editLabel,
}: {
  icon: LucideIcon
  label: string
  value: ReactNode
  hint?: ReactNode
  children?: ReactNode
  onClick?: () => void
  editLabel: string
}) {
  const body = (
    <>
      <span className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <Icon className="size-3.5 shrink-0" aria-hidden />
        {label}
      </span>
      <span className="mt-1.5 block truncate text-base font-semibold leading-snug">{value}</span>
      {children}
      {hint && <span className="mt-1.5 block truncate text-xs text-muted-foreground">{hint}</span>}
    </>
  )

  if (!onClick) {
    return <div className={surface('card', 'flex flex-col rounded-xl p-3')}>{body}</div>
  }

  return (
    <button
      type="button"
      onClick={onClick}
      title={editLabel}
      className={cn(
        // Уровень как у соседних карточек: рядом с ними «поднятая» плитка
        // читалась подсвеченной, будто выделена. Отклик остаётся на наведении.
        'group admin-overview-clickable admin-overview-clickable--surface relative rounded-xl p-3 text-left',
        // flex-col обязателен: содержимое <button> браузер центрирует по
        // вертикали, и плитка с полосой (она ниже соседей) съезжала вниз —
        // подписи в ряду переставали стоять на одной линии.
        'flex flex-col items-stretch justify-start',
        surface('card', 'hover:bg-secondary'),
      )}
    >
      <Pencil
        className="pointer-events-none absolute right-3 top-3 size-3.5 text-muted-foreground/70 transition-colors group-hover:text-primary"
        aria-hidden
      />
      {body}
    </button>
  )
}

interface Props {
  metrics: UserCardMetrics
  hasRwUser: boolean
  dateLocale: string
  onOpenModal: (key: UserEditModalKey) => void
  onOpenExpire: () => void
}

/** Подпись под сроком — та же шкала, что у пользователя на главной: дни вниз, меньше суток — часы, минуты. */
function expireHint(t: TFunction, metrics: Props['metrics']): string | undefined {
  if (metrics.days == null || !metrics.expireAt) return undefined
  if (new Date(metrics.expireAt).getTime() <= Date.now()) {
    return metrics.days < 0
      ? t('admin.users.overview.stripExpiredDays', { count: Math.abs(metrics.days) })
      : t('admin.users.overview.stripExpiredToday')
  }
  const left = timeLeft(metrics.expireAt)
  const key = { days: 'stripDaysLeft', hours: 'stripHoursLeft', minutes: 'stripMinutesLeft' }[left.unit]
  return t(`admin.users.overview.${key}`, { count: left.value })
}

export function AdminUserParamTiles({
  metrics,
  hasRwUser,
  dateLocale,
  onOpenModal,
  onOpenExpire,
}: Props) {
  const { t } = useTranslation()
  const editLabel = t('admin.users.overview.clickToEdit')

  const devicesPercent =
    metrics.devicesLimit > 0 ? (metrics.devicesUsed / metrics.devicesLimit) * 100 : 0

  const squadLabel =
    metrics.squadNames.length === 0
      ? t('admin.users.overview.statSquadEmpty')
      : metrics.squadNames[0]

  return (
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
      <Tile
        icon={CalendarDays}
        label={t('admin.users.overview.tileExpire')}
        value={metrics.expireAt ? shortDate(metrics.expireAt, dateLocale) : '—'}
        hint={expireHint(t, metrics)}
        onClick={onOpenExpire}
        editLabel={editLabel}
      />

      <Tile
        icon={Gauge}
        label={t('admin.users.overview.tileTraffic')}
        value={
          metrics.trafficLimitGb > 0 ? (
            t('admin.users.overview.tileTrafficValue', {
              used: formatDecimals(metrics.trafficUsedGb, 1),
              limit: formatDecimals(metrics.trafficLimitGb, 0),
            })
          ) : (
            /* Безлимит — знаком бесконечности: слово «без лимита» съедало
               строку и ломало ритм с соседними плитками. */
            <span className="inline-flex items-center gap-1.5">
              {t('admin.users.overview.tileTrafficUsedOnly', {
                used: formatDecimals(metrics.trafficUsedGb, 1),
              })}
              <span aria-hidden>·</span>
              <InfinityIcon className="size-4 text-muted-foreground" aria-label={t('subscriptionPage.unlimited')} />
            </span>
          )
        }
        onClick={hasRwUser ? () => onOpenModal('traffic') : undefined}
        editLabel={editLabel}
      >
        {metrics.trafficPercent != null && (
          <Bar
            percent={metrics.trafficPercent}
            tone={
              metrics.trafficPercent >= 95 ? 'danger' : metrics.trafficPercent >= 80 ? 'warn' : 'ok'
            }
          />
        )}
      </Tile>

      <Tile
        icon={Smartphone}
        label={t('admin.users.overview.tileDevices')}
        value={t('admin.users.overview.statDevicesValue', {
          used: metrics.devicesUsed,
          limit: metrics.devicesLimit,
        })}
        onClick={hasRwUser ? () => onOpenModal('devices') : undefined}
        editLabel={editLabel}
      >
        <Bar percent={devicesPercent} tone={metrics.devicesFull ? 'danger' : 'ok'} />
      </Tile>

      <Tile
        icon={Layers}
        label={t('admin.users.overview.tileSquads')}
        value={squadLabel}
        hint={
          metrics.squadNames.length > 1
            ? t('admin.users.overview.tileSquadsMore', { count: metrics.squadNames.length })
            : undefined
        }
        onClick={hasRwUser ? () => onOpenModal('squads') : undefined}
        editLabel={editLabel}
      />
    </div>
  )
}
