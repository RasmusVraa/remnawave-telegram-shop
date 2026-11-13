import type { ReactNode } from 'react'
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react'

import { cn } from '@/lib/utils'

export type SortDir = 'asc' | 'desc'

export interface SortState {
  key: string
  dir: SortDir
}

/**
 * Следующее состояние сортировки по клику на заголовок: новая колонка
 * включается в `firstDir`, повторный клик разворачивает, третий — снимает
 * сортировку и возвращает порядок по умолчанию.
 */
export function nextSort(current: SortState | null, key: string, firstDir: SortDir): SortState | null {
  if (!current || current.key !== key) return { key, dir: firstDir }
  if (current.dir === firstDir) return { key, dir: firstDir === 'asc' ? 'desc' : 'asc' }
  return null
}

/**
 * Заголовок колонки с сортировкой. Сортирует сервер, а не браузер: таблица
 * постраничная, и сортировка одной страницы вводила бы в заблуждение.
 */
export function SortableTh({
  sortKey,
  sort,
  onSort,
  children,
  className,
  disabled,
  filter,
}: {
  sortKey: string
  sort: SortState | null
  onSort: (key: string) => void
  children: ReactNode
  className?: string
  /** Сортировка недоступна (например, результаты поиска) — заголовок остаётся обычным. */
  disabled?: boolean
  /** Фильтр колонки (стрелка со списком) — рядом с кнопкой сортировки. */
  filter?: ReactNode
}) {
  const active = !disabled && sort?.key === sortKey
  const Icon = !active ? ArrowUpDown : sort?.dir === 'asc' ? ArrowUp : ArrowDown

  return (
    <th
      className={cn('px-3 py-2', className)}
      aria-sort={active ? (sort?.dir === 'asc' ? 'ascending' : 'descending') : undefined}
    >
      {disabled ? (
        children
      ) : (
        <span className="inline-flex items-center gap-1">
        <button
          type="button"
          onClick={() => onSort(sortKey)}
          className={cn(
            '-mx-1.5 inline-flex items-center gap-1 whitespace-nowrap rounded px-1.5 py-0.5 uppercase tracking-wider transition-colors hover:bg-accent/15 hover:text-foreground',
            active && 'text-foreground',
          )}
        >
          {children}
          <Icon className={cn('size-3.5 shrink-0', active ? 'text-primary' : 'text-muted-foreground/50')} aria-hidden />
        </button>
        {filter}
        </span>
      )}
    </th>
  )
}
