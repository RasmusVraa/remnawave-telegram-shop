import { Star } from 'lucide-react'

import { cn } from '@/lib/utils'

/** Заполнение звезды 0..1 для половинок. */
function fillForStar(rating: number, index: number): number {
  const left = rating - index
  if (left >= 1) return 1
  if (left <= 0) return 0
  return left >= 0.5 ? 0.5 : 0
}

export function ReviewStars({
  rating,
  size = 'md',
  className,
}: {
  rating: number
  size?: 'sm' | 'md' | 'lg'
  className?: string
}) {
  const box = size === 'sm' ? 'size-3.5' : size === 'lg' ? 'size-7' : 'size-4'
  return (
    <span className={cn('inline-flex items-center gap-0.5', className)} aria-label={`${rating}/5`}>
      {Array.from({ length: 5 }, (_, i) => {
        const fill = fillForStar(rating, i)
        return (
          <span key={i} className={cn('relative inline-flex', box)}>
            <Star className={cn(box, 'text-muted-foreground/35')} strokeWidth={1.75} />
            {fill > 0 && (
              <span
                className="absolute inset-0 overflow-hidden"
                style={{ width: fill === 1 ? '100%' : '50%' }}
              >
                <Star className={cn(box, 'fill-primary text-primary')} strokeWidth={1.75} />
              </span>
            )}
          </span>
        )
      })}
    </span>
  )
}

/**
 * Выбор оценки: 5 пустых звёзд, клик по левой/правой половине — шаг 0.5.
 */
export function StarRatingInput({
  value,
  onChange,
  className,
}: {
  value: number
  onChange: (rating: number) => void
  className?: string
}) {
  return (
    <div className={cn('inline-flex items-center gap-1', className)} role="radiogroup" aria-label="rating">
      {Array.from({ length: 5 }, (_, i) => {
        const starValue = i + 1
        const fill = fillForStar(value, i)
        return (
          <span key={i} className="relative inline-flex size-9 sm:size-10">
            <Star
              className="size-9 text-muted-foreground/35 sm:size-10"
              strokeWidth={1.5}
              aria-hidden
            />
            {fill > 0 && (
              <span
                className="pointer-events-none absolute inset-0 overflow-hidden"
                style={{ width: fill === 1 ? '100%' : '50%' }}
              >
                <Star className="size-9 fill-primary text-primary sm:size-10" strokeWidth={1.5} />
              </span>
            )}
            <button
              type="button"
              aria-label={`${starValue - 0.5}`}
              className="absolute inset-y-0 left-0 z-10 w-1/2 rounded-l-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => onChange(starValue - 0.5)}
            />
            <button
              type="button"
              aria-label={`${starValue}`}
              className="absolute inset-y-0 right-0 z-10 w-1/2 rounded-r-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => onChange(starValue)}
            />
          </span>
        )
      })}
    </div>
  )
}
