import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Pencil, Star, Trash2 } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminTablePagination } from '../components/AdminTablePagination'
import { AdminConfirmModal } from '../components/AdminConfirmModal'
import { AdminModal } from '../components/AdminModal'
import { AdminModalSaveFooter } from '../components/AdminModalSaveFooter'
import { AdminToggleSwitch } from '../components/AdminToggleSwitch'
import { StarRatingInput } from '@/features/reviews/StarRating'
import {
  togglesFromVisibility,
  visibilityFromToggles,
  type ReviewVisibility,
} from '@/features/reviews/reviewTypes'
import { ReviewCard } from '@/features/reviews/ReviewCard'
import { formatAdminDateTime } from '../utils/datetime'
import { formatAdminApiError } from '../utils/formatAdminApiError'
import { useErrorHaptic } from '@/lib/haptic'
import { Spinner } from '@/components/ui/spinner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  useAdminReviewDelete,
  useAdminReviews,
  useAdminReviewUpdate,
  type AdminReviewDTO,
} from '../hooks/useAdminReviews'

const PAGE_LIMIT = 20

export default function AdminReviewsPage() {
  const { t } = useTranslation()
  const [page, setPage] = useState(0)
  const offset = page * PAGE_LIMIT
  const { data, isLoading, isError } = useAdminReviews(PAGE_LIMIT, offset)
  const update = useAdminReviewUpdate()
  const remove = useAdminReviewDelete()

  const [edit, setEdit] = useState<AdminReviewDTO | null>(null)
  const [del, setDel] = useState<AdminReviewDTO | null>(null)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_LIMIT))

  return (
    <AdminLayout>
      <div className="space-y-5">
        <AdminPageHeader
          icon={Star}
          title={t('admin.reviews.title')}
          subtitle={t('admin.reviews.subtitle', { count: total })}
          accent="amber"
        />

        {isLoading ? (
          <div className="flex h-40 items-center justify-center">
            <Spinner className="size-6" />
          </div>
        ) : isError ? (
          <p className="rounded-xl border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {t('admin.reviews.loadError')}
          </p>
        ) : items.length === 0 ? (
          <p className="rounded-xl border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">
            {t('admin.reviews.empty')}
          </p>
        ) : (
          <>
            <div className="overflow-hidden rounded-xl border border-border bg-card">
              <table className="w-full text-left">
                <thead className="border-b border-border/60 bg-muted/30 text-xs uppercase tracking-wide text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2.5 font-medium">#</th>
                    <th className="px-3 py-2.5 font-medium">{t('admin.reviews.colAuthor')}</th>
                    <th className="px-3 py-2.5 font-medium">{t('admin.reviews.colRating')}</th>
                    <th className="hidden px-3 py-2.5 font-medium md:table-cell">{t('admin.reviews.colBody')}</th>
                    <th className="hidden px-3 py-2.5 font-medium lg:table-cell">{t('admin.reviews.colDate')}</th>
                    <th className="px-3 py-2.5 font-medium text-right">{t('admin.actions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((item) => (
                    <tr key={item.id} className="border-b border-border/40 last:border-0">
                      <td className="whitespace-nowrap px-3 py-2.5 font-mono text-sm tabular-nums">#{item.id}</td>
                      <td className="max-w-[10rem] truncate px-3 py-2.5 text-sm">
                        {item.author_name?.trim() || t('reviews.anonymous')}
                        {item.customer_id ? (
                          <Link
                            to={`/admin/users/${item.customer_id}`}
                            className="mt-0.5 block text-xs text-primary hover:underline"
                          >
                            #{item.customer_id}
                          </Link>
                        ) : null}
                      </td>
                      <td className="whitespace-nowrap px-3 py-2.5 text-sm tabular-nums">
                        {Number.isInteger(item.rating) ? item.rating : item.rating.toFixed(1)}
                      </td>
                      <td className="hidden max-w-[18rem] truncate px-3 py-2.5 text-sm text-muted-foreground md:table-cell">
                        {item.body}
                      </td>
                      <td className="hidden whitespace-nowrap px-3 py-2.5 text-sm text-muted-foreground lg:table-cell">
                        {formatAdminDateTime(item.created_at)}
                      </td>
                      <td className="px-3 py-2.5">
                        <div className="flex justify-end gap-1">
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              setError(null)
                              setEdit(item)
                            }}
                            aria-label={t('admin.reviews.edit')}
                          >
                            <Pencil className="size-4" />
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="text-destructive hover:text-destructive"
                            onClick={() => {
                              setError(null)
                              setDel(item)
                            }}
                            aria-label={t('admin.reviews.delete')}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <AdminTablePagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </>
        )}
      </div>

      {edit && (
        <AdminReviewEditModal
          review={edit}
          open
          error={error}
          saving={update.isPending}
          onClose={() => setEdit(null)}
          onSave={(body) => {
            setError(null)
            update.mutate(
              { id: edit.id, ...body },
              {
                onSuccess: () => setEdit(null),
                onError: (e) => setError(formatAdminApiError(e, t)),
              },
            )
          }}
        />
      )}

      <AdminConfirmModal
        open={Boolean(del)}
        onClose={() => setDel(null)}
        onConfirm={() => {
          if (!del) return
          setError(null)
          remove.mutate(del.id, {
            onSuccess: () => setDel(null),
            onError: (e) => setError(formatAdminApiError(e, t)),
          })
        }}
        title={t('admin.reviews.deleteTitle')}
        message={t('admin.reviews.deleteHint')}
        confirmLabel={t('admin.reviews.delete')}
        variant="destructive"
        loading={remove.isPending}
        icon={Trash2}
        iconAccent="rose"
      />
    </AdminLayout>
  )
}

