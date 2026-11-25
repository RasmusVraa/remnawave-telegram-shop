import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Gauge, Globe, Infinity as InfinityIcon, Smartphone, TriangleAlert } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { api, ApiError, type PaymentPreviewResponse, type TariffItem } from '@/lib/api'
import { hapticSelection, useErrorHaptic } from '@/lib/haptic'
import { cn, newIdempotencyKey } from '@/lib/utils'
import { reservePaymentWindow } from '@/lib/payment-window'
import { bypassServersTitle } from '@/lib/bypassLabel'
import { formatNumber } from '@/lib/format'
import { AdminToggleSwitch } from '@/features/admin/components/AdminToggleSwitch'
import { PROVIDER_ORDER, type Provider } from '@/features/checkout/checkoutShared'
import {
  CheckoutLayout,
  IncludedList,
  IncludedRow,
  OrderHeader,
  OrderReceipt,
  OrderSkeleton,
  discountLines,
  displayScenario,
  type ReceiptLine,
  type ReceiptMode,
} from '@/features/checkout/orderSummary'
import { PaymentMethodPicker, PaymentMethodRow } from '@/features/checkout/PaymentMethodPicker'
import { TariffLocationPicker } from '@/features/checkout/TariffLocationPicker'
import { useCheckoutMethod } from '@/features/checkout/paymentMethods'

