import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, XCircle } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import {
  PaymentMethodIcon,
  formatMoney,
  invoiceLabel,
  purchaseKindLabel,
} from '@/components/history-list'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { api, type PaymentStatusResponse } from '@/lib/api'
import { hapticError, hapticSuccess } from '@/lib/haptic'
import { BypassCard, packSizeLabel } from '@/features/home/trafficSplit'
import { bypassLabel } from '@/lib/bypassLabel'
import { splitDateTimeShort } from '@/lib/utils'

export default function PaymentStatusPage() {
  const { t } = useTranslation()
  const { id: idParam } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()

  // Поддерживаем /payment/status/:id и /payment/status?id=…
  const rawId = idParam ?? searchParams.get('id') ?? ''
  const paymentId = parseInt(rawId, 10)

  const { data, isLoading, error } = useQuery({
    queryKey: ['payment-status', paymentId],
    queryFn: () => api.paymentStatus(paymentId),
    enabled: !isNaN(paymentId) && paymentId > 0,
    // Polling: каждые 3 секунды пока платёж не финализирован.
    refetchInterval: (query) => {
      const status = query.state.data?.status
      if (status === 'paid' || status === 'failed' || status === 'expired') return false
      return 3000
    },
    staleTime: 0,
    retry: 2,
  })

  const status = data?.status

  // Итог оплаты — вибрацией: успех мягким двойным толчком, отказ — сильным.
  useEffect(() => {
    if (status === 'paid') hapticSuccess()
    else if (status === 'failed' || status === 'expired') hapticError()
  }, [status])

  return (
    <AppLayout>
      <PageReveal className="max-w-md mx-auto flex flex-col items-center justify-center min-h-[60vh] space-y-6">
        <RevealItem>
          <h1 className="text-xl font-semibold">{t('paymentStatus.title')}</h1>
        </RevealItem>

        <RevealItem className="w-full">
        <Card className="w-full">
          <CardContent className="pt-8 pb-8 flex flex-col items-center gap-5 text-center">
            {isLoading || status === 'new' || status === 'pending' ? (
              <PendingState />
            ) : status === 'paid' && data ? (
              <SuccessState data={data} />
            ) : (
              <FailedState expired={status === 'expired'} />
            )}

            {error && (
              <p className="text-xs text-destructive">{t('errors.unknown')}</p>
            )}
          </CardContent>
        </Card>
        </RevealItem>
      </PageReveal>
    </AppLayout>
  )
}

// ── States ─────────────────────────────────────────────────────────────────

function PendingState() {
  const { t } = useTranslation()
  return (
    <>
      <div className="relative flex items-center justify-center">
        <div className="h-16 w-16 rounded-full border-4 border-border border-t-primary animate-spin" />
        <div className="absolute h-8 w-8 rounded-full bg-primary/10 flex items-center justify-center">
          <div className="h-3 w-3 rounded-full bg-primary animate-pulse" />
        </div>
      </div>
      <div>
        <p className="font-medium">{t('paymentStatus.pending')}</p>
        <p className="text-sm text-muted-foreground mt-1">{t('paymentStatus.pendingHint')}</p>
      </div>
    </>
  )
}

/**
 * Строка чека: подпись слева, значение справа, точечная выноска между ними.
 *
 * Выноска на flex-1 и сжимается до нуля — на узком экране пропадают точки, а
 * не переносится значение.
 */
function ReceiptRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline gap-2 py-[3px] text-left">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <span className="cabinet-receipt-leader" aria-hidden />
      <dd className="m-0 flex shrink-0 items-center gap-1.5 whitespace-nowrap font-medium tabular-nums">
        {children}
      </dd>
    </div>
  )
}

/**
 * Экран успешной оплаты: чек вместо ссылки на подписку.
 *
 * Ссылку отсюда убрали намеренно. Человек, который только что заплатил,
 * первым делом хочет увидеть, сколько с него списали и за что, — а ссылка
 * никуда не девается, за ней ведёт кнопка «Моя подписка».
 *
 * Строки, для которой нет значения, просто нет: прочерк в чеке читается как
 * сбой оплаты, а не как отсутствие данных. По той же причине «Подписка до»
 * не показывается при покупке доп. устройств — подписку она не продлевает, и
 * дата рядом с такой покупкой вводит в заблуждение.
 */
