import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { ChevronLeft, ChevronRight, Plus, RotateCcw, SearchX, TicketPercent } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import type { AdminPromoCodeDTO, AdminPromoStatusFilter, AdminPromoTypeFilter } from '@/lib/types/admin'
import { nextSort, type SortDir, type SortState } from '../components/SortableTh'
import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminFeedback } from '../components/AdminFeedback'
import { useAdminMutationFeedback } from '../hooks/useAdminMutationFeedback'
import { useAdminPromoList } from '../hooks/useAdminPromos'
import { useAdminTariffList } from '../hooks/useAdminTariffs'
import { useAdminBootstrap } from '../hooks/useAdminBootstrap'
import { resolvePromoDisplayStatus } from '../utils/promoStatus'
import { usePromoListLayout } from '../promos/PromoBits'
import { PromoCard, PromoRow, PromoTableHeader, PROMO_TABLE_COLUMNS, type PromoItemActions } from '../promos/PromoRow'
import { PromoFilterMenu, PromoSearchInput, PromoStatusTabs } from '../promos/PromoFilters'
import { PromoFormModal, type PromoFormMode } from '../promos/PromoFormModal'
import { PromoUsedByModal } from '../promos/PromoUsedByModal'
import { PromoDetailSheet } from '../promos/PromoDetailSheet'
import { PromoDeleteModal } from '../promos/PromoDeleteModal'
import { isPromoType } from '../promos/promoPresentation'

const LIMIT = 20
const STATUSES: AdminPromoStatusFilter[] = ['all', 'active', 'ended', 'off']
const SORT_KEYS = ['new', 'uses', 'expiring', 'code'] as const
/** Первое направление при клике по заголовку (как SORT_FIRST_DIR у «Пользователей»). */
const SORT_FIRST_DIR: Record<string, SortDir> = { code: 'asc', uses: 'desc', expiring: 'asc' }

/** Сортировка из адреса; старые ссылки ?sort=uses без dir тоже понимаем. */
function parseSort(key: string | null, dir: string | null): SortState | null {
  if (!key || key === 'new') return dir === 'asc' ? { key: 'new', dir: 'asc' } : null
  if (!(SORT_KEYS as readonly string[]).includes(key)) return null
  const d: SortDir = dir === 'asc' || dir === 'desc' ? dir : (SORT_FIRST_DIR[key] ?? 'asc')
  return { key, dir: d }
}

/** Последнее непустое значение: окно уезжает со своим содержимым, а не пустым. */
function useLast<T>(value: T | null): T | null {
  const ref = useRef<T | null>(value)
  if (value != null) ref.current = value
  return ref.current
}

/** Вкладка, тип, сортировка, поиск и страница живут в адресе: перезагрузка и «назад» их сохраняют. */
function usePromoUrlState() {
  const [params, setParams] = useSearchParams()
  const rawStatus = params.get('status')
  const status: AdminPromoStatusFilter = STATUSES.includes(rawStatus as AdminPromoStatusFilter) ? (rawStatus as AdminPromoStatusFilter) : 'all'
  const rawType = params.get('type')
  const type: AdminPromoTypeFilter | null = isPromoType(rawType) ? rawType : null
  const sortParam = params.get('sort')
  const dirParam = params.get('dir')
  const sort = useMemo(() => parseSort(sortParam, dirParam), [sortParam, dirParam])
  const q = params.get('q') ?? ''
  const page = Math.max(1, Number(params.get('page')) || 1)

  const update = useCallback(
    (patch: Partial<{ status: AdminPromoStatusFilter; type: AdminPromoTypeFilter | null; sort: SortState | null; q: string; page: number }>, replace = false) => {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          const put = (k: string, v: string | null, def: string | null) => {
            if (v == null || v === '' || v === def) next.delete(k)
            else next.set(k, v)
          }
          if ('status' in patch) put('status', patch.status ?? null, 'all')
          if ('type' in patch) put('type', patch.type ?? null, null)
          if ('sort' in patch) {
            put('sort', patch.sort?.key ?? null, null)
            put('dir', patch.sort ? patch.sort.dir : null, null)
          }
          if ('q' in patch) put('q', patch.q?.trim() ?? null, '')
          // Любая смена фильтра возвращает на первую страницу.
          if ('page' in patch) put('page', patch.page && patch.page > 1 ? String(patch.page) : null, null)
          else next.delete('page')
          return next
        },
        { replace },
      )
    },
    [setParams],
  )
  return { status, type, sort, q, page, update }
}

