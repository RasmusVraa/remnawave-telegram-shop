import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ClipboardList, Check, RefreshCw, Smartphone, Trash2 } from 'lucide-react'
import type { TFunction } from 'i18next'

import { AppLayout } from '@/components/AppLayout'
import { ConfirmModal } from '@/components/ConfirmModal'
import { DevicePlatformIcon } from '@/components/DevicePlatformIcon'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { SubscriptionActions } from '@/components/SubscriptionActions'
import { SubscriptionExpireAtBlock } from '@/components/SubscriptionExpireAtBlock'
import { TrafficUsageBar } from '@/components/TrafficUsageBar'
import { LoyaltyCompactCard } from '@/features/loyalty/LoyaltyProgramPage'
import { AddDeviceSlot, ConnectExtraDeviceCard, ConnectInviteModal } from '@/features/subscription/ConnectExtraDevice'
import { SubscriptionExtraDevices } from '@/features/subscription/SubscriptionExtraDevices'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/components/ui/toast'
import { api, SUBSCRIPTION_STALE_MS } from '@/lib/api'
import { daysUntil, cn } from '@/lib/utils'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'

export default function SubscriptionPage() {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const toast = useToast()
  // CABINET_SUBSCRIPTION_SHOW_LOYALTY: плашку уровня на этой странице админ
  // включает сам, по умолчанию её нет. Пока bootstrap не приехал, не рисуем —
  // иначе карточка мелькала бы при каждой загрузке у тех, кто её выключил.
  const { data: bootstrap } = useAuthBootstrap()
  const loyaltyVisible = bootstrap?.subscription_loyalty_visible === true
  const [refreshDone, setRefreshDone] = useState(false)
  const [isRefreshing, setIsRefreshing] = useState(false)

  // staleTime вместо принудительного refetch на каждом монтировании — разбор
  // в DashboardPage. Кнопка «Обновить» рядом с заголовком остаётся для явного
  // запроса свежих данных.
  const { data: sub, isLoading, error, refetch } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
  })
  const { data: devices, refetch: refetchDevices, isLoading: devicesLoading } = useQuery({
    queryKey: ['devices'],
    queryFn: () => api.devices(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
    enabled: Boolean(sub),
  })
  const deleteDevice = useMutation({
    mutationFn: (hwid: string) => api.deleteDevice(hwid),
    onSuccess: async () => {
      await refetchDevices()
    },
    // Удаление устройства молча ничего не делало при отказе сервера.
    onError: () => toast.error(t('errors.unknown')),
  })

  const [deleteConfirmHwid, setDeleteConfirmHwid] = useState<string | null>(null)
  // Модалку приглашения открывают из двух мест: карточка-действие и пустой
  // слот в списке устройств — поэтому состояние живёт на странице.
  const [connectOpen, setConnectOpen] = useState(false)

  const days = sub?.expire_at ? daysUntil(sub.expire_at) : null
  const isExpiredByDate = sub?.expire_at != null && sub.expire_at !== '' && days !== null && days <= 0
  const isExpiredByTraffic = Boolean(
    sub?.traffic_limit_gb != null &&
    sub.traffic_limit_gb > 0 &&
    (sub.traffic_used_gb ?? 0) >= sub.traffic_limit_gb,
  )
  const isExpired = isExpiredByDate || isExpiredByTraffic
  const isActive = !isExpired
  const hasLink = Boolean(sub?.subscription_link && String(sub.subscription_link).trim() !== '')
  const hasExpire = Boolean(sub?.expire_at && String(sub.expire_at).trim() !== '')
  const hasRecord = hasLink || hasExpire
  const connectedDevices = Math.max(0, devices?.connected ?? 0)
  const deviceLimit = Math.max(
    sub?.tariff?.device_limit ?? 0,
    Math.max(0, devices?.device_limit ?? 0),
  )
  // Слот «добавить устройство» показываем, только когда его есть куда занять:
  // на исчерпанном лимите он звал бы в тупик, там уместнее «докупить».
  const canAddDevice = Boolean(
    devices?.enabled &&
      !devicesLoading &&
      !isExpired &&
      hasLink &&
      (deviceLimit === 0 || connectedDevices < deviceLimit),
  )

  async function handleRefresh() {
    if (isRefreshing) return
    setRefreshDone(false)
    setIsRefreshing(true)
    try {
      const [subResult] = await Promise.all([refetch(), refetchDevices()])
      if (subResult.isSuccess) {
        setRefreshDone(true)
        setTimeout(() => setRefreshDone(false), 2000)
      }
    } finally {
      setIsRefreshing(false)
    }
  }

  return (
    <AppLayout>
      <PageReveal className="space-y-4">
        <RevealItem className="flex items-center justify-between gap-3">
          <h1 className="text-2xl font-semibold">{t('subscriptionPage.title')}</h1>
          {!isLoading && hasRecord && (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => void handleRefresh()}
              disabled={isRefreshing}
              title={refreshDone ? t('subscriptionPage.refreshDone') : t('subscriptionPage.refresh')}
              aria-label={refreshDone ? t('subscriptionPage.refreshDone') : t('subscriptionPage.refresh')}
            >
              {refreshDone ? (
                <Check size={15} className="animate-fade-in text-emerald-500 dark:text-emerald-400" />
              ) : (
                <RefreshCw size={15} className={cn(isRefreshing && 'animate-spin')} />
              )}
            </Button>
          )}
        </RevealItem>

        {isLoading ? (
          <RevealItem>
            <SubscriptionSkeleton />
          </RevealItem>
        ) : error ? (
          <RevealItem>
            <p className="text-sm text-destructive">{t('errors.unknown')}</p>
          </RevealItem>
        ) : !hasRecord ? (
          <RevealItem>
          <Card className="max-w-lg mx-auto">
            <CardContent
             
              className="flex flex-col items-center gap-4 px-8 py-12 text-center"
            >
              <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-muted">
                <ClipboardList size={28} className="text-muted-foreground" />
              </div>
              <div>
                <p className="text-lg font-semibold">{t('subscriptionPage.emptyTitle')}</p>
                <p className="mt-1 text-sm text-muted-foreground">{t('subscriptionPage.emptySubtitle')}</p>
              </div>
              <Button asChild className="w-full max-w-xs">
                <Link to="/tariffs">{t('subscriptionPage.buySubscription')}</Link>
              </Button>
            </CardContent>
          </Card>
          </RevealItem>
        ) : (
          <>
            <RevealItem>
            <Card className="subscription-feature-card">
              <CardContent className="space-y-4 p-4 sm:p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <span className="inline-flex items-center rounded-full border border-primary/35 bg-primary/10 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-primary">
                      {t('subscriptionPage.tariff')}
                    </span>
                    <p className="mt-2 font-heading text-2xl font-bold">{subscriptionTariffLabel(sub, t)}</p>
                  </div>
                  <StatusBadge isActive={isActive} isExpired={isExpired} hasSubscription={Boolean(sub?.expire_at)} />
                </div>

                {/* Показатели полосами: трафик и устройства рядом, одинаковой формы. */}
                <div className="space-y-3">
                  <TrafficUsageBar
                    usedGb={sub?.traffic_used_gb}
                    limitGb={sub?.traffic_limit_gb}
                    usageTitle={t('dashboard.trafficUsage')}
                    gigabytesLabel={t('dashboard.gigabytes')}
                    unlimitedLabel={t('subscriptionPage.unlimited')}
                  />

                  <DevicesUsageBar
                    used={connectedDevices}
                    limit={deviceLimit}
                    loading={devicesLoading}
                    title={t('subscriptionPage.devices')}
                    allTakenHint={t('subscriptionPage.devicesAllTaken')}
                    unlimitedLabel={t('subscriptionPage.unlimited')}
                  />
                </div>

                {hasExpire && (
                  <SubscriptionExpireAtBlock
                    expireAt={sub?.expire_at}
                    lang={lang}
                    days={days}
                    isActive={isActive}
                  />
                )}

                <SubscriptionActions
                  days={isExpired ? 0 : days}
                  devicesUsed={connectedDevices}
                  devicesLimit={deviceLimit}
                  connectId="cabinet-onboarding-connect-target"
                />
              </CardContent>
            </Card>
            </RevealItem>

            {sub?.subscription_link && (
              <RevealItem>
                {/*
                  Раньше здесь лежала карточка «Ссылка подписки» с открытым URL.
                  Пользователи её видели, но не понимали, что именно ею
                  подключается второй телефон или компьютер, — и шли в поддержку
                  с вопросом «как добавить ещё устройство». Теперь на этом месте
                  названное действие, а ссылка живёт внутри, под раскрывашкой
                  «Настроить вручную».
                */}
                <ConnectExtraDeviceCard onOpen={() => setConnectOpen(true)} inactive={isExpired} />
              </RevealItem>
            )}

            {loyaltyVisible && (
              <RevealItem>
                <div id="cabinet-loyalty">
                  <LoyaltyCompactCard />
                </div>
              </RevealItem>
            )}

            {sub?.hwid_extra?.ui_visible && sub.hwid_extra.enabled && (
              <RevealItem>
                <SubscriptionExtraDevices hwid={sub.hwid_extra} inactive={isExpired} onUpdated={() => void refetch()} />
              </RevealItem>
            )}

            <RevealItem>
            {/*
              Строки-устройства вместо обведённых блоков с кнопкой-словом:
              иконка в скруглённом квадрате, название и платформа, удаление —
              иконкой справа. Счётчик уехал в шапку карточки, отдельная строка
              «Подключено: N / M» дублировала полосу устройств в главной карточке.
            */}
            <Card className="cabinet-elevated-card">
              <CardContent className="p-4">
                <div className="mb-2 flex items-center justify-between gap-2 px-1">
                  <p className="flex items-center gap-2 text-sm font-semibold">
                    <Smartphone size={15} className="text-primary" />
                    {t('subscriptionPage.myDevices')}
                  </p>
                  {devices?.enabled && (
                    <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                      {devices.connected ?? 0} /{' '}
                      {devices.device_limit > 0
                        ? devices.device_limit
                        : t('subscriptionPage.unlimited')}
                    </span>
                  )}
                </div>

                {!devices?.enabled ? (
                  <p className="px-1 text-sm text-muted-foreground">
                    {t('subscriptionPage.devicesUnavailable')}
                  </p>
                ) : devicesLoading ? (
                  <DeviceRowsSkeleton n={2} />
                ) : !devices.devices?.length ? (
                  <p className="px-1 py-4 text-center text-sm text-muted-foreground">
                    {t('subscriptionPage.noDevices')}
                  </p>
                ) : (
                  <ul className="space-y-1">
                    {devices.devices.map((d) => {
                      const title = d.device_model || d.platform || d.hwid
                      const subtitle = [d.platform, d.os_version].filter(Boolean).join(' · ')
                      return (
                        <li
                          key={d.hwid}
                          className="cabinet-row flex items-center justify-between gap-3 rounded-xl px-3 py-2"
                        >
                          <div className="flex min-w-0 items-center gap-3">
                            {/* Иконка по платформе: ноутбук для macOS/Windows, телефон для мобильных. */}
                            <span className="cabinet-icon-box inline-flex size-9 shrink-0 items-center justify-center rounded-lg">
                              <DevicePlatformIcon
                                platform={d.platform ?? d.device_model}
                                className="size-4"
                              />
                            </span>
                            <div className="min-w-0">
                              <p className="truncate text-sm font-medium">{title}</p>
                              <p className="truncate text-xs text-muted-foreground">
                                {subtitle || d.hwid}
                              </p>
                            </div>
                          </div>
                          <button
                            type="button"
                            disabled={deleteDevice.isPending}
                            onClick={() => setDeleteConfirmHwid(d.hwid)}
                            aria-label={t('subscriptionPage.deleteDevice')}
                            title={t('subscriptionPage.deleteDevice')}
                            className="shrink-0 rounded-lg p-2 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive disabled:pointer-events-none disabled:opacity-50"
                          >
                            <Trash2 size={15} />
                          </button>
                        </li>
                      )
                    })}
                  </ul>
                )}

                {canAddDevice && (
                  <AddDeviceSlot onOpen={() => setConnectOpen(true)} />
                )}
              </CardContent>
            </Card>
            </RevealItem>

            {/* Модалка смонтирована постоянно и сама решает, когда исчезнуть:
                при снятии по connectOpen анимация ухода не успевала бы
                проиграться. */}
            {sub?.subscription_link && (
              <ConnectInviteModal
                open={connectOpen}
                subscriptionLink={sub.subscription_link}
                onClose={() => setConnectOpen(false)}
              />
            )}

            <ConfirmModal
              open={Boolean(deleteConfirmHwid)}
              tone="danger"
              icon={Trash2}
              title={t('subscriptionPage.deleteDeviceConfirm')}
              description={t('subscriptionPage.deleteDeviceConfirmHint')}
              confirmLabel={t('subscriptionPage.deleteDevice')}
              loading={deleteDevice.isPending}
              onClose={() => setDeleteConfirmHwid(null)}
              onConfirm={() => {
                const hwid = deleteConfirmHwid
                if (!hwid) return
                setDeleteConfirmHwid(null)
                deleteDevice.mutate(hwid)
              }}
            />
          </>
        )}
      </PageReveal>
    </AppLayout>
  )
}

