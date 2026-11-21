import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { ApiError } from '@/lib/api'
import { StarPicker } from '@/features/reviews/StarRating'
import {
  togglesFromVisibility,
  visibilityFromToggles,
  type AdminReview,
  type ReviewStatus,
} from '@/features/reviews/reviewTypes'
import type { AdminReviewCustomerHit } from '@/features/reviews/reviewTypes'
import { useAdminReviewCustomers, useCreateAdminReview, useUpdateAdminReview } from '../hooks/useAdminReviews'
import { AdminModal } from './AdminModal'
import { AdminModalSaveFooter } from './AdminModalSaveFooter'
import { AdminToggleSwitch } from './AdminToggleSwitch'

const STATUSES: ReviewStatus[] = ['published', 'pending', 'hidden']
const STATUS_KEY: Record<ReviewStatus, string> = {
  published: 'admin.reviews.status.published',
  pending: 'admin.reviews.status.pending',
  hidden: 'admin.reviews.status.hidden',
}

interface Props {
  open: boolean
  onClose: () => void
  /** Правим существующий отзыв. Без него — создаём от имени клиента customerId. */
  review?: AdminReview | null
  customerId?: number | null
  /** Сфокусироваться на поле «Ответ команды» (кнопка «Ответить»). */
  focusReply?: boolean
  onSaved?: (message: string) => void
}

const fieldClass =
  'w-full rounded-lg border border-border bg-card px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

/** «2026-11-21» из даты в местном часовом поясе — для <input type="date">. */
function toDateInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function customerLabel(c: AdminReviewCustomerHit): string {
  const name = c.telegram_username ? `@${c.telegram_username}` : ''
  return [`#${c.id}`, name, c.email, c.telegram_id].filter(Boolean).join(' · ')
}

/**
 * Окно отзыва: правка (статус, оценка, текст, дата, ответ команды, как показывать автора)
 * и «отзыв от имени клиента». Правка текста тихая — клиент её не видит, а оригинал
 * хранится на сервере и показывается только здесь.
 */
