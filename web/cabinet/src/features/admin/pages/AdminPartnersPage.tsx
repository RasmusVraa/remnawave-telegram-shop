import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Handshake, Check, X, UserPlus, Loader2, AlertTriangle, FileText, Users, Banknote, type LucideIcon } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminModal } from '../components/AdminModal'
import { AdminPartnerModal, PayoutStatusChip, CopyValue } from '../components/AdminPartnerModal'
import {
  PartnerStatusFilter,
  loadPartnerStatusFilter,
  type PartnerStatus,
} from '../components/PartnerStatusFilter'
import {
  useAdminPartners,
  useAdminPartnerApprove,
  useAdminPartnerReject,
  useAdminPartnerPending,
  useAdminPartnerPayouts,
  useAdminPartnerPayoutAction,
  useAdminPartnerGrant,
} from '../hooks/useAdminPartners'
import { formatMoney, formatPercent, formatDayShort } from '@/features/partner/format'
import { RecordCard } from '@/components/RecordCard'
import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'
import type { AdminPartnerDTO, AdminPartnerPayoutDTO } from '@/lib/types/admin'
import { useErrorHaptic } from '@/lib/haptic'

type TabId = 'applications' | 'partners' | 'payouts'

const TAB_IDS: TabId[] = ['applications', 'partners', 'payouts']

function isTabId(value: string | null): value is TabId {
  return value !== null && (TAB_IDS as string[]).includes(value)
}

export default function AdminPartnersPage() {
  const { t } = useTranslation()
  // Вкладка живёт в адресе, чтобы на неё можно было сослаться снаружи:
  // «Заявок на выплату» с дашборда должно открывать выплаты, а не заявки.
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const tab: TabId = isTabId(tabParam) ? tabParam : 'applications'
  const setTab = (next: TabId) => {
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev)
        params.set('tab', next)
        return params
      },
      { replace: true },
    )
  }
  // Карточка партнёра открывается модалкой поверх очереди: разбор заявки и
  // обработка выплаты — один заход, уходить со страницы незачем.
  const [openPartner, setOpenPartner] = useState<number | null>(null)

  const pending = useAdminPartnerPending()
  const applications = useAdminPartners(['pending'])
  const [statusFilter, setStatusFilter] = useState<PartnerStatus[]>(loadPartnerStatusFilter)
  const partners = useAdminPartners(statusFilter)
  const applicationItems = applications.data?.pages.flatMap((p) => p.items) ?? []
  const partnerItems = partners.data?.pages.flatMap((p) => p.items) ?? []

  // Заявки и выплаты — несделанная работа, их счётчики подсвечены. Число
  // партнёров — просто справка, и выделять его незачем.
  const tabs = useMemo(
    () => [
      {
        id: 'applications' as const,
        label: t('admin.partners.tabs.applications'),
        count: pending.data?.applications,
        icon: FileText,
        urgent: true,
      },
      {
        id: 'partners' as const,
        label: t('admin.partners.tabs.partners'),
        count: partners.data?.pages[0]?.total,
        icon: Users,
        urgent: false,
      },
      {
        id: 'payouts' as const,
        label: t('admin.partners.tabs.payouts'),
        count: pending.data?.payouts,
        icon: Banknote,
        urgent: true,
      },
    ],
    [t, pending.data, partners.data],
  )

  return (
    <AdminLayout>
      <div className="space-y-5">
        <AdminPageHeader
          icon={Handshake}
          title={t('admin.partners.title')}
          subtitle={t('admin.partners.subtitle')}
          accent="amber"
        />

        {/* Пропущенные из-за незаданного курса звезды начисления — это деньги,
            которых партнёр не получил. Молчать о них нельзя. */}
        {pending.data && pending.data.skipped_stars_earnings > 0 ? (
          <div className="flex items-start gap-2 rounded-xl border border-amber-500/30 bg-amber-500/5 p-3 text-sm text-amber-700 dark:text-amber-400">
            <AlertTriangle className="mt-0.5 size-4 shrink-0" />
            <span>{t('admin.partners.starsWarning', { n: pending.data.skipped_stars_earnings })}</span>
          </div>
        ) : null}

        <div role="tablist" className="flex gap-1 overflow-x-auto rounded-xl bg-muted p-1">
          {tabs.map((item) => (
            <TabButton
              key={item.id}
              icon={item.icon}
              label={item.label}
              count={item.count}
              urgent={item.urgent}
              active={tab === item.id}
              onClick={() => setTab(item.id)}
            />
          ))}
        </div>

        {tab === 'applications' ? (
          <ApplicationsTab items={applicationItems} loading={applications.isLoading} more={applications} />
        ) : null}
        {tab === 'partners' ? (
          <PartnersTab
            items={partnerItems}
            loading={partners.isLoading}
            onOpen={setOpenPartner}
            more={partners}
            statusFilter={statusFilter}
            onStatusFilter={setStatusFilter}
          />
        ) : null}
        {tab === 'payouts' ? <PayoutsTab onOpenPartner={setOpenPartner} /> : null}

        <AdminPartnerModal partnerID={openPartner} onClose={() => setOpenPartner(null)} />
      </div>
    </AdminLayout>
  )
}

