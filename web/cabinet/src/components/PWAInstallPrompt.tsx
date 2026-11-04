import { useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { X, Download } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'

const LS_PWA_DISMISSED = 'cab_pwa_install_prompt_dismissed'
const LS_PWA_INSTALLED = 'cab_pwa_install_installed'

type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed'; platform: string }>
}

function isStandalone(): boolean {
  if (typeof window === 'undefined') return false
  const nav = window.navigator as Navigator & { standalone?: boolean }
  return Boolean(window.matchMedia('(display-mode: standalone)').matches || nav.standalone)
}

function isIOS(): boolean {
  if (typeof window === 'undefined') return false
  const ua = window.navigator.userAgent.toLowerCase()
  return /iphone|ipad|ipod/.test(ua)
}

function isSafari(): boolean {
  if (typeof window === 'undefined') return false
  const ua = window.navigator.userAgent.toLowerCase()
  return ua.includes('safari') && !ua.includes('chrome') && !ua.includes('android')
}

export function PWAInstallPrompt() {
  const { t } = useTranslation()
  const { data: bootstrap } = useAuthBootstrap()

  const [deferredPrompt, setDeferredPrompt] = useState<BeforeInstallPromptEvent | null>(null)
  const [iosHelp, setIosHelp] = useState(false)
  const [hidden, setHidden] = useState(false)

  const pwaEnabled = Boolean(bootstrap?.pwa_enabled)
  const appName = bootstrap?.pwa_app_name?.trim() || 'Cabinet'

  useEffect(() => {
    if (!pwaEnabled) return
    if (localStorage.getItem(LS_PWA_INSTALLED) === '1') {
      setHidden(true)
      return
    }
    if (localStorage.getItem(LS_PWA_DISMISSED) === '1') {
      setHidden(true)
      return
    }
    if (isStandalone()) {
      localStorage.setItem(LS_PWA_INSTALLED, '1')
      setHidden(true)
      return
    }
    if (isIOS() && isSafari()) {
      setIosHelp(true)
    }
  }, [pwaEnabled])

  useEffect(() => {
    if (!pwaEnabled) return
    function onBeforeInstallPrompt(e: Event) {
      e.preventDefault()
      setDeferredPrompt(e as BeforeInstallPromptEvent)
    }
    function onAppInstalled() {
      localStorage.setItem(LS_PWA_INSTALLED, '1')
      setHidden(true)
      setDeferredPrompt(null)
      setIosHelp(false)
    }
    window.addEventListener('beforeinstallprompt', onBeforeInstallPrompt)
    window.addEventListener('appinstalled', onAppInstalled)
    return () => {
      window.removeEventListener('beforeinstallprompt', onBeforeInstallPrompt)
      window.removeEventListener('appinstalled', onAppInstalled)
    }
  }, [pwaEnabled])

  useEffect(() => {
    if (!pwaEnabled) return
    if (!('serviceWorker' in navigator)) return
    let hasReloadedForUpdate = false

    function forceActivate(reg: ServiceWorkerRegistration | null) {
      if (!reg?.waiting) return
      reg.waiting.postMessage({ type: 'SKIP_WAITING' })
    }

    function onControllerChange() {
      if (hasReloadedForUpdate) return
      hasReloadedForUpdate = true
      window.location.reload()
    }

    navigator.serviceWorker.addEventListener('controllerchange', onControllerChange)
    navigator.serviceWorker
      .register('/cabinet/sw.js', { updateViaCache: 'none' })
      .then((reg) => {
        // Immediate activation when a new worker is already waiting.
        forceActivate(reg)

        reg.addEventListener('updatefound', () => {
          const worker = reg.installing
          if (!worker) return
          worker.addEventListener('statechange', () => {
            if (worker.state === 'installed' && navigator.serviceWorker.controller) {
              forceActivate(reg)
            }
          })
        })

        // Proactively ask browser to check updates on each app start.
        void reg.update()
      })
      .catch(() => {
        // no-op: pwa optional
      })

    return () => {
      navigator.serviceWorker.removeEventListener('controllerchange', onControllerChange)
    }
  }, [pwaEnabled])

  const visible = useMemo(() => {
    if (!pwaEnabled) return false
    if (hidden) return false
    return Boolean(deferredPrompt || iosHelp)
  }, [pwaEnabled, hidden, deferredPrompt, iosHelp])

  if (!visible) return null

  async function install() {
    if (!deferredPrompt) return
    await deferredPrompt.prompt()
    const choice = await deferredPrompt.userChoice
    if (choice.outcome === 'accepted') {
      localStorage.setItem(LS_PWA_INSTALLED, '1')
      setHidden(true)
    }
    setDeferredPrompt(null)
  }

  function dismiss() {
    localStorage.setItem(LS_PWA_DISMISSED, '1')
    setHidden(true)
  }

  return createPortal(
    <div className="fixed inset-x-3 bottom-[calc(5.75rem+env(safe-area-inset-bottom))] z-[900] sm:inset-x-auto sm:bottom-4 sm:right-4 sm:w-[360px]">
      <div className="rounded-xl border border-border bg-card shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)]">
        <div className="flex items-start justify-between gap-3 p-3">
          <div className="min-w-0">
            <p className="text-sm font-semibold">
              {t('pwa.installTitle', { app: appName })}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              {iosHelp ? t('pwa.iosHint') : t('pwa.installHint')}
            </p>
          </div>
          <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={dismiss} aria-label={t('pwa.dismiss')}>
            <X size={14} />
          </Button>
        </div>
        <div className="flex items-center justify-end gap-2 px-3 pb-3">
          {!iosHelp && (
            // Ждём не сервер, а ответ в системном окне установки — без спиннера.
            <Button size="sm" onClick={() => { void install() }}>
              <Download size={14} />
              {t('pwa.installAction')}
            </Button>
          )}
        </div>
      </div>
    </div>,
    document.body,
  )
}
