import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowRight } from 'lucide-react'

import { api } from '@/lib/api'
import { ReviewCard } from '@/features/reviews/ReviewCard'
import type { ReviewsListResponse } from '@/features/reviews/reviewTypes'
import { Reveal, useCardSpotlight } from './LandingMotion'
import { SectionHeading } from './LandingPrimitives'

/** Случайная витрина отзывов на лендинге (сервер сам перемешивает). */
export function LandingReviews() {
  const { t } = useTranslation()
  const onMouseMove = useCardSpotlight()
  const { data, isLoading } = useQuery({
    queryKey: ['public-reviews-featured'],
    queryFn: () => api.publicReviewsFeatured(3),
    staleTime: 60_000,
  })

  const items = useMemo(() => (data as ReviewsListResponse | undefined)?.items ?? [], [data])

  if (!isLoading && items.length === 0) return null

  return (
    <section id="reviews" className="px-4 py-14 sm:px-6 sm:py-20">
      <div className="mx-auto max-w-6xl">
        <div className="flex flex-col gap-6 sm:flex-row sm:items-end sm:justify-between">
          <SectionHeading
            eyebrow={t('landing.nav.reviews')}
            title={t('landing.reviews.title')}
            description={t('landing.reviews.subtitle')}
          />
          <Reveal delay={0.12}>
            <a
              href="/cabinet/reviews"
              className="landing-nav-link inline-flex items-center gap-1.5 text-sm font-semibold text-[hsl(var(--lp-cyan))]"
            >
              {t('landing.reviews.all')}
              <ArrowRight className="size-4" />
            </a>
          </Reveal>
        </div>

        <div className="mt-10 grid gap-4 sm:mt-14 sm:gap-5 md:grid-cols-3">
          {isLoading
            ? Array.from({ length: 3 }, (_, i) => (
                <div key={i} className="landing-card h-44 animate-pulse opacity-60" />
              ))
            : items.map((item, i) => (
                <Reveal key={item.id} delay={0.05 * i} className="h-full">
                  <ReviewCard
                    review={item}
                    compact
                    variant="landing"
                    className="h-full"
                    onMouseMove={onMouseMove}
                  />
                </Reveal>
              ))}
        </div>
      </div>
    </section>
  )
}
