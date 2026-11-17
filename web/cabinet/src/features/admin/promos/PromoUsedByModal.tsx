import { useEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ChevronRight, RotateCcw, Users } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import type { AdminPromoCodeDTO } from '@/lib/types/admin'
import { AdminModal } from '../components/AdminModal'
import { AdminUserAvatar } from '../components/overview/AdminUserAvatar'
import { useAdminPromoGet, useAdminPromoRedemptionsInfinite } from '../hooks/useAdminPromos'
import { formatAdminCustomerLabel } from '../utils/formatAdminCustomerLabel'
import { describeUsage, formatPromoMoney, paybackConversion } from './promoPresentation'

/** Плитка факта: подпись сверху, значение снизу; пока грузится — скелетон. */
export function PromoFact({
  label,
  children,
  loading,
  className,
  onClick,
  actionLabel,
}: {
  label: string
  children: ReactNode
  loading?: boolean
  className?: string
  /** Плитка-кнопка: правка этого поля или переход к списку. */
  onClick?: () => void
  actionLabel?: string
}) {
  const body = (
    <>
      <span className="text-[11.5px] leading-tight text-muted-foreground">{label}</span>
      {loading ? <Skeleton className="h-5 w-16 rounded-md" /> : <span className="text-[15px] font-bold tabular-nums">{children}</span>}
    </>
  )
  const cls = cn('flex min-w-0 flex-col justify-between gap-1 rounded-[14px] border border-border/60 bg-muted/40 px-3 py-2.5', className)
  if (!onClick) return <div className={cls}>{body}</div>
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={actionLabel ? `${label}: ${actionLabel}` : label}
      className={cn(cls, 'items-start text-left transition-colors hover:border-primary/40 hover:bg-primary/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 active:scale-[0.99]')}
    >
      {body}
    </button>
  )
}

/**
 * Факты отдачи — только у скидки: «Оплатили со скидкой» и «Выручка» с оплат,
 * где скидка кода применилась. У дней, триала и устройства их нет (show=false):
 * оплаты после ввода — не заслуга кода.
 */
export function usePromoPaybackFacts(promo: AdminPromoCodeDTO, enabled: boolean) {
  const { t, i18n } = useTranslation()
  const { data, isLoading } = useAdminPromoGet(enabled ? promo.id : null)
  const payback = data?.payback
  const conv = paybackConversion(payback, promo.uses_count)
  return {
    show: promo.type === 'discount',
    loading: isLoading,
    detail: data,
    paidLabel: t('admin.promos.payback.paidDiscount'),
    paid: payback ? (
      <>
        {payback.paid_customers}
        {conv != null && <span className="text-[13px] font-medium text-muted-foreground"> · {conv}%</span>}
      </>
    ) : (
      t('admin.promos.payback.none')
    ),
    revenue: (payback && formatPromoMoney(payback.revenue_rub, payback.revenue_stars, i18n.language)) || t('admin.promos.payback.none'),
  }
}

function relativeTime(iso: string, lang: string): string {
  const diffSec = (Date.parse(iso) - Date.now()) / 1000
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: 'auto', style: 'short' })
  const abs = Math.abs(diffSec)
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour')
  return rtf.format(Math.round(diffSec / 86400), 'day')
}

const pad2 = (n: number) => String(n).padStart(2, '0')

/** Ближайший предок с собственной прокруткой (тело окна); нет — окно браузера. */
function scrollParent(el: HTMLElement): HTMLElement | null {
  for (let p = el.parentElement; p; p = p.parentElement) {
    const oy = getComputedStyle(p).overflowY
    if (oy === 'auto' || oy === 'scroll') return p
  }
  return null
}

/**
 * «Кто использовал». Список подгружается сам по мере прокрутки — переключателя
 * страниц нет. Аватарка — та же, что в карточке пользователя (фото из Telegram
 * или инициалы).
 */
