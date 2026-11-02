import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeftRight, CalendarPlus, Lock, RotateCcw, TriangleAlert, type LucideIcon } from 'lucide-react'

import { cn } from '@/lib/utils'
import { AddDeviceIcon } from '@/components/AddDeviceIcon'
import { Button } from '@/components/ui/button'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import {
  DEFAULT_HOME_QUICK_ACTIONS,
  HOME_QUICK_ACTIONS,
  HOME_QUICK_ACTION_IDS,
  homeQuickActionAvailable,
  isHomeQuickActionId,
  parseHomeQuickActions,
  resolveHomeQuickActions,
  type HomeQuickActionId,
} from '@/features/home/homeQuickActions'
import { AdminSelect } from './AdminSelect'

interface AdminHomeQuickActionsEditorProps {
  /** Значение CABINET_HOME_QUICK_ACTIONS: «promo,invite». */
  value: string
  disabled?: boolean
  onChange: (next: string) => void
}

/**
 * Кнопки быстрых действий на главной кабинета. Места 1 и 2 показаны
 * заблокированными: их набор зависит от состояния подписки. Места 3 и 4
 * выбираются из списка разделов; выбор значения соседнего места меняет
 * кнопки местами, а не даёт сохранить две одинаковые.
 */
export function AdminHomeQuickActionsEditor({ value, disabled, onChange }: AdminHomeQuickActionsEditorProps) {
  const { t } = useTranslation()
  const { data: bootstrap } = useAuthBootstrap()
  const slots = useMemo(() => parseHomeQuickActions(value), [value])
  // Что увидят клиенты с учётом выключенных разделов.
  const shown = useMemo(
    () => resolveHomeQuickActions({ ...(bootstrap ?? { google_oauth_enabled: false }), home_quick_actions: slots }),
    [bootstrap, slots],
  )

  const options = useMemo(
    () => HOME_QUICK_ACTION_IDS.map((id) => ({ value: id, label: t(HOME_QUICK_ACTIONS[id].labelKey) })),
    [t],
  )

  const commit = (next: HomeQuickActionId[]) => onChange(next.join(','))

  const select = (index: number, id: HomeQuickActionId) => {
    const next = [...slots]
    const other = next.indexOf(id)
    if (other !== -1 && other !== index) next[other] = next[index]
    next[index] = id
    commit(next)
  }

  const isDefault = slots.every((id, i) => id === DEFAULT_HOME_QUICK_ACTIONS[i])

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t('admin.settings.homeQuickActions.intro')}</p>

      {/* На телефоне места 3 и 4 — во всю ширину, иначе название в селекте обрезается. */}
      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <LockedSlot n={1} icon={AddDeviceIcon} label={t('admin.settings.homeQuickActions.lockedExtra')} />
        <LockedSlot n={2} icon={CalendarPlus} label={t('admin.settings.homeQuickActions.lockedRenew')} />

        {slots.map((id, index) => {
          const Icon = HOME_QUICK_ACTIONS[id].icon
          const unavailable = !homeQuickActionAvailable(id, bootstrap)
          const slotLabel = t('admin.settings.homeQuickActions.slot', { n: index + 3 })
          return (
            <div
              key={index}
              className={cn(
                'col-span-2 flex min-w-0 flex-col gap-2 rounded-lg border bg-card p-2.5 shadow-sm sm:col-span-1',
                unavailable ? 'border-amber-500/50' : 'border-primary/40',
              )}
            >
              <div className="flex items-center gap-2">
                <span className="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-secondary text-primary">
                  <Icon className="size-4" aria-hidden />
                </span>
                <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{slotLabel}</span>
              </div>
              <AdminSelect
                value={id}
                options={options}
                onChange={(next) => {
                  if (next == null || !isHomeQuickActionId(next)) return
                  select(index, next)
                }}
                placeholder={slotLabel}
                ariaLabel={slotLabel}
                disabled={disabled}
              />
              {unavailable && (
                <p className="flex gap-1.5 text-[11px] leading-snug text-amber-700 dark:text-amber-400">
                  <TriangleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
                  {t('admin.settings.homeQuickActions.unavailable', {
                    fallback: t(HOME_QUICK_ACTIONS[shown[index]].labelKey),
                  })}
                </p>
              )}
            </div>
          )
        })}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled}
          onClick={() => commit([slots[1], slots[0]])}
        >
          <ArrowLeftRight className="mr-2 size-4" aria-hidden />
          {t('admin.settings.homeQuickActions.swap')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={disabled || isDefault}
          onClick={() => commit(DEFAULT_HOME_QUICK_ACTIONS)}
        >
          <RotateCcw className="mr-2 size-4" aria-hidden />
          {t('admin.settings.homeQuickActions.reset')}
        </Button>
      </div>

      <p className="text-xs text-muted-foreground">{t('admin.settings.homeQuickActions.expiredNote')}</p>
    </div>
  )
}

function LockedSlot({ n, icon: Icon, label }: { n: number; icon: LucideIcon; label: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-lg border border-dashed border-border/70 bg-muted/30 p-2.5">
      <div className="flex items-center gap-2">
        <span className="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-secondary text-muted-foreground">
          <Icon className="size-4" aria-hidden />
        </span>
        <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
          {t('admin.settings.homeQuickActions.slot', { n })}
        </span>
        <Lock className="ml-auto size-3.5 shrink-0 text-muted-foreground" aria-hidden />
      </div>
      <p className="flex min-h-10 items-center px-1 text-sm font-medium leading-tight text-muted-foreground">{label}</p>
      <p className="text-[11px] leading-snug text-muted-foreground/80">
        {t('admin.settings.homeQuickActions.lockedHint')}
      </p>
    </div>
  )
}
