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
import { useErrorHaptic } from '@/lib/haptic'
import { formatNumber } from '@/lib/format'
import { newIdempotencyKey } from '@/lib/utils'
import { reservePaymentWindow } from '@/lib/payment-window'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'
import { formatUtcDayMonth as formatDayMonth } from '@/lib/splitDates'
import { bypassLabel } from '@/lib/bypassLabel'
import { packSizeLabel } from '@/features/home/trafficSplit'
import { CheckoutPayButton, SummaryRow } from '@/features/checkout/checkoutShared'
import { PaymentMethodPicker } from '@/features/checkout/PaymentMethodPicker'
import { useCheckoutMethod } from '@/features/checkout/paymentMethods'

/**
 * Оформление докупки трафика: та же страница, что при оплате подписки и мест
 * для устройств (сводка → способ оплаты → «Оплатить»). Пакет приходит из окна
 * докупки в `?pack=`; цену сервер берёт из пакета тарифа, а не из запроса.
 * Скидка (лояльность, промокод) считается при создании счёта — здесь показана цена пакета.
 */
export default function TrafficCheckoutPage() {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const packId = parseInt(searchParams.get('pack') ?? '', 10)

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  const { data: split, isLoading } = useQuery({
    queryKey: ['my-traffic'],
    queryFn: () => api.myTraffic(),
    staleTime: SUBSCRIPTION_STALE_MS,
  })
  const { data: sub } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: SUBSCRIPTION_STALE_MS,
  })
  const pack = split?.packs.find((p) => p.id === packId)
  const valid = Boolean(split?.can_buy && pack)

  const checkoutMethod = useCheckoutMethod()
  const { provider: selectedProvider } = checkoutMethod
  const stars = selectedProvider === 'telegram'
  // Цена с учётом скидок (лояльность, промокод) считается на сервере тем же кодом, что и счёт: иначе здесь
  // была бы цена без скидки, а к оплате выставлялась бы со скидкой.
  const previewProvider = selectedProvider ?? 'yookassa'
  const { data: preview, isError: previewError } = useQuery({
    queryKey: ['trafficPackPreview', packId, previewProvider],
    queryFn: () => api.trafficPackPreview(packId, previewProvider),
    enabled: valid && !Number.isNaN(packId),
    staleTime: 0,
    retry: 1,
  })
  // Если пересчёт не удался, показываем цену из каталога (как и сказано в подсказке): спишется фактическая сумма со скидкой.
  const catalogPrice = pack ? (stars && pack.amount_stars != null ? pack.amount_stars : pack.amount_rub) : 0
  const priceReady = Boolean(preview) || previewError
  const amount = preview?.amount_rub ?? (previewError ? catalogPrice : 0)
  const hasDiscount = preview != null && preview.base_amount_rub > preview.amount_rub
  const amountSuffix = stars ? t('checkout.stars') : t('checkout.rub')
  const kindName = pack?.kind === 'bypass' ? bypassLabel(split?.label) : t('trafficSplit.tabMain')
  // Для звёзд у пакета может не быть цены (курс не задан): рубли с пометкой «⭐» были бы враньём.
  const starsMissing = stars && Boolean(pack) && pack?.amount_stars == null

  async function handlePay() {
    if (!selectedProvider || !valid || !pack) return
    setError(null)
    setLoading(true)
    // До первого await: иначе Safari заблокирует вкладку оплаты.
    const payWindow = reservePaymentWindow()
    try {
      const res = await api.trafficPackCheckout(pack.id, selectedProvider, newIdempotencyKey(), checkoutMethod.yookassaMethod)
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
              {isLoading ? (
                <div className="space-y-2.5">
                  <Skeleton className="h-3.5 w-full" />
                  <Skeleton className="h-3.5 w-4/5" />
                  <Skeleton className="h-6 w-full" />
                </div>
              ) : !valid || !pack || !split ? (
                // Ссылку открыли повторно, пакет выключили или тариф сменился.
                <div className="space-y-3">
                  <p className="text-sm text-muted-foreground">{t('checkout.notAvailable')}</p>
                  <Button asChild variant="outline" size="sm">
                    <Link to="/dashboard">{t('trafficSplit.backHome')}</Link>
                  </Button>
                </div>
              ) : (
                <div className="space-y-2 text-sm">
                  <SummaryRow label={t('checkout.scenario')} value={t('trafficSplit.scenario')} />
                  <SummaryRow label={t('checkout.plan')} value={tariffName} />
                  <SummaryRow label={t('trafficSplit.packLabel')} value={`${kindName}: +${packSizeLabel(pack.bytes, t('dashboard.gigabytes'))}`} />
                  <SummaryRow
                    label={t('checkout.period')}
                    value={t('trafficSplit.packTerm', { date: formatDayMonth(split.reset_at, lang) })}
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
                        priceReady ? (
                          <span className="text-lg font-bold">
                            {formatNumber(amount)} {amountSuffix}
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
            <PaymentMethodPicker
              methods={checkoutMethod.methods}
              method={checkoutMethod.method}
              initialId={checkoutMethod.initial.id}
              initialSource={checkoutMethod.initial.source}
              loading={checkoutMethod.loading}
              onSelect={checkoutMethod.setMethod}
            />
          </RevealItem>
        )}

        {starsMissing && (
          <RevealItem>
            <Alert variant="destructive">
              <AlertCircle size={14} />
              <AlertDescription>{t('trafficSplit.noStars')}</AlertDescription>
            </Alert>
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
            label={`${t('checkout.pay')}${priceReady ? ` ${formatNumber(amount)} ${amountSuffix}` : ''}`}
            disabled={!selectedProvider || loading || starsMissing || !priceReady}
            loading={loading}
            onPay={handlePay}
          />
        )}
      </PageReveal>
    </AppLayout>
  )
}
