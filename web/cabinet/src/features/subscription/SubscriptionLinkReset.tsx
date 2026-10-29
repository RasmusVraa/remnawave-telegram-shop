import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, Check, Clock, Link2Off, Plus, RotateCcw, Smartphone, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useSheetDragDismiss } from '@/hooks/useSheetDragDismiss'
import { api, ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'

/**
 * Тихая точка входа в сброс ссылки: серый текст с подчёркиванием, без заливки
 * и акцентного цвета. Действие редкое и разрушительное, поэтому на странице оно
 * не должно спорить с основными кнопками — его находит тот, кто ищет.
 */
export function SubscriptionLinkResetLink({
  onClick,
  label,
  className,
}: {
  onClick: () => void
  label: string
  className?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'inline-flex min-h-9 items-center gap-1.5 text-left text-xs font-medium text-muted-foreground transition-colors hover:text-foreground',
        className,
      )}
    >
      <RotateCcw size={13} className="shrink-0" />
      <span className="underline decoration-muted-foreground/40 underline-offset-[3px]">{label}</span>
    </button>
  )
}

/** Длительность появления и ухода; из неё же задержка размонтирования. */
const ANIM_MS = 250

type Step = 'confirm' | 'done'

/**
 * Сброс ссылки подписки: подтверждение → результат.
 *
 * Подтверждение говорит три вещи — что перестанет работать, что придётся
 * сделать самому и что не изменится. Последний пункт важнее, чем кажется:
 * без него «сбросить подписку» читается как «потерять оплаченные дни».
 *
 * Результат сразу ведёт к следующему шагу — подключить свои устройства заново,
 * потому что после сброса отвалились и они.
 */
