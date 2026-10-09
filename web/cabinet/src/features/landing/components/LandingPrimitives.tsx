import type { ReactNode } from 'react'
import { ArrowRight } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Reveal } from './LandingMotion'

/**
 * Монохромный глиф Telegram: в отличие от TelegramBrandIcon (синий круг),
 * наследует currentColor. Используется ссылкой на бота в футере.
 */
export function TelegramGlyph({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="currentColor" aria-hidden>
      <path d="M21.6 4.3 3.1 11.4c-.9.35-.9 1.62.02 1.94l4.55 1.58 1.76 5.53c.22.7 1.1.9 1.6.37l2.55-2.66 4.6 3.37c.66.48 1.6.11 1.76-.69l3.1-14.9c.18-.86-.66-1.58-1.44-1.27ZM8.6 14.06l9.5-5.86c.2-.12.42.16.24.32l-7.5 6.75c-.2.18-.33.43-.36.7l-.24 1.99-1.64-3.9Z" />
    </svg>
  )
}

interface CabinetCtaProps {
  /** Абсолютный URL кабинета — лендинг отдаётся с двух адресов, см. useLandingBrand. */
  href: string
  /** Подпись приходит из i18n — компонент её не выдумывает. */
  label: string
  className?: string
  size?: 'md' | 'lg'
  /**
   * Скрыть от lg и шире. Нужно только в hero: на десктопе кнопка дублирует
   * «Войти» в шапке, а на мобильных шапка сворачивается в бургер — там она
   * остаётся единственной точкой входа.
   */
  desktopHidden?: boolean
}

/**
 * Единственный CTA лендинга — вход в веб-кабинет.
 *
 * Кнопки «Открыть в Telegram» здесь намеренно нет: бот не продаёт тарифы, он
 * сам уводит пользователя в этот же кабинет, так что промежуточный прыжок в
 * Telegram и обратно только удлинял бы путь. Ссылка на бота осталась в футере.
 */
export function LandingCabinetCta({
  href,
  label,
  className,
  size = 'md',
  desktopHidden = false,
}: CabinetCtaProps) {
  const sizing =
    size === 'lg' ? 'h-12 px-7 text-base sm:h-14 sm:px-8' : 'h-12 px-6 text-[0.95rem]'

  return (
    <div className={cn('flex', desktopHidden && 'lg:hidden', className)}>
      <a
        href={href}
        className={cn(
          'landing-cta landing-cta--primary group inline-flex items-center justify-center gap-2.5 rounded-full font-semibold',
          'outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--lp-cyan))] focus-visible:ring-offset-2 focus-visible:ring-offset-[hsl(var(--background))]',
          sizing,
        )}
      >
        {label}
        <ArrowRight className="size-[1.05rem] transition-transform duration-300 group-hover:translate-x-0.5" />
      </a>
    </div>
  )
}

interface SectionHeadingProps {
  eyebrow: string
  title: ReactNode
  description?: ReactNode
  className?: string
}

/** Единая шапка секции: бейдж → заголовок → лид. Все три появляются лесенкой. */
export function SectionHeading({ eyebrow, title, description, className }: SectionHeadingProps) {
  return (
    <div className={cn('max-w-2xl text-left', className)}>
      <Reveal>
        <span className="landing-pill px-3.5 py-1.5 text-xs font-semibold uppercase tracking-[0.14em]">
          <span className="size-1.5 rounded-full bg-[hsl(var(--lp-cyan))]" aria-hidden />
          {eyebrow}
        </span>
      </Reveal>
      <Reveal delay={0.08}>
        <h2 className="mt-4 text-balance font-heading text-3xl font-bold tracking-tight sm:mt-5 sm:text-4xl">
          {title}
        </h2>
      </Reveal>
      {description && (
        <Reveal delay={0.16}>
          <p className="mt-4 text-pretty text-base leading-relaxed text-muted-foreground">
            {description}
          </p>
        </Reveal>
      )}
    </div>
  )
}
