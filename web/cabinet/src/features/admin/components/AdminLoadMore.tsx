import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown } from 'lucide-react'

import { Spinner } from '@/components/ui/spinner'

interface Props {
  hasMore: boolean
  loading: boolean
  onLoadMore: () => void
  /** Сколько раз список подгружается сам, дойдя до низа, прежде чем попросить нажать кнопку. */
  autoLoads?: number
  /** Меняется вместе с фильтром или сортировкой: счётчик автоподгрузок начинается заново. */
  resetKey: string
}

/**
 * Низ длинного списка админки: несколько страниц подтягиваются сами, когда
 * пользователь дошёл до конца, дальше — кнопка «Показать ещё». Так же устроена
 * лента «Мониторинга»: без кнопки список ушёл бы в бесконечную прокрутку,
 * в которой не остановиться и не дойти до подвала.
 */
export function AdminLoadMore({ hasMore, loading, onLoadMore, autoLoads = 2, resetKey }: Props) {
  const { t } = useTranslation()
  const sentinel = useRef<HTMLDivElement>(null)
  const done = useRef(0)
  const seenKey = useRef(resetKey)
  if (seenKey.current !== resetKey) {
    seenKey.current = resetKey
    done.current = 0
  }

  const canAuto = hasMore && done.current < autoLoads

  useEffect(() => {
    const el = sentinel.current
    if (!el || !canAuto || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting) && !loading) {
          done.current += 1
          onLoadMore()
        }
      },
      { rootMargin: '240px 0px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [canAuto, loading, onLoadMore])

  if (!hasMore) return null

  return (
    <div ref={canAuto ? sentinel : undefined} className="flex justify-center border-t border-border/40 px-3 py-3">
      {canAuto || loading ? (
        <span className="inline-flex items-center gap-2 text-[13px] text-muted-foreground">
          <Spinner className="size-4" />
          {t('admin.users.loadingMore')}
        </span>
      ) : (
        <button
          type="button"
          onClick={onLoadMore}
          className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-[13.5px] font-medium text-primary hover:bg-primary/10"
        >
          <ChevronDown className="size-4" aria-hidden />
          {t('admin.users.showMore')}
        </button>
      )}
    </div>
  )
}