export function SubscriptionLinkResetSheet({
  open,
  availableAt,
  onClose,
  onConnectThis,
  onConnectOther,
}: {
  open: boolean
  /** link_reset_available_at из /me/subscription: пауза после прошлого сброса. */
  availableAt?: string | null
  onClose: () => void
  /** Гайд подключения на этом устройстве. */
  onConnectThis: () => void
  /** Шторка с QR и приглашением — уже с новой ссылкой. */
  onConnectOther: () => void
}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [mounted, setMounted] = useState(open)
  const [visible, setVisible] = useState(false)
  const [step, setStep] = useState<Step>('confirm')
  const [devicesCleared, setDevicesCleared] = useState(true)
  const [cooldownUntil, setCooldownUntil] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  const reset = useMutation({
    mutationFn: () => api.resetSubscriptionLink(),
    onSuccess: async (res) => {
      setDevicesCleared(res.devices_cleared)
      setStep('done')
      // Приглашения, выпущенные до сброса, сервер больше не принимает, —
      // закэшированное нельзя показывать ни секунды.
      void qc.resetQueries({ queryKey: ['connect-invite'] })
      await Promise.all([
        qc.invalidateQueries({ queryKey: ['subscription'] }),
        qc.invalidateQueries({ queryKey: ['devices'] }),
      ])
    },
    onError: (e) => {
      if (e instanceof ApiError && e.status === 429) {
        setCooldownUntil(parseAvailableAt(e.body) ?? new Date(Date.now() + 60_000).toISOString())
        return
      }
      setFailed(true)
    },
  })
  const busy = reset.isPending

  const { sheetRef, handlers: dragHandlers } = useSheetDragDismiss({
    open,
    onDismiss: onClose,
    disabled: busy,
  })

  useEffect(() => {
    if (open) {
      setMounted(true)
      setStep('confirm')
      setFailed(false)
      setCooldownUntil(null)
      const id = requestAnimationFrame(() => setVisible(true))
      return () => cancelAnimationFrame(id)
    }
    setVisible(false)
    // По таймеру, а не по transitionend: при «уменьшить движение» событие не
    // приходит, и окно осталось бы в DOM.
    const id = window.setTimeout(() => setMounted(false), ANIM_MS + 30)
    return () => window.clearTimeout(id)
  }, [open])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape' && !busy) onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, busy, onClose])

  if (typeof document === 'undefined' || !mounted) return null

  const waitUntil = cooldownUntil ?? availableAt ?? null
  const waitMinutes = waitUntil ? minutesUntil(waitUntil) : 0
  const onCooldown = step === 'confirm' && waitMinutes > 0

  return createPortal(
    <div
      className={cn(
        'fixed inset-0 z-[2000] flex items-end justify-center bg-black/60 backdrop-blur-sm transition-opacity duration-[250ms] ease-out sm:items-center sm:p-4',
        visible ? 'opacity-100' : 'opacity-0',
      )}
      onClick={() => !busy && onClose()}
    >
      <div
        ref={sheetRef}
        role="dialog"
        aria-modal="true"
        aria-label={step === 'done' ? t('linkReset.doneTitle') : t('linkReset.title')}
        onClick={(e) => e.stopPropagation()}
        className={cn(
          'max-h-[88dvh] w-full max-w-md overflow-y-auto rounded-t-2xl border border-border bg-background/95 p-4 pb-[max(1rem,calc(0.75rem+var(--cabinet-sheet-safe-bottom)))] shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] backdrop-blur-sm transition-[opacity,transform] duration-[250ms] ease-out sm:max-h-[90vh] sm:rounded-2xl sm:p-5 sm:pb-5',
          visible
            ? 'translate-y-0 opacity-100 sm:scale-100'
            : 'translate-y-full opacity-0 sm:translate-y-0 sm:scale-[0.97]',
        )}
      >
        <div
          className="-mx-4 -mt-4 mb-1 flex cursor-grab touch-none justify-center pb-2 pt-4 sm:hidden"
          {...dragHandlers}
        >
          <span className="h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden />
        </div>

        {step === 'confirm' ? (
          <>
            <div className="mb-4 flex touch-none items-start justify-between gap-3 sm:touch-auto" {...dragHandlers}>
              <p className="font-heading text-lg font-semibold leading-snug">{t('linkReset.title')}</p>
              <CloseButton onClick={onClose} disabled={busy} label={t('common.close')} />
            </div>

            <ul className="space-y-2.5 text-sm leading-snug">
              <Point icon={Link2Off}>{t('linkReset.pointOld')}</Point>
              <Point icon={AlertTriangle} tone="warn">{t('linkReset.pointYours')}</Point>
              <Point icon={Check} tone="ok">{t('linkReset.pointKept')}</Point>
            </ul>

            {onCooldown && (
              <div className="mt-4 flex gap-2 rounded-xl bg-amber-500/10 px-3 py-2.5 text-xs text-amber-700 dark:text-amber-300">
                <Clock size={14} className="mt-0.5 shrink-0" />
                <span>{t('linkReset.cooldown', { minutes: waitMinutes })}</span>
              </div>
            )}
            {failed && !onCooldown && (
              <p className="mt-4 text-xs text-destructive">{t('linkReset.failed')}</p>
            )}

            <div className="mt-5 flex flex-col gap-2">
              <Button
                type="button"
                variant="destructive"
                className="w-full"
                loading={busy}
                disabled={busy || onCooldown}
                onClick={() => {
                  setFailed(false)
                  reset.mutate()
                }}
              >
                <RotateCcw size={16} />
                {t('linkReset.confirm')}
              </Button>
              <Button type="button" variant="outline" className="w-full" disabled={busy} onClick={onClose}>
                {onCooldown ? t('common.close') : t('common.cancel')}
              </Button>
            </div>
          </>
        ) : (
          <>
            <div className="flex touch-none justify-end sm:touch-auto" {...dragHandlers}>
              <CloseButton onClick={onClose} label={t('common.close')} />
            </div>
            <div className="mx-auto -mt-2 mb-3 flex size-12 items-center justify-center rounded-full bg-primary/10 text-primary">
              <Check size={24} />
            </div>
            <p className="text-center font-heading text-lg font-semibold">{t('linkReset.doneTitle')}</p>
            <p className="mx-auto mt-1.5 max-w-xs text-center text-sm text-muted-foreground">
              {devicesCleared ? t('linkReset.doneText') : t('linkReset.doneTextDevicesLeft')}
            </p>
            <div className="mt-5 flex flex-col gap-2">
              <Button type="button" className="w-full" onClick={onConnectThis}>
                <Smartphone size={16} />
                {t('linkReset.connectThis')}
              </Button>
              <Button type="button" variant="outline" className="w-full" onClick={onConnectOther}>
                <Plus size={16} />
                {t('linkReset.connectOther')}
              </Button>
            </div>
          </>
        )}
      </div>
    </div>,
    document.body,
  )
}

function CloseButton({ onClick, label, disabled }: { onClick: () => void; label: string; disabled?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      className="shrink-0 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
    >
      <X size={16} />
    </button>
  )
}

function Point({
  icon: Icon,
  tone,
  children,
}: {
  icon: typeof Check
  tone?: 'warn' | 'ok'
  children: string
}) {
  return (
    <li className="flex gap-2.5">
      <Icon
        size={16}
        className={cn(
          'mt-0.5 shrink-0',
          tone === 'warn' && 'text-amber-600 dark:text-amber-400',
          tone === 'ok' && 'text-primary',
          !tone && 'text-muted-foreground',
        )}
      />
      <span>{children}</span>
    </li>
  )
}

/** Тело 429: {"error":"cooldown","available_at":"..."}. */
function parseAvailableAt(body: string): string | null {
  try {
    const v = (JSON.parse(body) as { available_at?: string }).available_at
    return typeof v === 'string' && v ? v : null
  } catch {
    return null
  }
}

/** Сколько минут осталось до момента; округление вверх, чтобы не показать «0 мин». */
function minutesUntil(iso: string): number {
  const ms = new Date(iso).getTime() - Date.now()
  return ms > 0 ? Math.ceil(ms / 60_000) : 0
}
