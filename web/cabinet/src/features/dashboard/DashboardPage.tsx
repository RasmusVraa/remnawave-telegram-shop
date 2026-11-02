import { type ReactNode, useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { PWAInstallPrompt } from '@/components/PWAInstallPrompt'
import { ResponsiveSheet } from '@/components/ResponsiveSheet'
import { TrafficUsageChart } from '@/components/TrafficUsageChart'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/components/ui/toast'
import { LoyaltyCompactCard } from '@/features/loyalty/LoyaltyProgramPage'
import { DeviceManagerProvider, DevicesPanel, devicesSheetSubtitle } from '@/features/home/DeviceManager'
import {
  DevicesWideCard,
  HomeAlert,
  HomeMainButton,
  HomeRings,
  HomeTitle,
  QuickActionsDesktop,
  QuickActionsMobile,
  TrafficDetails,
  trafficSheetSubtitle,
} from '@/features/home/HomeParts'
import { hasSubscriptionData, useHomeData, type HomeData } from '@/features/home/useHomeData'
import { api, ApiError, SUBSCRIPTION_STALE_MS } from '@/lib/api'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useIsMobile } from '@/hooks/useIsMobile'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'
import { useAuthStore } from '@/store/auth'
import { ImportantNotificationBanner } from '@/features/notifications/ImportantNotificationBanner'
import { BackupLoginRow, TariffsOfferCard, TrialOfferCard } from './TrialOffer'

/**
 * Главная — единственная страница подписки (раньше «Главная» и «Подписка»
 * повторяли друг друга, см. .cursor/work-in-progress/home-redesign/SPEC.md).
 *
 * ПК — «вариант 1»: тариф, три кольца, главная кнопка, быстрые действия,
 * «Мои устройства» прямо на странице, график.
 * Телефон — «вариант 6.2»: то же сверху, ниже круглые быстрые действия и
 * широкие карточки, которые открывают шторки устройств и трафика.
 *
 * Состояние (обычное / заканчивается / истекла / трафик исчерпан) считается
 * в useHomeData и одинаково красит чип, кольца, подсказку и кнопки.
 */
