import type { StatusMapNode } from '@/features/landing/components/StatusMap'

export type RowState = 'up' | 'partial' | 'down'

/**
 * Что писать в колонке «Из мира». Состояние считает сервер (панель + зонды + выход каскада),
 * здесь оно только показывается. Пока замера нет и строку меряют внешние зонды — null («Ещё считаем»).
 */
export function displayState(node: Pick<StatusMapNode, 'state' | 'probe' | 'no_probe'>): RowState | null {
  // Замера ещё нет, но панель уже сказала «не работает» — это известно и без зондов.
  if (!node.probe && !node.no_probe) return node.state === 'down' ? 'down' : null
  return node.state === 'up' || node.state === 'partial' || node.state === 'down' ? node.state : null
}
