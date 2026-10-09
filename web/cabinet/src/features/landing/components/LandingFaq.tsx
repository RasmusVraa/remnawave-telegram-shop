import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AnimatePresence, motion } from 'framer-motion'
import { ChevronDown } from 'lucide-react'

import { LANDING_FAQ_IDS } from '../landingContent'

const EASE = [0.22, 1, 0.36, 1] as const

/**
 * FAQ-аккордеон. Одновременно раскрыт максимум один пункт: так секция
 * не «расползается» и остаётся читаемой на мобильных.
 */
export function LandingFaq() {
  const { t } = useTranslation()
  const [openId, setOpenId] = useState<string | null>(null)
  const baseId = useId()

  return (
    <section id="faq" className="px-4 pb-16 pt-4 sm:px-6 sm:pb-20">
      <div className="mx-auto max-w-3xl">
        <h2 className="font-heading text-4xl font-extrabold tracking-tight sm:text-5xl">
          {t('landing.faq.title')}
        </h2>
        <p className="mt-3 max-w-xl text-base text-muted-foreground">{t('landing.faq.subtitle')}</p>

        <div className="landing-faq-list mt-8">
          {LANDING_FAQ_IDS.map((id) => {
            const open = openId === id
            const panelId = `${baseId}-${id}`
            return (
              <div key={id}>
                <div className="landing-faq-item" data-open={open}>
                  <button
                    type="button"
                    onClick={() => setOpenId(open ? null : id)}
                    aria-expanded={open}
                    aria-controls={panelId}
                    className="flex w-full items-center justify-between gap-4 rounded-[inherit] px-5 py-4 text-left outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--lp-cyan))] sm:px-6 sm:py-5"
                  >
                    <span className="font-heading text-base font-bold tracking-tight sm:text-lg">
                      {t(`landing.faq.items.${id}.q`)}
                    </span>
                    <ChevronDown className="landing-faq-chevron size-5 shrink-0 text-muted-foreground" />
                  </button>

                  <AnimatePresence initial={false}>
                    {open && (
                      <motion.div
                        id={panelId}
                        key="panel"
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: 'auto', opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        transition={{ duration: 0.34, ease: EASE }}
                        className="overflow-hidden"
                      >
                        <p className="text-pretty px-5 pb-5 text-sm leading-relaxed text-muted-foreground sm:px-6 sm:pb-6 sm:text-[0.95rem]">
                          {t(`landing.faq.items.${id}.a`)}
                        </p>
                      </motion.div>
                    )}
                  </AnimatePresence>
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}