export function AdminReviewModal({ open, onClose, review, customerId, focusReply, onSaved }: Props) {
  const { t } = useTranslation()
  const update = useUpdateAdminReview()
  const create = useCreateAdminReview()
  const editing = Boolean(review)

  const [status, setStatus] = useState<ReviewStatus>('published')
  const [rating, setRating] = useState(5)
  const [body, setBody] = useState('')
  const [reply, setReply] = useState('')
  const [showName, setShowName] = useState(true)
  const [showAvatar, setShowAvatar] = useState(true)
  const [name, setName] = useState('')
  const [date, setDate] = useState('')
  const [initialDate, setInitialDate] = useState('')
  const [showOriginal, setShowOriginal] = useState(false)
  const [error, setError] = useState('')

  // Клиент для «от имени клиента»: выбирается из подсказок, можно вписать и просто ID.
  const [custQuery, setCustQuery] = useState('')
  const [custPicked, setCustPicked] = useState<number | null>(null)
  const [custDebounced, setCustDebounced] = useState('')
  const [listOpen, setListOpen] = useState(false)

  // Окно открывается заново на каждый отзыв: поля берутся из него.
  useEffect(() => {
    if (!open) return
    setError('')
    setShowOriginal(false)
    setStatus(review?.status ?? 'published')
    setRating(review?.rating ?? 5)
    setBody(review?.body ?? '')
    setReply(review?.reply?.body ?? '')
    const tg = togglesFromVisibility(review?.visibility)
    setShowName(review ? tg.showName : true)
    setShowAvatar(review ? tg.showAvatar : true)
    setName(review?.display_name ?? '')
    const d = review ? toDateInput(review.published_at ?? review.created_at) : ''
    setDate(d)
    setInitialDate(d)
    setCustQuery(customerId ? String(customerId) : '')
    setCustPicked(customerId ?? null)
    setListOpen(false)
  }, [open, review, customerId])

  useEffect(() => {
    const id = setTimeout(() => setCustDebounced(custQuery.trim().replace(/^@/, '')), 300)
    return () => clearTimeout(id)
  }, [custQuery])

  const search = useAdminReviewCustomers(open && !editing && listOpen ? custDebounced : '')
  const suggestions = search.data?.items.slice(0, 8) ?? []

  const pending = update.isPending || create.isPending
  const bodyLen = body.trim().length
  const custNum = custPicked ?? (/^\d+$/.test(custQuery.trim()) ? Number(custQuery.trim()) : 0)
  const valid = bodyLen >= 5 && bodyLen <= 1000 && (editing || (Number.isInteger(custNum) && custNum > 0))
  const today = toDateInput(new Date().toISOString())

  const submit = async () => {
    setError('')
    const visibility = visibilityFromToggles(showName, showName && showAvatar)
    const display_name = showName ? name.trim() : ''
    try {
      if (review) {
        await update.mutateAsync({
          id: review.id,
          body: {
            rating,
            body: body.trim(),
            visibility,
            display_name,
            status,
            reply: reply.trim(),
            // Дату шлём только если её поменяли: лишняя запись сдвинула бы время публикации.
            ...(date && date !== initialDate ? { published_at: date } : {}),
          },
        })
      } else {
        await create.mutateAsync({
          customer_id: custNum,
          rating,
          body: body.trim(),
          visibility,
          display_name,
          status,
          ...(date ? { published_at: date } : {}),
        })
      }
      onSaved?.(t('admin.reviews.saved'))
      onClose()
    } catch (err) {
      setError(
        err instanceof ApiError && err.status === 404 ? t('admin.reviews.modal.noCustomer') : t('admin.reviews.modal.error'),
      )
    }
  }

  const title = review
    ? t('admin.reviews.modal.titleEdit', { name: review.author_name || t('reviews.anonymous') })
    : t('admin.reviews.modal.titleCreate')

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={title}
      description={editing ? t('admin.reviews.modal.subtitleEdit') : t('admin.reviews.modal.subtitleCreate')}
      size="lg"
      footer={<AdminModalSaveFooter onCancel={onClose} onSave={() => void submit()} isPending={pending} saveDisabled={!valid} />}
    >
      <div className="space-y-4">
        {!editing && (
          <div className="relative space-y-1.5">
            <span className="text-sm font-medium">{t('admin.reviews.modal.customerId')}</span>
            <input
              value={custQuery}
              disabled={pending}
              autoComplete="off"
              onChange={(e) => {
                setCustQuery(e.target.value)
                setCustPicked(null)
                setListOpen(true)
              }}
              onFocus={() => setListOpen(true)}
              onBlur={() => setTimeout(() => setListOpen(false), 150)}
              placeholder={t('admin.reviews.modal.customerPlaceholder')}
              className={fieldClass}
            />
            {listOpen && custDebounced.length >= 1 && custPicked == null && (
              <div className="absolute inset-x-0 top-full z-20 mt-1 max-h-64 overflow-auto rounded-lg border border-border bg-card p-1 shadow-xl">
                {search.isLoading ? (
                  <p className="px-3 py-2 text-sm text-muted-foreground">{t('admin.reviews.modal.customerSearching')}</p>
                ) : suggestions.length === 0 ? (
                  <p className="px-3 py-2 text-sm text-muted-foreground">{t('admin.reviews.modal.customerNone')}</p>
                ) : (
                  suggestions.map((c) => (
                    <button
                      key={c.id}
                      type="button"
                      // mousedown, а не click: иначе blur поля закроет список раньше выбора.
                      onMouseDown={(e) => {
                        e.preventDefault()
                        setCustPicked(c.id)
                        setCustQuery(customerLabel(c))
                        setListOpen(false)
                      }}
                      className="flex w-full items-center rounded-md px-3 py-2 text-left text-sm hover:bg-accent"
                    >
                      <span className="truncate">{customerLabel(c)}</span>
                    </button>
                  ))
                )}
              </div>
            )}
          </div>
        )}

        <div className="space-y-1.5">
          <span className="text-sm font-medium">{t('admin.reviews.modal.status')}</span>
          <div className="flex gap-1 rounded-lg bg-muted/50 p-1" role="radiogroup">
            {STATUSES.map((s) => (
              <button
                key={s}
                type="button"
                role="radio"
                aria-checked={status === s}
                disabled={pending}
                onClick={() => setStatus(s)}
                className={cn(
                  'flex-1 rounded-md px-2 py-1.5 text-sm font-medium transition-colors',
                  status === s ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {t(STATUS_KEY[s])}
              </button>
            ))}
          </div>
        </div>

        <div className="space-y-1.5">
          <span className="text-sm font-medium">{t('admin.reviews.modal.rating')}</span>
          <StarPicker value={rating} onChange={setRating} disabled={pending} label={t('admin.reviews.modal.rating')} />
        </div>

        <label className="block space-y-1.5">
          <span className="text-sm font-medium">{t('admin.reviews.modal.body')}</span>
          <textarea
            value={body}
            rows={4}
            maxLength={1000}
            disabled={pending}
            onChange={(e) => setBody(e.target.value)}
            className={cn(fieldClass, 'min-h-[6rem] resize-y')}
          />
          {review?.original_body ? (
            <p className="text-xs text-muted-foreground">
              {t('admin.reviews.modal.edited')} ·{' '}
              <button type="button" className="text-primary hover:underline" onClick={() => setShowOriginal((v) => !v)}>
                {showOriginal ? t('admin.reviews.modal.hideOriginal') : t('admin.reviews.modal.showOriginal')}
              </button>
            </p>
          ) : null}
          {showOriginal && review?.original_body ? (
            <p className="whitespace-pre-line rounded-lg border border-dashed border-border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
              {review.original_body}
            </p>
          ) : null}
        </label>

        <label className="block space-y-1.5">
          <span className="block text-sm font-medium">{t('admin.reviews.modal.dateLabel')}</span>
          <input
            type="date"
            value={date}
            max={today}
            min="2015-01-01"
            disabled={pending}
            onChange={(e) => setDate(e.target.value)}
            className={cn(fieldClass, 'sm:max-w-[14rem]')}
          />
          <p className="text-xs text-muted-foreground">
            {editing ? t('admin.reviews.modal.dateHintEdit') : t('admin.reviews.modal.dateHintCreate')}
          </p>
        </label>

        {editing && (
          <label className="block space-y-1.5">
            <span className="text-sm font-medium">{t('admin.reviews.modal.replyLabel')}</span>
            <textarea
              value={reply}
              rows={3}
              maxLength={1000}
              autoFocus={focusReply}
              disabled={pending}
              onChange={(e) => setReply(e.target.value)}
              className={cn(fieldClass, 'resize-y')}
            />
            <p className="text-xs text-muted-foreground">{t('admin.reviews.modal.replyHint')}</p>
          </label>
        )}

        <div className="grid gap-3 sm:grid-cols-2">
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm font-medium">{t('admin.reviews.modal.showName')}</span>
            <AdminToggleSwitch checked={showName} onChange={setShowName} disabled={pending} aria-label={t('admin.reviews.modal.showName')} />
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className={cn('text-sm font-medium', !showName && 'text-muted-foreground')}>{t('admin.reviews.modal.showAvatar')}</span>
            <AdminToggleSwitch checked={showName && showAvatar} onChange={setShowAvatar} disabled={pending || !showName} aria-label={t('admin.reviews.modal.showAvatar')} />
          </div>
        </div>
        {showName && (
          <label className="block space-y-1.5">
            <span className="text-sm font-medium">{t('admin.reviews.modal.nameLabel')}</span>
            <input value={name} maxLength={120} disabled={pending} onChange={(e) => setName(e.target.value)} className={fieldClass} />
          </label>
        )}

        {error && <p className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
      </div>
    </AdminModal>
  )
}
