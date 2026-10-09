import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CreditCard, Bitcoin, AlertCircle, Star } from 'lucide-react'
import { createPortal } from 'react-dom'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Skeleton } from '@/components/ui/skeleton'
import { PlategaPaymentExpand, type PlategaMethodId } from '@/components/PlategaPaymentExpand'
import { ProviderMethodButton } from '@/components/ProviderMethodButton'
import { HeleketBrandIcon } from '@/components/BrandIcons'
import { PageTitleWithBack } from '@/components/PageTitleWithBack'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError, type TariffItem } from '@/lib/api'
import { newIdempotencyKey, cn } from '@/lib/utils'
import { reservePaymentWindow } from '@/lib/payment-window'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { LegalContinueDisclaimer } from '@/components/LegalContinueDisclaimer'
import { formatNumber } from '@/lib/format'

type Provider =
  | 'yookassa'
  | 'cryptopay'
  | 'telegram'
  | 'platega_sbp'
  | 'platega_cards'
  | 'platega_acquiring'
  | 'platega_worldwide'
  | 'platega_crypto'
  | 'heleket'

const PROVIDER_ORDER: Provider[] = [
  'yookassa',
  'platega_sbp',
  'platega_cards',
  'platega_acquiring',
  'platega_worldwide',
  'platega_crypto',
  'cryptopay',
  'telegram',
  'heleket',
]

