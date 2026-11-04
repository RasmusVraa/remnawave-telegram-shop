import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, type AuthTokenResponse } from '@/lib/api'
import { getTelegramInitData, getTelegramMiniAppStartParam } from '@/lib/utils'
import { mountTelegramLoginWidgetScript } from '@/lib/telegram-widget-mount'
import { GoogleBrandIcon, TelegramBrandIcon, VKBrandIcon, YandexBrandIcon } from '@/components/BrandIcons'
import type { TelegramWidgetUser } from './TelegramLoginWidget'

export type { TelegramWidgetUser } from './TelegramLoginWidget'

export type OAuthFlags = {
  google: boolean
  yandex?: boolean
  vk?: boolean
  telegramBot?: string
  telegramOIDCEnabled?: boolean
  telegramWebAuthMode?: 'widget' | 'oidc'
}

/** До ответа bootstrap — не скрываем блок «или», чтобы не было «пустой» карточки при медленном API. */
const OAUTH_BEFORE_BOOTSTRAP: OAuthFlags = {
  google: true,
  yandex: false,
  vk: false,
  telegramBot: undefined,
  telegramOIDCEnabled: false,
  telegramWebAuthMode: 'widget',
}

const BOOTSTRAP_TIMEOUT_MS = 10_000

type Page = 'login' | 'register'

type Props = {
  page: Page
  /** ref из ?ref= (реферальная регистрация); уходит в Google OAuth state и в POST /auth/telegram. */
  referralCode?: string
  /** true — виджет уже отрисован выше (widget mode); не монтировать второй раз в сетке. */
  telegramWidgetRenderedAbove?: boolean
  /** Для legacy Telegram Login Widget 1.0. */
  onTelegramAuth?: (user: TelegramWidgetUser) => Promise<void>
  /** Mini App: после успешного POST /auth/telegram с init_data. */
  onTelegramMiniAppSuccess: (data: AuthTokenResponse) => Promise<void>
  /** Ошибка входа через Telegram (виджет или Mini App). */
  onTelegramFlowError: (err: unknown) => void
}

/**
 * Блок «или» + Google + Telegram Login Widget для /login и /register.
 * Bootstrap с бэка; глобальный колбэк виджета разный для страниц, чтобы не затирать друг друга при навигации.
 */
