import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ListFilter, Search, X } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'
import type { AdminPromoListDTO, AdminPromoStatusFilter, AdminPromoTypeFilter } from '@/lib/types/admin'
import { AdminFilterMenuDivider, AdminFilterMenuItem, AdminFilterMenuTitle } from '../components/AdminFilterMenu'
import type { SortState } from '../components/SortableTh'
import { PROMO_TYPES } from './promoPresentation'

const STATUS_TABS: AdminPromoStatusFilter[] = ['all', 'active', 'ended', 'off']

type Counts = AdminPromoListDTO['counts']

/**
 * Готовые сортировки в меню. null — порядок по умолчанию (сначала новые).
 * В шапке таблицы сортировка та же, только с выбором направления.
 */
export const PROMO_SORT_PRESETS: { id: 'new' | 'uses' | 'expiring' | 'code'; sort: SortState | null }[] = [
  { id: 'new', sort: null },
  { id: 'uses', sort: { key: 'uses', dir: 'desc' } },
  { id: 'expiring', sort: { key: 'expiring', dir: 'asc' } },
  { id: 'code', sort: { key: 'code', dir: 'asc' } },
]

const sameSort = (a: SortState | null, b: SortState | null) => (a == null || b == null ? a === b : a.key === b.key && a.dir === b.dir)

/** Вкладки статуса — тот же вид, что у «Тарифов» и «Настроек бота». */
export function PromoStatusTabs({
  value,
  counts,
  busy,
  onChange,
}: {
  value: AdminPromoStatusFilter
  counts: Counts
  /** Список перезапрашивается по клику на вкладку — спиннер вместо счётчика активной. */
  busy?: boolean
  onChange: (v: AdminPromoStatusFilter) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="-mx-1 overflow-x-auto overscroll-x-contain px-1 pb-0.5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      <div role="tablist" className="inline-flex min-w-full gap-1 rounded-lg border border-border/50 bg-card/50 p-1 md:min-w-0">
        {STATUS_TABS.map((s) => {
          const active = value === s
          return (
            <button
              key={s}
              role="tab"
              type="button"
              aria-selected={active}
              tabIndex={active ? 0 : -1}
              onClick={() => onChange(s)}
              className={cn(
                'inline-flex min-h-9 shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-3 py-2 text-xs font-medium transition-colors sm:px-4',
                active ? 'bg-primary/10 text-primary dark:bg-primary/20' : 'text-foreground/80 hover:bg-accent hover:text-foreground',
              )}
            >
              {t(`admin.promos.tabs.${s}`)}
              {active && busy ? (
                <Spinner className="h-3 w-3" />
              ) : (
                counts && (
                  <span
                    className={cn(
                      'rounded-full px-1.5 py-px text-[10px] font-semibold tabular-nums',
                      active ? 'bg-primary/15 text-primary' : 'bg-secondary text-muted-foreground',
                    )}
                  >
                    {counts.status[s]}
                  </span>
                )
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}

/** Поиск — тот же вид, что у раскрытого поиска на «Нодах». */
export function PromoSearchInput({ value, onChange, className }: { value: string; onChange: (v: string) => void; className?: string }) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'flex h-10 min-w-0 items-center gap-2 rounded-xl border border-border bg-card pl-3 pr-1 transition-colors',
        'focus-within:border-primary/50 focus-within:bg-muted/60 focus-within:ring-2 focus-within:ring-primary/15',
        className,
      )}
    >
      <Search className="size-4 shrink-0 text-muted-foreground" aria-hidden />
      <input
        value={value}
        maxLength={64}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t('admin.promos.searchPlaceholder')}
        aria-label={t('admin.promos.searchPlaceholder')}
        className="h-8 min-w-0 flex-1 !border-0 !bg-transparent text-sm !shadow-none outline-none placeholder:text-muted-foreground"
      />
      {value && (
        <button
          type="button"
          onClick={() => onChange('')}
          aria-label={t('admin.promos.searchClear')}
          className="flex size-8 shrink-0 items-center justify-center rounded-[9px] text-muted-foreground hover:bg-secondary hover:text-foreground"
        >
          <X className="size-4" aria-hidden />
        </button>
      )}
    </div>
  )
}

/**
 * Одна кнопка «Сортировка и тип» — как на «Нодах»: на ПК с подписью, на
 * телефоне иконкой с точкой, когда выбрано не по умолчанию.
 */
export function PromoFilterMenu({
  sort,
  type,
  counts,
  busy,
  onSort,
  onType,
}: {
  sort: SortState | null
  type: AdminPromoTypeFilter | null
  counts: Counts
  busy?: boolean
  onSort: (s: SortState | null) => void
  onType: (v: AdminPromoTypeFilter | null) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const preset = PROMO_SORT_PRESETS.find((p) => sameSort(p.sort, sort))
  const sortLabel = preset
    ? t(`admin.promos.sort.${preset.id}`)
    : sort
      ? `${t(`admin.promos.sort.column.${sort.key}`)} ${sort.dir === 'asc' ? '↑' : '↓'}`
      : t('admin.promos.sort.new')
  const active = sort != null || type != null

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        aria-label={t('admin.promos.filters.sheetTitle')}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'relative inline-flex h-10 items-center justify-center gap-2 rounded-xl border px-0 text-sm font-semibold transition-colors',
          'w-10 sm:w-auto sm:px-3.5',
          open || active ? 'border-primary/50 bg-primary/10 text-foreground' : 'border-border bg-card hover:bg-secondary/80',
        )}
      >
        {busy ? <Spinner /> : <ListFilter className="size-4 shrink-0" aria-hidden />}
        <span className="hidden whitespace-nowrap sm:inline">
          <span className="font-medium text-muted-foreground">{t('admin.promos.sort.label')}:</span> {sortLabel}
          {type && <span className="text-primary"> · {t(`admin.promos.types.${type}`)}</span>}
        </span>
        <ChevronDown className="hidden size-3.5 shrink-0 sm:block" aria-hidden />
        {active && <span className="absolute -right-0.5 -top-0.5 size-2.5 rounded-full bg-primary sm:hidden" />}
      </button>
      {open && (
        <div
          role="menu"
          className="absolute right-0 top-12 z-30 w-72 max-w-[calc(100vw-2rem)] rounded-2xl border border-border bg-card p-2 shadow-[0_18px_50px_rgba(0,0,0,0.5)]"
        >
          <AdminFilterMenuTitle>{t('admin.promos.filters.sortSection')}</AdminFilterMenuTitle>
          {PROMO_SORT_PRESETS.map((p) => (
            <AdminFilterMenuItem
              key={p.id}
              active={sameSort(p.sort, sort)}
              onClick={() => {
                onSort(p.sort)
                setOpen(false)
              }}
            >
              {t(`admin.promos.sort.${p.id}`)}
            </AdminFilterMenuItem>
          ))}
          <AdminFilterMenuDivider />
          <AdminFilterMenuTitle>{t('admin.promos.filters.typeSection')}</AdminFilterMenuTitle>
          {([null, ...PROMO_TYPES] as (AdminPromoTypeFilter | null)[]).map((opt) => (
            <AdminFilterMenuItem
              key={opt ?? 'all'}
              active={opt === type}
              count={counts ? (opt ? counts.type[opt] : counts.type.all) : undefined}
              onClick={() => {
                onType(opt)
                setOpen(false)
              }}
            >
              {opt ? t(`admin.promos.types.${opt}`) : t('admin.promos.filters.allTypes')}
            </AdminFilterMenuItem>
          ))}
        </div>
      )}
    </div>
  )
}
