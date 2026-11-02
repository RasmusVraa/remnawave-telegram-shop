import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { BellOff, BellRing, CheckCheck, X } from 'lucide-react'

import { ResponsiveSheet } from '@/components/ResponsiveSheet'
import { Button } from '@/components/ui/button'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import type { NotificationDTO } from '@/lib/api'
import { enablePush, getPushState, syncPushSubscription, type PushState } from '@/lib/push'
import { cn } from '@/lib/utils'
import { renderTelegramHtml } from '@/features/admin/utils/telegramHtml'
import { NOTIFICATION_BUTTONS, formatWhen, notificationCategory, splitTitle, stripLeadingEmoji } from './notificationMeta'
import { UnreadBadge } from './UnreadBadge'
import { FEED_KEY, useNotificationsFeed, useNotificationsSheetState, useReadQueue, useUnreadNotifications } from './useNotifications'

/** Есть мышь — читаем по наведению; палец — по тому, что карточку видно. */
function canHover(): boolean {
  return typeof window !== 'undefined' && window.matchMedia?.('(hover: hover) and (pointer: fine)').matches
}

const HOVER_READ_MS = 700
const VISIBLE_READ_MS = 1000
/** Появление и уход окна на ПК; столько же ждёт и сброс вида карточек при закрытии. */
const POPOVER_MS = 200

/** «Уведомления» и число непрочитанных рядом, если они есть. */
export function NotificationsTitle() {
  const { t } = useTranslation()
  const { data } = useUnreadNotifications()
  const count = data?.total ?? 0
  return (
    <span className="inline-flex items-center gap-2">
      {t('notifications.title')}
      {count > 0 && <UnreadBadge count={count} important={(data?.important ?? 0) > 0} className="h-[20px] min-w-[20px] px-[6px]" />}
    </span>
  )
}

/** «Прочитать все» значком в шапке окна — рядом с крестиком. */
function ReadAllButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation()
  const { data } = useUnreadNotifications()
  if (!data?.total) return null
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={t('notifications.readAll')}
      title={t('notifications.readAll')}
      className="inline-flex size-9 shrink-0 items-center justify-center rounded-[10px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      <CheckCheck size={17} />
    </button>
  )
}

/**
 * «Прочитать все» из шапки окна: шапку рисует ResponsiveSheet или
 * NotificationsPopover, а очередь отметок живёт в ленте — лента кладёт сюда
 * свой обработчик.
 */
type ReadAllRef = React.MutableRefObject<(() => void) | null>

/** Телефон: шторка снизу. ПК — выпадающее окно от колокольчика (NotificationsPopover). */
export function NotificationsSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const readAll: ReadAllRef = useRef(null)
  return (
    <ResponsiveSheet
      open={open}
      onClose={onClose}
      title={<NotificationsTitle />}
      actions={<ReadAllButton onClick={() => readAll.current?.()} />}
    >
      <NotificationsBody open={open} readAll={readAll} />
    </ResponsiveSheet>
  )
}

/**
 * ПК: окно под колокольчиком, высотой по содержимому, но не выше экрана —
 * длинная лента прокручивается внутри. Закрывается кликом мимо и Esc.
 *
 * Выезжает из колокольчика: растёт от точки под ним и проявляется, уходит
 * обратно туда же. Тот же приём, что у ResponsiveSheet: mounted держит
 * разметку, пока идёт уход, visible переключает классы перехода.
 */
export function NotificationsPopover({
  open,
  onClose,
  anchorRef,
}: {
  open: boolean
  onClose: () => void
  anchorRef: React.RefObject<HTMLElement | null>
}) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDivElement>(null)
  const readAll: ReadAllRef = useRef(null)
  const [mounted, setMounted] = useState(open)
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (open) {
      setMounted(true)
      // Два кадра: в первом окно только вставляется, и без второго браузер
      // сразу посчитал бы конечные стили — появления не было бы.
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
    const id = window.setTimeout(() => setMounted(false), POPOVER_MS + 30)
    return () => window.clearTimeout(id)
  }, [open])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node
      if (ref.current?.contains(target) || anchorRef.current?.contains(target)) return
      // Клик по порталу (подтверждения, тосты) окно не закрывает.
      if ((target as HTMLElement).closest?.('[role="dialog"]')) return
      onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose, anchorRef])

  if (!mounted) return null
  return (
    <div
      ref={ref}
      role="dialog"
      aria-label={t('notifications.title')}
      className={cn(
        // Точка роста — под колокольчиком: окно прижато правым краем к его обёртке.
        'absolute right-0 top-full z-[500] mt-2 flex max-h-[min(640px,calc(100dvh-6rem))] w-[420px] origin-[calc(100%-1.25rem)_0] flex-col overflow-hidden rounded-[20px] border border-border bg-card shadow-[0_24px_60px_rgba(0,0,0,0.55)] transition-[opacity,transform] duration-200 ease-out',
        visible ? 'translate-y-0 scale-100 opacity-100' : 'pointer-events-none -translate-y-1.5 scale-[0.92] opacity-0',
      )}
    >
      <div className="flex shrink-0 items-center gap-1 py-2.5 pl-4 pr-2.5">
        <p className="flex-1 font-heading text-[17px] font-bold">
          <NotificationsTitle />
        </p>
        <ReadAllButton onClick={() => readAll.current?.()} />
        <button
          type="button"
          onClick={onClose}
          aria-label={t('common.close')}
          className="inline-flex size-9 items-center justify-center rounded-[10px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <X size={17} />
        </button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto overscroll-contain px-3 pb-3">
        <NotificationsBody open={open} readAll={readAll} />
      </div>
    </div>
  )
}

