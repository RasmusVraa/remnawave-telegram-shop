import { useCallback, useEffect, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient, type InfiniteData } from '@tanstack/react-query'
import { useLocation, useNavigate } from 'react-router-dom'

import { api, type NotificationsListDTO, type NotificationsUnreadDTO } from '@/lib/api'

export const UNREAD_KEY = ['notifications-unread'] as const
export const FEED_KEY = ['notifications-feed'] as const
const PAGE = 20

/**
 * Счётчик колокольчика и плашка важного. Опрос раз в минуту и при возврате во
 * вкладку: постоянное соединение ради счётчика не нужно, а пуш, пришедший в
 * открытую вкладку, обновляет его сразу (см. useServiceWorkerRefresh).
 */
export function useUnreadNotifications() {
  return useQuery({
    queryKey: UNREAD_KEY,
    queryFn: () => api.notificationsUnread(),
    refetchInterval: 60_000,
    refetchOnWindowFocus: true,
    staleTime: 30_000,
    retry: false,
  })
}

export function useNotificationsFeed(enabled: boolean) {
  return useInfiniteQuery({
    queryKey: FEED_KEY,
    queryFn: ({ pageParam }) => api.notifications({ limit: PAGE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + p.items.length, 0)
      return loaded < last.total ? loaded : undefined
    },
    enabled,
    staleTime: 0,
  })
}

/**
 * Отметки «прочитано» копятся и уходят одной пачкой: на телефоне при
 * прокрутке ленты видимыми становятся сразу несколько карточек, и запрос на
 * каждую был бы лишним шумом.
 */
export function useReadQueue() {
  const qc = useQueryClient()
  const pending = useRef(new Set<number>())
  const timer = useRef<number | null>(null)
  const [readIds, setReadIds] = useState<Set<number>>(() => new Set())

  const flush = useCallback(async () => {
    if (timer.current !== null) {
      window.clearTimeout(timer.current)
      timer.current = null
    }
    const ids = [...pending.current]
    if (ids.length === 0) return
    pending.current.clear()
    try {
      const unread = await api.notificationsRead({ ids })
      qc.setQueryData<NotificationsUnreadDTO>(UNREAD_KEY, unread)
      // Ленту в кэше — тоже: при следующем открытии она рисуется из кэша раньше,
      // чем придёт свежая, и прочитанное не должно показаться новым.
      const done = new Set(ids)
      qc.setQueryData<InfiniteData<NotificationsListDTO>>(FEED_KEY, (data) =>
        data
          ? { ...data, pages: data.pages.map((p) => ({ ...p, items: p.items.map((it) => (done.has(it.id) ? { ...it, read: true } : it)) })) }
          : data,
      )
    } catch {
      // Не дошло — вернём в очередь: отметится при следующей пачке.
      ids.forEach((id) => pending.current.add(id))
    }
  }, [qc])

  const markRead = useCallback(
    (id: number) => {
      if (pending.current.has(id)) return
      setReadIds((prev) => (prev.has(id) ? prev : new Set(prev).add(id)))
      pending.current.add(id)
      if (timer.current === null) timer.current = window.setTimeout(() => void flush(), 700)
    },
    [flush],
  )

  const markAllRead = useCallback(async () => {
    pending.current.clear()
    const unread = await api.notificationsRead({ all: true })
    qc.setQueryData<NotificationsUnreadDTO>(UNREAD_KEY, unread)
    qc.setQueryData<InfiniteData<NotificationsListDTO>>(FEED_KEY, (data) =>
      data
        ? { ...data, pages: data.pages.map((p) => ({ ...p, items: p.items.map((it) => ({ ...it, read: true })) })) }
        : data,
    )
  }, [qc])

  /** Крестик: карточка пропадает сразу, запрос уходит следом. */
  const remove = useCallback(
    async (id: number) => {
      pending.current.delete(id)
      qc.setQueryData<InfiniteData<NotificationsListDTO>>(FEED_KEY, (data) =>
        data
          ? {
              ...data,
              pages: data.pages.map((p) => ({ ...p, total: Math.max(0, p.total - 1), items: p.items.filter((it) => it.id !== id) })),
            }
          : data,
      )
      try {
        const unread = await api.notificationsDelete([id])
        qc.setQueryData<NotificationsUnreadDTO>(UNREAD_KEY, unread)
      } catch {
        void qc.invalidateQueries({ queryKey: FEED_KEY })
      }
    },
    [qc],
  )

  // Ушли со страницы — отправить, что успели прочитать.
  useEffect(() => () => void flush(), [flush])

  return { markRead, markAllRead, remove, flush, readIds }
}

const HASH = '#notifications'

/** Лента открывается якорем #notifications: его же ставит push по нажатию. */
export function useNotificationsSheetState() {
  const location = useLocation()
  const navigate = useNavigate()
  const open = location.hash === HASH

  const show = useCallback(() => {
    navigate({ pathname: location.pathname, search: location.search, hash: HASH }, { state: { fromBell: true } })
  }, [navigate, location.pathname, location.search])

  const hide = useCallback(() => {
    // Открыли кнопкой — шаг назад, чтобы «Назад» на телефоне не вернул ленту.
    if ((location.state as { fromBell?: boolean } | null)?.fromBell) navigate(-1)
    else navigate({ pathname: location.pathname, search: location.search, hash: '' }, { replace: true })
  }, [navigate, location.pathname, location.search, location.state])

  /**
   * Переход из ленты в раздел (кнопка уведомления). Закрыть ленту через hide()
   * нельзя: его шаг назад приходит позже и отменяет переход — кнопка «ничего
   * не делала». Поэтому запись с открытой лентой заменяется разделом: «Назад»
   * из раздела ведёт туда, где открыли колокольчик.
   */
  const leaveTo = useCallback(
    (to: string) => {
      const fromBell = (location.state as { fromBell?: boolean } | null)?.fromBell
      navigate(to, { replace: Boolean(fromBell) })
    },
    [navigate, location.state],
  )

  return { open, show, hide, leaveTo }
}

/** Push пришёл в открытую вкладку — счётчик обновляем сразу, без ожидания опроса. */
export function useServiceWorkerRefresh() {
  const qc = useQueryClient()
  useEffect(() => {
    if (!('serviceWorker' in navigator)) return
    const onMessage = (e: MessageEvent) => {
      if ((e.data as { type?: string } | null)?.type === 'cabinet-notification') {
        void qc.invalidateQueries({ queryKey: UNREAD_KEY })
        void qc.invalidateQueries({ queryKey: FEED_KEY })
      }
    }
    navigator.serviceWorker.addEventListener('message', onMessage)
    return () => navigator.serviceWorker.removeEventListener('message', onMessage)
  }, [qc])
}
