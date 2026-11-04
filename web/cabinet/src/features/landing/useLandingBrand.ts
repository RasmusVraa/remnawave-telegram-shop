import { useMemo } from 'react'

import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useAuthStore } from '@/store/auth'

/**
 * Бренд и внешние ссылки для лендинга.
 *
 * Всё берётся из публичного GET /cabinet/api/auth/bootstrap (без авторизации):
 * CABINET_BRAND_NAME, логотип и site_links из env бота (BOT_URL, SUPPORT_URL, …).
 * Если API недоступен (локальный dev без бэкенда) — useAuthBootstrap отдаёт
 * фолбэк, и лендинг просто рендерится с дефолтным названием и без внешних ссылок.
 */

const DEFAULT_BRAND = 'Cabinet'

/**
 * Кабинет всегда смонтирован на /cabinet (vite base + mux.Handle в router.go).
 * Лендинг открывается и с /landing, и с /cabinet/landing — с разным basename
 * у router'а, поэтому ссылки «в кабинет» держим абсолютными, а не через <Link>.
 */
const CABINET_BASE = '/cabinet'

export interface LandingBrand {
  name: string
  logoUrl?: string
  /** Ссылка на Telegram-бота проекта; null — кнопку «Открыть в Telegram» не показываем. */
  botUrl: string | null
  supportUrl: string | null
  channelUrl: string | null
  tosUrl: string | null
  privacyUrl: string | null
  offerUrl: string | null
  statusUrl: string | null
  /** false — в списке статуса нет серверов, ссылку на /status не показываем. */
  statusVisible: boolean
  /** Абсолютный URL кнопки «Личный кабинет»: дашборд, если сессия уже есть. */
  cabinetHref: string
  /** Абсолютный URL витрины тарифов в кабинете. */
  tariffsHref: string
  /** true — пользователь уже авторизован (меняем подпись кнопки). */
  authenticated: boolean
}

function pick(links: Record<string, string> | undefined, key: string): string | null {
  const v = links?.[key]?.trim()
  return v ? v : null
}

export function useLandingBrand(): LandingBrand {
  const { data } = useAuthBootstrap()
  const accessToken = useAuthStore((s) => s.accessToken)

  return useMemo(() => {
    const links = data?.site_links
    const widgetBot = data?.telegram_widget_bot?.trim()

    // BOT_URL приоритетнее; если его нет — собираем ссылку из юзернейма
    // login-виджета, он указывает на того же бота.
    const botUrl =
      pick(links, 'bot') ?? (widgetBot ? `https://t.me/${widgetBot.replace(/^@/, '')}` : null)

    return {
      name: (data?.brand_name?.trim() || DEFAULT_BRAND).trim() || DEFAULT_BRAND,
      logoUrl: data?.brand_logo_url?.trim() || undefined,
      botUrl,
      supportUrl: pick(links, 'support'),
      channelUrl: pick(links, 'channel'),
      tosUrl: pick(links, 'terms_of_service') ?? pick(links, 'tos'),
      privacyUrl: pick(links, 'privacy_policy'),
      offerUrl: pick(links, 'public_offer'),
      statusUrl: pick(links, 'server_status'),
      statusVisible: data?.status_nav_visible !== false,
      cabinetHref: `${CABINET_BASE}${accessToken ? '/dashboard' : '/login'}`,
      tariffsHref: `${CABINET_BASE}${accessToken ? '/tariffs' : '/login'}`,
      authenticated: Boolean(accessToken),
    }
  }, [data, accessToken])
}
