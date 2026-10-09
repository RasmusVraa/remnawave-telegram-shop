import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronUp, Loader2, Plus, RefreshCw, Trash2 } from 'lucide-react'

import { api } from '@/lib/api'
import { AdminFeedback, type AdminFeedbackState } from './AdminFeedback'
import { formatAdminApiError } from '../utils/formatAdminApiError'

export interface StatusTargetDraft {
  id: string
  name: string
  country: string
  address: string
  enabled: boolean
  interval_min: number
  world_probes: number
  russia_probes: number
  note: string
  whitelist: boolean
}

function emptyTarget(): StatusTargetDraft {
  return {
    id: '',
    name: '',
    country: '',
    address: '',
    enabled: true,
    interval_min: 0,
    world_probes: 0,
    russia_probes: 0,
    note: '',
    whitelist: false,
  }
}

const fieldClass =
  'h-10 w-full rounded-lg border border-border/60 bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring'

/** Список стран, которые страница статуса проверяет зондами. */
export function AdminStatusTargets() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [rows, setRows] = useState<StatusTargetDraft[]>([])
  const [ready, setReady] = useState(false)
  const [feedback, setFeedback] = useState<AdminFeedbackState | null>(null)

  const query = useQuery({
    queryKey: ['admin-status-targets'],
    queryFn: () => api.adminStatusTargets(),
  })

  useEffect(() => {
    if (!query.data || ready) return
    setRows(
      (query.data.targets ?? []).map((item) => ({
        ...item,
        note: item.note ?? '',
        whitelist: Boolean(item.whitelist),
      })),
    )
    setReady(true)
  }, [query.data, ready])

  const probe = useMutation({
    mutationFn: () => api.adminStatusProbe(),
    onSuccess: () => {
      setFeedback({ type: 'success', message: t('admin.statusPage.forceProbeStarted') })
    },
    onError: (err) => {
      setFeedback({ type: 'error', message: formatAdminApiError(err, t) })
    },
  })

  const save = useMutation({
    mutationFn: () => api.saveAdminStatusTargets(rows),
    onSuccess: async (data) => {
      setFeedback({ type: 'success', message: t('admin.statusPage.targetsSaved') })
      await queryClient.invalidateQueries({ queryKey: ['admin-status-targets'] })
      if (Array.isArray((data as { targets?: StatusTargetDraft[] }).targets)) {
        setRows((data as { targets: StatusTargetDraft[] }).targets)
      }
    },
    onError: (err) => {
      setFeedback({ type: 'error', message: formatAdminApiError(err, t) })
    },
  })

  function patch(index: number, next: Partial<StatusTargetDraft>) {
    setRows((prev) => prev.map((row, i) => (i === index ? { ...row, ...next } : row)))
  }

  function move(index: number, dir: -1 | 1) {
    setRows((prev) => {
      const next = index + dir
      if (next < 0 || next >= prev.length) return prev
      const copy = [...prev]
      const [row] = copy.splice(index, 1)
      copy.splice(next, 0, row)
      return copy
    })
  }

  return (
    <section className="space-y-3 rounded-xl border border-border/60 bg-card/40 p-4 sm:p-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-base font-semibold tracking-tight">{t('admin.statusPage.targetsTitle')}</h2>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">{t('admin.statusPage.targetsHint')}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            disabled={probe.isPending}
            onClick={() => probe.mutate()}
            className="inline-flex min-h-9 items-center justify-center gap-1.5 rounded-lg border border-border px-3 text-sm font-medium hover:bg-accent disabled:opacity-60"
          >
            {probe.isPending ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            {t('admin.statusPage.forceProbe')}
          </button>
          <button
            type="button"
            onClick={() => setRows((prev) => [...prev, emptyTarget()])}
            className="inline-flex min-h-9 items-center justify-center gap-1.5 rounded-lg border border-border px-3 text-sm font-medium hover:bg-accent"
          >
            <Plus className="size-4" />
            {t('admin.statusPage.addTarget')}
          </button>
        </div>
      </div>

      <AdminFeedback feedback={feedback} onDismiss={() => setFeedback(null)} autoDismissMs={4000} />

      {query.isLoading ? (
        <div className="flex justify-center py-8 text-muted-foreground">
          <Loader2 className="size-5 animate-spin" />
        </div>
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('admin.statusPage.targetsEmpty')}</p>
      ) : (
        <ul className="space-y-3">
          {rows.map((row, index) => (
            <li key={row.id || `new-${index}`} className="space-y-3 rounded-lg border border-border/50 p-3">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-[1.4fr_5rem_1fr_auto]">
                <label className="block text-xs font-medium text-muted-foreground">
                  {t('admin.statusPage.fieldName')}
                  <input
                    className={`${fieldClass} mt-1`}
                    value={row.name}
                    maxLength={80}
                    onChange={(e) => patch(index, { name: e.target.value })}
                  />
                </label>
                <label className="block text-xs font-medium text-muted-foreground">
                  {t('admin.statusPage.fieldCountry')}
                  <input
                    className={`${fieldClass} mt-1 uppercase`}
                    value={row.country}
                    maxLength={2}
                    onChange={(e) => patch(index, { country: e.target.value.toUpperCase() })}
                  />
                </label>
                <label className="block text-xs font-medium text-muted-foreground">
                  {t('admin.statusPage.fieldAddress')}
                  <input
                    className={`${fieldClass} mt-1`}
                    value={row.address}
                    inputMode="text"
                    autoComplete="off"
                    spellCheck={false}
                    onChange={(e) => patch(index, { address: e.target.value })}
                  />
                </label>
                <div className="flex items-end justify-between gap-2 sm:justify-end">
                  <label className="inline-flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={row.enabled}
                      onChange={(e) => patch(index, { enabled: e.target.checked })}
                    />
                    {t('admin.statusPage.fieldEnabled')}
                  </label>
                  <label className="inline-flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={row.whitelist}
                      onChange={(e) => patch(index, { whitelist: e.target.checked })}
                    />
                    {t('admin.statusPage.fieldWhitelist')}
                  </label>
                  <div className="flex items-center">
                    <button
                      type="button"
                      aria-label={t('admin.statusPage.moveUp')}
                      disabled={index === 0}
                      onClick={() => move(index, -1)}
                      className="inline-flex size-10 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted disabled:opacity-30"
                    >
                      <ChevronUp className="size-4" />
                    </button>
                    <button
                      type="button"
                      aria-label={t('admin.statusPage.moveDown')}
                      disabled={index === rows.length - 1}
                      onClick={() => move(index, 1)}
                      className="inline-flex size-10 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted disabled:opacity-30"
                    >
                      <ChevronDown className="size-4" />
                    </button>
                    <button
                      type="button"
                      aria-label={t('admin.statusPage.removeTarget')}
                      onClick={() => setRows((prev) => prev.filter((_, i) => i !== index))}
                      className="inline-flex size-10 items-center justify-center rounded-lg text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                    >
                      <Trash2 className="size-4" />
                    </button>
                  </div>
                </div>
              </div>
              <label className="block text-xs font-medium text-muted-foreground">
                {t('admin.statusPage.fieldNote')}
                <input
                  className={`${fieldClass} mt-1`}
                  value={row.note || ''}
                  maxLength={48}
                  placeholder={t('admin.statusPage.fieldNotePlaceholder')}
                  onChange={(e) => patch(index, { note: e.target.value })}
                />
              </label>
              <div className="grid gap-3 sm:grid-cols-3">
                <label className="block text-xs font-medium text-muted-foreground">
                  {t('admin.statusPage.fieldInterval')}
                  <input
                    className={`${fieldClass} mt-1`}
                    type="number"
                    min={5}
                    max={180}
                    placeholder={t('admin.statusPage.fieldIntervalPlaceholder')}
                    value={row.interval_min || ''}
                    onChange={(e) => patch(index, { interval_min: Number(e.target.value) || 0 })}
                  />
                </label>
                <label className="block text-xs font-medium text-muted-foreground">
                  {t('admin.statusPage.fieldWorld')}
                  <input
                    className={`${fieldClass} mt-1`}
                    type="number"
                    min={0}
                    max={10}
                    placeholder={t('admin.statusPage.fieldCountPlaceholder')}
                    value={row.world_probes || ''}
                    onChange={(e) => patch(index, { world_probes: Number(e.target.value) || 0 })}
                  />
                </label>
                <label className="block text-xs font-medium text-muted-foreground">
                  {t('admin.statusPage.fieldRussia')}
                  <input
                    className={`${fieldClass} mt-1`}
                    type="number"
                    min={0}
                    max={30}
                    placeholder={t('admin.statusPage.fieldCountPlaceholder')}
                    value={row.russia_probes || ''}
                    onChange={(e) => patch(index, { russia_probes: Number(e.target.value) || 0 })}
                  />
                </label>
              </div>
            </li>
          ))}
        </ul>
      )}

      <div className="flex justify-end">
        <button
          type="button"
          disabled={save.isPending}
          onClick={() => save.mutate()}
          className="inline-flex min-h-10 items-center justify-center rounded-lg bg-primary px-4 text-sm font-semibold text-primary-foreground disabled:opacity-60"
        >
          {save.isPending ? <Loader2 className="size-4 animate-spin" /> : t('admin.save')}
        </button>
      </div>
    </section>
  )
}
