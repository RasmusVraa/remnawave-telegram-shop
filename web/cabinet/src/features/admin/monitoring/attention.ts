import { useCallback, useEffect, useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { TFunction } from 'i18next'

import { api } from '@/lib/api'
import type { AdminAttentionDismissedDTO, AdminAttentionItemDTO } from '@/lib/types/admin'

/** Один пункт «Требует внимания»: и то, что горит сейчас, и дело на дни. */
export interface AttentionEntry {
  /** Стабильный ключ: по нему на сервере помнится «скрыто». */
  id: string
  label: string
  to?: string
  severity: 'crit' | 'warn'
  count: number
  /** С какого момента горит (только инциденты). */
  since?: string
}

const LINKS: Record<string, string> = {
  node_down: '/admin/nodes',
  panel_down: '/admin/nodes',
}

/** Подпись инцидента. */
export function incidentLabel(item: AdminAttentionItemDTO, t: TFunction): string {
  return t(`admin.monitoring.incident.${item.key}`, {
    count: item.count,
    name: String(item.params?.name ?? ''),
    defaultValue: item.key,
  })
}

/** Куда ведёт инцидент: нода — на «Ноды», остальное — в «Мониторинг». */
export function incidentLink(item: AdminAttentionItemDTO): string {
  return LINKS[item.key] ?? '/admin/monitoring'
}

/** Инциденты бэка — в общий вид. Нода различается по имени: упала другая — другой пункт. */
export function incidentEntries(items: AdminAttentionItemDTO[] | undefined, t: TFunction): AttentionEntry[] {
  return (items ?? []).map((it) => ({
    id: it.key === 'node_down' ? `node_down:${String(it.params?.name ?? '')}` : it.key,
    label: incidentLabel(it, t),
    to: incidentLink(it),
    severity: it.severity,
    count: it.count || 1,
    since: it.since,
  }))
}

/** Ключи, которые считает «Мониторинг»: дела на дни он не показывает и чужие отметки не трогает. */
export function isIncidentId(id: string): boolean {
  return /^(node_down|panel_down|support_)/.test(id)
}

export const DISMISSED_KEY = ['admin-attention-dismissed']

/**
 * Скрыто ли: пункт прочитан и с тех пор не стало хуже. Хуже — это счётчик
 * вырос, срочность поднялась до crit или событие случилось заново (since новее).
 */
export function isDismissed(entry: AttentionEntry, rec: AdminAttentionDismissedDTO | undefined): boolean {
  if (!rec) return false
  if (entry.count > rec.count) return false
  if (entry.severity === 'crit' && rec.severity !== 'crit') return false
  if (entry.since && rec.since && new Date(entry.since).getTime() > new Date(rec.since).getTime())
    return false
  return true
}

/**
 * Отметки «скрыто» этого админа. `entries` — полный список пунктов страницы,
 * `ready` — он уже загружен (иначе пустой список принять за «всё прошло» нельзя),
 * `owns` — какие ключи считает эта страница: остальные чистить не её дело.
 */
export function useAttentionDismissals(
  entries: AttentionEntry[],
  ready: boolean,
  owns: (id: string) => boolean = () => true,
) {
  const qc = useQueryClient()
  const query = useQuery<{ items: AdminAttentionDismissedDTO[] }>({
    queryKey: DISMISSED_KEY,
    queryFn: () => api.adminAttentionDismissed(),
    staleTime: 30_000,
    retry: 1,
  })
  const records = useMemo(() => new Map((query.data?.items ?? []).map((r) => [r.key, r])), [query.data])

  const visible = useMemo(() => entries.filter((e) => !isDismissed(e, records.get(e.id))), [entries, records])
  const hidden = entries.length - visible.length

  const setCache = useCallback(
    (fn: (items: AdminAttentionDismissedDTO[]) => AdminAttentionDismissedDTO[]) =>
      qc.setQueryData<{ items: AdminAttentionDismissedDTO[] }>(DISMISSED_KEY, (old) => ({
        items: fn(old?.items ?? []),
      })),
    [qc],
  )

  const dismissMut = useMutation({
    mutationFn: (list: AdminAttentionDismissedDTO[]) => api.adminAttentionDismiss(list),
    onMutate: (list) => {
      const keys = new Set(list.map((r) => r.key))
      setCache((items) => [...items.filter((r) => !keys.has(r.key)), ...list])
    },
    onError: () => void qc.invalidateQueries({ queryKey: DISMISSED_KEY }),
  })
  const restoreMut = useMutation({
    mutationFn: (key?: string) => api.adminAttentionRestore(key),
    onMutate: (key) => setCache((items) => (key ? items.filter((r) => r.key !== key) : [])),
    onError: () => void qc.invalidateQueries({ queryKey: DISMISSED_KEY }),
  })

  const toRecord = (e: AttentionEntry): AdminAttentionDismissedDTO => ({
    key: e.id,
    count: e.count,
    severity: e.severity,
    since: e.since,
  })

  // Пункт ушёл (нода поднялась, счёт оплачен) — отметка больше не нужна, иначе
  // новый такой же пункт через месяц родился бы уже «прочитанным».
  const staleKeys = useMemo(() => {
    if (!ready || !query.data) return []
    const live = new Set(entries.map((e) => e.id))
    return (query.data.items ?? []).filter((r) => owns(r.key) && !live.has(r.key)).map((r) => r.key)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, query.data, entries])
  const staleSig = staleKeys.join('|')
  useEffect(() => {
    if (!staleSig) return
    staleSig.split('|').forEach((k) => restoreMut.mutate(k))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [staleSig])

  return {
    visible,
    hidden,
    loading: query.isLoading,
    pending: dismissMut.isPending || restoreMut.isPending,
    dismiss: (e: AttentionEntry) => dismissMut.mutate([toRecord(e)]),
    dismissAll: () => dismissMut.mutate(visible.map(toRecord)),
    restoreAll: () => restoreMut.mutate(undefined),
  }
}
