import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  Activity,
  ArrowDown,
  ArrowUp,
  ChevronDown,
  GripVertical,
  ListFilter,
  RefreshCw,
  Search,
  Server,
  Users,
  X,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { AdminFilterMenuItem, AdminFilterMenuTitle } from '../components/AdminFilterMenu'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/components/ui/toast'
import type { AdminNodesDTO } from '@/lib/types/admin'
import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { useSortableList } from '../hooks/useSortableList'
import { NodeCard, type NodeChecks, type NodeLayout } from '../nodes/NodeCard'
import { splitBitrate } from '../nodes/format'
import {
  NODE_FILTER_KEYS,
  NODE_SORT_KEYS,
  countByFilter,
  filterNodes,
  loadNodePrefs,
  saveNodePrefs,
  sortNodes,
  summarize,
  type NodeFilterKey,
  type NodePrefs,
  type NodeSortKey,
} from '../nodes/nodeModel'

/** Пока вкладка открыта, обновляем раз в 10 с: скорость и «кто сейчас на ноде» быстро устаревают. */
const REFRESH_MS = 10_000
/** Пауза после последнего сдвига карточки: серия стрелок с клавиатуры уходит в панель одним запросом. */
const REORDER_SAVE_DELAY_MS = 300
/** Уже этой ширины список не помещается в строку: переходим на карточки, как на телефоне. */
const ROW_MIN_WIDTH_PX = 900

/**
 * Проверки «мир / РФ / РКН» из «Статусов серверов». Нода связана с целью по node_uuid: берём включённую
 * обычную цель, а если такой нет — включённую цель обхода. Нет цели или замера — у ноды нет значков.
 */
function useNodeChecks(): Map<string, NodeChecks> {
  const q = useQuery({
    queryKey: ['admin-status-targets'],
    queryFn: () => api.adminStatusTargets(),
    staleTime: 30_000,
    refetchInterval: 60_000,
    retry: 0,
  })
  const { targets, checks } = q.data ?? {}
  return useMemo(() => {
    const out = new Map<string, NodeChecks>()
    for (const row of targets ?? []) {
      const c = checks?.[row.id]
      if (!row.enabled || !row.node_uuid || !c) continue
      const bypass = row.kind === 'bypass'
      const measured = c.world.state !== 'none' || c.russia.state !== 'none' || (!bypass && !!c.registry && c.registry.state !== 'none')
      if (!measured) continue
      const cur = out.get(row.node_uuid)
      if (cur && (!cur.bypass || bypass)) continue
      out.set(row.node_uuid, { checks: c, bypass })
    }
    return out
  }, [targets, checks])
}

function useAdminNodes() {
  return useQuery<AdminNodesDTO>({
    queryKey: ['admin-nodes'],
    queryFn: () => api.adminNodes(),
    staleTime: 5_000,
    refetchInterval: REFRESH_MS,
    placeholderData: keepPreviousData,
    retry: 1,
  })
}

/** Раскладка по ширине самого списка, а не окна: рядом может стоять боковое меню. */
function useListLayout(ref: React.RefObject<HTMLElement | null>): NodeLayout {
  const [layout, setLayout] = useState<NodeLayout>('card')
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const apply = () => setLayout(el.clientWidth >= ROW_MIN_WIDTH_PX ? 'row' : 'card')
    apply()
    const ro = new ResizeObserver(apply)
    ro.observe(el)
    return () => ro.disconnect()
  }, [ref])
  return layout
}

function SummaryCard({
  icon: Icon,
  tone,
  label,
  shortLabel,
  children,
}: {
  icon: LucideIcon
  tone: string
  label: string
  shortLabel: string
  children: ReactNode
}) {
  return (
    <Card className="flex min-w-0 items-center gap-2.5 rounded-2xl p-2.5 sm:gap-3 sm:p-3">
      <div className={cn('flex size-9 shrink-0 items-center justify-center rounded-xl border sm:size-11', tone)}>
        <Icon className="size-[18px] sm:size-5" aria-hidden />
      </div>
      <div className="min-w-0">
        <p className="truncate text-[11px] font-semibold text-muted-foreground sm:text-[12.5px]">
          <span className="sm:hidden">{shortLabel}</span>
          <span className="hidden sm:inline">{label}</span>
        </p>
        <p className="whitespace-nowrap font-mono text-[15.5px] font-bold tracking-tight sm:text-xl">{children}</p>
      </div>
    </Card>
  )
}

