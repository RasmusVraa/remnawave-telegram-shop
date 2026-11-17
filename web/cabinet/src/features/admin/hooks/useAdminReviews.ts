import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'

export type AdminReviewDTO = {
  id: number
  account_id: number
  customer_id?: number | null
  rating: number
  body: string
  visibility: string
  display_name: string
  avatar_url?: string
  author_name?: string | null
  author_avatar_url?: string | null
  client_since?: string | null
  tariff_name: string
  created_at: string
  updated_at: string
  edited_at?: string | null
  has_original: boolean
}

export type AdminReviewsListResponse = {
  items: AdminReviewDTO[]
  total: number
  limit: number
  offset: number
}

const key = (limit: number, offset: number) => ['admin-reviews', limit, offset] as const

export function useAdminReviews(limit = 20, offset = 0) {
  return useQuery({
    queryKey: key(limit, offset),
    queryFn: () => api.adminReviews({ limit, offset }),
  })
}

export function useAdminReviewUpdate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: {
      id: number
      rating: number
      body: string
      visibility: string
      display_name?: string
      avatar_url?: string
      tariff_name?: string
    }) => api.adminReviewUpdate(args.id, args),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-reviews'] })
      void qc.invalidateQueries({ queryKey: ['public-reviews'] })
      void qc.invalidateQueries({ queryKey: ['public-reviews-featured'] })
    },
  })
}

export function useAdminReviewDelete() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.adminReviewDelete(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-reviews'] })
      void qc.invalidateQueries({ queryKey: ['public-reviews'] })
      void qc.invalidateQueries({ queryKey: ['public-reviews-featured'] })
    },
  })
}
