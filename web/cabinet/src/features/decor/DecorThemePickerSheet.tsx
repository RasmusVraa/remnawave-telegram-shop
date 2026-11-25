import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BadgePercent,
  Cat,
  Gem,
  Check,
  Flame,
  Flower2,
  Ghost,
  Heart,
  Leaf,
  Moon,
  Snowflake,
  Sparkles,
  Star,
  Sun,
  TreePine,
  Waves,
  Wine,
  Zap,
  type LucideIcon,
} from 'lucide-react'

import { ResponsiveSheet } from '@/components/ResponsiveSheet'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { DECOR_PICKER_ACCENT, DECOR_PICKER_GROUPS } from './decorThemePicker'
import { isDarkOnlyDecorTheme, type DecorThemeId } from './decorThemes'

export type DecorPickerValue = 'auto' | DecorThemeId

/**
 * Окно выбора оформления: шторка на телефоне, окно по центру на ПК.
 * Одно и то же для пользователя (профиль, есть пункт «Как у сервиса») и для админа
 * (тема по умолчанию, пункта «Как у сервиса» нет). Выбор применяется кнопкой внизу.
 */
export function DecorThemePickerSheet({
  open,
  onClose,
  value,
  onApply,
  includeAuto,
}: {
  open: boolean
  onClose: () => void
  value: DecorPickerValue
  /** Если вернёт Promise — кнопка показывает загрузку, окно закроется после ответа. */
  onApply: (next: DecorPickerValue) => void | Promise<void>
  includeAuto: boolean
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<DecorPickerValue>(value)

  // При каждом открытии начинаем с текущего значения.
  useEffect(() => {
    if (open) setDraft(value)
  }, [open, value])

  const darkOnly = draft !== 'auto' && isDarkOnlyDecorTheme(draft)

  return (
    <ResponsiveSheet
      open={open}
      onClose={onClose}
      centered
      title={t('home.decorPicker.title')}
      className="sm:max-w-[600px]"
      footer={
        <div className="flex gap-2">
          <Button type="button" variant="secondary" className="flex-1" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            type="button"
            className="flex-1"
            onClick={async () => {
              if (draft !== value) await onApply(draft)
              onClose()
            }}
          >
            {t('home.decorPicker.apply')}
          </Button>
        </div>
      }
    >
      <div role="radiogroup" aria-label={t('home.decorPicker.title')} className="space-y-3">
        <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
          {includeAuto && (
            <Tile
              selected={draft === 'auto'}
              onSelect={() => setDraft('auto')}
              label={t('home.decorPicker.auto')}
              accent="hsl(var(--primary))"
              preview={
                <span
                  className="absolute inset-0"
                  style={{
                    background: 'linear-gradient(135deg, hsl(var(--card)) 50%, hsl(var(--primary) / 0.35) 50%)',
                  }}
                />
              }
            />
          )}
          <ThemeTile id="off" draft={draft} onSelect={setDraft} />
        </div>
        {DECOR_PICKER_GROUPS.map((group) => (
          <div key={group.id}>
            <p className="mb-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
              {t(`home.decorPicker.groups.${group.id}`)}
            </p>
            <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
              {group.themes.map((id) => (
                <ThemeTile key={id} id={id} draft={draft} onSelect={setDraft} />
              ))}
            </div>
          </div>
        ))}
        {darkOnly && (
          <p className="flex items-start gap-2 rounded-xl border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
            <Moon className="mt-0.5 size-3.5 shrink-0" />
            {t('home.decorPicker.darkOnlyHint')}
          </p>
        )}
      </div>
    </ResponsiveSheet>
  )
}

function ThemeTile({
  id,
  draft,
  onSelect,
}: {
  id: DecorThemeId
  draft: DecorPickerValue
  onSelect: (v: DecorPickerValue) => void
}) {
  const { t } = useTranslation()
  const accent = DECOR_PICKER_ACCENT[id]
  return (
    <Tile
      selected={draft === id}
      onSelect={() => onSelect(id)}
      label={t(`admin.settings.enum.${id}`, { defaultValue: id })}
      accent={accent}
      tag={isDarkOnlyDecorTheme(id) ? t('home.decorPicker.darkOnly') : undefined}
      preview={
        <>
          <span
            className="absolute inset-0"
            style={{ background: `linear-gradient(160deg, hsl(220 30% 9%), ${accent.replace(')', ' / 0.28)')})` }}
          />
          <ThemeArt id={id} accent={accent} />
        </>
      }
    />
  )
}

/** Значок, по которому тема узнаётся: то, что рисует сама тема (снег, тыквы, листья и т.д.). */
const THEME_ICON: Partial<Record<DecorThemeId, LucideIcon>> = {
  new_year: Snowflake,
  new_year_2: TreePine,
  halloween_2: Ghost,
  summer: Sun,
  spring: Leaf,
  valentine: Heart,
  black_friday: BadgePercent,
  lime: Cat,
  emerald: Gem,
  ocean: Waves,
  aurora: Sparkles,
  nebula: Star,
  sunset: Flame,
  lavender: Flower2,
  wine: Wine,
  neon: Zap,
}

function ThemeArt({ id, accent }: { id: DecorThemeId; accent: string }) {
  if (id === 'cyber') return <MatrixGlyphs accent={accent} />
  if (id === 'halloween') {
    return (
      <span className="absolute bottom-1 right-1.5 text-[24px] leading-none" aria-hidden>
        🎃
      </span>
    )
  }
  const Icon = THEME_ICON[id]
  if (!Icon) return null
  return (
    <Icon
      className="absolute bottom-1.5 right-1.5 size-6 opacity-90"
      style={{ color: accent }}
      strokeWidth={1.75}
      aria-hidden
    />
  )
}

/** «Матрица» кибер-темы: падающие столбцы знаков, как в фильме. Статичная картинка, без анимации. */
const MATRIX_COLUMNS = ['ｱ7ｳ2ｴ9', 'ﾊ1ﾐ4ﾘ', '0ｾ5ﾏ3ｦ', 'ﾓ8ｹ1', 'ｼ6ﾈ0ｻ', 'ﾁ3ﾂ9']

function MatrixGlyphs({ accent }: { accent: string }) {
  return (
    <span className="pointer-events-none absolute inset-0 flex justify-around overflow-hidden" aria-hidden>
      {MATRIX_COLUMNS.map((col, i) => (
        <span
          key={i}
          className="flex flex-col font-mono text-[9px] font-bold leading-[10px]"
          style={{
            color: accent,
            marginTop: -(i % 3) * 5,
            maskImage: 'linear-gradient(to bottom, rgba(0,0,0,0.9), transparent 90%)',
            WebkitMaskImage: 'linear-gradient(to bottom, rgba(0,0,0,0.9), transparent 90%)',
          }}
        >
          {col.split('').map((ch, j) => (
            <span key={j}>{ch}</span>
          ))}
        </span>
      ))}
    </span>
  )
}

function Tile({
  selected,
  onSelect,
  label,
  tag,
  preview,
  accent,
}: {
  selected: boolean
  onSelect: () => void
  label: string
  tag?: string
  preview: ReactNode
  accent: string
}) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onSelect}
      className={cn(
        'relative rounded-[14px] border-[1.5px] bg-muted p-1.5 text-left transition-colors',
        selected
          ? 'border-primary shadow-[0_0_0_2px_hsl(var(--primary)/0.25)]'
          : 'border-border hover:border-muted-foreground/50',
      )}
    >
      <span className="relative mb-1.5 block h-[52px] overflow-hidden rounded-[9px]">
        {preview}
        <span className="absolute inset-x-1.5 top-1.5 h-3 rounded-[5px] bg-white/10" />
        <span className="absolute bottom-1.5 left-1.5 h-3.5 w-[46%] rounded-md" style={{ background: accent }} />
      </span>
      <span className="block px-0.5 text-[12.5px] leading-tight">{label}</span>
      {tag && <span className="block px-0.5 text-[10px] text-amber-400">{tag}</span>}
      {selected && (
        <span className="absolute right-2 top-2 grid size-[18px] place-items-center rounded-full bg-primary text-primary-foreground">
          <Check className="size-3" strokeWidth={3} />
        </span>
      )}
    </button>
  )
}
