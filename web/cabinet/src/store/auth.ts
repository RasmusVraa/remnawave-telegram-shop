import { create } from 'zustand'
import { api, setAuthStoreRef, type MeResponse } from '@/lib/api'
import { getTelegramInitData, getTelegramMiniAppStartParam } from '@/lib/utils'

/** Не держим «вечный» спиннер при недоступном API / зависшем fetch. */
const AUTH_NET_TIMEOUT_MS = 12_000
const TELEGRAM_INIT_DATA_WAIT_MS = 2_500
const TELEGRAM_INIT_DATA_POLL_MS = 150

function raceTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
  return new Promise((resolve, reject) => {
    const t = window.setTimeout(() => reject(new Error('timeout')), ms)
    p.then(
      (v) => {
        window.clearTimeout(t)
        resolve(v)
      },
      (e) => {
        window.clearTimeout(t)
        reject(e)
      },
    )
  })
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

async function waitForTelegramInitData(timeoutMs = TELEGRAM_INIT_DATA_WAIT_MS): Promise<string> {
  const startedAt = Date.now()
  while (Date.now() - startedAt < timeoutMs) {
    const data = getTelegramInitData()
    if (data) return data
    await sleep(TELEGRAM_INIT_DATA_POLL_MS)
  }
  return ''
}

interface AuthState {
  accessToken: string | null
  user: MeResponse | null
  initialized: boolean

  setToken: (token: string) => void
  setUser: (user: MeResponse | null) => void
  /** Окно обязательной подписки на канал: открыть (info) или закрыть (null). */
  setChannelGate: (info: { required: boolean; channel_url?: string } | null) => void
  logout: () => void
  getAccessToken: () => string | null
  fetchMe: () => Promise<void>
  initialize: () => Promise<void>
  /** После фоновой загрузки telegram-web-app.js (Mini App). */
  tryTelegramMiniAppAfterSdk: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set, get) => ({
  accessToken: null,
  user: null,
  initialized: false,

  setToken: (token) => set({ accessToken: token }),

  setUser: (user) => set({ user }),

  setChannelGate: (info) =>
    set((state) => {
      if (!state.user) return state
      const user = { ...state.user }
      if (info) user.channel_gate = info
      else delete user.channel_gate
      return { user }
    }),

  logout: () => {
    set({ accessToken: null, user: null })
    // best-effort logout на сервер (сбрасывает refresh cookie)
    api.logout().catch(() => {})
  },

  getAccessToken: () => get().accessToken,

  fetchMe: async () => {
    try {
      const user = await raceTimeout(api.me(), AUTH_NET_TIMEOUT_MS)
      set({ user })
    } catch {
      set({ user: null })
    }
  },

  initialize: async () => {
    // 1) Тихий refresh по cookie.
    try {
      const data = await raceTimeout(api.refresh(), AUTH_NET_TIMEOUT_MS)
      set({ accessToken: data.access_token })
      const user = await raceTimeout(api.me(), AUTH_NET_TIMEOUT_MS)
      set({ user, initialized: true })
      return
    } catch {
      /* fall through */
    }

    // 2) Telegram Mini App: автологин по initData (если открыто из бота).
    let tgData = getTelegramInitData()
    if (!tgData) {
      tgData = await waitForTelegramInitData()
    }
    if (tgData) {
      try {
        const ref = getTelegramMiniAppStartParam()
        const data = await raceTimeout(api.telegramAuthMiniApp(tgData, ref || undefined), AUTH_NET_TIMEOUT_MS)
        set({ accessToken: data.access_token })
        const user = await raceTimeout(api.me(), AUTH_NET_TIMEOUT_MS)
        set({ user, initialized: true })
        return
      } catch {
        /* fall through */
      }
    }

    set({ initialized: true })
  },

  tryTelegramMiniAppAfterSdk: async () => {
    if (get().accessToken) return
    const tgData = getTelegramInitData()
    if (!tgData) return
    try {
      const ref = getTelegramMiniAppStartParam()
      const data = await raceTimeout(api.telegramAuthMiniApp(tgData, ref || undefined), AUTH_NET_TIMEOUT_MS)
      const user = await raceTimeout(api.me(), AUTH_NET_TIMEOUT_MS)
      set({ accessToken: data.access_token, user })
    } catch {
      /* ignore */
    }
  },
}))

// Регистрируем ссылку в API-клиенте (lazy, без circular dep).
setAuthStoreRef({
  getAccessToken: () => useAuthStore.getState().getAccessToken(),
  setToken: (token) => useAuthStore.getState().setToken(token),
  logout: () => useAuthStore.getState().logout(),
  setChannelGate: (info) => useAuthStore.getState().setChannelGate(info),
})
