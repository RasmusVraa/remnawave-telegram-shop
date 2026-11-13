import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check } from 'lucide-react'

import { ProviderMethodButton } from '@/components/ProviderMethodButton'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { hapticSelection } from '@/lib/haptic'
import { cn } from '@/lib/utils'
import {
  checkoutMethodMeta,
  type CheckoutMethodId,
  type InitialMethodSource,
} from '@/features/checkout/paymentMethods'

/**
 * Выбор способа оплаты на странице оформления.
 *
 * Вид зависит от того, откуда взят способ при открытии страницы:
 * - прошлая оплата — одна строка с этим способом и «Изменить», полный список
 *   раскрывается по нажатию;
 * - способ по умолчанию от админа (человек ещё не платил) — крупная карточка
 *   «Рекомендуем» и остальные способы плитками;
 * - ни того ни другого — прежний список, ничего не выбрано.
 */
export function PaymentMethodPicker({
  methods,
  method,
  initialId,
  initialSource,
  loading,
  onSelect,
}: {
  methods: CheckoutMethodId[]
  method: CheckoutMethodId | null
  initialId: CheckoutMethodId | null
  initialSource: InitialMethodSource
  loading: boolean
  onSelect: (id: CheckoutMethodId) => void
}) {
  const { t } = useTranslation()
  const select = (id: CheckoutMethodId) => {
    if (id !== method) hapticSelection()
    onSelect(id)
  }

  let body
  if (loading) {
    body = (
      <div className="space-y-2" aria-hidden>
        <Skeleton className="h-[62px] w-full rounded-lg" />
        <Skeleton className="h-[62px] w-full rounded-lg" />
      </div>
    )
  } else if (methods.length === 0) {
    body = <p className="text-sm text-muted-foreground">{t('checkout.notAvailable')}</p>
  } else if (initialSource === 'last' && initialId) {
    body = (
      <LastUsedPicker
        methods={methods}
        method={method}
        lastId={initialId}
        onSelect={select}
      />
    )
  } else if (initialSource === 'default' && initialId && methods.length > 1) {
    body = <RecommendedPicker methods={methods} method={method} recommendedId={initialId} onSelect={select} />
  } else {
    body = <MethodList methods={methods} method={method} onSelect={select} />
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base font-medium text-muted-foreground">{t('checkout.chooseProvider')}</CardTitle>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}

/** Полный список способов оплаты. */
function MethodList({
  methods,
  method,
  onSelect,
}: {
  methods: CheckoutMethodId[]
  method: CheckoutMethodId | null
  onSelect: (id: CheckoutMethodId) => void
}) {
  const { t } = useTranslation()

  return (
    <div className="space-y-2">
      {methods.map((id) => {
        const meta = checkoutMethodMeta(id, t)
        return (
          <ProviderMethodButton
            key={id}
            selected={method === id}
            onClick={() => onSelect(id)}
            icon={meta.icon}
            label={meta.label}
            description={meta.provider}
          />
        )
      })}
    </div>
  )
}

/** Прошлый способ одной строкой; «Изменить» раскрывает полный список. */
function LastUsedPicker({
  methods,
  method,
  lastId,
  onSelect,
}: {
  methods: CheckoutMethodId[]
  method: CheckoutMethodId | null
  lastId: CheckoutMethodId
  onSelect: (id: CheckoutMethodId) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const shown = method ?? lastId

  if (open || methods.length === 1) {
    return (
      <MethodList
        methods={methods}
        method={method}
        onSelect={(id) => {
          onSelect(id)
          setOpen(false)
        }}
      />
    )
  }

  const meta = checkoutMethodMeta(shown, t)
  return (
    <button
      type="button"
      onClick={() => setOpen(true)}
      aria-expanded={false}
      className="flex w-full items-center gap-3 rounded-lg border border-primary bg-primary/10 p-3 text-left shadow-sm transition-colors hover:bg-primary/[0.14] dark:bg-primary/[0.18]"
    >
      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-muted dark:bg-white/[0.07]">
        {meta.icon}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{meta.label}</p>
        <p className="text-xs text-muted-foreground">{meta.provider}</p>
      </div>
      <span className="shrink-0 rounded-md bg-primary/15 px-2.5 py-1.5 text-xs font-medium text-primary">
        {t('checkout.changeMethod')}
      </span>
    </button>
  )
}

/** Способ по умолчанию крупно, остальные — плитками в две колонки. */
function RecommendedPicker({
  methods,
  method,
  recommendedId,
  onSelect,
}: {
  methods: CheckoutMethodId[]
  method: CheckoutMethodId | null
  recommendedId: CheckoutMethodId
  onSelect: (id: CheckoutMethodId) => void
}) {
  const { t } = useTranslation()
  const meta = checkoutMethodMeta(recommendedId, t)
  const others = methods.filter((id) => id !== recommendedId)

  return (
    <div className="space-y-3">
      <ProviderMethodButton
        selected={method === recommendedId}
        onClick={() => onSelect(recommendedId)}
        icon={meta.icon}
        label={meta.label}
        description={meta.provider}
        badge={<RecommendedBadge>{t('checkout.recommendedBadge')}</RecommendedBadge>}
      />
      <div className="space-y-2">
        <p className="text-xs text-muted-foreground">{t('checkout.otherMethods')}</p>
        <div className="grid grid-cols-2 gap-2">
          {others.map((id) => (
            <MethodTile key={id} id={id} selected={method === id} onClick={() => onSelect(id)} />
          ))}
        </div>
      </div>
    </div>
  )
}

function MethodTile({ id, selected, onClick }: { id: CheckoutMethodId; selected: boolean; onClick: () => void }) {
  const { t } = useTranslation()
  const meta = checkoutMethodMeta(id, t)
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      className={cn(
        'relative flex min-w-0 items-center gap-2 rounded-lg border p-2 text-left transition-all duration-150',
        selected
          ? 'border-primary bg-primary/10 shadow-sm dark:bg-primary/[0.18]'
          : 'border-border bg-foreground/[0.03] hover:border-primary/40 hover:bg-foreground/[0.06] dark:bg-black/20 dark:hover:bg-black/30',
      )}
    >
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-muted dark:bg-white/[0.07]">
        {meta.icon}
      </div>
      {/* Две строки вместо многоточия: «Другая крипта» в узкую плитку на телефоне не влезает. */}
      <div className="min-w-0 flex-1">
        <p className="line-clamp-2 text-[13px] font-medium leading-tight">{meta.label}</p>
        <p className="truncate text-[11px] text-muted-foreground">{meta.provider}</p>
      </div>
      {selected && (
        <span className="absolute right-1.5 top-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-primary">
          <Check size={10} className="text-white" />
        </span>
      )}
    </button>
  )
}

function RecommendedBadge({ children }: { children: string }) {
  return (
    <span className="shrink-0 whitespace-nowrap rounded-md bg-emerald-500/15 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-400">
      {children}
    </span>
  )
}
