import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertCircle } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { PageTitleWithBack } from '@/components/PageTitleWithBack'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError, SUBSCRIPTION_STALE_MS } from '@/lib/api'
import { formatNumber } from '@/lib/format'
import { newIdempotencyKey } from '@/lib/utils'
import { reservePaymentWindow } from '@/lib/payment-window'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'
import { formatDayMonth } from '@/features/subscription/DeviceSlots'
import {
  CheckoutPayButton,
  PROVIDER_ORDER,
  ProviderPickerCard,
  SummaryRow,
  useProviderEnabled,
  type Provider,
} from '@/features/checkout/checkoutShared'

/**
 * Оформление докупки мест для устройств. Та же страница, что при оплате
 * подписки (сводка → способ оплаты → «Оплатить»), только сводка про места.
 * Число мест приходит из окна «Места для устройств» в `?target=`.
 */
export default function DeviceSlotsCheckoutPage() {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const target = parseInt(searchParams.get('target') ?? '', 10)

  const [provider, setProvider] = useState<Provider | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { data: sub, isLoading: subLoading } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: SUBSCRIPTION_STALE_MS,
  })
  const hwid = sub?.hwid_extra
  const valid = Boolean(
    hwid?.enabled &&
      hwid.ui_visible &&
      hwid.can_increase &&
      Number.isFinite(target) &&
      target > hwid.current_limit &&
      target <= hwid.max_limit,
  )

  const providerEnabled = useProviderEnabled()
  const availableProviders = PROVIDER_ORDER.filter((p) => providerEnabled[p])
  const selectedProvider = provider && providerEnabled[provider] ? provider : null
  const previewProvider: Provider = selectedProvider ?? availableProviders[0] ?? 'yookassa'

  const { data: preview, isError: previewError } = useQuery({
    queryKey: ['hwidExtraPreview', target, previewProvider],
    queryFn: () => api.hwidExtraPreview(target, previewProvider),
    enabled: valid,
    staleTime: 20_000,
  })
  const amountSuffix = preview?.currency === 'STARS' ? t('checkout.stars') : t('checkout.rub')
  const hasDiscount = preview != null && preview.base_amount_rub > preview.amount_rub

  async function handlePay() {
    if (!selectedProvider || !valid) return
    setError(null)
    setLoading(true)
    // До первого await: иначе Safari заблокирует вкладку оплаты.
    const payWindow = reservePaymentWindow()
    try {
      const res = await api.hwidExtraCheckout(target, selectedProvider, newIdempotencyKey())
      if (payWindow.go(res.payment_url)) {
        navigate(`/payment/status/${res.checkout_id}`)
      }
    } catch (err) {
      payWindow.cancel()
      if (err instanceof ApiError) {
        setError(err.status === 429 ? t('errors.tooManyRequests') : t('checkout.notAvailable'))
      } else {
        setError(t('errors.unknown'))
      }
      setLoading(false)
    }
  }

  const tariffName = sub?.tariff?.name?.trim() || t('dashboard.basicPlan')
  const payDisabled = !selectedProvider || !valid || !preview || loading

  return (
    <AppLayout>
      <PageReveal className="max-w-lg mx-auto space-y-6">
        <RevealItem>
          <PageTitleWithBack title={t('checkout.title')} />
        </RevealItem>

        <RevealItem>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base font-medium text-muted-foreground">{t('checkout.summary')}</CardTitle>
            </CardHeader>
            <CardContent>
              {subLoading ? (
                <div className="space-y-2.5">
                  <Skeleton className="h-3.5 w-full" />
                  <Skeleton className="h-3.5 w-4/5" />
                  <Skeleton className="h-6 w-full" />
                </div>
              ) : !valid || !hwid ? (
                // Ссылку открыли повторно после оплаты или руками поменяли число.
                <div className="space-y-3">
                  <p className="text-sm text-muted-foreground">{t('checkout.notAvailable')}</p>
                  <Button asChild variant="outline" size="sm">
                    <Link to="/subscription">{t('subscriptionPage.title')}</Link>
                  </Button>
                </div>
              ) : (
                <div className="space-y-2 text-sm">
                  <SummaryRow label={t('checkout.scenario')} value={t('checkout.scenarioDeviceSlots')} />
                  <SummaryRow label={t('checkout.plan')} value={tariffName} />
                  <SummaryRow label={t('checkout.devices')} value={`${hwid.current_limit} → ${target}`} />
                  <SummaryRow
                    label={t('checkout.period')}
                    value={t('checkout.deviceSlotsTerm', {
                      date: formatDayMonth(sub?.expire_at, lang),
                      days: t('subscriptionPage.slotsDays', { count: preview?.days_left ?? hwid.days_left }),
                    })}
                  />
                  {previewError && <p className="text-xs text-muted-foreground">{t('checkout.previewError')}</p>}
                  {preview && hasDiscount && (preview.total_discount_pct ?? 0) > 0 && (
                    <div className="space-y-2 rounded-lg bg-muted/50 px-3 py-2 text-xs">
                      <SummaryRow
                        label={t('checkout.discountLabel')}
                        value={t('checkout.discountValueWithAmount', {
                          pct: preview.total_discount_pct,
                          amount: formatNumber(preview.base_amount_rub - preview.amount_rub),
                          unit: amountSuffix,
                        })}
                      />
                    </div>
                  )}
                  <div className="border-t border-border pt-2 mt-2">
                    <SummaryRow
                      label={t('checkout.total')}
                      value={
                        preview ? (
                          <span className="text-lg font-bold">
                            {formatNumber(preview.amount_rub)} {amountSuffix}
                          </span>
                        ) : (
                          <Skeleton className="h-6 w-20" />
                        )
                      }
                    />
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </RevealItem>

        {valid && (
          <RevealItem>
            <ProviderPickerCard
              providerEnabled={providerEnabled}
              provider={provider}
              selectedProvider={selectedProvider}
              onSelect={setProvider}
            />
          </RevealItem>
        )}

        {error && (
          <RevealItem>
            <Alert variant="destructive">
              <AlertCircle size={14} />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </RevealItem>
        )}

        {valid && (
          <CheckoutPayButton
            label={`${t('checkout.pay')}${preview ? ` ${formatNumber(preview.amount_rub)} ${amountSuffix}` : ''}`}
            disabled={payDisabled}
            loading={loading}
            onPay={handlePay}
          />
        )}
      </PageReveal>
    </AppLayout>
  )
}
