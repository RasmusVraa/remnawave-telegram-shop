import { useEffect } from 'react'

import { useCabinetDecorTheme } from './useCabinetDecorTheme'
import { useDecorCardSpotlight } from './useDecorCardSpotlight'

/** Синхронизирует data-cabinet-decor на <html> для CSS-тем (фон, neon и т.д.). */
export function CabinetDecorThemeSync() {
  const theme = useCabinetDecorTheme()

  // Подсветка карточек под курсором — нужна только теме nebula, хук сам это проверяет.
  useDecorCardSpotlight()

  useEffect(() => {
    const root = document.documentElement
    if (theme === 'off') {
      delete root.dataset.cabinetDecor
    } else {
      root.dataset.cabinetDecor = theme
    }
    return () => {
      delete root.dataset.cabinetDecor
    }
  }, [theme])

  return null
}
