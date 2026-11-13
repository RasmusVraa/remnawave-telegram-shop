import { useQuery } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type { AdminPaymentDetailDTO, AdminPaymentsListDTO } from '@/lib/types/admin'

export type { AdminPaymentListItemDTO, AdminPaymentDetailDTO } from '@/lib/types/admin'

export function useAdminPaymentsList(params: {
  status: string
  q: string
  page: number
  limit: number
  sort?: string
  dir?: string
}) {
  return useQuery<AdminPaymentsListDTO>({
    queryKey: ['admin-payments', params.status, params.q, params.page, params.limit, params.sort ?? '', params.dir ?? ''],
    queryFn: () => api.adminPayments(params),
    staleTime: 15_000,
  })
}

export function useAdminPayment(id: number | null) {
  return useQuery<AdminPaymentDetailDTO>({
    queryKey: ['admin-payment', id],
    queryFn: () => api.adminPayment(id!),
    enabled: id != null && id > 0,
    staleTime: 10_000,
  })
}
