import { useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { AlertCircle, ArrowLeftRight, Plus, RefreshCw, Sparkles } from 'lucide-react'

import { PageReveal, RevealItem } from '@/components/PageReveal'
import { PageTitleWithBack } from '@/components/PageTitleWithBack'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { PayButtonInline, PayButtonMobileBar, type PayButtonProps } from '@/features/checkout/checkoutShared'

/**
 * Сводка заказа на страницах оформления (подписка, места для устройств, трафик):
 * шапка с бейджем, список «что входит», чек с итогом и раскладка страницы.
 * Вид — по макету .cursor/work-in-progress/checkout-summary/mockup.html.
 *
 * Чек только раскладывает суммы сервера по строкам: итог всегда `amount_rub`
 * из предпросмотра, на клиенте не пересчитывается.
 */

// ── Сценарий ────────────────────────────────────────────────────────────

export type DisplayScenario = 'new' | 'renew' | 'upgrade' | 'downgrade' | 'addon'

/**
 * Сценарий для показа. Сервер ставит `new` только тому, у кого нет активной
 * подписки, а активный триал без тарифа получает `renew`. Для человека это
 * первая покупка, поэтому триал показываем как новую подписку. В запросы
 * подмена не уходит — только бейдж и надпись на кнопке.
 */
export function displayScenario(scenario: string | undefined, isTrial: boolean | undefined): DisplayScenario | null {
  switch (scenario) {
    case 'new':
    case 'classic_new':
      return 'new'
    case 'renew':
    case 'classic_renew':
      return isTrial ? 'new' : 'renew'
    case 'upgrade':
      return 'upgrade'
    case 'downgrade':
      return 'downgrade'
    default:
      return null
  }
}

const BADGE: Record<DisplayScenario, { key: string; icon: typeof Sparkles; tone: 'primary' | 'violet' }> = {
  new: { key: 'checkout.scenarioNew', icon: Sparkles, tone: 'primary' },
  renew: { key: 'checkout.scenarioRenew', icon: RefreshCw, tone: 'primary' },
  upgrade: { key: 'checkout.scenarioUpgrade', icon: ArrowLeftRight, tone: 'violet' },
  downgrade: { key: 'checkout.scenarioDowngrade', icon: ArrowLeftRight, tone: 'violet' },
  addon: { key: 'checkout.addonBadge', icon: Plus, tone: 'primary' },
}

/** Маленькая «таблетка» над заголовком заказа. Смена тарифа — фиолетовым, как акцент лояльности в кабинете. */
export function ScenarioBadge({ scenario }: { scenario: DisplayScenario }) {
  const { t } = useTranslation()
  const b = BADGE[scenario]
  const Icon = b.icon
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-[3px] text-xs font-semibold',
        b.tone === 'violet'
          ? 'bg-violet-500/[0.12] text-violet-700 dark:bg-violet-400/[0.14] dark:text-violet-300'
          : 'bg-primary/[0.12] text-primary',
      )}
    >
      <Icon size={12} strokeWidth={2.4} aria-hidden />
      {t(b.key)}
    </span>
  )
}

/**
 * Шапка карточки заказа: «Премиум · 1 месяц» и бейдж. Под заголовком ничего нет — решение владельца.
 *
 * Бейдж справа в одной строке с заголовком — экономим строку. Не влез (длинное
 * название тарифа) — flex-wrap-reverse переносит его над заголовком.
 */
export function OrderHeader({ scenario, title }: { scenario: DisplayScenario | null; title: string }) {
  return (
    <div className="flex flex-wrap-reverse items-center justify-between gap-x-3 gap-y-2">
      <h2 className="text-[19px] font-bold leading-tight">{title}</h2>
      {scenario && (
        <div className="shrink-0">
          <ScenarioBadge scenario={scenario} />
        </div>
      )}
    </div>
  )
}

// ── «Что входит» ────────────────────────────────────────────────────────

// На телефоне (< sm) список плотнее: иначе до чека с ценой приходится скроллить.
export function IncludedList({ children }: { children: ReactNode }) {
  return <ul className="mt-3 grid gap-2 sm:mt-3.5 sm:gap-2.5">{children}</ul>
}

