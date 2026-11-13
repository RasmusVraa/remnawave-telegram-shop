import type { TFunction } from 'i18next'

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

/** Итог по всем серверам: ключ заголовка (allUp, somePartial…) одинаковый у /status, кабинета и поддержки. */
export type StatusLevel = 'allUp' | 'somePartial' | 'someDown' | 'allDown'

export interface StatusSummary {
  level: StatusLevel
  tone: RowState
}

/**
 * Плашка берёт худшее из строк: хоть один «Недоступен» — красная, хоть один «Частично» — оранжевая.
 * Строки, по которым ещё считаем, не учитываются.
 */
export function summarizeStatus(nodes: Pick<StatusMapNode, 'state' | 'probe' | 'no_probe'>[]): StatusSummary {
  const known = nodes.map(displayState).filter((state): state is RowState => state !== null)
  const down = known.filter((state) => state === 'down').length
  const partial = known.filter((state) => state === 'partial').length
  if (down > 0 && down === known.length) return { level: 'allDown', tone: 'down' }
  if (down > 0) return { level: 'someDown', tone: 'down' }
  if (partial > 0) return { level: 'somePartial', tone: 'partial' }
  return { level: 'allUp', tone: 'up' }
}

/** Порог «частично» по зондам России — тот же, что у сервера (STATUS_RU_FAIL_THRESHOLD). */
export const DEFAULT_RU_FAIL_THRESHOLD = 2

/**
 * Цвет колонки «Из России» по последнему замеру.
 * Обычный сервер: не ответили меньше порога — зелёный (один сбой из десяти — шум зондов),
 * ответила половина или меньше — красный, иначе оранжевый.
 * Операторы белых списков: все — зелёный, никто — красный, часть — оранжевый.
 */
export function russiaTone(ok: number, total: number, whitelist: boolean, threshold = DEFAULT_RU_FAIL_THRESHOLD): RowState {
  if (whitelist) {
    if (ok >= total) return 'up'
    return ok === 0 ? 'down' : 'partial'
  }
  if (total - ok < Math.max(1, threshold)) return 'up'
  return ok * 2 <= total ? 'down' : 'partial'
}

/** Подпись к цвету колонки «Из России» / «Белые списки». */
export function russiaLabel(t: TFunction, tone: RowState, whitelist: boolean): string {
  if (tone === 'partial') return t('landing.status.russiaPartial')
  if (tone === 'down') return whitelist ? t('landing.status.hitDown') : t('landing.status.russiaDown')
  return whitelist ? t('landing.status.whitelistUp') : t('landing.status.russiaUp')
}
