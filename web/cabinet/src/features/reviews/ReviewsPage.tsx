import { Link } from 'react-router-dom'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Info, Lock, PenLine } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { ReviewCard } from './ReviewCard'
import type { MyReviewResponse, ReviewBlockReason } from './reviewTypes'

const PAGE = 12

const GATE_KEY: Record<ReviewBlockReason, { key: string; to: string }> = {
  need_paid: { key: 'paid', to: '/tariffs' },
  need_sub: { key: 'sub', to: '/tariffs' },
  verify: { key: 'verify', to: '/profile' },
  no_customer: { key: 'customer', to: '/dashboard' },
}

function MyReviewBanner({ mine }: { mine: MyReviewResponse }) {
  const { t } = useTranslation()
  const status = mine.review?.status
  if (!status) return null
  return (
    <div
      className={cn(
        'flex items-start gap-3 rounded-2xl border px-4 py-3 text-sm leading-snug',
        status === 'published'
          ? 'border-emerald-500/30 bg-card bg-gradient-to-r from-emerald-500/[0.12] to-emerald-500/[0.12] text-emerald-700 dark:text-emerald-300'
          : status === 'pending'
            ? 'border-primary/30 bg-card bg-gradient-to-r from-primary/[0.10] to-primary/[0.10]'
            : 'border-amber-500/30 bg-card bg-gradient-to-r from-amber-500/[0.12] to-amber-500/[0.12] text-amber-700 dark:text-amber-300',
      )}
    >
      <Info className="mt-0.5 size-4 shrink-0" aria-hidden />
      <p>{t(`reviews.mine.${status}`)}</p>
    </div>
  )
}

export default function ReviewsPage() {
  const { t } = useTranslation()

  // Свежий статус при каждом заходе: после сохранения отзыва сюда возвращаются сразу.
  const mineQ = useQuery({ queryKey: ['my-review'], queryFn: () => api.myReview(), staleTime: 0 })
  const listQ = useInfiniteQuery({
    queryKey: ['public-reviews'],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => api.publicReviews(PAGE, pageParam),
    getNextPageParam: (last, all) => (last.items.length < PAGE ? undefined : all.length * PAGE),
  })

  const mine = mineQ.data
  const items = listQ.data?.pages.flatMap((p) => p.items) ?? []
  const enabled = listQ.data?.pages[0]?.enabled !== false && mine?.enabled !== false
  const gate = mine && !mine.can_write && !mine.review && mine.reason ? GATE_KEY[mine.reason] : null

  return (
    <AppLayout>
      <PageReveal className="mx-auto max-w-5xl space-y-6">
        <RevealItem>
          <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <p className="text-xs font-bold uppercase tracking-[0.1em] text-primary">{t('reviews.eyebrow')}</p>
              <h1 className="mt-1.5 font-heading text-2xl font-bold tracking-tight sm:text-3xl">{t('reviews.title')}</h1>
              <p className="mt-1.5 text-muted-foreground">{t('reviews.subtitle')}</p>
            </div>
            {enabled && mine && (mine.can_write || mine.review) && (
              <Button asChild className="w-full sm:w-auto">
                <Link to="/reviews/write">
                  <PenLine />
                  {mine.review ? t('reviews.edit') : t('reviews.write')}
                </Link>
              </Button>
            )}
          </div>
        </RevealItem>

        {enabled && mine && <MyReviewBanner mine={mine} />}

        {gate && enabled && (
          <RevealItem>
            <Card className="flex flex-col items-center gap-3 px-6 py-8 text-center">
              <span className="flex size-14 items-center justify-center rounded-2xl bg-amber-500/15 text-amber-500">
                <Lock className="size-6" aria-hidden />
              </span>
              <h2 className="text-lg font-bold">{t(`reviews.gate.${gate.key}.title`)}</h2>
              <p className="max-w-md text-sm leading-relaxed text-muted-foreground">{t(`reviews.gate.${gate.key}.text`)}</p>
              <Button asChild>
                <Link to={gate.to}>{t(`reviews.gate.${gate.key}.cta`)}</Link>
              </Button>
            </Card>
          </RevealItem>
        )}

        {listQ.isLoading ? (
          <div className="flex justify-center py-12">
            <Spinner />
          </div>
        ) : listQ.isError ? (
          <p className="py-10 text-center text-sm text-destructive">{t('reviews.loadError')}</p>
        ) : !enabled ? (
          <p className="py-10 text-center text-sm text-muted-foreground">{t('reviews.disabled')}</p>
        ) : items.length === 0 ? (
          <p className="py-10 text-center text-sm text-muted-foreground">{t('reviews.empty')}</p>
        ) : (
          <>
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {items.map((r) => (
                <ReviewCard key={r.id} review={r} />
              ))}
            </div>
            {listQ.hasNextPage && (
              <div className="flex justify-center">
                <Button variant="outline" loading={listQ.isFetchingNextPage} onClick={() => void listQ.fetchNextPage()}>
                  {t('reviews.loadMore')}
                </Button>
              </div>
            )}
          </>
        )}
      </PageReveal>
    </AppLayout>
  )
}
