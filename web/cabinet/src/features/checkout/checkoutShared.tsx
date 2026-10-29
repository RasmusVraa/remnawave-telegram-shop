import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Bitcoin, CreditCard, Star } from 'lucide-react'

import { HeleketBrandIcon } from '@/components/BrandIcons'
import { LegalContinueDisclaimer } from '@/components/LegalContinueDisclaimer'
import { PlategaPaymentExpand, type PlategaMethodId } from '@/components/PlategaPaymentExpand'
import { ProviderMethodButton } from '@/components/ProviderMethodButton'
import { RevealItem } from '@/components/PageReveal'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
  | 'platega_sbp'
  | 'platega_cards'
  | 'platega_acquiring'
  | 'platega_worldwide'
  | 'platega_crypto'
  | 'heleket'

export const PROVIDER_ORDER: Provider[] = [
  'yookassa',
  'platega_sbp',
  'platega_cards',
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
    platega_sbp: bootstrap?.payment_providers?.platega_sbp ?? false,
    platega_cards: bootstrap?.payment_providers?.platega_cards ?? false,
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

export function ProviderPickerCard({
  providerEnabled,
  provider,
  selectedProvider,
  onSelect,
}: {
  providerEnabled: Record<Provider, boolean>
  /** Выбор как есть — Platega разворачивается по нему, даже если способ выключен. */
  provider: Provider | null
  /** Выбор с учётом включённых способов — по нему подсвечиваются кнопки. */
  selectedProvider: Provider | null
  onSelect: (p: Provider) => void
}) {
  const { t } = useTranslation()
  const availableProviders = PROVIDER_ORDER.filter((p) => providerEnabled[p])

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base font-medium text-muted-foreground">
          {t('checkout.chooseProvider')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        {availableProviders.includes('yookassa') && (
          <ProviderMethodButton
            selected={selectedProvider === 'yookassa'}
            onClick={() => onSelect('yookassa')}
            icon={<CreditCard size={18} className="text-primary" />}
            label={t('checkout.card')}
            description="YooKassa"
          />
        )}
        {availableProviders.includes('cryptopay') && (
          <ProviderMethodButton
            selected={selectedProvider === 'cryptopay'}
            onClick={() => onSelect('cryptopay')}
            icon={<Bitcoin size={18} className="text-orange-500" />}
            label={t('checkout.crypto')}
            description="CryptoPay"
          />
        )}
        <PlategaPaymentExpand
          enabled={providerEnabled}
          selected={provider}
          onSelect={(id: PlategaMethodId) => onSelect(id)}
          variant="checkout"
        />
        {availableProviders.includes('telegram') && (
          <ProviderMethodButton
            selected={selectedProvider === 'telegram'}
            onClick={() => onSelect('telegram')}
            icon={<Star size={18} className="text-amber-500" />}
            label={t('checkout.telegramStars')}
            description="Telegram Stars"
          />
        )}
        {availableProviders.includes('heleket') && (
          <ProviderMethodButton
            selected={selectedProvider === 'heleket'}
            onClick={() => onSelect('heleket')}
            icon={<HeleketBrandIcon className="h-[18px] w-[18px]" />}
            label={t('checkout.otherCrypto')}
            description="Heleket"
          />
        )}
        {availableProviders.length === 0 && (
          <p className="text-sm text-muted-foreground">{t('checkout.notAvailable')}</p>
        )}
      </CardContent>
    </Card>
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
