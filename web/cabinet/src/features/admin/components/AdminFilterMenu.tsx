import type { ReactNode } from 'react'
import { Check } from 'lucide-react'

import { cn } from '@/lib/utils'

/**
 * Пункты выпадающего меню «Сортировка и фильтр» в админке (страницы «Ноды»,
 * «Промокоды»). Одно место, чтобы меню на разных страницах выглядели одинаково.
 */

/** Заголовок раздела меню: «Сортировка», «Показать», «Тип». */
export function AdminFilterMenuTitle({ children }: { children: ReactNode }) {
  return (
    <p className="px-2.5 pb-1 pt-2 text-[11px] font-bold uppercase tracking-wider text-muted-foreground">{children}</p>
  )
}

/** Пункт-радио: галочка у выбранного, справа — необязательный счётчик. */
export function AdminFilterMenuItem({
  active,
  count,
  onClick,
  children,
}: {
  active: boolean
  count?: number
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      role="menuitemradio"
      aria-checked={active}
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-2.5 rounded-[10px] px-2.5 py-2 text-left text-sm font-medium transition-colors',
        active ? 'bg-primary/10 text-primary' : 'text-foreground/90 hover:bg-secondary',
      )}
    >
      <span className="flex size-4 shrink-0 items-center justify-center">
        {active && <Check className="size-3.5" aria-hidden />}
      </span>
      <span className="flex-1">{children}</span>
      {count != null && <span className="font-mono text-xs text-muted-foreground">{count}</span>}
    </button>
  )
}

/** Разделитель между разделами меню. */
export function AdminFilterMenuDivider() {
  return <div className="my-1.5 border-t border-border/60" />
}
