import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Gem,
  Plus,
  Trash2,
  Edit,
  RefreshCw,
  Save,
  Loader2,
  ChevronDown,
  ChevronUp,
  Layers,
  ListOrdered,
  Sparkles,
  Percent,
  Settings2,
} from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminModal } from '../components/AdminModal'
import { AdminConfirmModal } from '../components/AdminConfirmModal'
import {
  useAdminLoyaltyTiers,
  useAdminLoyaltyCreateTier,
  useAdminLoyaltyUpdateTier,
  useAdminLoyaltyDeleteTier,
  useAdminLoyaltyRecalc,
  type AdminLoyaltyTier,
} from '../hooks/useAdminLoyalty'
import { cn } from '@/lib/utils'

interface TierFormState {
  xp_min: number
  discount_percent: number
}

const emptyForm: TierFormState = { xp_min: 0, discount_percent: 0 }

function LoyaltyTierForm({
  form,
  onChange,
}: {
  form: TierFormState
  onChange: (next: TierFormState) => void
}) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div>
        <label className="mb-1 block text-xs font-medium text-muted-foreground">{t('admin.loyalty.xpMin')}</label>
        <input
          type="number"
          value={form.xp_min}
          onChange={(e) => onChange({ ...form, xp_min: Number(e.target.value) })}
          className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none"
        />
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-muted-foreground">{t('admin.loyalty.discountPercent')}</label>
        <input
          type="number"
          value={form.discount_percent}
          onChange={(e) => onChange({ ...form, discount_percent: Number(e.target.value) })}
          className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none"
        />
      </div>
    </div>
  )
}