/** Строка «иконка в плитке + заголовок + серая подпись». Без подписи текст по центру плитки. */
export function IncludedRow({
  icon,
  title,
  hint,
  hintFromSm = false,
}: {
  icon: ReactNode
  title: ReactNode
  hint?: ReactNode
  /** Подпись только с sm: на телефоне строка в одну линию. */
  hintFromSm?: boolean
}) {
  return (
    <li className={cn('flex gap-3', !hint ? 'items-center' : hintFromSm ? 'items-center sm:items-start' : 'items-start')}>
      <span className="flex h-[30px] w-[30px] shrink-0 items-center justify-center rounded-[10px] bg-muted text-primary sm:h-[34px] sm:w-[34px] sm:rounded-[11px] dark:bg-white/[0.07]">
        {icon}
      </span>
      <div className="min-w-0">
        <p className="font-semibold leading-snug">{title}</p>
        {hint && (
          <p className={cn('mt-px text-[13px] leading-snug text-muted-foreground', hintFromSm && 'hidden sm:block')}>
            {hint}
          </p>
        )}
      </div>
    </li>
  )
}

// ── Чек ─────────────────────────────────────────────────────────────────

export type ReceiptLine = { key: string; label: string; amount: number; discount?: boolean }

export type DiscountInput = {
  /** Сумма до скидок (`base_amount_rub`). */
  base: number
  /** Итог сервера (`amount_rub`). */
  amount: number
  loyaltyPct?: number
  promoPct?: number
  totalPct?: number
}

/**
 * Строки скидок, которые в сумме дают ровно `base − amount`.
 *
 * Сервер применяет к базе один общий процент (loyalty.ApplyCombinedPercentDiscount).
 * Если считать лояльность и промокод отдельно, при округлении строки расходились
 * бы с итогом на рубль — поэтому промокод берётся остатком. Если сработал потолок
 * LOYALTY_MAX_TOTAL_DISCOUNT_PERCENT, две строки не сложились бы в итог вообще —
 * тогда показываем одну общую.
 */
export function discountLines(t: TFunction, d: DiscountInput): ReceiptLine[] {
  const total = Math.max(0, d.base - d.amount)
  if (total <= 0) return []
  const loy = d.loyaltyPct ?? 0
  const promo = d.promoPct ?? 0
  const pct = d.totalPct ?? loy + promo
  if (loy > 0 && promo > 0) {
    if (loy + promo > pct) {
      return [{ key: 'combined', label: t('checkout.combinedDiscountLine', { pct }), amount: total, discount: true }]
    }
    const loyRub = Math.min(total, Math.round((d.base * loy) / 100))
    // На маленькой базе (звёзды, малый процент) одна из строк может округлиться в ноль — её не показываем.
    return [
      { key: 'loyalty', label: t('checkout.loyaltyDiscountLine', { pct: loy }), amount: loyRub, discount: true },
      { key: 'promo', label: t('checkout.promoDiscountLine', { pct: promo }), amount: total - loyRub, discount: true },
    ].filter((l) => l.amount > 0)
  }
  // Одна скидка тоже могла упереться в потолок — тогда в подписи процент потолка, а не исходный.
  if (loy > 0) return [{ key: 'loyalty', label: t('checkout.loyaltyDiscountLine', { pct: Math.min(loy, pct) }), amount: total, discount: true }]
  if (promo > 0) return [{ key: 'promo', label: t('checkout.promoDiscountLine', { pct: Math.min(promo, pct) }), amount: total, discount: true }]
  return [{ key: 'combined', label: t('checkout.combinedDiscountLine', { pct }), amount: total, discount: true }]
}

/**
 * Где и какую часть чека рисовать: `inline` — целиком в карточке заказа (телефон,
 * планшет); `lines` и `total` — строки и итог по отдельности в карточке «Расчёт» на ПК,
 * чтобы итог с кнопкой можно было прижать к низу карточки.
 */
export type ReceiptMode = 'inline' | 'lines' | 'total'

/**
 * Чек: строки, линия, «Итого». `total == null` — суммы ещё нет (скелетон).
 * `busy` — идёт пересчёт: суммы прошлого ответа бледные, у итога спиннер.
 */
