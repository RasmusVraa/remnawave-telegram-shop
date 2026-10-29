import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { AlertTriangle, ChevronRight, Minus, Plus, Smartphone, X } from 'lucide-react'

import { ConfirmModal } from '@/components/ConfirmModal'
import { Button } from '@/components/ui/button'
import { useToast } from '@/components/ui/toast'
import { api, type HwidExtraPreviewResponse, type SubscriptionHwidExtraInfo } from '@/lib/api'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useSheetDragDismiss } from '@/hooks/useSheetDragDismiss'
import { useTranslationWithLang } from '@/hooks/useTranslationWithLang'

/**
 * Места для устройств: докупка и уменьшение лимита.
 *
 * Раньше это был отдельный блок «Дополнительные опции» с двумя равными
 * кнопками «Докупить» / «Уменьшить» и выпадающим списком «Новый лимит» —
 * цену было не видно до открытия формы, а сам блок стоял отдельно от списка
 * устройств, лимит которого менял. Теперь вход — строка под «Моими
 * устройствами», а число меняется одним переключателем −/+: вверх — докупка
 * с ценой сразу, вниз — уменьшение с подтверждением.
 */

const PRICE_PROVIDER_ORDER = [
  'yookassa',
  'platega_sbp',
  'platega_cards',
  'platega_acquiring',
  'platega_worldwide',
  'platega_crypto',
  'cryptopay',
  'heleket',
] as const

/**
 * Способ оплаты, по которому считаем цену до перехода к оплате. Рублёвые
 * способы стоят одинаково, поэтому берём первый включённый; Stars — только
 * если ничего другого нет, у них своя цена, и её покажет страница оплаты.
 */
function usePriceProvider(): string {
  const { data: bootstrap } = useAuthBootstrap()
  const enabled = bootstrap?.payment_providers as Record<string, boolean | undefined> | undefined
  const rub = PRICE_PROVIDER_ORDER.find((p) =>
    // До bootstrap ЮKassa считается включённой — как на странице оплаты.
    enabled ? enabled[p] === true : p === 'yookassa',
  )
  return rub ?? (enabled?.telegram ? 'telegram' : 'yookassa')
}

export function useHwidSlotPreview(target: number, enabled: boolean) {
  const provider = usePriceProvider()
  return useQuery({
    queryKey: ['hwidExtraPreview', target, provider],
    queryFn: () => api.hwidExtraPreview(target, provider),
    enabled,
    staleTime: 20_000,
  })
}

export function previewUnit(t: TFunction, preview: HwidExtraPreviewResponse | undefined): string {
  return preview?.currency === 'STARS' ? t('checkout.stars') : t('checkout.rub')
}

/** «21 декабря» — без года: место всё равно живёт не дольше подписки. */
export function formatDayMonth(iso: string | null | undefined, lang: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (!Number.isFinite(d.getTime())) return ''
  return d.toLocaleDateString(lang === 'ru' ? 'ru-RU' : 'en-US', { day: 'numeric', month: 'long' })
}

/**
 * Строка под списком устройств. Текст зависит от того, что с лимитом можно
 * сделать: докупить, только уменьшить (упёрлись в максимум) или ничего —
 * когда число закрепил админ, вместо кнопки тихое пояснение.
 */
export function DeviceSlotsRow({
  hwid,
  onOpen,
}: {
  hwid: SubscriptionHwidExtraInfo
  onOpen: () => void
}) {
  const { t } = useTranslation()

  if (hwid.pinned) {
    return (
      <p className="mt-3 border-t border-border/70 px-2 pt-3 text-sm text-muted-foreground">
        {t('subscriptionPage.slotsPinned')}
      </p>
    )
  }
  if (!hwid.can_increase && !hwid.can_decrease) return null

  const atMax = hwid.max_limit > 0 && hwid.current_limit >= hwid.max_limit
  const title = hwid.can_increase
    ? t('subscriptionPage.slotsMoreTitle')
    : atMax
      ? t('subscriptionPage.slotsMaxTitle')
      : t('subscriptionPage.slotsChangeTitle')
  const action = hwid.can_increase ? t('subscriptionPage.slotsMoreAction') : t('subscriptionPage.slotsChangeAction')

  return (
    <div className="mt-3 border-t border-border/70 pt-2">
      <button
        type="button"
        onClick={onOpen}
        className="cabinet-row group flex w-full items-center gap-3 rounded-xl px-2 py-2 text-left"
      >
        <span className="min-w-0 flex-1 text-sm font-medium">{title}</span>
        <span className="inline-flex shrink-0 items-center gap-0.5 text-sm font-semibold text-primary">
          {action}
          <ChevronRight size={15} className="transition-transform group-hover:translate-x-0.5" />
        </span>
      </button>
    </div>
  )
}

