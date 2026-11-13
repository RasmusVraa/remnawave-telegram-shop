import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { GripVertical, TriangleAlert } from 'lucide-react'

import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { cn } from '@/lib/utils'
import {
  PLATEGA_METHOD_IDS,
  YOOKASSA_METHOD_IDS,
  availableCheckoutMethods,
  checkoutMethodMeta,
  isPlategaMethodId,
  isYookassaMethodId,
  plategaCheckoutId,
  plategaMethodsFrom,
  yookassaMethodsFrom,
  type PlategaMethodId,
  type YookassaMethodId,
} from '@/features/checkout/paymentMethods'
import { useProviderEnabled } from '@/features/checkout/checkoutShared'
import { useSortableList } from '../hooks/useSortableList'
import { AdminSelect } from './AdminSelect'
import { AdminToggleSwitch } from './AdminToggleSwitch'

/** «sbp,bank_card» → список; пусто — только форма со всеми способами. */
export function parseYookassaMethodsSetting(value: string): YookassaMethodId[] {
  const out: YookassaMethodId[] = []
  for (const raw of value.split(',')) {
    const id = raw.trim().toLowerCase()
    if (isYookassaMethodId(id) && !out.includes(id)) out.push(id)
  }
  return out.length > 0 ? out : ['all']
}

/** «sbp,crypto» → список; пусто — только общая форма Platega. */
export function parsePlategaMethodsSetting(value: string): PlategaMethodId[] {
  const out: PlategaMethodId[] = []
  for (const raw of value.split(',')) {
    const id = raw.trim().toLowerCase()
    if (isPlategaMethodId(id) && !out.includes(id)) out.push(id)
  }
  return out.length > 0 ? out : ['all']
}

/** Одно «all» хранится пустой строкой. */
function serializeMethods(list: string[]): string {
  if (list.length === 0 || (list.length === 1 && list[0] === 'all')) return ''
  return list.join(',')
}

interface MethodItem {
  id: string
  label: string
  /** Серая строка под названием: код способа или пояснение. */
  hint: string
  icon: ReactNode
}

/**
 * Список кнопок оплаты одного провайдера: включённые идут сверху в порядке
 * кнопок и перетаскиваются за ручку, выключенные — ниже. Тумблер справа, как
 * у остальных настроек. Последнюю включённую кнопку выключить нельзя: без
 * кнопок оплата остаётся ни с чем.
 */
