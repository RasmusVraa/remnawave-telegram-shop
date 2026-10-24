import { useTranslation } from 'react-i18next'

import { CountryFlag } from '@/features/admin/components/CountryFlag'
import type { TariffLocationItem } from '@/lib/api'
import { cn } from '@/lib/utils'

/** Выбор страны на оплате тарифа «свой сервер»: крупная карточка, флаг и название снизу. */
export function TariffLocationPicker({
  locations,
  locationId,
  onSelect,
}: {
  locations: TariffLocationItem[]
  locationId: number | null
  onSelect: (id: number) => void
}) {
  const { t } = useTranslation()
  if (locations.length === 0) return null
  const allSoldOut = locations.every((loc) => loc.sold_out)

  return (
    <div className="space-y-3">
      <h2 className="text-base font-medium text-muted-foreground">{t('checkout.location')}</h2>
      {allSoldOut && <p className="text-sm text-muted-foreground">{t('checkout.allLocationsSoldOut')}</p>}
      <div className="grid grid-cols-2 gap-3">
        {locations.map((loc) => {
          const selected = locationId === loc.id
          const code = loc.country_code?.trim().toLowerCase()
          return (
            <button
              key={loc.id}
              type="button"
              disabled={loc.sold_out}
              aria-pressed={selected}
              onClick={() => onSelect(loc.id)}
              className={cn(
                'flex min-h-44 flex-col items-center justify-end rounded-2xl border bg-card px-3 pb-4 pt-5 text-center transition-colors',
                loc.sold_out && 'cursor-not-allowed opacity-50',
                selected ? 'border-primary ring-2 ring-primary/40' : 'border-border hover:bg-accent',
              )}
            >
              {code && /^[a-z]{2}$/.test(code) ? (
                <CountryFlag code={code} className="h-16 w-24 rounded-md shadow-md" />
              ) : (
                <span className="flex h-16 w-24 items-center justify-center rounded-md bg-muted text-2xl" aria-hidden>
                  🌐
                </span>
              )}
              <span className="mt-3 text-base font-semibold leading-tight">{loc.name}</span>
              {loc.sold_out && (
                <span className="mt-1 text-xs text-muted-foreground">{t('checkout.locationSoldOut')}</span>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}
