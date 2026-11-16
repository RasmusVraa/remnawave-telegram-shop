import { Trans, useTranslation } from 'react-i18next'

import { discountTtlKind, formatPromoDateTime } from './promoPresentation'

/** Значения, из которых строится фраза «Пользователь получит …» — у формы и у листа-карточки. */
export interface PromoEffectValues {
  type: string
  subscription_days?: number | null
  trial_days?: number | null
  extra_hwid_delta?: number | null
  discount_percent?: number | null
  discount_ttl_hours?: number | null
  discount_max_subscription_payments_per_customer: number
  tariffName?: string | null
}

const b = <b className="font-semibold text-foreground" />

/** «Пользователь получит +7 дней подписки» и т. п.; null — пока главное поле не заполнено. */
export function PromoEffectSentence({ v }: { v: PromoEffectValues }) {
  const { t } = useTranslation()
  switch (v.type) {
    case 'subscription_days':
      if (!v.subscription_days) return null
      return (
        <>
          <Trans i18nKey="admin.promos.form.preview.days" count={v.subscription_days} components={{ b }} />
          {v.tariffName && <Trans i18nKey="admin.promos.form.preview.tariff" values={{ name: v.tariffName }} components={{ b }} />}
        </>
      )
    case 'trial':
      if (!v.trial_days) return null
      return <Trans i18nKey="admin.promos.form.preview.trial" count={v.trial_days} components={{ b }} />
    case 'extra_hwid':
      if (!v.extra_hwid_delta) return null
      return <Trans i18nKey="admin.promos.form.preview.hwid" count={v.extra_hwid_delta} components={{ b }} />
    case 'discount': {
      if (!v.discount_percent) return null
      const ttl = discountTtlKind({
        discount_ttl_hours: v.discount_ttl_hours ?? null,
        discount_max_subscription_payments_per_customer: v.discount_max_subscription_payments_per_customer,
      })
      let ttlNode
      if (ttl.kind === 'none') ttlNode = <Trans i18nKey="admin.promos.form.preview.ttlNone" components={{ b }} />
      else if (ttl.kind === 'firstPay') ttlNode = <Trans i18nKey="admin.promos.form.preview.ttlFirstPay" components={{ b }} />
      else {
        const value =
          ttl.hours >= 48 && ttl.hours % 24 === 0
            ? t('admin.promos.form.preview.daysN', { count: ttl.hours / 24 })
            : t('admin.promos.form.preview.hours', { count: ttl.hours })
        ttlNode = <Trans i18nKey="admin.promos.form.preview.ttlFor" values={{ value }} components={{ b }} />
      }
      const n = v.discount_max_subscription_payments_per_customer
      const payments =
        n <= 0
          ? t('admin.promos.form.preview.unlimitedPayments')
          : n === 1
            ? t('admin.promos.form.preview.onePayment')
            : t('admin.promos.form.preview.nPayments', { count: n })
      return (
        <>
          <Trans i18nKey="admin.promos.form.preview.discount" values={{ percent: v.discount_percent }} components={{ b }} />
          {ttlNode}
          {ttl.kind !== 'firstPay' && <Trans i18nKey="admin.promos.form.preview.payments" values={{ value: payments }} components={{ b }} />}
        </>
      )
    }
    default:
      return null
  }
}

/** Хвост предпросмотра формы: срок кода, лимит, «один раз», «первая покупка». */
export function PromoPreviewTail({
  validUntil,
  maxUses,
  firstPurchaseOnly,
}: {
  validUntil: Date | null
  maxUses: number | null
  firstPurchaseOnly: boolean
}) {
  const { t } = useTranslation()
  return (
    <>
      {validUntil ? (
        <Trans i18nKey="admin.promos.form.preview.until" values={{ date: formatPromoDateTime(validUntil) }} components={{ b }} />
      ) : (
        <Trans i18nKey="admin.promos.form.preview.forever" components={{ b }} />
      )}
      {maxUses ? (
        <Trans i18nKey="admin.promos.form.preview.uses" count={maxUses} components={{ b }} />
      ) : (
        <Trans i18nKey="admin.promos.form.preview.noLimit" components={{ b }} />
      )}{' '}
      {t('admin.promos.form.preview.once')}
      {firstPurchaseOnly && <> {t('admin.promos.form.preview.firstPurchase')}</>}
    </>
  )
}
