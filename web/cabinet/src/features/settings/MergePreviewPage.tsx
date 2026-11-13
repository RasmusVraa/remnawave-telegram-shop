import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, CheckCircle2, Clock, XCircle } from 'lucide-react'
import { createPortal } from 'react-dom'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal } from '@/components/PageReveal'
import { Skeleton } from '@/components/ui/skeleton'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError, type MergePreviewResponse, type MergeCustomerSnapshot } from '@/lib/api'
import { useErrorHaptic } from '@/lib/haptic'
import { newIdempotencyKey, formatDate } from '@/lib/utils'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'
import { useAuthStore } from '@/store/auth'

export default function MergePreviewPage() {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const qc = useQueryClient()
  const nav = useNavigate()
  const [searchParams] = useSearchParams()
  const fetchMe = useAuthStore((s) => s.fetchMe)
  const accessToken = useAuthStore((s) => s.accessToken)
  const [merging, setMerging] = useState(false)
  const [force] = useState(false)
  const [keepSide, setKeepSide] = useState<'web' | 'tg' | null>(null)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)
  const [done, setDone] = useState(false)
  const [showDoneToast, setShowDoneToast] = useState(false)

  const { data: meFresh } = useQuery({
    queryKey: ['me-merge-fresh', accessToken],
    queryFn: () => api.me(),
    retry: false,
    staleTime: 0,
    refetchOnMount: 'always',
  })

  const { data: preview, isLoading, error: loadErr } = useQuery({
    queryKey: ['merge-preview'],
    queryFn: () => api.mergePreview(),
    retry: false,
    enabled: !done,
  })
  const { data: tariffsData } = useQuery({
    queryKey: ['tariffs-for-merge-preview'],
    queryFn: () => api.tariffs(),
    staleTime: 60_000,
    retry: 1,
  })

  const claimLeft = useClaimCountdown(preview?.claim_expires_at)
  const foundMethod = useMemo(() => {
    const provider = (searchParams.get('provider') || '').toLowerCase()
    switch (provider) {
      case 'google':
        return t('merge.methodGoogle')
      case 'yandex':
        return 'Yandex'
      case 'vk':
        return 'VK'
      case 'email':
        return t('merge.methodEmail')
      case 'telegram':
        return t('merge.methodTelegram')
      default:
        return t('merge.methodTelegram')
    }
  }, [searchParams, t])

  useEffect(() => {
    if (preview?.requires_subscription_choice) {
      setKeepSide(null)
    } else {
      setKeepSide('tg')
    }
  }, [preview?.requires_subscription_choice, preview?.customer_web?.id, preview?.customer_tg?.id])

  const mergeEnabled = useMemo(() => {
    if (!preview || preview.is_noop || done) return false
    if (preview.requires_subscription_choice) return keepSide === 'web' || keepSide === 'tg'
    return true
  }, [preview, done, keepSide])

  async function handleConfirm() {
    setError(null)
    setMerging(true)
    const key = newIdempotencyKey()
    try {
      await api.mergeConfirm(key, {
        force,
        keep_subscription: preview?.requires_subscription_choice ? (keepSide ?? undefined) : undefined,
      })
      setDone(true)
      setShowDoneToast(true)
      window.setTimeout(() => {
        setShowDoneToast(false)
        nav('/accounts')
      }, 1200)
      await fetchMe()
      await qc.invalidateQueries({ queryKey: ['subscription'] })
    } catch (e) {
      if (e instanceof ApiError && e.status === 202) {
        setDone(true)
        setShowDoneToast(true)
        window.setTimeout(() => {
          setShowDoneToast(false)
          nav('/accounts')
        }, 1200)
      } else if (e instanceof ApiError && e.status === 400) {
        setError(t('merge.needChoice'))
      } else if (e instanceof ApiError && e.status === 422 && e.body.includes('merge_claim_missing')) {
        setError(t('merge.claimMissing'))
      } else {
        setError(t('errors.unknown'))
      }
    } finally {
      setMerging(false)
    }
  }

  return (
    <AppLayout>
      <PageReveal className="max-w-lg mx-auto space-y-5 pb-10" wrapChildren>
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('merge.title')}</h1>
          <p className="text-sm text-muted-foreground mt-1">{t('merge.subtitle')}</p>
        </div>

        <Button variant="ghost" size="sm" asChild className="-ml-2 h-auto py-1">
          <Link to="/accounts">{t('merge.back')}</Link>
        </Button>

        {preview && !preview.is_noop ? (
          <Alert className="border-amber-500/40 bg-amber-500/5">
            <AlertTriangle className="size-4 text-amber-500 shrink-0" />
            <AlertDescription className="text-sm">{t('merge.mergeIntro')}</AlertDescription>
          </Alert>
        ) : null}

        {isLoading && (
          <div className="space-y-3" aria-hidden>
            <Skeleton className="h-24 w-full rounded-xl" />
            <Skeleton className="h-24 w-full rounded-xl" />
          </div>
        )}

        {loadErr && (
          <Alert variant="destructive">
            <AlertDescription>{t('errors.unknown')}</AlertDescription>
          </Alert>
        )}

        {preview?.requires_subscription_choice ? (
          <div className="rounded-lg border border-primary/30 bg-primary/5 px-3 py-2 text-sm text-foreground">{t('merge.chooseSubscription')}</div>
        ) : null}

        {preview && (
          <PreviewBody
            preview={preview}
            lang={lang}
            currentMethod={formatCurrentAccountMethods(meFresh?.providers, t)}
            foundMethod={foundMethod}
            keepSide={keepSide}
            onKeepSide={setKeepSide}
            tariffNamesById={buildTariffNamesMap(tariffsData?.tariffs)}
          />
        )}

        {preview?.is_noop && (
          <Alert>
            <AlertDescription>{t('merge.noop')}</AlertDescription>
          </Alert>
        )}

        {preview && !preview.is_noop && !done ? (
          <div className="space-y-3">
            <Button className="w-full" size="lg" loading={merging} disabled={!mergeEnabled} onClick={() => void handleConfirm()}>
              {t('merge.mergeAccounts')}
            </Button>
            <Button type="button" variant="ghost" className="w-full" onClick={() => nav('/accounts')}>
              {t('merge.cancel')}
            </Button>
          </div>
        ) : null}

        {claimLeft && !done ? (
          <p className="flex items-center justify-center gap-2 text-xs text-muted-foreground">
            <Clock className="size-3.5 shrink-0" />
            {t('merge.claimTimer', { time: claimLeft })}
          </p>
        ) : null}

        {typeof document !== 'undefined' && showDoneToast
          ? createPortal(
              <div className="fixed right-4 top-20 z-[1000] w-[min(360px,calc(100vw-2rem))] rounded-2xl border border-emerald-400/50 bg-background/95 px-4 py-3 text-sm font-medium text-emerald-400 shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] backdrop-blur-sm">
                <div className="flex items-center gap-2">
                  <CheckCircle2 className="size-4 shrink-0" />
                  <span>{t('merge.success')}</span>
                </div>
              </div>,
              document.body,
            )
          : null}
        {typeof document !== 'undefined' && (error || loadErr) && !showDoneToast
          ? createPortal(
              <div className="fixed right-4 top-20 z-[1000] w-[min(360px,calc(100vw-2rem))] rounded-2xl border border-destructive/40 bg-background/95 px-4 py-3 text-sm font-medium text-destructive shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] backdrop-blur-sm">
                <div className="flex items-center gap-2">
                  <XCircle className="size-4 shrink-0" />
                  <span>{error || t('errors.unknown')}</span>
                </div>
              </div>,
              document.body,
            )
          : null}
      </PageReveal>
    </AppLayout>
  )
}

