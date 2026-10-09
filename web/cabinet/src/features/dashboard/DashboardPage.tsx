import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import {
  Activity,
  Users,
  Handshake,
  Zap,
  ChevronRight,
  Ticket,
  FileText,
  Newspaper,
  Star,
  type LucideIcon,
} from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { PWAInstallPrompt } from '@/components/PWAInstallPrompt'
import { StatRing, UnboundedRing } from '@/components/StatRing'
import { SubscriptionActions } from '@/components/SubscriptionActions'
import { TrafficUsageChart } from '@/components/TrafficUsageChart'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/components/ui/toast'
import { api, SUBSCRIPTION_STALE_MS, type SubscriptionResponse } from '@/lib/api'
import { cn, daysUntil, formatDate, trafficUsagePercent } from '@/lib/utils'
import { daysTone, devicesTone, trafficTone } from '@/lib/subscriptionTone'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useAuthStore } from '@/store/auth'
import { BackupLoginRow, TariffsOfferCard, TrialOfferCard } from './TrialOffer'

/**
 * Главная — «приборная панель».
 *
 * Состояние подписки читается за секунду по трём кольцам: дни, трафик,
 * устройства. Все три красятся по общей шкале тревоги (lib/subscriptionTone),
 * поэтому цвет означает одно и то же независимо от показателя.
 *
 * Разделы кабинета разложены в два уровня. Шесть равновесных плиток заявляли
 * одинаковую важность, хотя «Тарифы» — это деньги, а «Отзывы» — ссылка в
 * Telegram: главные три остались карточками, редкие ушли строкой ниже.
 */

/** Опорный горизонт для кольца дней: 90 суток закрывают типичные периоды подписки. */
const DAYS_RING_HORIZON = 90