function NotificationsBody({ open, readAll }: { open: boolean; readAll: ReadAllRef }) {
  const { t, i18n } = useTranslation()
  const qc = useQueryClient()
  const feed = useNotificationsFeed(open)
  const { markRead, markAllRead, remove, flush, readIds } = useReadQueue()
  const { leaveTo } = useNotificationsSheetState()

  const items = useMemo(() => (feed.data?.pages ?? []).flatMap((p) => p.items), [feed.data])

  // Непрочитанные на момент открытия так и выглядят, пока окно открыто:
  // отметка «прочитано» уходит на сервер и снимает счётчик, но карточка не
  // бледнеет у человека на глазах — через секунду после открытия он её ещё
  // читает. Бледными становятся при следующем открытии.
  const [fresh, setFresh] = useState<Set<number>>(() => new Set())
  useEffect(() => {
    setFresh((prev) => {
      let next: Set<number> | null = null
      for (const it of items) if (!it.read && !prev.has(it.id)) (next ??= new Set(prev)).add(it.id)
      return next ?? prev
    })
  }, [items])

  // Закрыли — отправить отметки и при следующем открытии перечитать ленту:
  // прочитанные важные уйдут из закрепления.
  // Вид карточек сбрасываем после ухода окна, иначе они бледнели бы на глазах.
  useEffect(() => {
    if (open) return
    void flush()
    void qc.invalidateQueries({ queryKey: FEED_KEY })
    const id = window.setTimeout(() => setFresh(new Set()), POPOVER_MS + 100)
    return () => window.clearTimeout(id)
  }, [open, flush, qc])

  // «Прочитать все» — нажали сами, тут бледнеть можно сразу.
  useEffect(() => {
    readAll.current = () => {
      setFresh(new Set())
      void markAllRead()
    }
    return () => {
      readAll.current = null
    }
  }, [readAll, markAllRead])

  const unreadNow = (it: NotificationDTO) => !it.read || fresh.has(it.id)
  // Важное одно и стоит первым, пока не прочитано, — сервер ставит его в начало.
  const ordered = [...items.filter((it) => it.important && unreadNow(it)), ...items.filter((it) => !(it.important && unreadNow(it)))]

  const lang = i18n.language?.startsWith('en') ? 'en-US' : 'ru-RU'
  const today = t('notifications.today')
  const yesterday = t('notifications.yesterday')

  return (
    <>
      {feed.isLoading ? (
        <div className="space-y-1.5" aria-hidden>
          {[0, 1, 2].map((i) => (
            <div key={i} className="h-[96px] animate-pulse rounded-2xl bg-muted/50" />
          ))}
        </div>
      ) : feed.isError ? (
        <p className="py-8 text-center text-sm text-muted-foreground">{t('errors.unknown')}</p>
      ) : ordered.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-10 text-center">
          <BellRing className="size-8 text-muted-foreground/60" strokeWidth={1.5} />
          <p className="max-w-[280px] text-sm text-muted-foreground">{t('notifications.empty')}</p>
        </div>
      ) : (
        <div className="space-y-1.5">
          {ordered.map((it) => (
            <NotificationItem
              key={it.id}
              item={it}
              unread={unreadNow(it)}
              marked={it.read || readIds.has(it.id)}
              onRead={markRead}
              onRemove={remove}
              onNavigate={leaveTo}
              when={formatWhen(it.created_at, lang, today, yesterday)}
            />
          ))}
          {feed.hasNextPage && (
            <Button
              type="button"
              variant="outline"
              className="w-full"
              loading={feed.isFetchingNextPage}
              onClick={() => void feed.fetchNextPage()}
            >
              {t('notifications.more')}
            </Button>
          )}
        </div>
      )}

      <PushCard open={open} />
    </>
  )
}