function subscriptionTariffLabel(sub: Awaited<ReturnType<typeof api.subscription>> | null | undefined, t: TFunction): string {
  if (!sub) return t('dashboard.basicPlan')
  if (sub.is_trial) return t('dashboard.trialTariffLabel')
  const raw = sub.tariff?.name
  const name = typeof raw === 'string' ? raw.trim() : ''
  if (name) return name
  if (sub.tariff?.slug === 'classic') return t('dashboard.classicTariffFallback')
  return t('dashboard.basicPlan')
}

function DeviceCardIcon({ className }: { className?: string }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={cn('text-muted-foreground/70 dark:text-white/40', className)}
    >
      <path d="M10.5 1.5H8.25A2.25 2.25 0 006 3.75v16.5a2.25 2.25 0 002.25 2.25h7.5A2.25 2.25 0 0018 20.25V3.75a2.25 2.25 0 00-2.25-2.25H13.5m-3 0V3h3V1.5m-3 0h3m-3 18.75h3" />
    </svg>
  )
}

function StatusBadge({
  isActive,
  isExpired,
  hasSubscription,
}: {
  isActive: boolean
  isExpired: boolean
  hasSubscription: boolean
}) {
  const { t } = useTranslation()

  if (isActive) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:border-emerald-400/30 dark:bg-emerald-500/15 dark:text-emerald-200">
        <span className="size-1.5 rounded-full bg-emerald-500 animate-pulse" />
        {t('subscriptionPage.statusActive')}
      </span>
    )
  }
  if (isExpired) {
    return (
      <span className="inline-flex items-center rounded-full border border-destructive/40 bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">
        {t('subscriptionPage.statusExpired')}
      </span>
    )
  }
  if (!hasSubscription) {
    return (
      <span className="inline-flex items-center rounded-full border border-border bg-muted/60 px-2.5 py-1 text-xs font-medium text-muted-foreground">
        {t('subscriptionPage.statusNone')}
      </span>
    )
  }
  return null
}

