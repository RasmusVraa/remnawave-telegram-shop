import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Check, Megaphone, SquareArrowOutUpRight } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { hapticError, hapticSuccess } from '@/lib/haptic'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/store/auth'

/** Длительность появления и ухода; из неё же считается задержка размонтирования. */
const ANIM_MS = 250
/** Сколько держим «Готово», прежде чем закрыть окно. */
const DONE_MS = 1100

type Phase = 'ask' | 'done'

/**
 * Окно обязательной подписки на Telegram-канал. Показывается, когда сервер
 * ответил `channel_gate.required` в /me или любой закрытый запрос вернул
 * 403 channel_subscription_required (см. lib/api.ts).
 *
 * Закрыть нельзя: ни крестика, ни клика по фону, ни Esc — до подписки
 * кабинетом пользоваться не получится, и обойти это окно можно только
 * подпиской. Настоящую защиту держит сервер, окно — лишь объяснение.
 * Шторка снизу на телефоне, по центру на ПК.
 */
export function ChannelGateModal() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const gate = useAuthStore((s) => s.user?.channel_gate)
  const setChannelGate = useAuthStore((s) => s.setChannelGate)
  const open = Boolean(gate?.required)

  const [mounted, setMounted] = useState(open)
  const [visible, setVisible] = useState(false)
  const [phase, setPhase] = useState<Phase>('ask')
  const [checking, setChecking] = useState(false)
  const [message, setMessage] = useState<'notYet' | 'error' | null>(null)
  // Ссылку держим после закрытия: пока идёт анимация ухода, gate уже пуст.
  const urlRef = useRef<string | undefined>(undefined)
  const dialogRef = useRef<HTMLDivElement>(null)
  if (gate?.channel_url) urlRef.current = gate.channel_url

  useEffect(() => {
    if (open) {
      setMounted(true)
      // Два кадра: в первом разметка только вставляется — иначе выезда снизу не будет.
      let inner = 0
      const outer = requestAnimationFrame(() => {
        inner = requestAnimationFrame(() => setVisible(true))
      })
      return () => {
        cancelAnimationFrame(outer)
        cancelAnimationFrame(inner)
      }
    }
    setVisible(false)
    const id = window.setTimeout(() => {
      setMounted(false)
      setPhase('ask')
      setMessage(null)
    }, ANIM_MS + 30)
    return () => window.clearTimeout(id)
  }, [open])

  useEffect(() => {
    if (!mounted) return
    const before = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    // Окно без выхода: фокус внутри и по Tab не уходит на страницу под ним.
    const focusables = () =>
      Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('a[href], button:not([disabled])') ?? [])
    const first = window.setTimeout(() => focusables()[0]?.focus(), ANIM_MS)
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Tab') return
      const items = focusables()
      if (items.length === 0) return
      const active = document.activeElement as HTMLElement | null
      const inside = active ? dialogRef.current?.contains(active) : false
      if (e.shiftKey && (!inside || active === items[0])) {
        e.preventDefault()
        items[items.length - 1].focus()
      } else if (!e.shiftKey && (!inside || active === items[items.length - 1])) {
        e.preventDefault()
        items[0].focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      window.clearTimeout(first)
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = before
    }
  }, [mounted])

  if (typeof document === 'undefined' || !mounted) return null

  const url = urlRef.current

  async function handleCheck() {
    if (checking) return
    setChecking(true)
    setMessage(null)
    try {
      const res = await api.channelGateCheck()
      if (res.subscribed) {
        hapticSuccess()
        setPhase('done')
        window.setTimeout(() => {
          setChannelGate(null)
          // Всё, что успело получить 403, перечитываем уже без блокировки.
          void queryClient.invalidateQueries()
        }, DONE_MS)
      } else if (res.too_fast) {
        // Нажали слишком часто — ответа ещё нет, держим прежнюю подсказку.
        setMessage((m) => m ?? 'notYet')
      } else {
        hapticError()
        setMessage('notYet')
      }
    } catch {
      hapticError()
      setMessage('error')
    } finally {
      setChecking(false)
    }
  }

  function handleOpenChannel(e: React.MouseEvent<HTMLAnchorElement>) {
    // В мини-приложении Telegram ссылка на канал открывается самим Telegram, а не браузером.
    const tg = window.Telegram?.WebApp
    if (url && typeof tg?.openTelegramLink === 'function') {
      e.preventDefault()
      tg.openTelegramLink(url)
    }
  }

  const done = phase === 'done'

  return createPortal(
    <div
      className={cn(
        'fixed inset-0 z-[2100] flex items-end justify-center bg-black/60 backdrop-blur-sm transition-opacity duration-[250ms] ease-out sm:items-center sm:px-4',
        visible ? 'opacity-100' : 'opacity-0',
      )}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={t('channelGate.title')}
        className={cn(
          'home-sheet flex w-full flex-col items-stretch gap-3.5 border border-border bg-background px-5 pb-[max(1.5rem,calc(1rem+var(--cabinet-sheet-safe-bottom)))] pt-6 text-center shadow-[0_-20px_50px_rgba(0,0,0,0.5)] transition-[opacity,transform] duration-[250ms] ease-out',
          'rounded-t-[28px] sm:max-w-[440px] sm:rounded-[26px] sm:px-7 sm:pb-6 sm:shadow-[0_30px_80px_rgba(0,0,0,0.6)]',
          visible
            ? 'translate-y-0 opacity-100 sm:scale-100'
            : 'translate-y-full opacity-0 sm:translate-y-0 sm:scale-[0.97]',
        )}
      >
        <span
          className={cn(
            'mx-auto flex size-16 items-center justify-center rounded-[20px]',
            done ? 'bg-emerald-500/15 text-emerald-500' : 'bg-primary/15 text-primary',
          )}
          aria-hidden
        >
          {done ? <Check className="size-8" strokeWidth={2.2} /> : <Megaphone className="size-8" />}
        </span>

        <p className="font-heading text-[22px] font-extrabold leading-tight">
          {done ? t('channelGate.doneTitle') : t('channelGate.title')}
        </p>
        <p className="text-[14.5px] leading-relaxed text-muted-foreground">
          {done ? t('channelGate.doneText') : t('channelGate.text')}
        </p>

        {!done && (
          <>
            {message && (
              <p
                role="alert"
                className="rounded-2xl border border-amber-500/40 bg-amber-500/10 px-3 py-2.5 text-left text-[13.5px] text-amber-700 dark:text-amber-300"
              >
                {t(message === 'notYet' ? 'channelGate.notYet' : 'channelGate.error')}
              </p>
            )}
            {url && (
              <Button asChild size="lg" className="h-12 w-full rounded-full text-[15.5px] font-bold">
                <a href={url} target="_blank" rel="noopener noreferrer" onClick={handleOpenChannel}>
                  <SquareArrowOutUpRight />
                  {t('channelGate.open')}
                </a>
              </Button>
            )}
            <Button
              type="button"
              variant="outline"
              size="lg"
              loading={checking}
              onClick={() => void handleCheck()}
              className="h-12 w-full rounded-full border-emerald-500/45 bg-emerald-500/15 text-[15.5px] font-bold text-emerald-700 hover:border-emerald-500/60 hover:bg-emerald-500/25 hover:text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300 dark:hover:bg-emerald-500/25 dark:hover:text-emerald-200"
            >
              {!checking && <Check />}
              {checking ? t('channelGate.checking') : t('channelGate.check')}
            </Button>
          </>
        )}
      </div>
    </div>,
    document.body,
  )
}
