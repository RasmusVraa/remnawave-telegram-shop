import { createPortal } from 'react-dom'

import { LegalContinueDisclaimer } from '@/components/LegalContinueDisclaimer'
import { Button } from '@/components/ui/button'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { cn } from '@/lib/utils'

/**
 * Общие части страниц оформления: оплата подписки (CheckoutPage), докупка мест
 * для устройств (DeviceSlotsCheckoutPage) и трафика (TrafficCheckoutPage).
 * Сводка заказа и раскладка страницы — в orderSummary.tsx.
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

const disabledPayButtonClass =
  'disabled:opacity-100 disabled:bg-[hsl(174_31.2%_76.18%)] disabled:text-[hsl(0_0%_100%_/_57%)] disabled:brightness-100 dark:disabled:bg-primary dark:disabled:text-primary-foreground dark:disabled:brightness-[.45]'

export type PayButtonProps = {
  label: string
  disabled: boolean
  loading: boolean
  onPay: () => void
}

/** Кнопка «Оплатить» в потоке страницы (sm+) со строкой про соглашение под ней. */
export function PayButtonInline({ label, disabled, loading, onPay }: PayButtonProps) {
  const { data: bootstrap } = useAuthBootstrap()
  return (
    <div className="space-y-2">
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
    </div>
  )
}

/**
 * Кнопка «Оплатить» на телефоне — плашкой над нижним меню. Ставится последней
 * внутри PageReveal: заодно добавляет отступ, чтобы плашка не закрывала контент.
 * Отдельно от PayButtonInline, потому что та на ПК живёт внутри скрытой на
 * телефоне карточки, а распорка и плашка должны остаться видимыми.
 */
export function PayButtonMobileBar({ label, disabled, loading, onPay }: PayButtonProps) {
  const { data: bootstrap } = useAuthBootstrap()

  return (
    <>
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
