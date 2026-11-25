import { useState, useMemo, useCallback, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  AtSign,
  CalendarClock,
  ChevronRight,
  CircleDot,
  ListFilter,
  Search,
  Send,
  Users,
  X,
  Zap,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminModal } from '../components/AdminModal'
import { AdminLoadMore } from '../components/AdminLoadMore'
import { AdminFilterMenuDivider, AdminFilterMenuItem, AdminFilterMenuTitle } from '../components/AdminFilterMenu'
import { AdminUserAvatar } from '../components/overview/AdminUserAvatar'
import { HeaderFilter } from '../components/HeaderFilter'
import { SortableTh, nextSort, type SortDir, type SortState } from '../components/SortableTh'
import type { RwIconTone } from '../utils/rwStatusStyles'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import {
  useAdminUsersInfinite,
  useAdminUserSearch,
  type AdminCustomerDTO,
} from '../hooks/useAdminUsers'
import { useAdminTariffList, type AdminTariff } from '../hooks/useAdminTariffs'
import { resolveTariffLabel } from '../utils/resolveTariffLabel'

/**
 * Подпись колонки со значком.
 *
 * Значок здесь не украшение: шапка набрана капителью в один вес, и глазу не за
 * что зацепиться при возврате к таблице. Иконка даёт колонке силуэт, по
 * которому её находят быстрее, чем по слову.
 */
function HeadCell({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <Icon className="size-3.5 shrink-0 text-muted-foreground/70" aria-hidden />
      {label}
    </span>
  )
}

const SCOPES = ['all', 'active', 'expiring', 'trial', 'expired', 'inactive'] as const
type Scope = (typeof SCOPES)[number]

/** Сколько клиентов приходит за один запрос: на строку уходит один лёгкий запрос на оплаты, 50 — без ощутимой нагрузки. */
const PAGE_LIMIT = 50

/** Колонки, по которым сортирует сервер, и направление первого клика по ним. */
const SORT_FIRST_DIR: Record<string, SortDir> = {
  id: 'desc',
  telegram_id: 'asc',
  username: 'asc',
  expire_at: 'asc',
  status: 'asc',
  tariff: 'asc',
}

function parseSort(key: string | null, dir: string | null): SortState | null {
  if (!key || !(key in SORT_FIRST_DIR)) return null
  return { key, dir: dir === 'desc' ? 'desc' : 'asc' }
}

function statusBadge(status: string, t: (k: string) => string) {
  const map: Record<string, { labelKey: string; cls: string }> = {
    active: { labelKey: 'admin.users.statusActive', cls: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' },
    expired: { labelKey: 'admin.users.statusExpired', cls: 'bg-red-500/15 text-red-700 dark:text-red-400' },
    trial: { labelKey: 'admin.users.statusTrial', cls: 'bg-amber-500/15 text-amber-700 dark:text-amber-400' },
    inactive: { labelKey: 'admin.users.statusInactive', cls: 'bg-muted text-muted-foreground' },
    disabled: { labelKey: 'admin.users.rwStatusDisabled', cls: 'bg-red-500/15 text-red-700 dark:text-red-400' },
  }
  const entry = map[status]
  const label = entry ? t(entry.labelKey) : status
  const cls = entry?.cls ?? 'bg-muted text-muted-foreground'
  return (
    <span className={cn('inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium', cls)}>
      {label}
    </span>
  )
}

function formatDate(iso?: string | null): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit' })
  } catch {
    return iso
  }
}

/**
 * Триал в списке — не статус, а «тариф»: сверху обычный Активен, снизу Триал.
 * Серверный `trial` бывает только у действующей подписки, поэтому истёкший
 * триал сюда приходит уже как `expired`.
 */
function statusView(user: AdminCustomerDTO): { status: string; trial: boolean } {
  return user.status === 'trial' ? { status: 'active', trial: true } : { status: user.status, trial: false }
}

function avatarTone(status: string): RwIconTone {
  if (status === 'active') return 'success'
  if (status === 'expired' || status === 'disabled') return 'danger'
  return 'default'
}

function userLabel(user: AdminCustomerDTO): string {
  if (user.telegram_username) return `@${user.telegram_username}`
  return user.panel_login || `#${user.id}`
}

/** Цвет даты окончания: прошла или осталось до 3 дней — красный, до 7 — жёлтый. */
function expireTone(iso?: string | null): string {
  if (!iso) return 'text-muted-foreground'
  const days = (new Date(iso).getTime() - Date.now()) / 86_400_000
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-amber-600 dark:text-amber-400'
  return 'text-muted-foreground'
}

