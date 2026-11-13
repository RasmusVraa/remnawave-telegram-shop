import { useState, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { CreditCard, Download, Search, ChevronRight } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminTablePagination } from '../components/AdminTablePagination'
import { HeaderFilter } from '../components/HeaderFilter'
import { SortableTh, nextSort, type SortDir, type SortState } from '../components/SortableTh'
import { AdminPaymentDetailModal } from '../components/AdminPaymentDetailModal'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import { useAdminPaymentsList, type AdminPaymentListItemDTO } from '../hooks/useAdminPayments'
import { formatInvoiceType } from '../utils/formatInvoiceType'
import { formatAdminDateTime } from '../utils/datetime'
import { formatPaymentAmount } from '../utils/formatPaymentAmount'
import { paymentStatusInfo, describePayment } from '../utils/paymentStatus'

const STATUSES = ['all', 'paid', 'pending', 'new', 'cancel'] as const
type StatusScope = (typeof STATUSES)[number]

const PAGE_LIMIT = 20

const STATUS_LABEL_KEYS: Record<StatusScope, string> = {
  all: 'admin.payments.scopeAll',
  paid: 'admin.payments.status.paid',
  pending: 'admin.payments.status.pending',
  new: 'admin.payments.status.new',
  cancel: 'admin.payments.status.cancel',
}

function PaymentRow({
  item,
  onClick,
  t,
}: {
  item: AdminPaymentListItemDTO
  onClick: () => void
  t: ReturnType<typeof useTranslation>['t']
}) {
  const status = paymentStatusInfo(item.status, t)
  const amount = formatPaymentAmount(item.amount, item.currency || '', item.invoice_type)
  const username = item.telegram_username ? `@${item.telegram_username}` : (item.panel_login ?? `#${item.customer_id}`)

  return (
    <tr
      onClick={onClick}
      className="cursor-pointer border-b border-border/40 transition-colors hover:bg-accent/50 last:border-0"
    >
      <td className="w-[1%] whitespace-nowrap px-3 py-2.5 text-sm font-mono tabular-nums">#{item.id}</td>
      <td className="max-w-[9rem] truncate px-3 py-2.5 text-sm" title={username}>{username}</td>
      <td className="whitespace-nowrap px-3 py-2.5 text-sm font-mono tabular-nums">{amount.text}</td>
      <td className="hidden px-3 py-2.5 text-sm sm:table-cell">{formatInvoiceType(item.invoice_type, t)}</td>
      <td className="hidden max-w-[12rem] truncate px-3 py-2.5 text-sm md:table-cell" title={describePayment(item, t)}>
        {describePayment(item, t)}
      </td>
      <td className="px-3 py-2.5 text-sm">
        <span className={cn('inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium', status.cls)}>
          {status.label}
        </span>
      </td>
      <td className="hidden whitespace-nowrap px-3 py-2.5 text-sm text-muted-foreground lg:table-cell">
        {formatAdminDateTime(item.created_at)}
      </td>
    </tr>
  )
}

function PaymentMobileCard({
  item,
  onClick,
  t,
}: {
  item: AdminPaymentListItemDTO
  onClick: () => void
  t: ReturnType<typeof useTranslation>['t']
}) {
  const status = paymentStatusInfo(item.status, t)
  const amount = formatPaymentAmount(item.amount, item.currency || '', item.invoice_type)
  const username = item.telegram_username ? `@${item.telegram_username}` : (item.panel_login ?? `#${item.customer_id}`)

  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center justify-between gap-3 rounded-lg border border-border/60 bg-card px-4 py-3 text-left transition-colors hover:bg-accent/40 active:bg-accent/60"
    >
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold">{username}</p>
        <p className="truncate text-xs text-muted-foreground">
          {amount.text} · {formatAdminDateTime(item.created_at)}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <span className={cn('inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium', status.cls)}>
          {status.label}
        </span>
        <ChevronRight className="size-4 text-muted-foreground" />
      </div>
    </button>
  )
}

/** Колонки, по которым умеет сортировать сервер; «Описание» собирается на лету и не сортируется. */
const SORT_FIRST_DIR: Record<string, SortDir> = {
  id: 'desc',
  user: 'asc',
  amount: 'desc',
  provider: 'asc',
  status: 'asc',
  created_at: 'desc',
}