export default function DashboardPage() {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const qc = useQueryClient()
  const toast = useToast()
  const navigate = useNavigate()

  const { data: sub, isPending: subPending } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
  })

  const { data: trial, isPending: trialPending } = useQuery({
    queryKey: ['trial-info'],
    queryFn: () => api.trialInfo(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
  })

  // isLoading, а не isPending: запрос выключен до появления sub, и в этом
  // состоянии isPending остаётся true — скелетон висел бы вечно.
  const { data: devices, isLoading: devicesLoading } = useQuery({
    queryKey: ['devices'],
    queryFn: () => api.devices(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
    enabled: Boolean(sub),
  })
  // График расхода за расчётный период. Панель отдаёт готовый ряд по дням;
  // ошибки и отсутствие интеграции возвращаются как пустой ряд, а не как сбой.
  const { data: usage, isLoading: usageLoading } = useQuery({
    queryKey: ['traffic-usage'],
    queryFn: () => api.trafficUsage(),
    staleTime: 5 * 60_000,
    retry: 1,
    enabled: Boolean(sub),
  })

  const { data: bootstrap, isPending: bootstrapPending } = useAuthBootstrap()

  const hasSubscription = hasSubscriptionData(sub)
  /*
   * Пока подписка не пришла, hasSubscription === false, и раньше отрисовывалась
   * карточка триала с `trial?.days ?? 0` — пользователь на медленной сети видел
   * «0 дней / 0 ГБ / 0 устройств» и «Пробный период недоступен», а через секунду
   * содержимое подменялось. Поэтому до ответа показываем скелетон.
   */
  const mainCardLoading = subPending || (!hasSubscription && trialPending)

  const days = sub?.expire_at ? daysUntil(sub.expire_at) : null
  const trafficPct = trafficUsagePercent(sub?.traffic_used_gb, sub?.traffic_limit_gb)
  const connectedDevices = Math.max(0, devices?.connected ?? 0)
  const deviceLimit = Math.max(
    sub?.tariff?.device_limit ?? 0,
    Math.max(0, devices?.device_limit ?? 0),
  )
  const isExpiredByTraffic = Boolean(
    sub?.traffic_limit_gb != null &&
      sub.traffic_limit_gb > 0 &&
      (sub.traffic_used_gb ?? 0) >= sub.traffic_limit_gb,
  )
  const isActive = !(isExpiredByTraffic || (days !== null && days <= 0))
  const effectiveDays = isActive ? days : 0

  const newsUrl = bootstrap?.site_links?.channel?.trim()
  const feedbackUrl = bootstrap?.site_links?.feedback?.trim()

  const activateTrial = useMutation({
    mutationFn: () => api.activateTrial(),
    onSuccess: async () => {
      await Promise.all([
        qc.refetchQueries({ queryKey: ['subscription'] }),
        qc.refetchQueries({ queryKey: ['trial-info'] }),
      ])
      /*
       * Сразу на страницу подключения, а не обратно на приборную панель.
       *
       * Пробный период человек берёт ради одного — попробовать VPN, и сразу
       * после активации ему нужны инструкции по подключению. Раньше он
       * оставался на главной, где подсказка лишь показывала пальцем на кнопку
       * «Подключить устройство»: лишний экран и лишнее нажатие между «взял
       * пробный» и «работающим VPN».
       *
       * Подсказку про кнопку «Подключить устройство» здесь намеренно НЕ гасим.
       * Мы привели человека на страницу подключения за руку, но дорогу к ней
       * не показали: в следующий раз он будет искать её сам. Подсказка ждёт
       * первого самостоятельного захода на главную или в подписку — и гаснет
       * сама, когда устройство подключено (см. CabinetOnboarding).
       */
      navigate('/connections')
    },
    // Без этого при ошибке кнопка просто разблокировалась, и пользователь
    // не понимал, активировался триал или нет.
    onError: () => toast.error(t('dashboard.trialActivateFailed')),
  })

  const trialOfferAvailable = Boolean(trial?.enabled && trial?.can_activate)

  /*
   * Один способ входа — потерял его, потерял аккаунт. Строка исчезает сама,
   * как только привязан второй. Тот же порог, что подсвечивает CTA
   * «Привязанные аккаунты» в профиле.
   */
  const user = useAuthStore((s) => s.user)
  const needsBackupLogin = (user?.providers?.length ?? 0) < 2

  return (
    <AppLayout>
      <PWAInstallPrompt />
      <PageReveal className="space-y-4">
        {mainCardLoading ? (
          <RevealItem>
            <DashboardSkeleton />
          </RevealItem>
        ) : hasSubscription ? (
          <>
            <RevealItem className="flex items-end justify-between gap-3">
              <div className="min-w-0">
                <h1 className="font-heading text-3xl font-bold tracking-tight">
                  {subscriptionTariffLabel(sub, t)}
                </h1>
                {/* Дата вместо блока «Действует до»: он дублировал бы кольцо дней. */}
                <p className="mt-1 truncate text-sm text-muted-foreground">
                  {isActive && sub?.expire_at
                    ? t('dashboard.untilDate', { date: formatDate(sub.expire_at, lang) })
                    : t('subscriptionPage.expiredBlockTitle')}
                </p>
              </div>
              <StatusBadge days={effectiveDays} isActive={isActive} />
            </RevealItem>

            <RevealItem>
              <div className="grid grid-cols-3 gap-2.5">
                <Card className="cabinet-elevated-card">
                  <CardContent className="flex items-center justify-center px-2 py-4">
                    <StatRing
                      value={
                        isActive && days !== null
                          ? Math.min(100, (days / DAYS_RING_HORIZON) * 100)
                          : 0
                      }
                      tone={daysTone(effectiveDays)}
                      label={t('dashboard.statDays')}
                    >
                      <span className="font-heading text-base font-bold">
                        {isActive && days !== null ? days : 0}
                      </span>
                    </StatRing>
                  </CardContent>
                </Card>

                <Card className="cabinet-elevated-card">
                  <CardContent className="flex items-center justify-center px-2 py-4">
                    {trafficPct === null ? (
                      <UnboundedRing label={t('dashboard.statTraffic')} />
                    ) : (
                      <StatRing
                        value={trafficPct}
                        tone={trafficTone(trafficPct)}
                        label={t('dashboard.statTraffic')}
                      >
                        <span className="font-heading text-base font-bold">
                          {Math.round(trafficPct)}%
                        </span>
                      </StatRing>
                    )}
                  </CardContent>
                </Card>

                <Card className="cabinet-elevated-card">
                  <CardContent className="flex items-center justify-center px-2 py-4">
                    {devicesLoading ? (
                      <Skeleton className="size-[72px] rounded-full" />
                    ) : (
                      <StatRing
                        value={deviceLimit > 0 ? (connectedDevices / deviceLimit) * 100 : 0}
                        tone={devicesTone(connectedDevices, deviceLimit)}
                        label={t('dashboard.statDevices')}
                      >
                        <span className="font-heading text-base font-bold">{connectedDevices}</span>
                      </StatRing>
                    )}
                  </CardContent>
                </Card>
              </div>
            </RevealItem>

            <RevealItem>
              <SubscriptionActions
                days={effectiveDays}
                devicesUsed={connectedDevices}
                devicesLimit={deviceLimit}
                connectId="cabinet-onboarding-connect-target"
              />
            </RevealItem>

            <RevealItem>
              <TrafficUsageChart data={usage} loading={usageLoading} lang={lang} />
            </RevealItem>
          </>
        ) : (
          <RevealItem>
            {/*
              Пробный доступен — предлагаем его; недоступен — витрину тарифов.
              Раньше во втором случае кнопка просто гасла с подписью
              «Пробный период недоступен», и человеку было некуда идти.
            */}
            {trialOfferAvailable ? (
              <TrialOfferCard
                trial={trial}
                onActivate={() => activateTrial.mutate()}
                activating={activateTrial.isPending}
              />
            ) : (
              <TariffsOfferCard trialEnabled={Boolean(trial?.enabled)} />
            )}
          </RevealItem>
        )}

        {hasSubscription && needsBackupLogin && (
          <RevealItem>
            <BackupLoginRow />
          </RevealItem>
        )}

        {/*
          Разделы прячем, пока подписки нет: тарифы, рефералы и промокоды
          уводят от единственной задачи — довести человека до работающего VPN.
          Кто пришёл покупать, уходит по ссылке под кнопкой предложения.
        */}
        {(mainCardLoading || hasSubscription) && (
          <RevealItem>
            <QuickLinks
              newsUrl={newsUrl}
              feedbackUrl={feedbackUrl}
              bootstrapPending={bootstrapPending}
            />
          </RevealItem>
        )}
      </PageReveal>
    </AppLayout>
  )
}

/** Статус подписки по общей шкале: зелёный → янтарный → красный. */
function StatusBadge({ days, isActive }: { days: number | null; isActive: boolean }) {
  const { t } = useTranslation()
  const tone = isActive ? daysTone(days) : 'danger'

  if (!isActive) {
    return (
      <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-destructive/45 bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">
        <span className="size-1.5 rounded-full bg-destructive" />
        {t('subscriptionPage.statusExpired')}
      </span>
    )
  }

  if (tone !== 'calm') {
    const danger = tone === 'danger'
    return (
      <span
        className={cn(
          'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium',
          danger
            ? 'border-destructive/45 bg-destructive/10 text-destructive'
            : 'border-amber-400/50 bg-amber-500/10 text-amber-700 dark:text-amber-300',
        )}
      >
        <span className={cn('size-1.5 rounded-full', danger ? 'bg-destructive' : 'bg-amber-500')} />
        {t('subscriptionPage.statusEnding')}
      </span>
    )
  }

  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:border-emerald-400/30 dark:bg-emerald-500/15 dark:text-emerald-200">
      <span className="size-1.5 rounded-full bg-emerald-500" />
      {t('subscriptionPage.statusActive')}
    </span>
  )
}