export default function DashboardPage() {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const qc = useQueryClient()
  const toast = useToast()
  const navigate = useNavigate()
  const location = useLocation()
  const isMobile = useIsMobile()
  const data = useHomeData()
  const { sub, subQuery } = data

  const { data: trial, isPending: trialPending } = useQuery({
    queryKey: ['trial-info'],
    queryFn: () => api.trialInfo(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
  })
  const { data: bootstrap } = useAuthBootstrap()
  const loyaltyVisible = bootstrap?.subscription_loyalty_visible === true

  const hasSubscription = hasSubscriptionData(sub)
  /*
   * Пока подписка не пришла, hasSubscription === false, и раньше отрисовывалась
   * карточка триала с `trial?.days ?? 0`. Поэтому до ответа — скелетон.
   */
  const mainCardLoading = subQuery.isPending || (!hasSubscription && trialPending)

  const [devicesOpen, setDevicesOpen] = useState(false)
  const [trafficOpen, setTrafficOpen] = useState(false)

  /*
   * /subscription теперь редирект сюда с #devices (старые кнопки бота, письма,
   * страница оплаты). На телефоне открываем шторку устройств, на ПК —
   * прокручиваем к блоку.
   */
  useEffect(() => {
    if (location.hash !== '#devices' || !hasSubscription) return
    if (isMobile) {
      setDevicesOpen(true)
      return
    }
    const id = window.setTimeout(() => document.getElementById('devices')?.scrollIntoView({ behavior: 'smooth', block: 'start' }), 150)
    return () => window.clearTimeout(id)
  }, [location.hash, hasSubscription, isMobile])

  const activateTrial = useMutation({
    mutationFn: () => api.activateTrial(),
    onSuccess: async () => {
      await Promise.all([
        qc.refetchQueries({ queryKey: ['subscription'] }),
        qc.refetchQueries({ queryKey: ['trial-info'] }),
      ])
      // Сразу на страницу подключения: пробный берут ради одного — попробовать VPN.
      navigate('/connections')
    },
    onError: (err) => {
      // Почта не подтверждена: пробный выдаётся только после подтверждения.
      if (err instanceof ApiError && err.status === 403 && err.body.includes('email not verified')) {
        navigate('/verify-email', { state: { email: useAuthStore.getState().user?.email ?? '' } })
        return
      }
      toast.error(t('dashboard.trialActivateFailed'))
    },
  })

  const trialOfferAvailable = Boolean(trial?.enabled && trial?.can_activate)

  // Один способ входа — потерял его, потерял аккаунт.
  const user = useAuthStore((s) => s.user)
  const needsBackupLogin = (user?.providers?.length ?? 0) < 2

  return (
    <AppLayout>
      <PWAInstallPrompt />
      {mainCardLoading ? (
        <PageReveal className="space-y-4">
          <RevealItem>
            <DashboardSkeleton />
          </RevealItem>
        </PageReveal>
      ) : hasSubscription ? (
        <DeviceManagerProvider data={data}>
          {isMobile ? (
            <HomeMobile
              data={data}
              lang={lang}
              onDevices={() => setDevicesOpen(true)}
              onTraffic={() => setTrafficOpen(true)}
              footer={
                <>
                  {loyaltyVisible && <LoyaltyCompactCard />}
                  {needsBackupLogin && <BackupLoginRow />}
                </>
              }
            />
          ) : (
            <HomeDesktop
              data={data}
              lang={lang}
              onTraffic={() => setTrafficOpen(true)}
              footer={
                <>
                  {loyaltyVisible && <LoyaltyCompactCard />}
                  {needsBackupLogin && <BackupLoginRow />}
                </>
              }
            />
          )}

          <ResponsiveSheet
            open={devicesOpen}
            onClose={() => setDevicesOpen(false)}
            title={t('home.myDevices')}
            subtitle={devicesSheetSubtitle(data, t)}
          >
            <DevicesPanel data={data} variant="sheet" />
          </ResponsiveSheet>

          <ResponsiveSheet
            open={trafficOpen}
            onClose={() => setTrafficOpen(false)}
            title={t('home.usageCardTitle')}
            subtitle={trafficSheetSubtitle(data, lang, t)}
          >
            <TrafficDetails data={data} lang={lang} />
          </ResponsiveSheet>
        </DeviceManagerProvider>
      ) : (
        <PageReveal className="space-y-4">
          {/* Важное нужно видеть и без подписки: «перебои» касаются и тех, кто только выбирает тариф. */}
          <RevealItem className="empty:hidden">
            <ImportantNotificationBanner />
          </RevealItem>
          <RevealItem>
            {/* Пробный доступен — предлагаем его; недоступен — витрину тарифов. */}
            {trialOfferAvailable ? (
              <TrialOfferCard trial={trial} onActivate={() => activateTrial.mutate()} activating={activateTrial.isPending} />
            ) : (
              <TariffsOfferCard trialEnabled={Boolean(trial?.enabled)} />
            )}
          </RevealItem>
        </PageReveal>
      )}
    </AppLayout>
  )
}

function HomeDesktop({
  data,
  lang,
  onTraffic,
  footer,
}: {
  data: HomeData
  lang: string
  onTraffic: () => void
  footer: ReactNode
}) {
  return (
    <PageReveal className="space-y-[18px]">
      <RevealItem>
        <HomeTitle data={data} lang={lang} />
      </RevealItem>
      <RevealItem className="empty:hidden">
        <ImportantNotificationBanner />
      </RevealItem>
      <RevealItem>
        <HomeRings
          data={data}
          onTraffic={onTraffic}
          onDevices={() => document.getElementById('devices')?.scrollIntoView({ behavior: 'smooth', block: 'start' })}
          devicesChevron={false}
        />
      </RevealItem>
      <RevealItem className="empty:hidden">
        <HomeAlert data={data} desktop />
      </RevealItem>
      <RevealItem>
        <HomeMainButton data={data} />
      </RevealItem>
      <RevealItem>
        <QuickActionsDesktop data={data} />
      </RevealItem>
      <RevealItem>
        <DevicesPanel data={data} variant="desktop" />
      </RevealItem>
      <RevealItem>
        {/* Вся карточка графика — вход в подробности трафика. */}
        <button type="button" onClick={onTraffic} className="block w-full text-left">
          <TrafficUsageChart data={data.usage} loading={data.usageQuery.isLoading} lang={lang} />
        </button>
      </RevealItem>
      <RevealItem className="space-y-[18px] empty:hidden">{footer}</RevealItem>
    </PageReveal>
  )
}

function HomeMobile({
  data,
  lang,
  onDevices,
  onTraffic,
  footer,
}: {
  data: HomeData
  lang: string
  onDevices: () => void
  onTraffic: () => void
  footer: ReactNode
}) {
  return (
    <PageReveal className="space-y-3">
      <RevealItem>
        <HomeTitle data={data} lang={lang} compact />
      </RevealItem>
      <RevealItem className="empty:hidden">
        <ImportantNotificationBanner />
      </RevealItem>
      <RevealItem>
        <HomeRings data={data} compact onTraffic={onTraffic} onDevices={onDevices} />
      </RevealItem>
      <RevealItem className="empty:hidden">
        <HomeAlert data={data} />
      </RevealItem>
      <RevealItem>
        <HomeMainButton data={data} />
      </RevealItem>
      <RevealItem className="!mt-4">
        <QuickActionsMobile data={data} />
      </RevealItem>
      <RevealItem className="!mt-4 space-y-2.5">
        <DevicesWideCard data={data} onOpen={onDevices} />
      </RevealItem>
      <RevealItem className="space-y-3 empty:hidden">{footer}</RevealItem>
    </PageReveal>
  )
}

/** Скелетон под главную: заголовок, три кольца, кнопка. */
function DashboardSkeleton() {
  return (
    <div className="space-y-5">
      <div className="flex items-end justify-between gap-3">
        <div className="space-y-2">
          <Skeleton className="h-8 w-40" />
          <Skeleton className="h-3.5 w-28" />
        </div>
        <Skeleton className="h-6 w-24 rounded-full" />
      </div>
      <div className="grid grid-cols-3 gap-3">
        {[0, 1, 2].map((i) => (
          <Card key={i} className="cabinet-elevated-card">
            <CardContent className="flex flex-col items-center gap-2 px-2 py-4">
              <Skeleton className="size-[72px] rounded-full" />
              <Skeleton className="h-3 w-14" />
            </CardContent>
          </Card>
        ))}
      </div>
      <Skeleton className="h-[52px] w-full rounded-full" />
      <Skeleton className="h-[4.5rem] w-full rounded-xl" />
    </div>
  )
}
