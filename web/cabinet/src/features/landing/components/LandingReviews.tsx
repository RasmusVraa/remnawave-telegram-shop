import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'
import { ReviewCard } from '@/features/reviews/ReviewCard'
import { Reveal } from './LandingMotion'
import { SectionHeading } from './LandingPrimitives'

/**
 * Витрина отзывов: случайные опубликованные. Сервер сам отдаёт пусто, если раздел
 * выключен, лимит нулевой или отзывов меньше трёх, — тогда секции нет вовсе.
 */
export function LandingReviews() {
  const { t } = useTranslation()
  const { data } = useQuery({
    queryKey: ['public-reviews-featured'],
    queryFn: () => api.publicReviewsFeatured(),
    staleTime: 60_000,
    retry: false,
  })
  const items = data?.items ?? []
  if (items.length === 0) return null

  return (
    <section id="reviews" className="px-4 py-14 sm:px-6 sm:py-20">
      <div className="mx-auto max-w-6xl">
        <SectionHeading eyebrow={t('reviews.eyebrow')} title={t('reviews.title')} description={t('reviews.subtitle')} />
        <div className="mt-9 grid gap-4 sm:mt-12 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((r, i) => (
            <Reveal key={r.id} delay={0.05 * (i % 3)} y={18}>
              <ReviewCard review={r} variant="landing" />
            </Reveal>
          ))}
        </div>
      </div>
    </section>
  )
}