export function OrderReceipt({
  lines,
  total,
  strike,
  unit,
  busy = false,
  note,
  mode = 'inline',
}: {
  lines: ReceiptLine[]
  total: number | null
  /** Сумма до скидок — зачёркнутая перед итогом. */
  strike?: number | null
  unit: string
  busy?: boolean
  note?: ReactNode
  mode?: ReceiptMode
}) {
  const { t } = useTranslation()
  const money = (n: number) => `${formatNumber(n)} ${unit}`
  return (
    <div className={cn(mode === 'inline' && 'mt-3.5 border-t border-dashed border-border pt-3')} aria-busy={busy || undefined}>
      {mode !== 'total' && (
      <div className="grid gap-[7px] text-sm">
        {total == null && !note && lines.length === 0 && (
          <>
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
          </>
        )}
        {lines.map((l) => (
          <div
            key={l.key}
            className={cn(
              'flex justify-between gap-2.5',
              l.discount ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground',
            )}
          >
            <span className="min-w-0">{l.label}</span>
            <span
              className={cn(
                'shrink-0 whitespace-nowrap tabular-nums transition-opacity',
                !l.discount && 'text-foreground',
                busy && 'opacity-40',
              )}
            >
              {l.discount ? `−${money(l.amount)}` : money(l.amount)}
            </span>
          </div>
        ))}
      </div>
      )}
      {mode !== 'lines' && (
      <>
      <div className="mt-3 flex items-baseline justify-between gap-3 border-t border-border pt-3">
        <span className="font-semibold">{t('checkout.total')}</span>
        {total == null && note ? (
          // Суммы не будет (ошибка) — прочерк, а не вечный скелетон.
          <span className="text-[26px] font-extrabold leading-none text-muted-foreground">—</span>
        ) : total == null ? (
          <Skeleton className="h-7 w-24" />
        ) : (
          <span className="flex items-center whitespace-nowrap">
            {busy && (
              <span className="mr-2 text-primary" role="status" aria-label={t('checkout.recalculating')}>
                <Spinner />
              </span>
            )}
            {strike != null && strike > total && (
              <span className={cn('mr-2 text-sm font-medium text-muted-foreground line-through tabular-nums', busy && 'opacity-40')}>
                {money(strike)}
              </span>
            )}
            <span className={cn('text-[26px] font-extrabold leading-none tabular-nums transition-opacity', busy && 'opacity-40')}>
              {money(total)}
            </span>
          </span>
        )}
      </div>
      {note && <p className="mt-2 text-xs text-muted-foreground">{note}</p>}
      </>
      )}
    </div>
  )
}

/** Скелетон карточки заказа на время загрузки: бейдж, заголовок, строки. Чек рисует OrderReceipt с total=null. */
export function OrderSkeleton() {
  return (
    <div aria-hidden>
      <Skeleton className="h-5 w-28 rounded-full" />
      <Skeleton className="mt-2.5 h-6 w-48" />
      <div className="mt-4 grid gap-2.5">
        {[0, 1, 2].map((i) => (
          <div key={i} className="flex items-center gap-3">
            <Skeleton className="h-[34px] w-[34px] rounded-[11px]" />
            <Skeleton className="h-4 w-40" />
          </div>
        ))}
      </div>
    </div>
  )
}

// ── Раскладка страницы ──────────────────────────────────────────────────

/**
 * Одинаковая высота карточки заказа и «Расчёта» на ПК.
 *
 * Обе карточки нельзя поставить в одну строку сетки: у элемента сетки
 * содержащий блок — его ячейка, и липкая правая карточка перестала бы липнуть.
 * Поэтому естественная высота содержимого обеих меряется ResizeObserver'ом, а
 * общая минимальная высота отдаётся через CSS-переменную — действует только
 * с lg (класс lg:min-h-…), на телефоне ничего не меняет. Меряются внутренние
 * блоки, которые не растягиваются, поэтому min-height не влияет на замер.
 */
function useEqualCardHeight(enabled: boolean) {
  const orderInner = useRef<HTMLDivElement>(null)
  const calcTop = useRef<HTMLDivElement>(null)
  const calcBottom = useRef<HTMLDivElement>(null)
  const [height, setHeight] = useState<number | null>(null)

  useLayoutEffect(() => {
    const els = [orderInner.current, calcTop.current, calcBottom.current]
    if (!enabled || typeof ResizeObserver === 'undefined' || els.some((e) => !e)) {
      setHeight(null)
      return
    }
    const [order, top, bottom] = els as HTMLDivElement[]
    const measure = () => {
      // Скрытая на этой ширине карточка «Расчёт» даёт 0 — выравнивать не с чем.
      if (top.offsetHeight === 0 || order.offsetHeight === 0) {
        setHeight(null)
        return
      }
      // 12 — зазор между строками чека и итогом: margin-top строки «Итого» (mt-3) выходит за блок.
      const next = Math.ceil(Math.max(order.offsetHeight, top.offsetHeight + bottom.offsetHeight + 12))
      setHeight((prev) => (prev === next ? prev : next))
    }
    measure()
    const ro = new ResizeObserver(measure)
    els.forEach((e) => ro.observe(e as HTMLDivElement))
    return () => ro.disconnect()
  }, [enabled])

  // 2rem — p-4 сверху и снизу, 2px — рамка карточки.
  const style =
    height != null ? ({ '--checkout-card-h': `calc(${height}px + 2rem + 2px)` } as CSSProperties) : undefined
  return { orderInner, calcTop, calcBottom, style, active: height != null }
}

