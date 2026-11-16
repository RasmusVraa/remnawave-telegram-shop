import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type {
  AdminPromoCodeDTO,
  AdminPromoGetDTO,
  AdminPromoListDTO,
  AdminPromoStatusFilter,
  AdminPromoTypeFilter,
} from '@/lib/types/admin'
import { promoListBucket } from '../utils/promoStatus'
import type { SortState } from '../components/SortableTh'
import type { AdminPromoSort } from '@/lib/types/admin'

export type AdminPromoCode = AdminPromoCodeDTO

export interface CreatePromoInput {
  code: string
  type: string
  note?: string | null
  subscription_days?: number | null
  trial_days?: number | null
  extra_hwid_delta?: number | null
  discount_percent?: number | null
  discount_ttl_hours?: number | null
  max_uses?: number | null
  valid_until?: string | null
  first_purchase_only?: boolean
  tariff_id?: number | null
  discount_max_subscription_payments_per_customer?: number
}

export interface AdminPromoListParams {
  page: number
  limit?: number
  status: AdminPromoStatusFilter
  type: AdminPromoTypeFilter | null
  q: string
  /** null — по умолчанию (сначала новые); key — new | uses | expiring | code. */
  sort: SortState | null
}

export function useAdminPromoList(params: AdminPromoListParams) {
  const { page, limit = 20, status, type, q, sort } = params
  return useQuery<AdminPromoListDTO>({
    queryKey: ['admin-promos', { page, limit, status, type, q, sort: sort?.key ?? null, dir: sort?.dir ?? null }],
    queryFn: () =>
      api.adminPromos({ page, limit, status, type, q, sort: (sort?.key as AdminPromoSort | undefined) ?? undefined, dir: sort?.dir }),
    // Смена вкладки, фильтра или страницы не мигает пустым списком.
    placeholderData: keepPreviousData,
  })
}

export function useAdminPromoGet(id: number | null) {
  return useQuery<AdminPromoGetDTO>({
    queryKey: ['admin-promo', id],
    queryFn: () => api.adminPromoGet(id!),
    enabled: id != null,
  })
}

export function useAdminPromoCreate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreatePromoInput) => api.adminPromoCreate(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-promos'] }),
  })
}

export function useAdminPromoUpdate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, fields }: { id: number; fields: Record<string, unknown> }) =>
      api.adminPromoUpdate(id, fields),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-promos'] })
      qc.invalidateQueries({ queryKey: ['admin-promo'] })
    },
  })
}

/**
 * Тумблер «Активен» в строке списка. Строка остаётся на месте с новым статусом,
 * даже если по текущей вкладке она больше не подходит: иначе она пропадала бы
 * из-под пальца. Счётчики вкладок сдвигаются на месте, а свежий список придёт
 * при следующей смене вкладки, фильтра, поиска или страницы.
 */
export function useAdminPromoToggle() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ promo, active }: { promo: AdminPromoCode; active: boolean }) =>
      api.adminPromoUpdate(promo.id, { active }),
    onSuccess: (updated, { promo }) => {
      const before = promoListBucket(promo)
      const after = promoListBucket(updated)
      for (const [key, old] of qc.getQueriesData<AdminPromoListDTO>({ queryKey: ['admin-promos'] })) {
        if (!old) continue
        const items = old.items.map((p) => (p.id === updated.id ? updated : p))
        // Счётчики двигаем только там, куда код попадает по типу и поиску: в кэше
        // вкладки «Триал» переключение скидки ничего не меняет.
        const f = key[1] as Partial<AdminPromoListParams> | undefined
        const q = f?.q?.trim().toLowerCase() ?? ''
        const matches =
          (!f?.type || f.type === updated.type) &&
          (!q || updated.code.toLowerCase().includes(q) || (updated.note ?? '').toLowerCase().includes(q))
        if (!old.counts || before === after || !matches) {
          qc.setQueryData(key, { ...old, items })
          continue
        }
        const status = { ...old.counts.status }
        status[before] = Math.max(0, status[before] - 1)
        status[after] = status[after] + 1
        qc.setQueryData(key, { ...old, items, counts: { ...old.counts, status } })
      }
      qc.invalidateQueries({ queryKey: ['admin-promos'], refetchType: 'none' })
      qc.invalidateQueries({ queryKey: ['admin-promo', updated.id] })
    },
  })
}

export function useAdminPromoDelete() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.adminPromoDelete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-promos'] }),
  })
}

/**
 * «Кто использовал» — подгрузка страницами по мере прокрутки (без переключателя
 * страниц). Страница — 20 строк, как у бэкенда по умолчанию.
 */
export function useAdminPromoRedemptionsInfinite(promoId: number | null, limit = 20) {
  return useInfiniteQuery({
    queryKey: ['admin-promo-redemptions', promoId, 'infinite', limit],
    queryFn: ({ pageParam }) => api.adminPromoRedemptions(promoId!, { page: pageParam, limit }),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page * last.limit < last.total ? last.page + 1 : undefined),
    enabled: promoId != null,
  })
}
