import { useEffect, useSyncExternalStore } from 'react'

import { isTelegramMiniAppSession } from '@/lib/telegram-web-app-loader'

/**
 * Вибрация кабинета — как в Stars Meows.
 *
 * В Mini App — Telegram HapticFeedback (iOS и Android), в браузере — Vibration API
 * (работает только на Android; iOS Safari его не поддерживает, на ПК нет мотора —
 * вызовы там просто ничего не делают).
 *
 * Вибрируем скупо: ошибки, итог важного действия, смена выбора и нижняя панель.
 * Обычные кнопки не вибрируют — иначе через минуту вибрация перестаёт что-то значить,
 * а ошибка на этом фоне уже не ощущается «сильной».
 *
 * Пользователь может выключить вибрацию в профиле («Настройки»), выбор хранится в браузере.
 */

type ImpactStyle = 'light' | 'medium' | 'heavy'

const STORAGE_KEY = 'cab_haptics'

function readEnabled(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) !== 'off'
  } catch {
    return true
  }
}

let enabled = readEnabled()
const listeners = new Set<() => void>()

export function isHapticsEnabled(): boolean {
  return enabled
}

export function setHapticsEnabled(next: boolean): void {
  enabled = next
  try {
    localStorage.setItem(STORAGE_KEY, next ? 'on' : 'off')
  } catch {
    // приватный режим — выбор живёт до перезагрузки
  }
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/** Текущее состояние переключателя «Вибрация» для настроек. */
export function useHapticsEnabled(): boolean {
  return useSyncExternalStore(subscribe, isHapticsEnabled, isHapticsEnabled)
}

function feedback(): TelegramWebAppHapticFeedback | undefined {
  if (!isTelegramMiniAppSession()) return undefined
  return window.Telegram?.WebApp?.HapticFeedback
}

function vibrate(pattern: number | number[]): void {
  if (typeof navigator === 'undefined' || typeof navigator.vibrate !== 'function') return
  try {
    navigator.vibrate(pattern)
  } catch {
    // без жеста пользователя Chrome может отказать — не страшно
  }
}

/**
 * Telegram отдаёт HapticFeedback и на ПК (там вызов — no-op), поэтому сначала он,
 * а Vibration API — только когда кабинет открыт не в Telegram.
 */
function run(tg: (fb: TelegramWebAppHapticFeedback) => void, pattern: number | number[]): void {
  if (!enabled) return
  const fb = feedback()
  if (fb) {
    try {
      tg(fb)
    } catch {
      // старый клиент Telegram без haptic
    }
    return
  }
  vibrate(pattern)
}

/** Лёгкий толчок: нижняя панель, копирование. */
export function hapticTap(): void {
  run((fb) => fb.impactOccurred?.('light'), 12)
}

/** Толчок заданной силы: крупный выигрыш, жест шторки. */
export function hapticImpact(style: ImpactStyle): void {
  run((fb) => fb.impactOccurred?.(style), style === 'heavy' ? 30 : style === 'medium' ? 18 : 12)
}

/** Самый тонкий тик: вкладки, тариф, способ оплаты, переключатели. */
export function hapticSelection(): void {
  run((fb) => fb.selectionChanged?.(), 8)
}

/**
 * Одно событие часто сообщают двумя путями (ошибка в форме и тост, копирование и тост) —
 * второй сигнал в пределах 300 мс глотаем, иначе два «уведомления» сливаются в дребезг.
 */
const NOTIFY_DEDUP_MS = 300
let lastNotifyAt = 0

function notify(type: 'success' | 'error', pattern: number[]): void {
  const now = Date.now()
  if (now - lastNotifyAt < NOTIFY_DEDUP_MS) return
  lastNotifyAt = now
  run((fb) => fb.notificationOccurred?.(type), pattern)
}

/** Итог важного действия: оплата прошла, промокод применён, триал активирован. */
export function hapticSuccess(): void {
  notify('success', [12, 60, 18])
}

/** Ошибка — сильнее и длиннее остальных. */
export function hapticError(): void {
  notify('error', [20, 40, 20])
}

/**
 * Вибрация, когда в форме появляется ошибка. Срабатывает на смену текста:
 * формы сбрасывают ошибку перед отправкой, так что повтор той же ошибки тоже вибрирует.
 */
export function useErrorHaptic(error: unknown): void {
  useEffect(() => {
    if (error) hapticError()
  }, [error])
}
