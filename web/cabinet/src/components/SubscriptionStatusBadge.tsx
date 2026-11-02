import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { daysTone } from '@/lib/subscriptionTone'

/**
 * Статус подписки по общей шкале: зелёный → янтарный → красный.
 *
 * Пробный период из шкалы исключён — он короткий и по общим порогам был бы
 * «Заканчивается» с первого дня. Исчерпанный трафик называется прямо:
 * панель помечает такого пользователя неактивным, и раньше здесь стояло
 * «Истекла», хотя срок подписки ещё не вышел.
 */
export function SubscriptionStatusBadge({
  days,
  isActive,
  isTrial,
  trafficExhausted,
}: {
  days: number | null
  isActive: boolean
  isTrial?: boolean
  trafficExhausted?: boolean
}) {
  const { t } = useTranslation()
  const tone = isActive ? (isTrial ? 'calm' : daysTone(days)) : 'danger'
  const base = 'inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full border px-3 py-1 text-[13px] font-bold'

  if (!isActive) {
    return (
      <span className={cn(base, 'border-destructive/45 bg-destructive/10 text-destructive')}>
        <span className="size-[7px] rounded-full bg-destructive" />
        {trafficExhausted ? t('home.statusTrafficExhausted') : t('subscriptionPage.statusExpired')}
      </span>
    )
  }

  if (tone !== 'calm') {
    const danger = tone === 'danger'
    return (
      <span
        className={cn(
          base,
          danger
            ? 'border-destructive/45 bg-destructive/10 text-destructive'
            : 'border-amber-400/50 bg-amber-500/10 text-amber-700 dark:text-amber-300',
        )}
      >
        <span className={cn('size-[7px] rounded-full', danger ? 'bg-destructive' : 'bg-amber-500')} />
        {t('subscriptionPage.statusEnding')}
      </span>
    )
  }

  return (
    <span
      className={cn(
        base,
        'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-400/40 dark:bg-emerald-500/15 dark:text-emerald-300',
      )}
    >
      <span className="size-[7px] rounded-full bg-emerald-500" />
      {t('subscriptionPage.statusActive')}
    </span>
  )
}
