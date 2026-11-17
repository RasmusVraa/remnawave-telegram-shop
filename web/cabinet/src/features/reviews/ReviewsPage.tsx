import { useLayoutEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { MessageSquarePlus, PenLine } from 'lucide-react'

import { api } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import { useLandingBrand } from '@/features/landing/useLandingBrand'
import { useLandingCopy } from '@/features/landing/useLandingCopy'
import { LandingHeader } from '@/features/landing/components/LandingHeader'
import { LandingFooter } from '@/features/landing/components/LandingFooter'
import { LandingHolidayFrame } from '@/features/landing/components/LandingHolidayFrame'
import { Reveal, Rise, useCardSpotlight } from '@/features/landing/components/LandingMotion'
import { SectionHeading } from '@/features/landing/components/LandingPrimitives'
import '@/features/landing/landing.css'

import { ReviewCard } from './ReviewCard'
import type { ReviewsListResponse } from './reviewTypes'

/** Публичный список отзывов: /reviews и /cabinet/reviews. */
export default function ReviewsPage() {
  const { t } = useTranslation()
  const brand = useLandingBrand()
  const copy = useLandingCopy()
  const onMouseMove = useCardSpotlight()
  const hasSession = Boolean(useAuthStore((s) => s.accessToken))
  const { data, isLoading, isError } = useQuery({
    queryKey: ['public-reviews'],
    queryFn: () => api.publicReviews(),
    staleTime: 30_000,
  })

  useLayoutEffect(() => {
    const root = document.documentElement
    root.dataset.landing = '1'
    return () => {
      delete root.dataset.landing
    }
  }, [])

  const items = ([...(data as ReviewsListResponse | undefined)?.items ?? []]).sort((a, b) => {
    const ta = Date.parse(a.created_at) || 0
    const tb = Date.parse(b.created_at) || 0
    return tb - ta
  })
  const writeHref = hasSession || brand.authenticated ? '/cabinet/reviews/write' : '/cabinet/login'

  return (
    <LandingHolidayFrame>
      <LandingHeader brand={brand} copy={copy} />
      <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-16">
        <div className="flex flex-col gap-6 sm:flex-row sm:items-end sm:justify-between">
          <SectionHeading
            eyebrow={t('reviews.eyebrow')}
            title={t('reviews.title')}
            description={t('reviews.subtitle')}
          />
          <Rise delay={0.12}>
            <a
              href={writeHref}
              className="landing-cta landing-cta--solid inline-flex h-11 shrink-0 items-center justify-center gap-2 rounded-full px-5 text-sm font-semibold"
            >
              {hasSession || brand.authenticated ? (
                <PenLine className="size-4" />
              ) : (
                <MessageSquarePlus className="size-4" />
              )}
              {hasSession || brand.authenticated ? t('reviews.writeCta') : t('reviews.loginToWrite')}
            </a>
          </Rise>
        </div>

        <div className="mt-10 flex flex-col gap-4 sm:mt-12">
          {isLoading &&
            Array.from({ length: 4 }, (_, i) => (
              <div key={i} className="landing-card h-40 w-full animate-pulse opacity-60" />
            ))}

          {isError && (
            <div className="landing-card border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
              {t('reviews.loadError')}
            </div>
          )}

          {!isLoading && !isError && items.length === 0 && (
            <div className="landing-card border-dashed p-8 text-center">
              <p className="text-sm text-muted-foreground">{t('reviews.empty')}</p>
            </div>
          )}

          {items.map((item, i) => (
            <Reveal key={item.id} delay={0.03 * Math.min(i, 12)}>
              <ReviewCard
                review={item}
                variant="landing"
                className="w-full"
                onMouseMove={onMouseMove}
              />
            </Reveal>
          ))}
        </div>
      </main>
      <LandingFooter brand={brand} />
    </LandingHolidayFrame>
  )
}
