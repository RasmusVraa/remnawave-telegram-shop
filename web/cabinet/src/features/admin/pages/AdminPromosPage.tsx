import { useState, useCallback, useEffect, type MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  TicketPercent,
  Plus,
  ChevronLeft,
  ChevronRight,
  Pencil,
  Trash2,
  ToggleLeft,
  ToggleRight,
  ChevronDown,
  ChevronUp,
  Save,
  Loader2,
  Users,
} from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { Card } from '@/components/ui/card'
import { ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'
import {
  useAdminPromoList,
  useAdminPromoGet,
  useAdminPromoCreate,
  useAdminPromoUpdate,
  useAdminPromoDelete,
  useAdminPromoRedemptions,
  type AdminPromoCode,
  type CreatePromoInput,
} from '../hooks/useAdminPromos'
import { useAdminTariffList } from '../hooks/useAdminTariffs'
import { AdminFeedback } from '../components/AdminFeedback'
import { AdminModal } from '../components/AdminModal'
import { AdminConfirmModal } from '../components/AdminConfirmModal'
import { AdminCheckboxField } from '../components/AdminCheckbox'
import { useAdminMutationFeedback } from '../hooks/useAdminMutationFeedback'
import { formatAdminApiError } from '../utils/formatAdminApiError'
import { truncatePreview } from '../utils/truncatePreview'
import { formatAdminCustomerLabel } from '../utils/formatAdminCustomerLabel'
import { resolvePromoDisplayStatus, type PromoDisplayStatus } from '../utils/promoStatus'

const PROMO_CODE_PREVIEW_LEN = 6

const PROMO_TYPES = ['subscription_days', 'trial', 'extra_hwid', 'discount'] as const

const PROMO_TYPE_KEYS: Record<string, string> = {
  subscription_days: 'admin.promos.typeSubscriptionDays',
  trial: 'admin.promos.typeTrial',
  extra_hwid: 'admin.promos.typeExtraHwid',
  discount: 'admin.promos.typeDiscount',
}

function promoTypeLabel(type: string, t: TFunction) {
  const key = PROMO_TYPE_KEYS[type]
  return key ? t(key) : type
}

function typeBadge(type: string, t: TFunction) {
  const colors: Record<string, string> = {
    subscription_days: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
    trial: 'bg-green-500/15 text-green-600 dark:text-green-400',
    extra_hwid: 'bg-purple-500/15 text-purple-600 dark:text-purple-400',
    discount: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  }
  return (
    <span className={cn('inline-flex rounded-full px-2 py-0.5 text-xs font-medium', colors[type] ?? 'bg-muted text-muted-foreground')}>
      {promoTypeLabel(type, t)}
    </span>
  )
}

function statusBadge(promo: Pick<AdminPromoCode, 'active' | 'valid_until'>, t: TFunction) {
  const status: PromoDisplayStatus = resolvePromoDisplayStatus(promo)
  const styles: Record<PromoDisplayStatus, string> = {
    active: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
    inactive: 'bg-red-500/15 text-red-500 dark:text-red-400',
    expired: 'bg-amber-500/15 text-amber-700 dark:text-amber-400',
  }
  const labelKeys: Record<PromoDisplayStatus, string> = {
    active: 'admin.promos.statusActive',
    inactive: 'admin.promos.statusInactive',
    expired: 'admin.promos.statusExpired',
  }
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium', styles[status])}>
      {t(labelKeys[status])}
    </span>
  )
}

