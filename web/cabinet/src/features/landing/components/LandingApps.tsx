import { useTranslation } from 'react-i18next'

import { LandingPlatformRow } from './LandingPlatforms'
import { Reveal } from './LandingMotion'
import { SectionHeading } from './LandingPrimitives'

/** Короткая секция про клиентские приложения — якорь «Приложения» в шапке. */
export function LandingApps() {
  const { t } = useTranslation()

  return (
    <section id="apps" className="px-4 py-10 sm:px-6 sm:py-14">
      <div className="mx-auto max-w-6xl">
        <SectionHeading
          eyebrow={t('landing.apps.eyebrow')}
          title={t('landing.apps.title')}
          description={t('landing.apps.text')}
        />
        <Reveal delay={0.12}>
          <LandingPlatformRow className="mt-8 justify-start" label={t('landing.apps.title')} />
        </Reveal>
      </div>
    </section>
  )
}
