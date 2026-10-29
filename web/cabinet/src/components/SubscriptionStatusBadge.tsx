import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { daysTone } from '@/lib/subscriptionTone'

/**
 * Статус подписки по общей шкале: зелёный → янтарный → красный.
 *
 * Один на главную и страницу подписки: раньше у страницы подписки был свой
 * бейдж, который до последнего дня оставался зелёным «Активна», хотя главная
 * уже показывала «Заканчивается».
 */
export function SubscriptionStatusBadge({ days, isActive }: { days: number | null; isActive: boolean }) {
  const { t } = useTranslation()
  const tone = isActive ? daysTone(days) : 'danger'

  if (!isActive) {
    return (
      <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-destructive/45 bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">
        <span className="size-1.5 rounded-full bg-destructive" />
        {t('subscriptionPage.statusExpired')}
      </span>
    )
  }

  if (tone !== 'calm') {
    const danger = tone === 'danger'
    return (
      <span
        className={cn(
          'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium',
          danger
            ? 'border-destructive/45 bg-destructive/10 text-destructive'
            : 'border-amber-400/50 bg-amber-500/10 text-amber-700 dark:text-amber-300',
        )}
      >
        <span className={cn('size-1.5 rounded-full', danger ? 'bg-destructive' : 'bg-amber-500')} />
        {t('subscriptionPage.statusEnding')}
      </span>
    )
  }

  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:border-emerald-400/30 dark:bg-emerald-500/15 dark:text-emerald-200">
      <span className="size-1.5 rounded-full bg-emerald-500" />
      {t('subscriptionPage.statusActive')}
    </span>
  )
}