/**
 * Карточка: строка «значок · категория … · ✕», заголовок и текст на всю
 * ширину, внизу кнопки слева и дата справа. Длинный текст сворачивается до
 * трёх строк.
 *
 * unread — как карточка выглядит (см. fresh в ленте), marked — ушла ли уже
 * отметка «прочитано»: по ней перестаём следить за видимостью и наведением.
 */
function NotificationItem({
  item,
  unread,
  marked,
  onRead,
  onRemove,
  onNavigate,
  when,
}: {
  item: NotificationDTO
  unread: boolean
  marked: boolean
  onRead: (id: number) => void
  onRemove: (id: number) => void
  onNavigate: (to: string) => void
  when: string
}) {
  const { t } = useTranslation()
  const { data: bootstrap } = useAuthBootstrap()
  const ref = useRef<HTMLElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
  const hoverTimer = useRef<number | null>(null)
  const [expanded, setExpanded] = useState(false)
  const [clamped, setClamped] = useState(false)
  const cat = notificationCategory(item)
  const Icon = cat.icon
  const { title, body } = useMemo(() => splitTitle(item), [item])

  // Телефон: прочитано, когда карточка видна больше чем наполовину около секунды.
  useEffect(() => {
    if (marked || canHover()) return
    const el = ref.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    let timer: number | null = null
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.intersectionRatio >= 0.6) {
          if (timer === null) timer = window.setTimeout(() => onRead(item.id), VISIBLE_READ_MS)
        } else if (timer !== null) {
          window.clearTimeout(timer)
          timer = null
        }
      },
      { threshold: [0, 0.6, 1] },
    )
    io.observe(el)
    return () => {
      io.disconnect()
      if (timer !== null) window.clearTimeout(timer)
    }
  }, [marked, item.id, onRead])

  // ПК: прочитано, если мышь задержалась на карточке, — беглый проход курсором не считается.
  const onEnter = () => {
    if (marked || !canHover()) return
    hoverTimer.current = window.setTimeout(() => onRead(item.id), HOVER_READ_MS)
  }
  const onLeave = () => {
    if (hoverTimer.current !== null) window.clearTimeout(hoverTimer.current)
    hoverTimer.current = null
  }
  useEffect(() => onLeave, [])

  // «Показать полностью» — только если текст правда не влез в три строки.
  useLayoutEffect(() => {
    const el = bodyRef.current
    if (!el || expanded) return
    const check = () => setClamped(el.scrollHeight > el.clientHeight + 1)
    check()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(check)
    ro.observe(el)
    return () => ro.disconnect()
  }, [body, expanded])

  const buttons = item.buttons.filter((b) => {
    if (!NOTIFICATION_BUTTONS[b.key]) return false
    if (b.key === 'fortune' && bootstrap?.fortune_nav_visible === false) return false
    if (b.key === 'partner' && bootstrap?.partner_nav_visible === false) return false
    return true
  })

  return (
    <article
      ref={ref}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      className={cn(
        'rounded-2xl border px-3 pb-2.5 pt-2 transition-[opacity,background-color,border-color] duration-300',
        // Прочитанное бледнеет целиком — значок, текст, кнопки и дата.
        !unread && 'opacity-60',
        !unread
          ? 'border-border/60 bg-transparent'
          : item.important
            ? 'border-red-500/40 bg-gradient-to-b from-red-500/[0.13] to-red-500/[0.04]'
            : 'border-border bg-secondary/60',
      )}
    >
      <div className="flex h-7 items-center gap-2">
        <span className={cn('inline-flex size-6 shrink-0 items-center justify-center rounded-lg', cat.tint, cat.tone)}>
          <Icon size={14} />
        </span>
        <span className={cn('min-w-0 flex-1 truncate text-[12.5px] font-semibold', cat.tone)}>{t(cat.labelKey)}</span>
        {unread && <span className={cn('size-2 shrink-0 rounded-full bg-current', cat.tone)} aria-label={t('notifications.unread')} />}
        <button
          type="button"
          onClick={() => onRemove(item.id)}
          aria-label={t('notifications.remove')}
          title={t('notifications.remove')}
          className="-mr-1.5 inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <X size={15} />
        </button>
      </div>

      {title && <p className="mt-1 text-sm font-semibold leading-snug">{title}</p>}
      {body && (
        <div
          ref={bodyRef}
          className={cn(
            'break-words text-[13px] leading-[1.45] [&_a]:text-primary [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-2 [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_pre]:whitespace-pre-wrap',
            title ? 'mt-0.5' : 'mt-1',
            title ? 'text-muted-foreground' : 'text-foreground/90',
            !expanded && 'line-clamp-3',
          )}
          // Тело — Telegram HTML; renderTelegramHtml экранирует всё и пропускает только белый список тегов.
          dangerouslySetInnerHTML={{ __html: renderTelegramHtml(body) }}
        />
      )}
      {(clamped || expanded) && (
        <button
          type="button"
          onClick={() => {
            setExpanded((v) => !v)
            onRead(item.id)
          }}
          className="mt-0.5 text-[13px] font-semibold text-primary transition-opacity hover:opacity-80"
        >
          {expanded ? t('notifications.showLess') : t('notifications.showMore')}
        </button>
      )}

      <div className={cn('flex items-end gap-2', buttons.length > 0 ? 'mt-2' : 'mt-0.5')}>
        {buttons.length > 0 && (
          <div className="flex min-w-0 flex-wrap gap-1.5">
            {buttons.map((b, i) => {
              const meta = NOTIFICATION_BUTTONS[b.key]
              const BtnIcon = meta.icon
              return (
                <Link
                  key={b.key}
                  to={meta.route}
                  onClick={(e) => {
                    // Переходим сами: лента закрывается вместе с переходом (см. leaveTo).
                    e.preventDefault()
                    onRead(item.id)
                    onNavigate(meta.route)
                  }}
                  className={cn(
                    'inline-flex h-8 items-center gap-1.5 rounded-[10px] px-3 text-[13px] font-semibold transition-colors',
                    item.important && unread && i === 0
                      ? 'bg-red-500 text-white hover:bg-red-500/90'
                      : 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
                  )}
                >
                  <BtnIcon size={15} className="shrink-0" />
                  {stripLeadingEmoji(t(meta.labelKey))}
                </Link>
              )
            })}
          </div>
        )}
        {/* Высота строки как у кнопки — дата стоит на одной линии с подписями кнопок. */}
        <span
          className={cn(
            'ml-auto shrink-0 whitespace-nowrap text-[11.5px] tabular-nums text-muted-foreground',
            buttons.length > 0 ? 'leading-8' : 'leading-5',
          )}
        >
          {when}
        </span>
      </div>
    </article>
  )
}

