import { useEffect, useState } from 'react'

type Theme = 'dark' | 'light'

function readSavedTheme(): Theme {
  return localStorage.getItem('cab_theme') === 'light' ? 'light' : 'dark'
}

const getInitialTheme = readSavedTheme

/** Только классы на <html>, без сохранения выбора (принудительная тёмная у праздничных тем). */
function setThemeClass(theme: Theme) {
  const root = document.documentElement
  root.classList.toggle('dark', theme === 'dark')
  root.classList.toggle('light', theme === 'light')
}

function applyTheme(theme: Theme) {
  setThemeClass(theme)
  localStorage.setItem('cab_theme', theme)
}

export { applyTheme, readSavedTheme, setThemeClass }

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(getInitialTheme)

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  const toggle = () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'))
  const set = (t: Theme) => setTheme(t)

  return { theme, toggle, set }
}
