import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CalendarClock, Smartphone } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { Button } from '@/components/ui/button'
import { api, ApiError, SUBSCRIPTION_STALE_MS } from '@/lib/api'
import { useErrorHaptic } from '@/lib/haptic'
import { formatNumber } from '@/lib/format'
import { newIdempotencyKey } from '@/lib/utils'
import { reservePaymentWindow } from '@/lib/payment-window'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'
import { formatDayMonth } from '@/features/subscription/DeviceSlots'
import { PROVIDER_ORDER, type Provider } from '@/features/checkout/checkoutShared'
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

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

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

  const checkoutMethod = useCheckoutMethod()
  const { providerEnabled, provider: selectedProvider } = checkoutMethod
  const availableProviders = PROVIDER_ORDER.filter((p) => providerEnabled[p])
  const previewProvider: Provider = selectedProvider ?? availableProviders[0] ?? 'yookassa'

  const {
    data: preview,
    isError: previewError,
    isFetching,
    isPlaceholderData,
  } = useQuery({
    queryKey: ['hwidExtraPreview', target, previewProvider],
    queryFn: () => api.hwidExtraPreview(target, previewProvider),
    enabled: valid,
    staleTime: 20_000,
    placeholderData: keepPreviousData,
  })
  const recalculating = isFetching && isPlaceholderData
  const amountSuffix = preview?.currency === 'STARS' ? t('checkout.stars') : t('checkout.rub')

  async function handlePay() {
    if (!selectedProvider || !valid) return
    setError(null)
    setLoading(true)
    // До первого await: иначе Safari заблокирует вкладку оплаты.
    const payWindow = reservePaymentWindow()
    try {
      const res = await api.hwidExtraCheckout(target, selectedProvider, newIdempotencyKey(), checkoutMethod.yookassaMethod)
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

  // isPlaceholderData, а не recalculating: без сети запрос встаёт на паузу, а сумма на кнопке остаётся прошлой.
  const payDisabled = !selectedProvider || !valid || !preview || loading || isPlaceholderData

  let order
  if (subLoading) {
    order = <OrderSkeleton />
  } else if (!valid || !hwid) {
    // Ссылку открыли повторно после оплаты или руками поменяли число.
    order = (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('checkout.notAvailable')}</p>
        <Button asChild variant="outline" size="sm">
          <Link to="/dashboard#devices">{t('home.myDevices')}</Link>
        </Button>
      </div>
    )
  } else {
    const delta = target - hwid.current_limit
    order = (
      <>
        <OrderHeader scenario="addon" title={t('checkout.slotsTitle')} />
        <IncludedList>
          <IncludedRow
            icon={<Smartphone size={17} />}
            title={t('checkout.slotsAdd', { count: delta })}
            hint={t('checkout.slotsChange', { from: hwid.current_limit, to: target })}
          />
          {/* Место живёт до конца текущей подписки — от числа дней зависит цена. */}
          <IncludedRow
            icon={<CalendarClock size={17} />}
            title={t('checkout.deviceSlotsTerm', {
              date: formatDayMonth(sub?.expire_at, lang),
              days: t('subscriptionPage.slotsDays', { count: preview?.days_left ?? hwid.days_left }),
            })}
          />
        </IncludedList>
      </>
    )
  }

  const renderReceipt = (mode: ReceiptMode) => {
    if (!preview) {
      return (
        <OrderReceipt
          mode={mode}
          lines={[]}
          total={null}
          unit={amountSuffix}
          note={previewError ? t('checkout.previewFailed') : undefined}
        />
      )
    }
    const base = preview.base_amount_rub > 0 ? preview.base_amount_rub : preview.amount_rub
    return (
      <OrderReceipt
        mode={mode}
        lines={[
          { key: 'slots', label: t('checkout.slotsReceipt', { count: preview.delta }), amount: base },
          ...discountLines(t, {
            base,
            amount: preview.amount_rub,
            loyaltyPct: preview.loyalty_discount_pct,
            promoPct: preview.promo_discount_pct,
            totalPct: preview.total_discount_pct,
          }),
        ]}
        total={preview.amount_rub}
        strike={base}
        unit={amountSuffix}
        busy={recalculating}
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
        receipt={subLoading || valid ? renderReceipt : null}
        equalHeights
        payment={valid ? <PaymentMethodPicker {...pickerProps} /> : undefined}
        paymentRow={valid ? <PaymentMethodRow {...pickerProps} /> : undefined}
        error={error}
        pay={
          valid
            ? {
                label: needMethod
                  ? t('checkout.choosePaymentMethod')
                  : `${t('checkout.pay')}${preview ? ` ${formatNumber(preview.amount_rub)} ${amountSuffix}` : ''}`,
                disabled: payDisabled,
                loading,
                onPay: handlePay,
              }
            : null
        }
      />
    </AppLayout>
  )
}
