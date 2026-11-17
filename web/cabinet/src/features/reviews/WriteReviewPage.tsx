import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, Check } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { AdminToggleSwitch } from '@/features/admin/components/AdminToggleSwitch'
import { api, ApiError } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import { useErrorHaptic } from '@/lib/haptic'

import { ReviewCard } from './ReviewCard'
import { StarRatingInput } from './StarRating'
import {
  togglesFromVisibility,
  visibilityFromToggles,
  type MyReviewResponse,
} from './reviewTypes'

export default function WriteReviewPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const user = useAuthStore((s) => s.user)

  const { data, isLoading } = useQuery({
    queryKey: ['me-review'],
    queryFn: () => api.myReview(),
  })

  const defaultNick = user?.display_name || user?.username || ''
  const [rating, setRating] = useState(5)
  const [body, setBody] = useState('')
  const [showName, setShowName] = useState(true)
  const [showAvatar, setShowAvatar] = useState(false)
  const [displayName, setDisplayName] = useState(defaultNick)
  const [savedOk, setSavedOk] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  useEffect(() => {
    if (!data?.has_review) {
      setDisplayName(defaultNick)
      return
    }
    setRating(data.rating || 5)
    setBody(data.body || '')
    const toggles = togglesFromVisibility(data.visibility)
    setShowName(toggles.showName)
    setShowAvatar(toggles.showAvatar)
    setDisplayName(data.display_name || data.author_name || defaultNick)
  }, [data, defaultNick])

  const visibility = visibilityFromToggles(showName, showAvatar)

  const preview = useMemo(() => {
    const name = displayName.trim() || defaultNick || t('reviews.anonymous')
    return {
      id: 0,
      rating,
      body: body.trim() || t('reviews.previewPlaceholder'),
      author_name: showName ? name : null,
      author_avatar_url: showName && showAvatar ? user?.avatar_url || null : null,
      client_since: user?.registered_at?.slice(0, 7) || '2024-03',
      tariff_name: t('reviews.previewTariff'),
      created_at: new Date().toISOString(),
    }
  }, [rating, body, showName, showAvatar, displayName, defaultNick, user, t])

  const save = useMutation({
    mutationFn: () =>
      api.saveReview({
        rating,
        body: body.trim(),
        visibility,
        display_name: showName ? displayName.trim() : '',
      }),
    meta: { haptics: true },
    onSuccess: (resp) => {
      setSavedOk(true)
      setError(null)
      qc.setQueryData(['me-review'], resp)
      void qc.invalidateQueries({ queryKey: ['public-reviews'] })
      void qc.invalidateQueries({ queryKey: ['public-reviews-featured'] })
    },
    onError: (err) => {
      setSavedOk(false)
      setError(err instanceof ApiError ? err.message : t('reviews.saveError'))
    },
  })

  const canSubmit = body.trim().length >= 10 && rating >= 0.5 && rating <= 5 && !save.isPending

  return (
    <AppLayout>
      <PageReveal className="mx-auto w-full max-w-6xl space-y-6 px-1 sm:px-0">
        <RevealItem>
          <a
            href="/cabinet/reviews"
            className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
          >
            <ArrowLeft className="size-3.5" />
            {t('reviews.backToList')}
          </a>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
            {data?.has_review ? t('reviews.editTitle') : t('reviews.writeTitle')}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('reviews.writeSubtitle')}</p>
        </RevealItem>

        {isLoading ? (
          <RevealItem>
            <Skeleton className="h-72 w-full rounded-[var(--radius)]" />
          </RevealItem>
        ) : (
          <div className="space-y-6">
            <RevealItem>
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">{t('reviews.ratingLabel')}</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-wrap items-center gap-4">
                  <StarRatingInput value={rating} onChange={setRating} />
                  <span className="text-sm tabular-nums text-muted-foreground">
                    {t('reviews.ratingValue', {
                      n: Number.isInteger(rating) ? rating : rating.toFixed(1),
                    })}
                  </span>
                </CardContent>
              </Card>
            </RevealItem>

            <RevealItem>
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">
                    <Label htmlFor="review-body">{t('reviews.bodyLabel')}</Label>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <textarea
                    id="review-body"
                    value={body}
                    onChange={(e) => setBody(e.target.value)}
                    rows={7}
                    maxLength={2000}
                    placeholder={t('reviews.bodyPlaceholder')}
                    className="cabinet-card min-h-[9rem] w-full resize-y rounded-[calc(var(--radius)-2px)] border border-border bg-background px-3 py-2.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                  <p className="mt-2 text-xs text-muted-foreground">
                    {t('reviews.bodyHint', { n: body.trim().length })}
                  </p>
                </CardContent>
              </Card>
            </RevealItem>

            <RevealItem>
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">{t('reviews.visibilityLabel')}</CardTitle>
                </CardHeader>
                <CardContent className="space-y-1">
                  <div className="flex items-start justify-between gap-4 border-b border-border/50 py-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{t('reviews.toggles.showName')}</p>
                      <p className="mt-0.5 text-xs text-muted-foreground">
                        {t('reviews.toggles.showNameHint')}
                      </p>
                    </div>
                    <AdminToggleSwitch
                      checked={showName}
                      onChange={(v) => {
                        setShowName(v)
                        if (!v) setShowAvatar(false)
                      }}
                      aria-label={t('reviews.toggles.showName')}
                    />
                  </div>
                  <div className="flex items-start justify-between gap-4 border-b border-border/50 py-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{t('reviews.toggles.showAvatar')}</p>
                      <p className="mt-0.5 text-xs text-muted-foreground">
                        {t('reviews.toggles.showAvatarHint')}
                      </p>
                    </div>
                    <AdminToggleSwitch
                      checked={showAvatar}
                      disabled={!showName}
                      onChange={setShowAvatar}
                      aria-label={t('reviews.toggles.showAvatar')}
                    />
                  </div>
                  {showName && (
                    <div className="space-y-1.5 pt-3">
                      <Label htmlFor="review-nick">{t('reviews.nicknameLabel')}</Label>
                      <Input
                        id="review-nick"
                        value={displayName}
                        onChange={(e) => setDisplayName(e.target.value)}
                        maxLength={120}
                        placeholder={t('reviews.nicknamePlaceholder')}
                      />
                    </div>
                  )}
                </CardContent>
              </Card>
            </RevealItem>

            <RevealItem>
              <p className="mb-2 text-sm font-medium text-muted-foreground">{t('reviews.previewLabel')}</p>
              <ReviewCard review={preview} variant="cabinet" />
            </RevealItem>

            {error && (
              <RevealItem>
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              </RevealItem>
            )}
            {savedOk && (
              <RevealItem>
                <Alert>
                  <AlertDescription className="inline-flex items-center gap-1.5 text-emerald-700 dark:text-emerald-300">
                    <Check className="size-4" />
                    {t('reviews.saved')}
                  </AlertDescription>
                </Alert>
              </RevealItem>
            )}

            <RevealItem>
              <Button
                type="button"
                disabled={!canSubmit}
                loading={save.isPending}
                onClick={() => save.mutate()}
              >
                {(data as MyReviewResponse | undefined)?.has_review
                  ? t('reviews.update')
                  : t('reviews.publish')}
              </Button>
            </RevealItem>
          </div>
        )}
      </PageReveal>
    </AppLayout>
  )
}
