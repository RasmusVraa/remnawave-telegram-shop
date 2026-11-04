import { useInfiniteQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Users, Receipt } from 'lucide-react'

import { RevealItem } from '@/components/PageReveal'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { api } from '@/lib/api'

import { formatMoney, formatPercent, formatDayShort, formatDayMonth } from './format'
import { PARTNER_MOBILE_ROW, PARTNER_MOBILE_BADGE } from './layout'

/**
 * Размер страницы.
 *
 * Листаем через offset, а не наращиваем limit: сервер обрезает limit сотней
 * (paginationParams(r, 25, 100)), и «показать ещё» после сотой строки просто
 * перезапрашивало бы ту же страницу, а кнопка не исчезала бы никогда.
 */
const PAGE_SIZE = 25

/** Приведённые клиенты. Контакты маскирует бэкенд. */
export function PartnerCustomersTab() {
  const { t } = useTranslation()

  const { data, isLoading, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ['partner-customers'],
    queryFn: ({ pageParam }) => api.partnerCustomers({ limit: PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, pages) => {
      const loaded = pages.reduce((n, p) => n + p.items.length, 0)
      return loaded < lastPage.total ? loaded : undefined
    },
    staleTime: 30_000,
  })

  const items = data?.pages.flatMap((p) => p.items) ?? []
  const total = data?.pages[0]?.total ?? 0

  return (
    <RevealItem>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2 pb-2">
          <CardTitle className="flex items-center gap-2 text-base font-medium">
            <Users size={18} className="text-primary" />
            {t('partnerPage.customers.title')}
          </CardTitle>
          {data ? <Badge variant="secondary">{total}</Badge> : null}
        </CardHeader>
        <CardContent>
          {isLoading ? <ListSkeleton /> : null}

          {data && items.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">{t('partnerPage.customers.empty')}</p>
          ) : null}

          {items.length > 0 ? (
            <ul className="divide-y divide-border rounded-lg border border-border">
              {items.map((row, i) => (
                <li key={`${row.label}-${i}`} className={PARTNER_MOBILE_ROW}>
                  <div className="min-w-0 max-w-full">
                    <p className="truncate font-mono text-xs">{row.label}</p>
                    <p className="text-xs text-muted-foreground">
                      {[row.link_name, formatDayShort(row.attached_at)].filter(Boolean).join(' · ')}
                    </p>
                  </div>
                  <div className="w-full shrink-0 sm:w-auto sm:text-right">
                    <Badge
                      className={PARTNER_MOBILE_BADGE}
                      variant={row.has_paid ? (row.active ? 'success' : 'destructive') : 'secondary'}
                    >
                      {row.has_paid
                        ? row.active
                          ? t('partnerPage.customers.paying')
                          : t('partnerPage.customers.expired')
                        : t('partnerPage.customers.notPaid')}
                    </Badge>
                    <p className="mt-0.5 text-xs tabular-nums text-muted-foreground">
                      {row.earned > 0 ? t('partnerPage.customers.earnedFrom', { amount: formatMoney(row.earned) }) : '—'}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          ) : null}

          {hasNextPage ? (
            <Button
              variant="secondary"
              size="sm"
              className="mt-3 w-full"
              loading={isFetchingNextPage}
              onClick={() => void fetchNextPage()}
            >
              {t('partnerPage.showMore')}
            </Button>
          ) : null}
        </CardContent>
      </Card>
    </RevealItem>
  )
}

/** Лента начислений: сумма платежа, процент и результат в каждой строке. */
export function PartnerEarningsTab() {
  const { t } = useTranslation()

  const { data, isLoading, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ['partner-earnings'],
    queryFn: ({ pageParam }) => api.partnerEarnings({ limit: PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, pages) => {
      const loaded = pages.reduce((n, p) => n + p.items.length, 0)
      return loaded < lastPage.total ? loaded : undefined
    },
    staleTime: 30_000,
  })

  const items = data?.pages.flatMap((p) => p.items) ?? []
  const total = data?.pages[0]?.total ?? 0

  return (
    <RevealItem>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2 pb-2">
          <CardTitle className="flex items-center gap-2 text-base font-medium">
            <Receipt size={18} className="text-primary" />
            {t('partnerPage.earnings.title')}
          </CardTitle>
          {data ? <Badge variant="secondary">{total}</Badge> : null}
        </CardHeader>
        <CardContent>
          {isLoading ? <ListSkeleton /> : null}

          {data && items.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">{t('partnerPage.earnings.empty')}</p>
          ) : null}

          {items.length > 0 ? (
            <ul className="divide-y divide-border rounded-lg border border-border">
              {items.map((row) => (
                <li key={row.id} className={cn(PARTNER_MOBILE_ROW, row.status === 'cancelled' && 'opacity-60')}>
                  <div className="min-w-0 max-w-full">
                    {/* Ручная корректировка бывает отрицательной — знак берётся
                        из суммы, а не приписывается всегда как «+». */}
                    <p className="font-semibold tabular-nums">
                      {row.amount < 0 ? '−' : '+'}
                      {formatMoney(Math.abs(row.amount))}
                    </p>
                    <p className="truncate text-xs text-muted-foreground">
                      {[
                        row.customer_label,
                        row.kind === 'first'
                          ? t('partnerPage.earnings.kindFirst', { percent: formatPercent(row.percent) })
                          : row.kind === 'renewal'
                            ? t('partnerPage.earnings.kindRenewal', { percent: formatPercent(row.percent) })
                            : t('partnerPage.earnings.kindAdjustment'),
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {row.kind === 'adjustment'
                        ? row.note || formatDayShort(row.created_at)
                        : t('partnerPage.earnings.fromAmount', {
                            amount: formatMoney(row.base_amount_rub),
                            date: formatDayShort(row.created_at),
                          })}
                    </p>
                  </div>
                  <EarningStatus status={row.status} holdUntil={row.hold_until} />
                </li>
              ))}
            </ul>
          ) : null}

          {hasNextPage ? (
            <Button
              variant="secondary"
              size="sm"
              className="mt-3 w-full"
              loading={isFetchingNextPage}
              onClick={() => void fetchNextPage()}
            >
              {t('partnerPage.showMore')}
            </Button>
          ) : null}

          <p className="mt-3 text-xs text-muted-foreground">{t('partnerPage.earnings.hint')}</p>
        </CardContent>
      </Card>
    </RevealItem>
  )
}

function EarningStatus({ status, holdUntil }: { status: string; holdUntil?: string }) {
  const { t } = useTranslation()
  const className = cn('shrink-0', PARTNER_MOBILE_BADGE)
  if (status === 'hold') {
    return (
      <Badge variant="default" className={className}>
        {holdUntil
          ? t('partnerPage.earnings.holdUntil', { date: formatDayMonth(holdUntil) })
          : t('partnerPage.earnings.hold')}
      </Badge>
    )
  }
  if (status === 'cancelled') {
    return (
      <Badge variant="destructive" className={className}>
        {t('partnerPage.earnings.cancelled')}
      </Badge>
    )
  }
  return (
    <Badge variant="success" className={className}>
      {t('partnerPage.earnings.available')}
    </Badge>
  )
}

function ListSkeleton() {
  return (
    <div className="divide-y divide-border rounded-lg border border-border" aria-hidden>
      {[0, 1, 2, 3].map((i) => (
        <div key={i} className="flex items-center justify-between gap-2 px-3 py-2.5">
          <Skeleton className="h-3.5 w-32" />
          <Skeleton className="h-5 w-16 rounded-full" />
        </div>
      ))}
    </div>
  )
}
