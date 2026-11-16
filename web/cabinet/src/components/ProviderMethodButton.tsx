import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

export function ProviderMethodButton({
  selected,
  onClick,
  icon,
  label,
  description,
  trailing,
  badge,
}: {
  selected: boolean
  onClick: () => void
  icon: ReactNode
  label: string
  description: string
  /** Например ChevronDown у родителя Platega */
  trailing?: ReactNode
  /** Плашка справа перед радио-точкой: «Рекомендуем». */
  badge?: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      role="radio"
      aria-checked={selected}
      className={cn(
        'flex w-full items-center gap-3 rounded-[16px] border px-3 py-2.5 text-left transition-all duration-150',
        selected
          ? 'border-primary/60 bg-primary/[0.07] dark:bg-primary/[0.1]'
          : // Своя подложка: без неё кнопка сливалась с карточкой и держалась на
            // одной рамке. В тёмной теме кнопка утоплена — затемнение поверх
            // карточки, а не осветление: оно светилось ярче самой карточки.
            'border-border bg-foreground/[0.03] hover:border-primary/40 hover:bg-foreground/[0.06] dark:bg-black/20 dark:hover:bg-black/30',
      )}
    >
      {/* Плашка иконки светлее самой кнопки: на утопленном фоне bg-muted почти совпал бы с ним. */}
      <div className="flex h-[38px] w-[38px] shrink-0 items-center justify-center rounded-full bg-muted dark:bg-white/[0.07]">
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold">{label}</p>
        <p className="text-[13px] text-muted-foreground">{description}</p>
      </div>
      {badge}
      {trailing != null ? <div className="flex shrink-0 items-center">{trailing}</div> : <RadioDot selected={selected} />}
    </button>
  )
}

/** Радио-точка справа: у выбранного способа заполнена цветом темы, у остальных пустое кольцо. */
function RadioDot({ selected }: { selected: boolean }) {
  return (
    <span
      aria-hidden
      className={cn(
        'flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded-full border-2 transition-colors',
        selected ? 'border-primary' : 'border-muted-foreground/40',
      )}
    >
      {selected && <span className="h-2.5 w-2.5 rounded-full bg-primary" />}
    </span>
  )
}
