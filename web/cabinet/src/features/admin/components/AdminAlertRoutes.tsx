import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Activity, Check, ChevronDown, CircleCheck, CircleX, Handshake, Mail, Send, Server, Wallet, type LucideIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { ApiError, api, type AdminAlertTestResult } from '@/lib/api'
import { hapticError, hapticSuccess } from '@/lib/haptic'
import type { AdminSettingGroupDTO } from '@/lib/types/admin'
import { cn } from '@/lib/utils'
import { AdminToggleSwitch } from './AdminToggleSwitch'

interface AlertKind {
  kind: string
  enabledKey: string
  chatKey: string
  threadKey: string
  icon: LucideIcon
}

function prefixed(kind: string, prefix: string, icon: LucideIcon): AlertKind {
  return { kind, enabledKey: `${prefix}_ENABLED`, chatKey: `${prefix}_CHAT_ID`, threadKey: `${prefix}_THREAD_ID`, icon }
}

/**
 * Синхрон с internal/config/admin_alerts.go: порядок — порядок строк.
 * Оплаты и партнёрка — на своих старых ключах, их уже заполнили в .env и в БД.
 */
const KINDS: AlertKind[] = [
  prefixed('system', 'ADMIN_ALERT_SYSTEM', Mail),
  prefixed('servers', 'ADMIN_ALERT_SERVERS', Activity),
  prefixed('infra', 'ADMIN_ALERT_INFRA', Server),
  {
    kind: 'payments',
    enabledKey: 'PAYMENTS_NOTIFY_ENABLED',
    chatKey: 'PAYMENTS_NOTIFY_CHAT_ID',
    threadKey: 'PAYMENTS_NOTIFY_MESSAGE_THREAD_ID',
    icon: Wallet,
  },
  {
    kind: 'partner',
    enabledKey: 'PARTNER_NOTIFY_ENABLED',
    chatKey: 'PARTNER_NOTIFY_CHAT_ID',
    threadKey: 'PARTNER_NOTIFY_MESSAGE_THREAD_ID',
    icon: Handshake,
  },
]

const SHARED_KEY = 'ADMIN_ALERTS_CHAT_ID'
const EVENTS_KEY = 'PAYMENTS_NOTIFY_EVENTS'
const PAYMENT_EVENTS = ['paid', 'cancel'] as const

/** PAYMENTS_NOTIFY_EVENTS: пусто — оба события (так было всегда), none — ни одного. */
function parseEvents(raw: string): Set<string> {
  const v = raw.trim().toLowerCase()
  if (v === '') return new Set(PAYMENT_EVENTS)
  if (v === 'none') return new Set()
  return new Set(v.split(',').map((p) => p.trim()).filter((p) => (PAYMENT_EVENTS as readonly string[]).includes(p)))
}

function formatEvents(set: Set<string>): string {
  const list = PAYMENT_EVENTS.filter((e) => set.has(e))
  return list.length === 0 ? 'none' : list.join(',')
}

const inputClass = 'admin-input w-full rounded-lg border border-border bg-card px-3 py-2 text-sm'

type TestState = { kind: 'ok'; result: AdminAlertTestResult } | { kind: 'fail'; message: string }

interface Props {
  group: AdminSettingGroupDTO
  draft: Record<string, string>
  saving: boolean
  togglingKey: string | null
  onDraftChange: (key: string, value: string) => void
  onToggle: (key: string, checked: boolean) => void
}

/**
 * «Оповещения админу»: общий чат сверху и у каждого вида — выключатель, свой чат и тема.
 * Куда уйдёт сообщение, считается здесь по тем же правилам, что adminalert.Resolve:
 * чат вида → общий чат → личка админу. «Тест» шлёт то, что сейчас введено, без сохранения.
 */
