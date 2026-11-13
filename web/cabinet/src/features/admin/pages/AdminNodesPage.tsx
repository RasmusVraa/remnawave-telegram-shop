import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { keepPreviousData } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Activity, ArrowDown, ArrowUp, Check, ChevronDown, ListFilter, RefreshCw, Search, Server, Users, X } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { AdminNodesDTO } from '@/lib/types/admin'
import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { NodeCard, type NodeLayout } from '../nodes/NodeCard'
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
/** Уже этой ширины список не помещается в строку: переходим на карточки, как на телефоне. */
const ROW_MIN_WIDTH_PX = 900

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
  const q = useAdminNodes()
  const [prefs, setPrefs] = useState<NodePrefs>(loadNodePrefs)
  const [query, setQuery] = useState('')
  const [searchOpen, setSearchOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const listRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const layout = useListLayout(listRef)

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
  const visible = useMemo(
    () => sortNodes(filterNodes(nodes ?? [], prefs.filter, query), prefs.sort),
    [nodes, prefs, query],
  )

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
            <MenuTitle>{t('admin.nodes.sortTitle')}</MenuTitle>
            {NODE_SORT_KEYS.map((k: NodeSortKey) => (
              <MenuItem key={k} active={prefs.sort === k} onClick={() => updatePrefs({ sort: k })}>
                {t(`admin.nodes.sort.${k}`)}
              </MenuItem>
            ))}
            <div className="my-1.5 border-t border-border/60" />
            <MenuTitle>{t('admin.nodes.showTitle')}</MenuTitle>
            {NODE_FILTER_KEYS.map((k: NodeFilterKey) => (
              <MenuItem
                key={k}
                active={prefs.filter === k}
                count={counts[k]}
                onClick={() => {
                  updatePrefs({ filter: k })
                  setMenuOpen(false)
                }}
              >
                {t(`admin.nodes.filter.${k}`)}
              </MenuItem>
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

          {visible.map((n) => (
            <NodeCard key={n.uuid} node={n} layout={layout} />
          ))}
        </div>
      </div>
    </AdminLayout>
  )
}

function MenuTitle({ children }: { children: ReactNode }) {
  return (
    <p className="px-2.5 pb-1 pt-2 text-[11px] font-bold uppercase tracking-wider text-muted-foreground">{children}</p>
  )
}

function MenuItem({
  active,
  count,
  onClick,
  children,
}: {
  active: boolean
  count?: number
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      role="menuitemradio"
      aria-checked={active}
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-2.5 rounded-[10px] px-2.5 py-2 text-left text-sm font-medium transition-colors',
        active ? 'bg-primary/10 text-primary' : 'text-foreground/90 hover:bg-secondary',
      )}
    >
      <span className="flex size-4 shrink-0 items-center justify-center">
        {active && <Check className="size-3.5" aria-hidden />}
      </span>
      <span className="flex-1">{children}</span>
      {count != null && <span className="font-mono text-xs text-muted-foreground">{count}</span>}
    </button>
  )
}
