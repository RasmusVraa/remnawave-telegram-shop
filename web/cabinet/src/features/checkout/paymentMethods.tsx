import { useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Bitcoin, CreditCard, Globe, Layers, Smartphone, Star, Wallet } from 'lucide-react'

import { HeleketBrandIcon, SbpBrandIcon } from '@/components/BrandIcons'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { api } from '@/lib/api'
import { PROVIDER_ORDER, useProviderEnabled, type Provider } from '@/features/checkout/checkoutShared'

/**
 * Способ оплаты на странице оформления — кнопка, а не провайдер: у ЮKassa
 * кнопок может быть несколько (YOOKASA_PAYMENT_METHODS), по одной на способ.
 *
 * id: «yookassa:<способ>» для ЮKassa, у остальных — сам провайдер
 * («platega_sbp», «telegram»…). Тот же формат у CABINET_CHECKOUT_DEFAULT_METHOD
 * и у GET /payments/last-method (internal/cabinet/payments/last_method.go).
 */
export type CheckoutMethodId = string

/** Способы ЮKassa — синхрон с config.YookasaPaymentMethodIDs. */
export const YOOKASSA_METHOD_IDS = ['all', 'sbp', 'bank_card', 'sberbank', 'tinkoff_bank', 'yoo_money'] as const
export type YookassaMethodId = (typeof YOOKASSA_METHOD_IDS)[number]

/** Способы Platega — синхрон с config.PlategaPaymentMethodIDs; «all» — общая форма Platega. */
export const PLATEGA_METHOD_IDS = ['all', 'sbp', 'acquiring', 'worldwide', 'crypto'] as const
export type PlategaMethodId = (typeof PLATEGA_METHOD_IDS)[number]

export function isPlategaMethodId(v: string): v is PlategaMethodId {
  return (PLATEGA_METHOD_IDS as readonly string[]).includes(v)
}

/** Кнопки Platega из bootstrap; без поля (старый бэкенд) — одна общая форма. */
export function plategaMethodsFrom(raw: string[] | undefined): PlategaMethodId[] {
  const list = (raw ?? []).filter(isPlategaMethodId)
  return list.length > 0 ? list : ['all']
}

/** id кнопки кабинета для способа Platega: «platega» или «platega_sbp»… */
export function plategaCheckoutId(id: PlategaMethodId): CheckoutMethodId {
  return id === 'all' ? 'platega' : `platega_${id}`
}

const YOOKASSA_PREFIX = 'yookassa:'

export function isYookassaMethodId(v: string): v is YookassaMethodId {
  return (YOOKASSA_METHOD_IDS as readonly string[]).includes(v)
}

export function checkoutMethodProvider(id: CheckoutMethodId): Provider {
  return (id.startsWith(YOOKASSA_PREFIX) ? 'yookassa' : id) as Provider
}

/** Способ ЮKassa для тела запроса; «all» и не-ЮKassa — undefined. */
export function checkoutMethodYookassa(id: CheckoutMethodId): string | undefined {
  if (!id.startsWith(YOOKASSA_PREFIX)) return undefined
  const m = id.slice(YOOKASSA_PREFIX.length)
  return m === 'all' ? undefined : m
}

/** Кнопки ЮKassa из bootstrap; без поля (старый бэкенд) — одна «все способы». */
export function yookassaMethodsFrom(raw: string[] | undefined): YookassaMethodId[] {
  const list = (raw ?? []).filter(isYookassaMethodId)
  return list.length > 0 ? list : ['all']
}

/** Включённые сейчас кнопки в порядке показа. */
export function availableCheckoutMethods(
  providerEnabled: Record<Provider, boolean>,
  yookassaMethods: YookassaMethodId[],
  plategaMethods: PlategaMethodId[] = ['all'],
): CheckoutMethodId[] {
  const out: CheckoutMethodId[] = []
  for (const p of PROVIDER_ORDER) {
    if (p === 'yookassa') {
      if (providerEnabled.yookassa) out.push(...yookassaMethods.map((m) => YOOKASSA_PREFIX + m))
    } else if (p.startsWith('platega')) {
      // Кнопки Platega идут одним куском, в порядке настройки.
      if (p !== 'platega') continue
      for (const m of plategaMethods) {
        const id = plategaCheckoutId(m)
        if (providerEnabled[id as Provider]) out.push(id)
      }
    } else if (providerEnabled[p]) {
      out.push(p)
    }
  }
  return out
}

/**
 * Прошлый способ, если он ещё доступен. Если ровно его кнопки нет, но касса
 * та же (например, платил через форму ЮKassa, а теперь у неё кнопки СБП и
 * карты), — первая кнопка этой кассы.
 */
export function matchAvailableMethod(
  id: CheckoutMethodId | undefined | null,
  available: CheckoutMethodId[],
): CheckoutMethodId | null {
  if (!id) return null
  if (available.includes(id)) return id
  if (id.startsWith(YOOKASSA_PREFIX)) {
    return available.find((m) => m.startsWith(YOOKASSA_PREFIX)) ?? null
  }
  return null
}

export interface CheckoutMethodMeta {
  label: string
  /** Касса под названием — как в прежнем списке. */
  provider: string
  icon: ReactNode
}