function AdminMethodList({
  items,
  enabledIds,
  disabled,
  warning,
  onChange,
}: {
  items: MethodItem[]
  enabledIds: string[]
  disabled?: boolean
  warning?: string | null
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation()
  const byId = new Map(items.map((i) => [i.id, i]))
  const enabled = enabledIds.filter((id) => byId.has(id))
  const off = items.filter((i) => !enabled.includes(i.id))
  const sortable = useSortableList(enabled, onChange)

  const row = (item: MethodItem, on: boolean) => {
    const dragging = on && sortable.dragId === item.id
    const lastOn = on && enabled.length === 1
    return (
      <li
        key={item.id}
        ref={on ? sortable.itemRef(item.id) : undefined}
        className={cn(
          'flex items-center gap-3 rounded-xl border border-border/60 bg-background/40 p-3 transition-[opacity,box-shadow,border-color,background-color] duration-200',
          !on && 'opacity-60',
          dragging && 'relative z-20 border-primary/60 bg-card opacity-100 shadow-2xl shadow-black/40',
        )}
      >
        {on ? (
          <button
            type="button"
            aria-label={t('admin.statusPage.drag')}
            title={t('admin.statusPage.drag')}
            disabled={disabled || enabled.length < 2}
            {...sortable.handleProps(item.id)}
            className={cn(
              '-ml-1 flex h-9 w-6 shrink-0 touch-none items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-30',
              dragging ? 'cursor-grabbing' : 'cursor-grab',
            )}
          >
            <GripVertical className="size-5" />
          </button>
        ) : (
          <span className="-ml-1 h-9 w-6 shrink-0" aria-hidden />
        )}
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-muted dark:bg-white/[0.07]">
          {item.icon}
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium leading-snug">{item.label}</p>
          <p className="text-xs text-muted-foreground">{item.hint}</p>
        </div>
        <AdminToggleSwitch
          checked={on}
          disabled={disabled || lastOn}
          aria-label={item.label}
          onChange={(v) => onChange(v ? [...enabled, item.id] : enabled.filter((x) => x !== item.id))}
        />
      </li>
    )
  }

  return (
    <div className="space-y-3">
      <ul
        ref={sortable.listProps.ref}
        onPointerMove={sortable.listProps.onPointerMove}
        onPointerUp={sortable.listProps.onPointerUp}
        onPointerCancel={sortable.listProps.onPointerCancel}
        className={cn('relative space-y-2', sortable.dragId && 'select-none')}
      >
        {enabled.map((id) => row(byId.get(id)!, true))}
      </ul>
      {off.length > 0 && <ul className="space-y-2">{off.map((item) => row(item, false))}</ul>}
      {warning && (
        <p className="flex gap-1.5 text-[11px] leading-snug text-amber-700 dark:text-amber-400">
          <TriangleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
          {warning}
        </p>
      )}
    </div>
  )
}

/** YOOKASA_PAYMENT_METHODS: какие кнопки ЮKassa показывать и в каком порядке. */
export function AdminYookassaMethodsEditor({
  value,
  disabled,
  onChange,
}: {
  value: string
  disabled?: boolean
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const checked = useMemo(() => parseYookassaMethodsSetting(value), [value])
  const items: MethodItem[] = YOOKASSA_METHOD_IDS.map((id) => ({
    id,
    label: id === 'all' ? t('admin.settings.yookassaMethods.all') : t(`checkout.yookassaMethod.${id}`),
    hint: id === 'all' ? t('admin.settings.yookassaMethods.allHint') : id,
    icon: checkoutMethodMeta(`yookassa:${id}`, t).icon,
  }))
  const hasSpecific = checked.some((id) => id !== 'all')
  const warning = hasSpecific
    ? `${t('admin.settings.yookassaMethods.connectWarning')}${
        checked.includes('all') ? '' : ` ${t('admin.settings.yookassaMethods.noAllWarning')}`
      }`
    : null

  return (
    <AdminMethodList
      items={items}
      enabledIds={checked}
      disabled={disabled}
      warning={warning}
      onChange={(next) => onChange(serializeMethods(next))}
    />
  )
}

/** PLATEGA_PAYMENT_METHODS: какие кнопки Platega показывать и в каком порядке. */
export function AdminPlategaMethodsEditor({
  value,
  disabled,
  onChange,
}: {
  value: string
  disabled?: boolean
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const checked = useMemo(() => parsePlategaMethodsSetting(value), [value])
  const items: MethodItem[] = PLATEGA_METHOD_IDS.map((id) => ({
    id,
    label: t(`admin.settings.plategaMethods.${id}`),
    hint: id === 'all' ? t('admin.settings.plategaMethods.allHint') : id,
    icon: checkoutMethodMeta(plategaCheckoutId(id), t).icon,
  }))
  const hasSpecific = checked.some((id) => id !== 'all')

  return (
    <AdminMethodList
      items={items}
      enabledIds={checked}
      disabled={disabled}
      warning={hasSpecific ? t('admin.settings.plategaMethods.connectWarning') : null}
      onChange={(next) => onChange(serializeMethods(next))}
    />
  )
}

/**
 * CABINET_CHECKOUT_DEFAULT_METHOD: способ, выбранный заранее для тех, кто
 * ещё не платил. Список — включённые сейчас способы, с кнопками ЮKassa и
 * Platega из черновиков соседних настроек.
 */
export function AdminCheckoutDefaultMethodEditor({
  value,
  yookassaMethodsValue,
  plategaMethodsValue,
  disabled,
  onChange,
}: {
  value: string
  /** Черновик YOOKASA_PAYMENT_METHODS — чтобы список кнопок совпадал с тем, что сохранят. */
  yookassaMethodsValue: string
  /** Черновик PLATEGA_PAYMENT_METHODS. */
  plategaMethodsValue: string
  disabled?: boolean
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data: bootstrap } = useAuthBootstrap()
  const providerEnabled = useProviderEnabled()
  const yookassaMethods = yookassaMethodsFrom(parseYookassaMethodsSetting(yookassaMethodsValue))
  const plategaMethods = plategaMethodsFrom(parsePlategaMethodsSetting(plategaMethodsValue))
  // Платёжная система целиком выключена — её кнопок в списке нет, даже если способ отмечен.
  const anyPlatega = PLATEGA_METHOD_IDS.some((id) => providerEnabled[plategaCheckoutId(id) as keyof typeof providerEnabled])
  const enabledForList = anyPlatega
    ? { ...providerEnabled, ...Object.fromEntries(PLATEGA_METHOD_IDS.map((id) => [plategaCheckoutId(id), true])) }
    : providerEnabled
  const methods = availableCheckoutMethods(enabledForList, yookassaMethods, plategaMethods)

  const options = methods.map((id) => {
    const meta = checkoutMethodMeta(id, t)
    return { value: id, label: `${meta.label} · ${meta.provider}` }
  })
  const current = value.trim().toLowerCase()
  const unavailable = current !== '' && bootstrap != null && !methods.includes(current)

  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">{t('admin.settings.checkoutDefault.intro')}</p>
      <AdminSelect
        value={current === '' ? null : current}
        options={unavailable ? [...options, { value: current, label: current }] : options}
        onChange={(next) => onChange(next ?? '')}
        allowEmpty
        emptyLabel={t('admin.settings.checkoutDefault.none')}
        placeholder={t('admin.settings.checkoutDefault.none')}
        ariaLabel={t('admin.settings.fields.CABINET_CHECKOUT_DEFAULT_METHOD.label')}
        disabled={disabled}
      />
      {unavailable && (
        <p className="flex gap-1.5 text-[11px] leading-snug text-amber-700 dark:text-amber-400">
          <TriangleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
          {t('admin.settings.checkoutDefault.unavailable')}
        </p>
      )}
    </div>
  )
}
