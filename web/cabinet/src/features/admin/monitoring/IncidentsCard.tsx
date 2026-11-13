import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { ChevronRight, MessageCircleWarning, ServerOff, TriangleAlert, Unplug } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { cn } from '@/lib/utils'
import type { AdminAttentionItemDTO } from '@/lib/types/admin'
import { sinceText } from './eventView'

const ICONS: Record<string, LucideIcon> = {
  node_down: ServerOff,
  panel_down: Unplug,
  support_waiting: MessageCircleWarning,
  support_undelivered: MessageCircleWarning,
}

/** Больше проблем в плашке не показываем. */
const MAX_SHOWN = 3

const LINKS: Record<string, string> = {
  node_down: '/admin/nodes',
  panel_down: '/admin/nodes',
}

/** Подпись инцидента — та же в «Мониторинге» и в «Обзоре». */
export function incidentLabel(item: AdminAttentionItemDTO, t: TFunction): string {
  return t(`admin.monitoring.incident.${item.key}`, {
    count: item.count,
    name: String(item.params?.name ?? ''),
    defaultValue: item.key,
  })
}

/** Куда ведёт инцидент из «Обзора»: нода — на «Ноды», остальное — в «Мониторинг». */
export function incidentLink(item: AdminAttentionItemDTO): string {
  return LINKS[item.key] ?? '/admin/monitoring'
}

/** «Требует внимания»: только то, что горит сейчас, с таймером «сколько уже». */
export function IncidentsCard({ items, compact }: { items: AdminAttentionItemDTO[]; compact?: boolean }) {
  const { t } = useTranslation()

  // Всё спокойно — плашки нет вовсе; больше трёх проблем не показываем.
  if (items.length === 0) return null
  const shown = items.slice(0, MAX_SHOWN)
  const crit = shown.some((i) => i.severity === 'crit')
  return (
    <section
      className={cn(
        'flex w-full flex-col rounded-[--radius] border px-4 pb-1.5 pt-3.5 sm:flex-row sm:items-center sm:gap-4 sm:px-4 sm:py-2.5',
        crit
          ? 'border-rose-500/45 bg-gradient-to-br from-rose-500/[0.13] via-rose-500/[0.05] to-transparent'
          : 'border-amber-500/45 bg-gradient-to-br from-amber-500/[0.13] via-amber-500/[0.05] to-transparent',
      )}
    >
      <h2 className="mb-1 flex shrink-0 items-center gap-2 text-[15px] font-semibold sm:mb-0 sm:whitespace-nowrap">
        <TriangleAlert className={cn('size-4', crit ? 'text-rose-500' : 'text-amber-500')} aria-hidden />
        {t('admin.monitoring.attention')}
      </h2>
      <ul className="flex flex-col sm:flex-row sm:flex-wrap sm:gap-2.5">
        {shown.map((item, i) => {
          const Icon = ICONS[item.key] ?? TriangleAlert
          const to = LINKS[item.key]
          const tone = item.severity === 'crit' ? 'text-rose-500' : 'text-amber-500'
          const row = (
            <>
              <span
                className={cn(
                  'flex shrink-0 items-center justify-center rounded-[10px] sm:size-[30px]',
                  compact ? 'size-8' : 'size-9',
                  item.severity === 'crit' ? 'bg-rose-500/[0.12]' : 'bg-amber-500/[0.12]',
                  tone,
                )}
              >
                <Icon className="size-4" data-pending-icon aria-hidden />
              </span>
              <span className="min-w-0 flex-1 truncate text-sm sm:flex-none sm:whitespace-nowrap">{incidentLabel(item, t)}</span>
              {item.since && (
                <span className={cn('shrink-0 text-[13px] font-medium tabular-nums', tone)}>{sinceText(item.since, t)}</span>
              )}
              {to && <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
            </>
          )
          return (
            <li key={`${item.key}-${i}`} className={cn(
                'flex sm:rounded-[14px] sm:border sm:border-border/60 sm:bg-background/50',
                i > 0 && 'border-t border-border/50 sm:border-t-border/60',
              )}
            >
              {to ? (
                <Link to={to} className="flex min-h-12 flex-1 items-center gap-3 rounded-lg py-2 hover:opacity-90 sm:min-h-0 sm:gap-2.5 sm:py-1.5 sm:pl-2 sm:pr-3.5">
                  {row}
                </Link>
              ) : (
                <div className="flex min-h-12 flex-1 items-center gap-3 py-2 sm:min-h-0 sm:gap-2.5 sm:py-1.5 sm:pl-2 sm:pr-3.5">{row}</div>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
