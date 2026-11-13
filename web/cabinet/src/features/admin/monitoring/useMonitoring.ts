import { keepPreviousData, useQuery } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type { AdminAttentionItemDTO, AdminMonitoringPeriod, AdminMonitoringSummaryDTO } from '@/lib/types/admin'

export const ATTENTION_KEY = ['admin-attention']

/**
 * Инциденты «Мониторинга». Меню опрашивает их раз в 30 с ради бейджа, страница
 * «Мониторинг» — чаще: она для того и открыта.
 */
export function useAdminAttention(refetchMs = 30_000) {
  return useQuery<{ items: AdminAttentionItemDTO[] }>({
    queryKey: ATTENTION_KEY,
    queryFn: () => api.adminAttention(),
    staleTime: 5_000,
    refetchInterval: refetchMs,
    refetchIntervalInBackground: false,
    retry: 1,
  })
}

export function useAdminMonitoringSummary(period: AdminMonitoringPeriod) {
  return useQuery<AdminMonitoringSummaryDTO>({
    queryKey: ['admin-monitoring-summary', period],
    queryFn: () => api.adminMonitoringSummary(period),
    staleTime: 5_000,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
    placeholderData: keepPreviousData,
    retry: 1,
  })
}
