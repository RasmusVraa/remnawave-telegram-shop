import { useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { Check, ChevronDown, CircleX } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { api, type ChannelGateProbeResult } from '@/lib/api'
import { hapticError, hapticSuccess } from '@/lib/haptic'
import { cn } from '@/lib/utils'
import { AdminToggleRow } from './AdminToggleSwitch'

const ENABLED_KEY = 'CHANNEL_GATE_ENABLED'
const CHANNEL_KEY = 'CHANNEL_GATE_CHANNEL'
const SCOPE_KEY = 'CHANNEL_GATE_SCOPE'

/** t.me/имя из «Ссылок в боте» — годится как канал для проверки; приватные приглашения (t.me/+…) нет. */
const PUBLIC_LINK = /^(?:https?:\/\/)?(?:t|telegram)\.me\/([A-Za-z][A-Za-z0-9_]{4,31})\/?$/i

interface AdminChannelGateEditorProps {
  draft: Record<string, string>
  saving: boolean
  onDraftChange: (key: string, value: string) => void
}

/**
 * Содержимое секции «Подписка на канал»: переключатель, поле канала с кнопкой
 * «Проверить» и памятка «Как подключить». Сохраняет обычная кнопка «Сохранить
 * секцию» — сервер сам проверит канал, и с неработающим включить не даст.
 */
export function AdminChannelGateEditor({ draft, saving, onDraftChange }: AdminChannelGateEditorProps) {
  const { t, i18n } = useTranslation()
  const [checking, setChecking] = useState(false)
  const [result, setResult] = useState<ChannelGateProbeResult | null>(null)
  const [failed, setFailed] = useState(false)
  // Памятка свёрнута: нужна один раз при подключении, дальше только занимает место.
  const [howOpen, setHowOpen] = useState(false)

  const enabled = (draft[ENABLED_KEY] ?? 'false').trim().toLowerCase() === 'true'
  const channel = draft[CHANNEL_KEY] ?? ''
  const scope: 'all' | 'trial' = (draft[SCOPE_KEY] ?? 'all').trim().toLowerCase() === 'trial' ? 'trial' : 'all'
  const linkMatch = PUBLIC_LINK.exec((draft.CHANNEL_URL ?? '').trim())
  const suggestion = !channel.trim() && linkMatch ? `@${linkMatch[1]}` : null

  async function check() {
    if (checking) return
    setChecking(true)
    setFailed(false)
    try {
      const res = await api.adminChannelGateProbe(channel)
      setResult(res)
      if (res.ok) hapticSuccess()
      else hapticError()
    } catch {
      setResult(null)
      setFailed(true)
      hapticError()
    } finally {
      setChecking(false)
    }
  }

  function changeChannel(value: string) {
    onDraftChange(CHANNEL_KEY, value)
    setResult(null)
    setFailed(false)
  }

  const errorCode = result && !result.ok ? (result.error ?? 'unavailable') : failed ? 'unavailable' : null

  return (
    <div className="mb-4">
        <div className="rounded-lg border border-border/60 bg-card/80 px-3 shadow-sm">
          <AdminToggleRow
            label={t(`admin.settings.fields.${ENABLED_KEY}.label`)}
            hint={t(`admin.settings.fields.${ENABLED_KEY}.hint`)}
            checked={enabled}
            disabled={saving}
            onChange={(checked) => onDraftChange(ENABLED_KEY, checked ? 'true' : 'false')}
          />

          <div className="space-y-2 py-3">
            <label htmlFor={`setting-${CHANNEL_KEY}`} className="block text-sm font-medium leading-snug text-foreground">
              {t(`admin.settings.fields.${CHANNEL_KEY}.label`)}
            </label>
            <div className="flex gap-2">
              <input
                id={`setting-${CHANNEL_KEY}`}
                type="text"
                inputMode="url"
                autoCapitalize="off"
                autoCorrect="off"
                spellCheck={false}
                placeholder="@meows_news"
                className={cn(
                  'admin-input min-w-0 flex-1 rounded-lg border bg-card px-3 py-2 text-sm',
                  errorCode ? 'border-destructive/60' : 'border-border',
                )}
                value={channel}
                disabled={saving}
                onChange={(e) => changeChannel(e.target.value)}
              />
              <Button
                type="button"
                variant="outline"
                loading={checking}
                disabled={saving || !channel.trim()}
                onClick={() => void check()}
                className="shrink-0"
              >
                {!checking && <Check />}
                {t('admin.settings.channelGate.check')}
              </Button>
            </div>

            {suggestion && (
              <button
                type="button"
                className="text-left text-xs text-primary hover:underline"
                onClick={() => changeChannel(suggestion)}
              >
                {t('admin.settings.channelGate.useLinks', { value: suggestion })}
              </button>
            )}

            {result?.ok && (
              <div className="flex items-center gap-3 rounded-xl border border-emerald-500/35 bg-emerald-500/[0.08] px-3 py-2.5">
                <span
                  className="flex size-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-sky-500 to-violet-500 text-sm font-extrabold text-white"
                  aria-hidden
                >
                  {(result.title ?? result.username ?? '?').trim().charAt(0).toUpperCase()}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-semibold">{result.title}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {(result.public_url ?? '').replace(/^https?:\/\//, '')}
                    {result.members ? ` · ${t('admin.settings.channelGate.members', { n: result.members.toLocaleString(i18n.language) })}` : ''}
                  </p>
                </div>
                <span className="inline-flex shrink-0 items-center gap-1 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                  <Check className="size-3.5" aria-hidden />
                  {t('admin.settings.channelGate.botAdmin')}
                </span>
              </div>
            )}

            {errorCode && (
              <div
                role="alert"
                className="flex items-start gap-2.5 rounded-xl border border-destructive/35 bg-destructive/[0.08] px-3 py-2.5 text-destructive"
              >
                <CircleX className="mt-0.5 size-4 shrink-0" aria-hidden />
                <div className="min-w-0 text-[13px] leading-snug">
                  <p className="font-semibold">{t(`admin.settings.channelGate.err.${errorCode}`)}</p>
                  {errorCode === 'bot_not_admin' && (
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {t('admin.settings.channelGate.err.bot_not_admin_hint')} {t('admin.settings.channelGate.mustCheck')}
                    </p>
                  )}
                </div>
              </div>
            )}
          </div>

          <div className="space-y-2 border-t border-border/50 py-3">
            <p className="text-sm font-medium leading-snug text-foreground">
              {t(`admin.settings.fields.${SCOPE_KEY}.label`)}
            </p>
            <div
              role="radiogroup"
              aria-label={t(`admin.settings.fields.${SCOPE_KEY}.label`)}
              className="grid grid-cols-2 gap-1 rounded-xl border border-border/60 bg-muted/40 p-1"
            >
              {(['all', 'trial'] as const).map((value) => (
                <button
                  key={value}
                  type="button"
                  role="radio"
                  aria-checked={scope === value}
                  disabled={saving}
                  onClick={() => onDraftChange(SCOPE_KEY, value)}
                  className={cn(
                    'rounded-lg px-3 py-2 text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                    scope === value
                      ? 'bg-primary/15 text-foreground ring-1 ring-primary/50'
                      : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  {t(`admin.settings.channelGate.scope.${value}`)}
                </button>
              ))}
            </div>
            <p className="text-xs leading-relaxed text-muted-foreground">
              {t(`admin.settings.channelGate.scope.${scope}Hint`)}
            </p>
          </div>

          <div className="border-t border-border/50 py-3">
            <button
              type="button"
              onClick={() => setHowOpen((v) => !v)}
              aria-expanded={howOpen}
              className="inline-flex items-center gap-1.5 rounded-md text-sm font-semibold text-primary outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {t('admin.settings.channelGate.howTitle')}
              <ChevronDown className={cn('size-4 transition-transform duration-200', howOpen && 'rotate-180')} aria-hidden />
            </button>
            {howOpen && (
              <ol className="admin-reveal mt-3 list-decimal space-y-1.5 pl-5 text-[13.5px] leading-relaxed text-foreground/90">
                <li>{t('admin.settings.channelGate.step1')}</li>
                <li>{t('admin.settings.channelGate.step2')}</li>
                <li>
                  <Trans
                    i18nKey="admin.settings.channelGate.step3"
                    components={{ c: <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[12.5px]" /> }}
                  />
                </li>
                <li>{t('admin.settings.channelGate.step4')}</li>
              </ol>
            )}
          </div>
        </div>
    </div>
  )
}