export default function CheckoutPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const tariffSlug = searchParams.get('tariff') ?? ''
  const months = parseInt(searchParams.get('months') ?? '1', 10)

  // Выбор человека по тумблеру доп. мест; null — не трогал (тогда места продлеваются).
  const [extraChoice, setExtraChoice] = useState<boolean | null>(null)
  const [switchOpen, setSwitchOpen] = useState(false)
  const [locationId, setLocationId] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  // Fetch tariffs to get the selected tariff details.
  const { data: tariffsData, isLoading: tariffsLoading } = useQuery({
    queryKey: ['tariffs'],
    queryFn: () => api.tariffs(),
    staleTime: 5 * 60_000,
  })
  const { data: subscription, isError: subscriptionError } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: 30_000,
  })
  // Пока подписка не пришла, неизвестно, есть ли доп. места: предпросмотр без них
  // ушёл бы зря и на мгновение показал сумму без мест.
  const subscriptionReady = subscription !== undefined || subscriptionError
  const extraHwidActive = subscription?.hwid_extra?.extra_active ?? 0
  const shouldAskExtraRenew = extraHwidActive > 0

  // Докупленные места по умолчанию продлеваются: человек продлевает то, что
  // у него уже есть. Цена мест видна в строке переключателя и входит в «Итого».
  // Считается из выбора, а не ставится эффектом: эффект на один кадр отставал
  // от подписки, и успевал уйти лишний предпросмотр без мест.
  const renewExtraHwid = shouldAskExtraRenew && (extraChoice ?? true)

  const tariff: TariffItem | undefined = tariffsData?.tariffs.find(
    (t) => t.slug === tariffSlug && t.months === months,
  ) ?? tariffsData?.tariffs.find((t) => t.slug === tariffSlug)

  const locations = tariff?.locations ?? []
  const needsLocation = locations.length > 0
  const allLocationsSoldOut = needsLocation && locations.every((loc) => loc.sold_out)
  const locationReady = !needsLocation || (locationId != null && locationId > 0 && !allLocationsSoldOut)

  const previewTariffId =
    tariffsData?.sales_mode === 'tariffs' && tariff != null && tariff.id != null && tariff.id > 0
      ? tariff.id
      : undefined

  const checkoutMethod = useCheckoutMethod()
  const { providerEnabled, provider: selectedProvider } = checkoutMethod
  const defaultPreviewProvider =
    PROVIDER_ORDER.find((p) => providerEnabled[p]) ?? ('yookassa' as Provider)
  const selectedProviderForPreview: Provider = selectedProvider ?? defaultPreviewProvider
  const extraOn = renewExtraHwid
  // keepPreviousData: при переключении тумблера или способа оплаты прошлый ответ
  // остаётся на экране (бледным), а не подменяется на секунду ценой из каталога.
  const {
    data: preview,
    isError: previewError,
    isFetching: previewFetching,
    isPlaceholderData: previewStale,
  } = useQuery({
    queryKey: [
      'paymentPreview',
      tariff?.months,
      previewTariffId,
      tariffsData?.sales_mode,
      selectedProviderForPreview,
      renewExtraHwid,
    ],
    queryFn: () =>
      api.paymentPreview(
        tariff!.months,
        previewTariffId,
        selectedProviderForPreview,
        renewExtraHwid,
      ),
    enabled:
      Boolean(tariff) && !tariffsLoading && subscriptionReady,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })
  // Спиннер — пока запрос идёт; кнопка — пока на экране прошлый ответ (в том числе
  // когда запрос встал на паузу без сети и isFetching уже false).
  const recalculating = previewFetching && previewStale

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

  const amountSuffix = preview?.currency === 'STARS' ? t('checkout.stars') : t('checkout.rub')
  const scenario = displayScenario(preview?.scenario, subscription?.is_trial)
  const termMonths = tariff?.months ?? months
  const monthsLabel = t('checkout.months', { count: termMonths })
  // В заголовке и чеке коротко: «Премиум · 12 мес.» (решение владельца).
  const monthsShort = t('checkout.monthsShort', { count: termMonths })
  // В classic у «тарифа» нет имени: normalizeTariffsResponse подставляет slug «classic».
  const tariffTitle =
    tariffsData?.sales_mode === 'tariffs' && tariff?.name?.trim() ? tariff.name.trim() : t('checkout.subscriptionFallback')

  // Если пересчёт не удался — цена из каталога (в рублях) и доп. места по цене
  // из подписки; при оплате всё равно спишется сумма, посчитанная сервером.
  const catalogExtra = extraOn ? (subscription?.hwid_extra?.price_rub_month ?? 0) * extraHwidActive * termMonths : 0
  const catalogTotal = (tariff?.price_rub ?? 0) + catalogExtra
  const fallback = previewError && !preview
  const payAmount = preview ? preview.amount_rub : fallback ? catalogTotal : null
  const payUnit = preview ? amountSuffix : t('checkout.rub')

  async function handlePay() {
    if (!selectedProvider) return
    if (!tariff) { setError(t('errors.unknown')); return }
    if (needsLocation && (locationId == null || locationId <= 0)) {
      setError(t('checkout.locationRequired'))
      return
    }
    if (allLocationsSoldOut) {
      setError(t('checkout.allLocationsSoldOut'))
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
          renewExtraHwid,
          locationId: needsLocation ? locationId : undefined,
        },
        idempotencyKey,
      )
      if (payWindow.go(res.payment_url)) {
        navigate(`/payment/status/${res.checkout_id}`, {
          state: { paymentUrl: res.payment_url },
        })
      }
    } catch (err) {
      payWindow.cancel()
      if (err instanceof ApiError) {
        if (err.status === 429) {
          setError(t('errors.tooManyRequests'))
        } else if (err.status === 400 || err.status === 422) {
          const detail = `${err.body} ${err.message}`
          if (detail.includes('location_required')) {
            setError(t('checkout.locationRequired'))
          } else if (detail.includes('location_sold_out') || detail.includes('location_invalid')) {
            setError(t('checkout.locationInvalid'))
          } else {
            setError(t('checkout.notAvailable'))
          }
        } else {
          setError(t('errors.unknown'))
        }
      } else {
        setError(t('errors.unknown'))
      }
      setLoading(false)
    }
  }

  const showTariffSwitchBreakdown =
    preview != null &&
    (preview.scenario === 'upgrade' || preview.scenario === 'downgrade') &&
    typeof preview.tariff_switch_total_days === 'number'
  const canInitiatePay = Boolean(selectedProvider) && Boolean(tariff) && payAmount != null && locationReady
  // Пока на экране прошлый ответ, на кнопке его сумма — нажимать нельзя.
  const payButtonDisabled = !canInitiatePay || loading || previewStale

  const payVerb =
    scenario === 'renew'
      ? t('checkout.payRenew')
      : scenario === 'upgrade' || scenario === 'downgrade'
        ? t('checkout.payUpgrade')
        : t('checkout.pay')

  // ── Чек ──
  function receiptLines(p: PaymentPreviewResponse): ReceiptLine[] {
    const includedExtra = p.extra_hwid_included ? (p.extra_hwid_amount_rub ?? 0) : 0
    const base = p.base_amount_rub && p.base_amount_rub > 0 ? p.base_amount_rub : p.amount_rub
    const lines: ReceiptLine[] = [
      {
        key: 'tariff',
        label: t('checkout.receiptTariff', { tariff: tariffTitle, period: monthsShort }),
        amount: base - includedExtra,
      },
    ]
    // Строка мест следует тумблеру сразу; сумма — из ответа сервера (при пересчёте — прошлого).
    const extraRow = extraOn ? (p.extra_hwid_included ? includedExtra : extraHwidAmount) : 0
    if (extraOn && extraRow != null && extraRow > 0) {
      lines.push({ key: 'extra', label: t('checkout.receiptExtra', { count: extraHwidActive }), amount: extraRow })
    }
    return lines.concat(
      discountLines(t, {
        base,
        amount: p.amount_rub,
        loyaltyPct: p.loyalty_discount_pct,
        promoPct: p.promo_discount_pct,
        totalPct: p.total_discount_pct,
      }),
    )
  }

  const renderReceipt = (mode: ReceiptMode) => {
    if (preview) {
      const base = preview.base_amount_rub && preview.base_amount_rub > 0 ? preview.base_amount_rub : preview.amount_rub
      return (
        <OrderReceipt
          mode={mode}
          lines={receiptLines(preview)}
          total={preview.amount_rub}
          strike={base}
          unit={amountSuffix}
          busy={recalculating}
        />
      )
    }
    if (fallback && tariff) {
      const lines: ReceiptLine[] = [
        { key: 'tariff', label: t('checkout.receiptTariff', { tariff: tariffTitle, period: monthsShort }), amount: tariff.price_rub },
      ]
      if (catalogExtra > 0) {
        lines.push({ key: 'extra', label: t('checkout.receiptExtra', { count: extraHwidActive }), amount: catalogExtra })
      }
      return (
        <OrderReceipt mode={mode} lines={lines} total={catalogTotal} unit={t('checkout.rub')} note={t('checkout.previewError')} />
      )
    }
    return <OrderReceipt mode={mode} lines={[]} total={null} unit={amountSuffix} />
  }

  // ── Карточка заказа ──
  let order
  if (tariffsLoading) {
    order = <OrderSkeleton />
  } else if (!tariff) {
    order = <p className="text-sm text-destructive">{t('errors.unknown')}</p>
  } else {
    const hwidMax = subscription?.hwid_extra?.max_limit ?? 0
    let devices = tariff.device_limit + (extraOn ? extraHwidActive : 0)
    // Сервер режет лимит по HWID_MAX_DEVICES так же.
    if (extraOn && hwidMax > 0) devices = Math.min(devices, Math.max(hwidMax, tariff.device_limit))
    const extraShown = devices - tariff.device_limit
    const hasBypass = tariff.bypass_gb != null && tariff.bypass_gb > 0

    order = (
      <>
        <OrderHeader scenario={scenario} title={`${tariffTitle} · ${monthsShort}`} />

        <IncludedList>
          {tariff.device_limit > 0 && (
            <IncludedRow
              icon={<Smartphone size={17} />}
              title={t('checkout.devicesTitle', { count: devices })}
              hint={extraShown > 0 ? t('checkout.devicesBreakdown', { base: tariff.device_limit, count: extraShown }) : undefined}
            />
          )}
          {tariff.traffic_gb != null && tariff.traffic_gb > 0 ? (
            <IncludedRow icon={<Gauge size={17} />} title={t('checkout.trafficLimited', { gb: formatNumber(tariff.traffic_gb) })} />
          ) : (
            <IncludedRow icon={<InfinityIcon size={17} />} title={t('checkout.trafficUnlimitedTitle')} />
          )}
          {hasBypass && (
            <IncludedRow
              icon={<Globe size={17} />}
              title={bypassServersTitle(formatNumber(tariff.bypass_gb!), tariff.bypass_label)}
              hint={t('checkout.bypassHint')}
              hintFromSm
            />
          )}
        </IncludedList>

        {needsLocation && (
          <div className="mt-4">
            <TariffLocationPicker
              locations={locations}
              locationId={locationId}
              onSelect={(id) => {
                hapticSelection()
                setLocationId(id)
                setError(null)
              }}
            />
          </div>
        )}

        {shouldAskExtraRenew && (
          <>
            <div
              className={cn(
                'mt-3 flex items-center justify-between gap-3 rounded-[16px] border bg-muted/60 px-3.5 py-2.5 transition-colors sm:mt-3.5 sm:py-3 dark:bg-white/[0.04]',
                extraOn ? 'border-primary/50' : 'border-border',
              )}
            >
              <div className="min-w-0">
                <p className="font-semibold leading-snug">{t('checkout.keepExtraTitle', { count: extraHwidActive })}</p>
                <p className="mt-0.5 text-[13px] text-muted-foreground">
                  {extraHwidAmount != null
                    ? termMonths === 1
                      ? t('checkout.keepExtraHintMonth', { amount: formatNumber(extraHwidAmount), unit: extraHwidUnit })
                      : t('checkout.keepExtraHintPeriod', {
                          amount: formatNumber(extraHwidAmount),
                          unit: extraHwidUnit,
                          period: monthsLabel,
                        })
                    : t('subscriptionPage.slotsCount', { count: extraHwidActive })}
                </p>
              </div>
              <AdminToggleSwitch
                checked={renewExtraHwid}
                onChange={(v) => {
                  hapticSelection()
                  setExtraChoice(v)
                }}
                aria-label={t('checkout.keepExtraTitle', { count: extraHwidActive })}
              />
            </div>
            {/* Оплата без мест сразу возвращает лимит к базе (applyExtraAfterSubscription, в т.ч. без tariff_id после 958fccb3). */}
            {!extraOn && (
              <p className="mt-2 flex gap-1.5 text-[13px] text-amber-700 dark:text-amber-400">
                <TriangleAlert size={14} className="mt-[2px] shrink-0" aria-hidden />
                {t('checkout.extraWillBeLost', { count: extraHwidActive })}
              </p>
            )}
          </>
        )}

        {showTariffSwitchBreakdown && preview && (
          <TariffSwitchBreakdown
            periodDays={
              preview.tariff_switch_period_days ??
              (preview.tariff_switch_total_days ?? 0) - (preview.tariff_switch_bonus_days ?? 0)
            }
            remainingDays={preview.tariff_switch_remaining_days ?? 0}
            bonusDays={preview.tariff_switch_bonus_days ?? 0}
            totalDays={preview.tariff_switch_total_days ?? 0}
            open={switchOpen}
            onToggle={() => setSwitchOpen((v) => !v)}
          />
        )}
        {preview?.is_early_downgrade && !showTariffSwitchBreakdown && (
          <p className="mt-3 text-xs text-amber-700 dark:text-amber-400">{t('checkout.earlyDowngradeHint')}</p>
        )}
        {preview?.scenario === 'upgrade' && !showTariffSwitchBreakdown && (
          <p className="mt-3 text-xs text-amber-700 dark:text-amber-400">{t('checkout.earlyUpgradeHint')}</p>
        )}
      </>
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
  const needLocation = needsLocation && !locationReady && !allLocationsSoldOut

  return (
    <AppLayout>
      <CheckoutLayout
        title={t('checkout.title')}
        order={order}
        receipt={!tariffsLoading && !tariff ? null : renderReceipt}
        // Раскрытая разбивка срока делает карточку заказа заметно выше — «Расчёт» вытянулся бы пустым.
        equalHeights={!showTariffSwitchBreakdown || !switchOpen}
        payment={<PaymentMethodPicker {...pickerProps} />}
        paymentRow={<PaymentMethodRow {...pickerProps} />}
        error={error}
        pay={{
          label: allLocationsSoldOut
            ? t('checkout.allLocationsSoldOut')
            : needLocation
              ? t('checkout.locationRequired')
              : needMethod
                ? t('checkout.choosePaymentMethod')
                : payAmount != null
                  ? `${payVerb} ${formatNumber(payAmount)} ${payUnit}`
                  : payVerb,
          disabled: payButtonDisabled,
          loading,
          onPay: handlePay,
        }}
      />
    </AppLayout>
  )
}

/**
 * «Как считается срок после оплаты». Сервер считает итог как оплаченный период
 * плюс остаток, пересчитанный по цене нового тарифа (TariffSwitchTotalDays =
 * PeriodDays + BonusDays). Остаток текущего периода — не слагаемое, а то, из
 * чего получился бонус, поэтому он стоит в подписи строки, а не отдельной строкой.
 */
function TariffSwitchBreakdown({
  periodDays,
  remainingDays,
  bonusDays,
  totalDays,
  open,
  onToggle,
}: {
  periodDays: number
  remainingDays: number
  bonusDays: number
  totalDays: number
  open: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  // Свёрнут до строки итога, разбивка — по тапу (решение владельца: меньше
  // лишнего текста; на телефоне иначе чек с ценой уходит под плашку оплаты).
  return (
    <div className="mt-3 rounded-[16px] border border-border text-sm sm:mt-3.5">
      <button
        type="button"
        className="flex w-full items-center justify-between gap-3 px-3.5 py-3 text-left font-semibold"
        aria-expanded={open}
        onClick={() => {
          hapticSelection()
          onToggle()
        }}
      >
        <span>{t('checkout.switchTotal')}</span>
        <span className="flex shrink-0 items-center gap-1.5 whitespace-nowrap text-primary">
          {t('checkout.switchDaysExact', { days: totalDays })}
          <ChevronDown
            size={16}
            className={cn('text-muted-foreground transition-transform', open && 'rotate-180')}
            aria-hidden
          />
        </span>
      </button>
      {open && (
        <div className="border-t border-border px-3.5 pb-2.5 pt-2">
          <div className="flex justify-between gap-3 py-[3px] text-muted-foreground">
            <span>{t('checkout.switchPeriod')}</span>
            <span className="shrink-0 whitespace-nowrap text-foreground">
              {t('checkout.switchDaysExact', { days: periodDays })}
            </span>
          </div>
          <div className="flex justify-between gap-3 py-[3px] text-muted-foreground">
            <span>{t('checkout.switchRemainder', { days: remainingDays })}</span>
            <span className="shrink-0 whitespace-nowrap text-emerald-600 dark:text-emerald-400">
              {t('checkout.switchDaysBonus', { days: bonusDays })}
            </span>
          </div>
        </div>
      )}
    </div>
  )
}