/**
 * Разделы кабинета в два уровня: главные — карточками, редкие — строкой.
 * «Новости» и «Отзывы» приходят из bootstrap, поэтому до его ответа держим
 * места заглушками, иначе строка подпрыгивает.
 */
interface QuickAction {
  to: string
  icon: LucideIcon
  label: string
  hint: string
  accent: string
}

/**
 * Строка быстрого перехода.
 *
 * compact — режим «две штуки в ряд на телефоне»: подсказка прячется, иначе
 * подписи наезжают друг на друга на узких экранах.
 */
function QuickActionLink({ action, compact }: { action: QuickAction; compact?: boolean }) {
  const { to, icon: Icon, label, hint, accent } = action
  return (
    <Link
      to={to}
      className={cn('cabinet-elevated-card cabinet-row flex items-center gap-3 px-3 py-2.5', accent)}
    >
      <span className="cabinet-icon-box inline-flex size-9 shrink-0 items-center justify-center rounded-lg">
        <Icon size={16} aria-hidden />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{label}</span>
        <span className={cn('block truncate text-xs text-muted-foreground', compact && 'hidden sm:block')}>
          {hint}
        </span>
      </span>
      <ChevronRight size={18} className="cabinet-row-chevron shrink-0 text-muted-foreground" aria-hidden />
    </Link>
  )
}

