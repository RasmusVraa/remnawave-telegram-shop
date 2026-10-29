import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { MonitorSmartphone, Zap } from 'lucide-react'

import { cn } from '@/lib/utils'
import { daysTone } from '@/lib/subscriptionTone'

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
 */
export function RenewSubscriptionButton({ days }: { days: number | null | undefined }) {
  const { t } = useTranslation()
  const tone = daysTone(days)
  if (tone === 'calm') return null

  if (tone === 'warn') {
    // Янтарь лежит на сплошной подложке: на голых 10% сквозь кнопку
    // просвечивала карточка, и она выглядела полупрозрачной.
    return (
      <Link
        to="/tariffs"
        className={cn(
          'inline-flex h-11 w-full items-center justify-center gap-2 rounded-lg border px-4 text-sm font-semibold transition-[background,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          'border-amber-400/80 bg-amber-50 text-amber-800 shadow-[0_6px_16px_-10px_rgb(217_119_6/0.55)] hover:bg-amber-100',
          'dark:border-amber-400/65 dark:text-amber-300 dark:shadow-[0_6px_18px_-10px_rgb(245_158_11/0.6)]',
          'dark:[background:linear-gradient(180deg,rgb(245_158_11/0.26),rgb(245_158_11/0.16)),hsl(var(--background))]',
          'dark:hover:[background:linear-gradient(180deg,rgb(245_158_11/0.34),rgb(245_158_11/0.22)),hsl(var(--background))]',
        )}
      >
        <Zap size={15} />
        {t('subscriptionPage.renewSubscription')}
      </Link>
    )
  }

  const expired = days == null || days <= 0

  return (
    <span className="cabinet-attn-sheen block">
      <Link
        to="/tariffs"
        className="cabinet-btn cabinet-btn-primary flex h-12 w-full items-center justify-center gap-2 rounded-lg bg-primary text-sm font-semibold text-primary-foreground shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] transition-all hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:scale-[0.98]"
      >
        <Zap size={16} />
        {expired ? t('subscriptionPage.resumeSubscription') : t('subscriptionPage.renewNow')}
      </Link>
    </span>
  )
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
}: {
  days: number | null | undefined
  devicesUsed: number
  devicesLimit: number
  id?: string
}) {
  const { t } = useTranslation()
  const tone = daysTone(days)
  const expired = days == null || days <= 0
  const demoted = tone === 'danger'

  if (expired) {
    return (
      <span
        id={id}
        aria-disabled
        className="inline-flex h-11 w-full cursor-not-allowed items-center justify-center gap-2 rounded-lg border border-border bg-muted/40 px-4 text-sm font-medium text-muted-foreground opacity-70"
      >
        <MonitorSmartphone size={16} />
        {t('subscriptionPage.connectDevice')}
      </span>
    )
  }

  const counter = devicesUsed > 0 && (
    <span
      className={cn(
        'ml-1 rounded-md px-1.5 py-0.5 text-[11px] tabular-nums',
        demoted ? 'bg-muted' : 'bg-white/20',
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
        'flex h-11 w-full items-center justify-center gap-2 rounded-lg text-sm transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        demoted
          ? 'border border-border bg-card/60 font-medium hover:bg-secondary'
          : 'cabinet-btn cabinet-btn-primary bg-primary font-semibold text-primary-foreground shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] hover:brightness-110 active:scale-[0.98]',
      )}
    >
      <MonitorSmartphone size={16} />
      {t('subscriptionPage.connectDevice')}
      {counter}
    </Link>
  )

  if (devicesUsed > 0 || demoted) return button
  return <span className="cabinet-attn-sheen block">{button}</span>
}

/** Пара кнопок: при срочности продление идёт первым. */
export function SubscriptionActions({
  days,
  devicesUsed,
  devicesLimit,
  connectId,
}: {
  days: number | null | undefined
  devicesUsed: number
  devicesLimit: number
  connectId?: string
}) {
  return (
    <div className="space-y-2">
      <RenewSubscriptionButton days={days} />
      <ConnectDeviceButton
        days={days}
        devicesUsed={devicesUsed}
        devicesLimit={devicesLimit}
        id={connectId}
      />
    </div>
  )
}