type Dialog =
  | { kind: 'form'; mode: PromoFormMode }
  | { kind: 'used'; promo: AdminPromoCodeDTO }
  | { kind: 'detail'; promo: AdminPromoCodeDTO }
  | { kind: 'delete'; promo: AdminPromoCodeDTO }
  | null

export default function AdminPromosPage() {
  const { t, i18n } = useTranslation()
  const { status, type, sort, q, page, update } = usePromoUrlState()
  const { feedback, clear, showError, showSuccess, showInfo } = useAdminMutationFeedback()
  const { data: tariffList } = useAdminTariffList()
  const { data: bootstrap } = useAdminBootstrap()
  const salesModeTariffs = bootstrap?.sales_mode === 'tariffs'
  const loyaltyEnabled = Boolean(bootstrap?.loyalty_enabled)

  // Поиск: поле обновляется сразу, запрос — через 300 мс тишины.
  const [search, setSearch] = useState(q)
  // Из адреса — только если там правда другое: иначе дебаунс съедал бы пробел в конце.
  useEffect(() => setSearch((s) => (s.trim() === q ? s : q)), [q])
  useEffect(() => {
    if (search.trim() === q) return
    const id = window.setTimeout(() => {
      setPendingCtl(null)
      update({ q: search }, true)
    }, 300)
    return () => window.clearTimeout(id)
  }, [search, q, update])

  const list = useAdminPromoList({ page, limit: LIMIT, status, type, q, sort })
  // Какой контрол запустил перезапрос списка: спиннер только на нём (правило лоадеров).
  // 'head:<колонка>' — клик в шапке таблицы (code | uses | expiring | type | status).
  const [pendingCtl, setPendingCtl] = useState<string | null>(null)
  const busyCtl = list.isPlaceholderData ? pendingCtl : null
  // Сброс и на ответе из кэша (без placeholder-фазы): иначе спиннер всплыл бы
  // на «чужом» контроле при следующем перезапросе (например, «назад» в браузере).
  useEffect(() => {
    if (!list.isPlaceholderData) setPendingCtl(null)
  }, [list.isPlaceholderData, list.data])
  const go = (ctl: string, patch: Parameters<typeof update>[0]) => {
    setPendingCtl(ctl)
    update(patch)
  }
  const data = list.data
  const counts = data?.counts
  // Старый бэкенд без counts: фильтры и вкладки не показываем — счётчики были бы враньём (§6.7 спеки).
  const filtersSupported = !data || Boolean(counts)

  const tariffs = useMemo(() => (tariffList ?? []).map((x) => ({ id: x.id, name: x.name ?? x.slug })), [tariffList])
  const tariffName = useCallback(
    (p: AdminPromoCodeDTO) => (p.tariff_id != null ? tariffs.find((x) => x.id === p.tariff_id)?.name ?? null : null),
    [tariffs],
  )

  const listRef = useRef<HTMLDivElement>(null)
  const { layout, twoColumns } = usePromoListLayout(listRef)

  const [dialog, setDialog] = useState<Dialog>(null)
  // Окно поверх листа-карточки: лист остаётся под ним и снова виден после закрытия.
  const [detail, setDetail] = useState<AdminPromoCodeDTO | null>(null)
  const closeDialog = () => setDialog(null)

  // Лист-карточка показывает свежую версию кода после тумблера и правки.
  const detailFresh = detail ? data?.items.find((p) => p.id === detail.id) ?? detail : null
  // Закрытое окно уезжает со своим содержимым (анимация AdminModal), поэтому
  // держим последнее значение, а не обнуляем сразу.
  const detailShown = useLast(detailFresh)
  const usedShown = useLast(dialog?.kind === 'used' ? dialog.promo : null)
  const deleteShown = useLast(dialog?.kind === 'delete' ? dialog.promo : null)
  const formShown = useLast(dialog?.kind === 'form' ? dialog.mode : null)

  const onHeaderSort = (key: string) => go(`head:${key}`, { sort: nextSort(sort, key, SORT_FIRST_DIR[key] ?? 'asc') })

  const actions: PromoItemActions = {
    onEdit: (p, focus) => setDialog({ kind: 'form', mode: { kind: 'edit', promo: p, focus } }),
    onOpen: (p) => setDetail(p),
    onUsed: (p) => setDialog({ kind: 'used', promo: p }),
    onDuplicate: (p) => setDialog({ kind: 'form', mode: { kind: 'duplicate', source: p } }),
    onDelete: (p) => setDialog({ kind: 'delete', promo: p }),
    onToggled: (p) => {
      if (p.active) {
        const s = resolvePromoDisplayStatus(p)
        if (s === 'expired') return showInfo(t('admin.promos.toggle.expiredOn'))
        if (s === 'exhausted') return showInfo(t('admin.promos.toggle.exhaustedOn'))
      }
      showSuccess(t('admin.feedback.saved'))
    },
    onError: showError,
  }

  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / LIMIT))
  const items = data?.items ?? []
  const hasFilters = status !== 'all' || type != null || q !== ''
  const from = total ? (page - 1) * LIMIT + 1 : 0
  const to = Math.min(page * LIMIT, total)

  // Страница опустела (удалили последний код на ней) — назад, а не пустой экран.
  useEffect(() => {
    if (data && !list.isPlaceholderData && data.items.length === 0 && page > 1 && total > 0) update({ page: totalPages }, true)
  }, [data, list.isPlaceholderData, page, total, totalPages, update])

  const createButton = (
    <button
      type="button"
      onClick={() => setDialog({ kind: 'form', mode: { kind: 'create' } })}
      className="inline-flex h-10 items-center gap-2 rounded-[12px] bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
    >
      <Plus className="size-4" />
      {t('admin.promos.create')}
    </button>
  )

  return (
    <AdminLayout>
      <AdminFeedback feedback={feedback} onDismiss={clear} />
      <div className="space-y-4">
        <AdminPageHeader
          icon={TicketPercent}
          title={t('admin.promos.title')}
          accent="amber"
          actionsEnd
          actions={createButton}
        />

        {filtersSupported && <PromoStatusTabs value={status} counts={counts} busy={busyCtl === 'status'} onChange={(s) => go('status', { status: s })} />}

        {/* Панель как на «Нодах»: поиск и одна кнопка «Сортировка и тип»; справа сводка (ПК). */}
        <div className="flex items-center gap-2">
          <PromoSearchInput value={search} onChange={setSearch} className="flex-1 md:w-80 md:flex-none" />
          {filtersSupported && (
            <PromoFilterMenu
              sort={sort}
              type={type}
              counts={counts}
              busy={busyCtl === 'menu'}
              onSort={(v) => go('menu', { sort: v })}
              onType={(v) => go('menu', { type: v })}
            />
          )}
          {data?.summary && (
            <p className="ml-auto hidden whitespace-nowrap text-[13.5px] text-muted-foreground md:block">
              {t('admin.promos.summary.last24h')} <b className="font-semibold text-foreground tabular-nums">{data.summary.redemptions_24h}</b> ·{' '}
              {t('admin.promos.summary.total')}{' '}
              <b className="font-semibold text-foreground tabular-nums">{data.summary.redemptions_total.toLocaleString(i18n.language.startsWith('en') ? 'en-US' : 'ru-RU')}</b>
            </p>
          )}
        </div>

        <div ref={listRef}>
          {list.isError && !data ? (
            <EmptyState
              icon={<RotateCcw className="size-6" />}
              title={t('admin.promos.loadError')}
              action={
                <button type="button" onClick={() => void list.refetch()} className={secondaryBtn}>
                  {list.isFetching ? <SpinnerIcon /> : <RotateCcw className="size-4" />}
                  {t('admin.promos.retry')}
                </button>
              }
            />
          ) : list.isLoading ? (
            <ListSkeleton layout={layout} />
          ) : items.length === 0 ? (
            hasFilters ? (
              <EmptyState
                icon={<SearchX className="size-6" />}
                title={t('admin.promos.emptyFiltered')}
                action={
                  <button
                    type="button"
                    onClick={() => {
                      setSearch('')
                      update({ status: 'all', type: null, q: '' })
                    }}
                    className={secondaryBtn}
                  >
                    {t('admin.promos.resetFilters')}
                  </button>
                }
              />
            ) : (
              <EmptyState icon={<TicketPercent className="size-6" />} title={t('admin.promos.empty')} action={createButton} />
            )
          ) : layout === 'table' ? (
            <div role="table" className={cn('overflow-hidden rounded-[18px] border border-border/60 bg-card transition-opacity', list.isPlaceholderData && 'opacity-60')}>
              <PromoTableHeader
                sort={sort}
                onSort={filtersSupported ? onHeaderSort : undefined}
                type={type}
                onType={filtersSupported ? (v) => go('head:type', { type: v }) : undefined}
                status={status}
                onStatus={filtersSupported ? (v) => go('head:status', { status: v }) : undefined}
                busyKey={busyCtl?.startsWith('head:') ? busyCtl.slice(5) : null}
              />
              {items.map((p) => (
                <PromoRow key={p.id} promo={p} tariffName={tariffName(p)} actions={actions} />
              ))}
              <div className="flex items-center justify-between border-t border-border/60 px-5 py-3.5 text-[13px] text-muted-foreground">
                <span className="tabular-nums">{t('admin.promos.rangeOf', { from, to, total })}</span>
                <Pager page={page} totalPages={totalPages} onPage={(p) => go(p < page ? 'prev' : 'next', { page: p })} busyDir={busyCtl === 'prev' || busyCtl === 'next' ? busyCtl : null} />
              </div>
            </div>
          ) : (
            <div className={cn('transition-opacity', list.isPlaceholderData && 'opacity-60')}>
              <div className={cn('grid gap-3', twoColumns ? 'grid-cols-2' : 'grid-cols-1')}>
                {items.map((p) => (
                  <PromoCard key={p.id} promo={p} tariffName={tariffName(p)} actions={actions} />
                ))}
              </div>
              {totalPages > 1 && (
                <div className="mt-4 flex items-center justify-center gap-3 text-sm text-muted-foreground">
                  <Pager page={page} totalPages={totalPages} onPage={(p) => go(p < page ? 'prev' : 'next', { page: p })} busyDir={busyCtl === 'prev' || busyCtl === 'next' ? busyCtl : null} compact />
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <PromoDetailSheet
        promo={detailShown}
        tariffName={detailShown ? tariffName(detailShown) : null}
        open={detail != null && dialog == null}
        onClose={() => setDetail(null)}
        onEdit={actions.onEdit}
        onUsed={actions.onUsed}
        onDuplicate={actions.onDuplicate}
        onDelete={actions.onDelete}
        onToggled={actions.onToggled}
        onError={showError}
      />

      {formShown && (
        <PromoFormModal
          open={dialog?.kind === 'form'}
          mode={formShown}
          onClose={closeDialog}
          onSaved={(kind) => {
            closeDialog()
            showSuccess(kind === 'created' ? t('admin.promos.createdToast') : t('admin.feedback.saved'))
          }}
          tariffs={tariffs}
          salesModeTariffs={salesModeTariffs}
          loyaltyEnabled={loyaltyEnabled}
        />
      )}

      <PromoUsedByModal key={usedShown?.id ?? 'none'} promo={usedShown} open={dialog?.kind === 'used'} onClose={closeDialog} />

      <PromoDeleteModal
        promo={deleteShown}
        open={dialog?.kind === 'delete'}
        onClose={closeDialog}
        onDeleted={() => {
          closeDialog()
          setDetail(null)
          showSuccess(t('admin.promos.deletedToast'))
        }}
        onDisabled={() => {
          closeDialog()
          showSuccess(t('admin.promos.disabledToast'))
        }}
        onError={showError}
      />
    </AdminLayout>
  )
}

const secondaryBtn =
  'inline-flex h-10 items-center gap-2 rounded-[12px] border border-border px-4 text-sm font-semibold transition-colors hover:bg-muted'

function SpinnerIcon() {
  return <span className="cabinet-btn-spinner box-border inline-block size-4 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent" aria-hidden />
}

function Pager({
  page,
  totalPages,
  onPage,
  compact,
  busyDir,
}: {
  page: number
  totalPages: number
  onPage: (p: number) => void
  compact?: boolean
  busyDir?: 'prev' | 'next' | null
}) {
  const { t } = useTranslation()
  const btn =
    'inline-flex size-9 items-center justify-center rounded-[10px] border border-border text-foreground transition-colors hover:bg-primary/10 disabled:opacity-40'
  return (
    <span className="flex items-center gap-2">
      <button type="button" aria-label={t('admin.prev')} disabled={page <= 1 || busyDir != null} onClick={() => onPage(page - 1)} className={btn}>
        {busyDir === 'prev' ? <SpinnerIcon /> : <ChevronLeft className="size-4" />}
      </button>
      {compact && (
        <span className="min-w-12 text-center tabular-nums">
          {page} / {totalPages}
        </span>
      )}
      <button type="button" aria-label={t('admin.next')} disabled={page >= totalPages || busyDir != null} onClick={() => onPage(page + 1)} className={btn}>
        {busyDir === 'next' ? <SpinnerIcon /> : <ChevronRight className="size-4" />}
      </button>
    </span>
  )
}

function EmptyState({ icon, title, action }: { icon: ReactNode; title: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-[18px] border border-border/60 bg-card px-6 py-12 text-center">
      <span className="inline-flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">{icon}</span>
      <p className="text-sm font-semibold">{title}</p>
      {action}
    </div>
  )
}

function ListSkeleton({ layout }: { layout: 'table' | 'cards' }) {
  if (layout === 'table') {
    return (
      <div className="overflow-hidden rounded-[18px] border border-border/60 bg-card">
        <PromoTableHeader />
        {Array.from({ length: 6 }, (_, i) => (
          <div key={i} className="grid min-h-[68px] items-center gap-4 border-t border-border/60 px-5" style={{ gridTemplateColumns: PROMO_TABLE_COLUMNS }}>
            <Skeleton className="h-4 w-24 rounded-md" />
            <div className="flex items-center gap-3">
              <Skeleton className="size-[34px] rounded-[10px]" />
              <Skeleton className="h-4 w-36 rounded-md" />
            </div>
            <Skeleton className="h-3 w-full rounded-md" />
            <Skeleton className="h-3 w-20 rounded-md" />
            <Skeleton className="h-5 w-20 rounded-full" />
            <Skeleton className="h-7 w-12 rounded-full" />
            <Skeleton className="size-[34px] rounded-[10px]" />
          </div>
        ))}
      </div>
    )
  }
  return (
    <div className="grid gap-3">
      {Array.from({ length: 4 }, (_, i) => (
        <Skeleton key={i} className="h-[136px] rounded-[18px]" />
      ))}
    </div>
  )
}
