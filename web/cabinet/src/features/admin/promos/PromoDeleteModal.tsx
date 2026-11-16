import type { ReactNode } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { Power, RotateCcw, Trash2, X } from 'lucide-react'

import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import type { AdminPromoCodeDTO } from '@/lib/types/admin'
import { AdminModal } from '../components/AdminModal'
import { useAdminPromoDelete, useAdminPromoGet, useAdminPromoUpdate } from '../hooks/useAdminPromos'

/**
 * Удаление честно перечисляет, что пропадёт. Это факты схемы: promo_redemption и
 * customer_pending_discount удаляются каскадом (000007), у purchase.promo_code_id —
 * SET NULL (000013). Обычно хватает выключить — поэтому «Выключить» главная кнопка.
 */
export function PromoDeleteModal({
  promo,
  open,
  onClose,
  onDeleted,
  onDisabled,
  onError,
}: {
  promo: AdminPromoCodeDTO | null
  open: boolean
  onClose: () => void
  onDeleted: (p: AdminPromoCodeDTO) => void
  onDisabled: (p: AdminPromoCodeDTO) => void
  onError: (err: unknown) => void
}) {
  const { t } = useTranslation()
  const { data, isFetching, isError, refetch } = useAdminPromoGet(open && promo ? promo.id : null)
  const del = useAdminPromoDelete()
  const update = useAdminPromoUpdate()
  if (!promo) return null

  // Последствия считаем только по свежему ответу: по кэшу или без ответа нельзя
  // обещать «удаление ничего не затронет» — невыкупленные скидки могли появиться.
  const ready = Boolean(data) && !isFetching && !isError
  const uses = data?.promo.uses_count ?? promo.uses_count
  const pending = data?.pending_discounts ?? 0
  const simple = ready && uses === 0 && pending === 0
  const isLoading = !ready && !isError
  const busy = del.isPending || update.isPending
  const b = <b className="font-semibold text-foreground" />

  const doDelete = () =>
    del.mutate(promo.id, {
      onSuccess: () => onDeleted(promo),
      onError,
    })
  const doDisable = () =>
    update.mutate(
      { id: promo.id, fields: { active: false } },
      {
        onSuccess: () => onDisabled(promo),
        onError,
      },
    )

  const title = simple
    ? t('admin.promos.deleteModal.title', { code: promo.code })
    : t('admin.promos.deleteModal.titleForever', { code: promo.code })

  return (
    <AdminModal
      open={open}
      onClose={busy ? () => {} : onClose}
      title={title}
      icon={Trash2}
      iconAccent="rose"
      panelClassName="sm:max-w-[580px]"
      footer={
        <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-between">
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="h-10 rounded-[12px] border border-border px-4 text-sm font-semibold transition-colors hover:bg-muted disabled:opacity-50"
          >
            {t('admin.cancel')}
          </button>
          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            {simple ? (
              <button
                type="button"
                onClick={doDelete}
                disabled={busy}
                className="inline-flex h-10 items-center justify-center gap-2 whitespace-nowrap rounded-[12px] bg-destructive px-4 text-sm font-semibold text-destructive-foreground transition-colors hover:bg-destructive/90 disabled:opacity-60"
              >
                {del.isPending ? <Spinner /> : <Trash2 className="size-4" />}
                {t('admin.promos.deleteModal.confirm')}
              </button>
            ) : (
              <>
                <button
                  type="button"
                  onClick={doDelete}
                  disabled={busy || !ready}
                  className="inline-flex h-10 items-center justify-center gap-2 whitespace-nowrap rounded-[12px] border border-destructive/50 bg-destructive/10 px-4 text-sm font-semibold text-destructive transition-colors hover:bg-destructive/15 disabled:opacity-60"
                >
                  {del.isPending ? <Spinner /> : <Trash2 className="size-4" />}
                  {t('admin.promos.deleteModal.forever')}
                </button>
                {promo.active && (
                  <button
                    type="button"
                    onClick={doDisable}
                    disabled={busy || isLoading}
                    className="inline-flex h-10 items-center justify-center gap-2 whitespace-nowrap rounded-[12px] bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-60"
                  >
                    {update.isPending ? <Spinner /> : <Power className="size-4" />}
                    {t('admin.promos.deleteModal.disableInstead')}
                  </button>
                )}
              </>
            )}
          </div>
        </div>
      }
    >
      {isError ? (
        <div className="space-y-3 text-sm">
          <p className="text-destructive">{t('common.error')}</p>
          <button
            type="button"
            onClick={() => void refetch()}
            className="inline-flex h-9 items-center gap-2 rounded-[10px] border border-border px-3 text-sm font-semibold transition-colors hover:bg-muted"
          >
            {isFetching ? <Spinner /> : <RotateCcw className="size-4" />}
            {t('admin.promos.retry')}
          </button>
        </div>
      ) : isLoading ? (
        <div className="space-y-2.5">
          <Skeleton className="h-4 w-3/4 rounded-md" />
          <Skeleton className="h-4 w-2/3 rounded-md" />
          <Skeleton className="h-4 w-4/5 rounded-md" />
        </div>
      ) : simple ? (
        <p className="text-sm leading-relaxed text-muted-foreground">{t('admin.promos.deleteModal.simple')}</p>
      ) : (
        <div className="space-y-4">
          <ul className="space-y-2.5">
            {uses > 0 && (
              <Consequence>
                <Trans i18nKey="admin.promos.deleteModal.redemptions" values={{ count: uses }} components={{ b }} />
              </Consequence>
            )}
            {pending > 0 && (
              <Consequence>
                <Trans i18nKey="admin.promos.deleteModal.pending" values={{ count: pending }} components={{ b }} />
              </Consequence>
            )}
            <Consequence>{t('admin.promos.deleteModal.purchases')}</Consequence>
            <Consequence>{t('admin.promos.deleteModal.recreate')}</Consequence>
          </ul>
          {promo.active && (
            <p className="rounded-[14px] border border-border/60 bg-muted/40 px-3.5 py-3 text-[13.5px] text-muted-foreground">
              <Trans i18nKey="admin.promos.deleteModal.hint" components={{ b }} />
            </p>
          )}
        </div>
      )}
    </AdminModal>
  )
}

function Consequence({ children }: { children: ReactNode }) {
  return (
    <li className="flex items-start gap-2.5 text-sm">
      <X className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden />
      <span>{children}</span>
    </li>
  )
}
