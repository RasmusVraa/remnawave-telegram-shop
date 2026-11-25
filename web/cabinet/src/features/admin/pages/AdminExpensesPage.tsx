import { useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownRight,
  ArrowUpRight,
  Building2,
  ChevronLeft,
  ChevronRight,
  Cloud,
  Globe2,
  Megaphone,
  Pencil,
  Plus,
  Receipt,
  Server,
  Trash2,
  TrendingUp,
  Wallet,
  type LucideIcon,
} from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminModal } from '../components/AdminModal'
import { AdminFeedback } from '../components/AdminFeedback'
import { useAdminMutationFeedback } from '../hooks/useAdminMutationFeedback'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import type { AdminExpenseDTO, AdminExpenseWriteInput } from '@/lib/types/admin'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { hapticSelection } from '@/lib/haptic'

const CATEGORIES = ['servers', 'traffic', 'domains', 'ads', 'software', 'taxes', 'salary', 'other'] as const
type ExpenseCategory = (typeof CATEGORIES)[number]

const CATEGORY_ICONS: Record<ExpenseCategory, LucideIcon> = {
  servers: Server,
  traffic: Cloud,
  domains: Globe2,
  ads: Megaphone,
  software: Building2,
  taxes: Receipt,
  salary: Wallet,
  other: Receipt,
}

const CATEGORY_ACCENT: Record<ExpenseCategory, string> = {
  servers: 'bg-sky-500/15 text-sky-700 dark:text-sky-300',
  traffic: 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-300',
  domains: 'bg-indigo-500/15 text-indigo-700 dark:text-indigo-300',
  ads: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
  software: 'bg-violet-500/15 text-violet-700 dark:text-violet-300',
  taxes: 'bg-rose-500/15 text-rose-700 dark:text-rose-300',
  salary: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
  other: 'bg-slate-500/15 text-slate-700 dark:text-slate-300',
}

function monthLabel(year: number, month: number, locale: string) {
  return new Intl.DateTimeFormat(locale.startsWith('ru') ? 'ru-RU' : 'en-US', {
    month: 'long',
    year: 'numeric',
  }).format(new Date(Date.UTC(year, month - 1, 1)))
}