/**
 * Кнопка вкладки со счётчиком.
 *
 * urgent — счётчик означает несделанную работу (заявки, необработанные
 * выплаты) и подсвечивается; иначе это просто справочное число, и подсветка
 * приучала бы игнорировать её и там, где она важна.
 */
function TabButton({
  icon: Icon,
  label,
  count,
  urgent,
  active,
  onClick,
}: {
  icon: LucideIcon
  label: string
  count?: number
  urgent: boolean
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      role="tab"
      type="button"
      aria-selected={active}
      onClick={onClick}
      className={cn(
        'flex flex-1 items-center justify-center gap-1.5 whitespace-nowrap rounded-lg px-3 py-1.5 text-xs font-medium transition-colors',
        active ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
      )}
    >
      <Icon size={14} className={cn('shrink-0', active && 'text-primary')} />
      {label}
      {count ? (
        <span
          className={cn(
            'rounded-full px-1.5 py-0.5 text-[10px] font-semibold',
            urgent ? 'bg-primary text-primary-foreground' : 'bg-secondary text-muted-foreground',
          )}
        >
          {count}
        </span>
      ) : null}
    </button>
  )
}

/** Заявки: проценты задаются прямо здесь, до одобрения. */
function ApplicationsTab({
  items,
  loading,
  more,
}: {
  items: AdminPartnerDTO[]
  loading: boolean
  more: PageQuery
}) {
  const { t } = useTranslation()
  if (loading) return <LoadingBlock />
  if (items.length === 0) return <EmptyBlock text={t('admin.partners.applications.empty')} />

  return (
    <div className="space-y-3">
      {items.map((row) => (
        <ApplicationCard key={row.id} partner={row} />
      ))}
      <ShowMoreButton query={more} />
    </div>
  )
}

/** Постраничный запрос: нужны только эти три поля, форма источника неважна. */
interface PageQuery {
  hasNextPage: boolean
  isFetchingNextPage: boolean
  fetchNextPage: () => unknown
}

function ShowMoreButton({ query }: { query: PageQuery }) {
  const { t } = useTranslation()
  if (!query.hasNextPage) return null
  return (
    <button
      type="button"
      disabled={query.isFetchingNextPage}
      onClick={() => void query.fetchNextPage()}
      className={cn('flex w-full items-center justify-center gap-1.5 rounded-lg border border-border bg-secondary px-3 py-2 text-xs font-medium transition-colors hover:bg-border disabled:opacity-60', query.isFetchingNextPage && '!opacity-100')}
    >
      {query.isFetchingNextPage && <Spinner className="h-3.5 w-3.5" />}
      {query.isFetchingNextPage ? t('admin.partners.detail.loading') : t('admin.partners.detail.showMore')}
    </button>
  )
}

