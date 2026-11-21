import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Reply, UserRound } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Stars } from './StarRating'
import type { ReviewItem } from './reviewTypes'

interface ReviewCardProps {
  review: Pick<
    ReviewItem,
    'rating' | 'body' | 'author_name' | 'author_avatar_url' | 'client_since' | 'tariff_label' | 'is_trial' | 'created_at' | 'reply'
  >
  /** landing — карточка тёмного лендинга, cabinet — карточка кабинета. */
  variant?: 'cabinet' | 'landing'
  className?: string
}

/** «2026-10-02» → «02.10.26»; старые записи с месяцем без дня («2026-08») показываем как «08.26». */
function clientSinceLabel(value: string): string {
  const m = /^(\d{4})-(\d{2})(?:-(\d{2}))?$/.exec(value)
  if (!m) return value
  const [, y, mo, d] = m
  return d ? `${d}.${mo}.${y.slice(2)}` : `${mo}.${y.slice(2)}`
}

/** Дата отзыва и ответа: «21.11.26» — коротко и одинаково на любом языке. */
function dayLabel(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${pad(d.getFullYear() % 100)}`
}

function initial(name: string): string {
  return name.trim().charAt(0).toUpperCase() || '?'
}

export function ReviewCard({ review, variant = 'cabinet', className }: ReviewCardProps) {
  const { t } = useTranslation()
  const [imgFailed, setImgFailed] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const name = review.author_name?.trim() || ''
  const showImg = Boolean(review.author_avatar_url) && !imgFailed
  const tariff = review.is_trial ? t('reviews.trial') : review.tariff_label
  const meta = [
    review.client_since ? t('reviews.clientSince', { date: clientSinceLabel(review.client_since) }) : '',
    tariff,
  ]
    .filter(Boolean)
    .join(' · ')
  const landing = variant === 'landing'
  // Длинный текст не растягивает карточку на пол-экрана: больше шести строк сворачиваем.
  const isLong = review.body.length > 280 || review.body.split('\n').length > 6

  return (
    <article
      className={cn(
        'flex h-full flex-col gap-3',
        landing ? 'landing-card p-5 sm:p-6' : 'cabinet-elevated-card rounded-[var(--radius)] p-4 sm:p-5',
        className,
      )}
    >
      <div className="flex items-center gap-3">
        {showImg ? (
          <img
            src={review.author_avatar_url ?? undefined}
            alt=""
            loading="lazy"
            // Картинка не загрузилась (удалили фото, закрыли приватностью) — буква вместо сломанной иконки.
            onError={() => setImgFailed(true)}
            className="size-11 shrink-0 rounded-full object-cover ring-1 ring-border/80"
          />
        ) : (
          <span
            className={cn(
              'flex size-11 shrink-0 items-center justify-center rounded-full text-base font-bold ring-1 ring-border/80',
              name ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground',
            )}
          >
            {name ? initial(name) : <UserRound className="size-5" strokeWidth={1.75} aria-hidden />}
          </span>
        )}
        <div className="min-w-0 flex-1">
          <p className="truncate font-heading text-[15px] font-bold tracking-tight text-foreground">
            {name || t('reviews.anonymous')}
          </p>
          {meta && <p className="mt-0.5 truncate text-xs text-muted-foreground">{meta}</p>}
        </div>
      </div>

      <Stars value={review.rating} />
      <div>
        <p
          className={cn(
            'whitespace-pre-line break-words text-[14.5px] leading-relaxed text-foreground/90',
            !expanded && isLong && 'line-clamp-6',
          )}
        >
          {review.body}
        </p>
        {isLong && (
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            className="mt-1.5 text-sm font-medium text-primary hover:underline"
          >
            {expanded ? t('reviews.showLess') : t('reviews.showMore')}
          </button>
        )}
      </div>

      {review.reply && (
        <div className="rounded-r-xl rounded-l-sm border-l-[3px] border-primary bg-primary/[0.07] px-3.5 py-3">
          <div className="mb-1 flex items-center gap-2 text-xs font-bold text-primary">
            <Reply className="size-3.5" aria-hidden />
            {t('reviews.replyTitle')}
            <time className="ml-auto font-normal text-muted-foreground">{dayLabel(review.reply.at)}</time>
          </div>
          <p className="whitespace-pre-line break-words text-sm leading-relaxed text-foreground/85">{review.reply.body}</p>
        </div>
      )}

      <p className="mt-auto pt-1 text-right text-xs text-muted-foreground">{t('reviews.publishedOn', { date: dayLabel(review.created_at) })}</p>
    </article>
  )
}
