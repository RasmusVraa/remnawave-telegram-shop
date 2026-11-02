import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { MonitorSmartphone, Zap } from 'lucide-react'

import { cn } from '@/lib/utils'
import { daysTone, type ToneLevel } from '@/lib/subscriptionTone'

/**
 * Продление подписки. Эскалация в три ступени по общим порогам:
 *
 *  - больше 7 дней — кнопки нет, делать нечего;
 *  - 3–7 дней — вторичная янтарная кнопка, главным действием остаётся
 *    подключение устройства;
 *  - 3 дня и меньше или подписка истекла — продление становится главным
 *    действием, с бликом.
 *
 * Последняя ступень содержательная, а не косметическая: когда подписка
 * вот-вот кончится, подключать новое устройство бессмысленно, и самая
 * заметная кнопка на экране не должна предлагать именно это.
 *
 * Блика на ступени 3–7 нет намеренно: сигналом там служит само появление
 * кнопки, которой раньше не было. Если мерцают обе ступени, эскалация
 * сплющивается и «осталось 3 дня» перестаёт отличаться от «осталось 7».
 *
 * Пробный период из шкалы исключён: он короткий (обычно 3 дня), и по общим
 * порогам был бы «красным» с первого дня.
 */

/** Главная кнопка страницы: пилюля 52px, как в макете главной. */
const PRIMARY_PILL =
  'cabinet-btn cabinet-btn-primary flex h-[52px] w-full items-center justify-center gap-2.5 rounded-full bg-primary text-base font-bold text-primary-foreground shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] transition-all hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:scale-[0.98]'

export function subscriptionDaysTone(days: number | null | undefined, isTrial?: boolean): ToneLevel {
  const expired = days == null || days <= 0
  if (isTrial && !expired) return 'calm'
  return daysTone(days)
}

/** Янтарная «Продлить подписку» для 7–4 дней — без блика. */
export function RenewWarnButton({ className }: { className?: string }) {
  const { t } = useTranslation()
  // Янтарь лежит на сплошной подложке: на голых 10% сквозь кнопку
  // просвечивала карточка, и она выглядела полупрозрачной.
  return (
    <Link
      to="/tariffs"
      className={cn(
        'inline-flex h-11 w-full items-center justify-center gap-2 rounded-full border px-4 text-[15px] font-semibold transition-[background,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        'border-amber-400/80 bg-amber-50 text-amber-800 shadow-[0_6px_16px_-10px_rgb(217_119_6/0.55)] hover:bg-amber-100',
        'dark:border-amber-400/65 dark:text-amber-300 dark:shadow-[0_6px_18px_-10px_rgb(245_158_11/0.6)]',
        'dark:[background:linear-gradient(180deg,rgb(245_158_11/0.26),rgb(245_158_11/0.16)),hsl(var(--background))]',
        'dark:hover:[background:linear-gradient(180deg,rgb(245_158_11/0.34),rgb(245_158_11/0.22)),hsl(var(--background))]',
        className,
      )}
    >
      <Zap size={16} />
      {t('subscriptionPage.renewSubscription')}
    </Link>
  )
}

/** «Продлить сейчас» / «Возобновить подписку» — главная кнопка с бликом. */
export function RenewNowButton({ expired }: { expired: boolean }) {
  const { t } = useTranslation()
  return (
    <span className="cabinet-attn-sheen block rounded-full">
      <Link to="/tariffs" className={PRIMARY_PILL}>
        <Zap size={20} />
        {expired ? t('subscriptionPage.resumeSubscription') : t('subscriptionPage.renewNow')}
      </Link>
    </span>
  )
}

export function RenewSubscriptionButton({
  days,
  isTrial,
}: {
  days: number | null | undefined
  isTrial?: boolean
}) {
  const tone = subscriptionDaysTone(days, isTrial)
  if (tone === 'calm') return null
  if (tone === 'warn') return <RenewWarnButton />
  return <RenewNowButton expired={days == null || days <= 0} />
}

/**
 * Подключение устройства.
 *
 * Блик — только пока не подключено ни одного устройства: кнопку не находят
 * в основном новички, а у остальных вечное движение превращается в шум.
 * При истёкшей подписке кнопка неактивна: подключать нечего.
 */
export function ConnectDeviceButton({
  days,
  devicesUsed,
  devicesLimit,
  id,
  isTrial,
}: {
  days: number | null | undefined
  devicesUsed: number
  devicesLimit: number
  id?: string
  isTrial?: boolean
}) {
  const { t } = useTranslation()
  const tone = subscriptionDaysTone(days, isTrial)
  const expired = days == null || days <= 0
  const demoted = tone === 'danger'

  if (expired) {
    return (
      <span
        id={id}
        aria-disabled
        className="inline-flex h-[52px] w-full cursor-not-allowed items-center justify-center gap-2.5 rounded-full border border-border bg-muted/40 px-4 text-base font-medium text-muted-foreground opacity-70"
      >
        <MonitorSmartphone size={20} />
        {t('subscriptionPage.connectDevice')}
      </span>
    )
  }

  const counter = devicesUsed > 0 && (
    <span
      className={cn(
        'rounded-full px-2 py-0.5 text-xs font-bold tabular-nums',
        demoted ? 'bg-muted' : 'bg-white/[0.22]',
      )}
    >
      {devicesUsed}
      {devicesLimit > 0 ? `/${devicesLimit}` : ''}
    </span>
  )

  const button = (
    <Link
      id={id}
      to="/connections"
      className={cn(
        demoted
          ? 'flex h-[52px] w-full items-center justify-center gap-2.5 rounded-full border border-border bg-card/60 text-base font-semibold transition-all hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
          : PRIMARY_PILL,
      )}
    >
      <MonitorSmartphone size={20} />
      {t('subscriptionPage.connectDevice')}
      {counter}
    </Link>
  )

  if (devicesUsed > 0 || demoted) return button
  return <span className="cabinet-attn-sheen block rounded-full">{button}</span>
}

/** Пара кнопок: при срочности продление идёт первым. */
export function SubscriptionActions({
  days,
  devicesUsed,
  devicesLimit,
  connectId,
  isTrial,
}: {
  days: number | null | undefined
  devicesUsed: number
  devicesLimit: number
  connectId?: string
  isTrial?: boolean
}) {
  return (
    <div className="space-y-2">
      <RenewSubscriptionButton days={days} isTrial={isTrial} />
      <ConnectDeviceButton
        days={days}
        devicesUsed={devicesUsed}
        devicesLimit={devicesLimit}
        id={connectId}
        isTrial={isTrial}
      />
    </div>
  )
}