function SuccessState({ data }: { data: PaymentStatusResponse }) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language

  const kind = data.purchase_kind ?? 'subscription'
  const isExtraHwid = kind === 'extra_hwid'
  const isTrafficPack = kind === 'extra_traffic'
  const successHintKey = isTrafficPack
    ? 'trafficSplit.successHint'
    : isExtraHwid
      ? 'paymentStatus.successHintExtraHwid'
      : 'paymentStatus.successHint'
  // Главная кэширует лимиты и подписку: без сброса после оплаты пакета она до минуты показывала бы
  // «Обход исчерпан» и красную плашку.
  const qc = useQueryClient()
  useEffect(() => {
    if (!isTrafficPack) return
    void qc.invalidateQueries({ queryKey: ['my-traffic'] })
    void qc.invalidateQueries({ queryKey: ['subscription'] })
    void qc.invalidateQueries({ queryKey: ['traffic-usage'] })
  }, [isTrafficPack, qc])
  // Свежие лимиты для карточки на экране успеха (пакет уже в панели или будет через минуту).
  const { data: split } = useQuery({
    queryKey: ['my-traffic', 'after-pack', data.payment_id],
    queryFn: () => api.myTraffic(),
    enabled: isTrafficPack,
    refetchOnMount: 'always',
    staleTime: 0,
  })
  const packLabel = data.traffic_pack_kind === 'bypass' ? bypassLabel(split?.label) : t('trafficSplit.tabMain')

  const months = data.month ?? 0
  const what = isTrafficPack
    ? t('trafficSplit.scenario')
    : purchaseKindLabel(t, {
        purchase_kind: kind,
        month: months,
        extra_hwid: data.extra_hwid ?? 0,
      })
  // «Подписка» + «3 месяца»: сам purchaseKindLabel срок не называет, а на чеке
  // он — половина ответа на вопрос «за что списали».
  const whatFull =
    months > 0 && kind === 'subscription'
      ? `${what} · ${t('paymentStatus.receiptMonths', { count: months })}`
      : what

  const paidAt = splitDateTimeShort(data.paid_at)
  // Пакет трафика подписку не продлевает: «Действует до» со сроком подписки рядом с ним вводит в заблуждение.
  const expireAt = isExtraHwid || isTrafficPack ? null : splitDateTimeShort(data.expire_at)
  const hasAmount = typeof data.amount === 'number' && data.amount > 0

  return (
    <>
      <div className="h-16 w-16 rounded-full bg-emerald-500/15 flex items-center justify-center">
        <CheckCircle2 size={36} className="text-emerald-500" strokeWidth={1.5} />
      </div>
      <div>
        <p className="font-semibold text-lg">
          {isTrafficPack && data.traffic_pack_bytes
            ? t('trafficSplit.successTitle', { size: packSizeLabel(data.traffic_pack_bytes, t('dashboard.gigabytes')), label: packLabel })
            : t('paymentStatus.success')}
        </p>
        <p className="text-sm text-muted-foreground mt-1">{t(successHintKey)}</p>
      </div>

      {isTrafficPack && data.traffic_pack_kind === 'bypass' && split && (
        <div className="w-full text-left">
          <BypassCard split={split} lang={lang} />
        </div>
      )}

      {hasAmount && (
        <section
          className="cabinet-receipt w-full px-4 pb-5 pt-4 sm:px-5"
          aria-label={t('paymentStatus.receiptTitle')}
        >
          <div className="pb-3 text-center">
            <div className="font-heading text-3xl font-extrabold leading-none tracking-tight tabular-nums sm:text-4xl">
              {formatMoney(data.amount as number, data.currency ?? '')}
            </div>
            <div className="mt-1.5 text-xs text-muted-foreground sm:text-[13px]">{whatFull}</div>
          </div>

          <dl className="border-t border-dashed border-border pt-2 text-xs sm:text-[13px]">
            {data.invoice_type && (
              <ReceiptRow label={t('paymentStatus.receiptMethod')}>
                <PaymentMethodIcon invoiceType={data.invoice_type} className="size-3.5" />
                {invoiceLabel(t, data.invoice_type)}
              </ReceiptRow>
            )}
            {data.payment_id != null && data.payment_id > 0 && (
              <ReceiptRow label={t('paymentStatus.receiptNumber')}>#{data.payment_id}</ReceiptRow>
            )}
            {paidAt && (
              <ReceiptRow label={t('paymentStatus.receiptTime')}>
                {paidAt.date}, {paidAt.time}
              </ReceiptRow>
            )}
            {expireAt && (
              <ReceiptRow label={t('paymentStatus.receiptExpireAt')}>{expireAt.date}</ReceiptRow>
            )}
          </dl>
        </section>
      )}

      <Button asChild className="w-full">
        <Link to="/dashboard">{isTrafficPack ? t('trafficSplit.done') : t('paymentStatus.toSubscription')}</Link>
      </Button>
    </>
  )
}

function FailedState({ expired }: { expired: boolean }) {
  const { t } = useTranslation()
  return (
    <>
      <div className="h-16 w-16 rounded-full bg-destructive/15 flex items-center justify-center">
        <XCircle size={36} className="text-destructive" strokeWidth={1.5} />
      </div>
      <div>
        <p className="font-semibold text-lg">
          {expired ? t('paymentStatus.expired') : t('paymentStatus.failed')}
        </p>
        <p className="text-sm text-muted-foreground mt-1">
          {expired ? t('paymentStatus.failedHint') : t('paymentStatus.failedHint')}
        </p>
      </div>
      <div className="flex flex-col gap-2 w-full">
        <Button asChild variant="outline">
          <Link to="/tariffs">{t('paymentStatus.retry')}</Link>
        </Button>
        <Button asChild variant="ghost">
          <Link to="/dashboard">{t('paymentStatus.toSubscription')}</Link>
        </Button>
      </div>
    </>
  )
}