export function AuthSocialProviders({
  page,
  referralCode,
  telegramWidgetRenderedAbove = false,
  onTelegramAuth,
  onTelegramMiniAppSuccess,
  onTelegramFlowError,
}: Props) {
  const { t } = useTranslation()
  const [oauth, setOauth] = useState<OAuthFlags>(OAUTH_BEFORE_BOOTSTRAP)
  const [tgWidgetError, setTgWidgetError] = useState(false)
  const [tgWidgetLoading, setTgWidgetLoading] = useState(true)
  const [miniLoading, setMiniLoading] = useState(false)
  /** Нажатая кнопка OAuth: страница уходит к провайдеру, до ухода крутится спиннер. */
  const [redirecting, setRedirecting] = useState<string | null>(null)
  /** Виджет вернул пользователя — ждём вход на сервере. */
  const [tgAuthPending, setTgAuthPending] = useState(false)
  const tgMountRef = useRef<HTMLDivElement>(null)
  const onTelegramAuthRef = useRef(onTelegramAuth)
  onTelegramAuthRef.current = onTelegramAuth
  const miniOkRef = useRef(onTelegramMiniAppSuccess)
  miniOkRef.current = onTelegramMiniAppSuccess
  const flowErrRef = useRef(onTelegramFlowError)
  flowErrRef.current = onTelegramFlowError

  const initData = typeof window !== 'undefined' ? getTelegramInitData() : ''
  const inMiniApp = initData.length > 0

  const bot = oauth.telegramBot
  const oidcEnabled = oauth.telegramOIDCEnabled ?? false
  const showWidget = !!bot && !inMiniApp
  const embedTelegramWidget =
    showWidget && !telegramWidgetRenderedAbove && oauth.telegramWebAuthMode === 'widget'
  const showOIDC = oidcEnabled && !inMiniApp
  const showSocial =
    inMiniApp || !!oauth.google || !!oauth.yandex || !!oauth.vk || showWidget || showOIDC
  const socialButtons: Array<{ key: string; label: string; icon: ReactNode; onClick: () => void; loading?: boolean }> = []

  if (oauth.google) {
    socialButtons.push({
      key: 'google',
      label: t('auth.socialGoogle'),
      icon: <GoogleBrandIcon className="size-5" />,
      loading: redirecting === 'google',
      onClick: () => {
        setRedirecting('google')
        const ref = referralCode?.trim()
        const start = '/cabinet/api/auth/google/start'
        window.location.href = ref ? `${start}?ref=${encodeURIComponent(ref)}` : start
      },
    })
  }
  if (oauth.yandex) {
    socialButtons.push({
      key: 'yandex',
      label: t('auth.socialYandex'),
      icon: <YandexBrandIcon className="size-5" />,
      loading: redirecting === 'yandex',
      onClick: () => {
        setRedirecting('yandex')
        const ref = referralCode?.trim()
        const start = '/cabinet/api/auth/yandex/start'
        window.location.href = ref ? `${start}?ref=${encodeURIComponent(ref)}` : start
      },
    })
  }
  if (oauth.vk) {
    socialButtons.push({
      key: 'vk',
      label: t('auth.socialVK'),
      icon: <VKBrandIcon className="size-5" />,
      loading: redirecting === 'vk',
      onClick: () => {
        setRedirecting('vk')
        const ref = referralCode?.trim()
        const start = '/cabinet/api/auth/vk/start'
        window.location.href = ref ? `${start}?ref=${encodeURIComponent(ref)}` : start
      },
    })
  }
  if (inMiniApp) {
    socialButtons.push({
      key: 'telegram-miniapp',
      label: t('auth.socialTelegram'),
      icon: <TelegramBrandIcon className="size-5" />,
      loading: miniLoading,
      onClick: async () => {
        setMiniLoading(true)
        try {
          const ref = referralCode?.trim() || getTelegramMiniAppStartParam() || undefined
          const data = await api.telegramAuthMiniApp(getTelegramInitData(), ref)
          await miniOkRef.current(data)
        } catch (e) {
          flowErrRef.current(e)
        } finally {
          setMiniLoading(false)
        }
      },
    })
  }
  if (showOIDC) {
    socialButtons.push({
      key: 'telegram-oidc',
      label: t('auth.socialTelegram'),
      icon: <TelegramBrandIcon className="size-5" />,
      loading: redirecting === 'telegram-oidc',
      onClick: () => {
        setRedirecting('telegram-oidc')
        const ref = referralCode?.trim()
        const start = '/cabinet/api/auth/telegram/start'
        window.location.href = ref ? `${start}?ref=${encodeURIComponent(ref)}` : start
      },
    })
  }

  useEffect(() => {
    let cancelled = false
    const c = new AbortController()
    const tid = window.setTimeout(() => c.abort(), BOOTSTRAP_TIMEOUT_MS)
    ;(async () => {
      try {
        const b = await api.authBootstrap(c.signal)
        if (cancelled) return
        setOauth({
          google: b.google_oauth_enabled,
          yandex: b.yandex_oauth_enabled ?? false,
          vk: b.vk_oauth_enabled ?? false,
          telegramBot: b.telegram_widget_bot,
          telegramOIDCEnabled: b.telegram_oidc_enabled ?? false,
          telegramWebAuthMode: b.telegram_web_auth_mode,
        })
      } catch {
        if (cancelled) return
        setOauth(OAUTH_BEFORE_BOOTSTRAP)
      } finally {
        window.clearTimeout(tid)
      }
    })()
    return () => {
      cancelled = true
      c.abort()
    }
  }, [])

  useEffect(() => {
    if (!inMiniApp) return
    try {
      window.Telegram?.WebApp?.ready?.()
      window.Telegram?.WebApp?.expand?.()
    } catch {
      /* ignore */
    }
  }, [inMiniApp])

  useLayoutEffect(() => {
    setTgWidgetError(false)
    const el = tgMountRef.current
    if (!embedTelegramWidget || !el || !onTelegramAuthRef.current || !bot) {
      setTgWidgetLoading(false)
      return
    }

    setTgWidgetLoading(true)
    return mountTelegramLoginWidgetScript(el, {
      page,
      bot,
      onAuth: async (user) => {
        setTgAuthPending(true)
        try {
          await (onTelegramAuthRef.current?.(user) ?? Promise.resolve())
        } finally {
          setTgAuthPending(false)
        }
      },
      onScriptFailed: () => setTgWidgetError(true),
      onScriptLoadingChange: setTgWidgetLoading,
    })
  }, [bot, page, embedTelegramWidget])

  if (!showSocial) return null

  return (
    <>
      <div className="relative my-4">
        <div className="absolute inset-0 flex items-center">
          <span className="w-full border-t border-border" />
        </div>
        <div className="relative flex justify-center text-xs text-[rgb(100,116,139)] dark:text-[rgb(107,114,128)]">
          <span className="bg-card px-2">{t('common.or')}</span>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        {socialButtons.map((btn, i) => {
          const isOddTail = socialButtons.length % 2 === 1 && i === socialButtons.length - 1
          return (
            <Button
              key={btn.key}
              type="button"
              variant="outline"
              loading={btn.loading}
              className={`h-16 w-full flex-col justify-center gap-1 rounded-xl border-border/80 bg-card/70 hover:bg-muted/60 ${
                isOddTail ? 'col-span-2 mx-auto max-w-[220px]' : ''
              }`}
              onClick={() => void btn.onClick()}
            >
              {btn.icon}
              <span className="text-sm font-medium text-[rgb(100,116,139)] dark:text-[rgb(107,114,128)]">{btn.label}</span>
            </Button>
          )
        })}
        {embedTelegramWidget && (
          <div className="col-span-2 flex min-h-[48px] w-full flex-col items-stretch justify-center gap-2">
            {tgWidgetError && (
              <Alert variant="destructive">
                <AlertDescription>{t('auth.telegramWidgetLoadError')}</AlertDescription>
              </Alert>
            )}
            <div className="relative flex min-h-[48px] w-full justify-center">
              {(tgWidgetLoading || tgAuthPending) && !tgWidgetError && (
                <div
                  className="absolute inset-0 z-10 flex items-center justify-center rounded-lg bg-background/70"
                  aria-busy
                  aria-label={t('auth.telegramWidgetLoading')}
                >
                  <Loader2 className="size-7 animate-spin text-primary" aria-hidden />
                </div>
              )}
              <div ref={tgMountRef} className="flex w-full justify-center [&_iframe]:max-w-full" />
            </div>
          </div>
        )}
      </div>
    </>
  )
}
