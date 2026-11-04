import { useTranslation } from 'react-i18next'
import {
  CalendarPlus,
  Check,
  Copy,
  Loader2,
  Power,
  PowerOff,
  Shield,
  Trash2,
  Zap,
  type LucideIcon,
} from 'lucide-react'

import { AdminModal } from '../AdminModal'
import { cn } from '@/lib/utils'

interface ActionItem {
  key: string
  label: string
  icon: LucideIcon
  onClick: () => void
  disabled?: boolean
  pending?: boolean
  variant?: 'default' | 'success' | 'danger'
}

interface Props {
  open: boolean
  onClose: () => void
  hasRwUser: boolean
  rwStatus?: string
  onExtend: () => void
  onDisable: () => void
  onEnable: () => void
  onDelete: () => void
  disablePending?: boolean
  enablePending?: boolean
  /** Копирование ссылки на подписку; недоступно, когда ссылки нет. */
  copy: { available: boolean; copied: boolean; copy: () => void }
  /** Смена тарифа доступна только в режиме продаж «тарифы». */
  onChangeTariff?: () => void
}

const variantClasses: Record<NonNullable<ActionItem['variant']>, string> = {
  default: 'border border-border bg-secondary hover:bg-accent',
  success:
    'border border-emerald-500/40 bg-emerald-500/10 font-medium text-emerald-700 hover:bg-emerald-500/15 dark:text-emerald-400',
  danger:
    'border border-red-500/40 bg-red-500/10 font-medium text-red-700 hover:bg-red-500/15 dark:text-red-400',
}

/**
 * Меню действий на телефоне.
 *
 * Здесь тот же набор, что и в колонке действий на ПК: пряталось бы что-то
 * одно — админу пришлось бы искать это на другом устройстве. Удаление
 * отделено чертой, чтобы не попадать по нему вслепую.
 */
export function AdminUserActionsModal({
  open,
  onClose,
  hasRwUser,
  rwStatus,
  onExtend,
  onDisable,
  onEnable,
  onDelete,
  disablePending,
  enablePending,
  copy,
  onChangeTariff,
}: Props) {
  const { t } = useTranslation()
  const isActive = rwStatus?.toUpperCase() === 'ACTIVE'

  const run = (fn: () => void) => () => {
    onClose()
    fn()
  }

  const actions: ActionItem[] = [
    {
      key: 'extend',
      label: t('admin.users.extend'),
      icon: CalendarPlus,
      onClick: run(onExtend),
      variant: 'success',
    },
    ...(copy.available
      ? [{
          key: 'copy',
          label: copy.copied
            ? t('admin.users.copyLinkSuccess')
            : t('admin.users.copySubscriptionLink'),
          icon: copy.copied ? Check : Copy,
          // Окно не закрывается: подпись на кнопке — единственное
          // подтверждение, что ссылка легла в буфер.
          onClick: copy.copy,
        }]
      : []),
    ...(onChangeTariff
      ? [{
          key: 'tariff',
          label: t('admin.users.changeTariff'),
          icon: Zap,
          onClick: run(onChangeTariff),
        }]
      : []),
    ...(hasRwUser
      ? isActive
        ? [{
            key: 'disable',
            label: t('admin.users.disable'),
            icon: PowerOff,
            onClick: run(onDisable),
            pending: disablePending,
          }]
        : [{
            key: 'enable',
            label: t('admin.users.enable'),
            icon: Power,
            onClick: run(onEnable),
            pending: enablePending,
            variant: 'success' as const,
          }]
      : []),
  ]

  return (
    <AdminModal
      open={open}
      onClose={onClose}
      title={t('admin.users.actionsModal.title')}
      icon={Shield}
      iconAccent="indigo"
      size="sm"
    >
      <div className="grid gap-2">
        {!hasRwUser && (
          <p className="mb-1 rounded-btn border border-dashed px-3 py-2 text-xs text-muted-foreground">
            {t('admin.users.subscription.noRwUser')}
          </p>
        )}
        {actions.map((action) => (
          <button
            key={action.key}
            type="button"
            onClick={action.onClick}
            disabled={action.disabled || action.pending}
            className={cn(
              'flex items-center gap-2 rounded-btn px-3 py-2.5 text-sm disabled:opacity-50',
              variantClasses[action.variant ?? 'default'],
              (action.variant ?? 'default') === 'default' && '[&_svg]:text-primary',
              action.pending && '!opacity-100',
            )}
          >
            {action.pending ? (
              <Loader2 className="size-4 shrink-0 animate-spin" />
            ) : (
              <action.icon className="size-4 shrink-0" />
            )}
            {action.label}
          </button>
        ))}

        <div className="my-1 h-px bg-border" />

        <button
          type="button"
          onClick={run(onDelete)}
          className={cn(
            'flex items-center gap-2 rounded-btn px-3 py-2.5 text-sm',
            variantClasses.danger,
          )}
        >
          <Trash2 className="size-4 shrink-0" />
          {t('admin.users.delete')}
        </button>
      </div>
    </AdminModal>
  )
}