export function AdminAlertRoutes({ group, draft, saving, togglingKey, onDraftChange, onToggle }: Props) {
  const { t } = useTranslation()
  const [help, setHelp] = useState(false)
  const [tests, setTests] = useState<Record<string, TestState>>({})

  // Старые ключи оплат и партнёрки хранят «не задано» как 0: в поле это пусто.
  const value = (key: string) => {
    const raw = (draft[key] ?? group.fields.find((f) => f.key === key)?.value ?? '').trim()
    return raw === '0' ? '' : raw
  }
  const shared = value(SHARED_KEY)
  // Ключи, которых нет в ответе (бэкенд старее фронта), не рисуем.
  const kinds = KINDS.filter((k) => group.fields.some((f) => f.key === k.enabledKey))

  const runTest = async ({ kind, chatKey, threadKey }: AlertKind) => {
    try {
      const result = await api.adminAlertTest({
        kind,
        draft: { shared_chat_id: shared, chat_id: value(chatKey), thread_id: value(threadKey) },
      })
      if (result.ok) {
        hapticSuccess()
        setTests((prev) => ({ ...prev, [kind]: { kind: 'ok', result } }))
      } else {
        hapticError()
        const hint = result.hint ? t(`admin.settings.alerts.hints.${result.hint}`) : ''
        setTests((prev) => ({
          ...prev,
          [kind]: { kind: 'fail', message: [hint, result.error].filter(Boolean).join(' · ') },
        }))
      }
    } catch (err) {
      hapticError()
      const message =
        err instanceof ApiError && err.status === 429
          ? t('admin.settings.alerts.tooFast')
          : err instanceof ApiError && err.status === 400
            ? t('admin.settings.alerts.badId')
            : t('admin.settings.alerts.testFailed')
      setTests((prev) => ({ ...prev, [kind]: { kind: 'fail', message } }))
    }
  }

  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-border/60 bg-card/80 p-3 shadow-sm">
        <label htmlFor="admin-alerts-shared" className="block text-sm font-medium">
          {t('admin.settings.alerts.sharedChat')}
        </label>
        <input
          id="admin-alerts-shared"
          className={cn(inputClass, 'mt-1.5')}
          inputMode="numeric"
          placeholder={t('admin.settings.alerts.sharedPlaceholder')}
          value={draft[SHARED_KEY] ?? shared}
          disabled={saving}
          onChange={(e) => onDraftChange(SHARED_KEY, e.target.value)}
        />
        <p className="mt-1.5 text-xs text-muted-foreground">{t('admin.settings.alerts.sharedHint')}</p>
        <button
          type="button"
          aria-expanded={help}
          onClick={() => setHelp((v) => !v)}
          className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
        >
          {t('admin.settings.alerts.howTo')}
          <ChevronDown className={cn('size-3.5 transition-transform', help && 'rotate-180')} />
        </button>
        {help && (
          <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs leading-relaxed text-muted-foreground">
            <li>{t('admin.settings.alerts.howTo1')}</li>
            <li>{t('admin.settings.alerts.howTo2')}</li>
            <li>{t('admin.settings.alerts.howTo3')}</li>
          </ol>
        )}
      </div>

      {/* ПК: по два вида в ряд, карточки ряда одной высоты; «Куда уйдёт» прижат к низу. */}
      <div className="grid gap-3 xl:grid-cols-2">
        {kinds.map((item, index) => {
          const { kind, enabledKey, chatKey, threadKey, icon: Icon } = item
          const enabled = value(enabledKey) !== 'false'
          const chat = value(chatKey)
          const thread = value(threadKey)
          const threadNum = Number(thread) > 0 ? thread : ''
          const target = chat
            ? t('admin.settings.alerts.toOwnChat', { chat })
            : shared
              ? t('admin.settings.alerts.toShared')
              : t('admin.settings.alerts.toDirect')
          const where = enabled
            ? chat || shared
              ? threadNum
                ? `${target} · ${t('admin.settings.alerts.topic', { id: threadNum })}`
                : target
              : target
            : t('admin.settings.alerts.off')
          const test = tests[kind]
          const okWhere =
            test?.kind === 'ok'
              ? test.result.direct
                ? t('admin.settings.alerts.toDirect')
                : [test.result.chat_title || String(test.result.chat_id), test.result.thread_id ? t('admin.settings.alerts.topic', { id: test.result.thread_id }) : '']
                    .filter(Boolean)
                    .join(' · ')
              : ''

          const events = kind === 'payments' && group.fields.some((f) => f.key === EVENTS_KEY) ? parseEvents(draft[EVENTS_KEY] ?? group.fields.find((f) => f.key === EVENTS_KEY)?.value ?? '') : null

          return (
            <div
              key={kind}
              className={cn(
                'flex flex-col rounded-lg border border-border/60 bg-card/80 p-3 shadow-sm',
                // Нечётный последний вид — на всю ширину, как общий чат: без дыры справа.
                kinds.length % 2 === 1 && index === kinds.length - 1 && 'xl:col-span-2',
              )}
            >
              <div className="flex items-start gap-3">
                <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 dark:bg-primary/20">
                  <Icon className="size-4 text-primary" aria-hidden />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium leading-snug">{t(`admin.settings.alerts.kinds.${kind}.title`)}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">{t(`admin.settings.alerts.kinds.${kind}.desc`)}</p>
                </div>
                <AdminToggleSwitch
                  checked={enabled}
                  busy={togglingKey === enabledKey}
                  disabled={saving}
                  aria-label={t(`admin.settings.alerts.kinds.${kind}.title`)}
                  onChange={(next) => onToggle(enabledKey, next)}
                />
              </div>

              <div className="mt-3 grid grid-cols-[minmax(0,1fr)_6.5rem] items-end gap-2 sm:grid-cols-[minmax(0,1fr)_8rem_auto]">
                <label className="block">
                  <span className="text-xs font-medium text-muted-foreground">{t('admin.settings.alerts.chat')}</span>
                  <input
                    className={cn(inputClass, 'mt-1')}
                    inputMode="numeric"
                    placeholder={t('admin.settings.alerts.chatPlaceholder')}
                    value={value(chatKey)}
                    disabled={saving}
                    onChange={(e) => onDraftChange(chatKey, e.target.value)}
                  />
                </label>
                <label className="block">
                  <span className="text-xs font-medium text-muted-foreground">{t('admin.settings.alerts.thread')}</span>
                  <input
                    className={cn(inputClass, 'mt-1')}
                    inputMode="numeric"
                    placeholder="—"
                    value={value(threadKey)}
                    disabled={saving}
                    onChange={(e) => onDraftChange(threadKey, e.target.value)}
                  />
                </label>
                <Button
                  type="button"
                  variant="outline"
                  className="col-span-2 h-10 sm:col-span-1"
                  onClick={() => runTest(item)}
                >
                  <Send />
                  {t('admin.settings.alerts.test')}
                </Button>
              </div>

              {events && (
                <div className="mt-3">
                  <p className="text-xs font-medium text-muted-foreground">{t('admin.settings.alerts.events')}</p>
                  <div className="mt-1.5 flex flex-wrap gap-2">
                    {PAYMENT_EVENTS.map((ev) => {
                      const on = events.has(ev)
                      return (
                        <button
                          key={ev}
                          type="button"
                          role="checkbox"
                          aria-checked={on}
                          disabled={saving}
                          onClick={() => {
                            const next = new Set(events)
                            if (on) next.delete(ev)
                            else next.add(ev)
                            onDraftChange(EVENTS_KEY, formatEvents(next))
                          }}
                          className={cn(
                            'inline-flex min-h-9 items-center gap-2 rounded-lg border px-3 text-sm font-medium transition-colors disabled:opacity-60',
                            on
                              ? 'border-primary/50 bg-primary/10 text-foreground'
                              : 'border-dashed border-border text-muted-foreground hover:text-foreground',
                          )}
                        >
                          <span
                            className={cn(
                              'flex size-4 items-center justify-center rounded-[5px]',
                              on ? 'bg-primary text-primary-foreground' : 'border-[1.5px] border-muted-foreground/60',
                            )}
                          >
                            {on && <Check className="size-3" strokeWidth={3} />}
                          </span>
                          {t(`admin.settings.alerts.event.${ev}`)}
                        </button>
                      )
                    })}
                  </div>
                  <p className={cn('mt-1.5 text-xs', events.size === 0 ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground')}>
                    {events.size === 0 ? t('admin.settings.alerts.eventsNone') : t('admin.settings.alerts.eventCancelHint')}
                  </p>
                </div>
              )}

              <div className="mt-auto">
                <p className="mt-2 text-xs text-muted-foreground">
                  {t('admin.settings.alerts.where')}: <span className={cn('font-medium', enabled ? 'text-foreground' : 'text-muted-foreground')}>{where}</span>
                  {enabled && !chat && !shared && threadNum ? ` · ${t('admin.settings.alerts.threadIgnored')}` : ''}
                </p>
                {test && (
                  <p
                    className={cn(
                      'mt-1.5 flex items-start gap-1.5 text-xs',
                      test.kind === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400',
                    )}
                    role="status"
                  >
                    {test.kind === 'ok' ? <CircleCheck className="mt-px size-3.5 shrink-0" /> : <CircleX className="mt-px size-3.5 shrink-0" />}
                    <span>{test.kind === 'ok' ? t('admin.settings.alerts.sent', { where: okWhere }) : test.message}</span>
                  </p>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
