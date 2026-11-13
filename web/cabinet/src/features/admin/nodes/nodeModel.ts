import type { AdminNodeDTO, AdminNodeState } from '@/lib/types/admin'

export type NodeSortKey = 'users_desc' | 'users_asc' | 'name' | 'memory' | 'traffic' | 'uptime'
export type NodeFilterKey = 'all' | AdminNodeState

export const NODE_SORT_KEYS: NodeSortKey[] = ['users_desc', 'users_asc', 'name', 'memory', 'traffic', 'uptime']
export const NODE_FILTER_KEYS: NodeFilterKey[] = ['all', 'online', 'offline', 'connecting', 'disabled']

const PREFS_KEY = 'cabinet.admin.nodes.prefs'

export interface NodePrefs {
  sort: NodeSortKey
  filter: NodeFilterKey
}

const DEFAULT_PREFS: NodePrefs = { sort: 'users_desc', filter: 'all' }

/** Выбранные сортировка и фильтр помним в браузере; без хранилища (приватное окно) работаем с умолчаниями. */
export function loadNodePrefs(): NodePrefs {
  try {
    const raw = localStorage.getItem(PREFS_KEY)
    if (!raw) return DEFAULT_PREFS
    const p = JSON.parse(raw) as Partial<NodePrefs>
    return {
      sort: NODE_SORT_KEYS.includes(p.sort as NodeSortKey) ? (p.sort as NodeSortKey) : DEFAULT_PREFS.sort,
      filter: NODE_FILTER_KEYS.includes(p.filter as NodeFilterKey) ? (p.filter as NodeFilterKey) : DEFAULT_PREFS.filter,
    }
  } catch {
    return DEFAULT_PREFS
  }
}

export function saveNodePrefs(p: NodePrefs): void {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(p))
  } catch {
    // Хранилище недоступно — настройка просто не переживёт перезагрузку.
  }
}

/** Ноды, у которых панель не прислала живые метрики, в сортировках по метрикам уходят вниз. */
function cmpNullableDesc(a: number | null | undefined, b: number | null | undefined): number {
  const av = a ?? -Infinity
  const bv = b ?? -Infinity
  return bv - av
}

const byName = (a: AdminNodeDTO, b: AdminNodeDTO) => a.name.localeCompare(b.name, undefined, { numeric: true })

export function sortNodes(nodes: AdminNodeDTO[], key: NodeSortKey): AdminNodeDTO[] {
  const out = [...nodes]
  out.sort((a, b) => {
    let c = 0
    switch (key) {
      case 'users_desc':
        c = b.users_online - a.users_online
        break
      case 'users_asc':
        c = a.users_online - b.users_online
        break
      case 'name':
        c = byName(a, b)
        break
      case 'memory':
        c = cmpNullableDesc(a.memory_used_pct, b.memory_used_pct)
        break
      case 'traffic':
        c = b.traffic_used_bytes - a.traffic_used_bytes
        break
      case 'uptime':
        c = b.xray_uptime_sec - a.xray_uptime_sec
        break
    }
    return c !== 0 ? c : byName(a, b)
  })
  return out
}

export function filterNodes(nodes: AdminNodeDTO[], filter: NodeFilterKey, query: string): AdminNodeDTO[] {
  const q = query.trim().toLowerCase()
  return nodes.filter((n) => {
    if (filter !== 'all' && n.state !== filter) return false
    if (!q) return true
    return [n.name, n.country_code, n.provider?.name ?? '', n.address].some((s) => s.toLowerCase().includes(q))
  })
}

export function countByFilter(nodes: AdminNodeDTO[]): Record<NodeFilterKey, number> {
  const c: Record<NodeFilterKey, number> = { all: nodes.length, online: 0, offline: 0, connecting: 0, disabled: 0 }
  for (const n of nodes) c[n.state] += 1
  return c
}

export interface NodesSummary {
  users: number
  online: number
  total: number
  rxBytesPerSec: number
  txBytesPerSec: number
}

/** Скорость и людей суммируем только по нодам в сети: у остальных метрики пустые или устаревшие. */
export function summarize(nodes: AdminNodeDTO[]): NodesSummary {
  const s: NodesSummary = { users: 0, online: 0, total: nodes.length, rxBytesPerSec: 0, txBytesPerSec: 0 }
  for (const n of nodes) {
    if (n.state !== 'online') continue
    s.online += 1
    s.users += n.users_online
    s.rxBytesPerSec += n.rx_bytes_per_sec ?? 0
    s.txBytesPerSec += n.tx_bytes_per_sec ?? 0
  }
  return s
}
