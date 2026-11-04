import { useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft,
  Calendar,
  ChevronDown,
  ChevronUp,
  PowerOff,
  Trash2,
  CreditCard,
  Users,
  Loader2,
  AlertTriangle,
} from 'lucide-react'
import { useState, useEffect, useMemo } from 'react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminSectionCard } from '../components/AdminSectionCard'
import { AdminSetExpireModal } from '../components/AdminSetExpireModal'
import { AdminFeedback } from '../components/AdminFeedback'
import { AdminUserEditModals } from '../components/user-modals/AdminUserEditModals'
import { AdminUserActionsModal } from '../components/user-modals/AdminUserActionsModal'
import type { UserEditModalKey } from '../components/user-modals/types'
import { ClickableOverviewControl } from '../components/overview/ClickableOverviewControl'
import { AdminUserIdentityCard } from '../components/overview/AdminUserIdentityCard'
import { AdminUserParamTiles } from '../components/overview/AdminUserParamTiles'
import { AdminUserSystemCard } from '../components/overview/AdminUserSystemCard'
import { AdminUserActionsPanel } from '../components/overview/AdminUserActionsPanel'
import { AdminUserMobileActionBar } from '../components/overview/AdminUserMobileActionBar'
import { AdminUserMobileTopBar } from '../components/overview/AdminUserMobileTopBar'
import { useCopySubscriptionLink } from '../components/overview/useCopySubscriptionLink'
import { buildUserCardMetrics } from '../components/overview/userCardMetrics'
import { resolveLoyaltyDiscountPercent } from '../components/overview/loyaltyDiscount'
import { useAdminMutationFeedback } from '../hooks/useAdminMutationFeedback'
import { formatAdminApiError } from '../utils/formatAdminApiError'
import { formatInvoiceType } from '../utils/formatInvoiceType'
import { formatPaymentAmount } from '../utils/formatPaymentAmount'
import { unifiedAccountStatus } from '../utils/accountStatus'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import {
  useAdminUser,
  useAdminUserPanel,
  useAdminUserPayments,
  useAdminUserReferrals,
  useAdminUserDevices,
  useAdminUserSetExpire,
  useAdminUserDisable,
  useAdminUserEnable,
  useAdminUserDelete,
  type AdminPurchaseDTO,
  type AdminRefereeDTO,
} from '../hooks/useAdminUsers'

import { formatAdminDateTime } from '../utils/datetime'
import { resolveTariffLabel } from '../utils/resolveTariffLabel'
import { useAdminTariffList } from '../hooks/useAdminTariffs'
import { useAdminBootstrap } from '../hooks/useAdminBootstrap'
import { useAdminLoyaltyTiers } from '../hooks/useAdminLoyalty'
import type { AdminTariffBriefDTO } from '@/lib/types/admin'
import { AdminModal } from '../components/AdminModal'
import { AdminTablePagination } from '../components/AdminTablePagination'
import { formatNumber } from '@/lib/format'

const LIST_PAGE_LIMIT = 20

/**
 * Сколько строк списка видно в свёрнутом виде.
 *
 * Смысл в симметрии первого экрана: платежи и рефералы должны занимать
 * одинаковую предсказуемую высоту независимо от того, один у человека платёж
 * или сорок. Всё остальное открывается кнопкой.
 */
const PREVIEW_ROWS = 3

/** Кнопка «раскрыть/свернуть список» под таблицей. */
function ListToggle({
  expanded,
  hidden,
  onToggle,
}: {
  expanded: boolean
  hidden: number
  onToggle: () => void
}) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      onClick={onToggle}
      className="mt-3 inline-flex w-full items-center justify-center gap-1.5 rounded-btn border border-border bg-secondary px-3 py-1.5 text-xs font-medium transition-colors hover:bg-accent"
    >
      {expanded ? (
        <>
          <ChevronUp className="size-3.5" />
          {t('admin.users.overview.collapseList')}
        </>
      ) : (
        <>
          <ChevronDown className="size-3.5" />
          {t('admin.users.overview.expandList', { count: hidden })}
        </>
      )}
    </button>
  )
}

