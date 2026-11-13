import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { History } from 'lucide-react'

import { api } from '@/lib/api'
import { Skeleton } from '@/components/ui/skeleton'
import { AdminModal } from '../components/AdminModal'
import { EventList } from './EventList'
import { EventFilterMenu, type FeedFilter } from './MonitoringMenus'
import { useEventFeed } from './useEventFeed'

/** Сколько событий у клиента — для подписи кнопки «История · N». */
export function useAdminUserEventCount(customerId: number | undefined) {
  return useQuery({
    queryKey: ['admin-user-event-count', customerId],
    queryFn: () => api.adminMonitoringEvents({ customerId, limit: 1 }),
    enabled: !!customerId,
    staleTime: 30_000,
    select: (d) => d.total ?? d.items?.length ?? 0,
  })
}

/** Подпись кнопки: «История · 47», пока число неизвестно — просто «История». */
export function historyButtonLabel(t: (k: string, o?: Record<string, unknown>) => string, count?: number) {
  return count ? t('admin.monitoring.history.button', { count }) : t('admin.monitoring.history.title')
}

/**
 * Окно «История» из блока действий карточки клиента: все события этого
 * человека. На ПК — обычное окно по центру, на телефоне — шторка снизу, как
 * остальные окна админки. Нажатия в боте в истории видны всегда, если пишутся.
 */
export function AdminUserHistoryModal({
  customerId,
  open,
  onClose,
}: {
  customerId: number
  open: boolean
  onClose: () => void
}) {
  if (!open) return null
  return <HistoryModal customerId={customerId} onClose={onClose} />
}

function HistoryModal({ customerId, onClose }: { customerId: number; onClose: () => void }) {
  const { t } = useTranslation()
  const [filter, setFilter] = useState<FeedFilter>({ categories: [], botActions: false })
  const categories = useMemo(() => [...filter.categories], [filter.categories])
  const feed = useEventFeed({ categories, botActions: false, customerId, live: false, pageSize: 40 })

  return (
    <AdminModal
      open
      onClose={onClose}
      title={t('admin.monitoring.history.title')}
      icon={History}
      size="lg"
      // Минимальная высота: раскрытый список фильтра не должен обрезаться
      // краем окна, даже когда событий у клиента нет.
      panelClassName="min-h-[min(30rem,92dvh)] sm:max-h-[min(86dvh,860px)]"
      bodyClassName="px-4 pb-4 pt-3 sm:px-5"
      hideClose
      headerAction={<EventFilterMenu value={filter} onChange={setFilter} botActionsAvailable={false} align="right" />}
    >
      {feed.loading && feed.items.length === 0 ? (
        <div className="space-y-2 pt-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-11 rounded-xl" />
          ))}
        </div>
      ) : feed.items.length === 0 ? (
        <p className="py-10 text-center text-sm text-muted-foreground">
          {feed.error ? t('admin.monitoring.error') : t('admin.monitoring.history.empty')}
        </p>
      ) : (
        <EventList
          items={feed.items}
          fresh={feed.fresh}
          withCustomer={false}
          hasMore={feed.hasMore}
          loadingMore={feed.loadingMore}
          onLoadMore={() => void feed.loadMore()}
          resetKey={categories.join(',')}
        />
      )}
    </AdminModal>
  )
}
