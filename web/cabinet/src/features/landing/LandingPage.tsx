import { useLayoutEffect } from 'react'

import './landing.css'
import { useLandingBrand } from './useLandingBrand'
import { useLandingCopy } from './useLandingCopy'
import { LandingHolidayFrame } from './components/LandingHolidayFrame'
import { LandingHeader } from './components/LandingHeader'
import { LandingHero } from './components/LandingHero'
import { LandingPath } from './components/LandingPath'
import { LandingTariffs } from './components/LandingTariffs'
import { LandingSteps } from './components/LandingSteps'
import { LandingFaq } from './components/LandingFaq'
import { LandingFooter } from './components/LandingFooter'

/**
 * Публичный лендинг на корне домена.
 * Первый экран — заголовок и орбита. Дальше путь трафика, тарифы, подключение и вопросы.
 */
export default function LandingPage() {
  useLayoutEffect(() => {
    const root = document.documentElement
    root.dataset.landing = '1'
    return () => {
      delete root.dataset.landing
    }
  }, [])

  const brand = useLandingBrand()
  const copy = useLandingCopy()

  return (
    <LandingHolidayFrame>
      <LandingHeader brand={brand} copy={copy} />
      <main>
        <LandingHero brand={brand} copy={copy} />
        {copy.showFeatures && <LandingPath />}
        {copy.showTariffs && <LandingTariffs brand={brand} />}
        {copy.showSteps && <LandingSteps brand={brand} />}
        {copy.showFaq && <LandingFaq />}
      </main>
      <LandingFooter brand={brand} />
    </LandingHolidayFrame>
  )
}