export function checkoutMethodMeta(id: CheckoutMethodId, t: (k: string) => string): CheckoutMethodMeta {
  if (id.startsWith(YOOKASSA_PREFIX)) {
    const m = id.slice(YOOKASSA_PREFIX.length)
    const provider = t('checkout.providerHintYookassa')
    switch (m) {
      case 'sbp':
        return { label: t('checkout.yookassaMethod.sbp'), provider, icon: <SbpBrandIcon className="h-[18px] w-[18px]" /> }
      case 'bank_card':
        return { label: t('checkout.yookassaMethod.bank_card'), provider, icon: <CreditCard size={18} className="text-primary" /> }
      case 'sberbank':
        return { label: t('checkout.yookassaMethod.sberbank'), provider, icon: <Smartphone size={18} className="text-emerald-500" /> }
      case 'tinkoff_bank':
        return { label: t('checkout.yookassaMethod.tinkoff_bank'), provider, icon: <Smartphone size={18} className="text-yellow-500" /> }
      case 'yoo_money':
        return { label: t('checkout.yookassaMethod.yoo_money'), provider, icon: <Wallet size={18} className="text-violet-500" /> }
      default:
        return { label: t('checkout.card'), provider, icon: <CreditCard size={18} className="text-primary" /> }
    }
  }
  switch (id) {
    case 'cryptopay':
      return { label: t('checkout.crypto'), provider: t('checkout.providerHintCryptopay'), icon: <Bitcoin size={18} className="text-orange-500" /> }
    case 'telegram':
      return { label: t('checkout.telegramStars'), provider: t('checkout.providerHintTelegram'), icon: <Star size={18} className="text-amber-500" /> }
    case 'heleket':
      return { label: t('checkout.otherCrypto'), provider: t('checkout.providerHintHeleket'), icon: <HeleketBrandIcon className="h-[18px] w-[18px]" /> }
    case 'platega':
      return { label: t('checkout.plategaAll'), provider: t('checkout.providerHintPlatega'), icon: <Layers size={18} className="text-primary" /> }
    case 'platega_sbp':
      return { label: t('checkout.plategaSbp'), provider: t('checkout.providerHintPlatega'), icon: <SbpBrandIcon className="h-[18px] w-[18px]" /> }
    case 'platega_acquiring':
      return { label: t('checkout.plategaAcquiring'), provider: t('checkout.providerHintPlatega'), icon: <CreditCard size={18} className="text-violet-500" /> }
    case 'platega_worldwide':
      return { label: t('checkout.plategaWorldwide'), provider: t('checkout.providerHintPlatega'), icon: <Globe size={18} className="text-indigo-500" /> }
    case 'platega_crypto':
      return { label: t('checkout.plategaCrypto'), provider: t('checkout.providerHintPlatega'), icon: <Bitcoin size={18} className="text-primary" /> }
    default:
      return { label: id, provider: '', icon: <CreditCard size={18} className="text-primary" /> }
  }
}

/** Откуда взят способ, выбранный при открытии страницы. */
export type InitialMethodSource = 'last' | 'default' | null

/**
 * Выбор способа на странице оформления.
 *
 * Сначала подставляется способ прошлой оплаты, для тех, кто ещё не платил, —
 * способ по умолчанию от админа (CABINET_CHECKOUT_DEFAULT_METHOD). Если нет ни
 * того ни другого — ничего не выбрано, как было раньше.
 */
export function useCheckoutMethod() {
  const providerEnabled = useProviderEnabled()
  const { data: bootstrap } = useAuthBootstrap()
  const yookassaMethods = yookassaMethodsFrom(bootstrap?.yookassa_methods)
  const plategaMethods = plategaMethodsFrom(bootstrap?.platega_methods)
  const methods = useMemo(
    () => availableCheckoutMethods(providerEnabled, yookassaMethods, plategaMethods),
    // providerEnabled — новый объект на каждый рендер; сравниваем по значениям.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [JSON.stringify(providerEnabled), yookassaMethods.join(','), plategaMethods.join(',')],
  )

  const lastQuery = useQuery({
    queryKey: ['paymentLastMethod'],
    queryFn: () => api.paymentLastMethod(),
    staleTime: 60_000,
    retry: false,
  })

  const [picked, setPicked] = useState<CheckoutMethodId | null>(null)

  const initial = useMemo((): { id: CheckoutMethodId | null; source: InitialMethodSource } => {
    const last = matchAvailableMethod(lastQuery.data?.method, methods)
    if (last) return { id: last, source: 'last' }
    const def = matchAvailableMethod(bootstrap?.checkout_default_method, methods)
    if (def) return { id: def, source: 'default' }
    return { id: null, source: null }
  }, [lastQuery.data?.method, bootstrap?.checkout_default_method, methods])

  const candidate = picked ?? initial.id
  const method = candidate && methods.includes(candidate) ? candidate : null

  return {
    methods,
    method,
    setMethod: setPicked,
    provider: method ? checkoutMethodProvider(method) : null,
    yookassaMethod: method ? checkoutMethodYookassa(method) : undefined,
    initial,
    /** Пока не знаем прошлый способ — не рисуем список, чтобы он не прыгал. */
    loading: lastQuery.isLoading || !bootstrap,
    providerEnabled,
  }
}
