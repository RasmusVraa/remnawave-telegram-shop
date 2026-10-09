import { useLayoutEffect } from 'react'

import './landing.css'
import { useLandingBrand } from './useLandingBrand'
import { useLandingCopy } from './useLandingCopy'
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
    <div className="landing-root">
      <div className="landing-backdrop" aria-hidden>
        <div className="landing-backdrop__wash" />
        <div className="landing-orb landing-orb--cyan" />
        <div className="landing-orb landing-orb--violet" />
        <div className="landing-orb landing-orb--emerald" />
        <div className="landing-orb landing-orb--rose" />
      </div>

      <div className="relative z-10">
        <LandingHeader brand={brand} copy={copy} />
        <main>
          <LandingHero brand={brand} copy={copy} />
          {copy.showFeatures && <LandingPath />}
          {copy.showTariffs && <LandingTariffs brand={brand} />}
          {copy.showSteps && <LandingSteps brand={brand} />}
          {copy.showFaq && <LandingFaq />}
        </main>
        <LandingFooter brand={brand} />
      </div>
    </div>
  )
}
