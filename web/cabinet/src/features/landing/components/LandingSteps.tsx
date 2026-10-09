import { useTranslation } from 'react-i18next'

import { LANDING_STEPS } from '../landingContent'
import type { LandingBrand } from '../useLandingBrand'

/**
 * Три шага подключения в одну линию: номер, заголовок, короткая строка.
 * Без отдельных карточек — секция остаётся плотной.
 */
export function LandingSteps({ brand }: { brand: LandingBrand }) {
  const { t } = useTranslation()
  const label = brand.authenticated ? t('landing.nav.cabinet') : t('landing.steps.ctaCabinet')

  return (
    <section id="connect" className="px-4 py-12 sm:px-6 sm:py-16">
      <div className="mx-auto max-w-6xl">
        <div className="grid items-end gap-4 lg:grid-cols-[minmax(0,1fr)_auto]">
          <div>
            <h2 className="font-heading text-4xl font-extrabold leading-[1.05] tracking-tight sm:text-5xl">
              {t('landing.steps.title')}
            </h2>
            <p className="mt-3 max-w-xl text-pretty text-base leading-relaxed text-muted-foreground">
              {t('landing.steps.subtitle')}
            </p>
          </div>
          <a
            href={brand.cabinetHref}
            className="landing-cta landing-cta--solid inline-flex h-11 items-center justify-center rounded-full px-6 text-sm font-semibold"
          >
            {label}
          </a>
        </div>

        <ol className="mt-8 grid gap-6 sm:grid-cols-3">
          {LANDING_STEPS.map((step, i) => (
            <li key={step.id} className="border-t border-border/80 pt-4">
              <span className="font-heading text-sm font-bold text-[hsl(var(--lp-cyan))]">
                {String(i + 1).padStart(2, '0')}
              </span>
              <h3 className="mt-2 font-heading text-xl font-bold tracking-tight">
                {t(`landing.steps.items.${step.id}.title`)}
              </h3>
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                {t(`landing.steps.items.${step.id}.text`)}
              </p>
            </li>
          ))}
        </ol>
      </div>
    </section>
  )
}
