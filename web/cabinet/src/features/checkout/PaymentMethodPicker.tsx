import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check } from 'lucide-react'

import { ProviderMethodButton } from '@/components/ProviderMethodButton'
import { Card } from '@/components/ui/card'
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
 *   раскрывается по нажатию, «Свернуть» в заголовке карточки закрывает его;
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
  // Раскрыт ли список у «прошлого способа». Состояние здесь, а не в LastUsedPicker:
  // ссылка «Свернуть» стоит в заголовке карточки.
  const [open, setOpen] = useState(false)
  const cardRef = useRef<HTMLDivElement>(null)
  // Раскрытый по «Изменить» список на телефоне уходит под плашку оплаты —
  // докручиваем страницу, чтобы человеку не пришлось искать его самому.
  useEffect(() => {
    if (!open) return
    const raf = requestAnimationFrame(() => revealCard(cardRef.current))
    return () => cancelAnimationFrame(raf)
  }, [open])
  const select = (id: CheckoutMethodId) => {
    if (id !== method) hapticSelection()
    onSelect(id)
  }
  const lastMode = !loading && methods.length > 1 && initialSource === 'last' && Boolean(initialId)

  let body
  if (loading) {
    body = (
      <div className="space-y-2" aria-hidden>
        <Skeleton className="h-[60px] w-full rounded-[16px]" />
        <Skeleton className="h-[60px] w-full rounded-[16px]" />
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
        open={open}
        onOpen={() => setOpen(true)}
        onSelect={(id) => {
          select(id)
          setOpen(false)
        }}
      />
    )
  } else if (initialSource === 'default' && initialId && methods.length > 1) {
    body = <RecommendedPicker methods={methods} method={method} recommendedId={initialId} onSelect={select} />
  } else {
    body = <MethodList methods={methods} method={method} onSelect={select} />
  }

  return (
    <Card ref={cardRef} className="p-4">
      <div className="mb-2.5 flex items-center justify-between gap-3">
        <h2 className="text-[15px] font-medium text-muted-foreground">{t('checkout.chooseProvider')}</h2>
        {lastMode && open && (
          <button
            type="button"
            onClick={() => setOpen(false)}
            aria-expanded
            className="text-sm font-semibold text-primary hover:underline"
          >
            {t('checkout.collapseMethods')}
          </button>
        )}
      </div>
      {body}
    </Card>
  )
}

/**
 * Прокрутить окно так, чтобы карточка целиком оказалась между шапкой кабинета и
 * закреплённой плашкой оплаты (на телефоне). Если не помещается — к верху, под
 * шапку: список читается сверху. Уже видна — ничего не делаем.
 */
function revealCard(el: HTMLElement | null) {
  if (!el) return
  const gap = 12
  const rect = el.getBoundingClientRect()
  const header = document.querySelector('.cabinet-app-header')?.getBoundingClientRect().bottom ?? 0
  // Плашка оплаты с прозрачным градиентом сверху (pt-8) — считаем от её кнопки.
  const barButton = document.querySelector('.cabinet-pay-scrim button')?.getBoundingClientRect()
  const bottom = barButton && barButton.height > 0 ? barButton.top : window.innerHeight
  const top = header + gap
  if (rect.top >= top && rect.bottom <= bottom - gap) return
  const delta = rect.height <= bottom - gap - top ? rect.bottom - (bottom - gap) : rect.top - top
  const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  window.scrollBy({ top: delta, behavior: reduce ? 'auto' : 'smooth' })
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
    <div className="grid gap-2" role="radiogroup" aria-label={t('checkout.chooseProvider')}>
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

/** Прошлый способ одной строкой; «Изменить» раскрывает полный список, выбор сворачивает его. */
function LastUsedPicker({
  methods,
  method,
  lastId,
  open,
  onOpen,
  onSelect,
}: {
  methods: CheckoutMethodId[]
  method: CheckoutMethodId | null
  lastId: CheckoutMethodId
  open: boolean
  onOpen: () => void
  onSelect: (id: CheckoutMethodId) => void
}) {
  const { t } = useTranslation()
  const shown = method ?? lastId

  if (open || methods.length === 1) {
    return <MethodList methods={methods} method={method} onSelect={onSelect} />
  }

  const meta = checkoutMethodMeta(shown, t)
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-expanded={false}
      className="flex w-full items-center gap-3 rounded-[16px] border border-primary/60 bg-primary/[0.07] px-3 py-2.5 text-left transition-colors hover:bg-primary/[0.11] dark:bg-primary/[0.1] dark:hover:bg-primary/[0.14]"
    >
      <div className="flex h-[38px] w-[38px] shrink-0 items-center justify-center rounded-full bg-muted dark:bg-white/[0.07]">
        {meta.icon}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold">{meta.label}</p>
        <p className="text-[13px] text-muted-foreground">{meta.provider}</p>
      </div>
      <span className="shrink-0 rounded-[12px] bg-primary/[0.12] px-3 py-[7px] text-sm font-semibold text-primary">
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
    <div className="space-y-3" role="radiogroup" aria-label={t('checkout.chooseProvider')}>
      <ProviderMethodButton
        selected={method === recommendedId}
        onClick={() => onSelect(recommendedId)}
        icon={meta.icon}
        label={meta.label}
        description={meta.provider}
        badge={<RecommendedBadge>{t('checkout.recommendedBadge')}</RecommendedBadge>}
      />
      <div className="space-y-2">
        <p className="pt-1 text-xs text-muted-foreground">{t('checkout.otherMethods')}</p>
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
      role="radio"
      aria-checked={selected}
      className={cn(
        'relative flex min-w-0 items-center gap-2 rounded-[14px] border p-2 text-left transition-all duration-150',
        selected
          ? 'border-primary/60 bg-primary/[0.07] dark:bg-primary/[0.1]'
          : 'border-border bg-foreground/[0.03] hover:border-primary/40 hover:bg-foreground/[0.06] dark:bg-black/20 dark:hover:bg-black/30',
      )}
    >
      <div className="flex h-[30px] w-[30px] shrink-0 items-center justify-center rounded-full bg-muted dark:bg-white/[0.07]">
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
