import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Zap } from 'lucide-react'

import { AdminModal } from '../AdminModal'
import { AdminModalSaveFooter } from '../AdminModalSaveFooter'
import { cn } from '@/lib/utils'
import { surface } from '../Surface'
import type { AdminCustomerDTO, AdminTariffBriefDTO } from '@/lib/types/admin'
import { formatAdminApiError } from '../../utils/formatAdminApiError'
import { useAdminUserSetTariff } from '../../hooks/useAdminUsers'
import { useErrorHaptic } from '@/lib/haptic'

interface Props {
  open: boolean
  onClose: () => void
  userId: number
  customer: AdminCustomerDTO
  tariffs: AdminTariffBriefDTO[]
  onSuccess?: (message: string) => void
  onError?: (message: string) => void
}

export function AdminUserTariffModal({
  open,
  onClose,
  userId,
  customer,
  tariffs,
  onSuccess,
  onError,
}: Props) {
  const { t } = useTranslation()
  const currentTariffId = customer.current_tariff_id ?? null
  const setTariff = useAdminUserSetTariff(userId)

  const [draftTariffId, setDraftTariffId] = useState<number | null>(currentTariffId)
  const [error, setError] = useState<string | null>(null)
  useErrorHaptic(error)

  useEffect(() => {
    if (open) {
      setDraftTariffId(currentTariffId)
      setError(null)
    }
  }, [open, currentTariffId])

  const handleClose = () => {
    setError(null)
    onClose()
  }

  const handleSave = () => {
    if (draftTariffId == null || draftTariffId === currentTariffId) {
      handleClose()
      return
    }

    setError(null)
    setTariff.mutate(draftTariffId, {
      onSuccess: () => {
        onSuccess?.(t('admin.feedback.saved'))
        handleClose()
      },
      onError: (e) => {
        const msg = formatAdminApiError(e, t)
        setError(msg)
        onError?.(msg)
      },
    })
  }

  const hasChanges = draftTariffId != null && draftTariffId !== currentTariffId

  return (
    <AdminModal
      open={open}
      onClose={handleClose}
      title={t('admin.users.subscription.tariff')}
      description={t('admin.users.subscription.tariffHint')}
      icon={Zap}
      iconAccent="teal"
      size="md"
      footer={
        <AdminModalSaveFooter
          onCancel={handleClose}
          onSave={handleSave}
          isPending={setTariff.isPending}
          saveDisabled={!hasChanges}
        />
      }
    >
      <div className="space-y-4">
        {error && (
          <p className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        )}
        {tariffs.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('admin.noData')}</p>
        ) : (
          /*
            Список строками, а не чипами: у тарифов длинные имена, и в чипах
            они складывались в мозаику, по которой нельзя было пробежать
            глазами сверху вниз.
          */
          <div className="grid gap-2">
            {tariffs.map((tariff) => {
              const selected = draftTariffId === tariff.id
              return (
                <button
                  key={tariff.id}
                  type="button"
                  onClick={() => setDraftTariffId(tariff.id)}
                  className={cn(
                    'flex items-center gap-3 rounded-xl px-3 py-2.5 text-start text-sm transition-colors',
                    selected
                      ? 'border border-primary/50 bg-primary/10'
                      : surface('raised', 'hover:bg-accent/50'),
                  )}
                >
                  <span
                    className={cn(
                      'flex size-4 shrink-0 items-center justify-center rounded-full border',
                      selected ? 'border-primary' : 'border-muted-foreground/50',
                    )}
                  >
                    {selected && <span className="size-2 rounded-full bg-primary" />}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">{tariff.name}</span>
                    <span className="block truncate font-mono text-[11px] text-muted-foreground">
                      {tariff.slug}
                    </span>
                  </span>
                  {tariff.id === currentTariffId && (
                    <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
                      {t('admin.users.subscription.tariffCurrent')}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        )}
      </div>
    </AdminModal>
  )
}