export default function CheckoutPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const tariffSlug = searchParams.get('tariff') ?? ''
  const months = parseInt(searchParams.get('months') ?? '1', 10)

  const [provider, setProvider] = useState<Provider | null>(null)
  const [renewExtraHwid, setRenewExtraHwid] = useState<boolean | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Fetch tariffs to get the selected tariff details.
  const { data: tariffsData, isLoading: tariffsLoading } = useQuery({
    queryKey: ['tariffs'],
    queryFn: () => api.tariffs(),
    staleTime: 5 * 60_000,
  })
  const { data: bootstrap } = useAuthBootstrap()
  const { data: subscription } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: 30_000,
  })
  const extraHwidActive = subscription?.hwid_extra?.extra_active ?? 0
  const shouldAskExtraRenew = extraHwidActive > 0

  useEffect(() => {
    setRenewExtraHwid(shouldAskExtraRenew ? null : false)
  }, [shouldAskExtraRenew])

  const tariff: TariffItem | undefined = tariffsData?.tariffs.find(
    (t) => t.slug === tariffSlug && t.months === months,
  ) ?? tariffsData?.tariffs.find((t) => t.slug === tariffSlug)

  const previewTariffId =
    tariffsData?.sales_mode === 'tariffs' && tariff != null && tariff.id != null && tariff.id > 0
      ? tariff.id
      : undefined

  const providerEnabled: Record<Provider, boolean> = {
    yookassa: bootstrap?.payment_providers?.yookassa ?? true,
    cryptopay: bootstrap?.payment_providers?.cryptopay ?? true,
    telegram: bootstrap?.payment_providers?.telegram ?? false,
    platega_sbp: bootstrap?.payment_providers?.platega_sbp ?? false,
    platega_cards: bootstrap?.payment_providers?.platega_cards ?? false,
    platega_acquiring: bootstrap?.payment_providers?.platega_acquiring ?? false,
    platega_worldwide: bootstrap?.payment_providers?.platega_worldwide ?? false,
    platega_crypto: bootstrap?.payment_providers?.platega_crypto ?? false,
    heleket: bootstrap?.payment_providers?.heleket ?? false,
  }
  const defaultPreviewProvider =
    PROVIDER_ORDER.find((p) => providerEnabled[p]) ?? ('yookassa' as Provider)
  const selectedProviderForPreview: Provider = provider ?? defaultPreviewProvider
  const { data: preview, isError: previewError } = useQuery({
    queryKey: [
      'paymentPreview',
      tariff?.months,
      previewTariffId,
      tariffsData?.sales_mode,
      selectedProviderForPreview,
      shouldAskExtraRenew ? renewExtraHwid : false,
    ],
    queryFn: () =>
      api.paymentPreview(
        tariff!.months,
        previewTariffId,
        selectedProviderForPreview,
        shouldAskExtraRenew ? Boolean(renewExtraHwid) : false,
      ),
    enabled: Boolean(tariff) && !tariffsLoading && (!shouldAskExtraRenew || renewExtraHwid != null),
    staleTime: 30_000,
  })

  const amountValue = preview?.amount ?? preview?.amount_rub ?? tariff?.price_rub ?? 0
  const amountSuffix = preview?.currency === 'STARS' ? t('checkout.stars') : t('checkout.rub')
  const scenarioKey = checkoutScenarioI18nKey(preview?.scenario)

  async function handlePay() {
    if (!selectedProvider) return
    if (!tariff) { setError(t('errors.unknown')); return }
    setError(null)
    setLoading(true)

    // До первого await: иначе Safari заблокирует вкладку оплаты.
    const payWindow = reservePaymentWindow()
    const idempotencyKey = newIdempotencyKey()

    try {
      const tariffId =
        tariffsData?.sales_mode === 'tariffs' && tariff.id != null && tariff.id > 0 ? tariff.id : undefined
      const res = await api.checkout(
        {
          period: tariff.months,
          provider: selectedProvider,
          tariffId,
          renewExtraHwid: shouldAskExtraRenew ? Boolean(renewExtraHwid) : false,
        },
        idempotencyKey,
      )
      if (payWindow.go(res.payment_url)) {
        navigate(`/payment/status/${res.checkout_id}`)
      }
    } catch (err) {
      payWindow.cancel()
      if (err instanceof ApiError) {
        if (err.status === 429) {
          setError(t('errors.tooManyRequests'))
        } else if (err.status === 400 || err.status === 422) {
          setError(t('checkout.notAvailable'))
        } else {
          setError(t('errors.unknown'))
        }
      } else {
        setError(t('errors.unknown'))
      }
      setLoading(false)
    }
  }

  const monthsLabel = pluralizeMonths(tariff?.months ?? months)
  const showTariffSwitchBreakdown =
    preview != null &&
    (preview.scenario === 'upgrade' || preview.scenario === 'downgrade') &&
    typeof preview.tariff_switch_total_days === 'number'
  const availableProviders: Provider[] = PROVIDER_ORDER.filter((p) => providerEnabled[p])
  const selectedProvider = provider && providerEnabled[provider] ? provider : null
  const canInitiatePay =
    Boolean(selectedProvider) &&
    Boolean(tariff) &&
    availableProviders.length > 0 &&
    (!shouldAskExtraRenew || renewExtraHwid != null)
  const payButtonDisabled = !canInitiatePay || loading
  const disabledPayButtonClass =
    'disabled:opacity-100 disabled:bg-[hsl(174_31.2%_76.18%)] disabled:text-[hsl(0_0%_100%_/_57%)] disabled:brightness-100 dark:disabled:bg-primary dark:disabled:text-primary-foreground dark:disabled:brightness-[.45]'

  return (
    <AppLayout>
      <PageReveal className="max-w-lg mx-auto space-y-6">
        <RevealItem>
          <PageTitleWithBack title={t('checkout.title')} />
        </RevealItem>

        {/* Order summary */}
        <RevealItem>
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base font-medium text-muted-foreground">
              {t('checkout.summary')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {tariffsLoading ? (
              <div className="space-y-2.5">
                <div className="flex items-center justify-between gap-4">
                  <Skeleton className="h-3.5 w-24" />
                  <Skeleton className="h-3.5 w-20" />
                </div>
                <div className="flex items-center justify-between gap-4">
                  <Skeleton className="h-3.5 w-20" />
                  <Skeleton className="h-3.5 w-16" />
                </div>
                <div className="flex items-center justify-between gap-4 border-t border-border pt-2">
                  <Skeleton className="h-3.5 w-16" />
                  <Skeleton className="h-6 w-24" />
                </div>
              </div>
            ) : !tariff ? (
              <p className="text-sm text-destructive">{t('errors.unknown')}</p>
            ) : (
              <div className="space-y-2 text-sm">
                {preview && (
                  <SummaryRow label={t('checkout.scenario')} value={t(scenarioKey)} />
                )}
                <SummaryRow label={t('checkout.period')} value={monthsLabel} />
                {tariff.name && (
                  <SummaryRow label={t('checkout.plan')} value={tariff.name} />
                )}
                {tariff.device_limit > 0 && (
                  <SummaryRow
                    label={t('checkout.devices')}
                    value={
                      preview?.extra_hwid_included && (preview.extra_hwid_active ?? 0) > 0
                        ? t('checkout.devicesWithExtra', {
                            base: tariff.device_limit,
                            extra: preview.extra_hwid_active ?? 0,
                            amount: formatNumber(preview.extra_hwid_amount_rub ?? 0),
                            unit: amountSuffix,
                          })
                        : String(tariff.device_limit)
                    }
                  />
                )}
                {showTariffSwitchBreakdown && preview && (
                  <div className="space-y-2 rounded-lg border border-border/80 bg-muted/35 px-3 py-2.5 text-xs dark:border-border dark:bg-muted/25">
                    <p className="font-medium text-foreground">{t('checkout.switchTitle')}</p>
                    <SummaryRow
                      label={t('checkout.switchRemaining')}
                      value={t('checkout.switchDaysApprox', { days: preview.tariff_switch_remaining_days ?? 0 })}
                    />
                    <SummaryRow
                      label={t('checkout.switchBonus')}
                      value={t('checkout.switchDaysBonus', { days: preview.tariff_switch_bonus_days ?? 0 })}
                    />
                    <SummaryRow
                      label={t('checkout.switchTotal')}
                      value={t('checkout.switchDaysExact', { days: preview.tariff_switch_total_days ?? 0 })}
                    />
                  </div>
                )}
                {preview?.is_early_downgrade && !showTariffSwitchBreakdown && (
                  <p className="text-xs text-amber-700 dark:text-amber-400">{t('checkout.earlyDowngradeHint')}</p>
                )}
                {preview?.scenario === 'upgrade' && !showTariffSwitchBreakdown && (
                  <p className="text-xs text-amber-700 dark:text-amber-400">{t('checkout.earlyUpgradeHint')}</p>
                )}
                {previewError && (
                  <p className="text-xs text-muted-foreground">{t('checkout.previewError')}</p>
                )}
                {preview &&
                  preview.base_amount_rub != null &&
                  preview.base_amount_rub > 0 &&
                  preview.amount_rub < preview.base_amount_rub && (
                    <div className="space-y-2 rounded-lg bg-muted/50 px-3 py-2 text-xs">
                      {(preview.total_discount_pct ?? 0) > 0 && (
                        <SummaryRow
                          label={t('checkout.discountLabel')}
                          value={t('checkout.discountValueWithAmount', {
                            pct: preview.total_discount_pct,
                            amount: formatNumber(preview.base_amount_rub - preview.amount_rub),
                            unit: amountSuffix,
                          })}
                        />
                      )}
                    </div>
                  )}
                <div className="border-t border-border pt-2 mt-2">
                  <SummaryRow
                    label={t('checkout.total')}
                    value={
                      <span className="text-lg font-bold">
                        {formatNumber(amountValue)} {amountSuffix}
                      </span>
                    }
                  />
                </div>
              </div>
            )}
          </CardContent>
        </Card>
        </RevealItem>

        {/* Provider selection */}
        {shouldAskExtraRenew && (
          <RevealItem>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base font-medium text-muted-foreground">
                {t('checkout.extraHwidRenewTitle')}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <p className="text-sm text-muted-foreground">
                {t('checkout.extraHwidRenewHint', { n: extraHwidActive })}
              </p>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className={cn(
                    'border-emerald-300/80 text-emerald-700 hover:bg-emerald-500/10 dark:border-emerald-800 dark:text-emerald-300 dark:hover:bg-emerald-500/15',
                    renewExtraHwid === true &&
                      'bg-emerald-500/15 border-emerald-400/80 dark:border-emerald-700 dark:bg-emerald-900/40',
                  )}
                  onClick={() => setRenewExtraHwid(true)}
                >
                  {t('checkout.extraHwidRenewYes')}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className={cn(
                    'border-rose-300/80 text-rose-700 hover:bg-rose-500/10 dark:border-rose-900 dark:text-rose-300 dark:hover:bg-rose-500/15',
                    renewExtraHwid === false &&
                      'bg-rose-500/15 border-rose-400/80 dark:border-rose-700 dark:bg-rose-900/40',
                  )}
                  onClick={() => setRenewExtraHwid(false)}
                >
                  {t('checkout.extraHwidRenewNo')}
                </Button>
              </div>
            </CardContent>
          </Card>
          </RevealItem>
        )}

        <RevealItem>
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base font-medium text-muted-foreground">
              {t('checkout.chooseProvider')}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {availableProviders.includes('yookassa') && (
              <ProviderMethodButton
                selected={selectedProvider === 'yookassa'}
                onClick={() => setProvider('yookassa')}
                icon={<CreditCard size={18} className="text-primary" />}
                label={t('checkout.card')}
                description="YooKassa"
              />
            )}
            {availableProviders.includes('cryptopay') && (
              <ProviderMethodButton
                selected={selectedProvider === 'cryptopay'}
                onClick={() => setProvider('cryptopay')}
                icon={<Bitcoin size={18} className="text-orange-500" />}
                label={t('checkout.crypto')}
                description="CryptoPay"
              />
            )}
            <PlategaPaymentExpand
              enabled={providerEnabled}
              selected={provider}
              onSelect={(id: PlategaMethodId) => setProvider(id)}
              variant="checkout"
            />
            {availableProviders.includes('telegram') && (
              <ProviderMethodButton
                selected={selectedProvider === 'telegram'}
                onClick={() => setProvider('telegram')}
                icon={<Star size={18} className="text-amber-500" />}
                label={t('checkout.telegramStars')}
                description="Telegram Stars"
              />
            )}
            {availableProviders.includes('heleket') && (
              <ProviderMethodButton
                selected={selectedProvider === 'heleket'}
                onClick={() => setProvider('heleket')}
                icon={<HeleketBrandIcon className="h-[18px] w-[18px]" />}
                label={t('checkout.otherCrypto')}
                description="Heleket"
              />
            )}
            {availableProviders.length === 0 && (
              <p className="text-sm text-muted-foreground">{t('checkout.notAvailable')}</p>
            )}
          </CardContent>
        </Card>
        </RevealItem>

        {error && (
          <RevealItem>
            <Alert variant="destructive">
              <AlertCircle size={14} />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </RevealItem>
        )}

        <RevealItem className="hidden sm:block space-y-2">
          <Button
            className={cn('w-full', !loading && disabledPayButtonClass)}
            size="lg"
            disabled={payButtonDisabled}
            loading={loading}
            onClick={handlePay}
          >
            {t('checkout.pay')}
            {tariff ? ` ${formatNumber(amountValue)} ${amountSuffix}` : ''}
          </Button>
          <LegalContinueDisclaimer siteLinks={bootstrap?.site_links} />
        </RevealItem>

        {/* Mobile spacer: fixed pay bar + legal line should not overlap content */}
        <div className="sm:hidden h-[5.5rem]" aria-hidden />
      </PageReveal>

      {/*
        Mobile: шторка от кнопки до низа экрана, нижнее меню (z-50) лежит поверх неё.
        Без подложки способы оплаты просвечивали сквозь текст согласия.
        z-[39] — под выпадающим мобильным меню (z-40).
      */}
      {typeof document !== 'undefined' &&
        createPortal(
          <div className="cabinet-pay-scrim sm:hidden fixed inset-x-0 bottom-0 z-[39] px-3 pt-8 pb-[calc(var(--cabinet-bottom-nav-h,calc(73px+var(--cabinet-tg-safe-bottom)))+0.375rem)]">
            <div className="mx-auto flex w-full max-w-lg flex-col gap-1.5">
              <Button
                className={cn('w-full shadow-none', !loading && disabledPayButtonClass)}
                size="lg"
                disabled={payButtonDisabled}
                loading={loading}
                onClick={handlePay}
              >
                {t('checkout.pay')}
                {tariff ? ` ${formatNumber(amountValue)} ${amountSuffix}` : ''}
              </Button>
              <LegalContinueDisclaimer siteLinks={bootstrap?.site_links} variant="short" />
            </div>
          </div>,
          document.body,
        )}
    </AppLayout>
  )
}

// ── Helpers ─────────────────────────────────────────────────────────────

function checkoutScenarioI18nKey(scenario: string | undefined): string {
  const map: Record<string, string> = {
    new: 'checkout.scenarioNew',
    renew: 'checkout.scenarioRenew',
    upgrade: 'checkout.scenarioUpgrade',
    downgrade: 'checkout.scenarioDowngrade',
    classic_new: 'checkout.scenarioClassicNew',
    classic_renew: 'checkout.scenarioClassicRenew',
    unknown: 'checkout.scenarioUnknown',
  }
  if (!scenario) return 'checkout.scenarioUnknown'
  return map[scenario] ?? 'checkout.scenarioUnknown'
}

function SummaryRow({
  label,
  value,
}: {
  label: string
  value: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium text-right">{value}</span>
    </div>
  )
}

function pluralizeMonths(n: number): string {
  if (n === 1) return '1 месяц'
  if (n >= 2 && n <= 4) return `${n} месяца`
  return `${n} месяцев`
}
