import { api } from '@/lib/api'
import { isTelegramMiniAppSession } from '@/lib/telegram-web-app-loader'

/*
 * Push в браузер: уведомления кабинета, когда вкладка закрыта.
 *
 * Сервер шлёт Web Push (VAPID-ключ — GET /me/push/key), service worker
 * кабинета (public/sw.js) показывает его системным уведомлением. Mini App
 * в Telegram push не умеет — там уведомления и так приходят от бота.
 */

export type PushState =
  /** Браузер не умеет push или это Telegram Mini App. */
  | 'unsupported'
  /** iPhone/iPad вне установленного на экран «Домой» приложения: Safari даёт push только так. */
  | 'ios-needs-install'
  /** Человек запретил уведомления в настройках браузера. */
  | 'denied'
  | 'off'
  | 'on'

function isIOS(): boolean {
  const ua = navigator.userAgent
  return /iPad|iPhone|iPod/.test(ua) || (ua.includes('Macintosh') && navigator.maxTouchPoints > 1)
}

function isStandalone(): boolean {
  return (
    window.matchMedia?.('(display-mode: standalone)').matches ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true
  )
}

function pushApiAvailable(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
}

/**
 * Регистрация service worker с активным воркером. register() отдаёт уже
 * существующую регистрацию: без PWA кабинет иначе SW не ставит, а push без
 * него не работает. Ждём активации сами и с таймаутом — serviceWorker.ready
 * при сбое установки висит вечно, и кнопка крутилась бы без конца.
 */
async function registration(): Promise<ServiceWorkerRegistration> {
  const reg = await navigator.serviceWorker.register('/cabinet/sw.js', { updateViaCache: 'none' })
  if (reg.active) return reg
  const worker = reg.installing ?? reg.waiting
  if (!worker) throw new Error('service worker not installing')
  await new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error('service worker activation timeout')), 15_000)
    const onState = () => {
      if (worker.state === 'activated') {
        window.clearTimeout(timer)
        resolve()
      } else if (worker.state === 'redundant') {
        window.clearTimeout(timer)
        reject(new Error('service worker install failed'))
      }
    }
    worker.addEventListener('statechange', onState)
    onState()
  })
  return reg
}

/** Подписать этот браузер и сообщить серверу. Разрешение уже должно быть. */
async function subscribe(): Promise<void> {
  const reg = await registration()
  const { public_key } = await api.pushKey()
  let sub = await reg.pushManager.getSubscription()
  if (!sub) {
    sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: vapidKey(public_key) })
  }
  await api.pushSubscribe(sub.toJSON())
}

/** Ключ VAPID из base64url в байты для pushManager.subscribe. */
function vapidKey(base64url: string): Uint8Array<ArrayBuffer> {
  const padded = (base64url + '='.repeat((4 - (base64url.length % 4)) % 4)).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(padded)
  const out = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}

export async function getPushState(): Promise<PushState> {
  if (isTelegramMiniAppSession()) return 'unsupported'
  if (!pushApiAvailable()) return isIOS() && !isStandalone() ? 'ios-needs-install' : 'unsupported'
  if (Notification.permission === 'denied') return 'denied'
  if (Notification.permission !== 'granted') return 'off'
  try {
    const reg = await navigator.serviceWorker.getRegistration('/cabinet/')
    const sub = await reg?.pushManager.getSubscription()
    return sub ? 'on' : 'off'
  } catch {
    return 'off'
  }
}

/** Включить push: спросить разрешение, подписать браузер, сообщить серверу. */
export async function enablePush(): Promise<PushState> {
  if (!pushApiAvailable() || isTelegramMiniAppSession()) return 'unsupported'
  const permission = await Notification.requestPermission()
  if (permission === 'denied') return 'denied'
  if (permission !== 'granted') return 'off'
  await subscribe()
  rememberPushOff(false)
  return 'on'
}

export async function disablePush(): Promise<PushState> {
  if (!pushApiAvailable()) return 'unsupported'
  const reg = await navigator.serviceWorker.getRegistration('/cabinet/')
  const sub = await reg?.pushManager.getSubscription()
  if (sub) {
    await api.pushUnsubscribe(sub.endpoint).catch(() => undefined)
    await sub.unsubscribe().catch(() => undefined)
  }
  rememberPushOff(true)
  return 'off'
}

const PUSH_OFF_KEY = 'cabinet_push_off'

/**
 * При входе в кабинет: уведомления для сайта разрешены — подписать браузер,
 * если он ещё не подписан. Разрешить их можно и в настройках браузера, мимо
 * нашей кнопки, и тогда push молча не приходил бы. Заодно сервер узнаёт о
 * подписке заново: он мог удалить её (push-сервис ответил 410) или браузер
 * теперь под другим аккаунтом — подписка переедет к нынешнему владельцу.
 * Выключил человек push сам — не включаем обратно.
 */
export async function syncPushSubscription(): Promise<void> {
  if (!pushApiAvailable() || isTelegramMiniAppSession() || Notification.permission !== 'granted') return
  try {
    if (localStorage.getItem(PUSH_OFF_KEY) === '1') return
  } catch {
    // Хранилище недоступно (приватный режим) — считаем, что не выключали.
  }
  try {
    await subscribe()
  } catch {
    // Сеть, сервер или push-сервис недоступны — попробуем при следующем входе.
  }
}

function rememberPushOff(off: boolean) {
  try {
    if (off) localStorage.setItem(PUSH_OFF_KEY, '1')
    else localStorage.removeItem(PUSH_OFF_KEY)
  } catch {
    // Не запомнилось — при следующем входе подпишем снова, это не страшно.
  }
}
