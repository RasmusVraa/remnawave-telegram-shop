import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Handshake,
  Pause,
  Play,
  Ban,
  Scale,
  SlidersHorizontal,
  Loader2,
  Copy,
  Check,
  LayoutGrid,
  Link2,
  Users,
  ArrowLeftRight,
  Banknote,
  Coins,
  Wallet,
  Sparkles,
  RefreshCw,
  CreditCard,
  FileText,
  type LucideIcon,
} from 'lucide-react'

import { AdminModal } from './AdminModal'
import {
  useAdminPartnerDetail,
  useAdminPartnerSetStatus,
  useAdminPartnerUpdateTerms,
  useAdminPartnerAdjust,
  useAdminPartnerCustomers,
  useAdminPartnerOperations,
  useAdminPartnerPayoutHistory,
} from '../hooks/useAdminPartners'
import { formatMoney, formatPercent, formatDayShort, formatDayMonth } from '@/features/partner/format'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { RecordCard } from '@/components/RecordCard'
import { cn } from '@/lib/utils'
import { surface } from './Surface'
import { useErrorHaptic } from '@/lib/haptic'
import type {
  AdminPartnerDTO,
  AdminPartnerCustomerDTO,
  AdminPartnerOperationDTO,
  AdminPartnerPayoutDTO,
} from '@/lib/types/admin'

type TabId = 'overview' | 'links' | 'customers' | 'operations' | 'payouts'

/**
 * Карточка партнёра в модалке.
 *
 * Разбор спора и обработка выплаты — это один заход: админ смотрит, сколько
 * человек заработал, откуда пришли его клиенты и что уже выплачено. Отдельная
 * страница уводила бы из очереди заявок и заставляла возвращаться назад.
 */