function useClaimCountdown(iso?: string | null): string | null {
  const [label, setLabel] = useState<string | null>(null)
  useEffect(() => {
    if (!iso) {
      setLabel(null)
      return
    }
    const end = Date.parse(String(iso))
    if (!Number.isFinite(end)) {
      setLabel(null)
      return
    }
    const tick = () => {
      const ms = end - Date.now()
      if (ms <= 0) {
        setLabel('0:00')
        return
      }
      const m = Math.floor(ms / 60000)
      const s = Math.floor((ms % 60000) / 1000)
      setLabel(`${m}:${s.toString().padStart(2, '0')}`)
    }
    tick()
    const id = window.setInterval(tick, 1000)
    return () => window.clearInterval(id)
  }, [iso])
  return label
}

function PreviewBody({
  preview,
  lang,
  currentMethod,
  foundMethod,
  keepSide,
  onKeepSide,
  tariffNamesById,
}: {
  preview: MergePreviewResponse
  lang: string
  currentMethod: string
  foundMethod: string
  keepSide: 'web' | 'tg' | null
  onKeepSide: (v: 'web' | 'tg') => void
  tariffNamesById: Record<number, string>
}) {
  const { t } = useTranslation()
  const mergedDays = useMemo(() => {
    // When both profiles have subscriptions and user picks a side,
    // show the selected profile's expiry, not the default preview field.
    if (preview.requires_subscription_choice) {
      if (keepSide === 'web') return daysFromNow(preview.customer_web?.expire_at ?? null)
      if (keepSide === 'tg') return daysFromNow(preview.customer_tg?.expire_at ?? null)
      return null
    }
    return daysFromNow(preview.merged_expire_at ?? null)
  }, [
    preview.requires_subscription_choice,
    preview.merged_expire_at,
    preview.customer_web?.expire_at,
    preview.customer_tg?.expire_at,
    keepSide,
  ])
  const needChoice = Boolean(preview.requires_subscription_choice)
  // При ui_swap_sides в API customer_web=peer (email), customer_tg=текущий кабинет с TG.
  const swap = Boolean(preview.ui_swap_sides)
  const currentSnap = swap ? preview.customer_tg : preview.customer_web
  const foundSnap = swap ? preview.customer_web : preview.customer_tg
  const keepForCurrentCard: 'web' | 'tg' = swap ? 'tg' : 'web'
  const keepForFoundCard: 'web' | 'tg' = swap ? 'web' : 'tg'

  return (
    <div className="space-y-4">
      {currentSnap && (
        <SnapshotCard
          label={t('merge.currentAccount')}
          method={currentMethod}
          s={currentSnap}
          lang={lang}
          tariffNamesById={tariffNamesById}
          selected={needChoice && keepSide === keepForCurrentCard}
          onSelect={needChoice ? () => onKeepSide(keepForCurrentCard) : undefined}
        />
      )}
      {foundSnap && (
        <SnapshotCard
          label={t('merge.foundAccount')}
          method={foundMethod}
          s={foundSnap}
          lang={lang}
          tariffNamesById={tariffNamesById}
          selected={needChoice && keepSide === keepForFoundCard}
          onSelect={needChoice ? () => onKeepSide(keepForFoundCard) : undefined}
        />
      )}

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t('merge.afterMerge')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <p className="flex items-start gap-2 text-muted-foreground">
            <CheckCircle2 className="size-4 text-emerald-500 shrink-0 mt-0.5" />
            <span>{t('merge.resultMethods')}</span>
          </p>
          <p className="flex items-start gap-2 text-muted-foreground">
            <CheckCircle2 className="size-4 text-emerald-500 shrink-0 mt-0.5" />
            <span>{t('merge.resultLoyaltyReferrals')}</span>
          </p>
          {!preview.is_noop && (
            <p className="flex items-start gap-2 text-muted-foreground">
              <CheckCircle2 className="size-4 text-emerald-500 shrink-0 mt-0.5" />
              <span>
                {t('merge.resultDays')}: <span className="text-foreground">{formatDaysLabel(mergedDays, t)}</span>
              </span>
            </p>
          )}
          {needChoice ? (
            <p className="flex items-start gap-2 text-muted-foreground">
              <AlertTriangle className="size-4 text-amber-500 shrink-0 mt-0.5" />
              <span>{t('merge.resultDropSubscription')}</span>
            </p>
          ) : null}
          <p className="flex items-start gap-2 text-muted-foreground">
            <CheckCircle2 className="size-4 text-emerald-500 shrink-0 mt-0.5" />
            <span>{t('merge.resultHistory')}</span>
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

function daysFromNow(expireAt?: string | null): number | null {
  if (!expireAt) return null
  const ts = Date.parse(String(expireAt))
  if (!Number.isFinite(ts)) return null
  const diff = ts - Date.now()
  if (diff <= 0) return 0
  return Math.ceil(diff / (24 * 60 * 60 * 1000))
}

function formatDaysLabel(days: number | null, t: (key: string, options?: Record<string, unknown>) => string): string {
  if (days == null) return t('merge.noSubscription')
  if (days <= 0) return t('merge.expired')
  return t('merge.daysLeft', { count: days })
}

function formatCurrentAccountMethods(
  providers: string[] | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
): string {
  if (!providers || providers.length === 0) return t('merge.methodUnknown')
  const labels = Array.from(new Set(providers)).map((p) => {
    switch (p) {
      case 'google':
        return t('merge.methodGoogle')
      case 'yandex':
        return 'Yandex'
      case 'vk':
        return 'VK'
      case 'telegram':
        return t('merge.methodTelegram')
      case 'email':
        return t('merge.methodEmail')
      default:
        return p
    }
  })
  return labels.join(', ')
}

function SnapshotCard({
  label,
  method,
  s,
  lang,
  tariffNamesById,
  selected,
  onSelect,
}: {
  label: string
  method: string
  s: MergeCustomerSnapshot
  lang: string
  tariffNamesById: Record<number, string>
  selected?: boolean
  onSelect?: () => void
}) {
  const { t } = useTranslation()
  const days = useMemo(() => daysFromNow(s.expire_at ?? null), [s.expire_at])
  const tariffLabel = useMemo(() => {
    if (s.current_tariff_id == null) return '—'
    return tariffNamesById[s.current_tariff_id] || `#${s.current_tariff_id}`
  }, [s.current_tariff_id, tariffNamesById])
  const interactive = Boolean(onSelect)

  return (
    <Card
      className={
        interactive
          ? selected
            ? 'ring-2 ring-primary border-primary cursor-pointer transition-shadow'
            : 'cursor-pointer hover:border-muted-foreground/30 transition-colors'
          : ''
      }
      onClick={onSelect}
      role={interactive ? 'button' : undefined}
      tabIndex={interactive ? 0 : undefined}
      onKeyDown={
        interactive
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onSelect?.()
              }
            }
          : undefined
      }
    >
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{label}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <div>
          <p className="text-muted-foreground text-xs">{t('merge.signInMethod')}</p>
          <p className="font-medium">{method}</p>
        </div>
        <div>
          <p className="text-muted-foreground text-xs">{t('merge.subscriptionDays')}</p>
          <p className="font-medium">{formatDaysLabel(days, t)}</p>
          <p className="text-xs text-muted-foreground">
            {t('subscriptionPage.tariff')}: {tariffLabel}
          </p>
          {s.expire_at && (
            <p className="text-xs text-muted-foreground">
              {t('merge.expireDate')}: {formatDate(String(s.expire_at), lang)}
            </p>
          )}
        </div>
        {onSelect ? (
          <label className="flex items-center gap-2 cursor-pointer text-sm">
            <input type="radio" className="accent-primary" checked={Boolean(selected)} onChange={() => onSelect()} />
            {t('merge.keepThisSubscription')}
          </label>
        ) : null}
      </CardContent>
    </Card>
  )
}

function buildTariffNamesMap(
  tariffs: Array<{ id: number | null; name: string }> | undefined,
): Record<number, string> {
  if (!tariffs || tariffs.length === 0) return {}
  const map: Record<number, string> = {}
  for (const item of tariffs) {
    if (item.id == null) continue
    if (!map[item.id]) {
      map[item.id] = String(item.name || '').trim() || `#${item.id}`
    }
  }
  return map
}
