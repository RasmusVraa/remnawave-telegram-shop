import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { RefreshCw, CheckCircle2, AlertCircle, AlertTriangle } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import type { AdminSyncStatusDTO } from '@/lib/types/admin'

const statusKey = ['admin', 'sync', 'status'] as const

export default function AdminSyncPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  // Синхронизация идёт в фоне; пока она не закончилась, опрашиваем итог.
  const status = useQuery({
    queryKey: statusKey,
    queryFn: () => api.adminSyncStatus(),
    refetchInterval: (query) => (query.state.data?.state === 'running' ? 2000 : false),
  })

  const syncMutation = useMutation({
    mutationFn: () => api.adminSync(),
    onSuccess: (data) => queryClient.setQueryData(statusKey, data),
  })

  const st = status.data
  const running = syncMutation.isPending || st?.state === 'running'

  return (
    <AdminLayout>
      <div className="space-y-6">
        <AdminPageHeader
          icon={RefreshCw}
          title={t('admin.sync.title')}
          subtitle={t('admin.sync.subtitle')}
          accent="cyan"
        />

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('admin.sync.cardTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              {t('admin.sync.description')}
            </p>

            <Button
              onClick={() => syncMutation.mutate()}
              disabled={running}
              className="gap-2"
            >
              <RefreshCw className={`size-4 ${running ? 'animate-spin' : ''}`} />
              {running ? t('admin.sync.inProgress') : t('admin.sync.trigger')}
            </Button>

            {st && !running && <SyncResult st={st} />}

            {syncMutation.isError && (
              <div className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
                <AlertCircle className="size-4 shrink-0" />
                {t('admin.sync.error')}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </AdminLayout>
  )
}

function SyncResult({ st }: { st: AdminSyncStatusDTO }) {
  const { t } = useTranslation()
  const counts = t('admin.sync.counts', { created: st.created, updated: st.updated, deleted: st.deleted, kept: st.kept })
  const when = st.finished_at ? new Date(st.finished_at).toLocaleString() : ''

  if (st.state === 'ok') {
    return (
      <div className="space-y-1 rounded-md border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-400">
        <div className="flex items-center gap-2">
          <CheckCircle2 className="size-4 shrink-0" />
          {t('admin.sync.done', { when })}
        </div>
        <div>{counts}</div>
      </div>
    )
  }
  if (st.state === 'delete_blocked') {
    return (
      <div className="space-y-1 rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400">
        <div className="flex items-center gap-2">
          <AlertTriangle className="size-4 shrink-0" />
          {t('admin.sync.deleteBlocked')}
        </div>
        <div>{counts}</div>
      </div>
    )
  }
  if (st.state === 'busy' || st.state === 'failed') {
    return (
      <div className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
        <AlertCircle className="size-4 shrink-0" />
        {st.state === 'busy' ? t('admin.sync.busy') : t('admin.sync.failed')}
      </div>
    )
  }
  return null
}
