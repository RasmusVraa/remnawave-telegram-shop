import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, Info } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { AdminToggleSwitch } from '@/features/admin/components/AdminToggleSwitch'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { ApiError, api } from '@/lib/api'
import { hapticError, hapticSuccess } from '@/lib/haptic'
import { ReviewCard } from './ReviewCard'
import { StarPicker } from './StarRating'
import { togglesFromVisibility, visibilityFromToggles } from './reviewTypes'

const MIN = 5
const MAX = 1000

export default function WriteReviewPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const mineQ = useQuery({ queryKey: ['my-review'], queryFn: () => api.myReview(), staleTime: 0 })
  const mine = mineQ.data
  const review = mine?.review ?? null

  const [rating, setRating] = useState(5)
  const [body, setBody] = useState('')
  const [showName, setShowName] = useState(true)
  const [showAvatar, setShowAvatar] = useState(true)
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  // Форма заполняется один раз, когда пришёл свой отзыв.
  useEffect(() => {
    if (!mine || loaded) return
    setLoaded(true)
    if (mine.review) {
      setRating(mine.review.rating)
      setBody(mine.review.body)
      const tg = togglesFromVisibility(mine.review.visibility)
      setShowName(tg.showName)
      setShowAvatar(tg.showAvatar)
      setName(mine.review.display_name)
    } else {
      // Новый отзыв: имя из профиля (Telegram, почта), чтобы не печатать самому.
      setName(mine.author?.name ?? '')
    }
  }, [mine, loaded])

  const save = useMutation({
    mutationFn: () =>
      api.saveMyReview({
        rating,
        body: body.trim(),
        visibility: visibilityFromToggles(showName, showName && showAvatar),
        display_name: showName ? name.trim() : '',
      }),
    onSuccess: async () => {
      hapticSuccess()
      await qc.invalidateQueries({ queryKey: ['my-review'] })
      await qc.invalidateQueries({ queryKey: ['public-reviews'] })
      navigate('/reviews')
    },
    onError: (err) => {
      hapticError()
      setError(err instanceof ApiError && err.status === 403 ? t('reviews.form.forbidden') : t('reviews.form.error'))
    },
  })

  const len = body.trim().length
  const valid = len >= MIN && len <= MAX && rating >= 1
  const blocked = mine ? !mine.enabled || (!mine.can_write && !review) : false

  // Писать нельзя — обратно к списку, там причина.
  useEffect(() => {
    if (mine && blocked) navigate('/reviews', { replace: true })
  }, [mine, blocked, navigate])

  const previewName = showName ? name.trim() || review?.display_name || null : null
  const preview = {
    rating,
    body: body.trim() || t('reviews.form.bodyPlaceholder'),
    author_name: previewName,
    author_avatar_url: showName && showAvatar ? (review?.author_avatar_url ?? mine?.author?.avatar_url ?? null) : null,
    client_since: review?.client_since ?? null,
    tariff_label: review?.tariff_label ?? '',
    is_trial: review?.is_trial ?? false,
    created_at: review?.created_at ?? new Date().toISOString(),
    reply: undefined,
  }

  const actions = (cls: string) => (
    <>
      <Button
        className={cls}
        loading={save.isPending}
        disabled={!valid}
        onClick={() => {
          setError('')
          save.mutate()
        }}
      >
        {review ? t('reviews.form.save') : t('reviews.form.publish')}
      </Button>
      <Button asChild variant="outline" className={cls} disabled={save.isPending}>
        <Link to="/reviews">{t('admin.cancel')}</Link>
      </Button>
    </>
  )

  return (
    <AppLayout>
      {/* Телефон: «Опубликовать» и «Отмена» закреплены внизу над нижним меню, как кнопка оплаты. */}
      {mine && typeof document !== 'undefined' &&
        createPortal(
          <div className="cabinet-pay-scrim fixed inset-x-0 bottom-0 z-[39] px-3 pt-8 pb-[calc(var(--cabinet-bottom-nav-h,calc(73px+var(--cabinet-tg-safe-bottom)))+0.375rem)] sm:hidden">
            <div className="mx-auto flex w-full max-w-lg gap-2">{actions('flex-1')}</div>
          </div>,
          document.body,
        )}
      <PageReveal className="mx-auto max-w-5xl space-y-5">
        <RevealItem>
          <Link to="/reviews" className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
            <ArrowLeft className="size-4" aria-hidden />
            {t('reviews.form.back')}
          </Link>
          <h1 className="mt-2 font-heading text-2xl font-bold tracking-tight sm:text-3xl">
            {review ? t('reviews.form.titleEdit') : t('reviews.form.titleNew')}
          </h1>
          <p className="mt-1.5 text-muted-foreground">{t('reviews.form.subtitle')}</p>
        </RevealItem>

        {mineQ.isLoading || !mine ? (
          <div className="flex justify-center py-12">
            <Spinner />
          </div>
        ) : (
          <>
            {mine.moderated && (
              <div className="flex items-start gap-3 rounded-2xl border border-primary/30 bg-primary/[0.07] px-4 py-3 text-sm">
                <Info className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
                <p>{t('reviews.form.moderatedNote')}</p>
              </div>
            )}
            <div className="grid gap-5 lg:grid-cols-[1fr_340px]">
              <div className="space-y-4">
                <Card className="space-y-3 p-5">
                  <h2 className="text-base font-semibold">{t('reviews.form.ratingTitle')}</h2>
                  <StarPicker value={rating} onChange={setRating} disabled={save.isPending} label={t('reviews.form.ratingTitle')} />
                  <p className="text-xs text-muted-foreground">{t('reviews.form.rating', { n: rating })}</p>
                </Card>

                <Card className="space-y-3 p-5">
                  <h2 className="text-base font-semibold">{t('reviews.form.bodyTitle')}</h2>
                  <textarea
                    value={body}
                    maxLength={MAX}
                    rows={6}
                    disabled={save.isPending}
                    onChange={(e) => setBody(e.target.value)}
                    placeholder={t('reviews.form.bodyPlaceholder')}
                    className="min-h-[9rem] w-full resize-y rounded-xl border border-input bg-background px-3.5 py-3 text-[15px] leading-relaxed shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                  />
                  <p className="text-xs text-muted-foreground">{t('reviews.form.bodyHint', { min: MIN, max: MAX, n: len })}</p>
                </Card>

                <Card className="space-y-4 p-5">
                  <h2 className="text-base font-semibold">{t('reviews.form.visibilityTitle')}</h2>
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <p className="text-sm font-medium">{t('reviews.form.showName')}</p>
                      <p className="text-xs text-muted-foreground">{t('reviews.form.showNameHint')}</p>
                    </div>
                    <AdminToggleSwitch checked={showName} onChange={setShowName} disabled={save.isPending} aria-label={t('reviews.form.showName')} />
                  </div>
                  {showName && (
                    <>
                      <div className="flex items-center justify-between gap-4">
                        <div>
                          <p className="text-sm font-medium">{t('reviews.form.showAvatar')}</p>
                          <p className="text-xs text-muted-foreground">{t('reviews.form.showAvatarHint')}</p>
                        </div>
                        <AdminToggleSwitch checked={showAvatar} onChange={setShowAvatar} disabled={save.isPending} aria-label={t('reviews.form.showAvatar')} />
                      </div>
                      <label className="block space-y-1.5">
                        <span className="text-sm font-medium">{t('reviews.form.nameLabel')}</span>
                        <input
                          value={name}
                          maxLength={120}
                          disabled={save.isPending}
                          onChange={(e) => setName(e.target.value)}
                          placeholder={t('reviews.form.namePlaceholder')}
                          className="h-11 w-full rounded-xl border border-input bg-background px-3.5 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                        />
                      </label>
                    </>
                  )}
                </Card>

                {error && <p className="rounded-xl border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}

                {/* ПК: кнопки в потоке страницы. */}
                <div className="hidden flex-wrap gap-2 sm:flex">{actions('')}</div>
                {/* Телефон: место под закреплённую панель, чтобы она не закрывала форму. */}
                <div className="h-20 sm:hidden" aria-hidden />
              </div>

              <div>
                <p className="mb-2 text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('reviews.form.preview')}</p>
                <ReviewCard review={preview} />
              </div>
            </div>
          </>
        )}
      </PageReveal>
    </AppLayout>
  )
}