const TRIAL_PILL = 'bg-amber-500/15 text-amber-700 dark:text-amber-400'

function UserRow({
  user,
  onClick,
  t,
  tariffs,
}: {
  user: AdminCustomerDTO
  onClick: () => void
  t: (k: string) => string
  tariffs?: AdminTariff[]
}) {
  const view = statusView(user)
  const tariffLabel = view.trial ? t('admin.users.statusTrial') : resolveTariffLabel(user.current_tariff_id, undefined, tariffs)

  return (
    <tr
      onClick={onClick}
      className="cursor-pointer border-b border-border/40 transition-colors hover:bg-accent/50 last:border-0"
    >
      <td className="w-[1%] py-2 pl-4 pr-2">
        <AdminUserAvatar url={user.avatar_url} name={userLabel(user)} tone={avatarTone(view.status)} size="sm" />
      </td>
      <td className="w-[7rem] max-w-[7rem] truncate px-3 py-2.5 text-sm font-mono tabular-nums" title={String(user.telegram_id)}>
        {user.telegram_id}
      </td>
      <td className="max-w-[9rem] truncate px-3 py-2.5 text-sm" title={user.telegram_username ? `@${user.telegram_username}` : undefined}>
        {user.telegram_username ? (
          <span className="text-foreground">@{user.telegram_username}</span>
        ) : user.panel_login ? (
          /* У web-клиента нет @username, зато есть логин профиля панели —
             до первой покупки это единственная человекочитаемая подпись. */
          <span className="font-mono text-foreground/80">{user.panel_login}</span>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      <td className="hidden px-3 py-2.5 text-sm sm:table-cell">{formatDate(user.expire_at)}</td>
      <td className="px-3 py-2.5 text-sm">{statusBadge(view.status, t)}</td>
      <td
        className={cn(
          'hidden max-w-[8rem] truncate px-3 py-2.5 text-sm md:table-cell',
          view.trial && 'text-amber-700 dark:text-amber-400',
        )}
        title={tariffLabel ?? undefined}
      >
        {tariffLabel ?? '—'}
      </td>
    </tr>
  )
}

/**
 * Карточка на телефоне: слева аватарка, имя и дата окончания; справа статус и
 * тариф друг под другом — на тех же двух строках, что имя и дата.
 */
function UserMobileCard({
  user,
  onClick,
  t,
  tariffs,
}: {
  user: AdminCustomerDTO
  onClick: () => void
  t: (k: string) => string
  tariffs?: AdminTariff[]
}) {
  const view = statusView(user)
  const displayName = userLabel(user)
  const tariffLabel = view.trial ? t('admin.users.statusTrial') : resolveTariffLabel(user.current_tariff_id, undefined, tariffs)

  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-3 rounded-[14px] border border-border/60 bg-card px-3 py-2.5 text-left transition-colors hover:bg-accent/40 active:bg-accent/60"
    >
      <AdminUserAvatar
        url={user.avatar_url}
        name={displayName}
        tone={avatarTone(view.status)}
        size="sm"
        className="m-1"
      />
      <div className="min-w-0 flex-1">
        <p className="truncate text-[15px] font-semibold leading-5">{displayName}</p>
        <p className={cn('mt-1 text-xs leading-5 tabular-nums', expireTone(user.expire_at))}>
          {user.expire_at ? `${t('admin.users.until')} ${formatDate(user.expire_at)}` : '—'}
        </p>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-1">
        {statusBadge(view.status, t)}
        {tariffLabel ? (
          <span
            className={cn(
              'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
              view.trial ? TRIAL_PILL : 'bg-muted text-muted-foreground',
            )}
          >
            {tariffLabel}
          </span>
        ) : (
          <span className="h-5" aria-hidden />
        )}
      </div>
      <ChevronRight className="-mr-1 size-4 shrink-0 text-muted-foreground" />
    </button>
  )
}

const SCOPE_LABEL_KEYS: Record<Scope, string> = {
  all: 'admin.users.scopeAll',
  active: 'admin.users.scopeActive',
  inactive: 'admin.users.scopeInactive',
  expiring: 'admin.users.scopeExpiring',
  trial: 'admin.users.scopeTrial',
  expired: 'admin.users.scopeExpired',
}

/**
 * Сортировки шторки на телефоне. Тот же серверный порядок, что у заголовков
 * таблицы на ПК, поэтому выбранное в шторке сохраняется при смене экрана.
 */
const SHEET_SORTS = [
  { id: 'def', sort: null },
  { id: 'expAsc', sort: { key: 'expire_at', dir: 'asc' } },
  { id: 'expDesc', sort: { key: 'expire_at', dir: 'desc' } },
  { id: 'name', sort: { key: 'username', dir: 'asc' } },
  { id: 'tariff', sort: { key: 'tariff', dir: 'asc' } },
  { id: 'status', sort: { key: 'status', dir: 'asc' } },
] as const satisfies ReadonlyArray<{ id: string; sort: SortState | null }>

function sheetSortId(sort: SortState | null): string {
  const hit = SHEET_SORTS.find((s) => (s.sort ? s.sort.key === sort?.key && s.sort.dir === sort?.dir : !sort))
  return hit?.id ?? 'def'
}

/**
 * Где мы стояли в списке: вкладка, страница и поисковый запрос.
 *
 * Живёт в адресе, а не в состоянии компонента: админ уходит в карточку
 * пользователя и возвращается кнопкой «назад», и без этого список каждый раз
 * открывался с первой страницы и пустым поиском — найденного человека
 * приходилось искать заново.
 *
 * Дубль в sessionStorage нужен для второго пути возврата: по хлебным крошкам
 * и по пункту меню адрес чистый, а вернуться человек ожидает туда же.
 */
const LIST_STATE_KEY = 'admin-users-list-state'

interface ListState {
  scope: Scope
  q: string
  sort: SortState | null
}

function readStoredListState(): ListState | null {
  try {
    const raw = sessionStorage.getItem(LIST_STATE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<ListState>
    const scope = SCOPES.includes(parsed.scope as Scope) ? (parsed.scope as Scope) : 'all'
    const sort = parseSort(parsed.sort?.key ?? null, parsed.sort?.dir ?? null)
    return { scope, q: typeof parsed.q === 'string' ? parsed.q : '', sort }
  } catch {
    // Приватный режим или переполненное хранилище — не повод ломать страницу.
    return null
  }
}

export default function AdminUsersPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const paramScope = searchParams.get('scope')
  const scope: Scope = SCOPES.includes(paramScope as Scope) ? (paramScope as Scope) : 'all'
  const urlQuery = searchParams.get('q') ?? ''
  const sortKeyParam = searchParams.get('sort')
  const sortDirParam = searchParams.get('dir')
  const sort = useMemo(() => parseSort(sortKeyParam, sortDirParam), [sortKeyParam, sortDirParam])

  const [searchQuery, setSearchQuery] = useState(urlQuery)
  const [debouncedSearch, setDebouncedSearch] = useState(urlQuery)

  // Состояние списка меняем через replace: перелистывание и набор в поиске не
  // должны копиться в истории — «назад» из карточки обязан выводить к списку,
  // а не отматывать страницы по одной.
  const applyListState = useCallback(
    (next: Partial<ListState>) => {
      const state: ListState = {
        scope: next.scope ?? scope,
        q: next.q ?? urlQuery,
        sort: next.sort === undefined ? sort : next.sort,
      }
      const params: Record<string, string> = {}
      if (state.scope !== 'all') params.scope = state.scope
      if (state.q.trim()) params.q = state.q
      if (state.sort) {
        params.sort = state.sort.key
        params.dir = state.sort.dir
      }
      setSearchParams(params, { replace: true })
      try {
        sessionStorage.setItem(LIST_STATE_KEY, JSON.stringify(state))
      } catch {
        /* ignore */
      }
    },
    [scope, setSearchParams, sort, urlQuery],
  )

  // Возврат по крошкам или через меню: адрес чистый, восстанавливаем из памяти.
  useEffect(() => {
    if (searchParams.toString() !== '') return
    const stored = readStoredListState()
    if (!stored || (stored.scope === 'all' && !stored.q && !stored.sort)) return
    setSearchQuery(stored.q)
    setDebouncedSearch(stored.q)
    applyListState(stored)
    // Только на первый заход с пустым адресом.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Адрес мог поменяться снаружи — кнопкой «назад» браузера.
  useEffect(() => {
    setSearchQuery(urlQuery)
    setDebouncedSearch(urlQuery)
  }, [urlQuery])

  const [sheetOpen, setSheetOpen] = useState(false)
  const isSearching = debouncedSearch.trim().length > 0
  const activeSortId = sheetSortId(sort)
  const filtersCount = (scope !== 'all' ? 1 : 0) + (activeSortId !== 'def' ? 1 : 0)

  const listQuery = useAdminUsersInfinite({ scope, limit: PAGE_LIMIT, sort: sort?.key, dir: sort?.dir })
  const searchResult = useAdminUserSearch(debouncedSearch)
  const { data: tariffs } = useAdminTariffList()

  const debounceRef = useMemo(() => ({ timer: null as ReturnType<typeof setTimeout> | null }), [])

  const onSearchChange = useCallback(
    (val: string) => {
      setSearchQuery(val)
      if (debounceRef.timer) clearTimeout(debounceRef.timer)
      debounceRef.timer = setTimeout(() => {
        setDebouncedSearch(val)
        applyListState({ q: val })
      }, 350)
    },
    [applyListState, debounceRef],
  )

  // Поиск отдаёт до 24 совпадений в порядке релевантности — сортировать там нечего.
  const onSort = useCallback(
    (key: string) => applyListState({ sort: nextSort(sort, key, SORT_FIRST_DIR[key] ?? 'asc') }),
    [applyListState, sort],
  )

  // Новый клиент между запросами сдвигает страницы, и строка может прийти дважды.
  const listItems = useMemo(() => {
    const seen = new Set<number>()
    return (listQuery.data?.pages ?? []).flatMap((p) => p.items).filter((u) => !seen.has(u.id) && seen.add(u.id))
  }, [listQuery.data])
  const items = isSearching ? (searchResult.data?.items ?? []) : listItems
  const total = isSearching ? items.length : (listQuery.data?.pages[0]?.total ?? 0)
  const isLoading = isSearching ? searchResult.isLoading : listQuery.isLoading
  const isError = isSearching ? searchResult.isError : listQuery.isError

  return (
    <AdminLayout>
      <div className="space-y-4">
        <AdminPageHeader
          icon={Users}
          title={t('admin.users.title')}
          subtitle={total > 0 ? t('admin.users.totalCount', { count: total }) : t('admin.users.subtitle')}
          accent="violet"
        />

        {/* Search + на телефоне кнопка «Сортировка и фильтр» (на ПК это шапка таблицы) */}
        <div className="flex items-center gap-2">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={t('admin.users.searchPlaceholder')}
              className="h-10 w-full rounded-xl border border-input bg-background pl-9 pr-3 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring md:h-9 md:rounded-md"
            />
          </div>
          <button
            type="button"
            disabled={isSearching}
            aria-label={t('admin.users.sortAndFilter')}
            onClick={() => setSheetOpen(true)}
            className={cn(
              'relative inline-flex h-10 shrink-0 items-center justify-center gap-1.5 rounded-xl border px-3 text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-50 md:hidden',
              filtersCount > 0
                ? 'border-primary/50 bg-primary/10 text-primary'
                : 'border-border bg-card hover:bg-secondary/80',
            )}
          >
            <ListFilter className="size-4" aria-hidden />
            {filtersCount > 0 && (
              <span className="flex min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[11px] font-bold leading-4 text-primary-foreground">
                {filtersCount}
              </span>
            )}
          </button>
        </div>

        {/* Выбранное в шторке — плашками, нажал и сбросил */}
        {!isSearching && filtersCount > 0 && (
          <div className="flex flex-wrap gap-1.5 md:hidden">
            {scope !== 'all' && (
              <button
                type="button"
                onClick={() => applyListState({ scope: 'all' })}
                className="inline-flex h-7 items-center gap-1 rounded-full border border-primary/45 bg-primary/10 pl-3 pr-1.5 text-xs font-medium text-primary"
              >
                {t(SCOPE_LABEL_KEYS[scope])}
                <X className="size-3.5" aria-hidden />
              </button>
            )}
            {activeSortId !== 'def' && (
              <button
                type="button"
                onClick={() => applyListState({ sort: null })}
                className="inline-flex h-7 items-center gap-1 rounded-full border border-primary/45 bg-primary/10 pl-3 pr-1.5 text-xs font-medium text-primary"
              >
                {t(`admin.users.sort.${activeSortId}`)}
                <X className="size-3.5" aria-hidden />
              </button>
            )}
          </div>
        )}

        <AdminModal
          open={sheetOpen}
          onClose={() => setSheetOpen(false)}
          title={t('admin.users.sortAndFilter')}
          footer={
            <div className="flex w-full gap-2">
              <Button
                type="button"
                variant="outline"
                className="flex-1"
                disabled={filtersCount === 0}
                onClick={() => applyListState({ scope: 'all', sort: null })}
              >
                {t('admin.users.reset')}
              </Button>
              <Button type="button" className="flex-1" onClick={() => setSheetOpen(false)}>
                {t('admin.users.showResult')}
              </Button>
            </div>
          }
        >
          <AdminFilterMenuTitle>{t('admin.users.sortTitle')}</AdminFilterMenuTitle>
          {SHEET_SORTS.map((s) => (
            <AdminFilterMenuItem
              key={s.id}
              active={activeSortId === s.id}
              onClick={() => applyListState({ sort: s.sort })}
            >
              {t(`admin.users.sort.${s.id}`)}
            </AdminFilterMenuItem>
          ))}
          <AdminFilterMenuDivider />
          <AdminFilterMenuTitle>{t('admin.users.showTitle')}</AdminFilterMenuTitle>
          {SCOPES.map((s) => (
            <AdminFilterMenuItem
              key={s}
              active={scope === s}
              onClick={() => applyListState({ scope: s })}
            >
              {t(SCOPE_LABEL_KEYS[s])}
            </AdminFilterMenuItem>
          ))}
        </AdminModal>

        {/* Table */}
        <Card className="overflow-hidden">
          {isLoading ? (
            <div className="flex items-center justify-center py-12">
              <span className="size-6 rounded-full border-2 border-primary border-t-transparent animate-spin" />
            </div>
          ) : isError ? (
            <div className="py-12 text-center text-sm text-destructive">
              {t('common.error', 'Ошибка загрузки')}
            </div>
          ) : items.length === 0 ? (
            <div className="py-12 text-center text-sm text-muted-foreground">
              {isSearching ? t('admin.users.searchEmpty') : t('admin.users.listEmpty')}
            </div>
          ) : (
            <>
              <div className="space-y-2 p-3 md:hidden">
                {items.map((u) => (
                  <UserMobileCard
                    key={u.id}
                    user={u}
                    t={t}
                    tariffs={tariffs}
                    onClick={() => navigate(`/admin/users/${u.id}`)}
                  />
                ))}
              </div>
              <div className="hidden overflow-x-auto md:block">
                <table className="w-full text-left">
                  <thead>
                    <tr className="border-b border-border bg-muted/40 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                      {/* Колонка аватарок: вместо внутреннего ID, без сортировки (порядок «новые» — по умолчанию). */}
                      <th className="w-[1%] px-2" aria-label={t('admin.users.avatar')} />
                      <SortableTh sortKey="telegram_id" sort={sort} onSort={onSort} disabled={isSearching} className="w-[9rem] max-w-[9rem]">
                        <HeadCell icon={Send} label={t('admin.users.telegramId')} />
                      </SortableTh>
                      <SortableTh sortKey="username" sort={sort} onSort={onSort} disabled={isSearching}>
                        <HeadCell icon={AtSign} label={t('admin.users.username')} />
                      </SortableTh>
                      <SortableTh sortKey="expire_at" sort={sort} onSort={onSort} disabled={isSearching} className="hidden sm:table-cell">
                        <HeadCell icon={CalendarClock} label={t('admin.users.expireAt')} />
                      </SortableTh>
                      <SortableTh
                        sortKey="status"
                        sort={sort}
                        onSort={onSort}
                        disabled={isSearching}
                        filter={
                          <HeaderFilter
                            value={scope}
                            options={SCOPES.map((s) => ({ value: s, label: t(SCOPE_LABEL_KEYS[s]) }))}
                            onChange={(s) => applyListState({ scope: s })}
                            ariaLabel={t('admin.users.status')}
                          />
                        }
                      >
                        <HeadCell icon={CircleDot} label={t('admin.users.status')} />
                      </SortableTh>
                      <SortableTh sortKey="tariff" sort={sort} onSort={onSort} disabled={isSearching} className="hidden md:table-cell">
                        <HeadCell icon={Zap} label={t('admin.users.tariff')} />
                      </SortableTh>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((u) => (
                      <UserRow
                        key={u.id}
                        user={u}
                        t={t}
                        tariffs={tariffs}
                        onClick={() => navigate(`/admin/users/${u.id}`)}
                      />
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          {/* Подгрузка: две страницы сами по достижении низа, дальше кнопка */}
          {!isSearching && !isLoading && (
            <AdminLoadMore
              hasMore={Boolean(listQuery.hasNextPage)}
              loading={listQuery.isFetchingNextPage}
              onLoadMore={() => void listQuery.fetchNextPage()}
              resetKey={`${scope}|${sort?.key ?? ''}|${sort?.dir ?? ''}`}
            />
          )}
        </Card>
      </div>
    </AdminLayout>
  )
}
