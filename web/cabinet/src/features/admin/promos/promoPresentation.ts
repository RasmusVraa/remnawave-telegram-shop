import type { TFunction } from 'i18next'
import { CalendarPlus, Percent, Smartphone, Timer, type LucideIcon } from 'lucide-react'

import type { AdminPromoCodeDTO, AdminPromoPaybackDTO, AdminPromoTypeFilter } from '@/lib/types/admin'
import { hasPromoUseLimit, isPromoExpired, resolvePromoDisplayStatus } from '../utils/promoStatus'

/**
 * Всё, что страница промокодов вычисляет из полей промокода: «суть», срок,
 * прогресс, отдача. Одно место на ПК и телефон — строка таблицы, карточка,
 * лист-карточка и предпросмотр формы говорят одно и то же.
 */

export const PROMO_TYPES: AdminPromoTypeFilter[] = ['subscription_days', 'discount', 'trial', 'extra_hwid']

export function isPromoType(v: string | null | undefined): v is AdminPromoTypeFilter {
  return v != null && (PROMO_TYPES as string[]).includes(v)
}

/** Цвета типов — те же, что у прежних бейджей; фиксированные, чтобы тип узнавался в любой теме. */
const TYPE_TONE: Record<AdminPromoTypeFilter, { tile: string; soft: string; icon: string; border: string }> = {
  subscription_days: {
    tile: 'bg-blue-500/15 text-blue-600 dark:bg-blue-500/20 dark:text-blue-400',
    soft: 'bg-blue-500/10 dark:bg-blue-500/15',
    icon: 'text-blue-600 dark:text-blue-400',
    border: 'border-blue-500/35',
  },
  discount: {
    tile: 'bg-amber-500/15 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400',
    soft: 'bg-amber-500/10 dark:bg-amber-500/15',
    icon: 'text-amber-600 dark:text-amber-400',
    border: 'border-amber-500/35',
  },
  trial: {
    tile: 'bg-emerald-500/15 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-400',
    soft: 'bg-emerald-500/10 dark:bg-emerald-500/15',
    icon: 'text-emerald-600 dark:text-emerald-400',
    border: 'border-emerald-500/35',
  },
  extra_hwid: {
    tile: 'bg-violet-500/15 text-violet-600 dark:bg-violet-500/20 dark:text-violet-400',
    soft: 'bg-violet-500/10 dark:bg-violet-500/15',
    icon: 'text-violet-600 dark:text-violet-400',
    border: 'border-violet-500/35',
  },
}

const NEUTRAL_TONE = { tile: 'bg-muted text-muted-foreground', soft: 'bg-muted/50', icon: 'text-muted-foreground', border: 'border-border' }

export function promoTypeTone(type: string) {
  return isPromoType(type) ? TYPE_TONE[type] : NEUTRAL_TONE
}

const TYPE_ICON: Record<AdminPromoTypeFilter, LucideIcon> = {
  subscription_days: CalendarPlus,
  discount: Percent,
  trial: Timer,
  extra_hwid: Smartphone,
}

export function promoTypeIcon(type: string): LucideIcon {
  return isPromoType(type) ? TYPE_ICON[type] : Percent
}

export function promoTypeLabel(type: string, t: TFunction): string {
  return isPromoType(type) ? t(`admin.promos.types.${type}`) : type
}

/** Срок скидки после ввода — как его трактует promo/service.go. */
export type DiscountTtlKind = { kind: 'none' } | { kind: 'firstPay' } | { kind: 'hours'; hours: number }

export function discountTtlKind(promo: Pick<AdminPromoCodeDTO, 'discount_ttl_hours' | 'discount_max_subscription_payments_per_customer'>): DiscountTtlKind {
  const h = promo.discount_ttl_hours
  if (h == null) return { kind: 'none' }
  // Старые коды из бота: 0 часов — «до первой оплаты» у одноразовой скидки и «без срока» у многоразовой.
  if (h <= 0) return promo.discount_max_subscription_payments_per_customer === 1 ? { kind: 'firstPay' } : { kind: 'none' }
  return { kind: 'hours', hours: h }
}

function ttlShort(kind: DiscountTtlKind, t: TFunction): string {
  if (kind.kind === 'none') return t('admin.promos.essence.ttlNone')
  if (kind.kind === 'firstPay') return t('admin.promos.essence.ttlFirstPay')
  const h = kind.hours
  if (h >= 48 && h % 24 === 0) return t('admin.promos.essence.ttlDays', { count: h / 24 })
  return t('admin.promos.essence.ttlHours', { count: h })
}

function paymentsShort(n: number, t: TFunction): string {
  if (n <= 0) return t('admin.promos.essence.paymentsUnlimited')
  return t('admin.promos.essence.payments', { count: n })
}

export interface PromoEssence {
  title: string
  sub: string
}

/** «Суть» промокода: крупная строка — что даёт, мелкая — уточнение. */
export function describePromo(promo: AdminPromoCodeDTO, t: TFunction, tariffName?: string | null): PromoEssence {
  let title: string
  let sub: string
  switch (promo.type) {
    case 'subscription_days':
      title = t('admin.promos.essence.days', { count: promo.subscription_days ?? 0 })
      sub =
        promo.tariff_id != null && tariffName
          ? t('admin.promos.essence.tariffOnly', { name: tariffName })
          : t('admin.promos.essence.daysSub')
      break
    case 'trial':
      title = t('admin.promos.essence.trial', { count: promo.trial_days ?? 0 })
      sub = t('admin.promos.essence.trialSub')
      break
    case 'extra_hwid':
      title = t('admin.promos.essence.hwid', { count: promo.extra_hwid_delta ?? 0 })
      sub = t('admin.promos.essence.hwidSub')
      break
    case 'discount': {
      title = t('admin.promos.essence.discount', { percent: promo.discount_percent ?? 0 })
      const ttl = discountTtlKind(promo)
      // «До первой оплаты» уже говорит, что оплата одна.
      sub =
        ttl.kind === 'firstPay'
          ? ttlShort(ttl, t)
          : `${ttlShort(ttl, t)} · ${paymentsShort(promo.discount_max_subscription_payments_per_customer, t)}`
      break
    }
    default:
      title = promo.type
      sub = ''
  }
  if (promo.first_purchase_only) sub = sub ? `${sub} · ${t('admin.promos.essence.firstPurchase')}` : t('admin.promos.essence.firstPurchase')
  return { title, sub }
}

