import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Check, Pencil, Trash2, X } from 'lucide-react'

import { DevicePlatformIcon } from '@/components/DevicePlatformIcon'
import { useToast } from '@/components/ui/toast'
import { api, type DeviceInfo } from '@/lib/api'

// Совпадает с DeviceNameMaxLen на бэкенде (VARCHAR(64) в device_name).
const DEVICE_NAME_MAX_LEN = 64

const iconButton =
  'inline-flex size-8 shrink-0 items-center justify-center rounded-[10px] text-muted-foreground transition-colors disabled:pointer-events-none disabled:opacity-50 sm:size-9'

/**
 * Строка устройства в «Мои устройства». Карандаш превращает название в поле
 * ввода: галочка сохраняет, крестик отменяет (Enter и Esc — то же самое).
 * Своё название показывается крупно, а исходное из Remnawave уходит в
 * подпись — чтобы было видно, что это за устройство на самом деле.
 */
export function DeviceRow({
  device,
  deleteDisabled,
  onDelete,
}: {
  device: DeviceInfo
  deleteDisabled: boolean
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const toast = useToast()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')

  const customName = device.custom_name?.trim() ?? ''
  const originalTitle = device.device_model || device.platform || device.hwid
  const details = [device.platform, device.os_version].filter(Boolean).join(' · ')
  const title = customName || originalTitle
  const subtitle = customName
    ? [originalTitle, details].filter(Boolean).join(' · ')
    : details || device.hwid

  const rename = useMutation({
    mutationFn: (name: string) => api.renameDevice(device.hwid, name),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['devices'] })
      setEditing(false)
    },
    onError: () => toast.error(t('subscriptionPage.renameDeviceError')),
  })

  function startEdit() {
    setDraft(title)
    setEditing(true)
  }

  function cancelEdit() {
    if (rename.isPending) return
    setEditing(false)
  }

  function save() {
    const name = draft.trim()
    // Ввели исходное название — значит, своё больше не нужно.
    const next = name === originalTitle ? '' : name
    if (next === customName) {
      setEditing(false)
      return
    }
    rename.mutate(next)
  }

  return (
    <li className="flex items-center justify-between gap-3 border-t border-border/[0.55] px-1 py-2.5 sm:gap-3.5">
      <div className="flex min-w-0 flex-1 items-center gap-3 sm:gap-3.5">
        {/* Иконка по платформе: ноутбук для macOS/Windows, телефон для мобильных. */}
        <span className="inline-flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/[0.12] text-primary">
          <DevicePlatformIcon platform={device.platform ?? device.device_model} className="size-5" />
        </span>
        {editing ? (
          <input
            autoFocus
            value={draft}
            maxLength={DEVICE_NAME_MAX_LEN}
            disabled={rename.isPending}
            placeholder={t('subscriptionPage.renameDevicePlaceholder')}
            aria-label={t('subscriptionPage.renameDevicePlaceholder')}
            onChange={(e) => setDraft(e.target.value)}
            onFocus={(e) => e.currentTarget.select()}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                save()
              } else if (e.key === 'Escape') {
                e.preventDefault()
                cancelEdit()
              }
            }}
            className="h-9 min-w-0 flex-1 rounded-lg border border-input bg-background px-3 text-sm text-foreground transition-colors placeholder:text-muted-foreground focus-visible:border-[hsl(var(--cabinet-accent)/0.5)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 dark:bg-input"
          />
        ) : (
          <div className="min-w-0">
            <p className="truncate text-[15px] font-semibold">{title}</p>
            <p className="mt-0.5 truncate text-[13px] text-muted-foreground">{subtitle}</p>
          </div>
        )}
      </div>

      <div className="flex shrink-0 items-center gap-0.5">
        {editing ? (
          <>
            <button
              type="button"
              disabled={rename.isPending}
              onClick={save}
              aria-label={t('subscriptionPage.renameDeviceSave')}
              title={t('subscriptionPage.renameDeviceSave')}
              className={`${iconButton} hover:bg-emerald-500/10 hover:text-emerald-500`}
            >
              <Check size={17} />
            </button>
            <button
              type="button"
              disabled={rename.isPending}
              onClick={cancelEdit}
              aria-label={t('subscriptionPage.renameDeviceCancel')}
              title={t('subscriptionPage.renameDeviceCancel')}
              className={`${iconButton} hover:bg-muted hover:text-foreground`}
            >
              <X size={17} />
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              onClick={startEdit}
              aria-label={t('subscriptionPage.renameDevice')}
              title={t('subscriptionPage.renameDevice')}
              className={`${iconButton} hover:bg-primary/10 hover:text-primary`}
            >
              <Pencil size={17} />
            </button>
            <button
              type="button"
              disabled={deleteDisabled}
              onClick={onDelete}
              aria-label={t('subscriptionPage.deleteDevice')}
              title={t('subscriptionPage.deleteDevice')}
              className={`${iconButton} hover:bg-destructive/10 hover:text-destructive`}
            >
              <Trash2 size={17} />
            </button>
          </>
        )}
      </div>
    </li>
  )
}
