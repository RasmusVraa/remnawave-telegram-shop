import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Gift, Loader2, Route, Trash2 } from 'lucide-react'

import '@/lib/splitColor'
import { api } from '@/lib/api'
import { bypassLabel } from '@/lib/bypassLabel'
import { cn } from '@/lib/utils'
import { formatDecimals } from '@/lib/format'
import { surface } from '../Surface'
import { AdminConfirmModal } from '../AdminConfirmModal'
import { formatAdminApiError } from '../../utils/formatAdminApiError'
import { formatAdminDateTime } from '../../utils/datetime'
import { useErrorHaptic } from '@/lib/haptic'

const GB = 1024 ** 3
const gb = (bytes: number) => formatDecimals(bytes / GB, bytes / GB >= 10 ? 0 : 1)

/**
 * Раздельный трафик в карточке пользователя: расход обхода, расход по нодам за месяц,
 * действующие пакеты (купленные и выданные) и выдача пакета без оплаты.
 */
export function AdminUserTrafficSplit({ userId }: { userId: number }) {
  const { t, i18n } = useTranslation()
  const qc = useQueryClient()
  const locale = i18n.language?.startsWith('en') ? 'en-GB' : 'ru-RU'
  const key = ['admin-user-traffic-split', userId]
  const { data, isLoading, isError } = useQuery({ queryKey: key, queryFn: () => api.adminUserTrafficSplit(userId) })

  const [kind, setKind] = useState<'main' | 'bypass'>('bypass')
  const [amount, setAmount] = useState('10')
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)
  const [revokeId, setRevokeId] = useState<number | null>(null)

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: key })
    void qc.invalidateQueries({ queryKey: ['admin-user', userId] })
  }
  const grant = useMutation({
    meta: { haptics: true },
    mutationFn: () => api.adminUserTrafficGrant(userId, kind, Math.round(Number(amount) * GB)),
    onSuccess: () => {
      setError(null)
      refresh()
    },
    onError: (e) => setError(formatAdminApiError(e, t)),
  })
  const revoke = useMutation({
    meta: { haptics: true },
    mutationFn: (extraId: number) => api.adminUserTrafficRevoke(userId, extraId),
    onSuccess: () => {
      setRevokeId(null)
      refresh()
    },
    onError: (e) => {
      setRevokeId(null)
      setError(formatAdminApiError(e, t))
    },
  })

  if (isLoading) return <div className="h-20 animate-pulse rounded-xl bg-muted/50" />
  if (isError || !data) return null

  const label = bypassLabel(data.label)
  const b = data.bypass
  const pct = b && b.limit_bytes > 0 ? Math.min(100, (b.used_bytes / b.limit_bytes) * 100) : 0
  const amountOk = Number(amount) > 0

  return (
    <div className={surface('raised', 'space-y-4 rounded-xl p-3')}>
      <p className="flex items-center gap-2 text-sm font-medium">
        <Route className="size-4 text-[hsl(var(--split-bypass))]" aria-hidden />
        {t('admin.users.trafficSplit.title', { label })}
      </p>

      {b ? (
        <div>
          <div className="flex items-baseline justify-between gap-3 text-sm">
            <span className="text-muted-foreground">{t('admin.users.trafficSplit.bypassUsed', { label })}</span>
            <span className="font-semibold tabular-nums">
              {gb(b.used_bytes)} / {gb(b.limit_bytes)} {t('dashboard.gigabytes')}
            </span>
          </div>
          <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted">
            <div
              className={cn('h-full rounded-full', pct >= 100 ? 'bg-red-500' : pct >= 80 ? 'bg-amber-500' : 'bg-[hsl(var(--split-bypass))]')}
              style={{ width: `${Math.max(2, pct)}%` }}
            />
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            {b.squad_removed ? (
              <span className="font-medium text-red-500">{t('admin.users.trafficSplit.squadRemoved')}</span>
            ) : (
              t('admin.users.trafficSplit.squadActive')
            )}
            {' · '}
            {t('admin.users.trafficSplit.plan', { base: gb(b.base_bytes), extra: gb(b.extra_bytes) })}
            {b.checked_at && ` · ${t('admin.users.trafficSplit.checked')} ${formatAdminDateTime(b.checked_at, locale)}`}
          </p>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">{t('admin.users.trafficSplit.noBypass')}</p>
      )}

      {data.nodes.length > 0 && (
        <div>
          <p className="mb-1.5 text-xs font-medium text-muted-foreground">{t('admin.users.trafficSplit.nodes')}</p>
          <ul className="space-y-1 text-sm">
            {data.nodes.map((n) => (
              <li key={n.name} className="flex items-center justify-between gap-3">
                <span className="flex min-w-0 items-center gap-2">
                  <i className={cn('size-2 shrink-0 rounded-full', n.bypass ? 'bg-[hsl(var(--split-bypass))]' : 'bg-primary')} />
                  <span className="truncate">{n.name}</span>
                  {n.bypass && <span className="text-[11px] text-[hsl(var(--split-bypass))]">{label}</span>}
                </span>
                <span className="tabular-nums text-muted-foreground">
                  {gb(n.bytes)} {t('dashboard.gigabytes')}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div>
        <p className="mb-1.5 text-xs font-medium text-muted-foreground">{t('admin.users.trafficSplit.extras')}</p>
        {data.extras.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t('admin.users.trafficSplit.noExtras')}</p>
        ) : (
          <ul className="space-y-1.5 text-sm">
            {data.extras.map((e) => (
              <li key={e.id} className="flex items-center gap-2">
                <span className="min-w-0 flex-1">
                  +{gb(e.bytes)} {t('dashboard.gigabytes')} · {e.kind === 'bypass' ? label : t('trafficSplit.tabMain')}
                  <span className="text-xs text-muted-foreground">
                    {' '}
                    · {e.paid ? t('admin.users.trafficSplit.paid') : t('admin.users.trafficSplit.granted')} ·{' '}
                    {t('trafficSplit.until', { date: formatAdminDateTime(e.valid_until, locale) })}
                  </span>
                </span>
                <button
                  type="button"
                  disabled={revoke.isPending}
                  onClick={() => setRevokeId(e.id)}
                  className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-destructive disabled:opacity-50"
                  aria-label={t('admin.users.trafficSplit.revoke')}
                >
                  {revoke.isPending && revoke.variables === e.id ? <Loader2 className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="border-t border-border/60 pt-3">
        <p className="mb-2 text-xs font-medium text-muted-foreground">{t('admin.users.trafficSplit.grantTitle')}</p>
        <div className="flex flex-wrap items-center gap-2">
          <div className="grid grid-cols-2 gap-1 rounded-lg border border-border/60 bg-muted/30 p-0.5 text-xs">
            {(['bypass', 'main'] as const).map((k) => (
              <button
                key={k}
                type="button"
                onClick={() => setKind(k)}
                className={cn('rounded-md px-3 py-1.5 font-medium', kind === k ? 'bg-card shadow-sm' : 'text-muted-foreground')}
              >
                {k === 'bypass' ? label : t('trafficSplit.tabMain')}
              </button>
            ))}
          </div>
          <input
            type="number"
            min={0}
            step={1}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            className="admin-input w-24 px-3 py-2"
            aria-label={t('dashboard.gigabytes')}
          />
          <span className="text-sm text-muted-foreground">{t('dashboard.gigabytes')}</span>
          <button
            type="button"
            disabled={!amountOk || grant.isPending}
            onClick={() => grant.mutate()}
            className="ml-auto inline-flex items-center gap-2 rounded-lg bg-primary px-3.5 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
          >
            {grant.isPending ? <Loader2 className="size-4 animate-spin" /> : <Gift className="size-4" />}
            {t('admin.users.trafficSplit.grant')}
          </button>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">{t('admin.users.trafficSplit.grantHint')}</p>
        {error && <p className="mt-2 text-xs text-destructive">{error}</p>}
      </div>
      <AdminConfirmModal
        open={revokeId !== null}
        onClose={() => setRevokeId(null)}
        onConfirm={() => revokeId !== null && revoke.mutate(revokeId)}
        title={t('admin.users.trafficSplit.revokeTitle')}
        message={t('admin.users.trafficSplit.revokeBody')}
        confirmLabel={t('admin.users.trafficSplit.revoke')}
        variant="destructive"
        loading={revoke.isPending}
      />
    </div>
  )
}
