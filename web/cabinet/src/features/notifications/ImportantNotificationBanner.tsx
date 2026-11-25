import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, ChevronRight } from 'lucide-react'

import { api, SUBSCRIPTION_STALE_MS } from '@/lib/api'
import { daysUntil } from '@/lib/utils'

import { stripLeadingEmoji } from './notificationMeta'
import { useNotificationsSheetState, useUnreadNotifications } from './useNotifications'

/** Напоминание уходит за 3 дня до конца — позже этого срока плашка не нужна. */
const EXPIRING_BANNER_DAYS = 3

/** Строки текста без разметки: у рассылки первая строка обычно и есть заголовок. */
function plainLines(html: string): string[] {
  // DOMParser даёт инертный документ: ни скриптов, ни загрузки картинок —
  // в отличие от innerHTML на отдельном div, где <img onerror> сработал бы.
  const doc = new DOMParser().parseFromString(html.replace(/<br\s*\/?>/gi, '\n'), 'text/html')
  return (doc.body.textContent ?? '')
    .split('\n')
    .map((l) => l.replace(/\s+/g, ' ').trim())
    .filter(Boolean)
}

/**
 * Плашка важного на главной: висит, пока свежее важное не прочитано. Нажатие
 * открывает ленту; снимет важность админ — плашка пропадёт у всех. Цвет —
 * красный, как у важного в ленте и у счётчика колокольчика.
 */
export function ImportantNotificationBanner() {
  const { t } = useTranslation()
  const { data } = useUnreadNotifications()
  const { show } = useNotificationsSheetState()
  const item = data?.latest_important
  // «Подписка заканчивается» живёт, только пока это правда: после окончания или продления
  // плашка устарела (запись в ленте остаётся). Пока подписка не загружена — не показываем.
  const { data: sub } = useQuery({
    queryKey: ['subscription'],
    queryFn: () => api.subscription(),
    staleTime: SUBSCRIPTION_STALE_MS,
    retry: 1,
    enabled: item?.kind === 'expiring',
  })
  if (!item) return null
  if (item.kind === 'expiring') {
    if (!sub?.expire_at) return null
    const left = daysUntil(sub.expire_at)
    if (left <= 0 || left > EXPIRING_BANNER_DAYS) return null
  }

  const lines = plainLines(item.body)
  // Эмодзи в начале заголовка («⚠️ Важно») убираем — рядом уже значок.
  const title = stripLeadingEmoji(item.title || lines.shift() || '') || t('notifications.importantBadge')
  const text = lines.join(' ')

  return (
    <button
      type="button"
      onClick={show}
      className="flex w-full items-start gap-3 rounded-2xl border border-red-500/40 bg-red-500/[0.1] px-3.5 py-3 text-left transition-colors hover:bg-red-500/[0.14] dark:border-red-500/45 dark:bg-red-500/[0.2] dark:hover:bg-red-500/[0.25]"
    >
      <AlertTriangle size={19} className="mt-px shrink-0 text-red-600 dark:text-red-400" />
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-semibold text-red-950 dark:text-red-50">{title}</span>
        {text && <span className="mt-0.5 line-clamp-2 text-[13px] leading-snug text-red-900/80 dark:text-red-100/75">{text}</span>}
      </span>
      <ChevronRight size={17} className="shrink-0 self-center text-red-700/70 dark:text-red-300/70" />
    </button>
  )
}
