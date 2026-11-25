import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bitcoin, ChevronDown, CreditCard, Wallet } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { ProviderMethodButton } from '@/components/ProviderMethodButton'

export const PLATEGA_METHOD_ORDER = [
  'platega_sbp',
  'platega_cards',
  'platega_acquiring',
  'platega_worldwide',
  'platega_crypto',
] as const

export type PlategaMethodId = (typeof PLATEGA_METHOD_ORDER)[number]

export function enabledPlategaMethods(enabled: Record<string, boolean | undefined>): PlategaMethodId[] {
  return PLATEGA_METHOD_ORDER.filter((p) => Boolean(enabled[p]))
}

function plategaParentTitle(t: (k: string) => string, methods: PlategaMethodId[]): string {
  const hasSbp = methods.includes('platega_sbp')
  const hasCard = methods.some(
    (p) => p === 'platega_cards' || p === 'platega_acquiring' || p === 'platega_worldwide',
  )
  const hasCrypto = methods.includes('platega_crypto')
  const parts: string[] = []
  if (hasSbp) parts.push(t('checkout.plategaSbp'))
  if (hasCard) parts.push(t('checkout.plategaCards'))
  if (hasCrypto) parts.push(t('checkout.plategaCrypto'))
  return parts.join(' / ')
}

function plategaMethodLabel(t: (k: string) => string, id: PlategaMethodId): string {
  switch (id) {
    case 'platega_sbp':
      return t('checkout.plategaSbp')
    case 'platega_cards':
      return t('checkout.plategaCards')
    case 'platega_acquiring':
      return t('checkout.plategaAcquiring')
    case 'platega_worldwide':
      return t('checkout.plategaWorldwide')
    case 'platega_crypto':
      return t('checkout.plategaCrypto')
  }
}

function plategaMethodIcon(id: PlategaMethodId) {
  switch (id) {
    case 'platega_sbp':
      return <Wallet size={18} className="text-primary" />
    case 'platega_cards':
      return <CreditCard size={18} className="text-primary" />
    case 'platega_acquiring':
      return <CreditCard size={18} className="text-violet-500" />
    case 'platega_worldwide':
      return <CreditCard size={18} className="text-indigo-500" />
    case 'platega_crypto':
      return <Bitcoin size={18} className="text-primary" />
  }
}

/** Иконки в ряду с ЮKassa / Stars (окно доп. HWID): цвет как у соседних кнопок (current). */
function plategaMethodIconCompact(id: PlategaMethodId) {
  switch (id) {
    case 'platega_sbp':
      return <Wallet size={14} className="shrink-0" />
    case 'platega_cards':
      return <CreditCard size={14} className="shrink-0" />
    case 'platega_acquiring':
      return <CreditCard size={14} className="shrink-0" />
    case 'platega_worldwide':
      return <CreditCard size={14} className="shrink-0" />
    case 'platega_crypto':
      return <Bitcoin size={14} className="shrink-0" />
  }
}

/** Подпись провайдера под основным текстом (как description на странице чекаута). */
function hwidCompactProviderHintClass(selected: boolean) {
  return cn(
    'text-xs font-normal leading-snug',
    selected ? 'text-primary-foreground/75' : 'text-muted-foreground',
  )
}

type Props = {
  enabled: Record<string, boolean | undefined>
  selected: string | null | undefined
  onSelect: (id: PlategaMethodId) => void
  /** Как на checkout: крупные карточки; как в HWID: компактные кнопки */
  variant?: 'checkout' | 'compact'
}

