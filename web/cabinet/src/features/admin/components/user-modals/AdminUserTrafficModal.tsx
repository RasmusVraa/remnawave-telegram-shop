import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Gauge, Infinity as InfinityIcon, RotateCcw } from 'lucide-react'

import { AdminModal } from '../AdminModal'
import { AdminModalSaveFooter } from '../AdminModalSaveFooter'
import { AdminConfirmModal } from '../AdminConfirmModal'
import { cn } from '@/lib/utils'
import { formatDecimals } from '@/lib/format'
import { surface } from '../Surface'
import { formatAdminApiError } from '../../utils/formatAdminApiError'
import { formatAdminDateTime } from '../../utils/datetime'
import {
  useAdminUserSetTraffic,
  useAdminUserSetStrategy,
  useAdminUserResetTraffic,
  type AdminUserPanelResponse,
} from '../../hooks/useAdminUsers'
import { trafficStrategyLabel } from './strategyLabels'
import { AdminUserTrafficSplit } from './AdminUserTrafficSplit'
import { useErrorHaptic } from '@/lib/haptic'

const GB = 1024 * 1024 * 1024

const chipClass =
  'rounded-full border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent'
const chipActiveClass = 'border-primary bg-primary/10 text-primary hover:bg-primary/15'

interface Props {
  open: boolean
  onClose: () => void
  userId: number
  panel: AdminUserPanelResponse
  onSuccess?: (message: string) => void
  onError?: (message: string) => void
}

