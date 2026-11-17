import { useState, type MouseEventHandler } from 'react'
import { useTranslation } from 'react-i18next'
import { UserRound } from 'lucide-react'

import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

import { ReviewStars } from './StarRating'
import type { ReviewItem } from './reviewTypes'

function formatClientSince(ym: string | null | undefined, locale: string): string {
  if (!ym || !/^\d{4}-\d{2}$/.test(ym)) return ''
  const [y, m] = ym.split('-').map(Number)
  const d = new Date(Date.UTC(y, m - 1, 1))
  // С днём месяца в ru получается родительный падеж («августа»), без дня — именительный («август»).
  if (locale.startsWith('ru')) {
    const parts = new Intl.DateTimeFormat('ru-RU', {
      day: 'numeric',
      month: 'long',
      year: 'numeric',
      timeZone: 'UTC',
    }).formatToParts(d)
    const month = parts.find((p) => p.type === 'month')?.value ?? ''
    const year = parts.find((p) => p.type === 'year')?.value ?? ''
    return `${month} ${year}`.trim()
  }
  return new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(d)
}

function formatReviewDate(iso: string, locale: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'long', year: 'numeric' }).format(d)
}

/**
 * Карточка отзыва.
 *
 * variant=landing — `.landing-card` под декор-темы лендинга.
 * variant=cabinet — `.cabinet-elevated-card` как у остальных экранов кабинета.
 */
export function ReviewCard({
  review,
  className,
  compact = false,
  variant = 'cabinet',
  onMouseMove,
}: {
  review: ReviewItem
  className?: string
  compact?: boolean
  variant?: 'landing' | 'cabinet'
  onMouseMove?: MouseEventHandler<HTMLElement>
}) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language === 'en' ? 'en-US' : 'ru-RU'
  const name = review.author_name?.trim() || t('reviews.anonymous')
  const since = formatClientSince(review.client_since, locale)
  const tariff = review.tariff_name?.trim()
  const meta = [since ? t('reviews.clientSince', { since }) : null, tariff || null].filter(Boolean).join(' · ')
  const posted = formatReviewDate(review.created_at, locale)
  const edited = review.edited_at ? formatReviewDate(review.edited_at, locale) : ''

  const [originalOpen, setOriginalOpen] = useState(false)
  const [originalBody, setOriginalBody] = useState<string | null>(null)
  const [originalLoading, setOriginalLoading] = useState(false)
  const [originalError, setOriginalError] = useState(false)

  const loadOriginal = async () => {
    if (originalBody != null) {
      setOriginalOpen(true)
      return
    }
    setOriginalLoading(true)
    setOriginalError(false)
    try {
      const res = await api.publicReviewOriginal(review.id)
      setOriginalBody(res.body)
      setOriginalOpen(true)
    } catch {
      setOriginalError(true)
    } finally {
      setOriginalLoading(false)
    }
  }

  return (
    <article
      onMouseMove={onMouseMove}
      className={cn(
        variant === 'landing'
          ? 'landing-card p-5 sm:p-6'
          : 'cabinet-elevated-card rounded-[var(--radius)] p-4 sm:p-5',
        className,
      )}
    >
      <div className="flex items-start gap-3">
        {review.author_avatar_url ? (
          <img
            src={review.author_avatar_url}
            alt=""
            className="size-11 shrink-0 rounded-full object-cover ring-1 ring-border/80"
            loading="lazy"
          />
        ) : (
          <span
            className={cn(
              'flex size-11 shrink-0 items-center justify-center rounded-full ring-1 ring-border/80',
              variant === 'landing'
                ? 'bg-[hsl(var(--lp-cyan)/0.12)] text-[hsl(var(--lp-cyan))]'
                : 'bg-primary/10 text-primary',
            )}
          >
            <UserRound className="size-5" strokeWidth={1.75} />
          </span>
        )}
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
            <h3 className="truncate font-heading text-base font-bold tracking-tight text-foreground">
              {name}
            </h3>
            <ReviewStars rating={review.rating} size={compact ? 'sm' : 'md'} />
          </div>
          {meta ? <p className="mt-1 text-xs text-muted-foreground">{meta}</p> : null}
        </div>
      </div>

      <p
        className={cn(
          'mt-3.5 text-pretty text-sm leading-relaxed text-muted-foreground',
          compact && 'line-clamp-4',
        )}
      >
        {review.body}
      </p>

      {(posted || edited || review.has_original) && (
        <div className="mt-3 flex flex-wrap items-end justify-between gap-x-3 gap-y-1">
          <div className="min-w-0">
            {review.has_original && (
              <>
                <button
                  type="button"
                  onClick={() => void loadOriginal()}
                  disabled={originalLoading}
                  className="text-xs font-medium text-primary hover:underline disabled:opacity-60"
                >
                  {originalLoading ? t('reviews.originalLoading') : t('reviews.viewOriginal')}
                </button>
                {originalError && (
                  <p className="mt-1 text-xs text-destructive">{t('reviews.originalError')}</p>
                )}
              </>
            )}
          </div>
          <p className="ml-auto text-right text-xs text-muted-foreground/90">
            {posted ? t('reviews.postedOn', { date: posted }) : null}
            {edited ? (
              <>
                {posted ? <br /> : null}
                {t('reviews.editedOn', { date: edited })}
              </>
            ) : null}
          </p>
        </div>
      )}

      {originalOpen && originalBody != null && (
        <div
          className={cn(
            'mt-2 rounded-xl border border-border/70 bg-background/50 p-3 text-sm text-muted-foreground',
            variant === 'landing' && 'border-[hsl(var(--lp-cyan)/0.25)]',
          )}
        >
          <p className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground/80">
            {t('reviews.originalLabel')}
          </p>
          <p className="text-pretty leading-relaxed">{originalBody}</p>
          <button
            type="button"
            className="mt-2 text-xs font-medium text-muted-foreground hover:text-foreground"
            onClick={() => setOriginalOpen(false)}
          >
            {t('reviews.hideOriginal')}
          </button>
        </div>
      )}
    </article>
  )
}