function todayISO() {
  const d = new Date()
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

type FormState = {
  occurred_on: string
  amount_rub: string
  category: ExpenseCategory
  title: string
  note: string
}

const emptyForm = (): FormState => ({
  occurred_on: todayISO(),
  amount_rub: '',
  category: 'servers',
  title: '',
  note: '',
})

function toWriteInput(form: FormState): AdminExpenseWriteInput {
  return {
    occurred_on: form.occurred_on,
    amount_rub: Number(form.amount_rub.replace(',', '.')),
    category: form.category,
    title: form.title.trim(),
    note: form.note.trim(),
    icon: form.category,
  }
}

export default function AdminExpensesPage() {
  const { t, i18n } = useTranslation()
  const qc = useQueryClient()
  const { feedback, clear, showError, showSuccess } = useAdminMutationFeedback()
  const now = new Date()
  const [year, setYear] = useState(now.getUTCFullYear())
  const [month, setMonth] = useState(now.getUTCMonth() + 1)
  const [filter, setFilter] = useState<ExpenseCategory | 'all'>('all')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<AdminExpenseDTO | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm)
  const [confirmDelete, setConfirmDelete] = useState<AdminExpenseDTO | null>(null)

  const queryKey = ['adminExpenses', year, month] as const
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey,
    queryFn: () => api.adminExpenses({ year, month }),
    staleTime: 15_000,
  })

  const saveMut = useMutation({
    mutationFn: async () => {
      const body = toWriteInput(form)
      if (!Number.isFinite(body.amount_rub) || body.amount_rub <= 0) {
        throw new Error('amount')
      }
      if (!body.title) throw new Error('title')
      if (editing) return api.adminExpenseUpdate(editing.id, body)
      return api.adminExpenseCreate(body)
    },
    onSuccess: async () => {
      const wasEdit = editing != null
      setModalOpen(false)
      setEditing(null)
      setForm(emptyForm())
      showSuccess(t(wasEdit ? 'admin.expenses.saved' : 'admin.expenses.created'))
      await qc.invalidateQueries({ queryKey: ['adminExpenses'] })
    },
    onError: (err) => showError(err),
  })

  const deleteMut = useMutation({
    mutationFn: (id: number) => api.adminExpenseDelete(id),
    onSuccess: async () => {
      setConfirmDelete(null)
      showSuccess(t('admin.expenses.deleted'))
      await qc.invalidateQueries({ queryKey: ['adminExpenses'] })
    },
    onError: (err) => showError(err),
  })

  const summary = data?.summary
  const items = useMemo(() => {
    const list = data?.items ?? []
    if (filter === 'all') return list
    return list.filter((x) => x.category === filter)
  }, [data?.items, filter])

  function shiftMonth(delta: number) {
    hapticSelection()
    const d = new Date(Date.UTC(year, month - 1 + delta, 1))
    setYear(d.getUTCFullYear())
    setMonth(d.getUTCMonth() + 1)
  }

  function openCreate() {
    setEditing(null)
    setForm(emptyForm())
    setModalOpen(true)
  }

  function openEdit(item: AdminExpenseDTO) {
    setEditing(item)
    setForm({
      occurred_on: item.occurred_on,
      amount_rub: String(item.amount_rub),
      category: (CATEGORIES.includes(item.category as ExpenseCategory)
        ? item.category
        : 'other') as ExpenseCategory,
      title: item.title,
      note: item.note ?? '',
    })
    setModalOpen(true)
  }

  const profit = summary?.profit_rub ?? 0
  const profitPositive = profit >= 0

  return (
    <AdminLayout>
      <div className="space-y-5">
        <AdminPageHeader
          icon={Wallet}
          accent="emerald"
          title={t('admin.expenses.title')}
          subtitle={t('admin.expenses.subtitle')}
          actions={
            <Button type="button" onClick={openCreate} className="gap-1.5">
              <Plus className="size-4" />
              {t('admin.expenses.add')}
            </Button>
          }
        />

        <AdminFeedback feedback={feedback} onDismiss={clear} />

        <div className="flex items-center justify-between gap-3 rounded-2xl border border-border/70 bg-card/80 px-3 py-2.5">
          <Button type="button" variant="ghost" size="icon" onClick={() => shiftMonth(-1)} aria-label={t('admin.expenses.prevMonth')}>
            <ChevronLeft className="size-5" />
          </Button>
          <div className="text-center">
            <p className="text-sm font-semibold capitalize">{monthLabel(year, month, i18n.language)}</p>
            <p className="text-xs text-muted-foreground">{t('admin.expenses.monthHint')}</p>
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={() => shiftMonth(1)} aria-label={t('admin.expenses.nextMonth')}>
            <ChevronRight className="size-5" />
          </Button>
        </div>

        <div className="grid gap-3 sm:grid-cols-3">
          <KpiCard
            label={t('admin.expenses.revenue')}
            value={summary?.revenue_rub}
            loading={isLoading}
            icon={ArrowUpRight}
            tone="income"
          />
          <KpiCard
            label={t('admin.expenses.spent')}
            value={summary?.expenses_rub}
            loading={isLoading}
            icon={ArrowDownRight}
            tone="spend"
          />
          <KpiCard
            label={t('admin.expenses.profit')}
            value={summary?.profit_rub}
            loading={isLoading}
            icon={TrendingUp}
            tone={profitPositive ? 'profit' : 'loss'}
            emphasize
          />
        </div>

        {(summary?.by_category?.length ?? 0) > 0 && (
          <div className="flex flex-wrap gap-2">
            <FilterChip active={filter === 'all'} onClick={() => setFilter('all')} label={t('admin.expenses.filterAll')} />
            {CATEGORIES.map((cat) => {
              const sum = summary?.by_category.find((c) => c.category === cat)
              if (!sum) return null
              const Icon = CATEGORY_ICONS[cat]
              return (
                <FilterChip
                  key={cat}
                  active={filter === cat}
                  onClick={() => setFilter(cat)}
                  label={`${t(`admin.expenses.categories.${cat}`)} · ${formatNumber(Math.round(sum.amount_rub))} ₽`}
                  icon={<Icon className="size-3.5" />}
                />
              )
            })}
          </div>
        )}

        {isLoading ? (
          <div className="flex justify-center py-16">
            <Spinner className="size-6" />
          </div>
        ) : isError ? (
          <div className="rounded-2xl border border-destructive/30 bg-destructive/5 p-6 text-center">
            <p className="text-sm text-destructive">{t('admin.expenses.loadError')}</p>
            <Button type="button" variant="outline" className="mt-3" onClick={() => void refetch()}>
              {t('errors.retry')}
            </Button>
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-border bg-muted/30 px-6 py-14 text-center">
            <div className="mx-auto mb-3 flex size-12 items-center justify-center rounded-2xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
              <Wallet className="size-6" />
            </div>
            <p className="font-medium">{t('admin.expenses.emptyTitle')}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t('admin.expenses.emptyHint')}</p>
            <Button type="button" className="mt-4 gap-1.5" onClick={openCreate}>
              <Plus className="size-4" />
              {t('admin.expenses.add')}
            </Button>
          </div>
        ) : (
          <ul className="space-y-2">
            {items.map((item) => {
              const cat = (CATEGORIES.includes(item.category as ExpenseCategory) ? item.category : 'other') as ExpenseCategory
              const Icon = CATEGORY_ICONS[cat]
              return (
                <li
                  key={item.id}
                  className="group flex items-center gap-3 rounded-2xl border border-border/70 bg-card px-3.5 py-3 transition-colors hover:bg-accent/30"
                >
                  <div className={cn('flex size-11 shrink-0 items-center justify-center rounded-xl', CATEGORY_ACCENT[cat])}>
                    <Icon className="size-5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate font-semibold leading-tight">{item.title}</p>
                        <p className="mt-0.5 text-xs text-muted-foreground">
                          {t(`admin.expenses.categories.${cat}`)} · {item.occurred_on}
                        </p>
                      </div>
                      <p className="shrink-0 text-sm font-semibold tabular-nums text-rose-600 dark:text-rose-400">
                        −{formatNumber(item.amount_rub)} ₽
                      </p>
                    </div>
                    {item.note ? <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{item.note}</p> : null}
                  </div>
                  <div className="flex shrink-0 gap-1 opacity-100 sm:opacity-0 sm:transition-opacity sm:group-hover:opacity-100">
                    <Button type="button" size="icon" variant="ghost" onClick={() => openEdit(item)} aria-label={t('admin.expenses.edit')}>
                      <Pencil className="size-4" />
                    </Button>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      className="text-destructive hover:text-destructive"
                      onClick={() => setConfirmDelete(item)}
                      aria-label={t('admin.expenses.delete')}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      <AdminModal
        open={modalOpen}
        onClose={() => {
          if (saveMut.isPending) return
          setModalOpen(false)
        }}
        title={editing ? t('admin.expenses.editTitle') : t('admin.expenses.addTitle')}
        description={t('admin.expenses.formHint')}
        icon={Wallet}
        iconAccent="emerald"
        size="md"
        footer={
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" disabled={saveMut.isPending} onClick={() => setModalOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="button" disabled={saveMut.isPending} onClick={() => saveMut.mutate()}>
              {saveMut.isPending ? <Spinner className="size-4" /> : t('common.save')}
            </Button>
          </div>
        }
      >
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="expense-amount">{t('admin.expenses.amount')}</Label>
              <Input
                id="expense-amount"
                inputMode="decimal"
                placeholder="0"
                value={form.amount_rub}
                onChange={(e) => setForm((f) => ({ ...f, amount_rub: e.target.value }))}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="expense-date">{t('admin.expenses.date')}</Label>
              <Input
                id="expense-date"
                type="date"
                value={form.occurred_on}
                onChange={(e) => setForm((f) => ({ ...f, occurred_on: e.target.value }))}
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="expense-title">{t('admin.expenses.name')}</Label>
            <Input
              id="expense-title"
              value={form.title}
              maxLength={200}
              placeholder={t('admin.expenses.namePlaceholder')}
              onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
            />
          </div>
          <div className="space-y-2">
            <Label>{t('admin.expenses.category')}</Label>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {CATEGORIES.map((cat) => {
                const Icon = CATEGORY_ICONS[cat]
                const active = form.category === cat
                return (
                  <button
                    key={cat}
                    type="button"
                    onClick={() => {
                      hapticSelection()
                      setForm((f) => ({ ...f, category: cat }))
                    }}
                    className={cn(
                      'flex flex-col items-center gap-1.5 rounded-xl border px-2 py-2.5 text-center text-xs transition-colors',
                      active ? 'border-primary bg-primary/10 text-foreground' : 'border-border hover:bg-accent/50',
                    )}
                  >
                    <span className={cn('flex size-8 items-center justify-center rounded-lg', CATEGORY_ACCENT[cat])}>
                      <Icon className="size-4" />
                    </span>
                    {t(`admin.expenses.categories.${cat}`)}
                  </button>
                )
              })}
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="expense-note">{t('admin.expenses.note')}</Label>
            <textarea
              id="expense-note"
              rows={3}
              value={form.note}
              maxLength={2000}
              placeholder={t('admin.expenses.notePlaceholder')}
              onChange={(e) => setForm((f) => ({ ...f, note: e.target.value }))}
              className="flex min-h-[80px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
          </div>
        </div>
      </AdminModal>

      <AdminModal
        open={confirmDelete != null}
        onClose={() => setConfirmDelete(null)}
        title={t('admin.expenses.deleteTitle')}
        description={confirmDelete ? t('admin.expenses.deleteHint', { title: confirmDelete.title }) : undefined}
        icon={Trash2}
        iconAccent="rose"
        size="sm"
        footer={
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" disabled={deleteMut.isPending} onClick={() => setConfirmDelete(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deleteMut.isPending || !confirmDelete}
              onClick={() => confirmDelete && deleteMut.mutate(confirmDelete.id)}
            >
              {deleteMut.isPending ? <Spinner className="size-4" /> : t('admin.expenses.delete')}
            </Button>
          </div>
        }
      >
        {null}
      </AdminModal>
    </AdminLayout>
  )
}

function KpiCard({
  label,
  value,
  loading,
  icon: Icon,
  tone,
  emphasize,
}: {
  label: string
  value?: number
  loading: boolean
  icon: LucideIcon
  tone: 'income' | 'spend' | 'profit' | 'loss'
  emphasize?: boolean
}) {
  const toneCls = {
    income: 'from-emerald-500/15 to-transparent text-emerald-700 dark:text-emerald-300',
    spend: 'from-rose-500/15 to-transparent text-rose-700 dark:text-rose-300',
    profit: 'from-teal-500/20 to-transparent text-teal-700 dark:text-teal-300',
    loss: 'from-orange-500/20 to-transparent text-orange-700 dark:text-orange-300',
  }[tone]

  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-br p-4',
        toneCls,
        emphasize && 'sm:col-span-1 ring-1 ring-inset ring-black/5 dark:ring-white/10',
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
        <Icon className="size-4 opacity-70" />
      </div>
      <p className={cn('mt-2 text-2xl font-semibold tabular-nums tracking-tight', emphasize && 'text-3xl')}>
        {loading || value == null ? '—' : `${formatNumber(Math.round(value))} ₽`}
      </p>
    </div>
  )
}

function FilterChip({
  active,
  onClick,
  label,
  icon,
}: {
  active: boolean
  onClick: () => void
  label: string
  icon?: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={() => {
        hapticSelection()
        onClick()
      }}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors',
        active ? 'border-primary bg-primary/10 text-foreground' : 'border-border bg-card text-muted-foreground hover:bg-accent/40',
      )}
    >
      {icon}
      {label}
    </button>
  )
}