export function AdminUserTrafficModal({
  open,
  onClose,
  userId,
  panel,
  onSuccess,
  onError,
}: Props) {
  const { t, i18n } = useTranslation()
  const dateLocale = i18n.language?.startsWith('en') ? 'en-GB' : 'ru-RU'
  const rw = panel.rw!
  const setTraffic = useAdminUserSetTraffic(userId)
  const setStrategy = useAdminUserSetStrategy(userId)
  const resetTraffic = useAdminUserResetTraffic(userId)

  const [draftBytes, setDraftBytes] = useState(rw.traffic_limit_bytes)
  const [draftStrategy, setDraftStrategy] = useState(rw.traffic_limit_strategy)
  const [customTrafficGB, setCustomTrafficGB] = useState('')
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)
  const [confirmReset, setConfirmReset] = useState(false)
  const [saving, setSaving] = useState(false)

  // saving держит кнопку и в паузе между двумя запросами, когда ни один isPending не взведён.
  const isPending = saving || setTraffic.isPending || setStrategy.isPending

  useEffect(() => {
    if (open) {
      setDraftBytes(rw.traffic_limit_bytes)
      setDraftStrategy(rw.traffic_limit_strategy)
      setCustomTrafficGB(rw.traffic_limit_bytes > 0 ? String(rw.traffic_limit_bytes / GB) : '0')
      setError(null)
      setConfirmReset(false)
    }
  }, [open, rw.traffic_limit_bytes, rw.traffic_limit_strategy])

  const handleClose = () => {
    setError(null)
    onClose()
  }

  const handleSave = async () => {
    setError(null)
    setSaving(true)
    try {
      const trafficChanged = draftBytes !== rw.traffic_limit_bytes
      const strategyChanged = draftStrategy !== rw.traffic_limit_strategy

      if (trafficChanged) {
        await setTraffic.mutateAsync(draftBytes)
      }
      if (strategyChanged) {
        await setStrategy.mutateAsync(draftStrategy)
      }

      if (trafficChanged || strategyChanged) {
        onSuccess?.(t('admin.feedback.saved'))
      }
      handleClose()
    } catch (e) {
      const msg = formatAdminApiError(e, t)
      setError(msg)
      onError?.(msg)
    } finally {
      setSaving(false)
    }
  }

  const handleResetTraffic = () => {
    resetTraffic.mutate(undefined, {
      onSuccess: () => {
        setConfirmReset(false)
        onSuccess?.(t('admin.feedback.resetTrafficSuccess'))
      },
      onError: (e) => {
        const msg = formatAdminApiError(e, t)
        setError(msg)
        onError?.(msg)
        setConfirmReset(false)
      },
    })
  }

  const hasChanges =
    draftBytes !== rw.traffic_limit_bytes || draftStrategy !== rw.traffic_limit_strategy

  const usedGb = rw.traffic_used_bytes / GB
  const limitGb = rw.traffic_limit_bytes > 0 ? rw.traffic_limit_bytes / GB : 0
  const percent = limitGb > 0 ? Math.min(100, (usedGb / limitGb) * 100) : null

  return (
    <>
      <AdminModal
        open={open}
        onClose={handleClose}
        title={t('admin.users.subscription.traffic')}
        description={t('admin.users.subscription.trafficModalHint')}
        icon={Gauge}
        iconAccent="blue"
        size="md"
        footer={
          <AdminModalSaveFooter
            onCancel={handleClose}
            onSave={() => void handleSave()}
            isPending={isPending}
            saveDisabled={!hasChanges}
            leading={
              <button
                type="button"
                onClick={() => setConfirmReset(true)}
                disabled={resetTraffic.isPending}
                className="inline-flex items-center gap-2 rounded-lg px-2.5 py-2 text-sm font-medium text-orange-700 hover:bg-orange-500/10 disabled:opacity-50 dark:text-orange-400"
              >
                <RotateCcw className="size-4 shrink-0" />
                {t('admin.users.resetTraffic')}
              </button>
            }
          />
        }
      >
        <div className="space-y-5">
          {error && (
            <p className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}

          {/*
            Сначала — что уже израсходовано. Лимит правят, глядя на расход, а
            раньше эту цифру приходилось помнить из карточки под окном.
          */}
          <div className={surface('raised', 'rounded-xl p-3')}>
            <div className="flex items-baseline justify-between gap-3 text-sm">
              <span className="text-muted-foreground">{t('admin.users.subscription.trafficUsedTitle')}</span>
              <span className="font-semibold tabular-nums">
                {limitGb > 0
                  ? t('admin.users.overview.tileTrafficValue', {
                      used: formatDecimals(usedGb, 1),
                      limit: formatDecimals(limitGb, 0),
                    })
                  : t('admin.users.overview.tileTrafficUnlimited', { used: formatDecimals(usedGb, 1) })}
              </span>
            </div>
            <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted">
              <div
                className={cn(
                  'h-full rounded-full transition-all',
                  percent == null
                    ? 'bg-primary/40'
                    : percent >= 95
                      ? 'bg-red-500'
                      : percent >= 80
                        ? 'bg-amber-500'
                        : 'bg-primary',
                )}
                style={{ width: percent == null ? '100%' : `${Math.max(2, percent)}%` }}
              />
            </div>
            {rw.last_traffic_reset_at && (
              <p className="mt-2 text-xs text-muted-foreground">
                {t('admin.users.subscription.lastTrafficReset')}:{' '}
                <span className="tabular-nums">
                  {formatAdminDateTime(rw.last_traffic_reset_at, dateLocale)}
                </span>
              </p>
            )}
          </div>

          <div>
            <p className="mb-2 text-sm font-medium">{t('admin.users.subscription.trafficLimitTitle')}</p>
            <div className="flex flex-wrap gap-2">
              {panel.traffic_presets_gb.map((gb) => {
                const bytes = gb * GB
                return (
                  <button
                    key={gb}
                    type="button"
                    onClick={() => {
                      setDraftBytes(bytes)
                      setCustomTrafficGB(String(gb))
                    }}
                    className={cn(chipClass, draftBytes === bytes && chipActiveClass)}
                  >
                    {t('admin.users.subscription.trafficPresetGb', { value: gb })}
                  </button>
                )
              })}
              <button
                type="button"
                onClick={() => {
                  setDraftBytes(0)
                  setCustomTrafficGB('0')
                }}
                className={cn(chipClass, 'inline-flex items-center gap-1.5', draftBytes === 0 && chipActiveClass)}
              >
                <InfinityIcon className="size-3.5" aria-hidden />
                {t('subscriptionPage.unlimited')}
              </button>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <input
                type="number"
                min={0}
                step={0.1}
                value={customTrafficGB}
                onChange={(e) => {
                  setCustomTrafficGB(e.target.value)
                  const parsed = parseFloat(e.target.value)
                  if (!Number.isNaN(parsed)) {
                    setDraftBytes(Math.round(parsed * GB))
                  }
                }}
                placeholder={t('admin.users.subscription.gbUnit')}
                className="admin-input w-28 px-3 py-2"
              />
              <span className="text-sm text-muted-foreground">
                {t('admin.users.subscription.gbUnitHint')}
              </span>
            </div>
          </div>

          <AdminUserTrafficSplit userId={userId} />

          <div>
            <p className="mb-2 text-sm font-medium">{t('admin.users.subscription.strategy')}</p>
            <div className="flex flex-wrap gap-2">
              {panel.strategies.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setDraftStrategy(s)}
                  className={cn(chipClass, draftStrategy === s && chipActiveClass)}
                >
                  {trafficStrategyLabel(s, t)}
                </button>
              ))}
            </div>
          </div>
        </div>
      </AdminModal>

      <AdminConfirmModal
        open={confirmReset}
        onClose={() => setConfirmReset(false)}
        onConfirm={handleResetTraffic}
        title={t('admin.users.resetTraffic')}
        message={t('admin.users.resetTrafficConfirm')}
        loading={resetTraffic.isPending}
        icon={RotateCcw}
        iconAccent="orange"
      />
    </>
  )
}