function ApplicationCard({ partner }: { partner: AdminPartnerDTO }) {
  const { t } = useTranslation()
  const approve = useAdminPartnerApprove()
  const reject = useAdminPartnerReject()

  const [first, setFirst] = useState(String(partner.effective_first_percent))
  const [renewal, setRenewal] = useState(String(partner.effective_renewal_percent))
  const [comment, setComment] = useState('')

  const busy = approve.isPending || reject.isPending

  // Пустое поле означает «брать общее значение из настроек», а не 0%:
  // Number('') === 0, и без явной проверки одобрение назначало бы партнёру
  // нулевую ставку, при которой он не зарабатывает ничего.
  function parsePercent(v: string): number | null {
    const trimmed = v.trim()
    if (!trimmed) return null
    const n = Number(trimmed.replace(',', '.'))
    return Number.isFinite(n) ? n : null
  }

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex flex-wrap gap-4">
        <div className="min-w-[240px] flex-1 space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold">{partner.label}</span>
            <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-600 dark:text-amber-400">
              {t('admin.partners.status.pending')}
            </span>
            <span className="text-xs text-muted-foreground">{formatDayShort(partner.app_submitted_at)}</span>
          </div>
          {partner.app_about ? <p className="text-sm text-muted-foreground">{partner.app_about}</p> : null}
          {partner.app_channels ? (
            <p className="text-xs text-muted-foreground">
              {t('admin.partners.applications.channels')}: <span className="font-mono">{partner.app_channels}</span>
              {partner.app_expected ? ` · ${t('admin.partners.applications.expected')}: ${partner.app_expected}` : ''}
            </p>
          ) : null}
          {/* История человека как клиента: по ней видно, живой это аккаунт или
              регистрация, заведённая ради заявки. */}
          <p className="text-xs text-muted-foreground">
            {t('admin.partners.applications.clientSince', { date: formatDayShort(partner.customer_since) })} ·{' '}
            {t('admin.partners.applications.clientPaid', {
              count: partner.customer_paid_count,
              sum: formatMoney(partner.customer_paid_sum),
            })}
          </p>
        </div>

        <div className="w-full space-y-2 sm:w-[260px]">
          <div className="flex items-center gap-2">
            <PercentInput label={t('admin.partners.terms.first')} value={first} onChange={setFirst} />
            <PercentInput label={t('admin.partners.terms.renewal')} value={renewal} onChange={setRenewal} />
          </div>
          <textarea
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            rows={2}
            placeholder={t('admin.partners.applications.commentPlaceholder')}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-xs focus:border-primary focus:outline-none"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={() =>
                approve.mutate({
                  id: partner.id,
                  terms: { first_percent: parsePercent(first), renewal_percent: parsePercent(renewal), comment },
                })
              }
              className={cn('flex flex-1 items-center justify-center gap-1 rounded-lg bg-primary px-3 py-2 text-xs font-medium text-primary-foreground disabled:opacity-60', approve.isPending && '!opacity-100')}
            >
              {approve.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <Check className="size-3.5" />}
              {t('admin.partners.applications.approve')}
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={() => reject.mutate({ id: partner.id, comment })}
              className={cn('flex flex-1 items-center justify-center gap-1 rounded-lg border border-border bg-secondary px-3 py-2 text-xs font-medium transition-colors hover:bg-border disabled:opacity-60', reject.isPending && '!opacity-100')}
            >
              {reject.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <X className="size-3.5" />}
              {t('admin.partners.applications.reject')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

/** Список партнёров плюс ручное назначение. */
function PartnersTab({
  items,
  loading,
  onOpen,
  more,
  statusFilter,
  onStatusFilter,
}: {
  items: AdminPartnerDTO[]
  loading: boolean
  onOpen: (id: number) => void
  more: PageQuery
  statusFilter: PartnerStatus[]
  onStatusFilter: (next: PartnerStatus[]) => void
}) {
  const { t } = useTranslation()
  const [grantOpen, setGrantOpen] = useState(false)

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-end gap-2">
        <PartnerStatusFilter value={statusFilter} onChange={onStatusFilter} />
        <button
          type="button"
          onClick={() => setGrantOpen(true)}
          className="flex items-center gap-1.5 rounded-lg border border-border bg-secondary px-3 py-2 text-xs font-medium transition-colors hover:bg-border"
        >
          <UserPlus className="size-3.5" />
          {t('admin.partners.grant.open')}
        </button>
      </div>

      {loading ? <LoadingBlock /> : null}
      {!loading && items.length === 0 ? (
        <EmptyBlock
          text={
            statusFilter.length
              ? t('admin.partners.filter.emptyFiltered')
              : t('admin.partners.list.empty')
          }
        />
      ) : null}

      {/* Карточки до sm, таблица начиная с sm: семь колонок в 360 точек не
          помещаются, а горизонтальная прокрутка прячет за краем ровно те
          числа, ради которых в список и заходят. */}
      {items.length > 0 ? (
        <div className="space-y-2 sm:hidden">
          {items.map((row) => (
            <RecordCard
              key={`m-${row.id}`}
              onClick={() => onOpen(row.id)}
              rows={[
                {
                  label: t('admin.partners.list.partner'),
                  value: (
                    <>
                      {row.label}
                      <p className="text-xs font-normal text-muted-foreground">
                        {t('admin.partners.list.since', { date: formatDayShort(row.approved_at || row.created_at) })}
                      </p>
                    </>
                  ),
                },
                {
                  label: t('admin.partners.list.percents'),
                  value: (
                    <span className="tabular-nums">
                      {formatPercent(row.effective_first_percent)} / {formatPercent(row.effective_renewal_percent)}
                    </span>
                  ),
                },
                {
                  label: t('admin.partners.list.customers'),
                  value: (
                    <span className="tabular-nums">
                      {row.customers}
                      <span className="text-muted-foreground"> / {row.paying_customers}</span>
                    </span>
                  ),
                },
                {
                  label: t('admin.partners.list.earned'),
                  value: <span className="tabular-nums">{formatMoney(row.total_earned)}</span>,
                },
                {
                  label: t('admin.partners.list.balance'),
                  value: <span className="tabular-nums">{formatMoney(row.balance)}</span>,
                },
                ...(row.open_payouts > 0
                  ? [
                      {
                        label: t('admin.partners.list.payouts'),
                        value: (
                          <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-600 dark:text-amber-400">
                            {t('admin.partners.list.openPayouts', { n: row.open_payouts })}
                          </span>
                        ),
                      },
                    ]
                  : []),
              ]}
              footer={<StatusChip status={row.status} block />}
            />
          ))}
        </div>
      ) : null}

      {items.length > 0 ? (
        <div className="hidden overflow-x-auto rounded-xl border border-border bg-card sm:block">
          <table className="w-full min-w-[720px] border-collapse text-sm">
            <thead>
              <tr className="text-left text-[10px] uppercase tracking-wide text-muted-foreground">
                <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.list.partner')}</th>
                <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.list.status')}</th>
                <th className="px-3 pb-2 pt-3 text-right font-medium">{t('admin.partners.list.percents')}</th>
                <th className="px-3 pb-2 pt-3 text-right font-medium">{t('admin.partners.list.customers')}</th>
                <th className="px-3 pb-2 pt-3 text-right font-medium">{t('admin.partners.list.earned')}</th>
                <th className="px-3 pb-2 pt-3 text-right font-medium">{t('admin.partners.list.balance')}</th>
                <th className="px-3 pb-2 pt-3 text-right font-medium">{t('admin.partners.list.payouts')}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((row) => (
                <tr
                  key={row.id}
                  className="cursor-pointer border-t border-border hover:bg-accent/40"
                  onClick={() => onOpen(row.id)}
                >
                  <td className="px-3 py-2.5">
                    <span className="font-medium">{row.label}</span>
                    <p className="text-xs text-muted-foreground">
                      {t('admin.partners.list.since', { date: formatDayShort(row.approved_at || row.created_at) })}
                    </p>
                  </td>
                  <td className="px-3 py-2.5">
                    <StatusChip status={row.status} />
                  </td>
                  <td className="px-3 py-2.5 text-right tabular-nums">
                    {formatPercent(row.effective_first_percent)} / {formatPercent(row.effective_renewal_percent)}
                  </td>
                  <td className="px-3 py-2.5 text-right tabular-nums">
                    {row.customers}
                    <span className="text-muted-foreground"> / {row.paying_customers}</span>
                  </td>
                  <td className="px-3 py-2.5 text-right tabular-nums">{formatMoney(row.total_earned)}</td>
                  <td className="px-3 py-2.5 text-right font-medium tabular-nums">{formatMoney(row.balance)}</td>
                  <td className="px-3 py-2.5 text-right">
                    {row.open_payouts > 0 ? (
                      <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-600 dark:text-amber-400">
                        {t('admin.partners.list.openPayouts', { n: row.open_payouts })}
                      </span>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <ShowMoreButton query={more} />

      <GrantModal open={grantOpen} onClose={() => setGrantOpen(false)} />
    </div>
  )
}

/**
 * Ручное назначение партнёром.
 *
 * Договорённости чаще случаются в личке, чем через форму: заставлять человека
 * подавать формальную заявку ради галочки — лишний шаг, на котором теряются
 * реальные партнёры.
 */
function GrantModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const grant = useAdminPartnerGrant()
  const [telegramID, setTelegramID] = useState('')
  const [first, setFirst] = useState('')
  const [renewal, setRenewal] = useState('')
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  function submit() {
    const tg = Number(telegramID.trim())
    if (!Number.isFinite(tg) || tg === 0) {
      setError(t('admin.partners.grant.errors.telegram'))
      return
    }
    setError(null)
    grant.mutate(
      {
        telegram_id: tg,
        // Пустое поле означает «как у всех», а не ноль.
        first_percent: first.trim() ? Number(first.replace(',', '.')) : null,
        renewal_percent: renewal.trim() ? Number(renewal.replace(',', '.')) : null,
      },
      {
        onSuccess: () => {
          setTelegramID('')
          onClose()
        },
        onError: () => setError(t('admin.partners.grant.errors.generic')),
      },
    )
  }

  return (
    <AdminModal open={open} onClose={onClose} title={t('admin.partners.grant.title')}>
      <div className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            {t('admin.partners.grant.telegramID')}
          </label>
          <input
            value={telegramID}
            onChange={(e) => setTelegramID(e.target.value)}
            inputMode="numeric"
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none"
          />
        </div>
        <div className="flex gap-2">
          <PercentInput label={t('admin.partners.terms.first')} value={first} onChange={setFirst} placeholder="—" />
          <PercentInput label={t('admin.partners.terms.renewal')} value={renewal} onChange={setRenewal} placeholder="—" />
        </div>
        <p className="text-xs text-muted-foreground">{t('admin.partners.grant.hint')}</p>
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
        <button
          type="button"
          onClick={submit}
          disabled={grant.isPending}
          className={cn('flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-60', grant.isPending && '!opacity-100')}
        >
          {grant.isPending && <Spinner />}
          {t('admin.partners.grant.submit')}
        </button>
      </div>
    </AdminModal>
  )
}

/**
 * Выплаты: очередь необработанных и история.
 *
 * История нужна не реже очереди: по ней сверяют «когда и по какому переводу
 * платили», когда партнёр приходит с вопросом через месяц.
 */
function PayoutsTab({ onOpenPartner }: { onOpenPartner: (id: number) => void }) {
  const { t } = useTranslation()
  const [scope, setScope] = useState<'open' | 'all'>('open')
  const payouts = useAdminPartnerPayouts(scope === 'open' ? 'open' : undefined)
  const items = payouts.data?.pages.flatMap((p) => p.items) ?? []

  // Заявка в работе — это задача, обработанная — запись в архиве. Вёрстка у
  // них разная, поэтому список делится здесь, а не внутри карточки.
  const open = items.filter((row) => row.status === 'pending' || row.status === 'approved')
  const closed = items.filter((row) => row.status !== 'pending' && row.status !== 'approved')

  return (
    <div className="space-y-3">
      <div className="flex gap-1 rounded-lg bg-muted p-1 text-xs">
        {(['open', 'all'] as const).map((id) => (
          <button
            key={id}
            type="button"
            onClick={() => setScope(id)}
            className={cn(
              'flex-1 rounded-md px-3 py-1.5 font-medium transition-colors',
              scope === id ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t(`admin.partners.payouts.scope.${id}`)}
          </button>
        ))}
      </div>

      {payouts.isLoading ? <LoadingBlock /> : null}
      {!payouts.isLoading && items.length === 0 ? (
        <EmptyBlock text={t(scope === 'open' ? 'admin.partners.payouts.empty' : 'admin.partners.payouts.historyEmpty')} />
      ) : null}

      {/* Заголовки нужны только там, где на экране обе группы: во вкладке
          «В работе» они повторяли бы название самой вкладки. */}
      {open.length > 0 ? (
        <>
          {closed.length > 0 ? <SectionLabel text={t('admin.partners.payouts.sectionOpen')} /> : null}
          <div className="space-y-3">
            {open.map((row) => (
              <PayoutCard key={row.id} payout={row} onOpenPartner={onOpenPartner} />
            ))}
          </div>
        </>
      ) : null}

      {closed.length > 0 ? (
        <>
          {open.length > 0 ? <SectionLabel text={t('admin.partners.payouts.sectionDone')} /> : null}
          <ProcessedPayoutsTable items={closed} onOpenPartner={onOpenPartner} />
        </>
      ) : null}

      <ShowMoreButton query={payouts} />
    </div>
  )
}

function SectionLabel({ text }: { text: string }) {
  return <p className="pt-1 text-[10px] uppercase tracking-wide text-muted-foreground">{text}</p>
}

/**
 * Обработанные выплаты — таблицей, а не карточками.
 *
 * Карточка нужна там, где есть работа: поля для чека и кнопки. У закрытой
 * заявки работы нет, и карточного размера ей доставалась пустая правая
 * колонка. История партнёра («заработал / выплачено») здесь тоже не
 * повторяется: это его свойство, одинаковое во всех его строках, и решение по
 * закрытой заявке принимать уже не нужно — за ним есть карточка партнёра.
 */
function ProcessedPayoutsTable({
  items,
  onOpenPartner,
}: {
  items: AdminPartnerPayoutDTO[]
  onOpenPartner: (id: number) => void
}) {
  const { t } = useTranslation()

  return (
    <>
      {/* На телефоне запись разворачивается вертикально, а статус занимает всю
          ширину: именно он отвечает на вопрос «чем кончилось», и в узкой
          колонке таблицы его приходилось искать прокруткой. */}
      <div className="space-y-2 sm:hidden">
        {items.map((row) => (
          <RecordCard
            key={`m-${row.id}`}
            rows={[
              {
                label: t('admin.partners.payouts.colDate'),
                value: (
                  <>
                    {formatDayShort(row.requested_at)}
                    <p className="text-xs font-normal text-muted-foreground">
                      {t('admin.partners.payouts.index', { n: row.payout_index })}
                    </p>
                  </>
                ),
              },
              {
                label: t('admin.partners.payouts.colPartner'),
                value: (
                  <button
                    type="button"
                    onClick={() => onOpenPartner(row.partner_id)}
                    className="font-medium hover:underline"
                  >
                    {row.partner_label}
                  </button>
                ),
              },
              {
                label: t('admin.partners.payouts.colAmount'),
                value: <span className="tabular-nums">{formatMoney(row.amount)}</span>,
              },
              {
                label: t('admin.partners.payouts.colDetails'),
                value: <CopyValue value={row.details_snapshot || ''} />,
              },
              {
                label: t('admin.partners.payouts.colResult'),
                value: (
                  <>
                    {row.external_ref ? (
                      <p className="font-mono text-xs">
                        {t('admin.partners.payouts.refDone', { ref: row.external_ref })}
                      </p>
                    ) : null}
                    {row.admin_comment ? (
                      <p className="text-xs font-normal text-muted-foreground">{row.admin_comment}</p>
                    ) : null}
                    {row.processed_at ? (
                      <p className="text-xs font-normal text-muted-foreground">
                        {t('admin.partners.payouts.processed', { date: formatDayShort(row.processed_at) })}
                      </p>
                    ) : null}
                    {!row.external_ref && !row.admin_comment && !row.processed_at ? <span>—</span> : null}
                  </>
                ),
              },
            ]}
            footer={<PayoutStatusChip status={row.status} block />}
          />
        ))}
      </div>

      <div className="hidden overflow-x-auto rounded-xl border border-border bg-card sm:block">
        <table className="w-full min-w-[720px] border-collapse text-sm">
        <thead>
          <tr className="text-left text-[10px] uppercase tracking-wide text-muted-foreground">
            <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.payouts.colDate')}</th>
            <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.payouts.colPartner')}</th>
            <th className="px-3 pb-2 pt-3 text-right font-medium">{t('admin.partners.payouts.colAmount')}</th>
            <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.payouts.colStatus')}</th>
            <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.payouts.colDetails')}</th>
            <th className="px-3 pb-2 pt-3 font-medium">{t('admin.partners.payouts.colResult')}</th>
          </tr>
        </thead>
        <tbody>
          {items.map((row) => (
            <tr key={row.id} className="border-t border-border align-top">
              {/* Дата подачи, а не обработки: колонка та же, что и в очереди,
                  поэтому строка не меняет смысл при переходе из одной группы в
                  другую. Дата обработки — в последней колонке, рядом с чеком. */}
              <td className="whitespace-nowrap px-3 py-2.5 text-xs text-muted-foreground">
                <p>{formatDayShort(row.requested_at)}</p>
                <p>{t('admin.partners.payouts.index', { n: row.payout_index })}</p>
              </td>
              <td className="px-3 py-2.5">
                <button
                  type="button"
                  onClick={() => onOpenPartner(row.partner_id)}
                  className="font-medium hover:underline"
                >
                  {row.partner_label}
                </button>
              </td>
              <td className="whitespace-nowrap px-3 py-2.5 text-right font-semibold tabular-nums">
                {formatMoney(row.amount)}
              </td>
              <td className="px-3 py-2.5">
                <PayoutStatusChip status={row.status} />
              </td>
              <td className="px-3 py-2.5">
                <CopyValue value={row.details_snapshot || ''} />
              </td>
              <td className="px-3 py-2.5 text-xs text-muted-foreground">
                {row.external_ref ? (
                  <p className="font-mono">{t('admin.partners.payouts.refDone', { ref: row.external_ref })}</p>
                ) : null}
                {row.admin_comment ? <p>{row.admin_comment}</p> : null}
                {row.processed_at ? (
                  <p>{t('admin.partners.payouts.processed', { date: formatDayShort(row.processed_at) })}</p>
                ) : null}
                {!row.external_ref && !row.admin_comment && !row.processed_at ? <span>—</span> : null}
              </td>
            </tr>
          ))}
        </tbody>
        </table>
      </div>
    </>
  )
}

function PayoutCard({
  payout,
  onOpenPartner,
}: {
  payout: AdminPartnerPayoutDTO
  onOpenPartner: (id: number) => void
}) {
  const { t } = useTranslation()
  const action = useAdminPartnerPayoutAction()
  const [externalRef, setExternalRef] = useState('')
  const [comment, setComment] = useState('')

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex flex-wrap gap-4">
        <div className="min-w-[220px] flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => onOpenPartner(payout.partner_id)}
              className="font-semibold hover:underline"
            >
              {payout.partner_label}
            </button>
            <span className="text-lg font-semibold tabular-nums">{formatMoney(payout.amount)}</span>
            <PayoutStatusChip status={payout.status} />
            <span className="text-xs text-muted-foreground">
              {t('admin.partners.payouts.requested', { date: formatDayShort(payout.requested_at) })} ·{' '}
              {t('admin.partners.payouts.index', { n: payout.payout_index })}
            </span>
          </div>
          <CopyValue value={payout.details_snapshot || ''} />
          {/* История партнёра: по ней за секунду видно нормальную заявку и
              подозрительную. */}
          <p className="text-xs text-muted-foreground">
            {t('admin.partners.payouts.history', {
              earned: formatMoney(payout.partner_total_earned),
              paid: formatMoney(payout.partner_total_paid),
            })}
          </p>
        </div>

        <div className="w-full space-y-2 sm:w-[280px]">
          <input
            value={externalRef}
            onChange={(e) => setExternalRef(e.target.value)}
            placeholder={t('admin.partners.payouts.refPlaceholder')}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-xs focus:border-primary focus:outline-none"
          />
          <input
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder={t('admin.partners.payouts.commentPlaceholder')}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-xs focus:border-primary focus:outline-none"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={action.isPending}
              onClick={() => action.mutate({ id: payout.id, action: 'paid', externalRef, comment })}
              className={cn('flex flex-1 items-center justify-center gap-1 rounded-lg bg-primary px-3 py-2 text-xs font-medium text-primary-foreground disabled:opacity-60', action.isPending && action.variables?.action === 'paid' && '!opacity-100')}
            >
              {action.isPending && action.variables?.action === 'paid' && <Spinner className="h-3.5 w-3.5" />}
              {t('admin.partners.payouts.markPaid')}
            </button>
            <button
              type="button"
              disabled={action.isPending}
              onClick={() => action.mutate({ id: payout.id, action: 'reject', comment })}
              className={cn('flex flex-1 items-center justify-center gap-1 rounded-lg border border-border bg-secondary px-3 py-2 text-xs font-medium transition-colors hover:bg-border disabled:opacity-60', action.isPending && action.variables?.action === 'reject' && '!opacity-100')}
            >
              {action.isPending && action.variables?.action === 'reject' && <Spinner className="h-3.5 w-3.5" />}
              {t('admin.partners.payouts.reject')}
            </button>
          </div>
          <p className="text-[11px] text-muted-foreground">{t('admin.partners.payouts.refHint')}</p>
        </div>
      </div>
    </div>
  )
}

// --- мелочи ---

export function StatusChip({ status, block }: { status: string; block?: boolean }) {
  const { t } = useTranslation()
  const styles: Record<string, string> = {
    active: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
    pending: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    suspended: 'bg-rose-500/15 text-rose-600 dark:text-rose-400',
    rejected: 'bg-muted text-muted-foreground',
  }
  return (
    <span
      className={cn(
        'rounded-full px-2 py-0.5 text-[11px] font-medium',
        block && 'block rounded-lg py-1.5 text-center text-xs',
        styles[status] ?? styles.rejected,
      )}
    >
      {t(`admin.partners.status.${status}`)}
    </span>
  )
}

export function PercentInput({
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
        inputMode="decimal"
        placeholder={placeholder}
        className="w-full rounded-lg border border-border bg-background px-2 py-1.5 text-sm focus:border-primary focus:outline-none"
      />
    </div>
  )
}

function LoadingBlock() {
  return (
    <div className="flex justify-center rounded-xl border border-border bg-card py-10">
      <Loader2 className="size-5 animate-spin text-muted-foreground" />
    </div>
  )
}

function EmptyBlock({ text }: { text: string }) {
  return (
    <div className="rounded-xl border border-border bg-card py-10 text-center text-sm text-muted-foreground">
      {text}
    </div>
  )
}
