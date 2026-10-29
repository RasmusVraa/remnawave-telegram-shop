import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, Minus, Plus, Trash2 } from 'lucide-react'

import { DevicePlatformIcon } from '@/components/DevicePlatformIcon'
import { cn } from '@/lib/utils'
import { surface } from './Surface'
import { AdminToggleSwitch } from './AdminToggleSwitch'
import type { AdminCustomerDTO, AdminDeviceDTO } from '@/lib/types/admin'
import { useAdminUserDeleteDevice } from '../hooks/useAdminUsers'
import { activeExtraHwidSlots } from '../utils/deviceLimit'
import { formatAdminDateTime } from '../utils/datetime'

const MIN_BASE = 1
const MAX_TOTAL = 100

function Stepper({
  label,
  hint,
  value,
  onChange,
  min,
  max,
  editable,
  decreaseLabel,
  increaseLabel,
}: {
  label: string
  hint?: string
  value: number
  onChange: (next: number) => void
  min: number
  max: number
  editable?: boolean
  decreaseLabel: string
  increaseLabel: string
}) {
  const [draft, setDraft] = useState<string | null>(null)
  const btn = surface(
    'raised',
    'inline-flex size-8 shrink-0 items-center justify-center rounded-lg hover:bg-accent disabled:opacity-40',
  )

  const commit = (raw: string) => {
    const parsed = parseInt(raw, 10)
    setDraft(null)
    if (Number.isNaN(parsed)) return
    onChange(Math.min(max, Math.max(min, parsed)))
  }

  return (
    <div className="min-w-0">
      <p className="text-xs font-medium">{label}</p>
      {hint && <p className="mt-0.5 text-[11px] leading-snug text-muted-foreground">{hint}</p>}
      <div className="mt-2 flex items-center gap-2">
        <button
          type="button"
          onClick={() => onChange(Math.max(min, value - 1))}
          disabled={value <= min}
          className={btn}
          aria-label={decreaseLabel}
        >
          <Minus className="size-4" />
        </button>
        {editable ? (
          <input
            type="number"
            value={draft ?? String(value)}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={(e) => commit(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                commit((e.target as HTMLInputElement).value)
              }
            }}
            className="admin-input h-8 w-12 px-1 text-center text-sm font-semibold tabular-nums [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
          />
        ) : (
          <span className="admin-input flex h-8 w-12 items-center justify-center px-1 text-center text-sm font-semibold tabular-nums">
            {value}
          </span>
        )}
        <button
          type="button"
          onClick={() => onChange(Math.min(max, value + 1))}
          disabled={value >= max}
          className={btn}
          aria-label={increaseLabel}
        >
          <Plus className="size-4" />
        </button>
      </div>
    </div>
  )
}

interface Props {
  userId: number
  customer?: AdminCustomerDTO | null
  devices: AdminDeviceDTO[]
  devicesLoading?: boolean
  /** Черновик лимитов: сохраняются кнопкой в футере окна, а не на каждый клик. */
  draftBase: number
  draftExtra: number
  onDraftBaseChange: (value: number) => void
  onDraftExtraChange: (value: number) => void
  /** Закрепить лимит: одно число вместо «база + доп. слоты», оплаты его не трогают. */
  pinned: boolean
  onPinnedChange: (value: boolean) => void
  draftPin: number
  onDraftPinChange: (value: number) => void
  /** Лимит без закрепления — с ним сравнивается закреплённое число. */
  unpinnedLimit?: number
  onSuccess?: (message?: string) => void
  onError?: (err: unknown) => void
}

/**
 * Устройства: лимиты сверху одной строкой, подключённые HWID — списком.
 *
 * Раньше лимиты занимали две карточки и три плитки-«итога», а список уезжал
 * под сгиб окна; отвязка устройства спрашивала подтверждение через
 * `window.confirm` — системное окно поверх модалки, вырывающее из контекста.
 * Теперь подтверждение живёт в той же строке, что и кнопка.
 */