function Unit({ children }: { children: ReactNode }) {
  return <small className="ml-1 text-[11px] font-medium text-muted-foreground sm:text-[13px]">{children}</small>
}

export default function AdminNodesPage() {
  const { t } = useTranslation()
  const toast = useToast()
  const qc = useQueryClient()
  const q = useAdminNodes()
  const checksByNode = useNodeChecks()
  const [prefs, setPrefs] = useState<NodePrefs>(loadNodePrefs)
  const [query, setQuery] = useState('')
  const [searchOpen, setSearchOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const listRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const layout = useListLayout(listRef)
  // Порядок, который админ только что выставил: показываем его, пока панель не подтвердила.
  const [localOrder, setLocalOrder] = useState<string[] | null>(null)
  const localOrderRef = useRef<string[] | null>(null)
  localOrderRef.current = localOrder

  const updatePrefs = useCallback((next: Partial<NodePrefs>) => {
    setPrefs((cur) => {
      const merged = { ...cur, ...next }
      saveNodePrefs(merged)
      return merged
    })
  }, [])

  useEffect(() => {
    if (searchOpen) searchRef.current?.focus()
  }, [searchOpen])

  useEffect(() => {
    if (!menuOpen) return
    const onDown = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) setMenuOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenuOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [menuOpen])

  const nodes = q.data?.nodes
  const summary = useMemo(() => summarize(nodes ?? []), [nodes])
  const counts = useMemo(() => countByFilter(nodes ?? []), [nodes])
  const visible = useMemo(() => {
    const sorted = sortNodes(filterNodes(nodes ?? [], prefs.filter, query), prefs.sort)
    if (!localOrder || prefs.sort !== 'panel') return sorted
    const pos = new Map(localOrder.map((id, i) => [id, i]))
    return sorted.sort((a, b) => (pos.get(a.uuid) ?? Infinity) - (pos.get(b.uuid) ?? Infinity))
  }, [nodes, prefs, query, localOrder])

  // Порядок можно менять, только когда виден весь список в порядке панели: при фильтре, поиске
  // или другой сортировке «место карточки» не значит место ноды в панели.
  const canReorder = prefs.sort === 'panel' && prefs.filter === 'all' && query.trim() === '' && !!nodes && nodes.length > 1

  const reorder = useMutation({
    meta: { haptics: true },
    mutationFn: async (uuids: string[]) => {
      // Опрос списка, уже ушедший к панели, вернул бы старый порядок поверх нашего.
      await qc.cancelQueries({ queryKey: ['admin-nodes'] })
      return api.adminNodesReorder(uuids)
    },
    onSuccess: (_res, uuids) => {
      const pos = new Map(uuids.map((id, i) => [id, i + 1]))
      qc.setQueryData<AdminNodesDTO>(['admin-nodes'], (cur) =>
        cur && {
          nodes: cur.nodes
            .map((n) => ({ ...n, view_position: pos.get(n.uuid) ?? n.view_position }))
            .sort((a, b) => a.view_position - b.view_position),
        },
      )
      // Если за время запроса админ успел двинуть ещё — это уже другой порядок, его не трогаем.
      if (localOrderRef.current?.join(',') === uuids.join(',')) setLocalOrder(null)
    },
    onError: (_err, uuids) => {
      toast.error(t('admin.nodes.reorderError'))
      if (localOrderRef.current?.join(',') === uuids.join(',')) setLocalOrder(null)
      void qc.invalidateQueries({ queryKey: ['admin-nodes'] })
    },
  })

  const sortable = useSortableList(
    visible.map((n) => n.uuid),
    setLocalOrder,
  )

  // Сохраняем после паузы и только когда карточку отпустили: пока её тянут, порядок меняется на каждом шаге.
  const { mutate: saveOrder } = reorder
  useEffect(() => {
    if (!localOrder || sortable.dragId) return
    const timer = window.setTimeout(() => saveOrder(localOrder), REORDER_SAVE_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [localOrder, sortable.dragId, saveOrder])

  const closeSearch = () => {
    setSearchOpen(false)
    setQuery('')
  }

  const down = splitBitrate(summary.rxBytesPerSec)
  const up = splitBitrate(summary.txBytesPerSec)
  const filterActive = prefs.filter !== 'all'

  const actions = searchOpen ? (
    <div className="flex items-center gap-2">
      <div className="flex h-10 w-full min-w-0 items-center gap-2 rounded-xl border border-primary/50 bg-muted/60 pl-3 pr-1 ring-2 ring-primary/15 sm:w-80">
        <Search className="size-4 shrink-0 text-muted-foreground" aria-hidden />
        <input
          ref={searchRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === 'Escape' && closeSearch()}
          placeholder={t('admin.nodes.searchPlaceholder')}
          aria-label={t('admin.nodes.search')}
          className="min-w-0 flex-1 !border-0 !bg-transparent text-sm !shadow-none outline-none placeholder:text-muted-foreground"
        />
        <button
          type="button"
          onClick={closeSearch}
          aria-label={t('admin.nodes.searchClose')}
          className="flex size-8 shrink-0 items-center justify-center rounded-[9px] text-muted-foreground hover:bg-secondary hover:text-foreground"
        >
          <X className="size-4" aria-hidden />
        </button>
      </div>
    </div>
  ) : (
    <div className="flex items-center gap-2">
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="size-10 rounded-xl bg-card dark:bg-card"
        aria-label={t('admin.nodes.search')}
        onClick={() => setSearchOpen(true)}
      >
        <Search />
      </Button>
      <div className="relative" ref={menuRef}>
        <button
          type="button"
          aria-label={t('admin.nodes.sortTitle')}
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          onClick={() => setMenuOpen((v) => !v)}
          className={cn(
            'relative inline-flex h-10 items-center justify-center gap-2 rounded-xl border px-0 text-sm font-semibold transition-colors',
            'w-10 sm:w-auto sm:px-3.5',
            menuOpen || filterActive
              ? 'border-primary/50 bg-primary/10 text-foreground'
              : 'border-border bg-card hover:bg-secondary/80',
          )}
        >
          <ListFilter className="size-4 shrink-0" aria-hidden />
          <span className="hidden sm:inline">
            <span className="font-medium text-muted-foreground">{t('admin.nodes.sortLabel')}</span>{' '}
            {t(`admin.nodes.sort.${prefs.sort}`)}
          </span>
          <ChevronDown className="hidden size-3.5 shrink-0 sm:block" aria-hidden />
          {filterActive && <span className="absolute -right-0.5 -top-0.5 size-2.5 rounded-full bg-primary sm:hidden" />}
        </button>
        {menuOpen && (
          <div
            role="menu"
            className="absolute right-0 top-12 z-30 w-72 max-w-[calc(100vw-2rem)] rounded-2xl border border-border bg-card p-2 shadow-[0_18px_50px_rgba(0,0,0,0.5)]"
          >
            <AdminFilterMenuTitle>{t('admin.nodes.sortTitle')}</AdminFilterMenuTitle>
            {NODE_SORT_KEYS.map((k: NodeSortKey) => (
              <AdminFilterMenuItem key={k} active={prefs.sort === k} onClick={() => updatePrefs({ sort: k })}>
                {t(`admin.nodes.sort.${k}`)}
              </AdminFilterMenuItem>
            ))}
            <div className="my-1.5 border-t border-border/60" />
            <AdminFilterMenuTitle>{t('admin.nodes.showTitle')}</AdminFilterMenuTitle>
            {NODE_FILTER_KEYS.map((k: NodeFilterKey) => (
              <AdminFilterMenuItem
                key={k}
                active={prefs.filter === k}
                count={counts[k]}
                onClick={() => {
                  updatePrefs({ filter: k })
                  setMenuOpen(false)
                }}
              >
                {t(`admin.nodes.filter.${k}`)}
              </AdminFilterMenuItem>
            ))}
          </div>
        )}
      </div>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="size-10 rounded-xl bg-card dark:bg-card"
        aria-label={t('admin.nodes.refresh')}
        loading={q.isFetching}
        onClick={() => void q.refetch()}
      >
        <RefreshCw />
      </Button>
    </div>
  )

  return (
    <AdminLayout>
      <div className="space-y-4">
        <AdminPageHeader
          icon={Server}
          title={t('admin.nodes.title')}
          accent="primary"
          actions={actions}
        />

        <div className="grid grid-cols-2 gap-2 xl:grid-cols-4 xl:gap-3">
          <SummaryCard
            icon={Users}
            tone="border-cyan-500/40 bg-cyan-500/10 text-cyan-400"
            label={t('admin.nodes.summary.users')}
            shortLabel={t('admin.nodes.summary.usersShort')}
          >
            {q.data ? summary.users : '—'}
          </SummaryCard>
          <SummaryCard
            icon={Activity}
            tone="border-emerald-500/40 bg-emerald-500/10 text-emerald-400"
            label={t('admin.nodes.summary.nodes')}
            shortLabel={t('admin.nodes.summary.nodes')}
          >
            {q.data ? summary.online : '—'}
            {q.data && <Unit>{t('admin.nodes.summary.of', { total: summary.total })}</Unit>}
          </SummaryCard>
          <SummaryCard
            icon={ArrowDown}
            tone="border-emerald-500/40 bg-emerald-500/10 text-emerald-400"
            label={t('admin.nodes.summary.down')}
            shortLabel={t('admin.nodes.summary.downShort')}
          >
            {q.data ? down.value : '—'}
            {q.data && <Unit>{down.unit}</Unit>}
          </SummaryCard>
          <SummaryCard
            icon={ArrowUp}
            tone="border-indigo-400/40 bg-indigo-500/10 text-indigo-300"
            label={t('admin.nodes.summary.up')}
            shortLabel={t('admin.nodes.summary.upShort')}
          >
            {q.data ? up.value : '—'}
            {q.data && <Unit>{up.unit}</Unit>}
          </SummaryCard>
        </div>

        <div ref={listRef} className="flex flex-col gap-2">
          {q.isLoading && Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-20 rounded-2xl" />)}

          {q.isError && !q.data && (
            <Card className="flex flex-col items-center gap-3 rounded-2xl p-8 text-center">
              <p className="text-sm text-muted-foreground">{t('admin.nodes.error')}</p>
              <Button type="button" variant="outline" size="sm" loading={q.isFetching} onClick={() => void q.refetch()}>
                <RefreshCw />
                {t('admin.nodes.retry')}
              </Button>
            </Card>
          )}

          {q.data && nodes?.length === 0 && (
            <Card className="rounded-2xl p-8 text-center text-sm text-muted-foreground">{t('admin.nodes.empty')}</Card>
          )}

          {q.data && nodes && nodes.length > 0 && visible.length === 0 && (
            <Card className="rounded-2xl p-8 text-center text-sm text-muted-foreground">{t('admin.nodes.noMatch')}</Card>
          )}

          <ul className="relative flex flex-col gap-2" {...(canReorder ? sortable.listProps : {})}>
            {visible.map((n) => (
              <li
                key={n.uuid}
                ref={canReorder ? sortable.itemRef(n.uuid) : undefined}
                className={cn(
                  'relative rounded-[14px] transition-shadow duration-200',
                  sortable.dragId === n.uuid && 'z-20 shadow-2xl shadow-black/40',
                )}
              >
                <NodeCard node={n} layout={layout} checks={checksByNode.get(n.uuid)} reserveHandle={canReorder} />
                {canReorder && (
                  <button
                    type="button"
                    aria-label={t('admin.nodes.drag')}
                    title={t('admin.nodes.drag')}
                    {...sortable.handleProps(n.uuid)}
                    className={cn(
                      'absolute right-1.5 top-1/2 flex h-9 w-7 -translate-y-1/2 touch-none items-center justify-center rounded-[9px] text-muted-foreground transition-colors',
                      'hover:bg-secondary/80 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                      sortable.dragId === n.uuid ? 'cursor-grabbing text-foreground' : 'cursor-grab',
                    )}
                  >
                    <GripVertical className="size-5" aria-hidden />
                  </button>
                )}
              </li>
            ))}
          </ul>
        </div>
      </div>
    </AdminLayout>
  )
}

