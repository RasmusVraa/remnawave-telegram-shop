import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronUp, RotateCcw, TriangleAlert } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { cn } from '@/lib/utils'
import {
  YOOKASSA_METHOD_IDS,
  availableCheckoutMethods,
  checkoutMethodMeta,
  isYookassaMethodId,
  yookassaMethodsFrom,
  type YookassaMethodId,
} from '@/features/checkout/paymentMethods'
import { useProviderEnabled } from '@/features/checkout/checkoutShared'
import { AdminCheckbox } from './AdminCheckbox'
import { AdminSelect } from './AdminSelect'

/** «sbp,bank_card» → список; пусто — только форма со всеми способами. */
export function parseYookassaMethodsSetting(value: string): YookassaMethodId[] {
  const out: YookassaMethodId[] = []
  for (const raw of value.split(',')) {
    const id = raw.trim().toLowerCase()
    if (isYookassaMethodId(id) && !out.includes(id)) out.push(id)
  }
  return out.length > 0 ? out : ['all']
}

/** Обратное к parseYookassaMethodsSetting: одно «all» хранится пустой строкой. */
function serializeYookassaMethods(list: YookassaMethodId[]): string {
  if (list.length === 0 || (list.length === 1 && list[0] === 'all')) return ''
  return list.join(',')
}

/**
 * YOOKASA_PAYMENT_METHODS: какие кнопки ЮKassa показывать и в каком порядке.
 * Отмеченные идут сверху в порядке кнопок, остальные — ниже.
 */
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
  const unchecked = YOOKASSA_METHOD_IDS.filter((id) => !checked.includes(id))
  const commit = (next: YookassaMethodId[]) => onChange(serializeYookassaMethods(next))

  const toggle = (id: YookassaMethodId, on: boolean) => {
    if (on) commit([...checked, id])
    else commit(checked.filter((x) => x !== id))
  }
  const move = (index: number, delta: number) => {
    const next = [...checked]
    const j = index + delta
    if (j < 0 || j >= next.length) return
    ;[next[index], next[j]] = [next[j], next[index]]
    commit(next)
  }

  const label = (id: YookassaMethodId) =>
    id === 'all' ? t('admin.settings.yookassaMethods.all') : t(`checkout.yookassaMethod.${id}`)
  const isDefault = checked.length === 1 && checked[0] === 'all'
  const hasSpecific = checked.some((id) => id !== 'all')

  const row = (id: YookassaMethodId, on: boolean, index: number) => (
    <div
      key={id}
      className={cn(
        'flex items-center gap-3 rounded-lg border px-3 py-2',
        on ? 'border-primary/40 bg-card shadow-sm' : 'border-dashed border-border/70 bg-muted/20',
      )}
    >
      <AdminCheckbox checked={on} disabled={disabled} onChange={(v) => toggle(id, v)} aria-label={label(id)} />
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-muted dark:bg-white/[0.07]">
        {checkoutMethodMeta(`yookassa:${id}`, t).icon}
      </span>
      <div className="min-w-0 flex-1">
        <p className={cn('text-sm font-medium', !on && 'text-muted-foreground')}>{label(id)}</p>
        {id === 'all' && <p className="text-[11px] text-muted-foreground">{t('admin.settings.yookassaMethods.allHint')}</p>}
        {id !== 'all' && <p className="text-[11px] text-muted-foreground">{id}</p>}
      </div>
      {on && checked.length > 1 && (
        <div className="flex shrink-0 gap-1">
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="h-7 w-7"
            disabled={disabled || index === 0}
            onClick={() => move(index, -1)}
            aria-label={t('admin.settings.yookassaMethods.moveUp')}
          >
            <ChevronUp className="size-4" />
          </Button>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="h-7 w-7"
            disabled={disabled || index === checked.length - 1}
            onClick={() => move(index, 1)}
            aria-label={t('admin.settings.yookassaMethods.moveDown')}
          >
            <ChevronDown className="size-4" />
          </Button>
        </div>
      )}
    </div>
  )

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t('admin.settings.yookassaMethods.intro')}</p>
      <div className="space-y-1.5">
        {checked.map((id, i) => row(id, true, i))}
        {unchecked.map((id) => row(id, false, -1))}
      </div>
      {hasSpecific && (
        <p className="flex gap-1.5 text-[11px] leading-snug text-amber-700 dark:text-amber-400">
          <TriangleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
          {t('admin.settings.yookassaMethods.connectWarning')}
          {!checked.includes('all') && ` ${t('admin.settings.yookassaMethods.noAllWarning')}`}
        </p>
      )}
      <Button
        type="button"
        size="sm"
        variant="ghost"
        disabled={disabled || isDefault}
        onClick={() => commit(['all'])}
      >
        <RotateCcw className="mr-2 size-4" aria-hidden />
        {t('admin.settings.yookassaMethods.reset')}
      </Button>
    </div>
  )
}

/**
 * CABINET_CHECKOUT_DEFAULT_METHOD: способ, выбранный заранее для тех, кто
 * ещё не платил. Список — включённые сейчас способы, с кнопками ЮKassa из
 * черновика соседней настройки.
 */
export function AdminCheckoutDefaultMethodEditor({
  value,
  yookassaMethodsValue,
  disabled,
  onChange,
}: {
  value: string
  /** Черновик YOOKASA_PAYMENT_METHODS — чтобы список кнопок совпадал с тем, что сохранят. */
  yookassaMethodsValue: string
  disabled?: boolean
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data: bootstrap } = useAuthBootstrap()
  const providerEnabled = useProviderEnabled()
  const yookassaMethods = yookassaMethodsFrom(parseYookassaMethodsSetting(yookassaMethodsValue))
  const methods = availableCheckoutMethods(providerEnabled, yookassaMethods)

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
