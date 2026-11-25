import { useSyncExternalStore } from 'react'

import { DECOR_THEMES, type DecorThemeId } from './decorThemes'

/**
 * Личный выбор оформления пользователя («Профиль → Настройки → Оформление»).
 *
 * Хранится в браузере, как вибрация и тема: на другом устройстве выбор свой.
 * `auto` (или пусто) — «как у сервиса»: тема админа и праздничное расписание.
 * Конкретная тема, включая `off`, сильнее расписания, пока админ не выключил выбор.
 */
export type UserDecorPref = 'auto' | DecorThemeId

const STORAGE_KEY = 'cab_decor_pref'

function readPref(): UserDecorPref {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v && v in DECOR_THEMES) return v as DecorThemeId
  } catch {
    // нет доступа к хранилищу — работаем как «auto»
  }
  return 'auto'
}

let pref = readPref()
const listeners = new Set<() => void>()

export function setUserDecorPref(next: UserDecorPref): void {
  pref = next
  try {
    if (next === 'auto') localStorage.removeItem(STORAGE_KEY)
    else localStorage.setItem(STORAGE_KEY, next)
  } catch {
    // приватный режим — выбор живёт до перезагрузки
  }
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  // Выбор, сделанный в другой вкладке.
  const onStorage = (e: StorageEvent) => {
    if (e.key !== STORAGE_KEY) return
    pref = readPref()
    listener()
  }
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(listener)
    window.removeEventListener('storage', onStorage)
  }
}

export function useUserDecorPref(): UserDecorPref {
  return useSyncExternalStore(subscribe, () => pref, () => 'auto' as UserDecorPref)
}