const pad2 = (n: number) => String(n).padStart(2, '0')

/** «01.11.26 11:52» — в часовом поясе браузера (фикс df34b5b6: не через UTC). */
export function formatPromoDateTime(iso: string | Date): string {
  const d = typeof iso === 'string' ? new Date(iso) : iso
  if (Number.isNaN(d.getTime())) return String(iso)
  return `${pad2(d.getDate())}.${pad2(d.getMonth() + 1)}.${pad2(d.getFullYear() % 100)} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

/** «01.11.2026» — дата создания и прочие даты без времени. */
export function formatPromoDate(iso: string | Date): string {
  const d = typeof iso === 'string' ? new Date(iso) : iso
  if (Number.isNaN(d.getTime())) return String(iso)
  return `${pad2(d.getDate())}.${pad2(d.getMonth() + 1)}.${d.getFullYear()}`
}

const SOON_MS = 3 * 24 * 60 * 60 * 1000

export type PromoExpiryTone = 'never' | 'normal' | 'soon' | 'expired'

/** Срок кода: просто дата и время, без «до», «истёк», «сегодня» (решение владельца); остальное — цветом. */
export function describeExpiry(validUntil: string | null | undefined, t: TFunction, now = Date.now()) {
  if (!validUntil) return { text: t('admin.promos.expires.never'), tone: 'never' as PromoExpiryTone }
  const date = formatPromoDateTime(validUntil)
  if (isPromoExpired(validUntil, now)) return { text: date, tone: 'expired' as PromoExpiryTone }
  const left = Date.parse(validUntil) - now
  return { text: date, tone: (left <= SOON_MS ? 'soon' : 'normal') as PromoExpiryTone }
}

/**
 * Красный — только когда скоро истечёт (≤ 3 суток): это повод что-то сделать.
 * Истёкший код — уже не срочно, просто серая дата (статус «Истёк» есть в чипе).
 */
export const EXPIRY_TONE_CLASS: Record<PromoExpiryTone, string> = {
  never: 'text-muted-foreground',
  normal: 'text-muted-foreground',
  soon: 'font-semibold text-rose-600 dark:text-rose-400',
  expired: 'text-muted-foreground/80',
}

export type PromoUsageTone = 'primary' | 'warn' | 'full' | 'off'

export function describeUsage(promo: AdminPromoCodeDTO) {
  const limited = hasPromoUseLimit(promo.max_uses)
  const max = limited ? (promo.max_uses as number) : null
  const pct = max ? Math.min(100, Math.round((promo.uses_count / max) * 100)) : 0
  const status = resolvePromoDisplayStatus(promo)
  let tone: PromoUsageTone = 'primary'
  if (status === 'inactive') tone = 'off'
  else if (max && promo.uses_count >= max) tone = 'full'
  else if (pct >= 80) tone = 'warn'
  return { used: promo.uses_count, max, pct, tone }
}

export const USAGE_BAR_CLASS: Record<PromoUsageTone, string> = {
  primary: 'bg-primary',
  warn: 'bg-amber-500',
  full: 'bg-rose-500',
  off: 'bg-muted-foreground/45',
}

/** Сколько процентов активировавших заплатили; null — когда активаций нет. */
export function paybackConversion(payback: AdminPromoPaybackDTO | undefined, uses: number): number | null {
  if (!payback || uses <= 0) return null
  return Math.round((payback.paid_customers / uses) * 100)
}

/** «8 400 ₽», «8 400 ₽ + 350 ⭐», «350 ⭐»; null — оплат нет. Рубли и Stars не складываем. */
export function formatPromoMoney(rub: number | undefined, stars: number | undefined, lang: string): string | null {
  const locale = lang.startsWith('en') ? 'en-US' : 'ru-RU'
  const parts: string[] = []
  if (rub && rub > 0) parts.push(`${rub.toLocaleString(locale, { maximumFractionDigits: 2 })} ₽`)
  if (stars && stars > 0) parts.push(`${stars.toLocaleString(locale, { maximumFractionDigits: 2 })} ⭐`)
  return parts.length ? parts.join(' + ') : null
}

/** Ключевое значение типа для листа-карточки: «7», «15%». */
export function promoKeyValue(promo: AdminPromoCodeDTO, t: TFunction): { label: string; value: string } {
  switch (promo.type) {
    case 'subscription_days':
      return { label: t('admin.promos.detail.daysValue'), value: String(promo.subscription_days ?? '—') }
    case 'trial':
      return { label: t('admin.promos.detail.trialValue'), value: String(promo.trial_days ?? '—') }
    case 'extra_hwid':
      return { label: t('admin.promos.detail.hwidValue'), value: String(promo.extra_hwid_delta ?? '—') }
    default:
      return { label: t('admin.promos.detail.discountValue'), value: `${promo.discount_percent ?? 0}%` }
  }
}
