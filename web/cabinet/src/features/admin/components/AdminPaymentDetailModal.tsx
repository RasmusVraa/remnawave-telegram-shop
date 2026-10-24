import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import {
  Check,
  Copy,
  CreditCard,
  Landmark,
  Loader2,
  Package,
  Receipt,
  User,
  UserRound,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { AdminModal } from './AdminModal'
import { AdminSectionCard } from './AdminSectionCard'
import { cn } from '@/lib/utils'
import { surface } from './Surface'
import { copyToClipboard } from '@/lib/clipboard'
import { useAdminPayment } from '../hooks/useAdminPayments'
import { formatInvoiceType } from '../utils/formatInvoiceType'
import { formatAdminDateTime } from '../utils/datetime'
import { formatPaymentAmount } from '../utils/formatPaymentAmount'
import { paymentStatusInfo, formatPurchaseKind, describePayment } from '../utils/paymentStatus'

interface Props {
  paymentId: number | null
  onClose: () => void
}

function Row({ label, value, copyValue }: { label: string; value: ReactNode; copyValue?: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    if (!copyValue) return
    const ok = await copyToClipboard(copyValue)
    if (ok) {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }
  }

  return (
    <div className="flex items-center justify-between gap-3 py-1.5 text-sm">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="flex min-w-0 items-center gap-1">
        <span className="min-w-0 truncate text-right font-medium">{value}</span>
        {copyValue && (
          <button
            type="button"
            onClick={() => void handleCopy()}
            className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            title={copied ? t('admin.copied') : t('admin.copy')}
            aria-label={copied ? t('admin.copied') : t('admin.copy')}
          >
            {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5" />}
          </button>
        )}
      </span>
    </div>
  )
}

function MiniTile({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: string }) {
  return (
    <div className={surface('raised', 'flex min-w-0 items-center gap-2.5 rounded-lg p-3')}>
      <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
        <Icon className="size-4" />
      </div>
      <div className="min-w-0">
        <p className="truncate text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</p>
        <p className="truncate text-sm font-semibold">{value}</p>
      </div>
    </div>
  )
}

export function AdminPaymentDetailModal({ paymentId, onClose }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const open = paymentId != null
  const { data, isLoading } = useAdminPayment(paymentId)

  const handleOpenUser = (customerId: number) => {
    onClose()
    navigate(`/admin/users/${customerId}`)
  }

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={paymentId != null ? t('admin.payments.modalTitle', { id: paymentId }) : ''}
      icon={CreditCard}
      iconAccent="emerald"
      panelClassName="sm:max-w-xl lg:max-w-3xl"
    >
      {isLoading || !data ? (
        <div className="flex items-center justify-center py-10">
          <Loader2 className="size-6 animate-spin text-muted-foreground" />
        </div>
      ) : (
        (() => {
          const status = paymentStatusInfo(data.status, t)
          const amount = formatPaymentAmount(data.amount, data.currency || '', data.invoice_type)
          const username = data.telegram_username ? `@${data.telegram_username}` : (data.panel_login ?? `#${data.customer_id}`)

          return (
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] lg:items-start">
              {/* Левая колонка: сводка платежа + пользователь */}
              <div className="space-y-4">
                <div className={surface('raised', 'rounded-xl p-4')}>
                  <div className="flex items-start gap-3">
                    <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-emerald-500/15 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-400">
                      <Receipt className="size-5" />
                    </div>
                    <div className="min-w-0">
                      <p className="text-2xl font-semibold tabular-nums">{amount.text}</p>
                      <p className="truncate text-sm text-muted-foreground">{describePayment(data, t)}</p>
                    </div>
                  </div>
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    <span className={cn('inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium', status.cls)}>
                      {status.label}
                    </span>
                    <span className="inline-flex items-center rounded-full bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground">
                      {data.invoice_type}
                    </span>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <MiniTile icon={Landmark} label={t('admin.payments.provider')} value={formatInvoiceType(data.invoice_type, t)} />
                  <MiniTile icon={Receipt} label={t('admin.payments.createdAt')} value={formatAdminDateTime(data.created_at)} />
                </div>

                <AdminSectionCard level="raised" title={t('admin.payments.sectionUser')} icon={User} iconAccent="violet">
                  <div className="divide-y divide-border/40">
                    <Row label={t('admin.payments.sectionUser')} value={username} />
                    <Row label={t('admin.users.id')} value={data.customer_id} copyValue={String(data.customer_id)} />
                    <Row label={t('admin.users.telegramId')} value={data.panel_login ? '—' : data.telegram_id} />
                  </div>
                  <button
                    type="button"
                    onClick={() => handleOpenUser(data.customer_id)}
                    className={surface('raised', 'mt-3 flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm font-medium text-primary transition-colors hover:bg-accent')}
                  >
                    <UserRound className="size-4 shrink-0" />
                    {t('admin.payments.openUserCard')}
                  </button>
                </AdminSectionCard>
              </div>

              {/* Правая колонка: платёж / провайдер / покупка */}
              <div className="space-y-4">
                <AdminSectionCard level="raised" title={t('admin.payments.sectionPayment')} icon={Receipt} iconAccent="emerald">
                  <div className="divide-y divide-border/40">
                    <Row label="ID" value={`#${data.id}`} copyValue={String(data.id)} />
                    <Row label={t('admin.payments.amount')} value={amount.text} />
                    <Row label={t('admin.payments.statusLabel')} value={status.label} />
                    <Row label={t('admin.payments.createdAt')} value={formatAdminDateTime(data.created_at)} />
                    {data.paid_at && <Row label={t('admin.payments.paidAt')} value={formatAdminDateTime(data.paid_at)} />}
                    <Row label={t('admin.payments.description')} value={describePayment(data, t)} />
                  </div>
                </AdminSectionCard>

                <AdminSectionCard level="raised" title={t('admin.payments.sectionProvider')} icon={Landmark} iconAccent="blue">
                  <div className="divide-y divide-border/40">
                    <Row label={t('admin.payments.provider')} value={formatInvoiceType(data.invoice_type, t)} />
                    <Row
                      label={t('admin.payments.providerTxnId')}
                      value={data.provider_txn_id ?? '—'}
                      copyValue={data.provider_txn_id ?? undefined}
                    />
                    <Row
                      label={t('admin.payments.idempotencyKey')}
                      value={data.idempotency_key ?? '—'}
                      copyValue={data.idempotency_key ?? undefined}
                    />
                  </div>
                </AdminSectionCard>

                <AdminSectionCard level="raised" title={t('admin.payments.sectionPurchase')} icon={Package} iconAccent="amber">
                  <div className="divide-y divide-border/40">
                    <Row label={t('admin.payments.kindLabel')} value={formatPurchaseKind(data.purchase_kind, t)} />
                    {data.tariff_name && <Row label={t('admin.payments.tariff')} value={data.tariff_name} />}
                    {data.tariff_location_name && (
                      <Row label={t('admin.payments.location')} value={data.tariff_location_name} />
                    )}
                    <Row label={t('admin.payments.period')} value={data.month > 0 ? t('admin.users.monthsShort', { count: data.month }) : '—'} />
                    <Row label={t('admin.payments.extraHwid')} value={data.extra_hwid > 0 ? `+${data.extra_hwid}` : '—'} />
                    <Row
                      label={t('admin.payments.promoCode')}
                      value={
                        data.promo_code
                          ? `${data.promo_code}${data.discount_percent ? ` (-${data.discount_percent}%)` : ''}`
                          : '—'
                      }
                    />
                    {data.is_early_downgrade && (
                      <Row label={t('admin.payments.earlyDowngrade')} value={t('admin.yes')} />
                    )}
                  </div>
                </AdminSectionCard>
              </div>
            </div>
          )
        })()
      )}
    </AdminModal>
  )
}
