import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'
import { initCabinetI18n } from './i18n'
import { loadTelegramWebAppScriptIfNeeded } from '@/lib/telegram-web-app-loader'
import { configureTelegramViewport } from '@/lib/telegram-web-app'
import { preventIosInputZoom } from '@/lib/ios-input-zoom'
import { useAuthStore } from '@/store/auth'
import { isIosWebKit } from '@/lib/utils'

// Применяем тему до первого рендера (избегаем мигания).
const savedTheme = localStorage.getItem('cab_theme')
if (savedTheme === 'light') {
  document.documentElement.classList.remove('dark')
  document.documentElement.classList.add('light')
} else {
  document.documentElement.classList.add('dark')
}

// iPhone: сплошные края шапки и нижнего меню под «стеклянными» панелями
// браузера и Telegram (index.css, html[data-cabinet-ios]).
if (isIosWebKit()) document.documentElement.setAttribute('data-cabinet-ios', '')

function bootTelegramWebAppShell(): void {
  window.Telegram?.WebApp?.ready?.()
  window.Telegram?.WebApp?.expand?.()
  // Fullscreen Mini App (8.0+): mobile + desktop; отступы — CSS --tg-*-safe-area-inset-*.
  configureTelegramViewport()
}

bootTelegramWebAppShell()

// До первого рендера: слушатели должны стоять раньше, чем пользователь доберётся
// до первого поля, иначе первый же фокус успеет приблизить страницу.
preventIosInputZoom()

void initCabinetI18n().then(() => {
  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  )

  // Mini App: SDK в фоне; после загрузки — ready/expand и повтор автологина (первый initialize мог быть без initData).
  void loadTelegramWebAppScriptIfNeeded().then(() => {
    bootTelegramWebAppShell()
    void useAuthStore.getState().tryTelegramMiniAppAfterSdk()
  })
})