/**
 * Заглушка страницы подписки: повторяет реальную раскладку (карточка тарифа,
 * блок ссылки, список устройств), чтобы после ответа сервера ничего не «прыгало».
 */
function SubscriptionSkeleton() {
  return (
    <div className="space-y-4 sm:space-y-6">
      <Card className="subscription-feature-card">
        <CardContent className="space-y-4 p-4 sm:p-5">
          <div className="flex items-start justify-between gap-3">
            <div className="space-y-2">
              <Skeleton className="h-3 w-24" />
              <Skeleton className="h-6 w-36" />
              <Skeleton className="h-3.5 w-44" />
            </div>
            <Skeleton className="h-6 w-24 rounded-full" />
          </div>

          <div className="space-y-2">
            <Skeleton className="h-3 w-32" />
            <Skeleton className="h-2.5 w-full rounded-full" />
          </div>

          <Skeleton className="h-[3.75rem] w-full rounded-xl" />
          <Skeleton className="h-16 w-full rounded-xl" />
        </CardContent>
      </Card>

      <Card className="subscription-feature-card">
        <CardContent className="px-5 py-5 sm:px-6">
          <Skeleton className="mb-3 h-4 w-40" />
          <div className="flex items-center gap-2">
            <Skeleton className="h-9 flex-1 rounded-lg" />
            <Skeleton className="h-9 w-28 rounded-lg" />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <Skeleton className="h-4 w-32" />
        </CardHeader>
        <CardContent className="space-y-3">
          <Skeleton className="h-3.5 w-40" />
          <DeviceRowsSkeleton n={2} />
        </CardContent>
      </Card>
    </div>
  )
}