function CreatePromoModal({
  onClose,
  onCreate,
  error,
  codeTaken,
  pending,
  onEdit,
}: {
  onClose: () => void
  onCreate: (d: CreatePromoInput) => void
  /** Ошибка создания — показывается в самом окне: под ним сообщение страницы не видно. */
  error: string | null
  /** Код уже занят — подсвечиваем поле кода. */
  codeTaken: boolean
  pending: boolean
  /** Пользователь изменил форму — старая ошибка больше не актуальна. */
  onEdit: () => void
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState<CreatePromoInput>({
    code: '',
    type: 'subscription_days',
    // Скидка по умолчанию — на одну оплату: 0 означает «без ограничения» и
    // раньше молча ставился любому созданному здесь скидочному промокоду.
    discount_max_subscription_payments_per_customer: 1,
  })

  const set = <K extends keyof CreatePromoInput>(k: K, v: CreatePromoInput[K]) => {
    onEdit()
    setForm((p) => ({ ...p, [k]: v }))
  }

  return (
    <AdminModal open onClose={onClose} title={t('admin.promos.createTitle')} panelClassName="sm:max-w-lg" icon={TicketPercent} iconAccent="violet">
      <div className="space-y-3">
          <div>
            <label className="mb-1 block text-sm font-medium">{t('admin.promos.code')}</label>
            <input
              className={cn(
                'admin-input w-full rounded-lg border bg-background px-3 py-2 text-sm',
                codeTaken ? 'border-destructive ring-1 ring-destructive' : 'border-border',
              )}
              aria-invalid={codeTaken}
              value={form.code}
              onChange={(e) => set('code', e.target.value.toUpperCase())}
              placeholder="PROMO2024"
            />
            {codeTaken && <p className="mt-1 text-xs text-destructive">{t('admin.promos.codeTaken')}</p>}
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium">{t('admin.promos.type')}</label>
            <select
              className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
              value={form.type}
              onChange={(e) => set('type', e.target.value)}
            >
              {PROMO_TYPES.map((pt) => (
                <option key={pt} value={pt}>{promoTypeLabel(pt, t)}</option>
              ))}
            </select>
          </div>
          {form.type === 'subscription_days' && (
            <div>
              <label className="mb-1 block text-sm font-medium">{t('admin.promos.subscriptionDays')}</label>
              <input type="number" className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={form.subscription_days ?? ''} onChange={(e) => set('subscription_days', e.target.value ? Number(e.target.value) : null)} />
            </div>
          )}
          {form.type === 'trial' && (
            <div>
              <label className="mb-1 block text-sm font-medium">{t('admin.promos.trialDays')}</label>
              <input type="number" className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={form.trial_days ?? ''} onChange={(e) => set('trial_days', e.target.value ? Number(e.target.value) : null)} />
            </div>
          )}
          {form.type === 'extra_hwid' && (
            <div>
              <label className="mb-1 block text-sm font-medium">{t('admin.promos.extraHwidDelta')}</label>
              <input type="number" className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={form.extra_hwid_delta ?? ''} onChange={(e) => set('extra_hwid_delta', e.target.value ? Number(e.target.value) : null)} />
            </div>
          )}
          {form.type === 'discount' && (
            <>
              <div>
                <label className="mb-1 block text-sm font-medium">{t('admin.promos.discountPercent')}</label>
                <input type="number" className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={form.discount_percent ?? ''} onChange={(e) => set('discount_percent', e.target.value ? Number(e.target.value) : null)} />
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium">{t('admin.promos.discountTTL')}</label>
                <input type="number" className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={form.discount_ttl_hours ?? ''} onChange={(e) => set('discount_ttl_hours', e.target.value ? Number(e.target.value) : null)} />
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium">{t('admin.promos.discountMaxPayments')}</label>
                <input type="number" min={0} className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={form.discount_max_subscription_payments_per_customer ?? 0} onChange={(e) => set('discount_max_subscription_payments_per_customer', e.target.value ? Number(e.target.value) : 0)} />
                <p className="mt-1 text-xs text-muted-foreground">{t('admin.promos.discountMaxPaymentsHint')}</p>
              </div>
            </>
          )}
          <div>
            <label className="mb-1 block text-sm font-medium">{t('admin.promos.maxUses')}</label>
            <input type="number" className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={form.max_uses ?? ''} onChange={(e) => set('max_uses', e.target.value ? Number(e.target.value) : null)} placeholder={t('admin.promos.unlimited')} />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium">{t('admin.promos.validUntil')}</label>
            <input type="datetime-local" className="admin-input w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={form.valid_until ? form.valid_until.slice(0, 16) : ''} onChange={(e) => set('valid_until', e.target.value ? new Date(e.target.value).toISOString() : null)} />
          </div>
          <AdminCheckboxField
            checked={form.first_purchase_only ?? false}
            onChange={(v) => set('first_purchase_only', v)}
            label={t('admin.promos.firstPurchaseOnly')}
          />
      </div>
      {error && (
        <div role="alert" className="mt-4 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}
      <div className="mt-4 flex justify-end gap-2 border-t border-border/50 pt-4">
          <button className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent" onClick={onClose}>
            {t('admin.cancel')}
          </button>
          <button
            className="rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            disabled={!form.code.trim() || !form.type || pending}
            onClick={() => onCreate(form)}
          >
            {t('admin.create')}
          </button>
      </div>
    </AdminModal>
  )
}

const promoNeutralButtonClass =
  'admin-overview-clickable admin-overview-clickable--surface border-border/50 bg-muted/15 hover:bg-muted/35'

function PromoActionButton({
  icon: Icon,
  label,
  className,
  onClick,
}: {
  icon: typeof Pencil
  label: string
  className?: string
  onClick: (e: MouseEvent) => void
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cn(
        'inline-flex min-h-10 items-center justify-center gap-1.5 rounded-md border px-3 py-2 text-sm font-medium transition-colors max-md:size-10 max-md:px-0',
        className,
      )}
      onClick={onClick}
    >
      <Icon className="size-3.5 shrink-0" />
      <span className="hidden md:inline">{label}</span>
    </button>
  )
}

function PromoUsedByModal({
  promo,
  open,
  onClose,
}: {
  promo: AdminPromoCode
  open: boolean
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const [page, setPage] = useState(1)
  const limit = 20
  const { data, isLoading, isError } = useAdminPromoRedemptions(open ? promo.id : null, page, limit)
  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.limit)) : 1

  useEffect(() => {
    if (open) setPage(1)
  }, [open, promo.id])

  const formatUsedAt = (iso: string) => {
    try {
      return new Date(iso).toLocaleString(i18n.language, {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    } catch {
      return iso
    }
  }

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={t('admin.promos.usedByTitle', { code: promo.code })}
      panelClassName="sm:max-w-md"
      icon={Users}
      iconAccent="violet"
      footer={
        totalPages > 1 ? (
          <div className="flex items-center justify-between gap-2">
            <button
              type="button"
              disabled={page <= 1}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              className="inline-flex items-center gap-1 rounded-md border border-border px-3 py-1.5 text-sm hover:bg-accent disabled:opacity-50"
            >
              <ChevronLeft className="size-4" />
              {t('admin.prev')}
            </button>
            <span className="text-sm text-muted-foreground tabular-nums">
              {page} / {totalPages}
            </span>
            <button
              type="button"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              className="inline-flex items-center gap-1 rounded-md border border-border px-3 py-1.5 text-sm hover:bg-accent disabled:opacity-50"
            >
              {t('admin.next')}
              <ChevronRight className="size-4" />
            </button>
          </div>
        ) : undefined
      }
    >
      {isLoading && (
        <div className="flex justify-center py-8">
          <Loader2 className="size-6 animate-spin text-muted-foreground" />
        </div>
      )}
      {isError && (
        <p className="py-6 text-center text-sm text-destructive">{t('common.error')}</p>
      )}
      {data && data.items.length === 0 && (
        <p className="py-6 text-center text-sm text-muted-foreground">{t('admin.promos.usedByEmpty')}</p>
      )}
      {data && data.items.length > 0 && (
        <ul className="space-y-2 text-sm">
          {data.items.map((row) => (
            <li
              key={`${row.customer_id}-${row.used_at}`}
              className="flex gap-2 rounded-lg border border-border/50 bg-muted/15 px-3 py-2"
            >
              <span className="shrink-0 text-muted-foreground tabular-nums">•</span>
              <span className="min-w-0">
                <span className="text-muted-foreground tabular-nums">{formatUsedAt(row.used_at)}</span>
                <span className="text-muted-foreground"> — </span>
                <span className="font-medium">
                  {formatAdminCustomerLabel({
                    telegram_username: row.telegram_username,
                    nickname: row.nickname,
                    customer_id: row.customer_id,
                  })}
                </span>
              </span>
            </li>
          ))}
        </ul>
      )}
      {data && data.total > 0 && (
        <p className="mt-3 text-xs text-muted-foreground">
          {t('admin.promos.usedByTotal', { count: data.total })}
        </p>
      )}
    </AdminModal>
  )
}

function PromoExpandedDetails({
  promo,
  detail,
  t,
  onEdit,
  onToggle,
  onDelete,
  onShowUsed,
}: {
  promo: AdminPromoCode
  detail?: { redemptions: number; redemptions_today: number }
  t: TFunction
  onEdit: () => void
  onToggle: () => void
  onDelete: () => void
  onShowUsed: () => void
}) {
  return (
    <div className="space-y-3 border-t border-border/50 bg-accent/20 px-4 py-4">
      {detail && (
        <div className="flex flex-wrap gap-x-6 gap-y-1 text-sm">
          <span>{t('admin.promos.redemptions')}: <strong>{detail.redemptions}</strong></span>
          <span>{t('admin.promos.redemptionsToday')}: <strong>{detail.redemptions_today}</strong></span>
        </div>
      )}
      <div className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
        <div className="sm:col-span-2">
          <span className="text-muted-foreground">{t('admin.promos.code')}: </span>
          <span className="font-mono font-medium">{promo.code}</span>
        </div>
        {promo.subscription_days != null && <div>{t('admin.promos.detailSubDays')}: {promo.subscription_days}</div>}
        {promo.trial_days != null && <div>{t('admin.promos.detailTrialDays')}: {promo.trial_days}</div>}
        {promo.extra_hwid_delta != null && <div>{t('admin.promos.detailHwidDelta')}: {promo.extra_hwid_delta}</div>}
        {promo.discount_percent != null && <div>{t('admin.promos.detailDiscount')}: {promo.discount_percent}%</div>}
        {promo.discount_ttl_hours != null && <div>{t('admin.promos.detailTTL')}: {promo.discount_ttl_hours}h</div>}
        {promo.tariff_id != null && <div>{t('admin.promos.detailTariffId')}: {promo.tariff_id}</div>}
        <div>{t('admin.promos.detailFirstPurchase')}: {promo.first_purchase_only ? t('admin.yes') : t('admin.no')}</div>
        <div>{t('admin.promos.detailCreated')}: {new Date(promo.created_at).toLocaleDateString()}</div>
      </div>

      <div className="flex flex-wrap gap-2">
        <PromoActionButton
          icon={Pencil}
          label={t('admin.edit')}
          className={promoNeutralButtonClass}
          onClick={(e) => {
            e.stopPropagation()
            onEdit()
          }}
        />
        <PromoActionButton
          icon={Users}
          label={t('admin.promos.usedBy')}
          className={promoNeutralButtonClass}
          onClick={(e) => {
            e.stopPropagation()
            onShowUsed()
          }}
        />
        <PromoActionButton
          icon={promo.active ? ToggleRight : ToggleLeft}
          label={promo.active ? t('admin.promos.deactivate') : t('admin.promos.activate')}
          className={
            promo.active
              ? 'border-red-500/40 bg-red-500/10 text-red-600 hover:bg-red-500/15 dark:text-red-400'
              : 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/15 dark:text-emerald-400'
          }
          onClick={(e) => {
            e.stopPropagation()
            onToggle()
          }}
        />
        <PromoActionButton
          icon={Trash2}
          label={t('admin.delete')}
          className="border-red-500/30 bg-red-500/5 text-red-500 hover:bg-red-500/10"
          onClick={(e) => {
            e.stopPropagation()
            onDelete()
          }}
        />
      </div>
    </div>
  )
}

function PromoListItem({
  promo,
  mutationHandlers,
  variant,
}: {
  promo: AdminPromoCode
  mutationHandlers: (successMessage?: string) => { onSuccess: () => void; onError: (err: unknown) => void }
  variant: 'table' | 'card'
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [usedOpen, setUsedOpen] = useState(false)
  const update = useAdminPromoUpdate()
  const del = useAdminPromoDelete()
  const { data: detail } = useAdminPromoGet(expanded ? promo.id : null)

  const toggle = useCallback(() => {
    update.mutate(
      { id: promo.id, fields: { active: !promo.active } },
      mutationHandlers(t('admin.feedback.saved')),
    )
  }, [promo.id, promo.active, update, mutationHandlers, t])

  const codePreview = truncatePreview(promo.code, PROMO_CODE_PREVIEW_LEN)
  const usesLabel = `${promo.uses_count}${promo.max_uses != null ? `/${promo.max_uses}` : ''}`
  const validUntilLabel = promo.valid_until ? new Date(promo.valid_until).toLocaleDateString() : '—'

  const modals = (
    <>
      <EditPromoModal
        open={editOpen}
        promo={promo}
        onClose={() => setEditOpen(false)}
        mutationHandlers={mutationHandlers}
      />
      <PromoUsedByModal promo={promo} open={usedOpen} onClose={() => setUsedOpen(false)} />
      <AdminConfirmModal
        open={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        onConfirm={() => {
          del.mutate(promo.id, {
            ...mutationHandlers(t('admin.feedback.saved')),
            onSuccess: () => {
              mutationHandlers(t('admin.feedback.saved')).onSuccess()
              setDeleteOpen(false)
              setExpanded(false)
            },
          })
        }}
        title={t('admin.delete')}
        message={t('admin.promos.confirmDeletePrompt')}
        confirmLabel={t('admin.delete')}
        variant="destructive"
        loading={del.isPending}
        icon={Trash2}
        iconAccent="rose"
      />
    </>
  )

  const expandedDetails = expanded ? (
    <PromoExpandedDetails
      promo={promo}
      detail={detail}
      t={t}
      onEdit={() => setEditOpen(true)}
      onToggle={toggle}
      onDelete={() => setDeleteOpen(true)}
      onShowUsed={() => setUsedOpen(true)}
    />
  ) : null

  if (variant === 'card') {
    return (
      <>
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
                <span
                  className="font-mono text-sm font-semibold"
                  title={promo.code.length > PROMO_CODE_PREVIEW_LEN ? promo.code : undefined}
                >
                  {codePreview}
                </span>
                {typeBadge(promo.type, t)}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {statusBadge(promo, t)}
                <span className="text-xs text-muted-foreground tabular-nums">
                  {t('admin.promos.uses')}: {usesLabel}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('admin.promos.validUntil')}: {validUntilLabel}
              </p>
            </div>
            <span className="mt-0.5 shrink-0 text-muted-foreground">
              {expanded ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
            </span>
          </button>
          {expandedDetails}
        </div>
        {modals}
      </>
    )
  }

  return (
    <>
      <tr
        className="cursor-pointer border-b border-border/50 transition-colors hover:bg-accent/50"
        onClick={() => setExpanded(!expanded)}
      >
        <td className="whitespace-nowrap px-4 py-3">
          <span
            className="font-mono text-sm font-medium"
            title={promo.code.length > PROMO_CODE_PREVIEW_LEN ? promo.code : undefined}
          >
            {codePreview}
          </span>
        </td>
        <td className="px-4 py-3">{typeBadge(promo.type, t)}</td>
        <td className="px-4 py-3">{statusBadge(promo, t)}</td>
        <td className="px-4 py-3 text-sm tabular-nums">{usesLabel}</td>
        <td className="px-4 py-3 text-sm text-muted-foreground">{validUntilLabel}</td>
        <td className="px-4 py-3">
          {expanded ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={6} className="p-0">
            {expandedDetails}
          </td>
        </tr>
      )}
      {modals}
    </>
  )
}

function EditPromoModal({
  open,
  promo,
  onClose,
  mutationHandlers,
}: {
  open: boolean
  promo: AdminPromoCode
  onClose: () => void
  mutationHandlers: (successMessage?: string) => { onSuccess: () => void; onError: (err: unknown) => void }
}) {
  const { t } = useTranslation()
  const update = useAdminPromoUpdate()
  const { data: tariffs } = useAdminTariffList()

  const [maxUses, setMaxUses] = useState(promo.max_uses?.toString() ?? '')
  const [subDays, setSubDays] = useState(promo.subscription_days?.toString() ?? '')
  const [trialDays, setTrialDays] = useState(promo.trial_days?.toString() ?? '')
  const [extraHwidDelta, setExtraHwidDelta] = useState(promo.extra_hwid_delta?.toString() ?? '')
  const [discountPercent, setDiscountPercent] = useState(promo.discount_percent?.toString() ?? '')
  const [discountTTL, setDiscountTTL] = useState(promo.discount_ttl_hours?.toString() ?? '')
  const [tariffId, setTariffId] = useState(promo.tariff_id?.toString() ?? '')
  const [discountMaxPayments, setDiscountMaxPayments] = useState(
    String(promo.discount_max_subscription_payments_per_customer ?? 0),
  )
  const [validUntil, setValidUntil] = useState(
    promo.valid_until ? promo.valid_until.slice(0, 16) : '',
  )
  const [firstPurchaseOnly, setFirstPurchaseOnly] = useState(promo.first_purchase_only)

  useEffect(() => {
    if (!open) return
    setMaxUses(promo.max_uses?.toString() ?? '')
    setSubDays(promo.subscription_days?.toString() ?? '')
    setTrialDays(promo.trial_days?.toString() ?? '')
    setExtraHwidDelta(promo.extra_hwid_delta?.toString() ?? '')
    setDiscountPercent(promo.discount_percent?.toString() ?? '')
    setDiscountTTL(promo.discount_ttl_hours?.toString() ?? '')
    setTariffId(promo.tariff_id?.toString() ?? '')
    setDiscountMaxPayments(String(promo.discount_max_subscription_payments_per_customer ?? 0))
    setValidUntil(promo.valid_until ? promo.valid_until.slice(0, 16) : '')
    setFirstPurchaseOnly(promo.first_purchase_only)
  }, [open, promo])

  const save = () => {
    const fields: Record<string, unknown> = {}

    const setIfChanged = (key: string, newVal: unknown, oldVal: unknown) => {
      if (newVal !== oldVal) fields[key] = newVal
    }

    const maxUsesVal = maxUses.trim() ? Number(maxUses) : null
    setIfChanged('max_uses', maxUsesVal, promo.max_uses ?? null)

    if (promo.type === 'subscription_days') {
      const v = subDays.trim() ? Number(subDays) : null
      setIfChanged('subscription_days', v, promo.subscription_days ?? null)
    }
    if (promo.type === 'trial') {
      const v = trialDays.trim() ? Number(trialDays) : null
      setIfChanged('trial_days', v, promo.trial_days ?? null)
    }
    if (promo.type === 'extra_hwid') {
      const v = extraHwidDelta.trim() ? Number(extraHwidDelta) : null
      setIfChanged('extra_hwid_delta', v, promo.extra_hwid_delta ?? null)
    }
    if (promo.type === 'discount') {
      const dp = discountPercent.trim() ? Number(discountPercent) : null
      const ttl = discountTTL.trim() ? Number(discountTTL) : null
      const tid = tariffId.trim() ? Number(tariffId) : null
      const dmax = discountMaxPayments.trim() ? Number(discountMaxPayments) : 0
      setIfChanged('discount_percent', dp, promo.discount_percent ?? null)
      setIfChanged('discount_ttl_hours', ttl, promo.discount_ttl_hours ?? null)
      setIfChanged('tariff_id', tid, promo.tariff_id ?? null)
      setIfChanged(
        'discount_max_subscription_payments_per_customer',
        dmax,
        promo.discount_max_subscription_payments_per_customer,
      )
    }

    const validUntilIso = validUntil ? new Date(validUntil).toISOString() : null
    const oldValidSlice = promo.valid_until ? promo.valid_until.slice(0, 16) : ''
    if (validUntil !== oldValidSlice) {
      fields.valid_until = validUntilIso
    }

    setIfChanged('first_purchase_only', firstPurchaseOnly, promo.first_purchase_only)

    if (Object.keys(fields).length === 0) {
      onClose()
      return
    }

    update.mutate(
      { id: promo.id, fields },
      {
        ...mutationHandlers(t('admin.feedback.saved')),
        onSuccess: () => {
          mutationHandlers(t('admin.feedback.saved')).onSuccess()
          onClose()
        },
      },
    )
  }

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={t('admin.promos.editPromo')}
      panelClassName="sm:max-w-lg"
      icon={Pencil}
      iconAccent="violet"
      footer={
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent"
          >
            {t('admin.cancel')}
          </button>
          <button
            type="button"
            onClick={save}
            disabled={update.isPending}
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {update.isPending ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
            {t('admin.save')}
          </button>
        </div>
      }
    >
      <div className="space-y-3">
        <p className="font-mono text-sm font-medium">{promo.code}</p>
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-xs font-medium">{t('admin.promos.maxUses')}</label>
            <input type="number" className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={maxUses} onChange={(e) => setMaxUses(e.target.value)} placeholder={t('admin.promos.unlimited')} />
          </div>
          {promo.type === 'subscription_days' && (
            <div>
              <label className="mb-1 block text-xs font-medium">{t('admin.promos.subscriptionDays')}</label>
              <input type="number" className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={subDays} onChange={(e) => setSubDays(e.target.value)} />
            </div>
          )}
          {promo.type === 'trial' && (
            <div>
              <label className="mb-1 block text-xs font-medium">{t('admin.promos.trialDays')}</label>
              <input type="number" className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={trialDays} onChange={(e) => setTrialDays(e.target.value)} />
            </div>
          )}
          {promo.type === 'extra_hwid' && (
            <div>
              <label className="mb-1 block text-xs font-medium">{t('admin.promos.extraHwidDelta')}</label>
              <input type="number" className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={extraHwidDelta} onChange={(e) => setExtraHwidDelta(e.target.value)} />
            </div>
          )}
          {promo.type === 'discount' && (
            <>
              <div>
                <label className="mb-1 block text-xs font-medium">{t('admin.promos.discountPercent')}</label>
                <input type="number" className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={discountPercent} onChange={(e) => setDiscountPercent(e.target.value)} />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium">{t('admin.promos.discountTTL')}</label>
                <input type="number" className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={discountTTL} onChange={(e) => setDiscountTTL(e.target.value)} />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium">{t('admin.promos.tariffId')}</label>
                <select className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={tariffId} onChange={(e) => setTariffId(e.target.value)}>
                  <option value="">{t('admin.promos.noTariff')}</option>
                  {tariffs?.map((tariff) => (
                    <option key={tariff.id} value={tariff.id}>{tariff.name ?? tariff.slug}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium">{t('admin.promos.discountMaxPayments')}</label>
                <input type="number" className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={discountMaxPayments} onChange={(e) => setDiscountMaxPayments(e.target.value)} />
              </div>
            </>
          )}
          <div className="sm:col-span-2">
            <label className="mb-1 block text-xs font-medium">{t('admin.promos.validUntil')}</label>
            <input type="datetime-local" className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" value={validUntil} onChange={(e) => setValidUntil(e.target.value)} />
          </div>
        </div>
        <AdminCheckboxField
          checked={firstPurchaseOnly}
          onChange={setFirstPurchaseOnly}
          label={t('admin.promos.firstPurchaseOnly')}
        />
      </div>
    </AdminModal>
  )
}

export default function AdminPromosPage() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [showCreate, setShowCreate] = useState(false)
  const { data, isLoading } = useAdminPromoList(page)
  const create = useAdminPromoCreate()
  const { feedback, clear, handlers } = useAdminMutationFeedback()

  const totalPages = data ? Math.ceil(data.total / data.limit) : 1

  const [createError, setCreateError] = useState<{ message: string; codeTaken: boolean } | null>(null)

  const handleCreate = (input: CreatePromoInput) => {
    setCreateError(null)
    create.mutate(input, {
      onSuccess: () => {
        handlers(t('admin.feedback.saved')).onSuccess()
        setShowCreate(false)
      },
      onError: (err) => {
        setCreateError({
          message: formatAdminApiError(err, t),
          codeTaken: err instanceof ApiError && err.status === 409,
        })
      },
    })
  }

  const closeCreate = () => {
    setShowCreate(false)
    setCreateError(null)
  }

  return (
    <AdminLayout>
      <AdminFeedback feedback={feedback} onDismiss={clear} />
      <div className="space-y-6">
        <AdminPageHeader
          icon={TicketPercent}
          title={t('admin.promos.title')}
          subtitle={t('admin.promos.subtitle')}
          accent="amber"
          actions={
            <button
              className="inline-flex items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
              onClick={() => setShowCreate(true)}
            >
              <Plus className="size-4" />
              {t('admin.promos.create')}
            </button>
          }
        />

        <Card className="overflow-hidden">
          {isLoading ? (
            <div className="p-8 text-center text-muted-foreground">{t('admin.loading')}</div>
          ) : !data || data.items.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">{t('admin.promos.empty')}</div>
          ) : (
            <>
              <div className="space-y-2 p-3 md:hidden">
                {data.items.map((promo) => (
                  <PromoListItem key={promo.id} promo={promo} mutationHandlers={handlers} variant="card" />
                ))}
              </div>
              <div className="hidden overflow-x-auto md:block">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-border bg-muted/50 text-left text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      <th className="whitespace-nowrap px-4 py-3">{t('admin.promos.code')}</th>
                      <th className="px-4 py-3">{t('admin.promos.type')}</th>
                      <th className="px-4 py-3">{t('admin.promos.status')}</th>
                      <th className="px-4 py-3">{t('admin.promos.uses')}</th>
                      <th className="px-4 py-3">{t('admin.promos.validUntil')}</th>
                      <th className="w-8 px-4 py-3" />
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((promo) => (
                      <PromoListItem key={promo.id} promo={promo} mutationHandlers={handlers} variant="table" />
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </Card>

        {totalPages > 1 && (
          <div className="flex items-center justify-center gap-2">
            <button
              className="inline-flex items-center gap-1 rounded-md border border-border px-3 py-1.5 text-sm hover:bg-accent disabled:opacity-50"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft className="size-4" />
            </button>
            <span className="text-sm text-muted-foreground">
              {page} / {totalPages}
            </span>
            <button
              className="inline-flex items-center gap-1 rounded-md border border-border px-3 py-1.5 text-sm hover:bg-accent disabled:opacity-50"
              disabled={page >= totalPages}
              onClick={() => setPage(page + 1)}
            >
              <ChevronRight className="size-4" />
            </button>
          </div>
        )}
      </div>

      {showCreate && (
        <CreatePromoModal
          onClose={closeCreate}
          onCreate={handleCreate}
          error={createError?.message ?? null}
          codeTaken={createError?.codeTaken ?? false}
          pending={create.isPending}
          onEdit={() => setCreateError(null)}
        />
      )}
    </AdminLayout>
  )
}
