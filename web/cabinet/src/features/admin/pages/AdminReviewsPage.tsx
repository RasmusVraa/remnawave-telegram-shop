import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, EyeOff, Eye, Pencil, Plus, Reply, Search, Star, Trash2 } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Card } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { Stars } from '@/features/reviews/StarRating'
import type { AdminReview, ReviewStatus } from '@/features/reviews/reviewTypes'
import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminTablePagination } from '../components/AdminTablePagination'
import { AdminConfirmModal } from '../components/AdminConfirmModal'
import { AdminFeedback } from '../components/AdminFeedback'
import { AdminReviewModal } from '../components/AdminReviewModal'
import { useAdminMutationFeedback } from '../hooks/useAdminMutationFeedback'
import { useAdminReviewsList, useAdminReviewsPending, useDeleteAdminReview, useUpdateAdminReview } from '../hooks/useAdminReviews'
import { formatAdminDateTime } from '../utils/datetime'

const TABS = ['all', 'pending', 'published', 'hidden'] as const
type Tab = (typeof TABS)[number]
const TAB_KEY: Record<Tab, string> = {
  all: 'admin.reviews.tabAll',
  pending: 'admin.reviews.tabPending',
  published: 'admin.reviews.tabPublished',
  hidden: 'admin.reviews.tabHidden',
}
const PAGE_LIMIT = 20

const STATUS_STYLE: Record<ReviewStatus, string> = {
  pending: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  published: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
  hidden: 'bg-muted text-muted-foreground',
}

function authorLine(r: AdminReview, anonymous: string): string {
  return r.author_name?.trim() || r.display_name.trim() || anonymous
}

function IconAction({
  label,
  onClick,
  tone,
  disabled,
  children,
}: {
  label: string
  onClick: () => void
  tone?: 'ok' | 'warn' | 'bad'
  disabled?: boolean
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation()
        onClick()
      }}
      className={cn(
        'inline-flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-secondary text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-50 dark:shadow-none',
        tone === 'ok' && 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20 dark:text-emerald-400',
        tone === 'warn' && 'border-amber-500/30 bg-amber-500/10 text-amber-600 hover:bg-amber-500/20 dark:text-amber-400',
        tone === 'bad' && 'border-rose-500/30 bg-rose-500/10 text-rose-600 hover:bg-rose-500/20 dark:text-rose-400',
      )}
    >
      {children}
    </button>
  )
}

