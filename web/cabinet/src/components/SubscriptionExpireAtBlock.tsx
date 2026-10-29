import { useTranslation } from 'react-i18next'
import { AlertTriangle, Calendar } from 'lucide-react'

import { cn, formatDate } from '@/lib/utils'
import { daysTone, type ToneLevel } from '@/lib/subscriptionTone'

type Props = {
  expireAt: string | null | undefined
  lang: string
  days: number | null
  isActive: boolean
  className?: string
}

/**
 * Срок действия подписки.
 *
 * Спокойное состояние **нейтральное**, а не зелёное, как было раньше. Причины
 * две. Бейдж «Активна» на той же карточке уже зелёный, и второй зелёный
 * дублирует его сообщение. Главное же: если норма покрашена, то появление
 * цвета перестаёт быть сигналом — остаётся только смена оттенка, а её глаз
 * ловит заметно хуже, чем переход «было серым, стало янтарным».
 *
 * Цвет никогда не единственный канал: вместе с ним меняются текст и иконка.
 * Красный и зелёный — самая частая пара при дальтонизме.
 *
 * Пороги общие с кнопкой продления (см. lib/subscriptionTone) — раньше они
 * расходились, и ровно на семи днях страница уже звала продлевать, а блок
 * даты ещё был зелёным.
 */
export function SubscriptionExpireAtBlock({ expireAt, lang, days, isActive, className }: Props) {
  const { t } = useTranslation()
  const expired = !isActive || days == null || days <= 0
  const tone: ToneLevel = expired ? 'danger' : daysTone(days)
  const skin = TONE_SKIN[tone]

  return (
    <div className={cn('flex items-center gap-3 rounded-xl border px-3.5 py-3', skin.box, className)}>
      <span
        className={cn('inline-flex size-9 shrink-0 items-center justify-center rounded-lg', skin.icon)}
      >
        {tone === 'calm' ? <Calendar size={15} /> : <AlertTriangle size={15} />}
      </span>
      <div className="min-w-0 flex-1">
        <p className={cn('text-[11px] uppercase tracking-[0.14em]', skin.label)}>
          {expired ? t('subscriptionPage.expiredBlockTitle') : t('subscriptionPage.expireAt')}
        </p>
        {/* У истёкшей подписки даты нет — прочерк на её месте выбивался из
            блока, хватает заголовка и подсказки. */}
        {!expired && expireAt && <p className="text-[0.95rem] font-medium">{formatDate(expireAt, lang)}</p>}
        <p className={cn('text-xs', skin.note)}>
          {expired
            ? t('subscriptionPage.expiredRestoreHint')
            : t('subscriptionPage.daysLeft', { n: days })}
        </p>
      </div>
    </div>
  )
}

const TONE_SKIN: Record<ToneLevel, { box: string; icon: string; label: string; note: string }> = {
  calm: {
    box: 'border-border/60 bg-background/40',
    icon: 'bg-muted text-muted-foreground',
    label: 'text-muted-foreground',
    note: 'text-muted-foreground',
  },
  warn: {
    box: 'border-amber-400/60 bg-amber-500/10 dark:border-amber-300/30',
    icon: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
    label: 'text-amber-800/90 dark:text-amber-300/90',
    note: 'text-amber-700 dark:text-amber-300',
  },
  danger: {
    box: 'border-destructive/55 bg-destructive/10',
    icon: 'bg-destructive/15 text-destructive',
    label: 'text-destructive/80',
    note: 'text-destructive',
  },
}
