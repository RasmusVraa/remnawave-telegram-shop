import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ChevronDown,
  GripVertical,
  Download,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Server,
  Trash2,
} from 'lucide-react'

import { api, type AdminRowChecksDTO, type AdminStatusNodeDTO, type AdminStatusTargetDTO } from '@/lib/api'
import { cn } from '@/lib/utils'
import { AdminFeedback, type AdminFeedbackState } from './AdminFeedback'
import { AdminModal } from './AdminModal'
import { AdminToggleSwitch } from './AdminToggleSwitch'
import { AdminStatusRowChecks } from './AdminStatusRowChecks'
import { CountryFlag } from './CountryFlag'
import { CountrySelect } from './CountrySelect'
import { AdminSelect } from './AdminSelect'
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
  /** regular — обычная нода, bypass — каскад обхода (проверка операторов с белыми списками). */
  kind: 'regular' | 'bypass'
  whitelist: boolean
  repeat_cities: boolean
  node_uuid: string
  exit_node_uuid: string
  no_external: boolean
  tcp_port: number
}

/** Самое свежее время замера по всем серверам, мс. Нет замеров — null. */
function newestMeasuredAt(nodes: { probe?: { measured_at?: string } }[] | undefined): number | null {
  let best: number | null = null
  for (const node of nodes ?? []) {
    const ts = node.probe?.measured_at ? Date.parse(node.probe.measured_at) : NaN
    if (!Number.isNaN(ts) && (best == null || ts > best)) best = ts
  }
  return best
}