function AdminReviewEditModal({
  review,
  open,
  error,
  saving,
  onClose,
  onSave,
}: {
  review: AdminReviewDTO
  open: boolean
  error: string | null
  saving: boolean
  onClose: () => void
  onSave: (body: {
    rating: number
    body: string
    visibility: string
    display_name: string
    avatar_url: string
    tariff_name: string
  }) => void
}) {
  const { t } = useTranslation()
  const initial = useMemo(() => togglesFromVisibility(review.visibility as ReviewVisibility), [review.visibility])
  const [rating, setRating] = useState(review.rating)
  const [body, setBody] = useState(review.body)
  const [showName, setShowName] = useState(initial.showName)
  const [showAvatar, setShowAvatar] = useState(initial.showAvatar)
  const [displayName, setDisplayName] = useState(review.display_name || review.author_name || '')
  const [tariffName, setTariffName] = useState(review.tariff_name || '')

  const visibility = visibilityFromToggles(showName, showAvatar)
  const canSave = body.trim().length >= 10 && rating >= 0.5 && rating <= 5 && !saving

  const preview = {
    id: review.id,
    rating,
    body: body.trim() || t('reviews.previewPlaceholder'),
    author_name: showName ? displayName.trim() || t('reviews.anonymous') : null,
    author_avatar_url: showName && showAvatar ? review.avatar_url || null : null,
    client_since: review.client_since,
    tariff_name: tariffName.trim() || t('reviews.previewTariff'),
    created_at: review.created_at,
  }

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={t('admin.reviews.editTitle')}
      description={t('admin.reviews.editSilentHint')}
      icon={Star}
      iconAccent="amber"
      size="lg"
      footer={
        <AdminModalSaveFooter
          onCancel={onClose}
          onSave={() =>
            onSave({
              rating,
              body: body.trim(),
              visibility,
              display_name: showName ? displayName.trim() : '',
              avatar_url: showName && showAvatar ? review.avatar_url || '' : '',
              tariff_name: tariffName.trim(),
            })
          }
          isPending={saving}
          saveDisabled={!canSave}
        />
      }
    >
      <div className="space-y-4">
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
          <label className="text-sm font-semibold" htmlFor="admin-reviews-body">
            {t('reviews.bodyLabel')}
          </label>
          <textarea
            id="admin-reviews-body"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={5}
            maxLength={2000}
            className="mt-2 w-full resize-y rounded-xl border border-border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-primary"
          />
        </div>
        <div>
          <p className="text-sm font-semibold">{t('reviews.visibilityLabel')}</p>
          <div className="mt-2 space-y-1">
            <div className="flex items-start justify-between gap-4 border-b border-border/50 py-3">
              <div className="min-w-0">
                <p className="text-sm font-medium">{t('reviews.toggles.showName')}</p>
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
                <label className="text-sm font-semibold" htmlFor="admin-reviews-nick">
                  {t('reviews.nicknameLabel')}
                </label>
                <input
                  id="admin-reviews-nick"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  maxLength={120}
                  className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-primary"
                />
              </div>
            )}
          </div>
        </div>
        <div>
          <label className="text-sm font-semibold" htmlFor="admin-reviews-tariff">
            {t('admin.reviews.tariffLabel')}
          </label>
          <input
            id="admin-reviews-tariff"
            value={tariffName}
            onChange={(e) => setTariffName(e.target.value)}
            maxLength={255}
            className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-primary"
          />
        </div>
        <div>
          <p className={cn('mb-2 text-sm font-semibold')}>{t('reviews.previewLabel')}</p>
          <ReviewCard review={preview} compact variant="cabinet" />
        </div>
      </div>
    </AdminModal>
  )
}
