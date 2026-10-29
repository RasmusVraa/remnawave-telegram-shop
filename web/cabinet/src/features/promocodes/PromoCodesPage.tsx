import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { BadgePercent, Gift, TicketPercent } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError, type PromoApplyResponse } from '@/lib/api'

export default function PromoCodesPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [code, setCode] = useState('')
  const [result, setResult] = useState<PromoApplyResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const { data: state, isLoading } = useQuery({
    queryKey: ['promocodes-state'],
    queryFn: () => api.promoState(),
    staleTime: 30_000,
  })

  const apply = useMutation({
    mutationFn: async () => api.applyPromoCode(code.trim()),
    onSuccess: async (res) => {
      setResult(res)
      setError(null)
      setCode('')
      await qc.invalidateQueries({ queryKey: ['promocodes-state'] })
      await qc.invalidateQueries({ queryKey: ['paymentPreview'] })
    },
    onError: (e) => {
      setResult(null)
      if (e instanceof ApiError) {
        const raw = e.body || ''
        if (raw.includes('hwid_pinned')) return setError(t('promocodes.errors.hwid_pinned'))
        if (raw.includes('already_used')) return setError(t('promocodes.errors.already_used'))
        if (raw.includes('inactive')) return setError(t('promocodes.errors.inactive'))
        if (raw.includes('not_found')) return setError(t('promocodes.errors.not_found'))
        if (raw.includes('pending_discount')) return setError(t('promocodes.errors.pending_discount'))
        if (raw.includes('tariff_mismatch')) return setError(t('promocodes.errors.tariff_mismatch'))
      }
      setError(t('promocodes.errors.generic'))
    },
  })

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (!code.trim()) {
      setError(t('promocodes.errors.empty'))
      return
    }
    setError(null)
    setResult(null)
    apply.mutate()
  }

  return (
    <AppLayout>
      <PageReveal className="mx-auto w-full max-w-xl space-y-6">
        <RevealItem>
          <h1 className="text-2xl font-semibold">{t('promocodes.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('promocodes.subtitle')}</p>
        </RevealItem>

        <RevealItem>
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <TicketPercent size={18} />
              {t('promocodes.applyTitle')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <form className="space-y-3" onSubmit={onSubmit}>
              <div className="space-y-1.5">
                <Label htmlFor="promo-code">{t('promocodes.codeLabel')}</Label>
                <Input
                  id="promo-code"
                  value={code}
                  onChange={(e) => setCode(e.target.value.toUpperCase())}
                  placeholder={t('promocodes.codePlaceholder')}
                  autoComplete="off"
                />
              </div>
              <Button type="submit" disabled={apply.isPending}>{apply.isPending ? t('common.loading') : t('promocodes.applyButton')}</Button>
            </form>
            {error && (
              <Alert className="mt-3" variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            {result && (
              <Alert variant="success" className="mt-3">
                <AlertDescription>{formatPromoResult(t, result)}</AlertDescription>
              </Alert>
            )}
          </CardContent>
        </Card>
        </RevealItem>

        <RevealItem>
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <BadgePercent size={18} />
              {t('promocodes.pendingTitle')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-4 w-52" />
                <Skeleton className="h-3.5 w-40" />
              </div>
            ) : !state?.has_pending_discount || !state.pending_discount ? (
              <p className="text-sm text-muted-foreground">{t('promocodes.noPending')}</p>
            ) : (
              <div className="space-y-2 text-sm">
                <p className="font-medium">{t('promocodes.pendingDiscount', { pct: state.pending_discount.percent })}</p>
                <p className="text-muted-foreground">
                  {state.pending_discount.until_first_purchase
                    ? t('promocodes.untilFirstPurchase')
                    : state.pending_discount.subscription_payments_remaining === -1
                      ? t('promocodes.unlimitedPayments')
                      : t('promocodes.remainingPayments', { n: state.pending_discount.subscription_payments_remaining })}
                </p>
                {state.pending_discount.expires_at ? (
                  <p className="text-muted-foreground">{t('promocodes.expiresAt', { dt: new Date(state.pending_discount.expires_at).toLocaleString() })}</p>
                ) : null}
              </div>
            )}
          </CardContent>
        </Card>
        </RevealItem>

        <RevealItem>
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Gift size={18} />
              {t('promocodes.infoTitle')}
            </CardTitle>
          </CardHeader>
          <CardContent className="text-sm text-muted-foreground space-y-2">
            <p>{t('promocodes.info1')}</p>
            <p>{t('promocodes.info2')}</p>
            <p>{t('promocodes.info3')}</p>
          </CardContent>
        </Card>
        </RevealItem>
      </PageReveal>
    </AppLayout>
  )
}

function formatPromoResult(t: (key: string, options?: Record<string, unknown>) => string, result: PromoApplyResponse): string {
  switch (result.type) {
    case 'discount':
      return t('promocodes.success.discount', { pct: result.discount_percent ?? 0 })
    case 'subscription_days':
      return t('promocodes.success.subscription_days', { n: result.subscription_days ?? 0 })
    case 'trial':
      if (result.trial_skipped_active_sub) return t('promocodes.success.trial_skipped')
      return t('promocodes.success.trial', { n: result.trial_days ?? 0 })
    case 'extra_hwid':
      return t('promocodes.success.extra_hwid', { n: result.extra_hwid_delta ?? 0 })
    default:
      return t('promocodes.success.generic')
  }
}
