import { Star } from 'lucide-react'

import { cn } from '@/lib/utils'

const STARS = [1, 2, 3, 4, 5] as const

/** Звёзды только для показа: целые, закрашенные до value. */
export function Stars({ value, className, size = 'size-4' }: { value: number; className?: string; size?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-0.5', className)} role="img" aria-label={`${value} / 5`}>
      {STARS.map((n) => (
        <Star
          key={n}
          className={cn(size, n <= value ? 'fill-amber-400 text-amber-400' : 'fill-muted/40 text-muted-foreground/30')}
          aria-hidden
        />
      ))}
    </span>
  )
}

/** Выбор оценки: пять целых звёзд, клавиатура и тап. */
export function StarPicker({
  value,
  onChange,
  disabled,
  label,
}: {
  value: number
  onChange: (v: number) => void
  disabled?: boolean
  label: string
}) {
  return (
    <div role="radiogroup" aria-label={label} className="flex items-center gap-1.5">
      {STARS.map((n) => (
        <button
          key={n}
          type="button"
          role="radio"
          aria-checked={value === n}
          aria-label={`${n}`}
          disabled={disabled}
          onClick={() => onChange(n)}
          className="rounded-md p-0.5 transition-transform hover:scale-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:scale-95 disabled:opacity-60"
        >
          <Star
            className={cn(
              'size-9 transition-colors sm:size-10',
              n <= value ? 'fill-amber-400 text-amber-400' : 'fill-muted/30 text-muted-foreground/40',
            )}
            aria-hidden
          />
        </button>
      ))}
    </div>
  )
}