export default function AdminPaymentsPage() {
  const { t } = useTranslation()

  const [scope, setScope] = useState<StatusScope>('all')
  const [page, setPage] = useState(1)
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [exporting, setExporting] = useState(false)
  const [exportError, setExportError] = useState<string | null>(null)
  const [sort, setSort] = useState<SortState | null>(null)

  const onSort = useCallback((key: string) => {
    setSort((cur) => nextSort(cur, key, SORT_FIRST_DIR[key] ?? 'asc'))
    setPage(1)
  }, [])

  const debounceRef = useMemo(() => ({ timer: null as ReturnType<typeof setTimeout> | null }), [])
  const onSearchChange = useCallback(
    (val: string) => {
      setSearchQuery(val)
      if (debounceRef.timer) clearTimeout(debounceRef.timer)
      debounceRef.timer = setTimeout(() => {
        setDebouncedSearch(val)
        setPage(1)
      }, 350)
    },
    [debounceRef],
  )

  const statusParam = scope === 'all' ? '' : scope
  const { data, isLoading, isError } = useAdminPaymentsList({
    status: statusParam,
    q: debouncedSearch,
    page,
    limit: PAGE_LIMIT,
    sort: sort?.key,
    dir: sort?.dir,
  })

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_LIMIT))

  const handleExport = async () => {
    setExporting(true)
    setExportError(null)
    try {
      await api.adminPaymentsExportCsv({ status: statusParam, q: debouncedSearch, sort: sort?.key, dir: sort?.dir })
    } catch {
      setExportError(t('admin.errors.requestFailed'))
    } finally {
      setExporting(false)
    }
  }

  return (
    <AdminLayout>
      <div className="space-y-4">
        <AdminPageHeader
          icon={CreditCard}
          title={t('admin.payments.title')}
          subtitle={total > 0 ? t('admin.payments.totalCount', { count: total }) : t('admin.payments.subtitle')}
          accent="emerald"
          actions={
            <button
              type="button"
              onClick={() => void handleExport()}
              disabled={exporting}
              className={cn('inline-flex items-center gap-1.5 rounded-md border border-input bg-background px-3 py-1.5 text-sm font-medium shadow-sm transition-colors hover:bg-accent disabled:pointer-events-none disabled:opacity-60', exporting && '!opacity-100')}
            >
              {exporting ? <Spinner /> : <Download className="size-4" />}
              CSV
            </button>
          }
        />

        {exportError && (
          <p className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {exportError}
          </p>
        )}

        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder={t('admin.payments.searchPlaceholder')}
            className="h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          />
        </div>

        <div className="-mx-1 overflow-x-auto overscroll-x-contain px-1 pb-0.5 md:hidden">
          <div className="inline-flex min-w-full gap-1 rounded-lg border border-border/50 bg-card/50 p-1 sm:min-w-0 sm:w-full">
            {STATUSES.map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => { setScope(s); setPage(1) }}
                className={cn(
                  'min-h-9 shrink-0 rounded-md px-3 py-2 text-center text-sm font-medium transition-colors sm:flex-1',
                  scope === s
                    ? 'bg-primary/10 text-primary dark:bg-primary/20'
                    : 'text-foreground/80 hover:bg-accent hover:text-foreground',
                )}
              >
                {t(STATUS_LABEL_KEYS[s])}
              </button>
            ))}
          </div>
        </div>

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
              {t('admin.payments.listEmpty')}
            </div>
          ) : (
            <>
              <div className="space-y-2 p-3 md:hidden">
                {items.map((it) => (
                  <PaymentMobileCard key={it.id} item={it} t={t} onClick={() => setSelectedId(it.id)} />
                ))}
              </div>
              <div className="hidden overflow-x-auto md:block">
                <table className="w-full text-left">
                  <thead>
                    <tr className="border-b border-border bg-muted/40 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                      <SortableTh sortKey="id" sort={sort} onSort={onSort} className="w-[1%] whitespace-nowrap">ID</SortableTh>
                      <SortableTh sortKey="user" sort={sort} onSort={onSort}>{t('admin.users.username')}</SortableTh>
                      <SortableTh sortKey="amount" sort={sort} onSort={onSort}>{t('admin.payments.amount')}</SortableTh>
                      <SortableTh sortKey="provider" sort={sort} onSort={onSort} className="hidden sm:table-cell">{t('admin.payments.provider')}</SortableTh>
                      <th className="hidden px-3 py-2 md:table-cell">{t('admin.payments.description')}</th>
                      <SortableTh
                        sortKey="status"
                        sort={sort}
                        onSort={onSort}
                        filter={
                          <HeaderFilter
                            value={scope}
                            options={STATUSES.map((s) => ({ value: s, label: t(STATUS_LABEL_KEYS[s]) }))}
                            onChange={(s) => { setScope(s); setPage(1) }}
                            ariaLabel={t('admin.payments.statusLabel')}
                          />
                        }
                      >
                        {t('admin.payments.statusLabel')}
                      </SortableTh>
                      <SortableTh sortKey="created_at" sort={sort} onSort={onSort} className="hidden lg:table-cell">{t('admin.payments.createdAt')}</SortableTh>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((it) => (
                      <PaymentRow key={it.id} item={it} t={t} onClick={() => setSelectedId(it.id)} />
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          <AdminTablePagination
            page={page}
            totalPages={totalPages}
            onPageChange={setPage}
            className="flex items-center justify-between border-t border-border px-3 py-2"
          />
        </Card>
      </div>

      <AdminPaymentDetailModal paymentId={selectedId} onClose={() => setSelectedId(null)} />
    </AdminLayout>
  )
}
