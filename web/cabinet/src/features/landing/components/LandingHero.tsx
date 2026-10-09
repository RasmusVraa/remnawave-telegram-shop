import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Shield } from 'lucide-react'

import { api } from '@/lib/api'
import { CountryFlag } from '@/features/admin/components/CountryFlag'
import { countryCentroid } from '../countryCentroids'
import type { LandingBrand } from '../useLandingBrand'
import type { LandingCopy } from '../useLandingCopy'
import { Rise, WordsReveal } from './LandingMotion'
import { TelegramGlyph } from './LandingPrimitives'

/**
 * Первый экран: крупный заголовок, орбита со щитом и две плашки.
 * Тарифы живут отдельной секцией ниже.
 */
export function LandingHero({
  brand,
  copy,
}: {
  brand: LandingBrand
  copy: LandingCopy
}) {
  const { t } = useTranslation()

  const title = copy.heroTitle || t('landing.hero.headline')
  const subtitle = copy.heroSubtitle || t('landing.hero.subtitle')
  const note = copy.note || t('landing.hero.note')
  const cabinetLabel = brand.authenticated ? t('landing.nav.cabinet') : t('landing.nav.try')

  return (
    <section className="relative px-4 pb-8 pt-10 sm:px-6 sm:pb-14 sm:pt-16">
      <div className="mx-auto grid max-w-6xl items-center gap-10 lg:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)] lg:gap-6">
        <div>
          <h1 className="landing-hero-title font-heading text-balance">
            <WordsReveal segments={title.split(' ').filter(Boolean)} delay={0.05} stagger={0.045} />
          </h1>

          <Rise delay={0.28} y={18}>
            <p className="mt-5 max-w-xl text-pretty text-base leading-relaxed text-muted-foreground sm:text-lg">
              {subtitle}
            </p>
          </Rise>

          <Rise delay={0.4} y={18}>
            <div className="mt-7 flex flex-col gap-3 sm:flex-row">
              <a
                href={brand.cabinetHref}
                className="landing-cta landing-cta--solid inline-flex h-12 items-center justify-center rounded-full px-7 text-base font-semibold sm:h-14"
              >
                {cabinetLabel}
              </a>
              {brand.botUrl && (
                <a
                  href={brand.botUrl}
                  className="landing-cta landing-cta--ghost inline-flex h-12 items-center justify-center gap-2 rounded-full px-6 text-base font-semibold sm:h-14"
                >
                  <TelegramGlyph className="size-4" />
                  {t('landing.hero.ctaTelegram')}
                </a>
              )}
            </div>
            {note ? <p className="mt-4 max-w-md text-sm text-muted-foreground">{note}</p> : null}
          </Rise>
        </div>

        <Rise delay={0.15} y={12}>
          <LandingConstellation logoUrl={brand.logoUrl} name={brand.name} />
        </Rise>
      </div>
    </section>
  )
}

const FLY_LANES = [
  { inset: '9%', dur: 34 },
  { inset: '18%', dur: 46 },
  { inset: '26%', dur: 27 },
]

function LandingConstellation({ logoUrl, name }: { logoUrl?: string; name: string }) {
  const { i18n } = useTranslation()
  const query = useQuery({
    queryKey: ['public-status'],
    queryFn: () => api.publicStatus(),
    staleTime: 60_000,
  })
  const seen = new Set<string>()
  const countries = (query.data?.nodes ?? []).flatMap((node) => {
    const code = (node.country ?? '').toUpperCase()
    if (!countryCentroid(code) || seen.has(code)) return []
    seen.add(code)
    let label = node.name
    try {
      label = new Intl.DisplayNames([i18n.language], { type: 'region' }).of(code) ?? node.name
    } catch {
      label = node.name
    }
    return [{ code, label }]
  })

  return (
    <div className="landing-constellation" aria-hidden>
      <div className="landing-constellation__wash" />
      <div className="landing-constellation__halo" />
      <div className="landing-constellation__orbit landing-constellation__orbit--one" />
      <div className="landing-constellation__orbit landing-constellation__orbit--two" />
      <div className="landing-constellation__orbit landing-constellation__orbit--three" />
      {countries.map((item, index) => {
        const lane = FLY_LANES[index % FLY_LANES.length]
        const reverse = index % 2 === 1
        const delay = `-${(index / Math.max(countries.length, 1)) * lane.dur}s`
        return (
          <span
            key={item.code}
            className="landing-constellation__lane"
            style={{
              inset: lane.inset,
              animationDuration: `${lane.dur}s`,
              animationDelay: delay,
              animationDirection: reverse ? 'reverse' : 'normal',
            }}
          >
            <span
              className="landing-constellation__pin"
              style={{
                animationDuration: `${lane.dur}s`,
                animationDelay: delay,
                animationDirection: reverse ? 'normal' : 'reverse',
              }}
            >
              <span className="landing-constellation__upright" style={{ animationDelay: `${index * 0.4}s` }}>
                <CountryFlag code={item.code} className="h-3.5 w-5 rounded-[2px]" />
                {item.label}
              </span>
            </span>
          </span>
        )
      })}
      <div className="landing-constellation__logo">
        {logoUrl ? (
          <img src={logoUrl} alt="" />
        ) : (
          <Shield className="size-12" strokeWidth={1.6} aria-label={name} />
        )}
      </div>
    </div>
  )
}
