import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type {
  AdminReview,
  AdminReviewCreateInput,
  AdminReviewUpdateInput,
  AdminReviewsResponse,
} from '@/features/reviews/reviewTypes'

export interface AdminReviewsParams {
  status: string
  q: string
  page: number
  limit: number
}

export function useAdminReviewsList(p: AdminReviewsParams) {
  return useQuery<AdminReviewsResponse>({
    queryKey: ['admin-reviews', p],
    queryFn: () => api.adminReviews({ status: p.status, q: p.q, limit: p.limit, offset: (p.page - 1) * p.limit }),
    placeholderData: keepPreviousData,
  })
}

/** Счётчик «на проверке» для бейджа в меню админки. */
export function useAdminReviewsPending() {
  return useQuery<AdminReviewsResponse>({
    queryKey: ['admin-reviews-pending'],
    queryFn: () => api.adminReviews({ limit: 1 }),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
}

function useInvalidateReviews() {
  const qc = useQueryClient()
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: ['admin-reviews'] }),
      qc.invalidateQueries({ queryKey: ['admin-reviews-pending'] }),
      qc.invalidateQueries({ queryKey: ['admin-review-customer'] }),
    ])
}

export function useUpdateAdminReview() {
  const invalidate = useInvalidateReviews()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: AdminReviewUpdateInput }) => api.adminReviewUpdate(id, body),
    onSuccess: invalidate,
  })
}

export function useDeleteAdminReview() {
  const invalidate = useInvalidateReviews()
  return useMutation({ mutationFn: (id: number) => api.adminReviewDelete(id), onSuccess: invalidate })
}

export function useCreateAdminReview() {
  const invalidate = useInvalidateReviews()
  return useMutation<AdminReview, Error, AdminReviewCreateInput>({
    mutationFn: (body) => api.adminReviewCreate(body),
    onSuccess: invalidate,
  })
}

/** Отзыв клиента для окна «от имени клиента» в карточке пользователя. */
export function useAdminReviewByCustomer(customerId: number | null) {
  return useQuery({
    queryKey: ['admin-review-customer', customerId],
    queryFn: () => api.adminReviewByCustomer(customerId as number),
    enabled: customerId != null && customerId > 0,
  })
}

/** Подсказки клиентов по ID, @нику, Telegram ID и почте. */
export function useAdminReviewCustomers(q: string) {
  return useQuery({
    queryKey: ['admin-review-customers', q],
    queryFn: () => api.adminReviewCustomers(q),
    enabled: q.trim().length >= 1,
    staleTime: 10_000,
  })
}
