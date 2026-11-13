import { useState, type FormEvent } from 'react'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CreditCard, History } from 'lucide-react'

import { RevealItem } from '@/components/PageReveal'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { api, ApiError, type PartnerAccountDTO } from '@/lib/api'
import { hapticSuccess, useErrorHaptic } from '@/lib/haptic'

import { PARTNER_STATE_KEY } from './partnerKeys'
import { formatMoney, formatDayMonth, formatDayShort } from './format'
import { PARTNER_MOBILE_ROW, PARTNER_MOBILE_BADGE, PARTNER_INPUT } from './layout'

/**
 * Тело отказа приходит JSON'ом со слагом ошибки и подробностями: минимальной
 * суммой либо датой окончания кулдауна. Разбираем, чтобы партнёр увидел
 * конкретное число, а не «попробуйте позже».
 */
function parseErrorBody(body: string): { error?: string; minimum?: number; available_at?: string } {
  try {
    return JSON.parse(body || '{}')
  } catch {
    return {}
  }
}

export function PartnerPayoutsTab({ partner }: { partner: PartnerAccountDTO }) {
  const { t } = useTranslation()
  const qc = useQueryClient()

  const [method, setMethod] = useState(partner.payout_method ?? '')
  const [details, setDetails] = useState(partner.payout_details ?? '')
  const [amount, setAmount] = useState(String(Math.floor(partner.balance)))
  const [detailsError, setDetailsError] = useState<string | null>(null)
  useErrorHaptic(detailsError)
  const [payoutError, setPayoutError] = useState<string | null>(null)
  useErrorHaptic(payoutError)
  const [saved, setSaved] = useState(false)

  /*
   * Размер страницы тот же, что у клиентов и начислений. Листаем через offset,
   * а не наращиваем limit: сервер обрезает limit сотней, и «показать ещё»
   * после сотой строки перезапрашивало бы ту же страницу.
   */
  const history = useInfiniteQuery({
    queryKey: ['partner-payouts'],
    queryFn: ({ pageParam }) => api.partnerPayouts({ limit: 25, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, pages) => {
      const loaded = pages.reduce((n, p) => n + p.items.length, 0)
      return loaded < lastPage.total ? loaded : undefined
    },
    staleTime: 30_000,
  })
  const isLoading = history.isLoading
  const payoutItems = history.data?.pages.flatMap((p) => p.items) ?? []

  const saveDetails = useMutation({
    mutationFn: () => api.partnerSavePayoutDetails(method.trim(), details.trim()),
    onSuccess: async () => {
      hapticSuccess()
      setDetailsError(null)
      setSaved(true)
      await qc.invalidateQueries({ queryKey: PARTNER_STATE_KEY })
    },
    onError: (e) => {
      setSaved(false)
      if (e instanceof ApiError && (e.body || '').includes('invalid_details')) {
        setDetailsError(t('partnerPage.payouts.errors.invalidDetails'))
        return
      }
      setDetailsError(t('partnerPage.errors.generic'))
    },
  })

  const requestPayout = useMutation({
    mutationFn: () => api.partnerRequestPayout(Number(amount.replace(',', '.'))),
    onSuccess: async () => {
      hapticSuccess()
      setPayoutError(null)
      await qc.invalidateQueries({ queryKey: PARTNER_STATE_KEY })
      await qc.invalidateQueries({ queryKey: ['partner-payouts'] })
    },
    onError: (e) => {
      if (e instanceof ApiError) {
        const payload = parseErrorBody(e.body)
        switch (payload.error) {
          case 'details_required':
            return setPayoutError(t('partnerPage.payouts.errors.detailsRequired'))
          case 'below_minimum':
            return setPayoutError(
              t('partnerPage.payouts.errors.belowMinimum', { min: formatMoney(payload.minimum ?? 0) }),
            )
          case 'insufficient_balance':
            return setPayoutError(t('partnerPage.payouts.errors.insufficient'))
          case 'payout_pending':
            return setPayoutError(t('partnerPage.payouts.errors.pending'))
          case 'cooldown':
            return setPayoutError(
              t('partnerPage.payouts.errors.cooldown', { date: formatDayMonth(payload.available_at) }),
            )
          case 'withdraw_blocked':
            return setPayoutError(t('partnerPage.payouts.errors.blocked'))
          case 'invalid_amount':
            return setPayoutError(t('partnerPage.payouts.errors.invalidAmount'))
        }
      }
      setPayoutError(t('partnerPage.errors.generic'))
    },
  })

  const hasDetails = Boolean(partner.payout_details)
  const cooldownActive = Boolean(partner.payout_available_at)
  const canRequest =
    partner.can_withdraw && hasDetails && !partner.has_open_payout && !cooldownActive && partner.balance > 0

  function onSaveDetails(e: FormEvent) {
    e.preventDefault()
    if (!method.trim() || !details.trim()) {
      setDetailsError(t('partnerPage.payouts.errors.invalidDetails'))
      return
    }
    setDetailsError(null)
    saveDetails.mutate()
  }

  function onRequest(e: FormEvent) {
    e.preventDefault()
    const value = Number(amount.replace(',', '.'))
    if (!Number.isFinite(value) || value <= 0) {
      setPayoutError(t('partnerPage.payouts.errors.invalidAmount'))
      return
    }
    setPayoutError(null)
    requestPayout.mutate()
  }

  return (
    <>
      <RevealItem>
        <Card className="border-primary/15 bg-gradient-to-br from-card via-card to-primary/5">
          <CardContent className="pt-4 sm:pt-6">
            <p className="text-xs uppercase tracking-wide text-muted-foreground">
              {t('partnerPage.overview.available')}
            </p>
            <p className="mt-1 text-3xl font-semibold text-primary">{formatMoney(partner.balance)}</p>

            <form className="mt-4 space-y-3" onSubmit={onRequest}>
              <div className="space-y-1.5">
                <Label htmlFor="payout-amount">{t('partnerPage.payouts.amount')}</Label>
                <Input
                  id="payout-amount"
                  inputMode="decimal"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  disabled={!canRequest}
                  className={PARTNER_INPUT}
                />
              </div>

              {payoutError ? (
                <Alert variant="destructive">
                  <AlertDescription>{payoutError}</AlertDescription>
                </Alert>
              ) : null}

              {/* Причина, по которой кнопка недоступна, показывается рядом с ней,
                  а не всплывает ошибкой после отправки формы. */}
              {/* Заявка принята — успех: партнёр сделал всё, что от него
                  требовалось. Кулдаун и отсутствие реквизитов — не ошибка, а
                  информация, поэтому синий, а не красный. */}
              {partner.has_open_payout ? (
                <Alert variant="success">
                  <AlertDescription>{t('partnerPage.payouts.pendingNotice')}</AlertDescription>
                </Alert>
              ) : null}
              {cooldownActive ? (
                <Alert variant="info">
                  <AlertDescription>
                    {t('partnerPage.payouts.cooldownNotice', {
                      date: formatDayMonth(partner.payout_available_at),
                    })}
                  </AlertDescription>
                </Alert>
              ) : null}
              {!hasDetails ? (
                <Alert variant="info">
                  <AlertDescription>{t('partnerPage.payouts.detailsFirst')}</AlertDescription>
                </Alert>
              ) : null}

              <Button type="submit" className="w-full" loading={requestPayout.isPending} disabled={!canRequest}>
                {t('partnerPage.payouts.request')}
              </Button>
              <p className="text-center text-xs text-muted-foreground">{t('partnerPage.payouts.manualNotice')}</p>
            </form>
          </CardContent>
        </Card>
      </RevealItem>

      <RevealItem>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base font-medium">
              <CreditCard size={18} className="text-primary" />
              {t('partnerPage.payouts.detailsTitle')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <form className="space-y-3" onSubmit={onSaveDetails}>
              <div className="space-y-1.5">
                <Label htmlFor="payout-method">{t('partnerPage.payouts.method')}</Label>
                <Input
                  id="payout-method"
                  value={method}
                  onChange={(e) => {
                    setMethod(e.target.value)
                    setSaved(false)
                  }}
                  maxLength={64}
                  placeholder={t('partnerPage.payouts.methodPlaceholder')}
                  className={PARTNER_INPUT}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="payout-details">{t('partnerPage.payouts.details')}</Label>
                <Input
                  id="payout-details"
                  value={details}
                  onChange={(e) => {
                    setDetails(e.target.value)
                    setSaved(false)
                  }}
                  maxLength={512}
                  placeholder={t('partnerPage.payouts.detailsPlaceholder')}
                  className={PARTNER_INPUT}
                />
              </div>

              {detailsError ? (
                <Alert variant="destructive">
                  <AlertDescription>{detailsError}</AlertDescription>
                </Alert>
              ) : null}
              {saved ? (
                <Alert variant="success">
                  <AlertDescription>{t('partnerPage.payouts.detailsSaved')}</AlertDescription>
                </Alert>
              ) : null}

              <Button type="submit" className="w-full" loading={saveDetails.isPending}>
                {t('partnerPage.payouts.saveDetails')}
              </Button>
            </form>
          </CardContent>
        </Card>
      </RevealItem>

      <RevealItem>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base font-medium">
              <History size={18} className="text-muted-foreground" />
              {t('partnerPage.payouts.historyTitle')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="space-y-2" aria-hidden>
                {[0, 1, 2].map((i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : null}

            {history.data && payoutItems.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">{t('partnerPage.payouts.empty')}</p>
            ) : null}

            {payoutItems.length > 0 ? (
              <ul className="divide-y divide-border rounded-lg border border-border">
                {payoutItems.map((p) => (
                  <li key={p.id} className={PARTNER_MOBILE_ROW}>
                    <div className="min-w-0 max-w-full">
                      <p className="font-semibold tabular-nums">{formatMoney(p.amount)}</p>
                      <p className="text-xs text-muted-foreground">
                        {[formatDayShort(p.requested_at), p.method].filter(Boolean).join(' · ')}
                      </p>
                      {p.external_ref ? (
                        <p className="font-mono text-xs text-muted-foreground">
                          {t('partnerPage.payouts.ref', { ref: p.external_ref })}
                        </p>
                      ) : null}
                      {p.admin_comment ? (
                        <p className="text-xs text-muted-foreground">{p.admin_comment}</p>
                      ) : null}
                    </div>
                    <PayoutStatus status={p.status} />
                  </li>
                ))}
              </ul>
            ) : null}

            {history.hasNextPage ? (
              <Button
                variant="secondary"
                size="sm"
                className="mt-3 w-full"
                loading={history.isFetchingNextPage}
                onClick={() => void history.fetchNextPage()}
              >
                {t('partnerPage.showMore')}
              </Button>
            ) : null}
          </CardContent>
        </Card>
      </RevealItem>
    </>
  )
}

function PayoutStatus({ status }: { status: string }) {
  const { t } = useTranslation()
  const className = cn('shrink-0', PARTNER_MOBILE_BADGE)
  if (status === 'paid')
    return (
      <Badge variant="success" className={className}>
        {t('partnerPage.payouts.statusPaid')}
      </Badge>
    )
  if (status === 'rejected')
    return (
      <Badge variant="destructive" className={className}>
        {t('partnerPage.payouts.statusRejected')}
      </Badge>
    )
  if (status === 'approved')
    return (
      <Badge variant="default" className={className}>
        {t('partnerPage.payouts.statusApproved')}
      </Badge>
    )
  return (
    <Badge variant="default" className={className}>
      {t('partnerPage.payouts.statusPending')}
    </Badge>
  )
}
