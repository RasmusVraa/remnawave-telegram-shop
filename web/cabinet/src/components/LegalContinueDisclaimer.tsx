import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type Props = {
  siteLinks?: Record<string, string> | null
  className?: string
  /** short — одна строка «Оплачивая, вы принимаете…» для плашки оплаты на телефоне. */
  variant?: 'full' | 'short'
}

function resolveAgreementURL(siteLinks?: Record<string, string> | null): string {
  if (!siteLinks) return ''
  return (
    siteLinks.terms_of_service?.trim() ||
    siteLinks.public_offer?.trim() ||
    siteLinks.tos?.trim() ||
    ''
  )
}

function DocLink({ href, children }: { href?: string; children: ReactNode }) {
  if (!href) {
    return <span className="underline underline-offset-2">{children}</span>
  }
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="underline underline-offset-2 hover:text-foreground"
    >
      {children}
    </a>
  )
}

/** Дисклеймер «Продолжая, Вы соглашаетесь…» со ссылками на политику и соглашение. */
export function LegalContinueDisclaimer({ siteLinks, className, variant = 'full' }: Props) {
  const { t } = useTranslation()
  const privacy = siteLinks?.privacy_policy?.trim() || ''
  const agreement = resolveAgreementURL(siteLinks)
  if (!privacy && !agreement) {
    return null
  }

  if (variant === 'short') {
    return (
      <p className={cn('text-center text-[11px] leading-snug text-muted-foreground', className)}>
        {t('legal.payShortBefore')}
        <DocLink href={agreement || undefined}>{t('legal.payShortAgreement')}</DocLink>
        {t('legal.continueAnd')}
        <DocLink href={privacy || undefined}>{t('legal.payShortPrivacy')}</DocLink>
      </p>
    )
  }

  return (
    <p className={cn('text-center text-xs leading-snug text-muted-foreground', className)}>
      {t('legal.continueBefore')}
      <DocLink href={privacy || undefined}>{t('legal.privacyPolicy')}</DocLink>
      {t('legal.continueAnd')}
      <DocLink href={agreement || undefined}>{t('legal.userAgreement')}</DocLink>
    </p>
  )
}