/**
 * Полоса занятых слотов устройств — той же формы, что полоса трафика.
 * До красного не эскалирует: занятые слоты не поломка, а обычная граница.
 */
function DevicesUsageBar({
  used,
  limit,
  loading,
  title,
  allTakenHint,
  unlimitedLabel,
}: {
  used: number
  limit: number
  loading: boolean
  title: string
  allTakenHint: string
  unlimitedLabel: string
}) {
  const unlimited = limit <= 0
  const percent = unlimited ? 0 : Math.min(100, (used / limit) * 100)
  const full = !unlimited && used >= limit

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between gap-2 text-xs">
        <span className="text-muted-foreground dark:text-slate-300">{title}</span>
        {loading ? (
          <Skeleton className="h-3.5 w-14" />
        ) : (
          <span
            className={cn(
              'font-semibold tabular-nums',
              full ? 'text-amber-700 dark:text-amber-300' : 'text-foreground',
            )}
          >
            {used} / {unlimited ? unlimitedLabel : limit}
          </span>
        )}
      </div>
      {!unlimited && (
        <div className="h-2 rounded-full bg-muted dark:bg-white/10">
          <div
            className={cn(
              'h-full rounded-full transition-all duration-500',
              full
                ? 'bg-gradient-to-r from-amber-500 via-orange-500 to-orange-600 dark:from-amber-400 dark:via-orange-400 dark:to-amber-500'
                : 'bg-gradient-to-r from-primary via-primary/90 to-primary/70',
            )}
            style={{ width: `${percent}%` }}
          />
        </div>
      )}
      {full && <p className="mt-1.5 text-xs text-amber-700 dark:text-amber-300">{allTakenHint}</p>}
    </div>
  )
}

/** Строки списка устройств — высота совпадает с реальным `li`. */
function DeviceRowsSkeleton({ n }: { n: number }) {
  return (
    <ul className="space-y-2">
      {Array.from({ length: n }).map((_, i) => (
        <li
          key={i}
          className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2"
        >
          <div className="flex min-w-0 items-center gap-2">
            <Skeleton className="size-8 shrink-0 rounded-lg" />
            <div className="min-w-0 space-y-1.5">
              <Skeleton className="h-3.5 w-28" />
              <Skeleton className="h-3 w-20" />
            </div>
          </div>
          <Skeleton className="h-8 w-24 rounded-md" />
        </li>
      ))}
    </ul>
  )
}
