import { useState } from 'react'
import { ChevronDown } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import { cn } from '@/lib/utils'
import { AppTile, PlatformIcon, platformLabel } from './glyphs'
import type { GuideText } from './guideText'
import type { AppGuide, PlatformKey } from './types'

/* ── Шаг-линия ─────────────────────────────────────────────────────── */

export function GuideCounter({
  text,
  current,
  total,
  finished,
}: {
  text: GuideText
  current: number
  total: number
  finished: boolean
}) {
  return finished ? (
    <span className="whitespace-nowrap text-[13px] font-bold text-emerald-700 dark:text-emerald-400">{text.doneShort} ✓</span>
  ) : (
    <span className="whitespace-nowrap text-[13px] font-bold text-muted-foreground">{text.stepOf(current + 1, total)}</span>
  )
}

/**
 * Сегменты с подписями, по одному на шаг (их 2 или 3 — шаг «Включение»
 * бывает не у всех приложений). Каждый — кнопка: так же попадают сразу на
 * шаг 2 те, у кого приложение уже стоит.
 */
export function GuideStepLine({
  text,
  current,
  total,
  done,
  finished,
  onPick,
  withCounter,
}: {
  text: GuideText
  current: number
  total: number
  done: boolean[]
  finished: boolean
  onPick: (step: number) => void
  withCounter: boolean
}) {
  // Всё уже пройдено, но человек вернулся посмотреть шаг: синий сегмент среди
  // зелёных читался бы как «этот шаг не сделан», поэтому текущий — ярко-зелёный.
  const allDone = done.slice(0, total).every(Boolean)
  return (
    <div className="flex items-start gap-3">
      <div className={cn('grid flex-1 gap-2', total === 2 ? 'grid-cols-2' : 'grid-cols-3')}>
        {text.stepShort.slice(0, total).map((label, i) => {
          const full = finished || done[i]
          const isCur = !finished && i === current
          const reviewing = allDone && !finished
          // Открыт уже пройденный шаг — сегмент ярко-зелёный, а не синий.
          const brightCurrent = isCur && done[i]
          return (
            <button
              key={label}
              type="button"
              onClick={() => onPick(i)}
              aria-current={isCur ? 'step' : undefined}
              aria-label={`${text.stepLabel(i + 1)}: ${label}`}
              className={cn(
                // items-stretch обязателен: в Safari у кнопки по умолчанию align-items: flex-start,
                // и пустая полоска сжимается до нулевой ширины — шаг-линию не видно.
                'group flex min-w-0 flex-col items-stretch gap-1.5 rounded-md pt-1 text-left text-xs font-semibold transition-colors',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
                brightCurrent
                  ? 'text-foreground'
                  : full
                    ? 'text-emerald-700 dark:text-emerald-400'
                    : isCur
                      ? 'text-foreground'
                      : 'text-muted-foreground',
                reviewing && !isCur && 'opacity-70 hover:opacity-100',
              )}
            >
              <i
                className={cn(
                  'relative block h-1.5 w-full overflow-hidden rounded-full transition-[background-color,box-shadow,height,margin]',
                  brightCurrent
                    ? '-mt-px h-2 bg-gradient-to-r from-emerald-500 to-emerald-300 shadow-[0_0_14px_rgb(16_185_129_/_0.7)]'
                    : full
                      ? 'bg-emerald-500'
                      : 'bg-foreground/10 group-hover:bg-foreground/20',
                )}
              >
                {isCur && !brightCurrent ? (
                  <span className="absolute inset-0 rounded-full bg-primary animate-[cg-seg_.6s_ease-out_both]" />
                ) : null}
              </i>
              <span className="truncate">
                {done[i] ? '✓ ' : ''}
                {label}
              </span>
            </button>
          )
        })}
      </div>
      {withCounter ? <GuideCounter text={text} current={current} total={total} finished={finished} /> : null}
    </div>
  )
}

/* ── Кнопка «устройство · приложение ▾» ──────────────────────────────── */

export function DeviceChip({
  text,
  platform,
  app,
  onOpen,
  compact,
}: {
  text: GuideText
  platform: PlatformKey
  app: AppGuide
  onOpen: () => void
  /** Телефон: только иконки и стрелка — кнопка стоит в строке заголовка. */
  compact?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      // Слова «Изменить» на кнопке нет — его заменяет стрелка, но читалке экрана оно нужно
      aria-label={`${platformLabel[platform] || platform} · ${app.name}. ${text.change}`}
      aria-haspopup="dialog"
      className={cn(
        'group inline-flex max-w-full items-center gap-2 border border-border bg-foreground/[0.04] text-sm font-medium text-foreground transition-colors',
        'hover:border-primary/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        'dark:border-white/10 dark:bg-white/[0.04]',
        compact ? 'shrink-0 rounded-full py-1 pl-1 pr-2' : 'rounded-full py-1 pl-1.5 pr-2.5',
      )}
    >
      <span className="grid size-[26px] shrink-0 place-items-center rounded-full bg-primary/15 text-primary">
        <PlatformIcon platform={platform} size={14} />
      </span>
      {compact ? null : (
        <>
          <span className="truncate">{platformLabel[platform] || platform}</span>
          <span className="opacity-35">·</span>
        </>
      )}
      <AppTile app={app} className="size-[22px] text-[8px]" />
      {compact ? null : <span className="truncate">{app.name}</span>}
      <ChevronDown
        size={16}
        aria-hidden
        className="ml-0.5 shrink-0 text-primary transition-transform duration-200 group-hover:translate-y-0.5"
      />
    </button>
  )
}

/* ── Текст из app-config (markdown) ────────────────────────────────── */

const mdClass = cn(
  'text-sm leading-6 text-muted-foreground dark:text-slate-300',
  '[&_a]:font-medium [&_a]:text-primary [&_a]:underline [&_a]:underline-offset-2 [&_a]:hover:no-underline',
  '[&_ol]:ml-4 [&_ol]:list-decimal [&_ol]:space-y-0.5 [&_ul]:ml-4 [&_ul]:list-disc [&_p]:mb-1.5 [&_p]:last:mb-0',
)

/** Длинный текст считаем от 150 символов или по нескольким абзацам. */
function isLong(text: string): boolean {
  return text.length > 150 || /\n\s*\n/.test(text)
}

export function GuideMarkdown({ text, clamp, labels, className }: { text: string; clamp?: boolean; labels: GuideText; className?: string }) {
  const [open, setOpen] = useState(false)
  if (!text.trim()) return null
  const collapsible = Boolean(clamp) && isLong(text)
  return (
    <div className={className}>
      <div className={cn(mdClass, collapsible && !open && 'line-clamp-3')}>
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          components={{
            a: ({ href, children }) => (
              <a href={href} target="_blank" rel="noopener noreferrer">
                {children}
              </a>
            ),
          }}
        >
          {text}
        </ReactMarkdown>
      </div>
      {collapsible ? (
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="mt-0.5 text-[13px] font-semibold text-primary hover:underline"
        >
          {open ? labels.less : labels.more}
        </button>
      ) : null}
    </div>
  )
}
