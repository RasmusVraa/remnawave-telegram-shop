import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type { AdminTariffSquadsPreviewDTO } from '@/lib/types/admin'

export interface AdminTariffPrice {
  tariff_id: number
  months: number
  amount_rub: number
  amount_stars?: number | null
}

export interface AdminTariffLocation {
  id: number
  name: string
  sort_order: number
  sold_out: boolean
  country_code?: string | null
}

export interface AdminTariff {
  id: number
  slug: string
  name?: string | null
  sort_order: number
  is_active: boolean
  device_limit: number
  traffic_limit_bytes: number
  traffic_limit_reset_strategy: string
  active_internal_squad_uuids: string
  external_squad_uuid?: string | null
  remnawave_tag?: string | null
  tier_level?: number | null
  description?: string | null
  description_detail?: string | null
  prices: AdminTariffPrice[]
  locations?: AdminTariffLocation[]
}

export interface CreateTariffInput {
  slug: string
  name?: string | null
  sort_order?: number
  is_active?: boolean
  device_limit: number
  traffic_limit_bytes: number
  traffic_limit_reset_strategy?: string
  active_internal_squad_uuids?: string
  remnawave_tag?: string | null
  tier_level?: number | null
  description?: string | null
  description_detail?: string | null
  rub: [number, number, number, number]
  stars: [number | null, number | null, number | null, number | null]
  locations?: AdminTariffLocation[]
}

export function useAdminTariffList() {
  return useQuery<AdminTariff[]>({
    queryKey: ['admin-tariffs'],
    queryFn: () => api.adminTariffs(),
  })
}

export function useAdminTariffGet(id: number | null) {
  return useQuery<AdminTariff>({
    queryKey: ['admin-tariff', id],
    queryFn: () => api.adminTariffGet(id!),
    enabled: id != null,
  })
}

export function useAdminTariffCreate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateTariffInput) => api.adminTariffCreate(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-tariffs'] }),
  })
}

export function useAdminTariffUpdate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, fields }: { id: number; fields: Record<string, unknown> }) =>
      api.adminTariffUpdate(id, fields),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-tariffs'] })
      qc.invalidateQueries({ queryKey: ['admin-tariff'] })
    },
  })
}

export function useAdminTariffDelete() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.adminTariffDelete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-tariffs'] }),
  })
}

/**
 * Состав сквадов тарифа + прогресс применения к действующим подписчикам.
 *
 * Опрос включается только пока прогон идёт: он живёт в памяти процесса и
 * событий наружу не шлёт, но и держать 30 запросов в минуту, пока админ просто
 * смотрит на вкладку, незачем — у админских ручек общий лимит 120/мин.
 */
export function useAdminTariffSquads(id: number | null, poll = false) {
  return useQuery<AdminTariffSquadsPreviewDTO>({
    queryKey: ['admin-tariff-squads', id],
    queryFn: () => api.adminTariffSquads(id!),
    enabled: id != null,
    refetchInterval: (query) => (poll && query.state.data?.run?.status === 'running' ? 2000 : false),
  })
}

export function useAdminTariffSquadsApply() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, add, remove }: { id: number; add: string[]; remove: string[] }) =>
      api.adminTariffSquadsApply(id, { add, remove }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['admin-tariff-squads', vars.id] })
    },
  })
}

export interface AdminSquadItem {
  uuid: string
  name: string
}

export function useAdminSquads() {
  return useQuery<{ items: AdminSquadItem[] }>({
    queryKey: ['admin-squads'],
    queryFn: () => api.adminSquads(),
    staleTime: 60_000,
  })
}

const STRATEGIES = ['no_reset', 'DAY', 'WEEK', 'MONTH', 'MONTH_ROLLING', 'NO_RESET'] as const
export { STRATEGIES }