function QuickLinks({
  newsUrl,
  feedbackUrl,
  bootstrapPending,
}: {
  newsUrl?: string
  feedbackUrl?: string
  bootstrapPending: boolean
}) {
  const { t } = useTranslation()

  /*
   * У каждого раздела свой акцент: он же красит иконку, кольцо на наведении
   * и подсветку строки, поэтому элемент читается как одно целое, а не как
   * цветная иконка на нейтральной плашке.
   */
  const bootstrap = useAuthBootstrap().data
  const partnerVisible = bootstrap?.partner_nav_visible !== false

  const primary: QuickAction[] = [
    {
      to: '/tariffs',
      icon: Zap,
      label: t('dashboard.tariffsCardTitle'),
      hint: t('dashboard.tariffsCardHint'),
      accent: 'cabinet-accent-violet',
    },
    {
      to: '/promocodes',
      icon: Ticket,
      label: t('dashboard.promocodesCardTitle'),
      hint: t('dashboard.promocodesCardHint'),
      accent: 'cabinet-accent-amber',
    },
  ]

  /*
   * Рефералка и партнёрка идут парой: это две программы про «приведи клиента»,
   * и стоять они должны рядом, а не через строку. Ряд держится в две колонки
   * даже на телефоне — подписи короткие, помещаются; подсказка под ними на
   * узком экране прячется, иначе строка ломается.
   *
   * Партнёрка выключается настройкой, и тогда рефералка остаётся одна: ряд
   * схлопывается в одну колонку, а не оставляет пустую половину.
   */
  const pair: QuickAction[] = [
    {
      to: '/referral',
      icon: Users,
      label: t('dashboard.referralsCardTitle'),
      hint: t('dashboard.referralsCardHint'),
      accent: '',
    },
  ]
  if (partnerVisible) {
    pair.push({
      to: '/partner',
      icon: Handshake,
      label: t('dashboard.partnerCardTitle'),
      hint: t('dashboard.partnerCardHint'),
      accent: 'cabinet-accent-emerald',
    })
  }

  return (
    <div className="space-y-2.5">
      <div className="grid gap-2.5 sm:grid-cols-2">
        {primary.map((action) => (
          <QuickActionLink key={action.to} action={action} />
        ))}
      </div>

      <div className={cn('grid gap-2.5', pair.length === 2 ? 'grid-cols-2' : 'grid-cols-1')}>
        {pair.map((action) => (
          <QuickActionLink key={action.to} action={action} compact={pair.length === 2} />
        ))}
      </div>

      <QuickActionLink
        action={{
          to: '/status',
          icon: Activity,
          label: t('dashboard.serversCardTitle'),
          hint: t('dashboard.serversCardHint'),
          accent: '',
        }}
      />

      <div className="flex flex-wrap items-center justify-center gap-x-5 gap-y-2 py-1">
        <Link
          to="/support#cabinet-info"
          className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          <FileText size={13} aria-hidden />
          {t('dashboard.infoCardTitle')}
        </Link>
        {bootstrapPending ? (
          <>
            <Skeleton className="h-3.5 w-16" />
            <Skeleton className="h-3.5 w-16" />
          </>
        ) : (
          <>
            {newsUrl && (
              <a
                href={newsUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
              >
                <Newspaper size={13} aria-hidden />
                {t('dashboard.newsCardTitle')}
              </a>
            )}
            {feedbackUrl && (
              <a
                href={feedbackUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
              >
                <Star size={13} aria-hidden />
                {t('dashboard.feedbackCardTitle')}
              </a>
            )}
          </>
        )}
      </div>
    </div>
  )
}

/** Скелетон под приборную панель: заголовок, три кольца, кнопки, срок. */
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

      <Skeleton className="h-11 w-full rounded-lg" />
      <Skeleton className="h-[4.5rem] w-full rounded-xl" />
    </div>
  )
}

function subscriptionTariffLabel(sub: SubscriptionResponse | null | undefined, t: TFunction): string {
  if (!sub) return t('dashboard.basicPlan')
  if (sub.is_trial) return t('dashboard.trialTariffLabel')
  const raw = sub.tariff?.name
  const name = typeof raw === 'string' ? raw.trim() : ''
  if (name) return name
  if (sub.tariff?.slug === 'classic') return t('dashboard.classicTariffFallback')
  return t('dashboard.basicPlan')
}

function hasSubscriptionData(sub?: SubscriptionResponse | null): boolean {
  if (!sub) return false
  if (sub.subscription_link && String(sub.subscription_link).trim() !== '') return true
  if (sub.expire_at && String(sub.expire_at).trim() !== '') return true
  return false
}