export function PlategaPaymentExpand({ enabled, selected, onSelect, variant = 'checkout' }: Props) {
  const { t } = useTranslation()
  const methods = useMemo(() => enabledPlategaMethods(enabled), [enabled])
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const selectedPlatega = methods.includes(selected as PlategaMethodId)
  const parentTitle = plategaParentTitle(t, methods)
  const compactDropdown = variant === 'compact' && methods.length > 1

  useEffect(() => {
    if (!open || !compactDropdown) return
    function onDocMouseDown(e: MouseEvent) {
      const el = rootRef.current
      if (el && !el.contains(e.target as Node)) setOpen(false)
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDocMouseDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onDocMouseDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open, compactDropdown])

  if (methods.length === 0) return null

  if (methods.length === 1) {
    const only = methods[0]
    if (variant === 'compact') {
      const sel = selected === only
      return (
        <Button
          type="button"
          size="sm"
          variant={sel ? 'default' : 'outline'}
          onClick={() => onSelect(only)}
          className="flex h-auto min-h-9 items-start gap-2 px-3 py-2 text-left font-normal"
        >
          <span className="shrink-0 pt-0.5">{plategaMethodIconCompact(only)}</span>
          <span className="flex min-w-0 flex-col items-start gap-0.5">
            <span className="text-sm font-medium leading-tight">{plategaMethodLabel(t, only)}</span>
            <span className={hwidCompactProviderHintClass(sel)}>{t('checkout.providerHintPlatega')}</span>
          </span>
        </Button>
      )
    }
    return (
      <ProviderMethodButton
        selected={selected === only}
        onClick={() => onSelect(only)}
        icon={plategaMethodIcon(only)}
        label={plategaMethodLabel(t, only)}
        description="Platega"
      />
    )
  }

  const chevronCheckout = (
    <ChevronDown
      size={18}
      className={cn('text-muted-foreground transition-transform duration-[400ms] ease-out', open && 'rotate-180')}
    />
  )

  if (variant === 'compact') {
    const chevronCompact = (
      <ChevronDown
        size={14}
        className={cn('shrink-0 opacity-70 transition-transform duration-[400ms] ease-out', open && 'rotate-180')}
      />
    )
    return (
      <div ref={rootRef} className="relative z-20 isolate inline-flex max-w-full flex-col items-stretch">
        <Button
          type="button"
          size="sm"
          variant={selectedPlatega ? 'default' : 'outline'}
          className={cn(
            'flex h-auto min-h-9 items-center justify-between gap-2 px-3 py-2 text-left font-normal',
            selectedPlatega && 'border-primary/60',
          )}
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          aria-haspopup="listbox"
          title={parentTitle}
        >
          <span className="inline-flex min-w-0 flex-1 items-center gap-1.5">
            <Wallet size={14} className="shrink-0" />
            <span className="flex min-w-0 flex-col items-start gap-0.5">
              <span className="truncate text-sm font-medium leading-tight">{parentTitle}</span>
              <span className={hwidCompactProviderHintClass(selectedPlatega)}>{t('checkout.providerHintPlatega')}</span>
            </span>
          </span>
          {chevronCompact}
        </Button>
        <div
          role="listbox"
          aria-hidden={!open}
          className={cn(
            'absolute left-0 right-0 top-full z-[80] mt-1.5 rounded-lg border border-border bg-card p-1.5 shadow-xl ring-1 ring-black/5 dark:ring-white/10',
            'origin-top transition-[opacity,transform] duration-[400ms] ease-out',
            open
              ? 'pointer-events-auto translate-y-0 opacity-100'
              : 'pointer-events-none -translate-y-1 opacity-0',
          )}
        >
          <div className="flex flex-col gap-1">
            {methods.map((id) => {
              const sel = selected === id
              return (
                <Button
                  key={id}
                  type="button"
                  size="sm"
                  variant={sel ? 'default' : 'outline'}
                  className="flex h-auto w-full items-start justify-start gap-2 py-2 pl-3 pr-3 text-left font-normal shadow-none"
                  role="option"
                  aria-selected={sel}
                  tabIndex={open ? undefined : -1}
                  onClick={() => {
                    onSelect(id)
                    setOpen(false)
                  }}
                >
                  <span className="shrink-0 pt-0.5">{plategaMethodIconCompact(id)}</span>
                  <span className="flex min-w-0 flex-col items-start gap-0.5">
                    <span className="text-sm font-medium leading-tight">{plategaMethodLabel(t, id)}</span>
                    <span className={hwidCompactProviderHintClass(sel)}>{t('checkout.providerHintPlatega')}</span>
                  </span>
                </Button>
              )
            })}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <ProviderMethodButton
        selected={selectedPlatega}
        onClick={() => setOpen((v) => !v)}
        icon={<Wallet size={18} className="text-primary" />}
        label={parentTitle}
        description="Platega"
        trailing={chevronCheckout}
      />
      <div
        className={cn(
          'grid overflow-hidden transition-[grid-template-rows,opacity,transform] duration-[400ms] ease-out',
          open
            ? 'visible grid-rows-[1fr] opacity-100 translate-y-0'
            : 'invisible grid-rows-[0fr] opacity-0 -translate-y-1 pointer-events-none',
        )}
        aria-hidden={!open}
      >
        <div className="min-h-0 space-y-2 overflow-hidden">
          {methods.map((id) => (
            <ProviderMethodButton
              key={id}
              selected={selected === id}
              onClick={() => {
                onSelect(id)
                setOpen(true)
              }}
              icon={plategaMethodIcon(id)}
              label={plategaMethodLabel(t, id)}
              description="Platega"
            />
          ))}
        </div>
      </div>
    </div>
  )
}
