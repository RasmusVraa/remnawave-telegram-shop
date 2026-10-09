import { useTranslation } from 'react-i18next'

const STEPS = ['device', 'provider', 'server', 'site'] as const

/** Путь трафика: заголовок слева, четыре точки на линии и плашка со ссылкой на тарифы. */
export function LandingPath() {
  const { t } = useTranslation()

  return (
    <section id="how" className="px-4 py-12 sm:px-6 sm:py-16">
      <div className="mx-auto max-w-6xl">
        <div className="grid items-start gap-6 lg:grid-cols-2 lg:gap-16">
          <h2 className="font-heading text-4xl font-extrabold leading-[1.05] tracking-tight sm:text-5xl">
            {t('landing.path.title')}
          </h2>
          <p className="text-pretty text-base leading-relaxed text-muted-foreground sm:text-lg">
            {t('landing.path.text')}
          </p>
        </div>

        <ol className="landing-timeline">
          {STEPS.map((id) => (
            <li key={id} className="landing-timeline__item">
              <span className="landing-timeline__dot" aria-hidden />
              <h3 className="font-heading text-base font-bold tracking-tight">
                {t(`landing.path.steps.${id}.title`)}
              </h3>
              <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">
                {t(`landing.path.steps.${id}.text`)}
              </p>
            </li>
          ))}
        </ol>
      </div>
    </section>
  )
}
