import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useIsMobile } from '@/hooks/useIsMobile'
import { syncPushSubscription } from '@/lib/push'
import { cn } from '@/lib/utils'
import { UnreadBadge } from './UnreadBadge'
import { NotificationsPopover, NotificationsSheet } from './NotificationsSheet'
import { useNotificationsSheetState, useServiceWorkerRefresh, useUnreadNotifications } from './useNotifications'

let pushSynced = false

/**
 * Значок колокольчика — те же контуры, что у Bell из lucide, но колокол и
 * язычок отдельными путями: по нажатию они качаются по-разному (.bell-ring в
 * index.css). ring меняется — svg пересоздаётся, и анимация идёт заново.
 */
function BellIcon({ ring }: { ring: number }) {
  return (
    <svg
      key={ring}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      className={cn('size-[18px] overflow-visible', ring > 0 && 'bell-ring')}
    >
      <g className="bell-body">
        <path d="M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326" />
        <path className="bell-clapper" d="M10.268 21a2 2 0 0 0 3.464 0" />
      </g>
    </svg>
  )
}

/**
 * Колокольчик в шапке: у всех вошедших, с подпиской или без. На ПК лента —
 * окно под колокольчиком, на телефоне — шторка снизу. Открывается якорем
 * #notifications (его же ставит push по нажатию).
 */
export function NotificationBell({ className }: { className?: string }) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const anchorRef = useRef<HTMLDivElement>(null)
  const { data } = useUnreadNotifications()
  const { open, show, hide } = useNotificationsSheetState()
  // Сколько раз звенел: каждое открытие ленты — новый звон.
  const [ring, setRing] = useState(0)
  useServiceWorkerRefresh()

  useEffect(() => {
    if (pushSynced) return
    pushSynced = true
    void syncPushSubscription()
  }, [])

  const count = data?.total ?? 0
  const label = count > 0 ? t('notifications.bellUnread', { count }) : t('notifications.title')

  return (
    <div className="relative" ref={anchorRef}>
      <Button
        variant="ghost"
        size="icon"
        type="button"
        aria-label={label}
        aria-expanded={open}
        title={label}
        // Полуквадратная подложка — при наведении и пока лента открыта.
        className={cn('relative size-10 rounded-[12px]', open && 'bg-secondary', className)}
        onClick={() => {
          if (open) return hide()
          setRing((n) => n + 1)
          show()
        }}
      >
        <BellIcon ring={ring} />
        {count > 0 && (
          <UnreadBadge
            count={count > 9 ? '9+' : count}
            important={(data?.important ?? 0) > 0}
            className="absolute right-1 top-1 h-[18px] min-w-[18px] px-1 ring-2 ring-card"
          />
        )}
      </Button>
      {isMobile ? (
        <NotificationsSheet open={open} onClose={hide} />
      ) : (
        <NotificationsPopover open={open} onClose={hide} anchorRef={anchorRef} />
      )}
    </div>
  )
}
