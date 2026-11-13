import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'

import { LegalContinueDisclaimer } from '@/components/LegalContinueDisclaimer'
import { RevealItem } from '@/components/PageReveal'
import { Button } from '@/components/ui/button'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { cn } from '@/lib/utils'

/**
 * Общие части страниц оформления: оплата подписки (CheckoutPage) и докупка
 * мест для устройств (DeviceSlotsCheckoutPage) выглядят одинаково и отличаются
 * только сводкой.
 */

export type Provider =
  | 'yookassa'
  | 'cryptopay'
  | 'telegram'
  | 'platega'
  | 'platega_sbp'
  | 'platega_acquiring'
  | 'platega_worldwide'
  | 'platega_crypto'
  | 'heleket'

export const PROVIDER_ORDER: Provider[] = [
  'yookassa',
  'platega',
  'platega_sbp',
  'platega_acquiring',
  'platega_worldwide',
  'platega_crypto',
  'cryptopay',
  'telegram',
  'heleket',
]

export function useProviderEnabled(): Record<Provider, boolean> {
  const { data: bootstrap } = useAuthBootstrap()
  return {
    yookassa: bootstrap?.payment_providers?.yookassa ?? true,
    cryptopay: bootstrap?.payment_providers?.cryptopay ?? true,
    telegram: bootstrap?.payment_providers?.telegram ?? false,
    platega: bootstrap?.payment_providers?.platega ?? false,
    platega_sbp: bootstrap?.payment_providers?.platega_sbp ?? false,
    platega_acquiring: bootstrap?.payment_providers?.platega_acquiring ?? false,
    platega_worldwide: bootstrap?.payment_providers?.platega_worldwide ?? false,
    platega_crypto: bootstrap?.payment_providers?.platega_crypto ?? false,
    heleket: bootstrap?.payment_providers?.heleket ?? false,
  }
}

export function SummaryRow({
  label,
  value,
}: {
  label: string
  value: ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium text-right">{value}</span>
    </div>
  )
}

const disabledPayButtonClass =
  'disabled:opacity-100 disabled:bg-[hsl(174_31.2%_76.18%)] disabled:text-[hsl(0_0%_100%_/_57%)] disabled:brightness-100 dark:disabled:bg-primary dark:disabled:text-primary-foreground dark:disabled:brightness-[.45]'

/**
 * Кнопка «Оплатить»: на sm+ — в потоке страницы, на телефоне — плашкой над
 * нижним меню. Ставится последней внутри PageReveal: заодно добавляет отступ,
 * чтобы плашка не закрывала контент.
 */
export function CheckoutPayButton({
  label,
  disabled,
  loading,
  onPay,
}: {
  label: string
  disabled: boolean
  loading: boolean
  onPay: () => void
}) {
  const { data: bootstrap } = useAuthBootstrap()

  return (
    <>
      <RevealItem className="hidden sm:block space-y-2">
        <Button
          className={cn('w-full', !loading && disabledPayButtonClass)}
          size="lg"
          disabled={disabled}
          loading={loading}
          onClick={onPay}
        >
          {label}
        </Button>
        <LegalContinueDisclaimer siteLinks={bootstrap?.site_links} />
      </RevealItem>

      {/* Mobile spacer: fixed pay bar + legal line should not overlap content */}
      <div className="sm:hidden h-[5.5rem]" aria-hidden />

      {/*
        Mobile: шторка от кнопки до низа экрана, нижнее меню (z-50) лежит поверх неё.
        Без подложки способы оплаты просвечивали сквозь текст согласия.
        z-[39] — под выпадающим мобильным меню (z-40).
      */}
      {typeof document !== 'undefined' &&
        createPortal(
          <div className="cabinet-pay-scrim sm:hidden fixed inset-x-0 bottom-0 z-[39] px-3 pt-8 pb-[calc(var(--cabinet-bottom-nav-h,calc(73px+var(--cabinet-tg-safe-bottom)))+0.375rem)]">
            <div className="mx-auto flex w-full max-w-lg flex-col gap-1.5">
              <Button
                className={cn('w-full shadow-none', !loading && disabledPayButtonClass)}
                size="lg"
                disabled={disabled}
                loading={loading}
                onClick={onPay}
              >
                {label}
              </Button>
              <LegalContinueDisclaimer siteLinks={bootstrap?.site_links} variant="short" />
            </div>
          </div>,
          document.body,
        )}
    </>
  )
}
