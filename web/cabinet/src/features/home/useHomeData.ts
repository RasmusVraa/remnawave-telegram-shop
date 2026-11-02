import { useQuery } from '@tanstack/react-query'

import { i18n } from '@/i18n'
import { api, SUBSCRIPTION_STALE_MS, type SubscriptionResponse } from '@/lib/api'
import { daysUntil, trafficUsagePercent } from '@/lib/utils'
import { daysTone, devicesTone, trafficTone, type ToneLevel } from '@/lib/subscriptionTone'

/**
 * Состояние главной одним словом — от него зависят чип, подсказка, главная
 * кнопка и набор быстрых действий (см. SPEC §6).
 *
 * ok — всё спокойно (и всегда у пробного периода: он короткий, по общей
 *   шкале был бы красным с первого дня);
 * warn — 7–4 дня; crit — 3 дня и меньше;
 * expired — срок вышел; traffic — исчерпан лимит трафика.
 */
export type HomeState = 'ok' | 'warn' | 'crit' | 'expired' | 'traffic'

export function useHomeData() {
  const subQuery = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
  })
  const sub = subQuery.data

  // isLoading, а не isPending: запрос выключен до появления sub, и в этом
  // состоянии isPending остаётся true — скелетон висел бы вечно.
  const devicesQuery = useQuery({
    queryKey: ['devices'],
    queryFn: () => api.devices(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
    enabled: Boolean(sub),
  })
  const usageQuery = useQuery({
    queryKey: ['traffic-usage'],
    queryFn: () => api.trafficUsage(),
    staleTime: 5 * 60_000,
    retry: 1,
    enabled: Boolean(sub),
  })

  const devices = devicesQuery.data
  const isTrial = Boolean(sub?.is_trial)
  const days = sub?.expire_at ? daysUntil(sub.expire_at) : null
  const isExpiredByDate = days !== null && days <= 0
  const trafficLimit = sub?.traffic_limit_gb != null && sub.traffic_limit_gb > 0 ? sub.traffic_limit_gb : null
  const trafficUsed = Math.max(0, sub?.traffic_used_gb ?? 0)
  const isExpiredByTraffic = trafficLimit !== null && trafficUsed >= trafficLimit
  const isActive = !isExpiredByDate && !isExpiredByTraffic
  const effectiveDays = isActive ? days : 0
  const trafficPct = trafficUsagePercent(sub?.traffic_used_gb, sub?.traffic_limit_gb)

  const connected = Math.max(0, devices?.connected ?? 0)
  const deviceLimit = Math.max(sub?.tariff?.device_limit ?? 0, Math.max(0, devices?.device_limit ?? 0))
  const allSlotsTaken = Boolean(devices?.enabled && deviceLimit > 0 && connected >= deviceLimit)

  const dayTone: ToneLevel = isTrial && !isExpiredByDate ? 'calm' : daysTone(effectiveDays)
  let state: HomeState = 'ok'
  if (isExpiredByDate) state = 'expired'
  else if (isExpiredByTraffic) state = 'traffic'
  else if (dayTone === 'danger') state = 'crit'
  else if (dayTone === 'warn') state = 'warn'

  const hwid = sub?.hwid_extra ?? null
  const stopped = state === 'expired' || state === 'traffic'
  // Блок мест: докупка включена, подписка оплачена и активна (бэкенд прячет
  // его у истёкших по дате, истёкших по трафику отсекаем здесь).
  const slotsVisible = Boolean(hwid?.enabled && hwid.ui_visible && !stopped)

  return {
    subQuery,
    devicesQuery,
    usageQuery,
    sub,
    devices,
    usage: usageQuery.data,
    isTrial,
    days,
    effectiveDays,
    isActive,
    isExpiredByDate,
    isExpiredByTraffic,
    state,
    stopped,
    dayTone,
    trafficLimit,
    trafficUsed,
    trafficPct,
    trafficTone: trafficTone(trafficPct),
    connected,
    deviceLimit,
    allSlotsTaken,
    devicesTone: devicesTone(connected, deviceLimit),
    hwid,
    slotsVisible,
    hasLink: Boolean(sub?.subscription_link && String(sub.subscription_link).trim() !== ''),
  }
}

export type HomeData = ReturnType<typeof useHomeData>

export function hasSubscriptionData(sub?: SubscriptionResponse | null): boolean {
  if (!sub) return false
  if (sub.subscription_link && String(sub.subscription_link).trim() !== '') return true
  if (sub.expire_at && String(sub.expire_at).trim() !== '') return true
  return false
}

/**
 * Объём трафика: до сотни — с одним знаком, дальше целым. Разделитель — из
 * языка интерфейса: в русском «1,5», а не «1.5», как дал бы toFixed.
 */
export function formatGbValue(gb: number): string {
  if (gb >= 100) return String(Math.round(gb))
  const v = Math.round(gb * 10) / 10
  return v.toLocaleString(i18n.language === 'en' ? 'en-US' : 'ru-RU', { maximumFractionDigits: 1 })
}