export function PromoUsedByModal({ promo, open, onClose }: { promo: AdminPromoCodeDTO | null; open: boolean; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const id = open && promo ? promo.id : null
  const list = useAdminPromoRedemptionsInfinite(id)
  const facts = usePromoPaybackFacts(promo ?? ({} as AdminPromoCodeDTO), open && promo != null)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const { hasNextPage, isFetchingNextPage, fetchNextPage, isFetchNextPageError } = list

  // Дошли до низа списка — следующая порция. Корень наблюдателя — прокрутка тела
  // окна, иначе запас rootMargin не работает. После ошибки сами не повторяем
  // (иначе запросы шли бы по кругу) — внизу кнопка «Повторить».
  useEffect(() => {
    const el = sentinelRef.current
    if (!el || !hasNextPage || isFetchNextPageError) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting) && !isFetchingNextPage) void fetchNextPage()
      },
      { root: scrollParent(el), rootMargin: '200px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, isFetchNextPageError, fetchNextPage])

  if (!promo) return null
  const usage = describeUsage(promo)
  const rows = list.data?.pages.flatMap((p) => p.items) ?? []
  const total = list.data?.pages[0]?.total ?? 0
  // Активации удалённых аккаунтов: строка истории уходит каскадом вместе с клиентом,
  // а счётчик промокода остаётся — честно показываем разницу.
  const deleted = list.data ? Math.max(0, promo.uses_count - total) : 0

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={`${t('admin.promos.usedByModal.title')} ${promo.code}`}
      titleSlot={
        <h3 className="truncate text-lg font-semibold leading-tight">
          {t('admin.promos.usedByModal.title')} <span className="font-mono text-primary">{promo.code}</span>
        </h3>
      }
      icon={Users}
      iconAccent="blue"
      panelClassName="sm:max-w-[560px]"
    >
      <div className="space-y-3">
        <div className={cn('grid grid-cols-2 gap-2', facts.show && 'sm:grid-cols-4')}>
          <PromoFact label={t('admin.promos.payback.activations')}>
            {usage.max != null ? `${usage.used} ${t('admin.promos.used.ofMax', { max: usage.max })}` : usage.used}
          </PromoFact>
          <PromoFact label={t('admin.promos.usedByModal.last24h')} loading={facts.loading}>
            {facts.detail?.redemptions_24h ?? facts.detail?.redemptions_today ?? '—'}
          </PromoFact>
          {facts.show && (
            <>
              <PromoFact label={facts.paidLabel} loading={facts.loading}>
                {facts.paid}
              </PromoFact>
              <PromoFact label={t('admin.promos.payback.revenue')} loading={facts.loading}>
                {facts.revenue}
              </PromoFact>
            </>
          )}
        </div>

        {list.isLoading && (
          <div className="space-y-2">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-[62px] rounded-[14px]" />
            ))}
          </div>
        )}
        {list.isError && <p className="py-6 text-center text-sm text-destructive">{t('common.error')}</p>}
        {list.data && rows.length === 0 && deleted === 0 && (
          <p className="py-6 text-center text-sm text-muted-foreground">{t('admin.promos.usedByEmpty')}</p>
        )}
        {rows.length > 0 && (
          <ul className="space-y-2">
            {rows.map((row) => {
              const d = new Date(row.used_at)
              const paid = facts.show ? formatPromoMoney(row.paid_rub, row.paid_stars, i18n.language) : null
              const label = formatAdminCustomerLabel({
                telegram_username: row.telegram_username,
                nickname: row.nickname,
                customer_id: row.customer_id,
              })
              const hasName = Boolean(row.telegram_username || row.nickname)
              return (
                <li key={`${row.customer_id}-${row.used_at}`}>
                  <Link
                    to={`/admin/users/${row.customer_id}`}
                    onClick={onClose}
                    className="flex items-center gap-3 rounded-[14px] border border-border/60 bg-muted/30 px-3 py-2.5 transition-colors hover:border-primary/40 hover:bg-primary/5"
                  >
                    <AdminUserAvatar url={row.avatar_url} name={label} tone="default" size="sm" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold">{label}</span>
                      <span className="block truncate font-mono text-xs text-muted-foreground">
                        {hasName ? `id ${row.customer_id}` : t('admin.promos.usedByModal.noNick')}
                      </span>
                    </span>
                    {paid && (
                      <span className="hidden shrink-0 rounded-full bg-emerald-500/15 px-2 py-0.5 text-[11.5px] font-semibold text-emerald-700 dark:text-emerald-400 sm:inline">
                        {paid}
                      </span>
                    )}
                    <span className="shrink-0 text-right text-[12.5px] text-muted-foreground">
                      <b className="block text-[13px] font-semibold text-foreground tabular-nums">
                        <span className="sm:hidden">{`${pad2(d.getDate())}.${pad2(d.getMonth() + 1)}`}</span>
                        <span className="hidden sm:inline">
                          {`${pad2(d.getDate())}.${pad2(d.getMonth() + 1)}.${d.getFullYear()} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`}
                        </span>
                      </b>
                      {paid ? (
                        <>
                          <span className="mt-0.5 inline-block rounded-full bg-emerald-500/15 px-2 py-0.5 text-[11.5px] font-semibold text-emerald-700 dark:text-emerald-400 sm:hidden">
                            {paid}
                          </span>
                          <span className="hidden sm:inline">{relativeTime(row.used_at, i18n.language)}</span>
                        </>
                      ) : (
                        relativeTime(row.used_at, i18n.language)
                      )}
                    </span>
                    <ChevronRight className="hidden size-4 shrink-0 text-muted-foreground sm:block" data-pending-icon />
                  </Link>
                </li>
              )
            })}
          </ul>
        )}
        {/* Низ списка: сюда смотрит наблюдатель подгрузки. */}
        <div ref={sentinelRef} aria-hidden />
        {isFetchingNextPage && (
          <div className="flex justify-center py-2 text-muted-foreground">
            <Spinner />
          </div>
        )}
        {isFetchNextPageError && !isFetchingNextPage && (
          <div className="flex justify-center">
            <button
              type="button"
              onClick={() => void fetchNextPage()}
              className="inline-flex h-9 items-center gap-2 rounded-[10px] border border-border px-3 text-sm font-semibold transition-colors hover:bg-muted"
            >
              <RotateCcw className="size-4" />
              {t('admin.promos.retry')}
            </button>
          </div>
        )}
        {deleted > 0 && !hasNextPage && (
          <p className="rounded-[12px] border border-dashed border-border/70 px-3 py-2.5 text-center text-xs text-muted-foreground">
            {t('admin.promos.usedByModal.deleted', { count: deleted })}
          </p>
        )}
      </div>
    </AdminModal>
  )
}
