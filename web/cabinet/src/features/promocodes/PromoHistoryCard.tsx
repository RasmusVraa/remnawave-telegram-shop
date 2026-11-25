import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { History } from 'lucide-react'

import {
  HISTORY_PAGE_SIZE,
  HistoryDateCell,
  HistoryPagination,
  historyDateInline,
} from '@/components/history-list'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { api, type PromoHistoryItem } from '@/lib/api'
import { cn } from '@/lib/utils'

type TFn = (key: string, options?: Record<string, unknown>) => string

/** Подпись типа под кодом: что за промокод. */
function typeLabel(t: TFn, type: PromoHistoryItem['type']): string {
  switch (type) {
    case 'discount':
      return t('promocodes.historyTypeDiscount')
    case 'subscription_days':
      return t('promocodes.historyTypeSubscriptionDays')
    case 'trial':
      return t('promocodes.historyTypeTrial')
    case 'extra_hwid':
      return t('promocodes.historyTypeExtraHwid')
  }
}

/** Что клиент получил: текст бейджа и его цвет — по одному на тип промокода. */
function reward(t: TFn, item: PromoHistoryItem): { text: string; tone: string } {
  switch (item.type) {
    case 'discount':
      return {
        text: `−${item.discount_percent ?? 0}%`,
        tone: 'bg-sky-500/15 text-sky-500 ring-sky-500/25',
      }
    case 'subscription_days':
      return {
        text: t('promocodes.historyRewardDays', { count: item.subscription_days ?? 0 }),
        tone: 'bg-emerald-500/15 text-emerald-500 ring-emerald-500/25',
      }
    case 'trial':
      return {
        text: t('promocodes.historyRewardTrial', { count: item.trial_days ?? 0 }),
        tone: 'bg-amber-500/15 text-amber-500 ring-amber-500/25',
      }
    case 'extra_hwid':
      return {
        text: t('promocodes.historyRewardHwid', { count: item.extra_hwid_delta ?? 0 }),
        tone: 'bg-violet-500/15 text-violet-500 ring-violet-500/25',
      }
  }
}

function RewardBadge({ item, t }: { item: PromoHistoryItem; t: TFn }) {
  const r = reward(t, item)
  return (
    <span
      className={cn(
        'inline-flex items-center whitespace-nowrap rounded-md px-2 py-0.5',
        'text-sm font-medium tabular-nums ring-1 ring-inset',
        r.tone,
      )}
    >
      {r.text}
    </span>
  )
}

function CodeText({ code }: { code: string }) {
  return <span className="font-mono font-semibold tracking-wide">{code}</span>
}

function rowKey(item: PromoHistoryItem): string {
  return `${item.used_at}|${item.code}`
}

/** История введённых клиентом промокодов (страница «Промокоды»). */
export function PromoHistoryCard() {
  const { t } = useTranslation()
  const [page, setPage] = useState(0)

  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ['promocodes-history', page],
    queryFn: () => api.promoHistory({ limit: HISTORY_PAGE_SIZE, offset: page * HISTORY_PAGE_SIZE }),
    staleTime: 30_000,
    retry: 1,
    placeholderData: (prev) => prev,
  })

  const items = data?.items ?? []
  const hasPrev = page > 0
  const hasNext = items.length === HISTORY_PAGE_SIZE
  const showPagination = hasPrev || hasNext

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <History size={18} />
          {t('promocodes.historyTitle')}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <PromoHistorySkeleton />
        ) : error ? (
          <p className="text-sm text-destructive">{t('errors.unknown')}</p>
        ) : items.length === 0 ? (
          <p className="text-sm text-muted-foreground py-6 text-center">{t('promocodes.historyEmpty')}</p>
        ) : (
          <div className={cn(isFetching && !isLoading && 'opacity-60 transition-opacity')}>
            {/* ПК: дата · промокод с типом · полученное. */}
            <table className="hidden w-full text-sm sm:table">
              <thead>
                <tr className="border-b border-border text-left text-muted-foreground">
                  <th className="w-px pb-2 pr-3 font-medium">{t('promocodes.historyColDate')}</th>
                  <th className="pb-2 pr-3 font-medium">{t('promocodes.historyColCode')}</th>
                  <th className="pb-2 text-right font-medium">{t('promocodes.historyColReward')}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={rowKey(item)} className="border-b border-border/60 last:border-0">
                    <td className="w-px py-2.5 pr-3 align-middle">
                      <HistoryDateCell iso={item.used_at} />
                    </td>
                    <td className="py-2.5 pr-3 align-middle">
                      <CodeText code={item.code} />
                      <span className="mt-0.5 block text-[11px] text-muted-foreground/70">
                        {typeLabel(t, item.type)}
                      </span>
                    </td>
                    <td className="py-2.5 text-right align-middle">
                      <RewardBadge item={item} t={t} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

            {/* Мобильные: слева код и тип, справа бейдж с датой под ним — подпись не обрезается. */}
            <ul className="sm:hidden">
              {items.map((item) => (
                <li
                  key={rowKey(item)}
                  className="flex items-center justify-between gap-3 border-b border-border/60 py-3 last:border-0"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm text-foreground">
                      <CodeText code={item.code} />
                    </p>
                    <p className="mt-0.5 truncate text-[11px] text-muted-foreground/70">
                      {typeLabel(t, item.type)}
                    </p>
                  </div>
                  <div className="shrink-0 text-right">
                    <RewardBadge item={item} t={t} />
                    <p className="mt-1 text-[11px] tabular-nums text-muted-foreground/70">
                      {historyDateInline(item.used_at)}
                    </p>
                  </div>
                </li>
              ))}
            </ul>

            {showPagination && (
              <HistoryPagination
                page={page}
                hasPrev={hasPrev}
                hasNext={hasNext}
                busy={isFetching}
                onPrev={() => setPage((p) => Math.max(0, p - 1))}
                onNext={() => setPage((p) => p + 1)}
              />
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/** Заглушка списка: три колонки, как в реальной разметке. */
function PromoHistorySkeleton() {
  return (
    <div className="space-y-3" aria-hidden>
      {[0, 1, 2, 3].map((i) => (
        <div key={i} className="flex items-center justify-between gap-3">
          <Skeleton className="h-7 w-16" />
          <Skeleton className="h-3.5 w-24" />
          <Skeleton className="h-3.5 w-20" />
        </div>
      ))}
    </div>
  )
}
