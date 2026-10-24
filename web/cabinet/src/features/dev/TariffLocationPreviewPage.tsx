import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { applyTheme } from '@/hooks/useTheme'
import { TariffLocationPicker } from '@/features/checkout/TariffLocationPicker'
import type { TariffLocationItem } from '@/lib/api'

const PREVIEW_LOCATIONS: TariffLocationItem[] = [
  { id: 1, name: 'Германия', sold_out: false, country_code: 'de' },
  { id: 2, name: 'Нидерланды', sold_out: false, country_code: 'nl' },
  { id: 3, name: 'Финляндия', sold_out: true, country_code: 'fi' },
  { id: 4, name: 'Эстония', sold_out: false, country_code: 'ee' },
]

/** Как клиент выбирает страну при покупке тарифа «свой сервер». Без бота и сессии. */
export default function TariffLocationPreviewPage() {
  const { t } = useTranslation()
  const [locationId, setLocationId] = useState<number | null>(1)
  const selected = PREVIEW_LOCATIONS.find((loc) => loc.id === locationId)

  useEffect(() => {
    applyTheme('dark')
  }, [])

  return (
    <div className="cabinet-shell relative min-h-dvh">
      <div className="cabinet-shell-gradient" aria-hidden />
      <div className="relative z-10 mx-auto max-w-lg space-y-4 px-4 py-8">
        <h1 className="text-2xl font-semibold">{t('checkout.title')}</h1>
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base font-medium text-muted-foreground">{t('checkout.summary')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Row label={t('checkout.plan')} value="Свой сервер" />
            <Row label={t('checkout.period')} value="1 месяц" />
            {selected && <Row label={t('checkout.location')} value={selected.name} />}
            <div className="mt-2 border-t border-border pt-2">
              <Row label={t('checkout.total')} value={<span className="text-lg font-bold">1 990 ₽</span>} />
            </div>
          </CardContent>
        </Card>
        <TariffLocationPicker locations={PREVIEW_LOCATIONS} locationId={locationId} onSelect={setLocationId} />
        <Button className="w-full" size="lg" disabled={locationId == null}>
          {t('checkout.pay')}
        </Button>
      </div>
    </div>
  )
}

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium">{value}</span>
    </div>
  )
}
