import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Globe, Moon, Smartphone, Sun, Vibrate, VibrateOff } from 'lucide-react'

import { useLightThemeAllowed } from '@/components/ThemePolicyProvider'
import { useTheme } from '@/hooks/useTheme'
import { setLanguage } from '@/i18n'
import { hapticSelection, setHapticsEnabled, useHapticsEnabled } from '@/lib/haptic'
import { cn } from '@/lib/utils'

/**
 * «Настройки» в профиле: язык, тема и вибрация.
 *
 * На телефоне шапка кабинета — только логотип и ☰, поэтому переключатели
 * переехали сюда. На ПК они остаются и в шапке: одна и та же логика,
 * карточка ничего не дублирует, а просто находится там, где её ищут.
 * Тему прячем, если админ разрешил только тёмную (праздничные темы).
 * Вибрация показывается всегда, даже на ПК: выбор общий для браузера и ничему не мешает.
 *
 * Язык — флагами, тема — луной и солнцем: короче подписей и читается сразу.
 * Флаги нарисованы SVG, а не эмодзи: на Windows эмодзи-флаги показываются
 * буквами «RU» и «US».
 */
export function ProfileSettingsCard() {
  const { t, i18n } = useTranslation()
  const lightAllowed = useLightThemeAllowed()
  const lang = i18n.language === 'en' ? 'en' : 'ru'

  return (
    // Отступы и кегль — как у карточки аккаунта над ней: cabinet-card-pad,
    // подписи text-sm. Иначе блок выглядит на ступень крупнее соседей.
    <section className="cabinet-elevated-card cabinet-card-pad">
      <p className="pb-2 text-base font-semibold">{t('home.settingsTitle')}</p>
      <SettingRow icon={<Globe className="size-4" />} label={t('home.settingsLanguage')}>
        <Segmented
          value={lang}
          options={[
            { value: 'ru', label: <FlagRu />, title: 'Русский' },
            { value: 'en', label: <FlagUs />, title: 'English' },
          ]}
          onChange={(v) => setLanguage(v)}
          ariaLabel={t('home.settingsLanguage')}
        />
      </SettingRow>
      {lightAllowed && <ThemeRow />}
      <HapticsRow />
    </section>
  )
}

function ThemeRow() {
  const { t } = useTranslation()
  const { theme, set } = useTheme()
  return (
    <SettingRow icon={<Moon className="size-4" />} label={t('home.settingsTheme')}>
      <Segmented
        value={theme}
        options={[
          { value: 'dark', label: <Moon className="size-4" strokeWidth={2} />, title: t('theme.dark') },
          { value: 'light', label: <Sun className="size-4" strokeWidth={2} />, title: t('theme.light') },
        ]}
        onChange={(v) => set(v as 'dark' | 'light')}
        ariaLabel={t('home.settingsTheme')}
      />
    </SettingRow>
  )
}

function HapticsRow() {
  const { t } = useTranslation()
  const on = useHapticsEnabled()
  return (
    <SettingRow icon={<Smartphone className="size-4" />} label={t('home.settingsHaptics')}>
      <Segmented
        value={on ? 'on' : 'off'}
        options={[
          { value: 'off', label: <VibrateOff className="size-4" strokeWidth={2} />, title: t('home.settingsHapticsOff') },
          { value: 'on', label: <Vibrate className="size-4" strokeWidth={2} />, title: t('home.settingsHapticsOn') },
        ]}
        onChange={(v) => setHapticsEnabled(v === 'on')}
        ariaLabel={t('home.settingsHaptics')}
      />
    </SettingRow>
  )
}

function SettingRow({ icon, label, children }: { icon: ReactNode; label: string; children: ReactNode }) {
  return (
    <div className="flex items-center gap-3 py-1 text-sm last:pb-0">
      <span className="text-muted-foreground">{icon}</span>
      <span className="min-w-0 flex-1">{label}</span>
      {children}
    </div>
  )
}

function Segmented({
  value,
  options,
  onChange,
  ariaLabel,
}: {
  value: string
  options: { value: string; label: ReactNode; title: string }[]
  onChange: (v: string) => void
  ariaLabel: string
}) {
  return (
    <div role="radiogroup" aria-label={ariaLabel} className="inline-flex shrink-0 rounded-[10px] border border-border bg-muted p-[3px]">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          aria-label={o.title}
          title={o.title}
          onClick={() => {
            if (value === o.value) return
            onChange(o.value)
            // После onChange: включение вибрации сразу даёт её почувствовать, выключение — молчит.
            hapticSelection()
          }}
          className={cn(
            'inline-flex h-7 w-10 items-center justify-center rounded-[7px] transition-[background-color,color,opacity]',
            value === o.value
              ? 'bg-secondary text-foreground shadow-sm'
              : 'text-muted-foreground opacity-60 hover:text-foreground hover:opacity-100',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

/** Флаг России: три полосы 3:2, со скруглением и тонкой обводкой — на тёмном фоне белая полоса не сливается. */
function FlagRu() {
  return (
    <svg viewBox="0 0 30 20" className="h-[15px] w-[22px] overflow-hidden rounded-[3px] ring-1 ring-black/15" aria-hidden>
      <rect width="30" height="20" fill="#fff" />
      <rect y="6.67" width="30" height="6.67" fill="#0039A6" />
      <rect y="13.33" width="30" height="6.67" fill="#D52B1E" />
    </svg>
  )
}

/** Флаг США в упрощённом виде: семь полос и синий крыж — в 22px звёзды всё равно не различить. */
function FlagUs() {
  return (
    <svg viewBox="0 0 30 20" className="h-[15px] w-[22px] overflow-hidden rounded-[3px] ring-1 ring-black/15" aria-hidden>
      <rect width="30" height="20" fill="#fff" />
      {[0, 2, 4, 6, 8, 10, 12].map((i) => (
        <rect key={i} y={(i * 20) / 13} width="30" height={20 / 13} fill="#B22234" />
      ))}
      <rect width="13" height={(20 * 7) / 13} fill="#3C3B6E" />
      {[0, 1, 2].map((r) =>
        [0, 1, 2, 3].map((c) => (
          <circle key={`${r}-${c}`} cx={2 + c * 3} cy={2 + r * 3.4} r="0.7" fill="#fff" />
        )),
      )}
    </svg>
  )
}
