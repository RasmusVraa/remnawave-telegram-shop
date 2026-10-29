import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type {
  AdminCustomerDTO,
  AdminDeviceDTO,
  AdminPaymentsDTO,
  AdminPurchaseDTO,
  AdminReferralsDTO,
  AdminUserPanelDTO,
  AdminUsersListDTO,
} from '@/lib/types/admin'

export type {
  AdminCustomerDTO,
  AdminDeviceDTO,
  AdminPurchaseDTO,
  AdminRefereeDTO,
  AdminRWPanelDTO,
  AdminSquadDTO,
  AdminTariffBriefDTO,
} from '@/lib/types/admin'

export type AdminPaymentsResponse = AdminPaymentsDTO
export type AdminReferralsResponse = AdminReferralsDTO
export type AdminUserPanelResponse = AdminUserPanelDTO
export type AdminUsersListResponse = AdminUsersListDTO
export type AdminUsersSearchResponse = { items: AdminCustomerDTO[] }

export function useAdminUsers(params: { scope: string; page: number; limit: number }) {
  return useQuery<AdminUsersListResponse>({
    queryKey: ['admin-users', params.scope, params.page, params.limit],
    queryFn: () => api.adminUsers(params),
    staleTime: 15_000,
  })
}

export function useAdminUserSearch(q: string) {
  return useQuery<AdminUsersSearchResponse>({
    queryKey: ['admin-users-search', q],
    queryFn: () => api.adminUserSearch(q),
    enabled: q.trim().length >= 1,
    staleTime: 10_000,
  })
}

export function useAdminUser(id: number | null) {
  return useQuery<AdminCustomerDTO>({
    queryKey: ['admin-user', id],
    queryFn: () => api.adminUser(id!),
    enabled: id != null && id > 0,
    staleTime: 10_000,
  })
}

export function useAdminUserPayments(id: number | null, page = 1, limit = 20) {
  return useQuery<AdminPaymentsResponse>({
    queryKey: ['admin-user-payments', id, page, limit],
    queryFn: () => api.adminUserPayments(id!, { page, limit }),
    enabled: id != null && id > 0,
    staleTime: 15_000,
  })
}

export function useAdminUserReferrals(id: number | null, page = 1, limit = 20) {
  return useQuery<AdminReferralsResponse>({
    queryKey: ['admin-user-referrals', id, page, limit],
    queryFn: () => api.adminUserReferrals(id!, { page, limit }),
    enabled: id != null && id > 0,
    staleTime: 15_000,
  })
}

function useInvalidateUser(id: number | null) {
  const qc = useQueryClient()
  return () => {
    qc.invalidateQueries({ queryKey: ['admin-user', id] })
    qc.invalidateQueries({ queryKey: ['admin-users'] })
    qc.invalidateQueries({ queryKey: ['admin-user-panel', id] })
  }
}

export function useAdminUserExtend(id: number | null) {
  const invalidate = useInvalidateUser(id)
  return useMutation({
    mutationFn: (days: number) => api.adminUserExtend(id!, days),
    onSuccess: invalidate,
  })
}

export function useAdminUserDisable(id: number | null) {
  const invalidate = useInvalidateUser(id)
  return useMutation({
    mutationFn: () => api.adminUserDisable(id!),
    onSuccess: invalidate,
  })
}

export function useAdminUserEnable(id: number | null) {
  const invalidate = useInvalidateUser(id)
  return useMutation({
    mutationFn: () => api.adminUserEnable(id!),
    onSuccess: invalidate,
  })
}

export function useAdminUserDelete(id: number | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.adminUserDelete(id!),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-users'] }),
  })
}

export function useAdminUserResetTraffic(id: number | null) {
  const invalidate = useInvalidateUser(id)
  return useMutation({
    mutationFn: () => api.adminUserResetTraffic(id!),
    onSuccess: invalidate,
  })
}

export function useAdminUserSetExpire(id: number | null) {
  const invalidate = useInvalidateUser(id)
  return useMutation({
    mutationFn: (expireAt: string) => api.adminUserSetExpire(id!, expireAt),
    onSuccess: invalidate,
  })
}

export function useAdminUserSetHwidLimit(id: number | null) {
  const invalidate = useInvalidateUser(id)
  return useMutation({
    mutationFn: (limit: number) => api.adminUserSetHwidLimit(id!, limit),
    onSuccess: invalidate,
  })
}

function useInvalidatePanel(id: number | null) {
  const qc = useQueryClient()
  return () => {
    qc.invalidateQueries({ queryKey: ['admin-user', id] })
    qc.invalidateQueries({ queryKey: ['admin-users'] })
    qc.invalidateQueries({ queryKey: ['admin-user-panel', id] })
    qc.invalidateQueries({ queryKey: ['admin-user-devices', id] })
  }
}

export function useAdminUserPanel(id: number | null) {
  return useQuery<AdminUserPanelResponse>({
    queryKey: ['admin-user-panel', id],
    queryFn: () => api.adminUserPanel(id!),
    enabled: id != null && id > 0,
    staleTime: 10_000,
  })
}

export function useAdminUserDevices(id: number | null) {
  return useQuery<{ items: AdminDeviceDTO[] }>({
    queryKey: ['admin-user-devices', id],
    queryFn: () => api.adminUserDevices(id!),
    enabled: id != null && id > 0,
    staleTime: 15_000,
  })
}

export function useAdminUserSetSquads(id: number | null) {
  const invalidate = useInvalidatePanel(id)
  return useMutation({
    mutationFn: (uuids: string[]) => api.adminUserSetSquads(id!, uuids),
    onSuccess: invalidate,
  })
}

export function useAdminUserSetTraffic(id: number | null) {
  const invalidate = useInvalidatePanel(id)
  return useMutation({
    mutationFn: (bytes: number) => api.adminUserSetTraffic(id!, bytes),
    onSuccess: invalidate,
  })
}

export function useAdminUserSetStrategy(id: number | null) {
  const invalidate = useInvalidatePanel(id)
  return useMutation({
    mutationFn: (strategy: string) => api.adminUserSetStrategy(id!, strategy),
    onSuccess: invalidate,
  })
}

export function useAdminUserSetDescription(id: number | null) {
  const invalidate = useInvalidatePanel(id)
  return useMutation({
    mutationFn: (desc: string | null) => api.adminUserSetDescription(id!, desc),
    onSuccess: invalidate,
  })
}

export function useAdminUserSetTariff(id: number | null) {
  const invalidate = useInvalidatePanel(id)
  return useMutation({
    mutationFn: (tariffId: number) => api.adminUserSetTariff(id!, tariffId),
    onSuccess: invalidate,
  })
}

export function useAdminUserExtraHwid(id: number | null) {
  const invalidate = useInvalidatePanel(id)
  return useMutation({
    mutationFn: (delta: number) => api.adminUserExtraHwid(id!, delta),
    onSuccess: invalidate,
  })
}

export function useAdminUserSetHwidPin(id: number | null) {
  const invalidate = useInvalidatePanel(id)
  return useMutation({
    mutationFn: (limit: number | null) => api.adminUserSetHwidPin(id!, limit),
    onSuccess: invalidate,
  })
}

export function useAdminUserDeleteDevice(id: number | null) {
  const invalidate = useInvalidatePanel(id)
  return useMutation({
    mutationFn: (hwid: string) => api.adminUserDeleteDevice(id!, hwid),
    onSuccess: invalidate,
  })
}