/**
 * Push на этом устройстве: предложить включить или объяснить, почему нельзя.
 * Включён — ничего не показываем: выключить можно в настройках браузера.
 */
function PushCard({ open }: { open: boolean }) {
  const { t } = useTranslation()
  const [state, setState] = useState<PushState | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState<ReactNode>(null)

  useEffect(() => {
    if (!open) return
    let alive = true
    // Уведомления уже разрешены в браузере — сначала подписываемся сами,
    // потом показываем состояние: напоминание «Включить» тогда не нужно.
    void syncPushSubscription()
      .then(getPushState)
      .then((s) => alive && setState(s))
    return () => {
      alive = false
    }
  }, [open])

  if (!state || state === 'unsupported' || state === 'on') return null

  const enable = async () => {
    setBusy(true)
    setNote(null)
    try {
      const next = await enablePush()
      setState(next)
      // Браузер не показал запрос (тихий режим Chrome, закрыли окно) — подсказать, где разрешить.
      if (next === 'off') setNote(t('notifications.pushPromptBlocked'))
    } catch {
      setNote(t('notifications.pushFailed'))
    } finally {
      setBusy(false)
    }
  }

  if (state === 'denied' || state === 'ios-needs-install') {
    return (
      <p className="flex items-start gap-2 px-1 pt-1 text-xs text-muted-foreground">
        <BellOff size={14} className="mt-px shrink-0" />
        {state === 'denied' ? t('notifications.pushDenied') : t('notifications.pushIosInstall')}
      </p>
    )
  }
  return (
    <div className="flex items-center gap-3 rounded-2xl border border-dashed border-border px-3 py-2.5">
      <BellRing size={17} className="shrink-0 text-primary" />
      <div className="min-w-0 flex-1">
        <p className="text-[13px] font-semibold">{t('notifications.pushTitle')}</p>
        <p className={cn('text-xs', note ? 'text-amber-600 dark:text-amber-300' : 'text-muted-foreground')}>
          {note ?? t('notifications.pushHint')}
        </p>
      </div>
      <Button type="button" size="sm" loading={busy} onClick={() => void enable()}>
        {t('notifications.pushEnable')}
      </Button>
    </div>
  )
}
