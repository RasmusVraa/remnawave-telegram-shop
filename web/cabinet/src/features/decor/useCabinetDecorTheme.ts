import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'

import { effectiveDecorTheme } from './decorSchedule'
import type { DecorThemeId } from './decorThemes'
import { useUserDecorPref } from './userDecorPref'

/**
 * Тема, которую видит этот пользователь: личный выбор (если админ его не выключил)
 * или тема сервиса из bootstrap (она уже с учётом праздничного расписания).
 */
export function useCabinetDecorTheme(): DecorThemeId {
  const { data } = useAuthBootstrap()
  const pref = useUserDecorPref()
  if (data?.user_theme_picker_enabled === true && pref !== 'auto') return pref
  return effectiveDecorTheme(data?.decor_theme)
}

export function useCabinetDecorActive(): boolean {
  return useCabinetDecorTheme() !== 'off'
}
