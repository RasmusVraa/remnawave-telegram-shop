import { useEffect, useMemo, useState, type MutableRefObject, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, Check, Plus, Route, Star, Trash2 } from 'lucide-react'

import '@/lib/splitColor'
import { api, ApiError } from '@/lib/api'
import { bypassLabel } from '@/lib/bypassLabel'
import { cn } from '@/lib/utils'
import { AdminToggleSwitch } from './AdminToggleSwitch'
import { InfoTip } from './InfoTip'
import { LockedHint } from './LockedHint'
import { useAdminSquads } from '../hooks/useAdminTariffs'
import { useAdminBotSettings } from '../hooks/useAdminBotSettings'
import { rubPerStarFromSettings, starsFromRub } from '../utils/tariffStarsPricing'

const GB = 1024 ** 3

type PackKind = 'main' | 'bypass'
type PackRow = { kind: PackKind; gb: string; rub: string; stars: string; active: boolean }

const csv = (raw: string) =>
  raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)

function ruNumber(n: number): string {
  return (Math.round(n * 10) / 10).toLocaleString('ru-RU')
}

/** Числовое поле пакета с единицей измерения внутри справа: из подписи сразу видно, что вводить. */
function PackField({
  value,
  onChange,
  suffix,
  label,
  placeholder,
  hint,
}: {
  value: string
  onChange: (v: string) => void
  suffix: ReactNode
  label: string
  placeholder?: string
  /** Подсказка за значком «i» внутри поля. */
  hint?: string
}) {
  return (
    <div className="relative min-w-0">
      <input
        type="number"
        min={0}
        className={cn('admin-input w-full min-w-0 py-2 pl-3 text-sm tabular-nums', hint ? 'pr-14' : 'pr-10')}
        value={value}
        placeholder={placeholder}
        aria-label={label}
        onChange={(e) => onChange(e.target.value)}
      />
      <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center gap-1.5 text-xs text-muted-foreground">
        {hint && <InfoTip text={hint} label={hint} />}
        {suffix}
      </span>
    </div>
  )
}

/** Карточка секции вкладки: значок, заголовок, подзаголовок, справа действие. */
function SectionCard({
  icon,
  accent,
  title,
  subtitle,
  action,
  className,
  children,
}: {
  icon: ReactNode
  accent: string
  title: string
  subtitle?: string
  action?: ReactNode
  className?: string
  children?: ReactNode
}) {
  return (
    <section className={cn('rounded-xl border border-border/70 bg-card/40', className)}>
      <header className={cn('flex items-center gap-2.5 px-3.5 py-3', children && 'border-b border-border/60')}>
        <span className={cn('flex size-7 shrink-0 items-center justify-center rounded-lg', accent)}>{icon}</span>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold leading-tight">{title}</p>
          {subtitle && <p className="text-xs text-muted-foreground">{subtitle}</p>}
        </div>
        {action}
      </header>
      {children && <div className="space-y-3 p-3.5">{children}</div>}
    </section>
  )
}

/**
 * Лимит обхода и пакеты докупки тарифа (вкладка «Трафик»).
 *
 * Сохраняется своей кнопкой, отдельно от остальных полей тарифа, и касается только будущих
 * покупок и продлений: у тех, кто уже на тарифе, остаётся то, что было при их последней оплате.
 */