/**
 * Плашка вместо пустого слота, когда все места заняты: ровно там, где человек
 * ищет, куда подключить ещё одно устройство. Цена — сразу, за одно место.
 */
export function DeviceSlotsFullCta({
  hwid,
  expireAt,
  onOpen,
}: {
  hwid: SubscriptionHwidExtraInfo
  expireAt: string | null | undefined
  onOpen: () => void
}) {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const { data: preview } = useHwidSlotPreview(hwid.current_limit + 1, hwid.can_increase)

  return (
    <button
      type="button"
      onClick={onOpen}
      className="mt-1 flex w-full items-center gap-3 rounded-xl border border-primary/45 bg-gradient-to-r from-primary/15 to-primary/[0.04] px-3 py-2.5 text-left transition-colors hover:border-primary/70"
    >
      <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
        <Plus size={16} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">{t('subscriptionPage.slotsFullCtaTitle')}</span>
        <span className="block text-xs text-muted-foreground">
          {preview
            ? t('subscriptionPage.slotsFullCtaPrice', {
                amount: formatNumber(preview.amount_rub),
                unit: previewUnit(t, preview),
                date: formatDayMonth(expireAt, lang),
              })
            : t('subscriptionPage.slotsFullCtaNoPrice')}
        </span>
      </span>
    </button>
  )
}

const MODAL_ANIM_MS = 250

function discountCaption(t: TFunction, p: HwidExtraPreviewResponse): string | null {
  if (!(p.base_amount_rub > p.amount_rub)) return null
  const parts: string[] = []
  if ((p.loyalty_discount_pct ?? 0) > 0) parts.push(t('subscriptionPage.slotsDiscountLoyalty', { pct: p.loyalty_discount_pct }))
  if ((p.promo_discount_pct ?? 0) > 0) parts.push(t('subscriptionPage.slotsDiscountPromo', { pct: p.promo_discount_pct }))
  if (parts.length > 0) {
    return t('subscriptionPage.slotsDiscount', { parts: parts.join(t('subscriptionPage.slotsDiscountJoin')) })
  }
  if ((p.total_discount_pct ?? 0) > 0) return t('subscriptionPage.slotsDiscountTotal', { pct: p.total_discount_pct })
  return null
}

/** Плитки мест: заняты, свободны, новые (докупка) и перечёркнутые (уменьшение). */
function SlotTiles({ connected, current, target }: { connected: number; current: number; target: number }) {
  const n = Math.max(current, target)
  return (
    <div className="flex flex-wrap justify-center gap-1.5" aria-hidden>
      {Array.from({ length: n }).map((_, i) => {
        const gone = i >= target && i < current
        const added = i >= current && i < target
        const used = !gone && !added && i < connected
        return (
          <span
            key={i}
            className={cn(
              'relative inline-flex size-8 items-center justify-center rounded-lg transition-colors duration-200',
              used && 'bg-primary/15 text-primary',
              added && 'border border-primary bg-primary/[0.06] text-primary ring-2 ring-primary/15',
              gone && 'border border-dashed border-destructive/60',
              !used && !added && !gone && 'border border-dashed border-border text-muted-foreground',
            )}
          >
            {used && <Smartphone size={14} />}
            {added && <Plus size={14} />}
            {gone && <span className="absolute h-[1.5px] w-7 -rotate-[35deg] rounded bg-destructive/80" />}
          </span>
        )
      })}
    </div>
  )
}