export default function AdminReviewsPage() {
  const { t } = useTranslation()
  // null — вкладку ещё не выбирали: открываем «На проверке», если там есть что проверять, иначе «Все».
  const [pickedTab, setTab] = useState<Tab | null>(null)
  const pendingCounter = useAdminReviewsPending()
  const tab: Tab = pickedTab ?? (pendingCounter.data && pendingCounter.data.counts.pending === 0 ? 'all' : 'pending')
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [debounced, setDebounced] = useState('')
  const [editing, setEditing] = useState<AdminReview | null>(null)
  const [focusReply, setFocusReply] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<AdminReview | null>(null)
  const debounce = useMemo(() => ({ timer: null as ReturnType<typeof setTimeout> | null }), [])

  const { feedback, clear, showSuccess, showError } = useAdminMutationFeedback()
  const update = useUpdateAdminReview()
  const del = useDeleteAdminReview()

  const onSearch = useCallback(
    (v: string) => {
      setSearch(v)
      if (debounce.timer) clearTimeout(debounce.timer)
      debounce.timer = setTimeout(() => {
        setDebounced(v)
        setPage(1)
      }, 350)
    },
    [debounce],
  )

  const { data, isLoading, isError } = useAdminReviewsList({ status: tab === 'all' ? '' : tab, q: debounced, page, limit: PAGE_LIMIT })
  const items = data?.items ?? []
  const counts = data?.counts
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_LIMIT))
  const countFor = (k: Tab) => (counts ? counts[k] : null)
  const anonymous = t('reviews.anonymous')

  const setStatus = (r: AdminReview, status: ReviewStatus) =>
    update.mutate(
      { id: r.id, body: { status } },
      { onSuccess: () => showSuccess(t('admin.reviews.saved')), onError: () => showError(new Error(t('admin.reviews.modal.error'))) },
    )

  const openEdit = (r: AdminReview, reply: boolean) => {
    setFocusReply(reply)
    setEditing(r)
  }

  // Кнопки стоят на постоянных местах: «скрыть» и «вернуть» — одна кнопка (глаз меняет вид,
  // но не позицию), а место «опубликовать» у уже опубликованных остаётся пустым.
  const renderActions = (r: AdminReview) => (
    <div className="flex items-center justify-end gap-1">
      {r.status === 'pending' ? (
        <IconAction label={t('admin.reviews.actionPublish')} tone="ok" disabled={update.isPending} onClick={() => setStatus(r, 'published')}>
          <Check className="size-4" />
        </IconAction>
      ) : (
        <span className="size-8 shrink-0" aria-hidden />
      )}
      <IconAction label={t('admin.reviews.actionReply')} onClick={() => openEdit(r, true)}>
        <Reply className="size-4" />
      </IconAction>
      <IconAction label={t('admin.reviews.actionEdit')} onClick={() => openEdit(r, false)}>
        <Pencil className="size-4" />
      </IconAction>
      {r.status === 'hidden' ? (
        <IconAction label={t('admin.reviews.actionRestore')} tone="ok" disabled={update.isPending} onClick={() => setStatus(r, 'published')}>
          <Eye className="size-4" />
        </IconAction>
      ) : (
        <IconAction label={t('admin.reviews.actionHide')} tone="warn" disabled={update.isPending} onClick={() => setStatus(r, 'hidden')}>
          <EyeOff className="size-4" />
        </IconAction>
      )}
      <IconAction label={t('admin.reviews.actionDelete')} tone="bad" onClick={() => setDeleting(r)}>
        <Trash2 className="size-4" />
      </IconAction>
    </div>
  )

  const statusChip = (r: AdminReview) => (
    <span className={cn('inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium', STATUS_STYLE[r.status])}>
      {t(`admin.reviews.status.${r.status}`)}
    </span>
  )

  const bodyCell = (r: AdminReview) => (
    <>
      <p className="line-clamp-3 whitespace-pre-line text-sm text-foreground/90">{r.body}</p>
      {r.reply && (
        <p className="mt-1.5 line-clamp-2 text-xs text-primary">
          ↳ {t('admin.reviews.replyShort')}: {r.reply.body}
        </p>
      )}
      {r.by_admin && <p className="mt-1 text-xs text-muted-foreground">{t('admin.reviews.byAdmin')}</p>}
    </>
  )

  return (
    <AdminLayout>
      <AdminFeedback feedback={feedback} onDismiss={clear} />
      <div className="space-y-4">
        <AdminPageHeader
          icon={Star}
          title={t('admin.reviews.title')}
          subtitle={t('admin.reviews.subtitle')}
          accent="rose"
          actions={
            <button
              type="button"
              onClick={() => setCreating(true)}
              className="inline-flex items-center gap-1.5 rounded-md border border-input bg-background px-3 py-1.5 text-sm font-medium shadow-sm transition-colors hover:bg-accent"
            >
              <Plus className="size-4" />
              {t('admin.reviews.onBehalf')}
            </button>
          }
        />

        {data && !data.enabled && (
          <p className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
            {t('admin.reviews.disabledNote')}
          </p>
        )}

        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => onSearch(e.target.value)}
            placeholder={t('admin.reviews.search')}
            className="h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          />
        </div>

        <div className="-mx-1 overflow-x-auto overscroll-x-contain px-1 pb-0.5">
          <div className="inline-flex min-w-full gap-1 rounded-lg border border-border/50 bg-card/50 p-1">
            {TABS.map((k) => {
              const n = countFor(k)
              return (
                <button
                  key={k}
                  type="button"
                  onClick={() => {
                    setTab(k)
                    setPage(1)
                  }}
                  className={cn(
                    'min-h-9 flex-1 shrink-0 whitespace-nowrap rounded-md px-3 py-2 text-center text-sm font-medium transition-colors',
                    tab === k ? 'bg-primary/10 text-primary dark:bg-primary/20' : 'text-foreground/80 hover:bg-accent hover:text-foreground',
                  )}
                >
                  {t(TAB_KEY[k])}
                  {n != null && n > 0 && (
                    <span
                      className={cn(
                        'ml-1.5 rounded-full px-1.5 py-px text-[11px] font-semibold',
                        k === 'pending' ? 'bg-amber-500 text-amber-950' : 'bg-muted text-muted-foreground',
                      )}
                    >
                      {n}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </div>

        <Card className="overflow-hidden">
          {isLoading ? (
            <div className="flex items-center justify-center py-12">
              <Spinner />
            </div>
          ) : isError ? (
            <div className="py-12 text-center text-sm text-destructive">{t('admin.reviews.loadError')}</div>
          ) : items.length === 0 ? (
            <div className="py-12 text-center text-sm text-muted-foreground">{t('admin.reviews.empty')}</div>
          ) : (
            <>
              <div className="divide-y divide-border/50 md:hidden">
                {items.map((r) => (
                  <div
                    key={r.id}
                    role="button"
                    tabIndex={0}
                    onClick={() => openEdit(r, false)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && e.target === e.currentTarget) openEdit(r, false)
                    }}
                    className={cn('cursor-pointer space-y-2 p-3 transition-colors hover:bg-accent/40 active:bg-accent/60', r.status === 'pending' && 'bg-amber-500/[0.05]')}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <p className="truncate text-sm font-semibold">{authorLine(r, anonymous)}</p>
                      {statusChip(r)}
                    </div>
                    <div className="flex items-center justify-between gap-2">
                      <Stars value={r.rating} />
                      <span className="text-xs text-muted-foreground">{formatAdminDateTime(r.created_at)}</span>
                    </div>
                    <div>{bodyCell(r)}</div>
                    {renderActions(r)}
                  </div>
                ))}
              </div>

              <div className="hidden overflow-x-auto md:block">
                <table className="w-full text-left">
                  <thead>
                    <tr className="border-b border-border bg-muted/40 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                      <th className="px-3 py-2">{t('admin.reviews.colAuthor')}</th>
                      <th className="px-3 py-2">{t('admin.reviews.colRating')}</th>
                      <th className="px-3 py-2">{t('admin.reviews.colBody')}</th>
                      <th className="px-3 py-2">{t('admin.reviews.colStatus')}</th>
                      <th className="px-3 py-2 text-right">{t('admin.reviews.colActions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((r) => (
                      <tr
                        key={r.id}
                        onClick={() => openEdit(r, false)}
                        className={cn('cursor-pointer border-b border-border/40 align-top transition-colors last:border-0 hover:bg-accent/50', r.status === 'pending' && 'bg-amber-500/[0.05]')}
                      >
                        <td className="min-w-[8rem] px-3 py-3">
                          <p className="truncate text-sm font-semibold">{authorLine(r, anonymous)}</p>
                          <p className="text-xs text-muted-foreground">{formatAdminDateTime(r.created_at)}</p>
                        </td>
                        <td className="px-3 py-3">
                          <Stars value={r.rating} />
                        </td>
                        <td className="max-w-[22rem] px-3 py-3">{bodyCell(r)}</td>
                        <td className="px-3 py-3">{statusChip(r)}</td>
                        <td className="px-3 py-3">{renderActions(r)}</td>
                      </tr>
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

      <AdminReviewModal
        open={editing != null || creating}
        review={editing}
        focusReply={focusReply}
        onClose={() => {
          setEditing(null)
          setCreating(false)
        }}
        onSaved={showSuccess}
      />

      <AdminConfirmModal
        open={deleting != null}
        onClose={() => setDeleting(null)}
        title={t('admin.reviews.deleteTitle')}
        message={t('admin.reviews.deleteHint')}
        confirmLabel={t('admin.reviews.actionDelete')}
        variant="destructive"
        loading={del.isPending}
        onConfirm={() => {
          if (!deleting) return
          del.mutate(deleting.id, {
            onSuccess: () => {
              showSuccess(t('admin.reviews.saved'))
              setDeleting(null)
            },
            onError: () => showError(new Error(t('admin.reviews.modal.error'))),
          })
        }}
      />
    </AdminLayout>
  )
}
