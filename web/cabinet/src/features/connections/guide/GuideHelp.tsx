import { forwardRef, type ReactNode } from 'react'
import { ChevronDown, CircleHelp, CirclePlay, Copy, ExternalLink, MessageCircle, Check } from 'lucide-react'

import { cn } from '@/lib/utils'
import { GuideMarkdown } from './GuideParts'
import type { GuideText } from './guideText'
import { pickText, type GuideStep, type Lang, type LinkButton } from './types'

type Props = {
  text: GuideText
  lang: Lang
  open: boolean
  onToggle: () => void
  /** additionalAfterAddSubscriptionStep выбранного приложения. */
  extra?: GuideStep
  /** null — копировать нечего (нет сырой ссылки), плитки нет. */
  onCopy: (() => void) | null
  copied: boolean
  /** null — поддержка не настроена, плитки нет. */
  onSupport: (() => void) | null
  className?: string
}

/** Кнопка похожа на видео: по тексту или по ссылке на видеохостинг. */
function isVideoButton(btn: LinkButton, lang: Lang): boolean {
  return /видео|video/i.test(pickText(btn.buttonText, lang)) || /youtu\.?be|rutube|vk\.com\/video/i.test(btn.buttonLink)
}

function Tile({
  icon,
  tone,
  title,
  hint,
  onClick,
  href,
}: {
  icon: ReactNode
  tone: string
  title: string
  hint?: string
  onClick?: () => void
  href?: string
}) {
  const cls = cn(
    'flex items-center gap-3 rounded-2xl border border-border bg-foreground/[0.04] p-3 text-left text-foreground transition-colors',
    'hover:border-primary/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:border-white/10',
  )
  const body = (
    <>
      <span className={cn('grid size-10 shrink-0 place-items-center rounded-xl', tone)}>{icon}</span>
      <span className="min-w-0">
        <b className="block text-sm font-semibold leading-tight">{title}</b>
        {hint ? <small className="block text-[12.5px] text-muted-foreground">{hint}</small> : null}
      </span>
    </>
  )
  if (href) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" className={cls}>
        {body}
      </a>
    )
  }
  return (
    <button type="button" onClick={onClick} className={cls}>
      {body}
    </button>
  )
}

/**
 * «Что-то не получилось?». Каждая плитка — только если для неё есть данные;
 * нет ни текста, ни плиток — блока нет совсем.
 */
export const GuideHelp = forwardRef<HTMLDivElement, Props>(function GuideHelp(
  { text, lang, open, onToggle, extra, onCopy, copied, onSupport, className },
  ref,
) {
  const title = pickText(extra?.title, lang)
  const description = pickText(extra?.description, lang)
  const buttons = (extra?.buttons || []).filter((b) => b.buttonLink?.trim())
  const hasTiles = buttons.length > 0 || Boolean(onCopy) || Boolean(onSupport)
  if (!description.trim() && !hasTiles) return null

  return (
    <div
      ref={ref}
      className={cn(
        'overflow-hidden rounded-2xl border border-border dark:border-white/10',
        // Прокрутка к раскрытому блоку не должна прятать его низ под нижнее меню телефона.
        'scroll-mt-20 scroll-mb-[calc(6.5rem+max(env(safe-area-inset-bottom,0px),var(--cabinet-tg-safe-bottom)))] sm:scroll-mb-4',
        className,
      )}
    >
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="flex w-full items-center gap-2.5 bg-foreground/[0.03] px-3.5 py-3 text-left text-[15px] font-semibold text-foreground"
      >
        <span className="grid size-7 place-items-center rounded-lg bg-amber-400/15 text-amber-600 dark:text-amber-300">
          <CircleHelp size={16} />
        </span>
        {text.helpTitle}
        <ChevronDown size={16} className={cn('ml-auto text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>
      {open ? (
        <div className="flex flex-col gap-3 px-3.5 pb-3.5 pt-3">
          {description.trim() ? (
            <div>
              <b className="mb-1 block text-sm font-semibold text-foreground">{title || text.helpFallbackTitle}</b>
              <GuideMarkdown text={description} labels={text} />
            </div>
          ) : null}
          {hasTiles ? (
            <div className="grid gap-2 md:grid-cols-3">
              {buttons.map((btn, i) => (
                <Tile
                  key={`${btn.buttonLink}-${i}`}
                  href={btn.buttonLink}
                  tone="bg-rose-500/15 text-rose-500 dark:text-rose-400"
                  icon={isVideoButton(btn, lang) ? <CirclePlay size={19} /> : <ExternalLink size={18} />}
                  title={pickText(btn.buttonText, lang)}
                  hint={text.helpHowTo}
                />
              ))}
              {onCopy ? (
                <Tile
                  onClick={onCopy}
                  tone="bg-primary/15 text-primary"
                  icon={copied ? <Check size={18} /> : <Copy size={18} />}
                  title={text.copyLink}
                  hint={copied ? text.copied : text.copyLinkHint}
                />
              ) : null}
              {onSupport ? (
                <Tile
                  onClick={onSupport}
                  tone="bg-violet-500/15 text-violet-600 dark:text-violet-300"
                  icon={<MessageCircle size={18} />}
                  title={text.support}
                />
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
})
