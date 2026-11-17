import { useTranslation } from 'react-i18next'
import { CopyPlus, Pencil, Trash2, Users } from 'lucide-react'

import { cn } from '@/lib/utils'
import type { AdminPromoCodeDTO } from '@/lib/types/admin'
import { AdminModal } from '../components/AdminModal'
import type { AdminSectionIconAccent } from '../utils/adminSectionIconAccents'
import { CopyCodeButton, PromoExpiryText, PromoUsageBar, PromoUsageText } from './PromoBits'
import type { PromoFormFocus } from './PromoFormModal'
import { PromoEffectSentence } from './PromoPreview'
import { PromoActiveSwitch } from './PromoRow'
import { PromoFact, usePromoPaybackFacts } from './PromoUsedByModal'
import { formatPromoDate, promoKeyValue, promoTypeIcon, promoTypeLabel, promoTypeTone } from './promoPresentation'

const TYPE_ACCENT: Record<string, AdminSectionIconAccent> = {
  subscription_days: 'blue',
  discount: 'amber',
  trial: 'emerald',
  extra_hwid: 'violet',
}

/** Карточка промокода (тап по карточке списка): цифры, отдача и действия. */
export function PromoDetailSheet({
  promo,
  tariffName,
  open,
  onClose,
  onEdit,
  onUsed,
  onDuplicate,
  onDelete,
  onToggled,
  onError,
}: {
  promo: AdminPromoCodeDTO | null
  tariffName?: string | null
  open: boolean
  onClose: () => void
  onEdit: (p: AdminPromoCodeDTO, focus?: PromoFormFocus) => void
  onUsed: (p: AdminPromoCodeDTO) => void
  onDuplicate: (p: AdminPromoCodeDTO) => void
  onDelete: (p: AdminPromoCodeDTO) => void
  onToggled: (p: AdminPromoCodeDTO) => void
  onError: (err: unknown) => void
}) {
  const { t } = useTranslation()
  const facts = usePromoPaybackFacts(promo ?? ({} as AdminPromoCodeDTO), open && promo != null)
  if (!promo) return null
  const tone = promoTypeTone(promo.type)
  const Icon = promoTypeIcon(promo.type)
  const key = promoKeyValue(promo, t)
  const effect = (
    <PromoEffectSentence
      v={{
        ...promo,
        tariffName: promo.type === 'subscription_days' && promo.tariff_id != null ? tariffName : null,
      }}
    />
  )
  // Плитки: настройки — открыть правку на этом поле, цифры отдачи — «Кто использовал».
  const edit = (focus: PromoFormFocus) => ({ onClick: () => onEdit(promo, focus), actionLabel: t('admin.promos.menu.edit') })
  const used = { onClick: () => onUsed(promo), actionLabel: t('admin.promos.menu.used') }
  // Кнопки — в цвете темы, как активная вкладка: иначе на ПК в ряд по 3 их не
  // отличить от плиток с цифрами.
  const actionRow =
    'flex h-[46px] w-full items-center gap-2.5 rounded-[12px] border border-primary/25 bg-primary/10 px-4 text-sm font-semibold text-primary transition-colors hover:bg-primary/15 dark:bg-primary/15 dark:hover:bg-primary/20 sm:justify-center sm:px-3'

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={promo.code}
      titleSlot={
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-1">
            <h3 className="truncate font-mono text-[17px] font-bold leading-tight">{promo.code}</h3>
            <CopyCodeButton code={promo.code} />
          </div>
          <p className="truncate text-[12.5px] text-muted-foreground">
            {promoTypeLabel(promo.type, t)} · {t('admin.promos.detail.created', { date: formatPromoDate(promo.created_at) })}
          </p>
          {promo.note && (
            <p className="truncate text-xs font-medium text-muted-foreground" title={promo.note}>
              {promo.note}
            </p>
          )}
        </div>
      }
      icon={Icon}
      iconAccent={TYPE_ACCENT[promo.type] ?? 'slate'}
      headerAction={<PromoActiveSwitch promo={promo} onToggled={onToggled} onError={onError} />}
      hideClose
      size="lg"
      footer={
        <div className="flex gap-2.5">
          <button
            type="button"
            onClick={() => onDelete(promo)}
            className="inline-flex h-11 flex-1 items-center justify-center gap-2 rounded-[12px] border border-destructive/50 bg-destructive/10 text-sm font-semibold text-destructive transition-colors hover:bg-destructive/15"
          >
            <Trash2 className="size-4" />
            {t('admin.promos.detail.delete')}
          </button>
          <button
            type="button"
            onClick={onClose}
            className="h-11 flex-1 rounded-[12px] bg-primary text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
          >
            {t('admin.promos.detail.done')}
          </button>
        </div>
      }
    >
      <div className="space-y-3.5">
        <div className={cn('flex items-center gap-2.5 rounded-[14px] border px-3.5 py-3 text-[13.5px]', tone.soft, tone.border)}>
          <Icon className={cn('size-4 shrink-0', tone.icon)} />
          <span>{effect}</span>
        </div>
        <div className={cn('grid grid-cols-2 gap-2', facts.show ? 'sm:grid-cols-3' : 'sm:grid-cols-4')}>
          <PromoFact label={t('admin.promos.detail.last24h')} loading={facts.loading} {...used}>
            {facts.detail?.redemptions_24h ?? facts.detail?.redemptions_today ?? '—'}
          </PromoFact>
          <PromoFact label={t('admin.promos.detail.expiry')} {...edit('validUntil')}>
            <PromoExpiryText validUntil={promo.valid_until} className="text-[15px] font-bold" />
          </PromoFact>
          <PromoFact label={t('admin.promos.detail.firstPurchase')} {...edit('firstPurchase')}>{promo.first_purchase_only ? t('admin.yes') : t('admin.no')}</PromoFact>
          <PromoFact label={key.label} {...edit('value')}>{key.value}</PromoFact>
          {facts.show && (
            <>
              <PromoFact label={facts.paidLabel} loading={facts.loading} {...used}>
                {facts.paid}
              </PromoFact>
              <PromoFact label={t('admin.promos.payback.revenue')} loading={facts.loading} {...used}>
                {facts.revenue}
              </PromoFact>
            </>
          )}
        </div>
        {/* Полоска между плитками и кнопками — разделяет цифры и действия. */}
        <button
          type="button"
          onClick={() => onUsed(promo)}
          aria-label={t('admin.promos.menu.used')}
          className="-mx-1.5 flex w-[calc(100%+0.75rem)] items-center gap-2.5 rounded-[10px] px-1.5 py-1 transition-colors hover:bg-primary/10"
        >
          <span className="shrink-0 text-xs font-semibold text-muted-foreground">{t('admin.promos.detail.usage')}</span>
          <PromoUsageBar promo={promo} className="min-w-0 flex-1" />
          <PromoUsageText promo={promo} />
        </button>
        <div className="space-y-2 sm:grid sm:grid-cols-3 sm:gap-2 sm:space-y-0">
          <button type="button" className={actionRow} onClick={() => onEdit(promo)}>
            <Pencil className="size-4" />
            {t('admin.promos.menu.edit')}
          </button>
          <button type="button" className={actionRow} onClick={() => onUsed(promo)}>
            <Users className="size-4" />
            {t('admin.promos.menu.used')}
            <span className="ml-auto tabular-nums text-primary/70 sm:ml-0">{promo.uses_count}</span>
          </button>
          <button type="button" className={actionRow} onClick={() => onDuplicate(promo)}>
            <CopyPlus className="size-4" />
            {t('admin.promos.menu.duplicate')}
          </button>
        </div>
      </div>
    </AdminModal>
  )
}
