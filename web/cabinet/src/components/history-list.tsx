import { useState } from 'react'
import { Bitcoin, ChevronLeft, ChevronRight, CreditCard, Gift, Star, Wallet } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { HeleketBrandIcon, TelegramBrandIcon } from '@/components/BrandIcons'
import { Button } from '@/components/ui/button'
import { formatNumber } from '@/lib/format'
import { cn, formatRub, splitDateTimeShort } from '@/lib/utils'

/** Обе истории кабинета — оплат и лояльности — листаются одинаково. */
export const HISTORY_PAGE_SIZE = 20

/**
 * Сумма покупки. Рубли округляем (в базе они дробные из-за скидок), у остальных
 * валют дробную часть сохраняем — у крипты копейки и есть вся точность.
 *
 * Разделитель берётся из локали интерфейса: голая интерполяция числа давала
 * «12.6 USDT» и в русской версии, где дробная часть пишется через запятую.
 */
export function formatMoney(amount: number, currency: string): string {
  const c = (currency || '').toUpperCase()
  if (c === 'STARS' || c === 'XTR') {
    return `${formatNumber(amount)} ⭐`
  }
  if (c === 'RUB' || c === 'RUR' || c === '') {
    return formatRub(Math.round(amount))
  }
  return `${formatNumber(amount)} ${currency}`
}

/** Дата в таблице истории: `28.08.26`, время второй строкой мелким и приглушённым. */
export function HistoryDateCell({ iso }: { iso?: string }) {
  const parts = splitDateTimeShort(iso)
  if (!parts) return <span className="text-muted-foreground">—</span>
  return (
    <span className="block leading-tight">
      <span className="block whitespace-nowrap tabular-nums">{parts.date}</span>
      <span className="mt-0.5 block whitespace-nowrap text-[11px] tabular-nums text-muted-foreground/70">
        {parts.time}
      </span>
    </span>
  )
}

/** Та же дата в одну строку — для мобильных карточек, где под ней уже нет места. */
export function historyDateInline(iso?: string): string {
  const parts = splitDateTimeShort(iso)
  return parts ? `${parts.date} · ${parts.time}` : '—'
}

export function HistoryPagination({
  page,
  hasPrev,
  hasNext,
  busy,
  onPrev,
  onNext,
}: {
  /** Номер страницы с нуля. */
  page: number
  hasPrev: boolean
  hasNext: boolean
  busy?: boolean
  onPrev: () => void
  onNext: () => void
}) {
  const { t } = useTranslation()
  // Спиннер — только у нажатой стрелки: busy приходит общий на обе.
  const [pressed, setPressed] = useState<'prev' | 'next' | null>(null)
  return (
    <div className="mt-4 flex items-center justify-between gap-3 border-t border-border pt-4">
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="gap-1"
        disabled={!hasPrev || busy}
        loading={busy && pressed === 'prev'}
        onClick={() => {
          setPressed('prev')
          onPrev()
        }}
      >
        <ChevronLeft size={16} aria-hidden />
        {t('common.pagePrev')}
      </Button>
      <span className="text-xs text-muted-foreground">{t('common.pageN', { n: page + 1 })}</span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="gap-1"
        disabled={!hasNext || busy}
        loading={busy && pressed === 'next'}
        onClick={() => {
          setPressed('next')
          onNext()
        }}
      >
        {t('common.pageNext')}
        <ChevronRight size={16} aria-hidden />
      </Button>
    </div>
  )
}

/**
 * Значок способа оплаты — тот же словарь, что в выборе метода на чекауте
 * (CheckoutPage / PlategaPaymentExpand): карта, СБП, крипта, Telegram.
 */
export function PaymentMethodIcon({ invoiceType, className }: { invoiceType: string; className?: string }) {
  const size = cn('size-4 shrink-0', className)
  switch (invoiceType) {
    case 'yookasa':
    case 'plt_cards':
      return <CreditCard className={cn(size, 'text-primary')} aria-hidden />
    case 'plt_acq':
      return <CreditCard className={cn(size, 'text-violet-500')} aria-hidden />
    case 'plt_ww':
      return <CreditCard className={cn(size, 'text-indigo-500')} aria-hidden />
    case 'plt_sbp':
      return <Wallet className={cn(size, 'text-primary')} aria-hidden />
    case 'crypto':
      return <Bitcoin className={cn(size, 'text-orange-500')} aria-hidden />
    case 'plt_crypto':
      return <Bitcoin className={cn(size, 'text-primary')} aria-hidden />
    case 'heleket':
      return <HeleketBrandIcon className={size} />
    case 'telegram':
      return <TelegramBrandIcon className={size} />
    default:
      return <Wallet className={cn(size, 'text-muted-foreground')} aria-hidden />
  }
}

/** Значок начисления за колесо фортуны — как у пункта «Колесо фортуны» в навигации. */
export function FortuneWheelIcon({ className }: { className?: string }) {
  return <Gift className={cn('size-4 shrink-0 text-amber-500', className)} aria-hidden />
}

/** Звезда Telegram Stars — для сумм в XTR, где значок валюты сам по себе. */
export function StarsIcon({ className }: { className?: string }) {
  return <Star className={cn('size-4 shrink-0 text-amber-500', className)} aria-hidden />
}

export function invoiceLabel(t: (k: string) => string, invoiceType: string): string {
  switch (invoiceType) {
    case 'yookasa':
      return t('payments.methodCard')
    case 'crypto':
      return t('payments.methodCrypto')
    case 'telegram':
      return t('payments.methodTelegram')
    case 'tribute':
      return t('payments.methodTribute')
    case 'plt_sbp':
      return t('payments.methodSbp')
    case 'plt_cards':
    case 'plt_acq':
    case 'plt_ww':
      return t('payments.methodCard')
    case 'plt_crypto':
      return t('payments.methodPlategaCrypto')
    case 'heleket':
      return t('payments.methodHeleket')
    default:
      return invoiceType
  }
}

function effectivePurchaseKind(p: { purchase_kind: string; month: number; extra_hwid?: number }): string {
  const raw = p.purchase_kind
  if (raw === 'tariff_upgrade' || raw === 'extra_hwid') {
    return raw
  }
  const extra = p.extra_hwid ?? 0
  if (p.month > 0 && extra > 0 && raw === 'subscription') {
    return 'subscription_with_hwid'
  }
  if (p.month <= 0 && extra > 0 && raw === 'subscription') {
    return 'extra_hwid'
  }
  return raw
}

export function purchaseKindLabel(
  t: (k: string, o?: Record<string, string | number>) => string,
  p: { purchase_kind: string; month: number; extra_hwid?: number },
): string {
  const kind = effectivePurchaseKind(p)
  const extra = p.extra_hwid ?? 0
  switch (kind) {
    case 'subscription':
      return t('payments.kindSubscription')
    case 'tariff_upgrade':
      return t('payments.kindUpgrade')
    case 'extra_hwid':
      return extra > 0 ? t('payments.kindExtraHwidSlots', { n: extra }) : t('payments.kindExtraHwid')
    case 'subscription_with_hwid':
      return t('payments.kindSubscriptionWithHwid', { months: p.month, n: extra })
    default:
      return kind || '—'
  }
}