/** Пустое состояние карточки: рамка пунктиром, значок и одна строка. */
function EmptyBox({ icon: Icon, text }: { icon: typeof Users; text: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border px-3 py-6 text-center text-xs text-muted-foreground">
      <Icon className="size-5" aria-hidden />
      {text}
    </div>
  )
}

export default function AdminUserDetailPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const userId = id ? parseInt(id, 10) : null
  const { data: user, isLoading, isError } = useAdminUser(userId)
  const { data: panel, isLoading: panelLoading } = useAdminUserPanel(userId)
  const { data: devicesData } = useAdminUserDevices(userId)
  const { data: allTariffs } = useAdminTariffList()
  const { data: bootstrap } = useAdminBootstrap()
  const { data: loyaltyTiers } = useAdminLoyaltyTiers()
  const salesModeTariffs = bootstrap?.sales_mode === 'tariffs'

  const tariffOptions = useMemo((): AdminTariffBriefDTO[] => {
    if (panel?.tariffs?.length) return panel.tariffs
    return (allTariffs ?? []).map((tariff) => ({
      id: tariff.id,
      slug: tariff.slug,
      name: tariff.name?.trim() || tariff.slug,
    }))
  }, [panel?.tariffs, allTariffs])

  const canEditTariff = salesModeTariffs && tariffOptions.length > 0

  const hasRwUser = Boolean(panel?.has_rw_user && panel?.rw)
  const rwStatus = panel?.rw?.status?.toUpperCase()

  const [expireModal, setExpireModal] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [confirmDisable, setConfirmDisable] = useState(false)
  const [disableError, setDisableError] = useState<string | null>(null)
  const [actionsModal, setActionsModal] = useState(false)
  const [editModal, setEditModal] = useState<UserEditModalKey | null>(null)
  const [paymentsPage, setPaymentsPage] = useState(1)
  const [referralsPage, setReferralsPage] = useState(1)
  const [paymentsExpanded, setPaymentsExpanded] = useState(false)
  const [referralsExpanded, setReferralsExpanded] = useState(false)
  const [expireError, setExpireError] = useState<string | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [overviewFeedback, setOverviewFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null)

  const { feedback, clear, handlers, showSuccess } = useAdminMutationFeedback()

  const setExpireMut = useAdminUserSetExpire(userId)
  const disableMut = useAdminUserDisable(userId)
  const enableMut = useAdminUserEnable(userId)
  const deleteMut = useAdminUserDelete(userId)

  const { data: paymentsData, isLoading: paymentsLoading } = useAdminUserPayments(userId, paymentsPage, LIST_PAGE_LIMIT)
  const { data: referralsData, isLoading: referralsLoading } = useAdminUserReferrals(userId, referralsPage, LIST_PAGE_LIMIT)

  const breadcrumbTail = user
    ? (user.telegram_username ? `@${user.telegram_username}` : `#${user.id}`)
    : undefined

  const openExpireModal = () => {
    setExpireError(null)
    setExpireModal(true)
  }

  const paymentsTotalPages = paymentsData
    ? Math.max(1, Math.ceil(paymentsData.total / LIST_PAGE_LIMIT))
    : 1

  const referralsTotalPages = referralsData
    ? Math.max(1, Math.ceil(referralsData.stats.total / LIST_PAGE_LIMIT))
    : 1

  useEffect(() => {
    setPaymentsPage(1)
    setReferralsPage(1)
    setPaymentsExpanded(false)
    setReferralsExpanded(false)
  }, [userId])

  const subscriptionLink =
    user?.subscription_link?.trim() || panel?.rw?.subscription_url?.trim() || ''
  const copy = useCopySubscriptionLink(subscriptionLink)

  if (isLoading) {
    return (
      <AdminLayout>
        <div className="flex justify-center py-20">
          <Loader2 className="size-8 animate-spin text-primary" />
        </div>
      </AdminLayout>
    )
  }

  if (isError || !user) {
    return (
      <AdminLayout>
        <button onClick={() => navigate('/admin/users')} className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors">
          <ArrowLeft className="size-4" /> {t('admin.users.backToList')}
        </button>
        <Card className="border-dashed p-8 text-center">
          <p className="text-muted-foreground">{t('admin.users.notFound')}</p>
        </Card>
      </AdminLayout>
    )
  }

  // Порядок подписей: @username → логин панели (web-клиенты) → #id. Web-клиент
  // до первой покупки иначе опознавался бы только по номеру.
  const displayName = user.telegram_username
    ? `@${user.telegram_username}`
    : user.panel_login
      ? user.panel_login
      : `#${user.id}`

  const tariffId = user.current_tariff_id ?? panel?.customer?.current_tariff_id ?? null
  const tariffName = resolveTariffLabel(tariffId, panel?.tariffs, allTariffs)
  const dateLocale = i18n.language?.startsWith('en') ? 'en-GB' : 'ru-RU'

  const metrics = buildUserCardMetrics({
    user,
    panel,
    devicesUsed: devicesData?.items?.length ?? 0,
  })

  const accountStatus = unifiedAccountStatus({
    status: user.status,
    rwStatus: panel?.rw?.status ?? user.rw_status,
    expireAt: metrics.expireAt,
  })

  const loyaltyDiscount = resolveLoyaltyDiscountPercent(
    user,
    loyaltyTiers,
    bootstrap?.loyalty_enabled ?? false,
  )

  const handleOverviewSuccess = (message: string) => setOverviewFeedback({ type: 'success', message })
  const handleOverviewError = (message: string) => setOverviewFeedback({ type: 'error', message })

  const paymentItems = paymentsData?.items ?? []
  const visiblePayments = paymentsExpanded ? paymentItems : paymentItems.slice(0, PREVIEW_ROWS)
  const referees = referralsData?.referees ?? []
  const visibleReferees = referralsExpanded ? referees : referees.slice(0, PREVIEW_ROWS)

  const tariffBadge = canEditTariff ? (
    <ClickableOverviewControl
      variant="badge"
      onClick={() => setEditModal('tariff')}
      title={t('admin.users.overview.clickToEdit')}
      className={
        tariffName ? undefined : 'border-dashed border-muted-foreground/40 bg-muted/20 text-muted-foreground'
      }
    >
      {tariffName ?? t('admin.users.subscription.assignTariff')}
    </ClickableOverviewControl>
  ) : tariffName ? (
    <span className="inline-flex items-center rounded-full border border-primary/25 bg-primary/10 px-2.5 py-0.5 text-xs font-medium text-primary">
      {tariffName}
    </span>
  ) : null

  return (
    <AdminLayout meta={{ breadcrumbTail, mobileBareHeader: true }}>
      <AdminFeedback feedback={feedback} onDismiss={clear} />
      {overviewFeedback && (
        <AdminFeedback
          feedback={overviewFeedback}
          onDismiss={() => setOverviewFeedback(null)}
        />
      )}

      <AdminUserMobileTopBar label={`#${user.id}`} onOpenActions={() => setActionsModal(true)} />

      {/*
        Раскладка «панель оператора»: слева личность и действия, справа данные.
        Обе колонки начинаются от одной линии, а списки платежей и рефералов
        свёрнуты до трёх строк — иначе карточка с сорока платежами растягивала
        страницу и ломала симметрию первого экрана.

        Ниже lg колонки складываются в одну ленту, а действия переезжают в
        нижнюю панель. Обёртка не должна получать overflow: это отрезает
        `position: sticky` у нижней панели.
      */}
      <div className="grid min-w-0 grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,270px)_minmax(0,1fr)]">
        <div className="flex min-w-0 flex-col gap-4 lg:sticky lg:top-4">
          <AdminUserIdentityCard
            user={user}
            displayName={displayName}
            status={accountStatus}
            tariffBadge={tariffBadge}
            hasRwUser={hasRwUser}
            payments={
              paymentsData ? { rubSum: paymentsData.rub_sum, count: paymentsData.rub_count } : null
            }
            dateLocale={dateLocale}
            copy={copy}
          />
          <div className="hidden lg:block">
            <AdminUserActionsPanel
              hasRwUser={hasRwUser}
              rwStatus={rwStatus}
              onExtend={openExpireModal}
              onDisable={() => setConfirmDisable(true)}
              onEnable={() => enableMut.mutate(undefined, handlers(t('admin.feedback.enableSuccess')))}
              onDelete={() => setConfirmDelete(true)}
              disablePending={disableMut.isPending}
              enablePending={enableMut.isPending}
              copy={copy}
            />
          </div>
        </div>

        <div className="flex min-w-0 flex-col gap-4">
          {panelLoading ? (
            <Card className="cabinet-elevated-card flex justify-center py-10">
              <Loader2 className="size-5 animate-spin text-muted-foreground" />
            </Card>
          ) : (
            <AdminUserParamTiles
              metrics={metrics}
              hasRwUser={hasRwUser}
              dateLocale={dateLocale}
              onOpenModal={setEditModal}
              onOpenExpire={openExpireModal}
            />
          )}

          <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)] xl:items-stretch">
            <AdminSectionCard
              title={t('admin.users.payments')}
              icon={CreditCard}
              iconAccent="emerald"
              fillHeight
              flushHeader
              headerRightDesktopOnly
              className="min-w-0"
              headerRight={
                <span className="text-xs text-muted-foreground">
                  {t('admin.users.overview.allTime')}
                </span>
              }
            >
              {paymentsLoading ? (
                <div className="flex flex-1 items-center justify-center py-6">
                  <Loader2 className="size-5 animate-spin text-muted-foreground" />
                </div>
              ) : paymentsData ? (
                <div className="flex min-h-0 flex-1 flex-col">
                  <div className="mb-4 grid grid-cols-2 gap-3">
                    <div className="rounded-xl border bg-muted/30 p-3">
                      <p className="text-xs text-muted-foreground">{t('admin.users.paymentsRub')}</p>
                      <p className="text-lg font-semibold tabular-nums">{formatNumber(paymentsData.rub_sum)} ₽</p>
                      <p className="text-xs text-muted-foreground">{paymentsData.rub_count} {t('admin.users.paymentsCount')}</p>
                    </div>
                    <div className="rounded-xl border bg-muted/30 p-3">
                      <p className="text-xs text-muted-foreground">{t('admin.users.paymentsStars')}</p>
                      <p className="text-lg font-semibold tabular-nums">{paymentsData.stars_sum} ⭐</p>
                      <p className="text-xs text-muted-foreground">{paymentsData.stars_count} {t('admin.users.paymentsCount')}</p>
                    </div>
                  </div>
                  <div className="min-h-0 flex-1">
                    {paymentItems.length === 0 ? (
                      <EmptyBox icon={CreditCard} text={t('admin.users.overview.noPayments')} />
                    ) : (
                      <>
                      {/*
                        На телефоне таблица из четырёх колонок не помещается и
                        уезжает в горизонтальный скролл. Там те же данные —
                        строкой-карточкой: способ и период сверху, дата под
                        ними, сумма справа.
                      */}
                      <ul className="flex flex-col gap-2 sm:hidden">
                        {visiblePayments.map((p: AdminPurchaseDTO) => (
                          <li
                            key={p.id}
                            className="flex items-center gap-3 rounded-xl border border-border bg-secondary p-2.5"
                          >
                            <div className="min-w-0 flex-1">
                              <p className="truncate text-xs font-medium">
                                {formatInvoiceType(p.invoice_type, t)}
                                {p.month > 0
                                  ? ' \u00b7 ' + t('admin.users.monthsShort', { count: p.month })
                                  : ''}
                              </p>
                              <p className="truncate font-mono text-[10px] tabular-nums text-muted-foreground">
                                {formatAdminDateTime(p.paid_at, dateLocale)}
                              </p>
                            </div>
                            <span className="shrink-0 text-sm font-semibold tabular-nums">
                              {formatPaymentAmount(p.amount, p.currency || '', p.invoice_type).text}
                            </span>
                          </li>
                        ))}
                      </ul>
                      <div className="hidden overflow-x-auto sm:block">
                        <table className="w-full text-sm">
                          <thead>
                            <tr className="border-b text-left text-xs text-muted-foreground">
                              <th className="pb-2 pr-4 font-normal">{t('admin.users.paymentDate')}</th>
                              <th className="pb-2 pr-4 font-normal">{t('admin.users.paymentAmount')}</th>
                              <th className="pb-2 pr-4 font-normal">{t('admin.users.paymentType')}</th>
                              <th className="pb-2 font-normal">{t('admin.users.paymentPeriod')}</th>
                            </tr>
                          </thead>
                          <tbody className="divide-y divide-border/50">
                            {visiblePayments.map((p: AdminPurchaseDTO) => (
                              <tr key={p.id}>
                                <td className="whitespace-nowrap py-2 pr-4 tabular-nums">{formatAdminDateTime(p.paid_at, dateLocale)}</td>
                                <td className="whitespace-nowrap py-2 pr-4 font-mono">
                                  {formatPaymentAmount(p.amount, p.currency || '', p.invoice_type).text}
                                </td>
                                <td className="py-2 pr-4">{formatInvoiceType(p.invoice_type, t)}</td>
                                <td className="py-2">{p.month > 0 ? t('admin.users.monthsShort', { count: p.month }) : '—'}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                      </>
                    )}
                  </div>
                  {paymentItems.length > PREVIEW_ROWS && (
                    <ListToggle
                      expanded={paymentsExpanded}
                      hidden={paymentItems.length - PREVIEW_ROWS}
                      onToggle={() => setPaymentsExpanded((v) => !v)}
                    />
                  )}
                  {paymentsExpanded && paymentsTotalPages > 1 && (
                    <AdminTablePagination
                      page={paymentsPage}
                      totalPages={paymentsTotalPages}
                      onPageChange={setPaymentsPage}
                      className="mt-3 flex items-center justify-between border-t border-border pt-3"
                    />
                  )}
                </div>
              ) : null}
            </AdminSectionCard>

            <AdminSectionCard
              title={t('admin.users.referrals')}
              icon={Users}
              iconAccent="rose"
              fillHeight
              flushHeader
              className="min-w-0"
            >
              {referralsLoading ? (
                <div className="flex flex-1 items-center justify-center py-6">
                  <Loader2 className="size-5 animate-spin text-muted-foreground" />
                </div>
              ) : referralsData ? (
                <div className="flex min-h-0 flex-1 flex-col">
                  <div className="mb-4 flex flex-wrap gap-2">
                    {/* Приглашено — все приведённые, активные и нет. */}
                    {[
                      { labelKey: 'admin.users.referralsInvited', value: referralsData.stats.total },
                      { labelKey: 'admin.users.referralsPaid', value: referralsData.stats.paid },
                      { labelKey: 'admin.users.referralsDays', value: referralsData.stats.earned_total },
                    ].map(({ labelKey, value }) => (
                      <div
                        key={labelKey}
                        className="min-w-[88px] flex-1 rounded-xl border bg-muted/30 p-2 text-center"
                      >
                        <p className="text-[10px] text-muted-foreground">{t(labelKey)}</p>
                        <p className="font-semibold tabular-nums">{value}</p>
                      </div>
                    ))}
                  </div>
                  <div className="min-h-0 flex-1">
                    {referees.length === 0 ? (
                      <EmptyBox icon={Users} text={t('admin.users.overview.noReferrals')} />
                    ) : (
                      <div className="overflow-x-auto">
                        <table className="w-full text-sm">
                          <thead>
                            <tr className="border-b text-left text-xs text-muted-foreground">
                              <th className="pb-2 pr-4 font-normal">{t('admin.users.username')}</th>
                              <th className="pb-2 font-normal">{t('admin.users.status')}</th>
                            </tr>
                          </thead>
                          <tbody className="divide-y divide-border/50">
                            {visibleReferees.map((ref: AdminRefereeDTO) => (
                              <tr key={ref.telegram_id}>
                                {/* Без ника подписью остаётся номер: иначе строку не опознать. */}
                                <td className="py-2 pr-4">
                                  {ref.telegram_username ? `@${ref.telegram_username}` : `#${ref.telegram_id}`}
                                </td>
                                <td className="py-2">
                                  <span className={cn('rounded-full px-2 py-0.5 text-xs', ref.active ? 'bg-emerald-500/15 text-emerald-600' : 'bg-muted text-muted-foreground')}>
                                    {ref.active ? t('admin.users.statusActive') : t('admin.users.referralInactive')}
                                  </span>
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
                  {referees.length > PREVIEW_ROWS && (
                    <ListToggle
                      expanded={referralsExpanded}
                      hidden={referees.length - PREVIEW_ROWS}
                      onToggle={() => setReferralsExpanded((v) => !v)}
                    />
                  )}
                  {referralsExpanded && referralsTotalPages > 1 && (
                    <AdminTablePagination
                      page={referralsPage}
                      totalPages={referralsTotalPages}
                      onPageChange={setReferralsPage}
                      className="mt-3 flex items-center justify-between border-t border-border pt-3"
                    />
                  )}
                </div>
              ) : null}
            </AdminSectionCard>
          </div>

          <AdminUserSystemCard
            user={user}
            panel={panel}
            hasRwUser={hasRwUser}
            loyaltyDiscount={loyaltyDiscount}
            dateLocale={dateLocale}
            onOpenModal={setEditModal}
          />
        </div>
      </div>

      <AdminUserMobileActionBar onExtend={openExpireModal} onOpenActions={() => setActionsModal(true)} />

      <AdminUserEditModals
        userId={userId!}
        panel={panel}
        customer={user}
        tariffs={tariffOptions}
        tariffsEnabled={salesModeTariffs}
        activeModal={editModal}
        onClose={() => setEditModal(null)}
        onSuccess={handleOverviewSuccess}
        onError={handleOverviewError}
      />

      <AdminUserActionsModal
        open={actionsModal}
        onClose={() => setActionsModal(false)}
        hasRwUser={hasRwUser}
        rwStatus={rwStatus}
        onExtend={openExpireModal}
        onDisable={() => setConfirmDisable(true)}
        onEnable={() => enableMut.mutate(undefined, handlers(t('admin.feedback.enableSuccess')))}
        onDelete={() => setConfirmDelete(true)}
        disablePending={disableMut.isPending}
        enablePending={enableMut.isPending}
        copy={copy}
        onChangeTariff={canEditTariff ? () => setEditModal('tariff') : undefined}
      />

      <AdminSetExpireModal
        open={expireModal}
        onClose={() => { setExpireModal(false); setExpireError(null) }}
        title={t('admin.users.subscription.expire')}
        icon={Calendar}
        iconAccent="amber"
        currentExpireAt={metrics.expireAt}
        isPending={setExpireMut.isPending}
        error={expireError}
        onClearError={() => setExpireError(null)}
        onApply={(iso) => {
          setExpireError(null)
          setExpireMut.mutate(iso, {
            onSuccess: () => {
              setExpireModal(false)
              showSuccess(t('admin.feedback.extendSuccess'))
            },
            onError: (e) => setExpireError(formatAdminApiError(e, t)),
          })
        }}
      />

      <AdminModal
        open={confirmDisable}
        onClose={() => { setConfirmDisable(false); setDisableError(null) }}
        title={t('admin.users.disable')}
        icon={PowerOff}
        iconTone="danger"
        size="sm"
      >
        <div className="space-y-4">
          <div className="flex gap-3 rounded-xl border border-destructive/30 bg-destructive/5 p-3">
            <AlertTriangle className="size-5 shrink-0 text-destructive" />
            <p className="text-sm text-muted-foreground">
              {t('admin.users.disableWarning', { name: displayName })}
            </p>
          </div>
          {disableError && (
            <p className="rounded-btn border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {disableError}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button
              onClick={() => { setConfirmDisable(false); setDisableError(null) }}
              className="rounded-btn border px-4 py-2 text-sm hover:bg-accent"
            >
              {t('admin.cancel')}
            </button>
            <button
              onClick={() => {
                setDisableError(null)
                disableMut.mutate(undefined, {
                  onSuccess: () => {
                    setConfirmDisable(false)
                    showSuccess(t('admin.feedback.disableSuccess'))
                  },
                  onError: (e) => setDisableError(formatAdminApiError(e, t)),
                })
              }}
              disabled={disableMut.isPending}
              className={cn('inline-flex items-center gap-2 rounded-btn bg-destructive px-4 py-2 text-sm text-destructive-foreground disabled:opacity-50', disableMut.isPending && '!opacity-100')}
            >
              {disableMut.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              {t('admin.users.disable')}
            </button>
          </div>
        </div>
      </AdminModal>

      <AdminModal
        open={confirmDelete}
        onClose={() => { setConfirmDelete(false); setDeleteError(null) }}
        title={t('admin.users.delete')}
        icon={Trash2}
        iconTone="danger"
        size="sm"
      >
        <div className="space-y-4">
          {/*
            Удаление необратимо, поэтому окно перечисляет последствия и
            предлагает отключение как безопасную замену: чаще всего админу
            нужно именно закрыть доступ, а не стереть историю.
          */}
          <div className="flex gap-3 rounded-xl border border-destructive/30 bg-destructive/5 p-3">
            <AlertTriangle className="size-5 shrink-0 text-destructive" />
            <div className="min-w-0 text-sm text-muted-foreground">
              <p>{t('admin.users.deleteWarning', { name: displayName })}</p>
              <ul className="mt-2 list-disc space-y-1 ps-4">
                <li>{t('admin.users.deleteConsequences.panel', { count: metrics.devicesUsed })}</li>
                {metrics.days != null && metrics.days > 0 && (
                  <li>{t('admin.users.deleteConsequences.days', { count: metrics.days })}</li>
                )}
                <li>{t('admin.users.deleteConsequences.payments')}</li>
              </ul>
            </div>
          </div>
          {deleteError && (
            <p className="rounded-btn border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {deleteError}
            </p>
          )}
          <div className="flex items-center justify-end gap-2">
            <button onClick={() => { setConfirmDelete(false); setDeleteError(null) }} className="rounded-btn border px-4 py-2 text-sm hover:bg-accent">
              {t('admin.cancel')}
            </button>
            <button
              onClick={() => {
                setDeleteError(null)
                deleteMut.mutate(undefined, {
                  onSuccess: () => navigate('/admin/users'),
                  onError: (e) => setDeleteError(formatAdminApiError(e, t)),
                })
              }}
              disabled={deleteMut.isPending}
              className={cn('inline-flex items-center gap-2 rounded-btn bg-destructive px-4 py-2 text-sm text-destructive-foreground disabled:opacity-50', deleteMut.isPending && '!opacity-100')}
            >
              {deleteMut.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              {t('admin.delete')}
            </button>
          </div>
        </div>
      </AdminModal>
    </AdminLayout>
  )
}
