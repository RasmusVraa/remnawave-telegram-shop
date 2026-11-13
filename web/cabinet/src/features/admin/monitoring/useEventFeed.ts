import { useCallback, useEffect, useRef, useState } from 'react'

import { api } from '@/lib/api'
import type { AdminEventDTO } from '@/lib/types/admin'

/** Опрос новых событий, пока вкладка видна. */
const POLL_MS = 4_000
/** Сколько секунд новая строка подсвечена. */
const FRESH_MS = 3_000

interface FeedOptions {
  categories: string[]
  botActions: boolean
  /** История одного клиента. */
  customerId?: number
  /** Окно ленты в мс: показываются только события за этот срок. */
  periodMs?: number
  paused?: boolean
  pageSize?: number
  /** false — не опрашивать новые (окно истории клиента). */
  live?: boolean
  enabled?: boolean
}

/**
 * Лента событий: первая страница, опрос новых по after_id и подгрузка вниз по
 * before_id. Смена фильтра перезагружает ленту с начала.
 */
export function useEventFeed({
  categories,
  botActions,
  customerId,
  periodMs,
  paused = false,
  pageSize = 50,
  live = true,
  enabled = true,
}: FeedOptions) {
  const [items, setItems] = useState<AdminEventDTO[]>([])
  const [fresh, setFresh] = useState<Set<number>>(() => new Set())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [botActionsEnabled, setBotActionsEnabled] = useState(false)
  // Поколение запроса: ответ на старый фильтр не должен лечь в новую ленту.
  const gen = useRef(0)
  const itemsRef = useRef<AdminEventDTO[]>([])
  itemsRef.current = items
  const loadingRef = useRef(true)
  loadingRef.current = loading
  // Опрос уже в пути: второй с тем же after_id добавил бы те же строки ещё раз.
  const polling = useRef(false)
  const timers = useRef<number[]>([])
  useEffect(() => () => timers.current.forEach((id) => window.clearTimeout(id)), [])
  const since = () => (periodMs ? Date.now() - periodMs : undefined)
  const filterKey = `${categories.join(',')}|${botActions ? 1 : 0}|${customerId ?? 0}|${periodMs ?? 0}`

  const load = useCallback(async () => {
    const my = ++gen.current
    // Сразу, не дожидаясь рендера: опрос того же тика не должен взять старый
    // верх ленты и прошлый фильтр.
    loadingRef.current = true
    itemsRef.current = []
    setLoading(true)
    setError(false)
    try {
      const res = await api.adminMonitoringEvents({ categories, botActions, customerId, sinceMs: since(), limit: pageSize })
      if (my !== gen.current) return
      setItems(res.items ?? [])
      setHasMore((res.items ?? []).length >= pageSize)
      setBotActionsEnabled(res.bot_actions_enabled)
    } catch {
      if (my === gen.current) setError(true)
    } finally {
      if (my === gen.current) setLoading(false)
    }
    // filterKey покрывает categories/botActions/customerId.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterKey, pageSize])

  useEffect(() => {
    if (!enabled) return
    setItems([])
    setHasMore(true)
    void load()
  }, [load, enabled])

  const poll = useCallback(async () => {
    // Первая страница ещё грузится или прошлый опрос не вернулся — ждём тика.
    if (loadingRef.current || polling.current) return
    polling.current = true
    const my = gen.current
    const top = itemsRef.current[0]?.id ?? 0
    try {
      const res = await api.adminMonitoringEvents({ categories, botActions, customerId, afterId: top, limit: pageSize })
      if (my !== gen.current) return
      setBotActionsEnabled(res.bot_actions_enabled)
      // Сервер отдаёт только новее top; фильтр — на случай повтора, чтобы строки не задвоились.
      const newer = (res.items ?? []).filter((e) => e.id > top)
      if (newer.length === 0) return
      if (newer.length >= pageSize) {
        // Пропустили больше страницы — проще начать ленту заново, чем держать дыру.
        setItems(newer)
        setHasMore(true)
      } else {
        // Сверка с текущим верхом, а не с тем, что был при отправке запроса.
        setItems((cur) => {
          const curTop = cur[0]?.id ?? 0
          const add = newer.filter((e) => e.id > curTop)
          return add.length ? [...add, ...cur] : cur
        })
      }
      const ids = newer.map((e) => e.id)
      setFresh((cur) => new Set([...cur, ...ids]))
      const timer = window.setTimeout(() => {
        timers.current = timers.current.filter((x) => x !== timer)
        setFresh((cur) => {
          const next = new Set(cur)
          ids.forEach((id) => next.delete(id))
          return next
        })
      }, FRESH_MS)
      timers.current.push(timer)
    } catch {
      // Сбой опроса молчит: следующий тик попробует снова, лента остаётся.
    } finally {
      polling.current = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterKey, pageSize])

  useEffect(() => {
    if (!enabled || !live || paused) return
    let timer = 0
    const tick = () => {
      if (document.visibilityState === 'visible') void poll()
    }
    timer = window.setInterval(tick, POLL_MS)
    const onVisible = () => {
      if (document.visibilityState === 'visible') void poll()
    }
    document.addEventListener('visibilitychange', onVisible)
    // Сняли паузу — сразу подтянуть пропущенное.
    tick()
    return () => {
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [enabled, live, paused, poll])

  const loadMore = useCallback(async () => {
    const last = itemsRef.current[itemsRef.current.length - 1]?.id
    if (!last || loadingMore || !hasMore) return
    const my = gen.current
    setLoadingMore(true)
    try {
      const res = await api.adminMonitoringEvents({ categories, botActions, customerId, beforeId: last, sinceMs: since(), limit: pageSize })
      if (my !== gen.current) return
      const older = (res.items ?? []).filter((e) => e.id < last)
      setItems((cur) => [...cur, ...older])
      setHasMore(older.length >= pageSize)
    } catch {
      if (my === gen.current) setError(true)
    } finally {
      if (my === gen.current) setLoadingMore(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterKey, pageSize, loadingMore, hasMore])

  return { items, fresh, loading, error, loadingMore, hasMore, loadMore, reload: load, botActionsEnabled }
}
