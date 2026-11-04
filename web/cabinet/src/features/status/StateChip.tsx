import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { displayState } from './statusState'
import type { StatusMapNode } from '@/features/landing/components/StatusMap'

/**
 * Итоговый статус сервера. Рисуется только при проблеме: у работающего сервера
 * отдельной плашки нет, ему и так всё понятно. Состояние считает сервер
 * (панель + зонды + выход каскада), поэтому оно может отличаться от колонок
 * с результатами зондов: колонки показывают, что ответили зонды, плашка — итог.
 */
export function StateChip({ node, className }: { node: Pick<StatusMapNode, 'state' | 'probe' | 'no_probe'>; className?: string }) {
  const { t } = useTranslation()
  const state = displayState(node)
  if (!state || state === 'up') return null
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-[11px] font-semibold',
        state === 'down' ? 'bg-rose-500/15 text-rose-400' : 'bg-amber-500/15 text-amber-300',
        className,
      )}
    >
      {t(`landing.status.row.${state}`)}
    </span>
  )
}

/** Подсветка карточки по итоговому статусу: тонкое кольцо по скруглению карточки. */
export function stateTint(node: Pick<StatusMapNode, 'state' | 'probe' | 'no_probe'>): string {
  const state = displayState(node)
  if (state === 'down') return 'ring-1 ring-rose-500/50'
  if (state === 'partial') return 'ring-1 ring-amber-500/45'
  return ''
}

/** Подсветка строки списка: только лёгкий фон, без рамок — строки лежат внутри скруглённой таблицы. */
export function stateRowTint(node: Pick<StatusMapNode, 'state' | 'probe' | 'no_probe'>): string {
  const state = displayState(node)
  if (state === 'down') return 'bg-rose-500/[0.07]'
  if (state === 'partial') return 'bg-amber-500/[0.06]'
  return ''
}
