import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertCircle, CalendarClock, Gauge, Globe } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { api, ApiError, SUBSCRIPTION_STALE_MS } from '@/lib/api'
import { useErrorHaptic } from '@/lib/haptic'
import { formatNumber } from '@/lib/format'
import { newIdempotencyKey } from '@/lib/utils'
import { reservePaymentWindow } from '@/lib/payment-window'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'
import { formatUtcDayMonth as formatDayMonth } from '@/lib/splitDates'
import { bypassServersTitle } from '@/lib/bypassLabel'
import { packSizeGb, packSizeLabel } from '@/features/home/trafficSplit'
import {
  CheckoutLayout,
  IncludedList,
  IncludedRow,
  OrderHeader,
  OrderReceipt,
  OrderSkeleton,
  discountLines,
  type ReceiptMode,
} from '@/features/checkout/orderSummary'
import { PaymentMethodPicker, PaymentMethodRow } from '@/features/checkout/PaymentMethodPicker'
import { useCheckoutMethod } from '@/features/checkout/paymentMethods'

/**
 * Оформление докупки трафика: та же страница, что при оплате подписки и мест
 * для устройств (сводка → способ оплаты → «Оплатить»). Пакет приходит из окна
 * докупки в `?pack=`; цену сервер берёт из пакета тарифа, а не из запроса.
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
  const pack = split?.packs.find((p) => p.id === packId)
  const valid = Boolean(split?.can_buy && pack)

  const checkoutMethod = useCheckoutMethod()
  const { provider: selectedProvider } = checkoutMethod
  const stars = selectedProvider === 'telegram'
  // Цена с учётом скидок (лояльность, промокод) считается на сервере тем же кодом, что и счёт: иначе здесь
  // была бы цена без скидки, а к оплате выставлялась бы со скидкой.
  const previewProvider = selectedProvider ?? 'yookassa'
  const {
    data: preview,
    isError: previewError,
    isFetching,
    isPlaceholderData,
  } = useQuery({
    queryKey: ['trafficPackPreview', packId, previewProvider],
    queryFn: () => api.trafficPackPreview(packId, previewProvider),
    enabled: valid && !Number.isNaN(packId),
    staleTime: 0,
    retry: 1,
    placeholderData: keepPreviousData,
  })
  const recalculating = isFetching && isPlaceholderData
  // Если пересчёт не удался, показываем цену из каталога (как и сказано в подсказке): спишется фактическая сумма со скидкой.
  const catalogPrice = pack ? (stars && pack.amount_stars != null ? pack.amount_stars : pack.amount_rub) : 0
  const priceReady = Boolean(preview) || previewError
  const amount = preview?.amount_rub ?? (previewError ? catalogPrice : 0)
  // Валюта — из ответа (при пересчёте он прошлый, вместе со своей суммой), до ответа — по выбранному способу.
  const amountSuffix = (preview ? preview.currency === 'STARS' : stars) ? t('checkout.stars') : t('checkout.rub')
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
        navigate(`/payment/status/${res.checkout_id}`, {
          state: { paymentUrl: res.payment_url },
        })
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

  const size = pack ? packSizeLabel(pack.bytes, t('dashboard.gigabytes')) : ''

  let order
  if (isLoading) {
    order = <OrderSkeleton />
  } else if (!valid || !pack || !split) {
    // Ссылку открыли повторно, пакет выключили или тариф сменился.
    order = (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('checkout.notAvailable')}</p>
        <Button asChild variant="outline" size="sm">
          <Link to="/dashboard">{t('trafficSplit.backHome')}</Link>
        </Button>
      </div>
    )
  } else {
    order = (
      <>
        <OrderHeader scenario="addon" title={t('checkout.trafficTitle')} />
        <IncludedList>
          {pack.kind === 'bypass' ? (
            <IncludedRow
              icon={<Globe size={17} />}
              title={bypassServersTitle(packSizeGb(pack.bytes), split.label)}
              hint={t('checkout.bypassHint')}
            />
          ) : (
            <IncludedRow icon={<Gauge size={17} />} title={t('checkout.trafficPackMain', { size })} />
          )}
          <IncludedRow
            icon={<CalendarClock size={17} />}
            title={t('trafficSplit.packTerm', { date: formatDayMonth(split.reset_at, lang) })}
          />
        </IncludedList>
      </>
    )
  }

  const renderReceipt = (mode: ReceiptMode) => {
    if (!priceReady) return <OrderReceipt mode={mode} lines={[]} total={null} unit={amountSuffix} />
    const base = preview ? (preview.base_amount_rub > 0 ? preview.base_amount_rub : preview.amount_rub) : catalogPrice
    return (
      <OrderReceipt
        mode={mode}
        lines={[
          { key: 'pack', label: t('checkout.trafficPackReceipt', { size }), amount: base },
          ...(preview
            ? discountLines(t, {
                base,
                amount: preview.amount_rub,
                loyaltyPct: preview.loyalty_discount_pct,
                promoPct: preview.promo_discount_pct,
                totalPct: preview.total_discount_pct,
              })
            : []),
        ]}
        total={amount}
        strike={base}
        unit={amountSuffix}
        busy={recalculating}
        note={previewError && !preview ? t('checkout.previewError') : undefined}
      />
    )
  }

  const pickerProps = {
    methods: checkoutMethod.methods,
    method: checkoutMethod.method,
    initialId: checkoutMethod.initial.id,
    initialSource: checkoutMethod.initial.source,
    loading: checkoutMethod.loading,
    onSelect: checkoutMethod.setMethod,
  }
  // Способ не выбран (новый человек без способа по умолчанию): на кнопке вместо суммы подсказка.
  const needMethod = !checkoutMethod.loading && checkoutMethod.methods.length > 0 && !selectedProvider

  return (
    <AppLayout>
      <CheckoutLayout
        title={t('checkout.title')}
        order={order}
        receipt={isLoading || valid ? renderReceipt : null}
        equalHeights
        payment={valid ? <PaymentMethodPicker {...pickerProps} /> : undefined}
        paymentRow={valid ? <PaymentMethodRow {...pickerProps} /> : undefined}
        notices={
          starsMissing ? (
            <Alert variant="destructive">
              <AlertCircle size={14} />
              <AlertDescription>{t('trafficSplit.noStars')}</AlertDescription>
            </Alert>
          ) : undefined
        }
        error={error}
        pay={
          valid
            ? {
                label: needMethod
                  ? t('checkout.choosePaymentMethod')
                  : `${t('checkout.pay')}${priceReady ? ` ${formatNumber(amount)} ${amountSuffix}` : ''}`,
                // isPlaceholderData, а не recalculating: без сети запрос встаёт на паузу, а сумма остаётся прошлой.
                disabled: !selectedProvider || loading || starsMissing || !priceReady || isPlaceholderData,
                loading,
                onPay: handlePay,
              }
            : null
        }
      />
    </AppLayout>
  )
}
