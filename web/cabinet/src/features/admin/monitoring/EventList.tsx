import { Fragment, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ArrowRight, ChevronDown } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'
import type { AdminEventDTO } from '@/lib/types/admin'
import { buildEventView, dayKey, dayLabel, formatTime } from './eventView'

/** Сколько раз лента подгружается сама, прежде чем попросить нажать «Показать ещё». */
const AUTO_LOADS = 4

interface EventListProps {
  items: AdminEventDTO[]
  fresh: Set<number>
  /** Общая лента: в строке ник клиента и ссылка на его карточку. В истории клиента — нет. */
  withCustomer: boolean
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
  /** Прокрутка внутри окна истории — у ленты на странице прокручивается окно браузера. */
  scrollRoot?: Element | null
  /** Меняется вместе с фильтром: счётчик автоподгрузок начинается заново. */
  resetKey: string
}

export function EventList({ items, fresh, withCustomer, hasMore, loadingMore, onLoadMore, scrollRoot, resetKey }: EventListProps) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language?.startsWith('en') ? 'en-US' : 'ru-RU'
  const sentinel = useRef<HTMLDivElement>(null)
  const autoLoads = useRef(0)
  const seenKey = useRef(resetKey)
  if (seenKey.current !== resetKey) {
    seenKey.current = resetKey
    autoLoads.current = 0
  }

  const canAuto = hasMore && autoLoads.current < AUTO_LOADS

  useEffect(() => {
    const el = sentinel.current
    if (!el || !canAuto) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting) && !loadingMore) {
          autoLoads.current += 1
          onLoadMore()
        }
      },
      { root: scrollRoot ?? null, rootMargin: '200px 0px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [canAuto, loadingMore, onLoadMore, scrollRoot, items.length])

  return (
    <div>
      {items.map((e, i) => {
        const v = buildEventView(e, t, locale)
        const newDay = i === 0 || dayKey(items[i - 1].at) !== dayKey(e.at)
        const Icon = v.icon
        // У действия админа в заголовке — админ, а клиент — в подстроке.
        const who = withCustomer && e.customer && e.type !== 'admin.action' ? e.customer : null
        const actor = e.type === 'admin.action' && e.actor ? e.actor : null
        const rowLink = withCustomer && e.customer_id ? `/admin/users/${e.customer_id}` : null
        const right = v.amount ? (
          <span
            className={cn(
              'whitespace-nowrap text-sm font-bold tabular-nums',
              v.amount.tone === 'plus' && 'text-emerald-500',
              v.amount.tone === 'minus' && 'text-rose-500',
            )}
          >
            {v.amount.text}
          </span>
        ) : v.link && withCustomer ? (
          <Link
            to={v.link.to}
            className="inline-flex items-center gap-1 whitespace-nowrap text-[13px] font-medium text-primary hover:underline"
          >
            {v.link.label}
            <ArrowRight className="size-3.5" data-pending-icon aria-hidden />
          </Link>
        ) : v.tag ? (
          <span className="whitespace-nowrap rounded-full bg-secondary px-2 py-0.5 text-[11px] font-semibold text-muted-foreground">
            {v.tag}
          </span>
        ) : null

        const body = v.quiet ? (
          <div className="flex min-w-0 items-center gap-3 py-1.5 text-[13px] text-muted-foreground sm:gap-3.5">
            <span className="hidden w-11 shrink-0 tabular-nums sm:block">{formatTime(e.at, locale)}</span>
            <span className="flex w-8 shrink-0 justify-center">
              <Icon className="size-3.5" data-pending-icon aria-hidden />
            </span>
            <span className="min-w-0 flex-1 truncate">
              {who && <span className="font-medium text-foreground/85">{who} </span>}
              {v.title}
            </span>
            <span className="shrink-0 tabular-nums sm:hidden">{formatTime(e.at, locale)}</span>
          </div>
        ) : (
          <div className="flex min-w-0 items-center gap-3 py-2.5 sm:gap-3.5">
            <span className="hidden w-11 shrink-0 text-[13px] tabular-nums text-muted-foreground sm:block">
              {formatTime(e.at, locale)}
            </span>
            <span className={cn('flex size-8 shrink-0 items-center justify-center rounded-[10px]', v.tone)}>
              <Icon className="size-4" data-pending-icon aria-hidden />
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {who && <span className="font-semibold text-primary">{who}</span>}
                {actor && <span className="font-semibold text-primary">{actor}</span>}
                {(who || actor) && <span className="text-muted-foreground"> · </span>}
                {v.title}
              </p>
              {(v.sub || (actor && e.customer)) && (
                <p className="truncate text-[12.5px] text-muted-foreground">
                  {actor && e.customer && withCustomer ? `${e.customer}${v.sub ? ' · ' : ''}` : ''}
                  {v.sub}
                </p>
              )}
            </div>
            <div className="flex shrink-0 flex-col items-end gap-0.5">
              {right}
              <span className={cn('text-xs tabular-nums text-muted-foreground sm:hidden', !right && 'text-[12.5px]')}>
                {formatTime(e.at, locale)}
              </span>
            </div>
          </div>
        )

        return (
          <Fragment key={e.id}>
            {newDay && (
              <p className="pb-1 pt-3 text-[11.5px] font-semibold uppercase tracking-wider text-muted-foreground first:pt-1">
                {dayLabel(e.at, locale, t)}
              </p>
            )}
            <div
              className={cn(
                '-mx-2.5 rounded-xl px-2.5 transition-colors duration-700',
                !newDay && !v.quiet && 'border-t border-border/40',
                fresh.has(e.id) && 'bg-primary/[0.08]',
              )}
            >
              {rowLink ? (
                <Link to={rowLink} className="block rounded-xl hover:bg-secondary/40">
                  {body}
                </Link>
              ) : (
                body
              )}
            </div>
          </Fragment>
        )
      })}

      {hasMore && (
        <div ref={canAuto ? sentinel : undefined} className="flex justify-center pb-1 pt-3">
          {canAuto || loadingMore ? (
            <span className="inline-flex items-center gap-2 text-[13px] text-muted-foreground">
              <Spinner className="size-4" />
              {t('admin.monitoring.loading')}
            </span>
          ) : (
            <button
              type="button"
              onClick={onLoadMore}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-[13.5px] font-medium text-primary hover:bg-primary/10"
            >
              <ChevronDown className="size-4" aria-hidden />
              {t('admin.monitoring.showMore')}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
