import { useTranslation } from 'react-i18next'
import { Shield } from 'lucide-react'

import type { LandingBrand } from '../useLandingBrand'
import { Reveal } from './LandingMotion'
import { TelegramGlyph } from './LandingPrimitives'

/**
 * Футер. Внешние ссылки берутся из site_links (env бота) — пункт появляется
 * только если соответствующий URL задан, поэтому у пустой конфигурации футер
 * схлопывается до бренда и копирайта.
 */
export function LandingFooter({ brand }: { brand: LandingBrand }) {
  const { t } = useTranslation()

  const productLinks: Array<{ key: string; href: string }> = [
    brand.channelUrl ? { key: 'channel', href: brand.channelUrl } : null,
    brand.supportUrl ? { key: 'support', href: brand.supportUrl } : null,
    brand.statusUrl ? { key: 'status', href: brand.statusUrl } : null,
  ].filter((v): v is { key: string; href: string } => v !== null)

  const legalLinks: Array<{ key: string; href: string }> = [
    brand.offerUrl ? { key: 'offer', href: brand.offerUrl } : null,
    brand.privacyUrl ? { key: 'privacy', href: brand.privacyUrl } : null,
    brand.tosUrl ? { key: 'tos', href: brand.tosUrl } : null,
  ].filter((v): v is { key: string; href: string } => v !== null)

  return (
    <footer className="landing-footer border-t border-border/60 px-4 py-12 sm:px-6">
      <Reveal y={16}>
        <div className="mx-auto max-w-6xl">
          <div className="flex flex-col gap-10 md:flex-row md:justify-between">
            <div className="max-w-xs">
              <div className="flex items-center gap-2.5">
                {brand.logoUrl ? (
                  <img src={brand.logoUrl} alt="" className="size-9 rounded-full object-contain" />
                ) : (
                  <span className="flex size-9 items-center justify-center rounded-xl bg-[hsl(var(--lp-cyan)/0.12)] text-[hsl(var(--lp-cyan))]">
                    <Shield className="size-5" strokeWidth={1.9} />
                  </span>
                )}
                <span className="font-heading text-lg font-bold tracking-tight">{brand.name}</span>
              </div>
              <p className="mt-4 text-sm leading-relaxed text-muted-foreground">
                {t('landing.footer.tagline')}
              </p>
              {brand.botUrl && (
                <a
                  href={brand.botUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="landing-footer-link mt-4 inline-flex items-center gap-2 text-sm font-semibold"
                >
                  <TelegramGlyph className="size-4" />
                  {t('landing.footer.botLink')}
                </a>
              )}
            </div>

            <div className="grid grid-cols-2 gap-10 sm:grid-cols-3">
              <FooterColumn title={t('landing.footer.sections')}>
                <a href={brand.cabinetHref} className="landing-footer-link text-sm">
                  {brand.authenticated ? t('landing.nav.cabinet') : t('landing.nav.login')}
                </a>
                <a href="/status" className="landing-footer-link text-sm">
                  {t('landing.nav.status')}
                </a>
                <a href="/#tariffs" className="landing-footer-link text-sm">
                  {t('landing.nav.tariffs')}
                </a>
                <a href="/#faq" className="landing-footer-link text-sm">
                  {t('landing.nav.faq')}
                </a>
              </FooterColumn>

              {productLinks.length > 0 && (
                <FooterColumn title={t('landing.footer.product')}>
                  {productLinks.map((link) => (
                    <a
                      key={link.key}
                      href={link.href}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="landing-footer-link text-sm"
                    >
                      {t(`landing.footer.links.${link.key}`)}
                    </a>
                  ))}
                </FooterColumn>
              )}

              {legalLinks.length > 0 && (
                <FooterColumn title={t('landing.footer.legal')}>
                  {legalLinks.map((link) => (
                    <a
                      key={link.key}
                      href={link.href}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="landing-footer-link text-sm"
                    >
                      {t(`landing.footer.links.${link.key}`)}
                    </a>
                  ))}
                </FooterColumn>
              )}
            </div>
          </div>

          <div className="landing-divider mt-10" />
          <p className="mt-6 text-center text-xs text-muted-foreground">
            © {new Date().getFullYear()} {brand.name}
          </p>
        </div>
      </Reveal>
    </footer>
  )
}

function FooterColumn({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="text-xs font-bold uppercase tracking-[0.14em] text-foreground/70">{title}</h3>
      <div className="mt-4 flex flex-col gap-2.5">{children}</div>
    </div>
  )
}
