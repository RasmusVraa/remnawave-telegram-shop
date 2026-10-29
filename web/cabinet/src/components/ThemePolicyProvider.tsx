import { createContext, useContext, useEffect, type ReactNode } from 'react'

import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { applyTheme, readSavedTheme, setThemeClass } from '@/hooks/useTheme'
import { effectiveDecorTheme } from '@/features/decor/decorSchedule'
import { isDarkOnlyDecorTheme } from '@/features/decor/decorThemes'

const LightThemeAllowedContext = createContext(true)

export function ThemePolicyProvider({ children }: { children: ReactNode }) {
  const { data } = useAuthBootstrap()
  const lightDisabledByAdmin = data?.light_theme_enabled === false
  // Сцены halloween_2 / new_year_2 рисованы только тёмными.
  const darkOnlyDecor = isDarkOnlyDecorTheme(effectiveDecorTheme(data?.decor_theme))
  const lightAllowed = !lightDisabledByAdmin && !darkOnlyDecor

  useEffect(() => {
    if (lightDisabledByAdmin) {
      applyTheme('dark')
      return
    }
    if (!darkOnlyDecor) return
    // Тема праздника временная: выбор пользователя в localStorage не трогаем
    // и возвращаем его, когда тема сменится.
    setThemeClass('dark')
    return () => setThemeClass(readSavedTheme())
  }, [lightDisabledByAdmin, darkOnlyDecor])

  return (
    <LightThemeAllowedContext.Provider value={lightAllowed}>
      {children}
    </LightThemeAllowedContext.Provider>
  )
}

export function useLightThemeAllowed(): boolean {
  return useContext(LightThemeAllowedContext)
}
