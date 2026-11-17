import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Star } from 'lucide-react'

import { AdminModal } from '../AdminModal'
import { AdminModalSaveFooter } from '../AdminModalSaveFooter'
import { AdminToggleSwitch } from '../AdminToggleSwitch'
import { formatAdminApiError } from '../../utils/formatAdminApiError'
import { useErrorHaptic } from '@/lib/haptic'
import { api } from '@/lib/api'
import { ReviewCard } from '@/features/reviews/ReviewCard'
import { StarRatingInput } from '@/features/reviews/StarRating'
import {
  togglesFromVisibility,
  visibilityFromToggles,
  type ReviewVisibility,
} from '@/features/reviews/reviewTypes'
import type { AdminCustomerDTO } from '@/lib/types/admin'
import { useAdminUserUpsertReview } from '../../hooks/useAdminUsers'

interface Props {
  open: boolean
  onClose: () => void
  userId: number
  customer: AdminCustomerDTO
  onSuccess?: (message: string) => void
  onError?: (message: string) => void
}

export function AdminUserReviewModal({
  open,
  onClose,
  userId,
  customer,
  onSuccess,
  onError,
}: Props) {
  const { t } = useTranslation()
  const upsert = useAdminUserUpsertReview(userId)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-user-review', userId],
    queryFn: () => api.adminUserReview(userId),
    enabled: open && userId > 0,
  })

  const defaultName =
    customer.telegram_username?.replace(/^@/, '') ||
    customer.cabinet?.email?.trim() ||
    `#${customer.id}`

  const [rating, setRating] = useState(5)
  const [body, setBody] = useState('')
  const [showName, setShowName] = useState(true)
  const [showAvatar, setShowAvatar] = useState(false)
  const [displayName, setDisplayName] = useState(defaultName)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  useEffect(() => {
    if (!open) return
    setError(null)
    if (data?.has_review) {
      setRating(data.rating || 5)
      setBody(data.body || '')
      const toggles = togglesFromVisibility(data.visibility as ReviewVisibility | undefined)
      setShowName(toggles.showName)
      setShowAvatar(toggles.showAvatar)
      setDisplayName(data.display_name || defaultName)
    } else {
      setRating(5)
      setBody('')
      setShowName(true)
      setShowAvatar(false)
      setDisplayName(defaultName)
    }
  }, [open, data, defaultName])

  const visibility = visibilityFromToggles(showName, showAvatar)

  const preview = useMemo(
    () => ({
      id: 0,
      rating,
      body: body.trim() || t('reviews.previewPlaceholder'),
      author_name: showName ? displayName.trim() || defaultName : null,
      author_avatar_url: showName && showAvatar ? customer.avatar_url || null : null,
      client_since: customer.created_at?.slice(0, 7) || null,
      tariff_name: t('reviews.previewTariff'),
      created_at: new Date().toISOString(),
    }),
    [rating, body, showName, showAvatar, displayName, defaultName, customer, t],
  )

  const handleClose = () => {
    setError(null)
    onClose()
  }

  const handleSave = () => {
    if (data && data.can_edit === false) {
      const msg = t('admin.users.review.noCabinet')
      setError(msg)
      onError?.(msg)
      return
    }
    setError(null)
    upsert.mutate(
      {
        rating,
        body: body.trim(),
        visibility,
        display_name: showName ? displayName.trim() : '',
        avatar_url: showName && showAvatar ? customer.avatar_url || '' : '',
      },
      {
        onSuccess: () => {
          onSuccess?.(t('admin.feedback.saved'))
          handleClose()
        },
        onError: (e) => {
          const msg = formatAdminApiError(e, t)
          setError(msg)
          onError?.(msg)
        },
      },
    )
  }

  const canSave = body.trim().length >= 10 && rating >= 0.5 && rating <= 5 && !upsert.isPending

  return (
    <AdminModal
      open={open}
      onClose={handleClose}
      title={data?.has_review ? t('admin.users.review.editTitle') : t('admin.users.review.title')}
      description={t('admin.users.review.hint')}
      icon={Star}
      iconAccent="amber"
      size="lg"
      footer={
        <AdminModalSaveFooter
          onCancel={handleClose}
          onSave={handleSave}
          isPending={upsert.isPending}
          saveDisabled={!canSave || data?.can_edit === false}
        />
      }
    >
      {isLoading ? (
        <div className="h-40 animate-pulse rounded-xl bg-muted/60" />
      ) : (
        <div className="space-y-4">
          {data?.can_edit === false && (
            <p className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-800 dark:text-amber-200">
              {t('admin.users.review.noCabinet')}
            </p>
          )}
          {error && (
            <p className="rounded-xl border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}

          <div>
            <p className="text-sm font-semibold">{t('reviews.ratingLabel')}</p>
            <div className="mt-2 flex flex-wrap items-center gap-3">
              <StarRatingInput value={rating} onChange={setRating} />
              <span className="text-sm tabular-nums text-muted-foreground">
                {t('reviews.ratingValue', {
                  n: Number.isInteger(rating) ? rating : rating.toFixed(1),
                })}
              </span>
            </div>
          </div>

          <div>
            <label className="text-sm font-semibold" htmlFor="admin-review-body">
              {t('reviews.bodyLabel')}
            </label>
            <textarea
              id="admin-review-body"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              rows={5}
              maxLength={2000}
              className="mt-2 w-full resize-y rounded-xl border border-border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-primary"
              placeholder={t('reviews.bodyPlaceholder')}
            />
          </div>

          <div>
            <p className="text-sm font-semibold">{t('reviews.visibilityLabel')}</p>
            <div className="mt-2 space-y-1">
              <div className="flex items-start justify-between gap-4 border-b border-border/50 py-3">
                <div className="min-w-0">
                  <p className="text-sm font-medium">{t('reviews.toggles.showName')}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">{t('reviews.toggles.showNameHint')}</p>
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
                  <p className="mt-0.5 text-xs text-muted-foreground">{t('reviews.toggles.showAvatarHint')}</p>
                </div>
                <AdminToggleSwitch
                  checked={showAvatar}
                  disabled={!showName}
                  onChange={setShowAvatar}
                  aria-label={t('reviews.toggles.showAvatar')}
                />
              </div>
              {showName && (
                <div className="pt-3">
                  <label className="text-sm font-semibold" htmlFor="admin-review-name">
                    {t('reviews.nicknameLabel')}
                  </label>
                  <input
                    id="admin-review-name"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    maxLength={120}
                    placeholder={t('reviews.nicknamePlaceholder')}
                    className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  />
                </div>
              )}
            </div>
          </div>

          <div>
            <p className="mb-2 text-sm font-semibold">{t('reviews.previewLabel')}</p>
            <ReviewCard review={preview} compact variant="cabinet" />
          </div>
        </div>
      )}
    </AdminModal>
  )
}
