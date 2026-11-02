import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertCircle } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Skeleton } from '@/components/ui/skeleton'
import { PageTitleWithBack } from '@/components/PageTitleWithBack'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api, ApiError, type TariffItem } from '@/lib/api'
import { TariffLocationPicker } from '@/features/checkout/TariffLocationPicker'
import { newIdempotencyKey } from '@/lib/utils'
import { reservePaymentWindow } from '@/lib/payment-window'
import { formatNumber } from '@/lib/format'
import { AdminToggleSwitch } from '@/features/admin/components/AdminToggleSwitch'
import { CheckoutPayButton, PROVIDER_ORDER, SummaryRow, type Provider } from '@/features/checkout/checkoutShared'
import { PaymentMethodPicker } from '@/features/checkout/PaymentMethodPicker'
import { useCheckoutMethod } from '@/features/checkout/paymentMethods'

export default function CheckoutPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const tariffSlug = searchParams.get('tariff') ?? ''
  const months = parseInt(searchParams.get('months') ?? '1', 10)

  const [renewExtraHwid, setRenewExtraHwid] = useState<boolean | null>(null)
  const [locationId, setLocationId] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Fetch tariffs to get the selected tariff details.
  const { data: tariffsData, isLoading: tariffsLoading } = useQuery({
    queryKey: ['tariffs'],
    queryFn: () => api.tariffs(),
    staleTime: 5 * 60_000,
  })
  const { data: subscription } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: 30_000,
  })
  const extraHwidActive = subscription?.hwid_extra?.extra_active ?? 0
  const shouldAskExtraRenew = extraHwidActive > 0

  // Докупленные места по умолчанию продлеваются: человек продлевает то, что
  // у него уже есть. Цена мест видна в строке переключателя и входит в «Итого».
  useEffect(() => {
    setRenewExtraHwid(shouldAskExtraRenew)
  }, [shouldAskExtraRenew])

  const tariff: TariffItem | undefined = tariffsData?.tariffs.find(
    (t) => t.slug === tariffSlug && t.months === months,
  ) ?? tariffsData?.tariffs.find((t) => t.slug === tariffSlug)
  const locations = tariff?.locations ?? []
  const needsLocation = locations.length > 0
  const allLocationsSoldOut = needsLocation && locations.every((loc) => loc.sold_out)

  useEffect(() => {
    setLocationId(null)
  }, [tariff?.id, tariff?.slug, tariff?.months])

  const previewTariffId =
    tariffsData?.sales_mode === 'tariffs' && tariff != null && tariff.id != null && tariff.id > 0
      ? tariff.id
      : undefined

  const checkoutMethod = useCheckoutMethod()
  const { providerEnabled, provider: selectedProvider } = checkoutMethod
  const defaultPreviewProvider =
    PROVIDER_ORDER.find((p) => providerEnabled[p]) ?? ('yookassa' as Provider)
  const selectedProviderForPreview: Provider = selectedProvider ?? defaultPreviewProvider
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

  // Цена мест нужна и при выключенном переключателе — чтобы человек видел,
  // сколько стоит включить. Ключ совпадает с основным при включённом, кэш общий.
  const { data: previewWithExtra } = useQuery({
    queryKey: ['paymentPreview', tariff?.months, previewTariffId, tariffsData?.sales_mode, selectedProviderForPreview, true],
    queryFn: () => api.paymentPreview(tariff!.months, previewTariffId, selectedProviderForPreview, true),
    enabled: Boolean(tariff) && !tariffsLoading && shouldAskExtraRenew,
    staleTime: 30_000,
  })
  const extraHwidAmount = previewWithExtra?.extra_hwid_included ? previewWithExtra.extra_hwid_amount_rub : undefined
  const extraHwidUnit = previewWithExtra?.currency === 'STARS' ? t('checkout.stars') : t('checkout.rub')

  const amountValue = preview?.amount ?? preview?.amount_rub ?? tariff?.price_rub ?? 0
  const amountSuffix = preview?.currency === 'STARS' ? t('checkout.stars') : t('checkout.rub')
  const scenarioKey = checkoutScenarioI18nKey(preview?.scenario)

  async function handlePay() {
    if (!selectedProvider) return
    if (!tariff) { setError(t('errors.unknown')); return }
    if (needsLocation && locationId == null) {
      setError(allLocationsSoldOut ? t('checkout.allLocationsSoldOut') : t('checkout.locationRequired'))
      return
    }
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
          yookassaMethod: checkoutMethod.yookassaMethod,
          tariffId,
          renewExtraHwid: shouldAskExtraRenew ? Boolean(renewExtraHwid) : false,
          locationId: needsLocation ? locationId : undefined,
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
          const body = err.body
          if (body.includes('location_sold_out')) setError(t('checkout.locationSoldOut'))
          else if (body.includes('location_required')) setError(t('checkout.locationRequired'))
          else if (body.includes('location_invalid')) setError(t('checkout.locationInvalid'))
          else setError(t('checkout.notAvailable'))
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
  const canInitiatePay =
    Boolean(selectedProvider) &&
    Boolean(tariff) &&
    (!shouldAskExtraRenew || renewExtraHwid != null) &&
    (!needsLocation || (locationId != null && !allLocationsSoldOut))
  const payButtonDisabled = !canInitiatePay || loading

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
                {shouldAskExtraRenew && (
                  <div className="flex items-center gap-3 rounded-lg bg-muted/50 px-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="font-medium text-foreground">{t('checkout.extraHwidRenewToggle')}</p>
                      <p className="text-xs text-muted-foreground">
                        {extraHwidAmount != null
                          ? t('checkout.extraHwidRenewToggleHint', {
                              slots: t('subscriptionPage.slotsCount', { count: extraHwidActive }),
                              amount: formatNumber(extraHwidAmount),
                              unit: extraHwidUnit,
                            })
                          : t('subscriptionPage.slotsCount', { count: extraHwidActive })}
                      </p>
                    </div>
                    <AdminToggleSwitch
                      checked={Boolean(renewExtraHwid)}
                      onChange={setRenewExtraHwid}
                      aria-label={t('checkout.extraHwidRenewToggle')}
                    />
                  </div>
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

        {needsLocation && (
          <RevealItem>
            <TariffLocationPicker
              locations={locations}
              locationId={locationId}
              onSelect={setLocationId}
            />
          </RevealItem>
        )}

        {/* Provider selection */}
        <RevealItem>
          <PaymentMethodPicker
            methods={checkoutMethod.methods}
            method={checkoutMethod.method}
            initialId={checkoutMethod.initial.id}
            initialSource={checkoutMethod.initial.source}
            loading={checkoutMethod.loading}
            providerEnabled={providerEnabled}
            onSelect={checkoutMethod.setMethod}
          />
        </RevealItem>

        {error && (
          <RevealItem>
            <Alert variant="destructive">
              <AlertCircle size={14} />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </RevealItem>
        )}

        <CheckoutPayButton
          label={`${t('checkout.pay')}${tariff ? ` ${formatNumber(amountValue)} ${amountSuffix}` : ''}`}
          disabled={payButtonDisabled}
          loading={loading}
          onPay={handlePay}
        />
      </PageReveal>

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

function pluralizeMonths(n: number): string {
  if (n === 1) return '1 месяц'
  if (n >= 2 && n <= 4) return `${n} месяца`
  return `${n} месяцев`
}
