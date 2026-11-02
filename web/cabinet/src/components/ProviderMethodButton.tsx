import type { ReactNode } from 'react'
import { Check } from 'lucide-react'

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
  /** Плашка справа перед галочкой: «в прошлый раз». */
  badge?: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'w-full flex items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
        selected
          ? 'border-primary bg-primary/10 shadow-sm dark:bg-primary/[0.18]'
          : // Своя подложка: без неё кнопка сливалась с карточкой и держалась на
            // одной рамке. В тёмной теме кнопка утоплена — затемнение поверх
            // карточки, а не осветление: оно светилось ярче самой карточки.
            'border-border bg-foreground/[0.03] hover:border-primary/40 hover:bg-foreground/[0.06] dark:bg-black/20 dark:hover:bg-black/30',
      )}
    >
      {/* Плашка иконки светлее самой кнопки: на утопленном фоне bg-muted почти совпал бы с ним. */}
      <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-muted dark:bg-white/[0.07]">{icon}</div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      {badge}
      {trailing != null ? (
        <div className="flex shrink-0 items-center">{trailing}</div>
      ) : (
        selected && (
          <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary">
            <Check size={12} className="text-white" />
          </div>
        )
      )}
    </button>
  )
}