function newDraftId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID().replace(/-/g, '').slice(0, 16)
  }
  return `row-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
}

function emptyTarget(): StatusTargetDraft {
  return {
    id: newDraftId(),
    name: '',
    country: '',
    address: '',
    enabled: true,
    interval_min: 0,
    world_probes: 0,
    russia_probes: 0,
    note: '',
    kind: 'regular',
    whitelist: false,
    repeat_cities: true,
    node_uuid: '',
    exit_node_uuid: '',
    no_external: false,
    tcp_port: 0,
  }
}

function fromDTO(item: Partial<AdminStatusTargetDTO> & { id: string }): StatusTargetDraft {
  const bypass = item.kind === 'bypass' || (!item.kind && Boolean(item.whitelist))
  return {
    ...emptyTarget(),
    ...item,
    note: item.note ?? '',
    kind: bypass ? 'bypass' : 'regular',
    whitelist: bypass,
    repeat_cities: item.repeat_cities !== false,
    node_uuid: item.node_uuid ?? '',
    exit_node_uuid: item.exit_node_uuid ?? '',
    no_external: Boolean(item.no_external),
    tcp_port: item.tcp_port ?? 0,
  }
}

function toDTO(row: StatusTargetDraft): AdminStatusTargetDTO {
  return { ...row, whitelist: row.kind === 'bypass' }
}

const inputClass =
  'h-10 w-full rounded-lg border border-border/60 bg-background px-3 text-sm outline-none transition-colors placeholder:text-muted-foreground/60 focus-visible:ring-2 focus-visible:ring-ring'

const STATE_DOT: Record<string, string> = {
  up: 'bg-emerald-500',
  partial: 'bg-amber-500',
  down: 'bg-rose-500',
}

/** Список серверов страницы статуса: строки, окно правки и панель сохранения. */
export function AdminStatusTargets() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [rows, setRows] = useState<StatusTargetDraft[]>([])
  const [saved, setSaved] = useState('[]')
  const [ready, setReady] = useState(false)
  const [feedback, setFeedback] = useState<AdminFeedbackState | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const listRef = useRef<HTMLUListElement>(null)
  const [dragId, setDragId] = useState<string | null>(null)
  const rowRefs = useRef(new Map<string, HTMLLIElement>())
  // Где строка стояла в прошлый раз (offsetTop не зависит от transform): по разнице играем анимацию переезда.
  const prevTops = useRef(new Map<string, number>())
  const pointerY = useRef(0)
  // Насколько ниже верхнего края строки пользователь взялся за ручку.
  const grabOffset = useRef(0)

  const query = useQuery({
    queryKey: ['admin-status-targets'],
    queryFn: () => api.adminStatusTargets(),
    refetchInterval: 60_000,
  })
  const nodesQuery = useQuery({
    queryKey: ['admin-status-nodes'],
    queryFn: () => api.adminStatusNodes(),
    retry: false,
    staleTime: 60_000,
  })
  const nodes = useMemo(() => nodesQuery.data?.nodes ?? [], [nodesQuery.data])
  // «Обновлено N мин назад»: берём тот же публичный ответ, что видят пользователи.
  const publicQuery = useQuery({
    queryKey: ['public-status'],
    queryFn: () => api.publicStatus(),
    refetchInterval: 60_000,
    staleTime: 30_000,
  })
  const measuredAt = newestMeasuredAt(publicQuery.data?.nodes)
  const minutesAgo = measuredAt ? Math.max(0, Math.round((Date.now() - measuredAt) / 60000)) : null

  useEffect(() => {
    if (!query.data || ready) return
    const next = (query.data.targets ?? []).map(fromDTO)
    setRows(next)
    setSaved(JSON.stringify(next))
    setReady(true)
  }, [query.data, ready])

  const dirty = ready && JSON.stringify(rows) !== saved

  const probe = useMutation({
    mutationFn: () => api.adminStatusProbe(),
    onSuccess: () => setFeedback({ type: 'success', message: t('admin.statusPage.checkStarted') }),
    onError: (err) => setFeedback({ type: 'error', message: formatAdminApiError(err, t) }),
  })

  const save = useMutation({
    mutationFn: () => api.saveAdminStatusTargets(rows.map(toDTO)),
    onSuccess: async (data) => {
      const next = (data.targets ?? []).map(fromDTO)
      setRows(next)
      setSaved(JSON.stringify(next))
      setFeedback({ type: 'success', message: t('admin.statusPage.saved') })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['admin-status-targets'] }),
        queryClient.invalidateQueries({ queryKey: ['admin-status-quota'] }),
        queryClient.invalidateQueries({ queryKey: ['public-status'] }),
      ])
    },
    onError: (err) => setFeedback({ type: 'error', message: formatAdminApiError(err, t) }),
  })

  function patch(id: string, next: Partial<StatusTargetDraft>) {
    setRows((prev) => prev.map((row) => (row.id === id ? { ...row, ...next } : row)))
  }

  function move(index: number, dir: -1 | 1) {
    setRows((prev) => {
      const to = index + dir
      if (to < 0 || to >= prev.length) return prev
      const copy = [...prev]
      const [row] = copy.splice(index, 1)
      copy.splice(to, 0, row)
      return copy
    })
  }

  // Перетаскивание за ручку из шести точек. Строка поднимается (тень, чуть крупнее) и идёт за
  // курсором, а соседи плавно расступаются: их переезд проигрывается в useLayoutEffect ниже.
  function placeDragged() {
    const el = dragId ? rowRefs.current.get(dragId) : undefined
    const ul = listRef.current
    if (!el || !ul) return
    const layoutTop = ul.getBoundingClientRect().top + el.offsetTop
    const dy = pointerY.current - grabOffset.current - layoutTop
    el.style.transition = 'none'
    el.style.transform = `translateY(${dy}px) scale(1.02)`
  }

  function dragMove(clientY: number) {
    if (!dragId || !listRef.current) return
    pointerY.current = clientY
    placeDragged()
    const dragged = rowRefs.current.get(dragId)
    const from = rows.findIndex((row) => row.id === dragId)
    if (!dragged || from < 0) return
    const ulTop = listRef.current.getBoundingClientRect().top
    const draggedCenter = clientY - grabOffset.current + dragged.offsetHeight / 2
    const to = rows.filter((row) => {
      if (row.id === dragId) return false
      const el = rowRefs.current.get(row.id)
      return el ? ulTop + el.offsetTop + el.offsetHeight / 2 < draggedCenter : false
    }).length
    if (to === from) return
    setRows((prev) => {
      const i = prev.findIndex((row) => row.id === dragId)
      if (i < 0) return prev
      const copy = [...prev]
      const [row] = copy.splice(i, 1)
      copy.splice(to, 0, row)
      return copy
    })
  }

  function endDrag() {
    if (!dragId) return
    // Строка плавно садится на своё место.
    const el = rowRefs.current.get(dragId)
    if (el) {
      el.style.transition = 'transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1), box-shadow 200ms, border-color 200ms'
      el.style.transform = ''
    }
    setDragId(null)
  }

  // Анимация переезда: после смены порядка каждая строка «отъезжает» от старого места к новому.
  useLayoutEffect(() => {
    const reduce = typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
    rowRefs.current.forEach((el, id) => {
      const top = el.offsetTop
      const prev = prevTops.current.get(id)
      if (!reduce && prev != null && prev !== top && id !== dragId) {
        el.style.transition = 'none'
        el.style.transform = `translateY(${prev - top}px)`
        void el.getBoundingClientRect()
        el.style.transition = 'transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1)'
        el.style.transform = ''
      }
      prevTops.current.set(id, top)
    })
    placeDragged()
  }, [rows, dragId])

  // Курсор «схватил» на всём экране, пока тянем.
  useEffect(() => {
    if (!dragId) return
    const prev = document.body.style.cursor
    document.body.style.cursor = 'grabbing'
    return () => {
      document.body.style.cursor = prev
    }
  }, [dragId])

  function addRow(base: Partial<StatusTargetDraft>) {
    const row = { ...emptyTarget(), ...base }
    setRows((prev) => [...prev, row])
    setAdding(false)
    setEditingId(row.id)
  }

  function remove(id: string) {
    setRows((prev) => prev.filter((row) => row.id !== id))
    setEditingId(null)
  }

  const editing = rows.find((row) => row.id === editingId) ?? null

  return (
    <section className="rounded-2xl border border-border/60 bg-card/50 p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3 sm:flex-nowrap">
        <h2 className="min-w-0 text-base font-semibold tracking-tight">{t('admin.statusPage.serversTitle')}</h2>
        <div className="flex w-full items-center gap-2 sm:w-auto sm:shrink-0 sm:justify-end">
          {rows.length > 0 && (
            <span className="mr-auto min-w-0 text-xs text-muted-foreground sm:mr-1">
              {minutesAgo == null
                ? t('admin.statusPage.neverMeasured')
                : minutesAgo < 1
                  ? t('landing.status.updatedJust')
                  : t('landing.status.updatedMin', { count: minutesAgo })}
            </span>
          )}
          <button
            type="button"
            disabled={probe.isPending || rows.length === 0 || dirty}
            title={dirty ? t('admin.statusPage.saveFirst') : t('admin.statusPage.checkNow')}
            aria-label={t('admin.statusPage.checkNow')}
            onClick={() => probe.mutate()}
            className="inline-flex min-h-10 shrink-0 items-center gap-2 rounded-lg border border-border bg-card px-3 text-sm font-medium hover:bg-muted disabled:opacity-50 sm:px-3.5"
          >
            {probe.isPending ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            <span className="max-sm:hidden">{t('admin.statusPage.checkNow')}</span>
          </button>
          <button
            type="button"
            onClick={() => setAdding(true)}
            className="inline-flex min-h-10 shrink-0 items-center gap-2 rounded-lg bg-primary px-3.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90"
          >
            <Plus className="size-4" />
            <span className="sm:hidden">{t('admin.statusPage.addShort')}</span>
            <span className="max-sm:hidden">{t('admin.statusPage.add')}</span>
          </button>
        </div>
      </div>

      <AdminFeedback feedback={feedback} onDismiss={() => setFeedback(null)} autoDismissMs={4000} />

      <div className="mt-4">
        {query.isLoading ? (
          <div className="flex justify-center py-10 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : rows.length === 0 ? (
          <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border/70 px-4 py-10 text-center">
            <div className="flex size-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
              <Server className="size-6" />
            </div>
            <div>
              <p className="font-medium">{t('admin.statusPage.emptyTitle')}</p>
              <p className="mt-1 max-w-sm text-sm text-muted-foreground">{t('admin.statusPage.emptyText')}</p>
            </div>
            <button
              type="button"
              onClick={() => setAdding(true)}
              className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-primary-foreground hover:bg-primary/90"
            >
              <Plus className="size-4" />
              {t('admin.statusPage.add')}
            </button>
          </div>
        ) : (
          <ul
            ref={listRef}
            className={cn('relative space-y-2', dragId && 'select-none')}
            onPointerMove={(e) => dragMove(e.clientY)}
            onPointerUp={endDrag}
            onPointerCancel={endDrag}
          >
            {rows.map((row, index) => (
              <TargetRow
                key={row.id}
                row={row}
                node={nodes.find((n) => n.uuid === row.node_uuid)}
                checks={query.data?.checks?.[row.id]}
                dragging={dragId === row.id}
                liRef={(el) => {
                  if (el) rowRefs.current.set(row.id, el)
                  else rowRefs.current.delete(row.id)
                }}
                onToggle={(enabled) => patch(row.id, { enabled })}
                onEdit={() => setEditingId(row.id)}
                onHandleDown={(e) => {
                  e.preventDefault()
                  e.currentTarget.setPointerCapture(e.pointerId)
                  const el = rowRefs.current.get(row.id)
                  pointerY.current = e.clientY
                  grabOffset.current = el ? e.clientY - el.getBoundingClientRect().top : 0
                  setDragId(row.id)
                }}
                onHandleKey={(dir) => move(index, dir)}
              />
            ))}
          </ul>
        )}
      </div>

      {dirty && (
        <div className="sticky bottom-3 z-20 mt-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-primary/40 bg-card px-4 py-3 shadow-lg">
          <span className="text-sm font-medium">{t('admin.statusPage.unsaved')}</span>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={save.isPending}
              onClick={() => setRows(JSON.parse(saved) as StatusTargetDraft[])}
              className="inline-flex min-h-9 items-center rounded-lg border border-border px-3 text-sm font-medium hover:bg-muted disabled:opacity-50"
            >
              {t('admin.statusPage.discard')}
            </button>
            <button
              type="button"
              disabled={save.isPending}
              onClick={() => save.mutate()}
              className="inline-flex min-h-9 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 active:bg-primary/75 disabled:opacity-60"
            >
              {save.isPending && <Loader2 className="size-4 animate-spin" />}
              {t('admin.statusPage.save')}
            </button>
          </div>
        </div>
      )}

      <AddServerModal
        open={adding}
        onClose={() => setAdding(false)}
        nodes={nodes}
        loading={nodesQuery.isLoading}
        failed={nodesQuery.isError}
        usedIds={new Set(rows.map((row) => row.node_uuid).filter(Boolean))}
        onPick={(node) =>
          addRow({
            name: node.name,
            country: (node.country ?? '').toUpperCase(),
            address: node.address,
            node_uuid: node.uuid,
          })
        }
        onManual={() => addRow({})}
      />

      <EditServerModal
        row={editing}
        nodes={nodes}
        onClose={() => setEditingId(null)}
        onPatch={(next) => editing && patch(editing.id, next)}
        onRemove={() => editing && remove(editing.id)}
      />
    </section>
  )
}

function TargetRow({
  row,
  node,
  checks,
  dragging,
  liRef,
  onToggle,
  onEdit,
  onHandleDown,
  onHandleKey,
}: {
  row: StatusTargetDraft
  node?: AdminStatusNodeDTO
  checks?: AdminRowChecksDTO
  dragging: boolean
  liRef: (el: HTMLLIElement | null) => void
  onToggle: (enabled: boolean) => void
  onEdit: () => void
  onHandleDown: (e: React.PointerEvent<HTMLButtonElement>) => void
  onHandleKey: (dir: -1 | 1) => void
}) {
  const { t } = useTranslation()
  const bypass = row.kind === 'bypass'
  const kindLabel = bypass ? t('admin.statusPage.kindBypass') : t('admin.statusPage.kindRegular')
  const kindClass = cn(
    'shrink-0 rounded-md px-1.5 py-0.5 text-[11px] font-semibold',
    bypass ? 'bg-violet-500/15 text-violet-600 dark:text-violet-300' : 'bg-primary/10 text-primary',
  )
  return (
    <li
      ref={liRef}
      onClick={(e) => {
        // Пустое место карточки открывает правку; кнопки, переключатель и значки работают сами.
        if ((e.target as HTMLElement).closest('button,input,label,[role="tooltip"],[role="switch"]')) return
        onEdit()
      }}
      className={cn(
        'flex cursor-pointer items-center gap-2 max-sm:grid max-sm:grid-cols-[auto_auto_minmax(0,1fr)_auto] max-sm:gap-y-2 rounded-xl border border-border/60 bg-background/40 p-3 transition-[opacity,box-shadow,border-color,background-color] duration-200',
        dragging && 'relative z-20 border-primary/60 bg-card opacity-100 shadow-2xl shadow-black/40',
      )}
    >
      <button
        type="button"
        aria-label={t('admin.statusPage.drag')}
        title={t('admin.statusPage.drag')}
        onPointerDown={onHandleDown}
        onKeyDown={(e) => {
          if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
            e.preventDefault()
            onHandleKey(e.key === 'ArrowUp' ? -1 : 1)
          }
        }}
        className={cn(
          !row.enabled && 'opacity-60',
          '-ml-1 flex h-9 w-6 shrink-0 touch-none max-sm:col-start-1 max-sm:row-start-1 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          dragging ? 'cursor-grabbing' : 'cursor-grab',
        )}
      >
        <GripVertical className="size-5" />
      </button>
      <div className={cn('flex h-7 w-9 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted max-sm:col-start-2 max-sm:row-start-1', !row.enabled && 'opacity-60')}>
        {row.country ? (
          <CountryFlag code={row.country} className="h-full w-full object-cover" />
        ) : (
          <Server className="size-4 text-muted-foreground" />
        )}
      </div>
      <button type="button" onClick={onEdit} className="min-w-0 flex-1 overflow-hidden text-left max-sm:contents">
        <div className={cn('flex items-center gap-2 max-sm:col-start-3 max-sm:row-start-1 max-sm:min-w-0', !row.enabled && 'opacity-60')}>
          <span className="truncate font-medium">{row.name || t('admin.statusPage.noName')}</span>
          <span className={cn(kindClass, 'max-sm:hidden')}>{kindLabel}</span>
        </div>
        <p className={cn('mt-0.5 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground max-sm:col-span-3 max-sm:col-start-1 max-sm:row-start-2 max-sm:mt-0', !row.enabled && 'opacity-60')}>
          <span className={cn(kindClass, 'sm:hidden')}>{kindLabel}</span>
          {node ? (
            <>
              <span className={cn('size-1.5 shrink-0 rounded-full', STATE_DOT[node.state] ?? 'bg-muted-foreground')} />
              <span className="shrink-0 max-sm:sr-only">{t(`admin.statusPage.panel.${node.state}`)}</span>
            </>
          ) : (
            <span className="shrink-0 max-sm:sr-only">{t('admin.statusPage.noPanel')}</span>
          )}
          <span className="truncate"><span className="max-sm:hidden">· </span>{row.address || t('admin.statusPage.noAddress')}</span>
        </p>
      </button>
      <AdminStatusRowChecks
        checks={checks}
        enabled={row.enabled}
        showRegistry={!bypass}
        className="max-sm:col-start-4 max-sm:row-start-2 max-sm:justify-self-end"
      />
      <div className="max-sm:col-start-4 max-sm:row-start-1 max-sm:justify-self-end">
        <AdminToggleSwitch checked={row.enabled} onChange={onToggle} aria-label={t('admin.statusPage.showOnPage')} />
      </div>
      <button
        type="button"
        aria-label={t('admin.statusPage.edit')}
        onClick={onEdit}
        className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground max-sm:hidden"
      >
        <Pencil className="size-4" />
      </button>
    </li>
  )
}

function AddServerModal({
  open,
  onClose,
  nodes,
  loading,
  failed,
  usedIds,
  onPick,
  onManual,
}: {
  open: boolean
  onClose: () => void
  nodes: AdminStatusNodeDTO[]
  loading: boolean
  failed: boolean
  usedIds: Set<string>
  onPick: (node: AdminStatusNodeDTO) => void
  onManual: () => void
}) {
  const { t } = useTranslation()
  const free = nodes.filter((node) => !usedIds.has(node.uuid))
  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={t('admin.statusPage.addTitle')}
      description={t('admin.statusPage.addHint')}
      size="md"
      icon={Download}
      iconAccent="cyan"
      footer={
        <button
          type="button"
          onClick={onManual}
          className="inline-flex min-h-10 items-center gap-2 rounded-lg border border-border px-3.5 text-sm font-medium hover:bg-muted"
        >
          <Plus className="size-4" />
          {t('admin.statusPage.addManual')}
        </button>
      }
    >
      {loading ? (
        <div className="flex justify-center py-8 text-muted-foreground">
          <Loader2 className="size-5 animate-spin" />
        </div>
      ) : failed ? (
        <p className="py-4 text-sm text-muted-foreground">{t('admin.statusPage.panelError')}</p>
      ) : free.length === 0 ? (
        <p className="py-4 text-sm text-muted-foreground">{t('admin.statusPage.panelEmpty')}</p>
      ) : (
        <>
          <p className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t('admin.statusPage.fromPanel')}
          </p>
          <ul className="space-y-2">
            {free.map((node) => (
              <li key={node.uuid}>
                <button
                  type="button"
                  onClick={() => onPick(node)}
                  className="flex w-full items-center gap-3 rounded-xl border border-border/60 px-3 py-2.5 text-left hover:bg-muted"
                >
                  <div className="flex h-7 w-9 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted">
                    <CountryFlag code={node.country} className="h-full w-full object-cover" />
                  </div>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{node.name}</span>
                    <span className="block truncate text-xs text-muted-foreground">{node.address || '—'}</span>
                  </span>
                  <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <span className={cn('size-1.5 rounded-full', STATE_DOT[node.state] ?? 'bg-muted-foreground')} />
                    {t(`admin.statusPage.panel.${node.state}`)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </AdminModal>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="text-sm font-medium">{label}</span>
      <div className="mt-1.5">{children}</div>
      {hint && <span className="mt-1 block text-xs text-muted-foreground">{hint}</span>}
    </label>
  )
}

function NodeSelect({
  value,
  nodes,
  emptyLabel,
  onChange,
}: {
  value: string
  nodes: AdminStatusNodeDTO[]
  emptyLabel: string
  onChange: (uuid: string) => void
}) {
  return (
    <AdminSelect
      value={value || null}
      options={[
        ...(value && !nodes.some((node) => node.uuid === value) ? [{ value, label: value }] : []),
        ...nodes.map((node) => ({ value: node.uuid, label: node.name })),
      ]}
      onChange={(next) => onChange(next ?? '')}
      allowEmpty
      emptyLabel={emptyLabel}
      placeholder={emptyLabel}
      ariaLabel={emptyLabel}
    />
  )
}

function EditServerModal({
  row,
  nodes,
  onClose,
  onPatch,
  onRemove,
}: {
  row: StatusTargetDraft | null
  nodes: AdminStatusNodeDTO[]
  onClose: () => void
  onPatch: (next: Partial<StatusTargetDraft>) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const [advanced, setAdvanced] = useState(false)
  useEffect(() => {
    setAdvanced(false)
  }, [row?.id])

  if (!row) return <AdminModal open={false} onClose={onClose} title="">{null}</AdminModal>

  const bypass = row.kind === 'bypass'
  // Адрес так и остался адресом самой ноды из панели: для обхода проверять нужно хост из белого списка.
  const boundNode = nodes.find((node) => node.uuid === row.node_uuid)
  const addressIsNode = Boolean(boundNode && boundNode.address.trim() !== '' && boundNode.address.trim() === row.address.trim())
  const choice = (kind: 'regular' | 'bypass', title: string, text: string) => (
    <button
      type="button"
      onClick={() => onPatch({ kind, whitelist: kind === 'bypass' })}
      className={cn(
        'rounded-xl border p-3 text-left transition-colors',
        row.kind === kind ? 'border-primary bg-primary/10' : 'border-border/60 hover:bg-muted',
      )}
    >
      <span className="block text-sm font-semibold">{title}</span>
      <span className="mt-1 block text-xs leading-snug text-muted-foreground">{text}</span>
    </button>
  )

  return (
    <AdminModal
      open
      onClose={onClose}
      title={row.name || t('admin.statusPage.newServer')}
      size="md"
      icon={Server}
      iconAccent="cyan"
      footer={
        <div className="flex w-full items-center justify-between gap-3">
          <button
            type="button"
            onClick={onRemove}
            className="inline-flex min-h-10 items-center gap-2 rounded-lg px-3 text-sm font-medium text-destructive hover:bg-destructive/10"
          >
            <Trash2 className="size-4" />
            {t('admin.statusPage.remove')}
          </button>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex min-h-10 items-center rounded-lg bg-primary px-5 text-sm font-semibold text-primary-foreground hover:bg-primary/90"
          >
            {t('admin.statusPage.done')}
          </button>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-2">
          {choice('regular', t('admin.statusPage.typeRegularTitle'), t('admin.statusPage.typeRegularText'))}
          {choice('bypass', t('admin.statusPage.typeBypassTitle'), t('admin.statusPage.typeBypassText'))}
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('admin.statusPage.fName')} hint={t('admin.statusPage.fNameHint')}>
            <input
              className={inputClass}
              value={row.name}
              maxLength={80}
              onChange={(e) => onPatch({ name: e.target.value })}
            />
          </Field>
          <div>
            <span className="text-sm font-medium">{t('admin.statusPage.fCountry')}</span>
            <CountrySelect
              className="mt-1.5"
              value={row.country}
              placeholder={t('admin.statusPage.countryPlaceholder')}
              onChange={(country) => onPatch({ country })}
            />
            <span className="mt-1 block text-xs text-muted-foreground">{t('admin.statusPage.fCountryHint')}</span>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_9rem]">
          <Field
            label={bypass ? t('admin.statusPage.fAddressBypass') : t('admin.statusPage.fAddress')}
            hint={bypass ? t('admin.statusPage.fAddressBypassHint') : t('admin.statusPage.fAddressHint')}
          >
            <input
              className={inputClass}
              value={row.address}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => onPatch({ address: e.target.value })}
            />
          </Field>
          <Field label={t('admin.statusPage.fPort')} hint={t('admin.statusPage.fPortHint')}>
            <input
              className={inputClass}
              type="number"
              inputMode="numeric"
              min={1}
              max={65535}
              placeholder="443"
              value={row.tcp_port || ''}
              onChange={(e) => onPatch({ tcp_port: Number(e.target.value) || 0 })}
            />
          </Field>
        </div>
        {bypass && addressIsNode && (
          <p className="-mt-2 rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
            {t('admin.statusPage.addressIsNode')}
          </p>
        )}

        <div className={cn('grid gap-3', bypass && 'sm:grid-cols-2')}>
          <Field
            label={bypass ? t('admin.statusPage.fEntryNode') : t('admin.statusPage.fNode')}
            hint={bypass ? undefined : t('admin.statusPage.fNodeHint')}
          >
            <NodeSelect
              value={row.node_uuid}
              nodes={nodes}
              emptyLabel={t('admin.statusPage.nodeAuto')}
              onChange={(node_uuid) => onPatch({ node_uuid })}
            />
          </Field>
          {bypass && (
            <Field label={t('admin.statusPage.fExitNode')} hint={t('admin.statusPage.fExitHint')}>
              <NodeSelect
                value={row.exit_node_uuid}
                nodes={nodes}
                emptyLabel={t('admin.statusPage.nodeNone')}
                onChange={(exit_node_uuid) => onPatch({ exit_node_uuid })}
              />
            </Field>
          )}
        </div>

        <Field label={t('admin.statusPage.fNote')} hint={t('admin.statusPage.fNoteHint')}>
          <input
            className={inputClass}
            value={row.note}
            maxLength={48}
            onChange={(e) => onPatch({ note: e.target.value })}
          />
        </Field>

        <div className="rounded-xl border border-border/60">
          <button
            type="button"
            aria-expanded={advanced}
            onClick={() => setAdvanced((v) => !v)}
            className="flex w-full items-center justify-between px-3 py-2.5 text-sm font-medium"
          >
            {t('admin.statusPage.advanced')}
            <ChevronDown className={cn('size-4 text-muted-foreground transition-transform', advanced && 'rotate-180')} />
          </button>
          {advanced && (
            <div className="space-y-4 border-t border-border/60 p-3">
              <div className="grid items-end gap-3 sm:grid-cols-3">
                <Field label={t('admin.statusPage.fInterval')}>
                  <input
                    className={inputClass}
                    type="number"
                    min={5}
                    max={180}
                    placeholder={t('admin.statusPage.shared')}
                    value={row.interval_min || ''}
                    onChange={(e) => onPatch({ interval_min: Number(e.target.value) || 0 })}
                  />
                </Field>
                <Field label={t('admin.statusPage.fWorld')}>
                  <input
                    className={inputClass}
                    type="number"
                    min={0}
                    max={10}
                    placeholder={t('admin.statusPage.shared')}
                    value={row.world_probes || ''}
                    onChange={(e) => onPatch({ world_probes: Number(e.target.value) || 0 })}
                  />
                </Field>
                <Field label={t('admin.statusPage.fRussia')}>
                  <input
                    className={inputClass}
                    type="number"
                    min={0}
                    max={30}
                    placeholder={t('admin.statusPage.shared')}
                    value={row.russia_probes || ''}
                    onChange={(e) => onPatch({ russia_probes: Number(e.target.value) || 0 })}
                  />
                </Field>
              </div>
              <ToggleLine
                label={t('admin.statusPage.fRepeat')}
                hint={t('admin.statusPage.fRepeatHint')}
                checked={row.repeat_cities}
                onChange={(repeat_cities) => onPatch({ repeat_cities })}
              />
              <ToggleLine
                label={t('admin.statusPage.fNoExternal')}
                hint={t('admin.statusPage.fNoExternalHint')}
                checked={row.no_external}
                onChange={(no_external) => onPatch({ no_external })}
              />
              {row.no_external && !row.node_uuid && (
                <p className="rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
                  {t('admin.statusPage.fNoExternalNeedsNode')}
                </p>
              )}
            </div>
          )}
        </div>
      </div>
    </AdminModal>
  )
}

function ToggleLine({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="min-w-0">
        <p className="text-sm font-medium">{label}</p>
        <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>
      </div>
      <AdminToggleSwitch checked={checked} onChange={onChange} aria-label={label} />
    </div>
  )
}