export function AdminPartnerModal({
  partnerID,
  onClose,
}: {
  partnerID: number | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<TabId>('overview')
  const { data, isLoading } = useAdminPartnerDetail(partnerID)

  const setStatus = useAdminPartnerSetStatus()
  const partner = data?.partner

  // Счётчики приходят с карточкой, а сами журналы — постранично из своих
  // ручек: у активного партнёра там сотни строк, и грузить их все ради
  // числа на вкладке незачем.
  const tabs: { id: TabId; label: string; icon: LucideIcon; count?: number }[] = [
    { id: 'overview', label: t('admin.partners.detail.tabOverview'), icon: LayoutGrid },
    { id: 'links', label: t('admin.partners.detail.linksTitle'), icon: Link2, count: data?.links.length },
    {
      id: 'customers',
      label: t('admin.partners.detail.tabCustomers'),
      icon: Users,
      count: data?.counts.customers,
    },
    {
      id: 'operations',
      label: t('admin.partners.detail.operationsTitle'),
      icon: ArrowLeftRight,
      count: data?.counts.operations,
    },
    { id: 'payouts', label: t('admin.partners.tabs.payouts'), icon: Banknote, count: data?.counts.payouts },
  ]

  return (
    <AdminModal
      open={partnerID != null}
      onClose={onClose}
      title={partner?.label ?? t('admin.partners.title')}
      icon={Handshake}
      /*
       * Ширину задаём с тем же брейкпоинтом, что и дефолт AdminModal
       * (sm:max-w-md): без префикса twMerge считает классы разными
       * группами, оба доезжают до разметки, и медиазапрос перебивает
       * безусловный max-w-5xl — модалка оставалась узкой.
       */
      panelClassName="sm:max-w-3xl lg:max-w-5xl"
    >
      {isLoading || !partner ? (
        <div className="flex justify-center py-10">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <StatusChip status={partner.status} />
            <span className="text-xs text-muted-foreground">
              {t('admin.partners.detail.clientSince', { date: formatDayShort(partner.customer_since) })} ·{' '}
              {t('admin.partners.applications.clientPaid', {
                count: partner.customer_paid_count,
                sum: formatMoney(partner.customer_paid_sum),
              })}
            </span>
          </div>

          {/* Заморозка блокирует вывод, но не начисления; отзыв снимает
              партнёрство совсем — после него человек может подать заявку заново. */}
          <div className="flex flex-wrap gap-2">
            {partner.status === 'active' ? (
              <ActionButton
                icon={Pause}
                label={t('admin.partners.detail.suspend')}
                busy={setStatus.isPending}
                loading={setStatus.isPending && setStatus.variables?.status === 'suspended'}
                tone="primary"
                onClick={() => setStatus.mutate({ id: partner.id, status: 'suspended' })}
              />
            ) : null}
            {partner.status === 'suspended' ? (
              <ActionButton
                icon={Play}
                label={t('admin.partners.detail.resume')}
                busy={setStatus.isPending}
                loading={setStatus.isPending && setStatus.variables?.status === 'active'}
                tone="primary"
                onClick={() => setStatus.mutate({ id: partner.id, status: 'active' })}
              />
            ) : null}
            {partner.status !== 'rejected' ? (
              <ActionButton
                icon={Ban}
                label={t('admin.partners.detail.revoke')}
                busy={setStatus.isPending}
                loading={setStatus.isPending && setStatus.variables?.status === 'rejected'}
                tone="danger"
                onClick={() => setStatus.mutate({ id: partner.id, status: 'rejected' })}
              />
            ) : null}
            {partner.status === 'rejected' ? (
              <ActionButton
                icon={Play}
                label={t('admin.partners.detail.restore')}
                busy={setStatus.isPending}
                loading={setStatus.isPending && setStatus.variables?.status === 'active'}
                tone="primary"
                onClick={() => setStatus.mutate({ id: partner.id, status: 'active' })}
              />
            ) : null}
          </div>

          <div role="tablist" className="flex gap-1 overflow-x-auto rounded-xl bg-muted p-1 lg:overflow-visible">
            {tabs.map((item) => (
              <button
                key={item.id}
                role="tab"
                type="button"
                aria-selected={tab === item.id}
                onClick={() => setTab(item.id)}
                className={cn(
                  'flex flex-1 items-center justify-center gap-1.5 whitespace-nowrap rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors',
                  tab === item.id ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
                )}
              >
                <item.icon className={cn('size-3.5 shrink-0', tab === item.id && 'text-primary')} />
                {item.label}
                {item.count ? ` · ${item.count}` : ''}
              </button>
            ))}
          </div>

          {tab === 'overview' ? <OverviewTab partner={partner} /> : null}
          {tab === 'links' ? <LinksTab links={data.links} /> : null}
          {tab === 'customers' ? <CustomersTab partnerID={partner.id} /> : null}
          {tab === 'operations' ? <OperationsTab partnerID={partner.id} /> : null}
          {tab === 'payouts' ? <PayoutsTab partnerID={partner.id} /> : null}
        </div>
      )}
    </AdminModal>
  )
}

function OverviewTab({ partner }: { partner: AdminPartnerDTO }) {
  const { t } = useTranslation()
  const [termsOpen, setTermsOpen] = useState(false)
  const [adjustOpen, setAdjustOpen] = useState(false)

  return (
    <div className="space-y-3">
      <div className="grid gap-2 sm:grid-cols-3 lg:gap-3">
        <Metric
          icon={Coins}
          label={t('admin.partners.detail.earnedTotal')}
          value={formatMoney(partner.total_earned)}
          sub={t('admin.partners.detail.paidTotal', { amount: formatMoney(partner.total_paid) })}
        />
        <Metric
          icon={Wallet}
          label={t('admin.partners.detail.available')}
          value={formatMoney(partner.balance)}
          sub={t('admin.partners.detail.holdReserved', {
            hold: formatMoney(partner.hold_balance),
            reserved: formatMoney(partner.reserved_balance),
          })}
        />
        <Metric
          icon={Users}
          label={t('admin.partners.detail.customers')}
          value={String(partner.customers)}
          sub={t('admin.partners.detail.paying', { n: partner.paying_customers })}
        />
      </div>

      <dl className={surface('raised', 'divide-y divide-border rounded-lg text-sm')}>
        <Row
          icon={Sparkles}
          label={t('admin.partners.terms.first')}
          value={
            <>
              {formatPercent(partner.effective_first_percent)}{' '}
              <span className="text-xs text-muted-foreground">
                {partner.first_percent == null ? t('admin.partners.terms.global') : t('admin.partners.terms.individual')}
              </span>
            </>
          }
        />
        <Row
          icon={RefreshCw}
          label={t('admin.partners.terms.renewal')}
          value={
            <>
              {formatPercent(partner.effective_renewal_percent)}{' '}
              <span className="text-xs text-muted-foreground">
                {partner.renewal_percent == null
                  ? t('admin.partners.terms.global')
                  : t('admin.partners.terms.individual')}
              </span>
            </>
          }
        />
        {/* Только число: лимит потоков — это ответ на вопрос «сколько», а
            откуда он взялся, видно в форме изменения условий. */}
        <Row icon={Link2} label={t('admin.partners.terms.linksLimit')} value={partner.effective_links_limit} />
        <Row
          icon={CreditCard}
          label={t('admin.partners.detail.payoutDetails')}
          value={<CopyValue value={partner.payout_details || ''} />}
        />
      </dl>

      {partner.app_about ? (
        <div className={surface('raised', 'rounded-lg p-3 text-sm')}>
          <p className="mb-1 flex items-center gap-1.5 text-xs uppercase tracking-wide text-muted-foreground">
            <FileText className="size-3.5 shrink-0 text-primary" />
            {t('admin.partners.detail.application')}
          </p>
          <p>{partner.app_about}</p>
          {partner.app_channels ? (
            <p className="mt-1 font-mono text-xs text-muted-foreground">{partner.app_channels}</p>
          ) : null}
        </div>
      ) : null}

      {partner.admin_note ? <p className="text-xs text-muted-foreground">{partner.admin_note}</p> : null}

      <div className="flex flex-wrap gap-2">
        <ActionButton icon={SlidersHorizontal} label={t('admin.partners.detail.editTerms')} busy={false} onClick={() => setTermsOpen(true)} />
        <ActionButton icon={Scale} label={t('admin.partners.detail.adjust')} busy={false} onClick={() => setAdjustOpen(true)} />
      </div>

      <TermsModal partner={partner} open={termsOpen} onClose={() => setTermsOpen(false)} />
      <AdjustModal partner={partner} open={adjustOpen} onClose={() => setAdjustOpen(false)} />
    </div>
  )
}

function LinksTab({ links }: { links: { id: number; name: string; code: string; archived: boolean; customers: number; paying: number; earned: number }[] }) {
  const { t } = useTranslation()
  if (links.length === 0) return <Empty text={t('admin.partners.detail.linksEmpty')} />
  return (
    <ul className={surface('raised', 'divide-y divide-border rounded-lg text-sm')}>
      {links.map((l) => (
        <li key={l.id} className={cn('flex items-center justify-between gap-3 px-3 py-2.5', l.archived && 'opacity-60')}>
          <div className="min-w-0">
            <p className="truncate font-medium">{l.name}</p>
            <p className="font-mono text-xs text-muted-foreground">{l.code}</p>
          </div>
          <div className="shrink-0 text-right text-xs text-muted-foreground">
            <p>
              {l.customers} / {l.paying}
            </p>
            <p className="tabular-nums">{formatMoney(l.earned)}</p>
          </div>
        </li>
      ))}
    </ul>
  )
}

function CustomersTab({ partnerID }: { partnerID: number }) {
  const { t } = useTranslation()
  const query = useAdminPartnerCustomers(partnerID, true)
  const items: AdminPartnerCustomerDTO[] = query.data?.pages.flatMap((p) => p.items) ?? []

  if (query.isLoading) return <TabLoader />
  if (items.length === 0) return <Empty text={t('admin.partners.detail.customersEmpty')} />

  return (
    <>
      <ul className={surface('raised', 'divide-y divide-border rounded-lg text-sm')}>
        {items.map((c, i) => (
          <li key={`${c.label}-${i}`} className="flex items-center justify-between gap-3 px-3 py-2.5">
            <div className="min-w-0">
              <p className="truncate font-mono text-xs">{c.label}</p>
              <p className="text-xs text-muted-foreground">
                {[c.link_name, formatDayShort(c.attached_at)].filter(Boolean).join(' · ')}
              </p>
            </div>
            <div className="shrink-0 text-right">
              <span
                className={cn(
                  'rounded-full px-2 py-0.5 text-[11px] font-medium',
                  c.has_paid && c.active
                    ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
                    : 'bg-secondary text-muted-foreground',
                )}
              >
                {c.has_paid
                  ? c.active
                    ? t('partnerPage.customers.paying')
                    : t('partnerPage.customers.expired')
                  : t('partnerPage.customers.notPaid')}
              </span>
              <p className="mt-0.5 text-xs tabular-nums text-muted-foreground">{formatMoney(c.earned)}</p>
            </div>
          </li>
        ))}
      </ul>
      <ShowMore query={query} />
    </>
  )
}

function OperationsTab({ partnerID }: { partnerID: number }) {
  const { t } = useTranslation()
  const query = useAdminPartnerOperations(partnerID, true)
  const items: AdminPartnerOperationDTO[] = query.data?.pages.flatMap((p) => p.items) ?? []

  if (query.isLoading) return <TabLoader />
  if (items.length === 0) return <Empty text={t('admin.partners.detail.operationsEmpty')} />

  return (
    <>
      {/* Карточки только на узком экране, таблица — начиная с sm. Одни и те же
          данные в двух вёрстках: колоночная таблица в 360 точек не помещается,
          а карточка на десктопе тратит высоту впустую. */}
      <div className="space-y-2 sm:hidden">
        {items.map((op, i) => (
          <RecordCard
            key={`m-${op.at}-${i}`}
            rows={[
              { label: t('admin.partners.detail.opDate'), value: formatDayShort(op.at) },
              {
                label: t('admin.partners.detail.opKind'),
                value:
                  op.kind === 'payout'
                    ? t('admin.partners.detail.opPayout')
                    : t(`admin.partners.detail.opEarning.${op.detail || 'renewal'}`),
              },
              {
                label: t('admin.partners.detail.opAmount'),
                value: (
                  <span
                    className={cn(
                      'tabular-nums',
                      op.status === 'cancelled'
                        ? 'text-muted-foreground line-through'
                        : op.amount < 0
                          ? 'text-foreground'
                          : 'text-emerald-600 dark:text-emerald-400',
                    )}
                  >
                    {op.amount < 0 ? '−' : '+'}
                    {formatMoney(Math.abs(op.amount))}
                  </span>
                ),
              },
              { label: t('admin.partners.detail.opRef'), value: op.ref || op.note || '—', mono: true },
            ]}
            footer={<OperationStatusChip status={op.status} block />}
          />
        ))}
      </div>

      <div className={surface('raised', 'hidden overflow-x-auto rounded-lg sm:block')}>
        <table className="w-full min-w-[520px] border-collapse text-sm">
          <thead>
            <tr className="text-left text-[10px] uppercase tracking-wide text-muted-foreground">
              <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.detail.opDate')}</th>
              <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.detail.opKind')}</th>
              <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.detail.opRef')}</th>
              <th className="px-3 pb-2 pt-3 text-right font-medium">{t('admin.partners.detail.opAmount')}</th>
              <th className="px-3 pb-2 pt-3 text-right font-medium">{t('admin.partners.detail.opStatus')}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((op, i) => (
              <tr key={`${op.at}-${i}`} className="border-t border-border">
                <td className="whitespace-nowrap px-3 py-2">{formatDayShort(op.at)}</td>
                <td className="px-3 py-2">
                  {op.kind === 'payout'
                    ? t('admin.partners.detail.opPayout')
                    : t(`admin.partners.detail.opEarning.${op.detail || 'renewal'}`)}
                </td>
                <td className="px-3 py-2 text-xs text-muted-foreground">{op.ref || op.note || '—'}</td>
                {/*
                  Минус здесь — это выплата партнёру, а не авария, поэтому
                  красным он не красится. Отменённое начисление гасим
                  зачёркиванием: сумма в баланс не попала.
                */}
                <td
                  className={cn(
                    'px-3 py-2 text-right font-medium tabular-nums',
                    op.status === 'cancelled'
                      ? 'text-muted-foreground line-through'
                      : op.amount < 0
                        ? 'text-foreground'
                        : 'text-emerald-600 dark:text-emerald-400',
                  )}
                >
                  {op.amount < 0 ? '−' : '+'}
                  {formatMoney(Math.abs(op.amount))}
                </td>
                <td className="px-3 py-2 text-right">
                  <OperationStatusChip status={op.status} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ShowMore query={query} />
    </>
  )
}

function PayoutsTab({ partnerID }: { partnerID: number }) {
  const { t } = useTranslation()
  const query = useAdminPartnerPayoutHistory(partnerID, true)
  const items: AdminPartnerPayoutDTO[] = query.data?.pages.flatMap((p) => p.items) ?? []

  if (query.isLoading) return <TabLoader />
  if (items.length === 0) return <Empty text={t('admin.partners.payouts.empty')} />

  return (
    <>
      <ul className={surface('raised', 'divide-y divide-border rounded-lg text-sm')}>
        {items.map((p) => (
          <li key={p.id} className="flex items-start justify-between gap-3 px-3 py-2.5">
            <div className="min-w-0">
              <p className="font-semibold tabular-nums">{formatMoney(p.amount)}</p>
              <p className="text-xs text-muted-foreground">
                {[formatDayMonth(p.requested_at), p.method].filter(Boolean).join(' · ')}
              </p>
              {p.external_ref ? <p className="font-mono text-xs text-muted-foreground">{p.external_ref}</p> : null}
              {p.admin_comment ? <p className="text-xs text-muted-foreground">{p.admin_comment}</p> : null}
            </div>
            <PayoutStatusChip status={p.status} />
          </li>
        ))}
      </ul>
      <ShowMore query={query} />
    </>
  )
}

/** Догрузка следующей страницы вкладки. */
function ShowMore({
  query,
}: {
  query: { hasNextPage: boolean; isFetchingNextPage: boolean; fetchNextPage: () => unknown }
}) {
  const { t } = useTranslation()
  if (!query.hasNextPage) return null
  return (
    <button
      type="button"
      disabled={query.isFetchingNextPage}
      onClick={() => void query.fetchNextPage()}
      className={cn(
        'mt-3 inline-flex w-full items-center justify-center gap-2 rounded-lg border border-border bg-secondary px-3 py-2 text-xs font-medium transition-colors hover:bg-border disabled:opacity-60',
        query.isFetchingNextPage && '!opacity-100',
      )}
    >
      {query.isFetchingNextPage && <Loader2 className="size-3.5 animate-spin" />}
      {t('admin.partners.detail.showMore')}
    </button>
  )
}

function TabLoader() {
  return (
    <div className="flex justify-center py-8">
      <Loader2 className="size-5 animate-spin text-muted-foreground" />
    </div>
  )
}

/**
 * Значение с кнопкой копирования.
 *
 * Реквизиты и номера переводов переносят в банковское приложение руками —
 * выделять их мышью из строки таблицы неудобно и легко прихватить лишнее.
 */
export function CopyValue({ value }: { value: string }) {
  const { state, copy } = useCopyToClipboard()
  if (!value) return <span className="text-muted-foreground">—</span>
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="font-mono text-xs">{value}</span>
      <button
        type="button"
        onClick={() => void copy(value)}
        className="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        aria-label="copy"
      >
        {state === 'done' ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
      </button>
    </span>
  )
}

/**
 * Статус строки в ленте операций.
 *
 * Раньше сюда падало сырое значение из базы («available», «pending») — админ
 * читал английский слаг и додумывал смысл. Цвет по общему правилу: успех
 * зелёный, отказ красный, ожидание синее.
 */
function OperationStatusChip({ status, block }: { status: string; block?: boolean }) {
  const { t } = useTranslation()
  const map: Record<string, { label: string; className: string }> = {
    available: { label: t('partnerPage.earnings.available'), className: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400' },
    hold: { label: t('partnerPage.earnings.hold'), className: 'bg-primary/15 text-primary' },
    cancelled: { label: t('partnerPage.earnings.cancelled'), className: 'bg-destructive/15 text-destructive' },
    paid: { label: t('partnerPage.payouts.statusPaid'), className: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400' },
    approved: { label: t('partnerPage.payouts.statusApproved'), className: 'bg-primary/15 text-primary' },
    pending: { label: t('partnerPage.payouts.statusPending'), className: 'bg-primary/15 text-primary' },
    rejected: { label: t('partnerPage.payouts.statusRejected'), className: 'bg-destructive/15 text-destructive' },
  }
  const view = map[status] ?? { label: status, className: 'bg-secondary text-muted-foreground' }
  return (
    <span
      className={cn(
        'rounded-full px-2 py-0.5 text-[11px] font-medium',
        block && 'block rounded-lg py-1.5 text-center text-xs',
        view.className,
      )}
    >
      {view.label}
    </span>
  )
}

export function PayoutStatusChip({ status, block }: { status: string; block?: boolean }) {
  const { t } = useTranslation()
  const styles: Record<string, string> = {
    paid: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
    approved: 'bg-sky-500/15 text-sky-600 dark:text-sky-400',
    pending: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    rejected: 'bg-destructive/15 text-destructive',
  }
  return (
    <span
      className={cn(
        'shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium',
        block && 'block rounded-lg py-1.5 text-center text-xs',
        styles[status] ?? styles.rejected,
      )}
    >
      {t(`partnerPage.payouts.status${status.charAt(0).toUpperCase()}${status.slice(1)}`)}
    </span>
  )
}

function StatusChip({ status }: { status: string }) {
  const { t } = useTranslation()
  const styles: Record<string, string> = {
    active: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
    pending: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    suspended: 'bg-rose-500/15 text-rose-600 dark:text-rose-400',
    rejected: 'bg-muted text-muted-foreground',
  }
  return (
    <span className={cn('rounded-full px-2 py-0.5 text-[11px] font-medium', styles[status] ?? styles.rejected)}>
      {t(`admin.partners.status.${status}`)}
    </span>
  )
}

/**
 * Кнопка действия над партнёром.
 *
 * Смена статуса меняет доступ к деньгам, поэтому такие кнопки залиты цветом, а
 * вспомогательные (условия, корректировка) остаются контурными: так в ряду
 * видно, что здесь главное.
 */
function ActionButton({
  icon: Icon,
  label,
  onClick,
  busy,
  loading = false,
  tone = 'outline',
}: {
  icon: LucideIcon
  label: string
  onClick: () => void
  busy: boolean
  /** Спиннер у той кнопки, чей запрос идёт; busy блокирует все соседние. */
  loading?: boolean
  tone?: 'outline' | 'primary' | 'danger'
}) {
  const tones: Record<string, string> = {
    outline: 'border-border bg-secondary hover:bg-border',
    primary: 'border-transparent bg-primary text-primary-foreground hover:bg-primary/90',
    danger: 'border-transparent bg-destructive text-destructive-foreground hover:bg-destructive/90',
  }
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={busy || loading}
      className={cn(
        'flex items-center gap-1.5 rounded-lg border px-3 py-2 text-xs font-medium transition-colors disabled:opacity-60',
        tones[tone],
        loading && '!opacity-100',
      )}
    >
      {loading ? <Loader2 className="size-3.5 animate-spin" /> : <Icon className="size-3.5" />}
      {label}
    </button>
  )
}

function Metric({
  icon: Icon,
  label,
  value,
  sub,
}: {
  icon: LucideIcon
  label: string
  value: string
  sub: string
}) {
  return (
    <div className={surface('raised', 'rounded-lg p-3')}>
      <p className="flex items-center gap-1.5 text-[10px] uppercase tracking-wide text-muted-foreground">
        <Icon className="size-3.5 shrink-0 text-primary" />
        {label}
      </p>
      <p className="mt-0.5 text-lg font-semibold tabular-nums">{value}</p>
      <p className="text-xs text-muted-foreground">{sub}</p>
    </div>
  )
}

function Row({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon
  label: string
  value: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2.5">
      <dt className="flex items-center gap-2 text-muted-foreground">
        <Icon className="size-4 shrink-0" />
        {label}
      </dt>
      <dd className="text-right font-medium">{value}</dd>
    </div>
  )
}

function Empty({ text }: { text: string }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{text}</p>
}

function TermsModal({ partner, open, onClose }: { partner: AdminPartnerDTO; open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const update = useAdminPartnerUpdateTerms()
  const [first, setFirst] = useState(partner.first_percent == null ? '' : String(partner.first_percent))
  const [renewal, setRenewal] = useState(partner.renewal_percent == null ? '' : String(partner.renewal_percent))
  const [limit, setLimit] = useState(partner.links_limit == null ? '' : String(partner.links_limit))
  const [comment, setComment] = useState('')

  // Пустое поле — «как у всех». Number('') === 0 назначил бы нулевую ставку.
  function parseOrNull(v: string): number | null {
    const trimmed = v.trim()
    if (!trimmed) return null
    const n = Number(trimmed.replace(',', '.'))
    return Number.isFinite(n) ? n : null
  }

  return (
    <AdminModal open={open} onClose={onClose} title={t('admin.partners.detail.editTerms')}>
      <div className="space-y-3">
        <div className="flex gap-2">
          <Field label={t('admin.partners.terms.first')} value={first} onChange={setFirst} placeholder="—" />
          <Field label={t('admin.partners.terms.renewal')} value={renewal} onChange={setRenewal} placeholder="—" />
        </div>
        <Field label={t('admin.partners.terms.linksLimit')} value={limit} onChange={setLimit} placeholder="—" />
        <Field label={t('admin.partners.detail.notePlaceholder')} value={comment} onChange={setComment} />
        <p className="text-xs text-muted-foreground">{t('admin.partners.terms.emptyHint')}</p>
        <button
          type="button"
          disabled={update.isPending}
          onClick={() =>
            update.mutate(
              {
                id: partner.id,
                terms: {
                  first_percent: parseOrNull(first),
                  renewal_percent: parseOrNull(renewal),
                  links_limit: parseOrNull(limit),
                  comment,
                },
              },
              { onSuccess: onClose },
            )
          }
          className={cn(
            'inline-flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-60',
            update.isPending && '!opacity-100',
          )}
        >
          {update.isPending && <Loader2 className="size-4 animate-spin" />}
          {t('admin.partners.detail.save')}
        </button>
      </div>
    </AdminModal>
  )
}

function AdjustModal({ partner, open, onClose }: { partner: AdminPartnerDTO; open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const adjust = useAdminPartnerAdjust()
  const [amount, setAmount] = useState('')
  const [comment, setComment] = useState('')
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  function submit() {
    const value = Number(amount.replace(',', '.'))
    if (!Number.isFinite(value) || value === 0) {
      setError(t('admin.partners.detail.adjustErrors.amount'))
      return
    }
    if (!comment.trim()) {
      setError(t('admin.partners.detail.adjustErrors.comment'))
      return
    }
    setError(null)
    adjust.mutate(
      { id: partner.id, amount: value, comment },
      {
        onSuccess: () => {
          setAmount('')
          setComment('')
          onClose()
        },
        onError: () => setError(t('admin.partners.detail.adjustErrors.failed')),
      },
    )
  }

  return (
    <AdminModal open={open} onClose={onClose} title={t('admin.partners.detail.adjust')}>
      <div className="space-y-3">
        <Field label={t('admin.partners.detail.adjustAmount')} value={amount} onChange={setAmount} placeholder="-500" />
        <p className="text-xs text-muted-foreground">{t('admin.partners.detail.adjustHint')}</p>
        <Field label={t('admin.partners.detail.adjustReason')} value={comment} onChange={setComment} />
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
        <button
          type="button"
          onClick={submit}
          disabled={adjust.isPending}
          className={cn(
            'inline-flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-60',
            adjust.isPending && '!opacity-100',
          )}
        >
          {adjust.isPending && <Loader2 className="size-4 animate-spin" />}
          {t('admin.partners.detail.save')}
        </button>
      </div>
    </AdminModal>
  )
}

function Field({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return (
    <div className="flex-1">
      <label className="mb-1 block text-[10px] uppercase tracking-wide text-muted-foreground">{label}</label>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={surface('well', 'w-full rounded-lg px-3 py-2 text-sm focus:border-primary focus:outline-none')}
      />
    </div>
  )
}