function LoyaltyTierMobileCard({
  tier,
  onEdit,
  onDelete,
}: {
  tier: AdminLoyaltyTier
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)

  return (
    <div
      className={cn(
        'overflow-hidden rounded-lg border border-border/60 bg-card transition-colors',
        expanded && 'ring-1 ring-primary/20',
      )}
    >
      <button
        type="button"
        className="flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-accent/40 active:bg-accent/60"
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
      >
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold">
              {t('loyaltyPage.levelNumber', { n: tier.sort_order })}
            </span>
            <span className="rounded-full bg-emerald-500/15 px-2 py-0.5 text-[11px] font-medium text-emerald-600 dark:text-emerald-400">
              {tier.discount_percent}%
            </span>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('admin.loyalty.xpMin')}: {tier.xp_min.toLocaleString()}
          </p>
        </div>
        <span className="mt-0.5 shrink-0 text-muted-foreground">
          {expanded ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
        </span>
      </button>

      {expanded && (
        <div className="space-y-3 border-t border-border/50 bg-accent/20 px-4 py-4">
          <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">{t('admin.loyalty.sortOrder')}</dt>
              <dd className="font-medium">{tier.sort_order}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('admin.loyalty.xpMin')}</dt>
              <dd className="font-medium tabular-nums">{tier.xp_min.toLocaleString()}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('admin.loyalty.discountPercent')}</dt>
              <dd className="font-medium tabular-nums">{tier.discount_percent}%</dd>
            </div>
          </dl>

          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                onEdit()
              }}
              className="inline-flex min-h-10 items-center gap-1.5 rounded-md border border-border px-3 py-2 text-sm hover:bg-accent"
            >
              <Edit className="size-3.5" />
              {t('admin.edit')}
            </button>
            {tier.sort_order !== 0 && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation()
                  onDelete()
                }}
                className="inline-flex min-h-10 items-center gap-1.5 rounded-md border border-red-500/30 px-3 py-2 text-sm text-red-500 hover:bg-red-500/10"
              >
                <Trash2 className="size-3.5" />
                {t('admin.delete')}
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export default function AdminLoyaltyPage() {
  const { t } = useTranslation()
  const { data: tiers, isLoading } = useAdminLoyaltyTiers()
  const createMut = useAdminLoyaltyCreateTier()
  const updateMut = useAdminLoyaltyUpdateTier()
  const deleteMut = useAdminLoyaltyDeleteTier()
  const recalcMut = useAdminLoyaltyRecalc()

  const [formOpen, setFormOpen] = useState(false)
  const [editTier, setEditTier] = useState<AdminLoyaltyTier | null>(null)
  const [form, setForm] = useState<TierFormState>(emptyForm)
  const [recalcOpen, setRecalcOpen] = useState(false)
  const [deleteTier, setDeleteTier] = useState<AdminLoyaltyTier | null>(null)
  const [recalcDone, setRecalcDone] = useState(false)

  function openCreate() {
    setEditTier(null)
    setForm(emptyForm)
    setFormOpen(true)
  }

  function openEdit(tier: AdminLoyaltyTier) {
    setEditTier(tier)
    setForm({
      xp_min: tier.xp_min,
      discount_percent: tier.discount_percent,
    })
    setFormOpen(true)
  }

  function closeForm() {
    setFormOpen(false)
    setEditTier(null)
  }

  function handleSubmit() {
    if (editTier) {
      updateMut.mutate(
        {
          id: editTier.id,
          fields: {
            sort_order: editTier.sort_order,
            xp_min: form.xp_min,
            discount_percent: form.discount_percent,
          },
        },
        { onSuccess: closeForm },
      )
    } else {
      createMut.mutate(
        {
          xp_min: form.xp_min,
          discount_percent: form.discount_percent,
        },
        { onSuccess: closeForm },
      )
    }
  }

  function handleRecalcConfirm() {
    recalcMut.mutate(undefined, {
      onSuccess: () => {
        setRecalcDone(true)
        setRecalcOpen(false)
      },
    })
  }

  const formSaving = createMut.isPending || updateMut.isPending

  return (
    <AdminLayout>
      <div className="space-y-6">
        <AdminPageHeader
          icon={Gem}
          title={t('admin.loyalty.title')}
          subtitle={t('admin.loyalty.subtitle')}
          accent="rose"
          actions={
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setRecalcOpen(true)}
                disabled={recalcMut.isPending}
                className={cn('inline-flex items-center gap-1.5 rounded-md bg-secondary px-3 py-1.5 text-sm font-medium text-secondary-foreground transition-colors hover:bg-secondary/80 disabled:opacity-50', recalcMut.isPending && '!opacity-100')}
              >
                <RefreshCw className={`size-4 ${recalcMut.isPending ? 'animate-spin' : ''}`} />
                {t('admin.loyalty.recalc')}
              </button>
              <button
                type="button"
                onClick={openCreate}
                className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
              >
                <Plus className="size-4" />
                {t('admin.loyalty.addTier')}
              </button>
            </div>
          }
        />

        {recalcDone && (
          <div className="rounded-md bg-green-500/10 p-3 text-sm text-green-700 dark:text-green-400">
            {t('admin.loyalty.recalcStarted')}
          </div>
        )}

        <div className="rounded-lg border border-border/50 bg-card">
          <div className="border-b border-border/50 px-4 py-3">
            <h3 className="flex items-center gap-2 text-sm font-medium">
              <Layers className="size-4" />
              {t('admin.loyalty.tiers')}
            </h3>
          </div>

          {isLoading ? (
            <div className="flex justify-center py-8">
              <span className="size-6 rounded-full border-2 border-primary border-t-transparent animate-spin" />
            </div>
          ) : !tiers?.length ? (
            <p className="px-4 py-6 text-center text-sm text-muted-foreground">{t('admin.noData')}</p>
          ) : (
            <>
              <div className="space-y-2 p-3 md:hidden">
                {tiers.map((tier) => (
                  <LoyaltyTierMobileCard
                    key={tier.id}
                    tier={tier}
                    onEdit={() => openEdit(tier)}
                    onDelete={() => setDeleteTier(tier)}
                  />
                ))}
              </div>
              <div className="hidden overflow-x-auto md:block">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border/50 text-muted-foreground">
                      <th className="px-4 py-2 text-left font-medium">
                        <span className="inline-flex items-center gap-1.5">
                          <ListOrdered className="size-3.5" />
                          {t('admin.loyalty.sortOrder')}
                        </span>
                      </th>
                      <th className="px-4 py-2 text-left font-medium">
                        <span className="inline-flex items-center gap-1.5">
                          <Sparkles className="size-3.5" />
                          {t('admin.loyalty.xpMin')}
                        </span>
                      </th>
                      <th className="px-4 py-2 text-left font-medium">
                        <span className="inline-flex items-center gap-1.5">
                          <Percent className="size-3.5" />
                          {t('admin.loyalty.discountPercent')}
                        </span>
                      </th>
                      <th className="px-4 py-2 text-right font-medium">
                        <span className="inline-flex items-center justify-end gap-1.5">
                          <Settings2 className="size-3.5" />
                          {t('admin.actions')}
                        </span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {tiers.map((tier) => (
                      <tr key={tier.id} className="border-b border-border/30 last:border-b-0">
                        <td className="px-4 py-2.5">{tier.sort_order}</td>
                        <td className="px-4 py-2.5">{tier.xp_min.toLocaleString()}</td>
                        <td className="px-4 py-2.5">{tier.discount_percent}%</td>
                        <td className="px-4 py-2.5 text-right">
                          <div className="inline-flex gap-1">
                            <button
                              type="button"
                              onClick={() => openEdit(tier)}
                              className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                            >
                              <Edit className="size-4" />
                            </button>
                            {tier.sort_order !== 0 && (
                              <button
                                type="button"
                                onClick={() => setDeleteTier(tier)}
                                className="rounded-md p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                              >
                                <Trash2 className="size-4" />
                              </button>
                            )}
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      </div>

      <AdminModal
        open={formOpen}
        onClose={closeForm}
        title={editTier ? t('admin.loyalty.editTier') : t('admin.loyalty.addTier')}
        icon={editTier ? Edit : Plus}
        iconAccent="rose"
        panelClassName="sm:max-w-lg"
        footer={
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={closeForm}
              className="rounded-lg border px-4 py-2 text-sm hover:bg-accent"
            >
              {t('admin.cancel')}
            </button>
            <button
              type="button"
              onClick={handleSubmit}
              disabled={formSaving}
              className={cn('inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50', formSaving && '!opacity-100')}
            >
              {formSaving ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
              {t('admin.save')}
            </button>
          </div>
        }
      >
        <LoyaltyTierForm form={form} onChange={setForm} />
      </AdminModal>

      <AdminConfirmModal
        open={recalcOpen}
        onClose={() => setRecalcOpen(false)}
        onConfirm={handleRecalcConfirm}
        title={t('admin.loyalty.recalc')}
        message={t('admin.loyalty.recalcConfirm')}
        confirmLabel={t('admin.loyalty.recalc')}
        loading={recalcMut.isPending}
        icon={RefreshCw}
        iconAccent="rose"
      />

      <AdminConfirmModal
        open={deleteTier != null}
        onClose={() => setDeleteTier(null)}
        onConfirm={() => {
          if (!deleteTier) return
          deleteMut.mutate(deleteTier.id, { onSuccess: () => setDeleteTier(null) })
        }}
        title={t('admin.delete')}
        message={t('admin.loyalty.confirmDelete', {
          name: deleteTier?.sort_order ?? deleteTier?.id ?? '',
        })}
        confirmLabel={t('admin.delete')}
        variant="destructive"
        loading={deleteMut.isPending}
        icon={Trash2}
        iconAccent="rose"
      />
    </AdminLayout>
  )
}
