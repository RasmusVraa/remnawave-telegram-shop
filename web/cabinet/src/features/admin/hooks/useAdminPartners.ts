import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type {
  AdminPartnerDetailDTO,
  AdminPartnerPayoutDTO,
  AdminPartnerPendingDTO,
  AdminPartnerTermsInput,
} from '@/lib/types/admin'

/**
 * Хуки админки партнёрской программы.
 *
 * Любое изменение сбрасывает и списки, и счётчик дел: бейдж в меню считает
 * заявки вместе с выплатами, и после одобрения он должен уменьшиться сразу, а
 * не после перезагрузки страницы.
 */
const PARTNERS_KEY = ['admin-partners']
const PAYOUTS_KEY = ['admin-partner-payouts']
export const PARTNERS_PENDING_KEY = ['admin-partners-pending']

function useInvalidatePartners() {
  const qc = useQueryClient()
  return async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: PARTNERS_KEY }),
      qc.invalidateQueries({ queryKey: PAYOUTS_KEY }),
      qc.invalidateQueries({ queryKey: PARTNERS_PENDING_KEY }),
      qc.invalidateQueries({ queryKey: ['admin-partner-detail'] }),
      qc.invalidateQueries({ queryKey: ['admin-partner-tab'] }),
    ])
  }
}

/**
 * Размер страницы списков раздела.
 *
 * Раньше здесь стоял фиксированный limit без продолжения: сто первый партнёр
 * в список уже не попадал, и понять это по экрану было нельзя — список просто
 * заканчивался.
 */
const LIST_PAGE_SIZE = 50

function useAdminPartnerList<T>(
  key: string,
  scope: string,
  fetchPage: (params: { limit: number; offset: number }) => Promise<{ items: T[]; total: number }>,
) {
  return useInfiniteQuery({
    queryKey: [key, scope],
    queryFn: ({ pageParam }) => fetchPage({ limit: LIST_PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, pages) => {
      const loaded = pages.reduce((n, p) => n + p.items.length, 0)
      return loaded < lastPage.total ? loaded : undefined
    },
    staleTime: 15_000,
  })
}

/**
 * Список партнёров. statuses — какие статусы показывать; пусто значит «все».
 *
 * Отбор уходит на сервер: выдача постраничная, и фильтрация уже загруженных
 * строк теряла бы совпадения со следующих страниц.
 */
export function useAdminPartners(statuses?: string[]) {
  const status = statuses?.length ? statuses.join(',') : undefined
  return useAdminPartnerList(PARTNERS_KEY[0], status ?? 'all', (params) =>
    api.adminPartners({ status, ...params }),
  )
}

export function useAdminPartnerPending() {
  return useQuery<AdminPartnerPendingDTO>({
    queryKey: PARTNERS_PENDING_KEY,
    queryFn: () => api.adminPartnerPending(),
    staleTime: 30_000,
  })
}

export function useAdminPartnerDetail(id: number | null) {
  return useQuery<AdminPartnerDetailDTO>({
    queryKey: ['admin-partner-detail', id],
    queryFn: () => api.adminPartnerDetail(id as number),
    enabled: id != null,
  })
}

/**
 * Постраничная вкладка карточки партнёра.
 *
 * Листаем через offset, а не наращиваем limit: сервер обрезает limit сотней, и
 * «показать ещё» после сотой строки перезапрашивало бы ту же страницу, а
 * кнопка не исчезала бы никогда.
 */
const TAB_PAGE_SIZE = 25

function usePartnerTab<T>(
  tab: string,
  id: number | null,
  enabled: boolean,
  fetchPage: (id: number, params: { limit: number; offset: number }) => Promise<{ items: T[]; total: number }>,
) {
  return useInfiniteQuery({
    queryKey: ['admin-partner-tab', tab, id],
    queryFn: ({ pageParam }) => fetchPage(id as number, { limit: TAB_PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, pages) => {
      const loaded = pages.reduce((n, p) => n + p.items.length, 0)
      return loaded < lastPage.total ? loaded : undefined
    },
    enabled: id != null && enabled,
    staleTime: 15_000,
  })
}

export function useAdminPartnerCustomers(id: number | null, enabled: boolean) {
  return usePartnerTab('customers', id, enabled, api.adminPartnerCustomers)
}

export function useAdminPartnerOperations(id: number | null, enabled: boolean) {
  return usePartnerTab('operations', id, enabled, api.adminPartnerOperations)
}

export function useAdminPartnerPayoutHistory(id: number | null, enabled: boolean) {
  return usePartnerTab('payouts', id, enabled, api.adminPartnerPayoutHistory)
}

export function useAdminPartnerPayouts(status?: string) {
  return useAdminPartnerList<AdminPartnerPayoutDTO>(PAYOUTS_KEY[0], status ?? 'all', (params) =>
    api.adminPartnerPayouts({ status, ...params }),
  )
}

export function useAdminPartnerApprove() {
  const invalidate = useInvalidatePartners()
  return useMutation({
    meta: { haptics: true },
    mutationFn: ({ id, terms }: { id: number; terms: AdminPartnerTermsInput }) =>
      api.adminPartnerApprove(id, terms),
    onSuccess: invalidate,
  })
}

export function useAdminPartnerReject() {
  const invalidate = useInvalidatePartners()
  return useMutation({
    meta: { haptics: true },
    mutationFn: ({ id, comment }: { id: number; comment: string }) => api.adminPartnerReject(id, comment),
    onSuccess: invalidate,
  })
}

export function useAdminPartnerSetStatus() {
  const invalidate = useInvalidatePartners()
  return useMutation({
    meta: { haptics: true },
    mutationFn: ({ id, status, comment }: { id: number; status: string; comment?: string }) =>
      api.adminPartnerSetStatus(id, status, comment ?? ''),
    onSuccess: invalidate,
  })
}

export function useAdminPartnerUpdateTerms() {
  const invalidate = useInvalidatePartners()
  return useMutation({
    meta: { haptics: true },
    mutationFn: ({ id, terms }: { id: number; terms: AdminPartnerTermsInput }) =>
      api.adminPartnerUpdateTerms(id, terms),
    onSuccess: invalidate,
  })
}

export function useAdminPartnerAdjust() {
  const invalidate = useInvalidatePartners()
  return useMutation({
    meta: { haptics: true },
    mutationFn: ({ id, amount, comment }: { id: number; amount: number; comment: string }) =>
      api.adminPartnerAdjust(id, amount, comment),
    onSuccess: invalidate,
  })
}

export function useAdminPartnerGrant() {
  const invalidate = useInvalidatePartners()
  return useMutation({
    meta: { haptics: true },
    mutationFn: (body: {
      customer_id?: number
      telegram_id?: number
      first_percent?: number | null
      renewal_percent?: number | null
      comment?: string
    }) => api.adminPartnerGrant(body),
    onSuccess: invalidate,
  })
}

export function useAdminPartnerPayoutAction() {
  const invalidate = useInvalidatePartners()
  return useMutation({
    meta: { haptics: true },
    mutationFn: ({
      id,
      action,
      externalRef,
      comment,
    }: {
      id: number
      action: 'approve' | 'paid' | 'reject'
      externalRef?: string
      comment?: string
    }) => api.adminPartnerPayoutAction(id, action, { external_ref: externalRef, comment }),
    onSuccess: invalidate,
  })
}
