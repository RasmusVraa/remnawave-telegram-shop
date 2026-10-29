import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Smartphone } from 'lucide-react'

import { AdminModal } from '../AdminModal'
import { AdminModalSaveFooter } from '../AdminModalSaveFooter'
import { AdminDeviceLimitEditor } from '../AdminDeviceLimitEditor'
import { formatAdminApiError } from '../../utils/formatAdminApiError'
import { baseLimitFromTotal } from '../../utils/deviceLimit'
import {
  useAdminUserDevices,
  useAdminUserSetHwidLimit,
  useAdminUserExtraHwid,
  useAdminUserSetHwidPin,
  type AdminUserPanelResponse,
} from '../../hooks/useAdminUsers'

interface Props {
  open: boolean
  onClose: () => void
  userId: number
  panel: AdminUserPanelResponse
  onSuccess?: (message: string) => void
  onError?: (message: string) => void
}

export function AdminUserDevicesModal({
  open,
  onClose,
  userId,
  panel,
  onSuccess,
  onError,
}: Props) {
  const { t } = useTranslation()
  const rw = panel.rw!
  const customer = panel.customer
  const totalLimit = rw.hwid_device_limit ?? 1
  const storedExtra = customer?.extra_hwid ?? 0
  const initialPin = customer?.hwid_limit_override ?? null
  const initialPinned = initialPin != null
  const unpinnedLimit = panel.hwid_unpinned_limit
  // Пока лимит закреплён, в панели лежит закреплённое число, а не тариф +
  // слоты: базу для обычного режима считаем от расчётного лимита.
  const unpinnedTotal = initialPinned ? (unpinnedLimit ?? totalLimit) : totalLimit
  const initialBase = baseLimitFromTotal(unpinnedTotal, customer)

  const { data: devicesData, isLoading: devicesLoading } = useAdminUserDevices(open ? userId : null)
  const setHwid = useAdminUserSetHwidLimit(userId)
  const extraHwid = useAdminUserExtraHwid(userId)
  const setPin = useAdminUserSetHwidPin(userId)

  const [draftBase, setDraftBase] = useState(initialBase)
  const [draftExtra, setDraftExtra] = useState(storedExtra)
  const [pinned, setPinned] = useState(initialPinned)
  const [draftPin, setDraftPin] = useState(initialPin ?? totalLimit)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setDraftBase(initialBase)
      setDraftExtra(storedExtra)
      setPinned(initialPinned)
      setDraftPin(initialPin ?? totalLimit)
      setError(null)
    }
  }, [open, initialBase, storedExtra, initialPinned, initialPin, totalLimit])

  const handleClose = () => {
    setError(null)
    onClose()
  }

  const targetTotal = draftBase + draftExtra
  const extraDelta = draftExtra - storedExtra
  const totalChanged = targetTotal !== unpinnedTotal
  const pinChanged = pinned !== initialPinned || (pinned && draftPin !== initialPin)
  const hasChanges = pinned
    ? pinChanged
    : pinChanged || draftBase !== initialBase || extraDelta !== 0

  const handleSave = async () => {
    setError(null)

    if (!hasChanges) {
      handleClose()
      return
    }

    try {
      if (pinned) {
        await setPin.mutateAsync(draftPin)
      } else {
        // Снятие закрепления сервер сам пересчитывает в тариф + слоты;
        // правки базы и слотов, если были, ложатся уже поверх.
        if (initialPinned) {
          await setPin.mutateAsync(null)
        }
        if (extraDelta !== 0) {
          await extraHwid.mutateAsync(extraDelta)
        }
        if (totalChanged) {
          await setHwid.mutateAsync(targetTotal)
        }
      }
      onSuccess?.(t('admin.feedback.saved'))
      handleClose()
    } catch (e) {
      const msg = formatAdminApiError(e, t)
      setError(msg)
      onError?.(msg)
    }
  }

  const isPending = setHwid.isPending || extraHwid.isPending || setPin.isPending

  return (
    <AdminModal
      open={open}
      onClose={handleClose}
      title={t('admin.users.subscription.devicesTitle')}
      icon={Smartphone}
      iconAccent="cyan"
      size="lg"
      footer={
        <AdminModalSaveFooter
          onCancel={handleClose}
          onSave={() => void handleSave()}
          isPending={isPending}
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
        <AdminDeviceLimitEditor
          userId={userId}
          customer={customer}
          devices={devicesData?.items ?? []}
          devicesLoading={devicesLoading}
          draftBase={draftBase}
          draftExtra={draftExtra}
          onDraftBaseChange={setDraftBase}
          onDraftExtraChange={setDraftExtra}
          pinned={pinned}
          onPinnedChange={setPinned}
          draftPin={draftPin}
          onDraftPinChange={setDraftPin}
          unpinnedLimit={unpinnedLimit}
          onSuccess={(msg) => onSuccess?.(msg ?? t('admin.feedback.deviceDeleted'))}
          onError={(e) => {
            const msg = formatAdminApiError(e, t)
            setError(msg)
            onError?.(msg)
          }}
        />
      </div>
    </AdminModal>
  )
}

