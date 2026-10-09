import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'

import { api, SUBSCRIPTION_STALE_MS } from '@/lib/api'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import {
  buildCabinetDeepLinkRedirectUrl,
  needsTelegramDeepLinkWorkaround,
  openCabinetDeepLinkRedirectExternally,
  prefersSameTabDeepLink,
  prefersSameTabIosAppDeepLink,
} from '@/lib/deep-link-redirect'
import type { AppConfig, AppGuide, Lang, PlatformKey } from './guide/types'

/**
 * Токен приглашения из фрагмента адреса (#t=...).
 *
 * Фрагмент, а не query: он не уходит на сервер, не попадает в Referer и не
 * виден краулеру, который разворачивает превью ссылки в мессенджере, — а
 * приглашение рассылают именно через мессенджеры.
 */
function parseInviteToken(hash: string): string {
  const raw = (hash || '').replace(/^#/, '')
  if (!raw) return ''
  return new URLSearchParams(raw).get('t')?.trim() || ''
}

function encodeBase64UrlSafe(value: string): string {
  const utf8 = unescape(encodeURIComponent(value))
  return btoa(utf8)
}

/** Часть deep link после urlScheme: base64, сырая ссылка или component-encoding. */
function subscriptionPayloadForScheme(scheme: string, subscriptionLink: string, isNeedBase64Encoding: boolean | undefined): string {
  if (isNeedBase64Encoding) {
    return encodeBase64UrlSafe(subscriptionLink)
  }
  const s = scheme.trim().toLowerCase()
  // Deep link: префикс + полный URL подписки без encodeURIComponent (иначе https:// и точки «ломаются»).
  const rawUrlPrefixes = ['happ://add/', 'incy://add/', 'v2raytun://import/', 'v2rayn://import/'] as const
  if (rawUrlPrefixes.some((p) => s.startsWith(p))) {
    return subscriptionLink
  }
  return encodeURIComponent(subscriptionLink)
}

/**
 * Готовый зашифрованный deep link для приложения, если он пришёл с приглашением.
 *
 * Сопоставление идёт по префиксу urlScheme, а не по id приложения: в
 * app-config один и тот же клиент встречается под разными id по платформам,
 * а схема у него одна.
 */
function encryptedLinkForApp(app: Pick<AppGuide, 'urlScheme'>, links: Record<string, string>): string {
  const scheme = (app.urlScheme || '').trim().toLowerCase()
  if (scheme.startsWith('happ://')) return links.happ || ''
  if (scheme.startsWith('incy://')) return links.incy || ''
  return ''
}

function detectPlatformFromUA(): PlatformKey | '' {
  if (typeof navigator === 'undefined') return ''
  const ua = navigator.userAgent.toLowerCase()

  if (ua.includes('android') && ua.includes('tv')) return 'androidTV'
  if (ua.includes('appletv') || ua.includes('apple tv')) return 'appleTV'
  if (ua.includes('android')) return 'android'
  if (ua.includes('iphone') || ua.includes('ipad') || ua.includes('ipod')) return 'ios'
  if (ua.includes('windows')) return 'windows'
  if (ua.includes('mac os x') || ua.includes('macintosh')) return 'macos'
  if (ua.includes('linux')) return 'linux'
  return ''
}

/**
 * Данные и действия гида подключения: конфиг приложений, подписка или
 * приглашение, выбор платформы и приложения, сборка deep link.
 *
 * Один и тот же гайд обслуживает две страницы: /connections для владельца
 * (подписка приходит из /me/subscription) и публичную /connect, куда попадают
 * по приглашению — там нет сессии, и всё нужное отдаёт /public/connect.
 */
export function useConnectionGuide() {
  const { i18n } = useTranslation()
  const lang: Lang = i18n.language?.toLowerCase().startsWith('en') ? 'en' : 'ru'
  const location = useLocation()
  const inviteToken = useMemo(() => parseInviteToken(location.hash), [location.hash])
  const inviteMode = Boolean(inviteToken)
  const [selectedPlatform, setSelectedPlatform] = useState<PlatformKey>('')
  const [selectedAppId, setSelectedAppId] = useState<string>('')

  const { data: config, isLoading: configLoading, error: configError } = useQuery<AppConfig>({
    queryKey: ['app-config'],
    queryFn: async () => {
      const resp = await fetch('/cabinet/api/content/app-config', { cache: 'no-store' })
      if (!resp.ok) throw new Error(`config status ${resp.status}`)
      return (await resp.json()) as AppConfig
    },
    // Список приложений и инструкций меняется вместе с деплоем, не в рантайме.
    staleTime: 5 * 60_000,
    retry: 1,
  })

  const { data: subscription, isLoading: subLoading } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
    enabled: !inviteMode,
  })

  // Приглашение: сервер сам решает, отдать сырую ссылку подписки (шифрование
  // deep link выключено) или только готовые зашифрованные ссылки по
  // приложениям. Клиенту гадать не нужно — он рисует то, что пришло.
  const {
    data: invite,
    isLoading: inviteLoading,
    error: inviteError,
  } = useQuery({
    queryKey: ['public-connect', inviteToken],
    queryFn: () => api.publicConnect(inviteToken),
    enabled: inviteMode,
    staleTime: 60_000,
    retry: 1,
  })

  const { data: bootstrap } = useAuthBootstrap()

  const encryptedLinks = useMemo(
    () => (invite?.mode === 'encrypted' ? invite.links || {} : {}),
    [invite],
  )
  const subscriptionLink = (
    inviteMode ? invite?.subscription_link || '' : subscription?.subscription_link || ''
  ).trim()

  // В защищённом режиме приглашения сырой ссылки подписки у страницы нет, а
  // значит нечего подставлять во все остальные клиенты — оставляем только те
  // приложения, для которых бэкенд прислал готовый зашифрованный deep link.
  const appsFor = useCallback(
    (platform: PlatformKey): AppGuide[] => {
      const all = config?.platforms?.[platform] || []
      if (invite?.mode !== 'encrypted') return all
      return all.filter((app) => Boolean(encryptedLinkForApp(app, encryptedLinks)))
    },
    [config, invite?.mode, encryptedLinks],
  )

  const availablePlatforms = useMemo(
    () => Object.keys(config?.platforms || {}).filter((key) => appsFor(key).length > 0),
    [config, appsFor],
  )

  const detectedPlatform = useMemo(() => detectPlatformFromUA(), [])

  useEffect(() => {
    if (!availablePlatforms.length) return
    if (selectedPlatform && availablePlatforms.includes(selectedPlatform)) return

    if (!selectedPlatform && detectedPlatform && availablePlatforms.includes(detectedPlatform)) {
      setSelectedPlatform(detectedPlatform)
      return
    }

    setSelectedPlatform(availablePlatforms[0])
  }, [availablePlatforms, selectedPlatform, detectedPlatform])

  // Пока приглашение не разрешилось, показывать нечего: фильтр по нему ещё
  // не применён.
  const apps = useMemo(() => (inviteLoading ? [] : appsFor(selectedPlatform)), [appsFor, selectedPlatform, inviteLoading])

  useEffect(() => {
    if (!apps.length) return
    const featured = apps.find((a) => a.isFeatured)
    const fallback = featured || apps[0]
    if (!fallback) return
    if (!apps.some((a) => a.id === selectedAppId)) setSelectedAppId(fallback.id)
  }, [apps, selectedAppId])

  const selectedApp = useMemo(
    () => apps.find((a) => a.id === selectedAppId) || apps[0],
    [apps, selectedAppId],
  )

  /** Смена платформы сразу берёт её рекомендуемое приложение. */
  const pickPlatform = useCallback(
    (platform: PlatformKey) => {
      setSelectedPlatform(platform)
      const list = appsFor(platform)
      const next = list.find((a) => a.isFeatured) || list[0]
      if (next) setSelectedAppId(next.id)
    },
    [appsFor],
  )

  // Для Happ/INCY при включённом админом шифровании deep link формируется на
  // бэкенде (happ://crypt5/ или incy://crypt1/), чтобы ссылку подписки нельзя было
  // подсмотреть/отредактировать в приложении и она не читалась сканерами чатов.
  //
  // В режиме приглашения этой ветки нет: /me/deeplink требует авторизации, а
  // готовая зашифрованная ссылка уже пришла вместе с приглашением.
  const encryptApp = useMemo<'happ' | 'incy' | ''>(() => {
    if (inviteMode) return ''
    const s = (selectedApp?.urlScheme || '').trim().toLowerCase()
    if (s.startsWith('happ://') && bootstrap?.deeplink_happ_encrypt) return 'happ'
    if (s.startsWith('incy://') && bootstrap?.deeplink_incy_encrypt) return 'incy'
    return ''
  }, [selectedApp, bootstrap, inviteMode])

  const inviteHref = useMemo(
    () => (selectedApp ? encryptedLinkForApp(selectedApp, encryptedLinks) : ''),
    [selectedApp, encryptedLinks],
  )

  // Предзагружаем зашифрованную ссылку заранее, чтобы клик по кнопке оставался
  // синхронным (иначе await ломает открытие deep link на iOS/Safari).
  const {
    data: encryptedDeeplink,
    isFetching: encryptedDeeplinkLoading,
    isError: encryptedDeeplinkError,
    refetch: refetchEncryptedDeeplink,
  } = useQuery({
    queryKey: ['deeplink', encryptApp, subscriptionLink],
    queryFn: () => api.deeplink(encryptApp as 'happ' | 'incy'),
    enabled: !!encryptApp && !!subscriptionLink,
    staleTime: 0,
    retry: 1,
  })

  const encryptedHref = (encryptedDeeplink?.deeplink || '').trim()

  /** true — переход в приложение запущен; false — ссылки ещё нет (гид шаг не засчитывает). */
  function openAddSubscription(): boolean {
    if (!selectedApp) return false
    const scheme = (selectedApp.urlScheme || '').trim()
    if (!scheme) return false
    let href: string
    if (inviteHref) {
      // Приглашение в защищённом режиме: ссылка уже зашифрована бэкендом,
      // собирать нечего.
      href = inviteHref
    } else if (!subscriptionLink) {
      return false
    } else if (encryptApp) {
      if (!encryptedHref) {
        // Ещё не готово или прошлая попытка упала — пробуем получить ссылку снова.
        void refetchEncryptedDeeplink()
        return false
      }
      href = encryptedHref
    } else {
      const payload = subscriptionPayloadForScheme(scheme, subscriptionLink, selectedApp.isNeedBase64Encoding)
      href = `${scheme}${payload}`
    }
    if (needsTelegramDeepLinkWorkaround()) {
      openCabinetDeepLinkRedirectExternally(href)
      return true
    }
    // iOS Chrome / Edge / Firefox / Opera: window.open(customScheme) → часто пустая вкладка.
    // Промежуточная /deeplink делает переход через location.assign (и запасная кнопка «Открыть приложение»).
    if (prefersSameTabIosAppDeepLink()) {
      window.location.assign(buildCabinetDeepLinkRedirectUrl(href))
      return true
    }
    if (prefersSameTabDeepLink()) {
      window.location.href = href
      return true
    }
    window.open(href, '_blank', 'noopener,noreferrer')
    return true
  }

  const addDisabled =
    !selectedApp ||
    (!subscriptionLink && !inviteHref) ||
    !(selectedApp.urlScheme || '').trim() ||
    (!!encryptApp && encryptedDeeplinkLoading)

  /** Почему «Добавить подписку» не сработает или ещё не готова. */
  const addHint: 'noSubscription' | 'encryptedError' | 'encryptedLoading' | '' =
    !subscriptionLink && !inviteHref
      ? 'noSubscription'
      : encryptedDeeplinkError
        ? 'encryptedError'
        : encryptApp && encryptedDeeplinkLoading
          ? 'encryptedLoading'
          : ''

  // Бренд — как в шапке кабинета (env), app-config — запасной источник.
  const brandName = (bootstrap?.brand_name?.trim() || config?.config?.branding?.name?.trim() || '').trim()
  const brandLogoUrl = (bootstrap?.brand_logo_url?.trim() || config?.config?.branding?.logoUrl?.trim() || '').trim()

  // Поддержка: чат кабинета (гостю по приглашению недоступен — нужна сессия),
  // иначе SUPPORT_URL, иначе ссылка из app-config. Ничего нет — плитки нет.
  const supportChat = !inviteMode && Boolean(bootstrap?.support_chat_enabled)
  const supportUrl = (bootstrap?.site_links?.support?.trim() || config?.config?.branding?.supportUrl?.trim() || '').trim()

  return {
    lang,
    inviteMode,
    inviteToken,
    loading: configLoading || subLoading || inviteLoading,
    configError,
    inviteError,
    availablePlatforms,
    selectedPlatform,
    pickPlatform,
    appsFor,
    selectedApp,
    setSelectedAppId,
    subscriptionLink,
    openAddSubscription,
    addDisabled,
    addHint,
    brandName,
    brandLogoUrl,
    supportChat,
    supportUrl,
  }
}