export function DeviceSlotsModal({
  open,
  hwid,
  connected,
  expireAt,
  initialTarget,
  onClose,
  onUpdated,
}: {
  open: boolean
  hwid: SubscriptionHwidExtraInfo
  connected: number
  expireAt: string | null | undefined
  /** С какого числа открыть окно: из «Докупить» — сразу +1. */
  initialTarget: number
  onClose: () => void
  onUpdated: () => void
}) {
  const { t } = useTranslation()
  const { lang } = useTranslationWithLang()
  const navigate = useNavigate()
  const toast = useToast()
  const [mounted, setMounted] = useState(open)
  const [visible, setVisible] = useState(false)
  const [target, setTarget] = useState(initialTarget)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const { sheetRef, handlers: dragHandlers } = useSheetDragDismiss({ open, onDismiss: onClose })

  const current = hwid.current_limit
  const min = hwid.can_decrease ? hwid.base_limit : current
  const max = hwid.can_increase ? hwid.max_limit : current
  const delta = target - current

  useEffect(() => {
    if (open) {
      setTarget(Math.min(max, Math.max(min, initialTarget)))
      setMounted(true)
      const id = requestAnimationFrame(() => setVisible(true))
      return () => cancelAnimationFrame(id)
    }
    setVisible(false)
    setConfirmOpen(false)
    // По таймеру, а не по transitionend: при «уменьшить движение» событие не
    // приходит, и окно осталось бы в DOM.
    const id = window.setTimeout(() => setMounted(false), MODAL_ANIM_MS + 30)
    return () => window.clearTimeout(id)
    // initialTarget/min/max читаем на момент открытия, а не следим за ними.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  useEffect(() => {
    if (!open || confirmOpen) return undefined
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [open, confirmOpen, onClose])

  const { data: preview, isFetching: previewLoading } = useHwidSlotPreview(target, open && delta > 0)

  const applyDecrease = useMutation({
    mutationFn: (limit: number) => api.hwidExtraApply(limit),
    onSuccess: () => {
      setConfirmOpen(false)
      toast.success(t('subscriptionPage.slotsDecreaseDone'))
      onUpdated()
      onClose()
    },
    onError: () => {
      setConfirmOpen(false)
      toast.error(t('errors.unknown'))
    },
  })

  if (typeof document === 'undefined' || !mounted) return null

  const date = formatDayMonth(expireAt, lang)
  const unit = previewUnit(t, preview)
  const shownPreview = preview && preview.target_limit === target ? preview : undefined
  const discount = shownPreview ? discountCaption(t, shownPreview) : null
  const removed = -delta

  return createPortal(
    <>
      <div
        className={cn(
          'fixed inset-0 z-[2000] flex items-end justify-center bg-black/60 backdrop-blur-sm transition-opacity duration-[250ms] ease-out sm:items-center sm:p-4',
          visible ? 'opacity-100' : 'opacity-0',
        )}
        onClick={onClose}
      >
        <div
          ref={sheetRef}
          role="dialog"
          aria-modal="true"
          aria-label={t('subscriptionPage.slotsModalTitle')}
          onClick={(e) => e.stopPropagation()}
          className={cn(
            'max-h-[88dvh] w-full max-w-md overflow-y-auto rounded-t-2xl border border-border bg-background/95 p-4 pb-[max(1rem,calc(0.75rem+var(--cabinet-sheet-safe-bottom)))] shadow-[0_24px_60px_-24px_rgb(0_0_0_/_0.55)] backdrop-blur-sm transition-[opacity,transform] duration-[250ms] ease-out sm:max-h-[90vh] sm:rounded-2xl sm:p-5 sm:pb-5',
            visible
              ? 'translate-y-0 opacity-100 sm:scale-100'
              : 'translate-y-full opacity-0 sm:translate-y-0 sm:scale-[0.97]',
          )}
        >
          <div
            className="-mx-4 -mt-4 mb-1 flex cursor-grab touch-none justify-center pb-2 pt-4 sm:hidden"
            {...dragHandlers}
          >
            <span className="h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden />
          </div>

          <div className="flex touch-none items-start justify-between gap-3 sm:touch-auto" {...dragHandlers}>
            <div className="min-w-0">
              <p className="font-heading text-lg font-semibold">{t('subscriptionPage.slotsModalTitle')}</p>
              <p className="mt-0.5 text-sm text-muted-foreground">{t('subscriptionPage.slotsModalSubtitle')}</p>
            </div>
            <button
              type="button"
              onClick={onClose}
              aria-label={t('common.close')}
              className="shrink-0 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <X size={16} />
            </button>
          </div>

          <div className="mt-4">
            <SlotTiles connected={connected} current={current} target={target} />
          </div>

          <div className="mt-4 flex items-center justify-center gap-6">
            <button
              type="button"
              aria-label={t('subscriptionPage.slotsStepDown')}
              disabled={target <= min}
              onClick={() => setTarget((v) => Math.max(min, v - 1))}
              className="inline-flex size-12 items-center justify-center rounded-full border border-border bg-muted/40 text-foreground transition-colors hover:bg-muted disabled:pointer-events-none disabled:opacity-35"
            >
              <Minus size={18} />
            </button>
            <div className="min-w-[7rem] text-center">
              <p className="font-heading text-4xl font-bold leading-none tabular-nums">{target}</p>
              <p className="mt-1.5 text-xs text-muted-foreground">
                {delta === 0
                  ? t('subscriptionPage.slotsStepperNow', { n: current })
                  : t('subscriptionPage.slotsStepperChange', { from: current, to: target })}
              </p>
            </div>
            <button
              type="button"
              aria-label={t('subscriptionPage.slotsStepUp')}
              disabled={target >= max}
              onClick={() => setTarget((v) => Math.min(max, v + 1))}
              className="inline-flex size-12 items-center justify-center rounded-full border border-border bg-muted/40 text-foreground transition-colors hover:bg-muted disabled:pointer-events-none disabled:opacity-35"
            >
              <Plus size={18} />
            </button>
          </div>
          <p className="mb-4 mt-2.5 text-center text-xs text-muted-foreground">
            {t('subscriptionPage.slotsRange', { min: hwid.base_limit, max: Math.max(max, current) })}
          </p>

          {delta > 0 ? (
            <>
              <div className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/25 px-3.5 py-3">
                <div className="min-w-0">
                  <p className="text-sm font-semibold">
                    {t('subscriptionPage.slotsBuySummary', {
                      slots: t('subscriptionPage.slotsCount', { count: delta }),
                      date,
                    })}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {[
                      t('subscriptionPage.slotsDays', { count: shownPreview?.days_left ?? hwid.days_left }),
                      discount,
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </p>
                </div>
                <div className="shrink-0 text-right">
                  {shownPreview ? (
                    <>
                      {discount && (
                        <s className="block text-xs text-muted-foreground">
                          {formatNumber(shownPreview.base_amount_rub)} {unit}
                        </s>
                      )}
                      <p className="font-heading text-2xl font-bold leading-tight tabular-nums">
                        {formatNumber(shownPreview.amount_rub)} {unit}
                      </p>
                    </>
                  ) : (
                    <span className="inline-block h-7 w-16 animate-pulse rounded-md bg-muted" />
                  )}
                </div>
              </div>
              <p className="mx-0.5 mb-4 mt-2.5 text-xs text-muted-foreground">{t('subscriptionPage.slotsRenewNote')}</p>
              <Button
                type="button"
                className="w-full"
                disabled={!shownPreview && previewLoading}
                onClick={() => {
                  onClose()
                  navigate(`/checkout/devices?target=${target}`)
                }}
              >
                {shownPreview
                  ? t('subscriptionPage.slotsGoToPay', { amount: formatNumber(shownPreview.amount_rub), unit })
                  : t('subscriptionPage.slotsGoToPayNoPrice')}
              </Button>
            </>
          ) : delta < 0 ? (
            <>
              <div className="mb-4 flex gap-2.5 rounded-xl border border-amber-500/40 bg-amber-500/10 px-3.5 py-3 text-sm text-amber-800 dark:text-amber-200">
                <AlertTriangle size={17} className="mt-px shrink-0 text-amber-600 dark:text-amber-400" />
                <span>{t('subscriptionPage.slotsDecreaseWarn', { count: removed })}</span>
              </div>
              <Button type="button" variant="destructive" className="w-full" onClick={() => setConfirmOpen(true)}>
                {t('subscriptionPage.slotsDecreaseButton', { count: removed })}
              </Button>
            </>
          ) : (
            <Button type="button" variant="secondary" className="w-full" disabled>
              {t('subscriptionPage.slotsPickHint')}
            </Button>
          )}
        </div>
      </div>

      <ConfirmModal
        open={confirmOpen}
        tone="danger"
        inlineTitle
        title={t('subscriptionPage.slotsDecreaseConfirmTitle', { count: removed > 0 ? removed : 1 })}
        description={t('subscriptionPage.slotsDecreaseConfirmText', {
          count: removed > 0 ? removed : 1,
          left: t('subscriptionPage.slotsCount', { count: target }),
        })}
        confirmLabel={t('subscriptionPage.slotsDecreaseConfirmButton')}
        loading={applyDecrease.isPending}
        onConfirm={() => applyDecrease.mutate(target)}
        onClose={() => setConfirmOpen(false)}
      />
    </>,
    document.body,
  )
}