export function AdminTariffTrafficSplit({
  tariffId,
  tariffSquads,
  formLimitGb,
  onMakeUnlimited,
  onDirtyChange,
  onBypassChange,
  saveRef,
  onValidChange,
}: {
  tariffId: number
  tariffSquads: string[]
  /** Лимит обычных серверов, как он сейчас введён в форме тарифа (ГБ, 0 — безлимит). */
  formLimitGb: number
  onMakeUnlimited: () => void
  onDirtyChange?: (dirty: boolean) => void
  /** Включён ли сейчас отдельный лимит обхода (форма тарифа закрывает поле лимита обычных серверов). */
  onBypassChange?: (enabled: boolean) => void
  /**
   * Сюда кладётся функция сохранения вкладки (null — сохранять нечего). Её вызывает общая кнопка «Сохранить»
   * окна тарифа: у вкладки своей кнопки нет.
   */
  saveRef?: MutableRefObject<(() => Promise<unknown>) | null>
  /** Заполнены ли обязательные поля обхода (лимит, сквад, ноды). */
  onValidChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const { data: squadsData } = useAdminSquads()
  const { data: botSettings } = useAdminBotSettings()
  const rubPerStar = rubPerStarFromSettings(botSettings)
  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-tariff-traffic', tariffId],
    queryFn: () => api.adminTariffTraffic(tariffId),
  })
  const [enabled, setEnabled] = useState(false)
  const [limitGb, setLimitGb] = useState('30')
  const [squads, setSquads] = useState<string[]>([])
  // Ноды показываем только после выбора сквада и только те, что его раздают (а не все ноды панели).
  const squadsKey = [...squads].sort().join(',')
  const {
    data: nodes,
    isError: nodesError,
    isLoading: nodesLoading,
  } = useQuery({
    queryKey: ['admin-tariff-traffic-nodes', tariffId, squadsKey],
    queryFn: () => api.adminTariffTrafficNodes(tariffId, squads),
    enabled: enabled && squads.length > 0,
    staleTime: 30_000,
    retry: 0,
  })
  const [nodeIds, setNodeIds] = useState<string[]>([])
  const [packs, setPacks] = useState<PackRow[]>([])
  const [baseline, setBaseline] = useState('')

  const snapshot = (e: boolean, l: string, s: string[], n: string[], p: PackRow[]) => JSON.stringify([e, l, s, n, p])

  useEffect(() => {
    if (!data) return
    const orig = data.bypass_limit_bytes > 0
    // Старый тариф с обоими лимитами: оставляем лимит обычных серверов (его держит панель), обход выключаем.
    const e = orig && !(formLimitGb > 0)
    const l = e ? String(Math.round((data.bypass_limit_bytes / GB) * 100) / 100) : '30'
    const s = csv(data.bypass_squad_uuids)
    const n = csv(data.bypass_node_uuids)
    const p = data.packs.map((x) => ({
      kind: x.kind,
      gb: String(Math.round((x.bytes / GB) * 100) / 100),
      rub: String(x.amount_rub),
      stars: x.amount_stars != null ? String(x.amount_stars) : '',
      active: x.is_active,
    }))
    setEnabled(e)
    setLimitGb(l)
    setSquads(s)
    setNodeIds(n)
    setPacks(p)
    setBaseline(snapshot(orig, l, s, n, p))
    onBypassChange?.(e)
  }, [data])

  const label = bypassLabel(data?.bypass_label)
  const candidateSquads = useMemo(() => {
    const all = squadsData?.items ?? []
    return tariffSquads.length > 0 ? all.filter((s) => tariffSquads.includes(s.uuid)) : all
  }, [squadsData, tariffSquads])

  const save = useMutation({
    meta: { haptics: true },
    mutationFn: () => {
      const bytes = enabled ? Math.round(Number(limitGb) * GB) : 0
      return api.adminTariffTrafficSave(tariffId, {
        bypass_limit_bytes: bytes,
        bypass_squad_uuids: enabled ? squads.join(',') : '',
        bypass_node_uuids: enabled ? nodeIds.join(',') : '',
        split_enabled: false,
        bypass_label: '',
        packs: packs
          .filter((p) => Number(p.gb) > 0 && p.rub !== '')
          // Два режима: при обходе обычные серверы безлимитны (пакеты только обхода), иначе только пакеты обычного трафика.
          .filter((p) => (enabled ? p.kind === 'bypass' : p.kind === 'main'))
          .map((p) => ({
            kind: p.kind,
            bytes: Math.round(Number(p.gb) * GB),
            amount_rub: Math.round(Number(p.rub)),
            // Пусто — звёзды считаются из рублей по курсу автоматически; введённое руками сохраняется как есть.
            ...(p.stars !== '' ? { amount_stars: Math.round(Number(p.stars)) } : {}),
            is_active: p.active,
          })),
      })
    },
    onSuccess: (res) => {
      qc.setQueryData(['admin-tariff-traffic', tariffId], res)
      // Лимит обхода и докупка видны и на карточке тарифа в списке.
      qc.invalidateQueries({ queryKey: ['admin-tariffs'] })
    },
  })

  const limitOk = !enabled || (Number(limitGb) > 0 && nodeIds.length > 0 && squads.length > 0)
  const dirty = baseline !== '' && snapshot(enabled, limitGb, squads, nodeIds, packs) !== baseline
  useEffect(() => {
    if (!nodes) return
    const known = new Set(nodes.map((n) => n.uuid))
    setNodeIds((cur) => (cur.some((id) => !known.has(id)) ? cur.filter((id) => known.has(id)) : cur))
  }, [nodes])
  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => {
    onValidChange?.(limitOk)
  }, [limitOk, onValidChange])
  useEffect(() => {
    if (!saveRef) return
    saveRef.current = dirty ? () => save.mutateAsync() : null
    return () => {
      saveRef.current = null
    }
  })
  // Панель держит один общий лимит на клиента и не умеет отключать только обычные серверы, поэтому отдельный лимит
  // обхода возможен лишь при безлимитных обычных серверах.
  const formLimited = formLimitGb > 0
  const savedLimited = (data?.main_limit_bytes ?? 0) > 0
  const globalLimit = Boolean(data?.main_limit_global)
  const blocked = (formLimited || globalLimit) && !enabled
  const droppedPacks = !enabled && packs.some((p) => p.kind === 'bypass')
  const toggle = (list: string[], set: (v: string[]) => void, id: string) =>
    set(list.includes(id) ? list.filter((x) => x !== id) : [...list, id])
  const update = (i: number, patch: Partial<PackRow>) => setPacks((rows) => rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const noTracking = (nodes ?? []).filter((n) => nodeIds.includes(n.uuid) && !n.tracking_enabled)

  if (isLoading) return <div className="h-24 animate-pulse rounded-xl bg-muted/50" />
  if (isError || !data) return <p className="text-xs text-muted-foreground">{t('admin.tariffs.trafficSplit.loadError')}</p>

  const packColumn = (kind: PackKind) => {
    const rows = packs.map((p, i) => ({ p, i })).filter((x) => x.p.kind === kind)
    const first = rows[0]?.p
    const basePerGb = first && Number(first.gb) > 0 ? Number(first.rub) / Number(first.gb) : 0
    const caption = 'text-[11px] font-medium uppercase tracking-wide text-muted-foreground'
    return (
      <div className="space-y-2">
        {rows.length > 0 && (
          <div className="grid grid-cols-3 gap-2 px-2.5">
            <span className={caption}>{t('admin.tariffs.trafficSplit.colSize')}</span>
            <span className={caption}>{t('admin.tariffs.trafficSplit.colRub')}</span>
            <span className={cn(caption, 'normal-case')}>
              {t('admin.tariffs.trafficSplit.colStars')}
            </span>
          </div>
        )}
        {rows.map(({ p, i }, idx) => {
          const perGb = Number(p.gb) > 0 && p.rub !== '' ? Number(p.rub) / Number(p.gb) : 0
          const saving = idx > 0 && basePerGb > 0 && perGb > 0 ? Math.round((1 - perGb / basePerGb) * 100) : 0
          const auto = starsFromRub(Number(p.rub), rubPerStar)
          return (
            <div key={i} className={cn('rounded-xl border border-border/60 bg-background/30 p-2.5', !p.active && 'opacity-70')}>
              <div className="grid grid-cols-3 gap-2">
                <PackField value={p.gb} suffix={t('dashboard.gigabytes')} label={t('admin.tariffs.trafficSplit.packSize')} onChange={(v) => update(i, { gb: v })} />
                <PackField value={p.rub} suffix="₽" label={t('admin.tariffs.trafficSplit.packPriceRub')} onChange={(v) => update(i, { rub: v })} />
                <PackField
                  value={p.stars}
                  suffix={<Star className="size-3.5 fill-amber-400 text-amber-400" aria-hidden />}
                  placeholder={auto != null ? String(auto) : ''}
                  hint={t('admin.tariffs.trafficSplit.starsAutoHint')}
                  label={t('admin.tariffs.trafficSplit.packPriceStars')}
                  onChange={(v) => update(i, { stars: v })}
                />
              </div>
              <div className="mt-2 flex items-center gap-3">
                <p className="min-w-0 flex-1 text-xs text-muted-foreground">
                  {perGb > 0 && (
                    <>
                      {ruNumber(perGb)} {t('admin.tariffs.trafficSplit.rubPerGb')}
                      {saving > 0 && <span className="text-emerald-500"> −{saving}%</span>}
                    </>
                  )}
                </p>
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                  {t('admin.tariffs.trafficSplit.packActive')}
                  <AdminToggleSwitch checked={p.active} onChange={(v) => update(i, { active: v })} aria-label={t('admin.tariffs.trafficSplit.packActive')} />
                </label>
                <button
                  type="button"
                  onClick={() => setPacks((r) => r.filter((_, j) => j !== i))}
                  className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-destructive"
                  aria-label={t('admin.delete')}
                >
                  <Trash2 className="size-4" />
                </button>
              </div>
            </div>
          )
        })}
        <button
          type="button"
          onClick={() => setPacks((r) => [...r, { kind, gb: '', rub: '', stars: '', active: true }])}
          className="inline-flex items-center gap-1.5 rounded-lg border border-dashed border-border px-3 py-1.5 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <Plus className="size-4" />
          {t('admin.tariffs.trafficSplit.addPackShort')}
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {!data.split_enabled && (
        <p className="flex items-start gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" aria-hidden />
          {t('admin.tariffs.trafficSplit.masterOff')}
        </p>
      )}

      <SectionCard
        icon={<Route className="size-4" />}
        accent="bg-[hsl(var(--split-bypass)/0.16)] text-[hsl(var(--split-bypass))]"
        title={t('admin.tariffs.trafficSplit.bypassTitle', { label })}
        subtitle={t('admin.tariffs.trafficSplit.bypassSub')}
        className={enabled ? 'border-[hsl(var(--split-bypass)/0.45)]' : undefined}
        action={
          <LockedHint
            locked={blocked}
            className="shrink-0"
            align="right"
            message={globalLimit && !formLimited ? t('admin.tariffs.trafficSplit.lockedGlobal') : t('admin.tariffs.trafficSplit.lockedLimited', { gb: ruNumber(formLimitGb) })}
            action={
              formLimited ? (
                <button type="button" onClick={onMakeUnlimited} className="rounded-md border border-border bg-background px-2.5 py-1 font-medium hover:bg-muted">
                  {t('admin.tariffs.trafficSplit.makeUnlimited')}
                </button>
              ) : undefined
            }
          >
            <AdminToggleSwitch
              checked={enabled}
              disabled={blocked}
              onChange={(v) => {
                setEnabled(v)
                onBypassChange?.(v)
              }}
              aria-label={t('admin.tariffs.trafficSplit.enable', { label })}
            />
          </LockedHint>
        }
      >
        {enabled && !formLimited && savedLimited && (
          <p className="flex items-start gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs">
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" aria-hidden />
            {t('admin.tariffs.trafficSplit.saveTariffFirst')}
          </p>
        )}
        {enabled && (
          <>
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1 block text-xs font-medium">{t('admin.tariffs.trafficSplit.limitPerMonth')}</label>
                <div className="flex items-center gap-2">
                  <input type="number" min={0} step={1} className="admin-input w-full px-3 py-2" value={limitGb} onChange={(e) => setLimitGb(e.target.value)} />
                  <span className="text-sm text-muted-foreground">{t('dashboard.gigabytes')}</span>
                </div>
                <p className="mt-1 text-[11.5px] text-muted-foreground">{t('admin.tariffs.trafficSplit.limitHint2')}</p>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium">{t('admin.tariffs.trafficSplit.squads')}</label>
                <div className="flex flex-wrap gap-1.5">
                  {candidateSquads.map((s) => {
                    const on = squads.includes(s.uuid)
                    return (
                      <button
                        key={s.uuid}
                        type="button"
                        onClick={() => toggle(squads, setSquads, s.uuid)}
                        className={cn(
                          'inline-flex items-center gap-1 rounded-full border px-3 py-1 text-xs transition-colors',
                          on ? 'border-[hsl(var(--split-bypass))] bg-[hsl(var(--split-bypass)/0.14)] text-foreground' : 'border-border text-muted-foreground hover:bg-muted',
                        )}
                      >
                        {on && <Check className="size-3" />}
                        {s.name}
                      </button>
                    )
                  })}
                </div>
                <p className="mt-1 text-[11.5px] text-muted-foreground">{t('admin.tariffs.trafficSplit.squadsHint2')}</p>
              </div>
            </div>

            <div>
              <label className="mb-1.5 block text-xs font-medium">{t('admin.tariffs.trafficSplit.nodesTitle')}</label>
              {squads.length === 0 ? (
                <p className="text-xs text-muted-foreground">{t('admin.tariffs.trafficSplit.nodesPickSquad')}</p>
              ) : nodesError ? (
                <p className="text-xs text-muted-foreground">{t('admin.tariffs.trafficSplit.nodesError')}</p>
              ) : nodesLoading ? (
                <div className="h-9 w-48 animate-pulse rounded-lg bg-muted/50" />
              ) : (nodes ?? []).length === 0 ? (
                <p className="text-xs text-muted-foreground">{t('admin.tariffs.trafficSplit.nodesEmpty')}</p>
              ) : (
                <div className="flex flex-wrap gap-2">
                  {(nodes ?? []).map((n) => {
                    const on = nodeIds.includes(n.uuid)
                    return (
                      <button
                        key={n.uuid}
                        type="button"
                        onClick={() => toggle(nodeIds, setNodeIds, n.uuid)}
                        className={cn(
                          'flex items-center gap-2 rounded-lg border px-2.5 py-1.5 text-sm transition-colors',
                          on ? 'border-[hsl(var(--split-bypass))] bg-[hsl(var(--split-bypass)/0.1)]' : 'border-border bg-background/40 hover:bg-muted',
                        )}
                      >
                        {on ? <Check className="size-3.5 text-[hsl(var(--split-bypass))]" /> : <span className="size-3.5 rounded border border-border" />}
                        {n.name}
                        <i className={cn('size-1.5 rounded-full', n.tracking_enabled ? 'bg-emerald-500' : 'bg-amber-500')} />
                        <span className={cn('text-[11px]', n.tracking_enabled ? 'text-emerald-500' : 'text-amber-500')}>
                          {n.tracking_enabled ? t('admin.tariffs.trafficSplit.trackingOn') : t('admin.tariffs.trafficSplit.trackingOff')}
                        </span>
                      </button>
                    )
                  })}
                </div>
              )}
            </div>

            {noTracking.length > 0 && (
              <p className="flex items-start gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" aria-hidden />
                {t('admin.tariffs.trafficSplit.trackingWarn2', { nodes: noTracking.map((n) => n.name).join(', ') })}
              </p>
            )}
          </>
        )}
      </SectionCard>

      <SectionCard
        icon={<Plus className="size-4" />}
        accent="bg-emerald-500/15 text-emerald-500"
        title={t('admin.tariffs.trafficSplit.packs')}
        subtitle={t('admin.tariffs.trafficSplit.packsSub')}
      >
        {droppedPacks && (
          <p className="flex items-start gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs">
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" aria-hidden />
            {t('admin.tariffs.trafficSplit.bypassPacksDropped', { label })}
          </p>
        )}
        {enabled ? (
          packColumn('bypass')
        ) : formLimited ? (
          packColumn('main')
        ) : (
          <p className="rounded-lg border border-dashed border-border/70 px-3 py-2 text-xs text-muted-foreground">{t('admin.tariffs.trafficSplit.packsUnlimited')}</p>
        )}
      </SectionCard>

      {enabled && !limitOk && (
        <p className="flex items-start gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" aria-hidden />
          {t('admin.tariffs.trafficSplit.needFields')}
        </p>
      )}
      {save.isError && (
        <p role="alert" className="rounded-lg border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-xs text-rose-500">
          {save.error instanceof ApiError && save.error.status === 400 ? save.error.body.trim() : t('admin.tariffs.trafficSplit.saveError')}
        </p>
      )}
    </div>
  )
}
