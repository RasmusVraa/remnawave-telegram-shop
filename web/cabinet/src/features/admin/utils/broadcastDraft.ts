import type { AdminBroadcastMediaKind } from '@/lib/types/admin'

import type { BroadcastButtonsState } from '../components/BroadcastButtonsPicker'
import { BROADCAST_LINK_KEYS, type BroadcastLinkKey } from './broadcastLinks'

/**
 * Черновик рассылки в браузере админа.
 *
 * Текст на тысячу знаков пропадал от случайного обновления страницы или
 * закрытой вкладки. Хранится в localStorage, а не на сервере: черновик —
 * незаконченная работа на этом рабочем месте, делиться им не с кем.
 *
 * Вложение хранится идентификатором файла в Telegram: он постоянный, и
 * рассылка уходит с ним и после перезагрузки. Не переживает перезагрузку
 * только картинка предпросмотра — она жила в памяти вкладки.
 */
export interface BroadcastDraft {
  html: string
  audiences: string[]
  tariffId: number | null
  buttons: BroadcastButtonsState
  media: { file_id: string; kind: AdminBroadcastMediaKind; name: string } | null
}

const STORAGE_KEY = 'admin-broadcast-draft-v1'
const MEDIA_KINDS: AdminBroadcastMediaKind[] = ['photo', 'video', 'document']

/**
 * Кнопки из сохранённого вида — черновика или записи истории. Раздел могли
 * убрать из кабинета, пока запись лежала: неизвестный ключ бэкенд отклонит, и
 * рассылка не уйдёт вовсе, поэтому такие ключи выкидываются.
 */
export function normalizeBroadcastButtons(raw: unknown): BroadcastButtonsState {
  const buttons = (raw && typeof raw === 'object' ? raw : {}) as Partial<BroadcastButtonsState>
  return {
    buy: buttons.buy === true,
    connect: buttons.connect === true,
    promo: buttons.promo === true,
    main_menu: buttons.main_menu === true,
    share_referral: buttons.share_referral === true,
    links: Array.isArray(buttons.links)
      ? buttons.links.filter((key): key is BroadcastLinkKey =>
          BROADCAST_LINK_KEYS.includes(key as BroadcastLinkKey),
        )
      : [],
  }
}

function isDraftWorthKeeping(draft: BroadcastDraft): boolean {
  // Одни отмеченные аудитории или кнопки — ещё не работа, которую жалко потерять.
  return draft.html.trim() !== '' || draft.media !== null
}

export function loadBroadcastDraft(): BroadcastDraft | null {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw === null) return null
    const parsed = JSON.parse(raw) as Partial<BroadcastDraft> | null
    if (!parsed || typeof parsed !== 'object') return null

    const media = parsed.media
    const draft: BroadcastDraft = {
      html: typeof parsed.html === 'string' ? parsed.html : '',
      audiences: Array.isArray(parsed.audiences)
        ? parsed.audiences.filter((a): a is string => typeof a === 'string')
        : [],
      tariffId: typeof parsed.tariffId === 'number' ? parsed.tariffId : null,
      buttons: normalizeBroadcastButtons(parsed.buttons),
      media:
        media &&
        typeof media.file_id === 'string' &&
        media.file_id !== '' &&
        MEDIA_KINDS.includes(media.kind)
          ? { file_id: media.file_id, kind: media.kind, name: String(media.name ?? '') }
          : null,
    }
    return isDraftWorthKeeping(draft) ? draft : null
  } catch {
    // Испорченная запись или заблокированное хранилище — начинаем с чистой формы.
    return null
  }
}

/** Сохраняет черновик; пустой стирает. Возвращает, удалось ли записать. */
export function saveBroadcastDraft(draft: BroadcastDraft): boolean {
  try {
    if (isDraftWorthKeeping(draft)) {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(draft))
    } else {
      window.localStorage.removeItem(STORAGE_KEY)
    }
    return true
  } catch {
    // Приватный режим или переполненное хранилище: форма работает и без памяти.
    return false
  }
}

export function clearBroadcastDraft() {
  try {
    window.localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Нечего стирать — нечего и чинить.
  }
}
