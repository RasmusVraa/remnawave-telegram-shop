import type { TFunction } from 'i18next'

const STATUS_STYLES: Record<string, { labelKey: string; cls: string }> = {
  paid: { labelKey: 'admin.payments.status.paid', cls: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' },
  pending: { labelKey: 'admin.payments.status.pending', cls: 'bg-amber-500/15 text-amber-700 dark:text-amber-400' },
  new: { labelKey: 'admin.payments.status.new', cls: 'bg-blue-500/15 text-blue-700 dark:text-blue-400' },
  cancel: { labelKey: 'admin.payments.status.cancel', cls: 'bg-red-500/15 text-red-700 dark:text-red-400' },
}

export function paymentStatusInfo(status: string, t: TFunction): { label: string; cls: string } {
  const entry = STATUS_STYLES[status]
  return {
    label: entry ? t(entry.labelKey) : status,
    cls: entry?.cls ?? 'bg-muted text-muted-foreground',
  }
}

const KIND_LABEL_KEYS: Record<string, string> = {
  subscription: 'admin.payments.kind.subscription',
  tariff_upgrade: 'admin.payments.kind.tariffUpgrade',
  extra_hwid: 'admin.payments.kind.extraHwid',
  extra_traffic: 'admin.payments.kind.extraTraffic',
}

export function formatPurchaseKind(kind: string, t: TFunction): string {
  const key = KIND_LABEL_KEYS[kind]
  return key ? t(key) : kind
}

/** Короткое человекочитаемое описание платежа для колонки «Описание» в списке. */
export function describePayment(
  item: { month: number; extra_hwid: number; purchase_kind: string; tariff_name?: string | null },
  t: TFunction,
): string {
  const parts: string[] = []
  if (item.month > 0) {
    parts.push(t('admin.users.monthsShort', { count: item.month }))
  }
  if (item.extra_hwid > 0) {
    parts.push(t('admin.payments.extraHwidShort', { count: item.extra_hwid }))
  }
  if (parts.length === 0) {
    return formatPurchaseKind(item.purchase_kind, t)
  }
  const base = parts.join(' + ')
  return item.tariff_name ? `${item.tariff_name} · ${base}` : base
}