/**
 * Раскладка оформления.
 *
 * - < sm (телефон, мини-апп): одна колонка, чек внутри карточки заказа, кнопка —
 *   закреплённая плашка снизу.
 * - sm…lg: одна колонка, кнопка в потоке под способом оплаты.
 * - lg+: две колонки. Справа карточка «Расчёт», липкая: при длинной левой колонке
 *   (раскрытый список способов) кнопка остаётся на экране. С `equalHeights`
 *   карточка заказа и «Расчёт» одной высоты, итог с кнопкой прижат к низу.
 *
 * Чек передаётся функцией и рисуется в двух местах (в карточке заказа и в правой
 * карточке) — расчёт строк один, меняется только рамка.
 */
export function CheckoutLayout({
  title,
  order,
  receipt,
  payment,
  notices,
  error,
  pay,
  equalHeights = false,
}: {
  title: string
  /** Содержимое карточки заказа без чека. */
  order: ReactNode
  /** Чек; null — не показывать (страница в ошибке). */
  receipt: ((mode: ReceiptMode) => ReactNode) | null
  payment?: ReactNode
  /** Предупреждения над кнопкой (например, «нет цены в звёздах»). */
  notices?: ReactNode
  error?: string | null
  pay: PayButtonProps | null
  /**
   * Выровнять карточку заказа и «Расчёт» по высоте (ПК). Не включать, когда
   * карточка заказа заметно выше (блок смены тарифа): «Расчёт» вытянулся бы пустым.
   */
  equalHeights?: boolean
}) {
  const { t } = useTranslation()
  const eq = useEqualCardHeight(equalHeights && receipt != null)
  const errorAlert = error ? (
    <Alert variant="destructive">
      <AlertCircle size={14} />
      <AlertDescription>{error}</AlertDescription>
    </Alert>
  ) : null

  return (
    <PageReveal className="mx-auto w-full max-w-lg space-y-4 lg:max-w-5xl">
      <RevealItem>
        <PageTitleWithBack title={title} />
      </RevealItem>

      <div
        className="space-y-3 lg:grid lg:grid-cols-[1.15fr_.85fr] lg:items-start lg:gap-3.5 lg:space-y-0"
        style={eq.style}
      >
        <div className="space-y-3">
          <RevealItem>
            <Card className={cn('p-4', eq.active && 'lg:min-h-[var(--checkout-card-h)]')}>
              <div ref={eq.orderInner}>
                {order}
                {receipt && <div className="lg:hidden">{receipt('inline')}</div>}
              </div>
            </Card>
          </RevealItem>
          {payment && <RevealItem>{payment}</RevealItem>}
          {(notices || errorAlert) && (
            <RevealItem className={cn('space-y-3', receipt && 'lg:hidden')}>
              {notices}
              {errorAlert}
            </RevealItem>
          )}
          {pay && (
            <RevealItem className={cn('hidden sm:block', receipt && 'lg:hidden')}>
              <PayButtonInline {...pay} />
            </RevealItem>
          )}
        </div>

        {receipt && (
          <RevealItem className="hidden lg:sticky lg:top-24 lg:block">
            <Card className={cn('flex flex-col p-4', eq.active && 'lg:min-h-[var(--checkout-card-h)]')}>
              <div ref={eq.calcTop}>
                <h2 className="mb-3 text-[15px] font-medium text-muted-foreground">{t('checkout.calcTitle')}</h2>
                {receipt('lines')}
              </div>
              {/* mt-auto: при выравнивании по высоте итог с кнопкой прижимается к низу карточки. */}
              <div ref={eq.calcBottom} className="mt-auto">
                {receipt('total')}
                {(notices || errorAlert) && (
                  <div className="mt-3.5 space-y-3">
                    {notices}
                    {errorAlert}
                  </div>
                )}
                {pay && (
                  <div className="mt-3.5">
                    <PayButtonInline {...pay} />
                  </div>
                )}
              </div>
            </Card>
          </RevealItem>
        )}
      </div>

      {pay && <PayButtonMobileBar {...pay} />}
    </PageReveal>
  )
}
