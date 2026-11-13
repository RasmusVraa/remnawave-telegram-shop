import { isMobileUserAgent } from '@/lib/utils'

/** Нативные десктоп-клиенты Telegram. */
const NATIVE_DESKTOP_PLATFORMS = new Set(['tdesktop', 'macos'])
/** Telegram Web — platform одинаковый на PC и телефоне, нужен доп. фильтр по UA. */
const WEB_PLATFORMS = new Set(['weba', 'webk', 'web'])

function webApp(): TelegramWebApp | undefined {
  return window.Telegram?.WebApp
}

/** Desktop Mini App (окно с ✕ в углу) — без большого top safe-area. */
function isTelegramDesktopPlatform(): boolean {
  const p = webApp()?.platform
  if (typeof p !== 'string') return false
  if (NATIVE_DESKTOP_PLATFORMS.has(p)) return true
  if (WEB_PLATFORMS.has(p) && !isMobileUserAgent()) return true
  return false
}

function syncTelegramChromeColors(tg: TelegramWebApp): void {
  const isDark = document.documentElement.classList.contains('dark')
  const color = isDark ? '#0b1426' : '#f8fafc'
  try {
    tg.setHeaderColor?.(color)
    tg.setBackgroundColor?.(color)
  } catch {
    // ignore
  }
}

/**
 * Bot API 7.7+: гасим жест «свайп вниз = свернуть Mini App».
 * У кабинета собственный вертикальный скролл; с включённым жестом webview
 * перехватывает часть тачей и дрожит у верха страницы. Закрытие остаётся по ✕.
 */
function disableTelegramVerticalSwipes(tg: TelegramWebApp): void {
  if (typeof tg.isVersionAtLeast === 'function' && !tg.isVersionAtLeast('7.7')) return
  try {
    tg.disableVerticalSwipes?.()
  } catch {
    // UNSUPPORTED
  }
}

/**
 * Включает CSS top safe-area только на мобильном Mini App.
 * На desktop TG отдаёт большой contentSafeAreaInset.top → «дыра» над логотипом.
 */
function syncCabinetTgSafeTopAttr(): void {
  const root = document.documentElement
  const tg = webApp()
  if (!tg) {
    root.removeAttribute('data-cabinet-tg-safe-top')
    return
  }
  if (isTelegramDesktopPlatform()) {
    root.setAttribute('data-cabinet-tg-safe-top', '0')
    return
  }
  root.setAttribute('data-cabinet-tg-safe-top', '1')
}

/**
 * Fullscreen Mini App (Bot API 8.0+): mobile и desktop.
 * Top insets (--cabinet-tg-safe-top) — только мобилка; desktop без лишнего padding.
 */
export function configureTelegramViewport(): void {
  const tg = webApp()
  if (!tg) {
    syncCabinetTgSafeTopAttr()
    return
  }

  syncTelegramChromeColors(tg)
  syncCabinetTgSafeTopAttr()
  disableTelegramVerticalSwipes(tg)

  if (tg.isFullscreen) return
  if (typeof tg.isVersionAtLeast === 'function' && !tg.isVersionAtLeast('8.0')) return
  try {
    tg.requestFullscreen?.()
  } catch {
    // UNSUPPORTED
  }
}