export function AdminDeviceLimitEditor({
  userId,
  customer,
  devices,
  devicesLoading,
  draftBase,
  draftExtra,
  onDraftBaseChange,
  onDraftExtraChange,
  pinned,
  onPinnedChange,
  draftPin,
  onDraftPinChange,
  unpinnedLimit,
  onSuccess,
  onError,
}: Props) {
  const { t, i18n } = useTranslation()
  const dateLocale = i18n.language?.startsWith('en') ? 'en-GB' : 'ru-RU'
  const deleteDevice = useAdminUserDeleteDevice(userId)
  const [confirmHwid, setConfirmHwid] = useState<string | null>(null)

  const connected = devices.length
  const total = pinned ? draftPin : draftBase + draftExtra
  const overLimit = !devicesLoading && connected > total
  const pinBelowTariff = pinned && unpinnedLimit != null && draftPin < unpinnedLimit
  const full = total > 0 && connected >= total
  const percent = total > 0 ? Math.min(100, (connected / total) * 100) : 0

  const storedExtra = customer?.extra_hwid ?? 0
  const storedActiveExtra = activeExtraHwidSlots(customer)

  return (
    <div className="space-y-4">
      <div className={surface('raised', 'rounded-xl p-3')}>
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <p className="text-sm font-medium tabular-nums">
            {t('admin.users.subscription.devicesUsage', { used: connected, limit: total })}
          </p>
          {full && (
            <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-400">
              {t('admin.users.subscription.devicesFull')}
            </span>
          )}
        </div>
        <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted">
          <div
            className={cn('h-full rounded-full transition-all', full ? 'bg-red-500' : 'bg-primary')}
            style={{ width: `${Math.max(2, percent)}%` }}
          />
        </div>

        <div className="mt-4 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <p className="text-xs font-medium">{t('admin.users.subscription.pinLimitTitle')}</p>
            <p className="mt-0.5 text-[11px] leading-snug text-muted-foreground">
              {t('admin.users.subscription.pinLimitHint')}
            </p>
          </div>
          <AdminToggleSwitch
            checked={pinned}
            onChange={onPinnedChange}
            aria-label={t('admin.users.subscription.pinLimitTitle')}
          />
        </div>

        {pinned ? (
          <div className="mt-4">
            <Stepper
              label={t('admin.users.subscription.pinnedLimitTitle')}
              hint={
                unpinnedLimit != null
                  ? t('admin.users.subscription.pinnedLimitHint', { n: unpinnedLimit })
                  : undefined
              }
              value={draftPin}
              onChange={onDraftPinChange}
              min={MIN_BASE}
              max={MAX_TOTAL}
              editable
              decreaseLabel={t('admin.users.subscription.decreaseLimit')}
              increaseLabel={t('admin.users.subscription.increaseLimit')}
            />
            {pinBelowTariff && (
              <p className="mt-2 text-xs text-amber-700 dark:text-amber-400">
                {t('admin.users.subscription.pinBelowTariff', { n: unpinnedLimit })}
              </p>
            )}
          </div>
        ) : (
          <div className="mt-4 flex flex-wrap items-end gap-x-6 gap-y-4">
            <Stepper
              label={t('admin.users.subscription.baseLimitTitle')}
              hint={t('admin.users.subscription.baseLimitHint')}
              value={draftBase}
              onChange={onDraftBaseChange}
              min={MIN_BASE}
              max={MAX_TOTAL - draftExtra}
              editable
              decreaseLabel={t('admin.users.subscription.decreaseLimit')}
              increaseLabel={t('admin.users.subscription.increaseLimit')}
            />
            <Stepper
              label={t('admin.users.subscription.extraHwidTitle')}
              hint={t('admin.users.subscription.extraHwidHint')}
              value={draftExtra}
              onChange={onDraftExtraChange}
              min={0}
              max={MAX_TOTAL - draftBase}
              decreaseLabel={t('admin.users.subscription.decreaseExtra')}
              increaseLabel={t('admin.users.subscription.increaseExtra')}
            />
            {/* На узком экране «Итого» уезжает на свою строку, иначе читается
                как подпись к соседнему степперу. */}
            <div className="w-full text-end sm:ms-auto sm:w-auto">
              <p className="text-xs text-muted-foreground">{t('admin.users.subscription.totalLimit')}</p>
              <p className="text-2xl font-semibold tabular-nums text-primary">{total}</p>
            </div>
          </div>
        )}

        {overLimit && (
          <p className="mt-2 text-xs text-amber-700 dark:text-amber-400">
            {t('admin.users.subscription.devicesOverLimit', { used: connected, limit: total })}
          </p>
        )}

        {!pinned && customer?.extra_hwid_expires_at && draftExtra > 0 && (
          <p className="mt-2 text-xs text-muted-foreground">
            {t('admin.users.subscription.extraHwidExpires', {
              date: formatAdminDateTime(customer.extra_hwid_expires_at, dateLocale),
            })}
          </p>
        )}
        {!pinned && storedExtra > 0 && storedActiveExtra === 0 && (
          <p className="mt-2 text-xs text-muted-foreground">
            {t('admin.users.subscription.extraHwidInactive', { count: storedExtra })}
          </p>
        )}
      </div>

      <div>
        <p className="mb-2 text-sm font-medium">
          {t('admin.users.subscription.connectedDevices', { count: connected })}
        </p>
        {devicesLoading ? (
          <div className="flex justify-center py-6">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : devices.length === 0 ? (
          <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
            {t('admin.users.subscription.noDevices')}
          </p>
        ) : (
          <ul className="space-y-2">
            {devices.map((d) => {
              const confirming = confirmHwid === d.hwid
              return (
                <li
                  key={d.hwid}
                  className={surface(
                    'raised',
                    cn(
                      'flex items-center gap-3 rounded-xl p-2.5',
                      confirming && 'border-destructive/40 bg-destructive/5',
                    ),
                  )}
                >
                  <div className={surface('well', 'flex size-9 shrink-0 items-center justify-center rounded-lg')}>
                    <DevicePlatformIcon platform={d.platform} className="text-muted-foreground" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">
                      {[d.platform, d.device_model].filter(Boolean).join(' · ') || '—'}
                    </p>
                    <p className="truncate font-mono text-[11px] text-muted-foreground">{d.hwid}</p>
                  </div>

                  {confirming ? (
                    <div className="flex shrink-0 items-center gap-1.5">
                      <span className="hidden text-xs text-destructive sm:inline">
                        {t('admin.users.subscription.confirmDeleteDeviceShort')}
                      </span>
                      <button
                        type="button"
                        onClick={() => setConfirmHwid(null)}
                        className="rounded-lg px-2 py-1.5 text-xs hover:bg-accent"
                      >
                        {t('admin.cancel')}
                      </button>
                      <button
                        type="button"
                        disabled={deleteDevice.isPending}
                        onClick={() =>
                          deleteDevice.mutate(d.hwid, {
                            onSuccess: () => {
                              setConfirmHwid(null)
                              onSuccess?.(t('admin.feedback.deviceDeleted'))
                            },
                            onError: (e) => {
                              setConfirmHwid(null)
                              onError?.(e)
                            },
                          })
                        }
                        className="inline-flex items-center gap-1.5 rounded-lg bg-destructive px-2.5 py-1.5 text-xs font-medium text-destructive-foreground disabled:opacity-50"
                      >
                        {deleteDevice.isPending && <Loader2 className="size-3 animate-spin" />}
                        {t('admin.users.subscription.unbindDevice')}
                      </button>
                    </div>
                  ) : (
                    <button
                      type="button"
                      onClick={() => setConfirmHwid(d.hwid)}
                      title={t('admin.users.subscription.unbindDevice')}
                      className="shrink-0 rounded-lg border border-destructive/30 p-1.5 text-destructive hover:bg-destructive/10"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  )}
                </li>
              )
            })}
          </ul>
        )}
        <p className="mt-2 text-xs text-muted-foreground">
          {t('admin.users.subscription.unbindDeviceHint')}
        </p>
      </div>
    </div>
  )
}
