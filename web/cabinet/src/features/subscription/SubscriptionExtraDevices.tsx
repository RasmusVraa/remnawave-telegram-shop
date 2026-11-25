import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, ChevronDown, ChevronRight, Cpu, CreditCard, Bitcoin, Smartphone, Star, X } from 'lucide-react'

import { HeleketBrandIcon } from '@/components/BrandIcons'
import { LegalContinueDisclaimer } from '@/components/LegalContinueDisclaimer'
import { PlategaPaymentExpand, enabledPlategaMethods, type PlategaMethodId } from '@/components/PlategaPaymentExpand'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { api, ApiError, type HwidExtraPreviewResponse, type SubscriptionHwidExtraInfo } from '@/lib/api'
import { newIdempotencyKey, cn } from '@/lib/utils'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'

type Panel = 'menu' | 'buy' | 'decrease'
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

function hwidSubpanelGridCls(open: boolean) {
  return cn(
    'grid overflow-hidden transition-[grid-template-rows,opacity,transform] duration-[400ms] ease-out',
    open
      ? 'visible grid-rows-[1fr] opacity-100 translate-y-0'
      : 'invisible grid-rows-[0fr] opacity-0 -translate-y-1 pointer-events-none',
  )
}

const HWID_PROVIDER_ORDER: Provider[] = [
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

type Props = {
  hwid: SubscriptionHwidExtraInfo
  inactive: boolean
  onUpdated: () => void
}

function HwidProviderPayButton({
  selected,
  onClick,
  icon,
  label,
  hint,
}: {
  selected: boolean
  onClick: () => void
  icon: ReactNode
  label: string
  hint: string
}) {
  return (
    <Button
      type="button"
      size="sm"
      variant={selected ? 'default' : 'outline'}
      onClick={onClick}
      className="flex h-auto min-h-9 items-start gap-2 px-3 py-2 text-left font-normal"
    >
      <span className="shrink-0 pt-0.5 [&_svg]:block">{icon}</span>
      <span className="flex min-w-0 flex-col items-start gap-0.5">
        <span className="text-sm font-medium leading-tight">{label}</span>
        <span
          className={cn(
            'text-xs font-normal leading-snug',
            selected ? 'text-primary-foreground/75' : 'text-muted-foreground',
          )}
        >
          {hint}
        </span>
      </span>
    </Button>
  )
}

function hwidPreviewDiscountCaption(
  t: (key: string, opts?: Record<string, string | number>) => string,
  preview: HwidExtraPreviewResponse,
  hasCut: boolean,
): string | null {
  if (!hasCut) return null
  const loy = preview.loyalty_discount_pct ?? 0
  const pro = preview.promo_discount_pct ?? 0
  const parts: string[] = []
  if (loy > 0) parts.push(t('subscriptionPage.extraDevicesPayDiscountLoyalty', { pct: loy }))
  if (pro > 0) parts.push(t('subscriptionPage.extraDevicesPayDiscountPromo', { pct: pro }))
  if (parts.length > 0) {
    return `${t('subscriptionPage.extraDevicesPayDiscountPrefix')} ${parts.join(' / ')}`
  }
  const totalPct = preview.total_discount_pct ?? 0
  if (totalPct > 0) {
    return t('subscriptionPage.extraDevicesPayDiscountCombined', { pct: totalPct })
  }
  return null
}

export function SubscriptionExtraDevices({ hwid, inactive, onUpdated }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: bootstrap } = useAuthBootstrap()
  const [panel, setPanel] = useState<Panel>('menu')
  const [buyTarget, setBuyTarget] = useState(() =>
    hwid.can_increase ? Math.min(hwid.current_limit + 1, hwid.max_limit) : hwid.current_limit,
  )
  const [decTarget, setDecTarget] = useState(() =>
    hwid.can_decrease ? hwid.current_limit - 1 : hwid.base_limit,
  )
  const [provider, setProvider] = useState<Provider>('yookassa')
  const [payError, setPayError] = useState<string | null>(null)
  const [payLoading, setPayLoading] = useState(false)
  const [decreaseModalMounted, setDecreaseModalMounted] = useState(false)
  const [decreaseModalVisible, setDecreaseModalVisible] = useState(false)
  const decreaseModalUnmountAfterTransition = useRef(false)
  const decreaseModalVisibleRef = useRef(false)

  const providerEnabled: Record<Provider, boolean> = {
    yookassa: bootstrap?.payment_providers?.yookassa ?? true,
    cryptopay: bootstrap?.payment_providers?.cryptopay ?? true,
    telegram: bootstrap?.payment_providers?.telegram ?? true,
    platega_sbp: bootstrap?.payment_providers?.platega_sbp ?? false,
    platega_cards: bootstrap?.payment_providers?.platega_cards ?? false,
    platega_acquiring: bootstrap?.payment_providers?.platega_acquiring ?? false,
    platega_worldwide: bootstrap?.payment_providers?.platega_worldwide ?? false,
    platega_crypto: bootstrap?.payment_providers?.platega_crypto ?? false,
    heleket: bootstrap?.payment_providers?.heleket ?? false,
  }
  const firstHwidProvider = HWID_PROVIDER_ORDER.find((p) => providerEnabled[p]) ?? 'yookassa'

  const buyOptions = useMemo(() => {
    const out: number[] = []
    if (!hwid.can_increase || hwid.max_limit <= 0) return out
    for (let n = hwid.current_limit + 1; n <= hwid.max_limit; n++) out.push(n)
    return out
  }, [hwid])

  const decOptions = useMemo(() => {
    const out: number[] = []
    if (!hwid.can_decrease) return out
    for (let n = hwid.base_limit; n < hwid.current_limit; n++) out.push(n)
    return out
  }, [hwid])

  const { data: preview, isFetching, isPending } = useQuery({
    queryKey: ['hwidExtraPreview', buyTarget, provider],
    queryFn: () => api.hwidExtraPreview(buyTarget, provider),
    enabled: panel === 'buy' && buyTarget > hwid.current_limit && buyOptions.length > 0,
    staleTime: 20_000,
  })

  function openDecreaseConfirmModal() {
    if (decreaseModalMounted) return
    decreaseModalUnmountAfterTransition.current = false
    setDecreaseModalMounted(true)
    setDecreaseModalVisible(false)
  }

  useEffect(() => {
    decreaseModalVisibleRef.current = decreaseModalVisible
  }, [decreaseModalVisible])

  useEffect(() => {
    if (!decreaseModalMounted) return
    const id = requestAnimationFrame(() => {
      requestAnimationFrame(() => setDecreaseModalVisible(true))
    })
    return () => cancelAnimationFrame(id)
  }, [decreaseModalMounted])

  function closeDecreaseConfirmModal() {
    if (!decreaseModalMounted) return
    decreaseModalUnmountAfterTransition.current = true
    if (!decreaseModalVisibleRef.current) {
      decreaseModalUnmountAfterTransition.current = false
      setDecreaseModalMounted(false)
      return
    }
    setDecreaseModalVisible(false)
  }

  function onDecreaseModalOverlayTransitionEnd(e: React.TransitionEvent<HTMLDivElement>) {
    if (e.target !== e.currentTarget) return
    if (e.propertyName !== 'opacity') return
    if (!decreaseModalUnmountAfterTransition.current) return
    decreaseModalUnmountAfterTransition.current = false
    setDecreaseModalMounted(false)
  }

  const applyDec = useMutation({
    mutationFn: (target: number) => api.hwidExtraApply(target),
    onSuccess: () => {
      closeDecreaseConfirmModal()
      onUpdated()
      setPanel('menu')
    },
    onError: () => {
      closeDecreaseConfirmModal()
    },
  })

  const closeSubpanel = () => {
    setPayError(null)
    closeDecreaseConfirmModal()
    setPanel('menu')
  }

  if (!hwid.ui_visible || !hwid.enabled) return null

  const limitLine = t('subscriptionPage.extraDevicesCurrentLimit', { n: hwid.current_limit })

  const previewUnit =
    preview?.currency === 'STARS' ? t('checkout.stars') : t('checkout.rub')

  // Скруглённый квадрат, как у строк устройств и разделов кабинета,
  // а не круг — чтобы блоки на странице читались одним набором.
  const deviceOptionIcon = (
    <div
      className="cabinet-icon-box flex size-9 shrink-0 items-center justify-center rounded-lg"
      aria-hidden
    >
      <Smartphone size={16} strokeWidth={2} />
    </div>
  )

  return (
    <Card
      className={cn(
        // Изумрудный акцент: докупка — про «добавить», а не про состояние подписки.
        'cabinet-elevated-card cabinet-accent-emerald',
        inactive && 'opacity-60 saturate-50 pointer-events-none',
      )}
    >
      <CardHeader className="px-4 pb-3 pt-4">
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="flex min-w-0 flex-1 items-center gap-2 text-sm font-semibold text-foreground">
            <Cpu size={15} className="shrink-0 text-primary" />
            <span className="leading-snug">{t('subscriptionPage.extraDevicesTitle')}</span>
          </CardTitle>
          {panel !== 'menu' && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8 shrink-0 rounded-full"
              aria-label={t('common.close')}
              onClick={closeSubpanel}
            >
              <X size={18} />
            </Button>
          )}
        </div>
      </CardHeader>
      {/* Дефолт CardContent — p-6; здесь нужен 1rem, как у CardHeader выше,
          иначе строки оказываются вложены глубже заголовка карточки. */}
      <CardContent className="space-y-3 px-4 pb-4">
        {inactive && (
          <p className="text-sm text-muted-foreground">{t('subscriptionPage.unavailableWhileInactive')}</p>
        )}

        {!inactive && (
          <div className="flex flex-col gap-0">
            <div className={hwidSubpanelGridCls(panel === 'menu')}>
              <div className="min-h-0 overflow-hidden">
                {/* На широком экране две опции встают в ряд: каждая занимает
                    половину, а когда доступна только одна — всю ширину, как и
                    прежде. На мобиле остаётся столбик. */}
                <div className="flex flex-col gap-2.5 sm:flex-row">
                  {hwid.can_increase && (
                    <button
                      type="button"
                      className="cabinet-row group flex w-full items-center gap-3 rounded-xl border border-border/70 bg-muted/25 p-2 text-left text-card-foreground hover:border-transparent sm:w-auto sm:flex-1"
                      onClick={() => {
                        setBuyTarget(buyOptions[0] ?? hwid.current_limit + 1)
                        setProvider(firstHwidProvider)
                        setPanel('buy')
                      }}
                    >
                      {deviceOptionIcon}
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium">{t('subscriptionPage.extraDevicesBuyTitle')}</p>
                        <p className="text-xs text-muted-foreground">{limitLine}</p>
                      </div>
                      <ChevronRight
                        size={16}
                        className="shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                      />
                    </button>
                  )}
                  {hwid.can_decrease && (
                    <button
                      type="button"
                      className="cabinet-row group flex w-full items-center gap-3 rounded-xl border border-border/70 bg-muted/25 p-2 text-left text-card-foreground hover:border-transparent sm:w-auto sm:flex-1"
                      onClick={() => {
                        const last = decOptions[decOptions.length - 1] ?? hwid.base_limit
                        setDecTarget(last)
                        setPanel('decrease')
                      }}
                    >
                      {deviceOptionIcon}
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium">{t('subscriptionPage.extraDevicesDecreaseTitle')}</p>
                        <p className="text-xs text-muted-foreground">{t('subscriptionPage.extraDevicesDecreaseHint')}</p>
                      </div>
                      <ChevronRight
                        size={16}
                        className="shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                      />
                    </button>
                  )}
                  {!hwid.can_increase && !hwid.can_decrease && (
                    <p className="text-sm text-muted-foreground">{t('subscriptionPage.extraDevicesNoActions')}</p>
                  )}
                </div>
              </div>
            </div>

            <div className={hwidSubpanelGridCls(panel === 'buy')}>
              <div className="min-h-0 overflow-hidden">
                <div className="space-y-4 pt-0">
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('subscriptionPage.extraDevicesNewLimit')}</label>
              <div className="relative mt-1.5">
                <select
                  className="w-full cursor-pointer appearance-none rounded-lg border border-input bg-background px-3 py-2 pr-10 text-sm"
                  value={buyTarget}
                  onChange={(e) => setBuyTarget(parseInt(e.target.value, 10))}
                >
                  {buyOptions.map((n) => (
                    <option key={n} value={n}>
                      {n}
                    </option>
                  ))}
                </select>
                <ChevronDown
                  size={16}
                  className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground"
                />
              </div>
            </div>
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-2">{t('checkout.chooseProvider')}</p>
              <div className="flex flex-wrap gap-2 items-start">
                {providerEnabled.yookassa && (
                  <HwidProviderPayButton
                    selected={provider === 'yookassa'}
                    onClick={() => setProvider('yookassa')}
                    icon={<CreditCard size={14} />}
                    label={t('checkout.card')}
                    hint={t('checkout.providerHintYookassa')}
                  />
                )}
                {providerEnabled.cryptopay && (
                  <HwidProviderPayButton
                    selected={provider === 'cryptopay'}
                    onClick={() => setProvider('cryptopay')}
                    icon={<Bitcoin size={14} />}
                    label={t('checkout.crypto')}
                    hint={t('checkout.providerHintCryptopay')}
                  />
                )}
                {enabledPlategaMethods(providerEnabled).length > 0 && (
                  <PlategaPaymentExpand
                    enabled={providerEnabled}
                    selected={provider}
                    onSelect={(id: PlategaMethodId) => setProvider(id)}
                    variant="compact"
                  />
                )}
                {providerEnabled.telegram && (
                  <HwidProviderPayButton
                    selected={provider === 'telegram'}
                    onClick={() => setProvider('telegram')}
                    icon={<Star size={14} />}
                    label={t('checkout.telegramStars')}
                    hint={t('checkout.providerHintTelegram')}
                  />
                )}
                {providerEnabled.heleket && (
                  <HwidProviderPayButton
                    selected={provider === 'heleket'}
                    onClick={() => setProvider('heleket')}
                    icon={<HeleketBrandIcon className="h-[14px] w-[14px]" />}
                    label={t('checkout.otherCrypto')}
                    hint={t('checkout.providerHintHeleket')}
                  />
                )}
              </div>
            </div>
            {(isPending || (isFetching && !preview)) && (
              <p className="text-sm text-muted-foreground">{t('common.loading')}</p>
            )}
            {preview && (
              <div className="space-y-2 rounded-lg border border-border bg-muted/25 px-3 py-2.5 text-sm leading-snug">
                <p className="font-medium">
                  {t('subscriptionPage.extraDevicesPaySummary', {
                    count: preview.delta,
                    from: preview.current_limit,
                    to: preview.target_limit,
                    max: hwid.max_limit,
                  })}
                </p>
                <p className="text-muted-foreground">
                  {t('subscriptionPage.extraDevicesPayPeriod', {
                    count: preview.days_left,
                    amount: preview.base_amount_rub,
                    unit: previewUnit,
                  })}
                </p>
                {(() => {
                  const hasCut = preview.base_amount_rub > preview.amount_rub
                  const disc = hwidPreviewDiscountCaption(t, preview, hasCut)
                  return (
                    <>
                      {disc != null && <p className="text-muted-foreground">{disc}</p>}
                      {hasCut && (
                        <p className="text-muted-foreground">
                          {t('subscriptionPage.extraDevicesPaySubtotal', {
                            base: preview.base_amount_rub,
                            unit: previewUnit,
                          })}
                        </p>
                      )}
                    </>
                  )
                })()}
                <p className="pt-0.5 font-semibold">
                  {t('subscriptionPage.extraDevicesPayToPay', {
                    total: preview.amount_rub,
                    unit: previewUnit,
                  })}
                </p>
              </div>
            )}
            {payError && <p className="text-sm text-destructive">{payError}</p>}
            <Button
              type="button"
              className="w-full"
              disabled={!preview || buyTarget <= hwid.current_limit}
              loading={payLoading}
              onClick={async () => {
                setPayError(null)
                setPayLoading(true)
                try {
                  const idem = newIdempotencyKey()
                  const res = await api.hwidExtraCheckout(buyTarget, provider, idem)
                  window.open(res.payment_url, '_blank', 'noopener,noreferrer')
                  navigate(`/payment/status/${res.checkout_id}`)
                } catch (err) {
                  if (err instanceof ApiError) {
                    setPayError(err.status === 429 ? t('errors.tooManyRequests') : t('checkout.notAvailable'))
                  } else {
                    setPayError(t('errors.unknown'))
                  }
                } finally {
                  setPayLoading(false)
                }
              }}
            >
              {t('checkout.pay')}
            </Button>
            <LegalContinueDisclaimer siteLinks={bootstrap?.site_links} />
                </div>
              </div>
            </div>

            <div className={hwidSubpanelGridCls(panel === 'decrease')}>
              <div className="min-h-0 overflow-hidden">
                <div className="space-y-4 pt-0">
                  <div>
                    <label className="text-xs font-medium text-muted-foreground">
                      {t('subscriptionPage.extraDevicesNewLimit')}
                    </label>
                    <div className="relative mt-1.5">
                      <select
                        className="w-full cursor-pointer appearance-none rounded-lg border border-input bg-background px-3 py-2 pr-10 text-sm"
                        value={decTarget}
                        onChange={(e) => setDecTarget(parseInt(e.target.value, 10))}
                      >
                        {decOptions.map((n) => (
                          <option key={n} value={n}>
                            {n}
                          </option>
                        ))}
                      </select>
                      <ChevronDown
                        size={16}
                        className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground"
                      />
                    </div>
                  </div>
                  <p className="text-xs text-muted-foreground">{t('subscriptionPage.extraDevicesDecreaseNote')}</p>
                  {applyDec.isError && <p className="text-sm text-destructive">{t('errors.unknown')}</p>}
                  <Button
                    type="button"
                    className="w-full"
                    variant="secondary"
                    disabled={applyDec.isPending || decTarget >= hwid.current_limit || decTarget < hwid.base_limit}
                    onClick={() => openDecreaseConfirmModal()}
                  >
                    {t('subscriptionPage.extraDevicesConfirmDecrease')}
                  </Button>
                </div>
              </div>
            </div>
          </div>
        )}
      </CardContent>

      {decreaseModalMounted && typeof document !== 'undefined'
        ? createPortal(
            <div
              role="presentation"
              className={cn(
                'fixed inset-0 z-[2000] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm transition-opacity duration-[400ms] ease-out',
                decreaseModalVisible ? 'opacity-100' : 'pointer-events-none opacity-0',
              )}
              onClick={() => !applyDec.isPending && closeDecreaseConfirmModal()}
              onTransitionEnd={onDecreaseModalOverlayTransitionEnd}
            >
              <div
                role="dialog"
                aria-modal="true"
                aria-labelledby="hwid-decrease-warning-title"
                className={cn(
                  'w-full max-w-md rounded-2xl border border-border bg-background/95 p-5 shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] backdrop-blur-sm transition-[opacity,transform] duration-[400ms] ease-out',
                  decreaseModalVisible ? 'scale-100 opacity-100' : 'scale-[0.97] opacity-0',
                )}
                onClick={(e) => e.stopPropagation()}
              >
                <div className="flex gap-3">
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-amber-500/15 text-amber-600 dark:text-amber-400">
                    <AlertTriangle className="h-5 w-5" aria-hidden />
                  </div>
                  <div className="min-w-0 flex-1 space-y-3 text-sm leading-relaxed text-foreground">
                    <h2 id="hwid-decrease-warning-title" className="text-base font-semibold">
                      {t('subscriptionPage.extraDevicesDecreaseModalTitle')}
                    </h2>
                    <p>{t('subscriptionPage.extraDevicesDecreaseModalIntro')}</p>
                    <p className="text-muted-foreground">{t('subscriptionPage.extraDevicesDecreaseModalNoRefund')}</p>
                    <p className="text-muted-foreground">{t('subscriptionPage.extraDevicesDecreaseModalIrreversible')}</p>
                    <p>{t('subscriptionPage.extraDevicesDecreaseModalEnsure')}</p>
                  </div>
                </div>
                <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="w-full sm:w-auto"
                    disabled={applyDec.isPending}
                    onClick={() => closeDecreaseConfirmModal()}
                  >
                    {t('common.cancel')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    className="w-full sm:w-auto"
                    loading={applyDec.isPending}
                    disabled={applyDec.isPending || decTarget >= hwid.current_limit || decTarget < hwid.base_limit}
                    onClick={() => applyDec.mutate(decTarget)}
                  >
                    {t('subscriptionPage.extraDevicesDecreaseModalConfirm')}
                  </Button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}
    </Card>
  )
}
