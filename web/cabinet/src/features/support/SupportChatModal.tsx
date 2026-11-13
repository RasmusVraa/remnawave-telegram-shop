import { useEffect, useMemo, useRef, useState, type DragEvent } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { FileImage, ImageUp, Loader2, Paperclip, Send, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useAuthBootstrap } from '@/hooks/useAuthBootstrap'
import { useSheetDragDismiss } from '@/hooks/useSheetDragDismiss'
import { cn } from '@/lib/utils'
import { ApiError, type SupportAttachmentDTO, type SupportMessageDTO } from '@/lib/api'
import { useErrorHaptic } from '@/lib/haptic'
import { useSupportChat } from '@/features/support/useSupportChat'
import { SupportAvatar } from '@/features/support/SupportAvatar'
import { SupportAttachments, SupportImageViewer } from '@/features/support/SupportAttachments'
import { ensureSupportChatAudio, playSupportReplyChime } from '@/features/support/supportChatAudio'
import {
  SUPPORT_IMAGE_ACCEPT,
  filesFromTransfer,
  isSupportImage,
  prepareSupportImage,
} from '@/features/support/supportImages'

const MODAL_Z = 2500

function inMessageBubbleClass(status?: SupportMessageDTO['delivery_status']): string {
  switch (status) {
    case 'sent':
      return 'bg-emerald-600 text-white dark:bg-emerald-600 dark:text-white'
    case 'failed':
      return 'bg-destructive text-destructive-foreground'
    default:
      return 'bg-primary text-primary-foreground'
  }
}

/** Точки «печатает…» как в Telegram: «.», «..», «...» и по кругу. */
function TypingDots() {
  const [count, setCount] = useState(1)
  useEffect(() => {
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
      setCount(3)
      return
    }
    const id = window.setInterval(() => setCount((c) => (c % 3) + 1), 400)
    return () => window.clearInterval(id)
  }, [])
  // Невидимый хвост держит ширину строки, чтобы текст рядом не дёргался.
  return (
    <span aria-hidden>
      {'.'.repeat(count)}
      <span className="invisible">{'.'.repeat(3 - count)}</span>
    </span>
  )
}

/** Файл, выбранный к отправке: превью — object URL, его надо освободить. */
type Draft = { key: string; file: File; url: string }

let draftSeq = 0

function toDraft(file: File): Draft {
  draftSeq += 1
  return { key: `d${draftSeq}`, file, url: URL.createObjectURL(file) }
}

function revokeDrafts(drafts: Draft[]) {
  for (const d of drafts) URL.revokeObjectURL(d.url)
}

/** Отказ сервера — понятной причиной: лимиты те же, что проверяем мы, но сервер мог их поменять. */
function sendErrorText(err: unknown, t: TFunction, maxFiles: number, maxMb: number): string {
  if (err instanceof ApiError) {
    if (err.status === 413) return t('supportChat.fileTooLarge', { mb: maxMb })
    if (err.status === 415) return t('supportChat.unsupportedType')
    if (err.status === 400 && err.body.includes('too many files')) return t('supportChat.tooManyFiles', { max: maxFiles })
  }
  return t('supportChat.sendError')
}

/** Миниатюра выбранного файла над полем ввода. HEIC браузер может не показать — тогда значок. */
function DraftThumb({ draft, onRemove }: { draft: Draft; onRemove: () => void }) {
  const { t } = useTranslation()
  const [broken, setBroken] = useState(false)
  return (
    <div className="relative size-[52px] shrink-0">
      {broken ? (
        <div className="flex size-full items-center justify-center rounded-[10px] bg-muted text-muted-foreground" title={draft.file.name}>
          <FileImage className="size-5" aria-hidden />
        </div>
      ) : (
        <img
          src={draft.url}
          alt=""
          className="size-full rounded-[10px] object-cover ring-1 ring-border"
          onError={() => setBroken(true)}
        />
      )}
      <button
        type="button"
        onClick={onRemove}
        aria-label={t('supportChat.removeAttachment')}
        className="absolute -right-1.5 -top-1.5 flex size-[18px] items-center justify-center rounded-full bg-foreground text-background shadow"
      >
        <X className="size-3" aria-hidden />
      </button>
    </div>
  )
}

type SupportChatModalProps = {
  open: boolean
  enabled: boolean
  onClose: () => void
}

function formatDayLabel(iso: string, todayLabel: string, yesterdayLabel: string): string {
  const d = new Date(iso)
  const now = new Date()
  const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startMsg = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const diffDays = Math.round((startToday.getTime() - startMsg.getTime()) / 86_400_000)
  if (diffDays === 0) return todayLabel
  if (diffDays === 1) return yesterdayLabel
  return d.toLocaleDateString()
}

function groupByDay(messages: SupportMessageDTO[], todayLabel: string, yesterdayLabel: string) {
  const groups: { day: string; items: SupportMessageDTO[] }[] = []
  for (const m of messages) {
    const day = formatDayLabel(m.created_at, todayLabel, yesterdayLabel)
    const last = groups[groups.length - 1]
    if (last?.day === day) last.items.push(m)
    else groups.push({ day, items: [m] })
  }
  return groups
}

/** Текст сообщения и его картинки: картинки сверху, подпись под ними. */
function MessageContent({ message, onOpen }: { message: SupportMessageDTO; onOpen: (a: SupportAttachmentDTO) => void }) {
  const atts = message.attachments ?? []
  if (atts.length === 0) return <>{message.text}</>
  // Подпись — шириной с картинки, а не растягивает пузырь.
  return (
    <div className="w-60 max-w-full">
      <SupportAttachments attachments={atts} onOpen={onOpen} />
      {message.text ? <div className="px-2 pb-1 pt-1.5">{message.text}</div> : null}
    </div>
  )
}

export function SupportChatModal({ open, enabled, onClose }: SupportChatModalProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [entered, setEntered] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)
  const lastOutMsgIdRef = useRef(0)
  const { conversation, sendMutation, markReadMutation } = useSupportChat(enabled, open)
  const bootstrap = useAuthBootstrap()
  const attachEnabled = Boolean(bootstrap.data?.support_attachments_enabled)
  const maxFiles = bootstrap.data?.support_attachments_max_files ?? 5
  const maxMb = bootstrap.data?.support_attachments_max_mb ?? 10
  const [drafts, setDrafts] = useState<Draft[]>([])
  const draftsRef = useRef<Draft[]>([])
  draftsRef.current = drafts
  // Сообщение с картинками уходит заметное время — до ответа сервера показываем его в ленте сразу.
  const [sending, setSending] = useState<{ text: string; drafts: Draft[] } | null>(null)
  const [attachError, setAttachError] = useState<string | null>(null)
  const [dragOver, setDragOver] = useState(false)
  const dragDepthRef = useRef(0)
  const [viewing, setViewing] = useState<SupportAttachmentDTO | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const drag = useSheetDragDismiss<HTMLDivElement, HTMLButtonElement>({
    open,
    onDismiss: onClose,
    desktopQuery: '(min-width: 768px)',
  })

  useEffect(() => {
    if (!open) {
      setEntered(false)
      return
    }
    const id = window.requestAnimationFrame(() => setEntered(true))
    return () => window.cancelAnimationFrame(id)
  }, [open])

  useEffect(() => {
    if (!open || typeof document === 'undefined') return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [open])

  useEffect(() => {
    if (!open || !enabled) return
    if ((conversation.data?.unread_count ?? 0) <= 0) return
    void markReadMutation.mutate()
  }, [open, enabled, conversation.data?.unread_count])

  useEffect(() => {
    if (!open) {
      lastOutMsgIdRef.current = 0
      return
    }
    const msgs = conversation.data?.messages ?? []
    const maxOutId = msgs.filter((m) => m.direction === 'out').reduce((max, m) => Math.max(max, m.id), 0)
    if (maxOutId === 0) {
      return
    }
    if (lastOutMsgIdRef.current === 0) {
      lastOutMsgIdRef.current = maxOutId
      return
    }
    if (maxOutId > lastOutMsgIdRef.current) {
      void ensureSupportChatAudio().then((ctx) => {
        if (ctx) playSupportReplyChime(ctx)
      })
    }
    lastOutMsgIdRef.current = maxOutId
  }, [open, conversation.data?.messages])

  useEffect(() => {
    if (!open) return
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [open, conversation.data?.messages.length, sending])

  // Превью живут, пока открыт кабинет; при уходе со страницы — освобождаем.
  useEffect(() => () => revokeDrafts(draftsRef.current), [])

  const messages = conversation.data?.messages ?? []
  const lastMessage = messages[messages.length - 1]
  const waitingForSupport =
    Boolean(lastMessage?.direction === 'in' && lastMessage.delivery_status === 'sent')
  const supportTyping = waitingForSupport && Boolean(conversation.data?.support_typing)

  const groups = useMemo(
    () => groupByDay(conversation.data?.messages ?? [], t('supportChat.today'), t('supportChat.yesterday')),
    [conversation.data?.messages, t],
  )

  async function addFiles(list: File[]) {
    if (!attachEnabled || list.length === 0) return
    setAttachError(null)
    const images = list.filter(isSupportImage)
    let problem: string | null = images.length < list.length ? t('supportChat.unsupportedType') : null
    const prepared = await Promise.all(images.map(prepareSupportImage))
    const fitting = prepared.filter((f) => f.size <= maxMb * 1024 * 1024)
    if (fitting.length < prepared.length) problem = t('supportChat.fileTooLarge', { mb: maxMb })
    const room = Math.max(0, maxFiles - draftsRef.current.length)
    if (fitting.length > room) problem = t('supportChat.tooManyFiles', { max: maxFiles })
    const added = fitting.slice(0, room).map(toDraft)
    if (added.length > 0) setDrafts((prev) => [...prev, ...added])
    setAttachError(problem)
  }

  function removeDraft(key: string) {
    setAttachError(null)
    setDrafts((prev) => {
      revokeDrafts(prev.filter((d) => d.key === key))
      return prev.filter((d) => d.key !== key)
    })
  }

  function openFilePicker() {
    if (drafts.length >= maxFiles) {
      setAttachError(t('supportChat.tooManyFiles', { max: maxFiles }))
      return
    }
    fileInputRef.current?.click()
  }

  function hasFiles(e: DragEvent) {
    return attachEnabled && Array.from(e.dataTransfer?.types ?? []).includes('Files')
  }

  async function handleSend() {
    const value = text.trim()
    const files = drafts
    if ((!value && files.length === 0) || sendMutation.isPending) return
    setText('')
    setDrafts([])
    setAttachError(null)
    if (files.length > 0) setSending({ text: value, drafts: files })
    try {
      await sendMutation.mutateAsync({ text: value, files: files.map((d) => d.file) })
      revokeDrafts(files)
    } catch {
      // Не ушло — возвращаем текст и картинки в поле, чтобы отправить ещё раз.
      setText(value)
      setDrafts(files)
    } finally {
      setSending(null)
    }
  }

  const errorText =
    attachError ?? (sendMutation.isError ? sendErrorText(sendMutation.error, t, maxFiles, maxMb) : null)
  useErrorHaptic(errorText)

  if (!open || !enabled) return null
  if (typeof document === 'undefined') return null

  return createPortal(
    <div
      className="fixed inset-0 flex flex-col justify-end md:items-center md:justify-center md:p-6"
      style={{ zIndex: MODAL_Z }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="support-chat-title"
    >
      <button
        ref={drag.backdropRef}
        type="button"
        className="absolute inset-0 bg-neutral-950/65 backdrop-blur-sm"
        aria-label={t('common.close')}
        onClick={onClose}
      />
      <div
        ref={drag.sheetRef}
        className={cn(
          'relative z-[1] flex w-full max-h-[min(88dvh,640px)] flex-col border border-border bg-card shadow-2xl transition-[transform,opacity] duration-300 ease-out md:max-h-[min(720px,90vh)] md:w-full md:max-w-md md:rounded-2xl',
          // У поля ввода свой отступ p-3, поэтому снизу добавляем только высоту
          // кнопок навигации Android, если окно лежит под ними.
          'rounded-t-3xl pb-[var(--cabinet-sheet-safe-bottom)] md:pb-0',
          entered ? 'translate-y-0 opacity-100 md:scale-100' : 'translate-y-full opacity-95 md:translate-y-2 md:scale-[0.97] md:opacity-0',
        )}
        onDragEnter={(e) => {
          if (!hasFiles(e)) return
          e.preventDefault()
          dragDepthRef.current += 1
          setDragOver(true)
        }}
        onDragOver={(e) => {
          if (hasFiles(e)) e.preventDefault()
        }}
        onDragLeave={() => {
          if (!dragOver) return
          dragDepthRef.current = Math.max(0, dragDepthRef.current - 1)
          if (dragDepthRef.current === 0) setDragOver(false)
        }}
        onDrop={(e) => {
          if (!hasFiles(e)) return
          e.preventDefault()
          dragDepthRef.current = 0
          setDragOver(false)
          void addFiles(filesFromTransfer(e.dataTransfer))
        }}
      >
        {dragOver ? (
          <div className="pointer-events-none absolute inset-2 z-10 flex flex-col items-center justify-center gap-1.5 rounded-2xl border-2 border-dashed border-primary bg-card/90 text-primary">
            <ImageUp className="size-6" aria-hidden />
            <p className="text-sm font-semibold">{t('supportChat.dropHere')}</p>
            <p className="text-[11px] text-muted-foreground">{t('supportChat.dropHint', { max: maxFiles, mb: maxMb })}</p>
          </div>
        ) : null}
        {/* Ручка шторки: за неё и за заголовок окно тянут вниз, чтобы закрыть. */}
        <div className="flex shrink-0 cursor-grab touch-none justify-center pb-0.5 pt-2.5 md:hidden" {...drag.handlers}>
          <span className="h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden />
        </div>
        <div
          className="flex touch-none items-center justify-between gap-2 border-b border-border px-4 pb-3 pt-1.5 md:touch-auto md:pt-3"
          {...drag.handlers}
        >
          <div className="min-w-0">
            <h2 id="support-chat-title" className="text-base font-semibold">
              {t('supportChat.title')}
            </h2>
            {supportTyping ? (
              <p className="text-xs text-primary" role="status">
                {t('supportChat.typing')}
                <TypingDots />
              </p>
            ) : waitingForSupport ? (
              <p className="text-xs text-muted-foreground">{t('supportChat.waitingHint')}</p>
            ) : null}
          </div>
          <Button type="button" variant="ghost" size="icon" className="size-9 shrink-0" onClick={onClose} aria-label={t('common.close')}>
            <X className="size-4" />
          </Button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
          {conversation.isLoading ? (
            <p className="py-8 text-center text-sm text-muted-foreground">{t('common.loading')}</p>
          ) : groups.length === 0 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">{t('supportChat.emptyHint')}</p>
          ) : (
            <div className="space-y-4">
              {groups.map((g) => (
                <div key={g.day} className="space-y-3">
                  <div className="flex items-center gap-2 px-1">
                    <div className="h-px flex-1 bg-border" />
                    <span className="text-[11px] text-muted-foreground">{g.day}</span>
                    <div className="h-px flex-1 bg-border" />
                  </div>
                  {g.items.map((m) =>
                    m.direction === 'out' ? (
                      <div key={m.id} className="flex gap-2">
                        <SupportAvatar />
                        <div className="min-w-0 max-w-[85%]">
                          <div
                            className={cn(
                              'whitespace-pre-wrap break-words rounded-2xl rounded-tl-md border border-border/80 bg-muted/50 text-sm shadow-sm',
                              m.attachments?.length ? 'p-1' : 'px-3 py-2',
                            )}
                          >
                            <MessageContent message={m} onOpen={setViewing} />
                          </div>
                          <p className="mt-1 text-[11px] text-muted-foreground">{t('supportChat.supportLabel')}</p>
                        </div>
                      </div>
                    ) : (
                      <div key={m.id} className="flex flex-col items-end gap-1">
                        <div
                          className={cn(
                            // pre-wrap в обоих пузырях: переносы строк — часть сообщения (списки, шаги
                            // в ответах поддержки и ИИ), без него текст склеивался в одну строку.
                            'max-w-[85%] whitespace-pre-wrap break-words rounded-2xl rounded-tr-md text-sm shadow-sm',
                            m.attachments?.length ? 'p-1' : 'px-3 py-2',
                            inMessageBubbleClass(m.delivery_status),
                          )}
                        >
                          <MessageContent message={m} onOpen={setViewing} />
                        </div>
                        {m.delivery_status === 'failed' ? (
                          <p className="text-[11px] text-destructive">{t('supportChat.notDelivered')}</p>
                        ) : null}
                      </div>
                    ),
                  )}
                </div>
              ))}
            </div>
          )}
          {sending ? (
            <div className="mt-3 flex flex-col items-end gap-1" aria-live="polite">
              <div className={cn('max-w-[85%] whitespace-pre-wrap break-words rounded-2xl rounded-tr-md p-1 text-sm opacity-70 shadow-sm', inMessageBubbleClass('pending'))}>
                <div className={cn(sending.drafts.length === 1 ? 'block' : 'grid w-60 max-w-full grid-cols-2 gap-1')}>
                  {sending.drafts.map((d) => (
                    <img
                      key={d.key}
                      src={d.url}
                      alt=""
                      className={cn('rounded-xl bg-background/40 object-cover', sending.drafts.length === 1 ? 'max-h-60 w-60 max-w-full' : 'aspect-square w-full')}
                    />
                  ))}
                </div>
                {sending.text ? <div className="px-2 pb-1 pt-1.5">{sending.text}</div> : null}
              </div>
              <p className="flex items-center gap-1 text-[11px] text-muted-foreground">
                <Loader2 className="size-3 animate-spin" aria-hidden />
                {t('supportChat.sending')}
              </p>
            </div>
          ) : null}
          <div ref={bottomRef} />
        </div>

        <div className="border-t border-border p-3">
          {errorText ? <p className="mb-2 text-xs text-destructive">{errorText}</p> : null}
          <div className="rounded-xl border border-primary/35 bg-background px-2 py-2 focus-within:ring-2 focus-within:ring-primary/30">
            {drafts.length > 0 ? (
              <div className="flex flex-wrap items-center gap-2 px-0.5 pb-2.5 pt-1.5">
                {drafts.map((d) => (
                  <DraftThumb key={d.key} draft={d} onRemove={() => removeDraft(d.key)} />
                ))}
                <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">
                  {t('supportChat.attachCounter', { count: drafts.length, max: maxFiles })}
                </span>
              </div>
            ) : null}
            {/* Поле фиксированной высоты (две строки, без растягивания) — кнопки по центру ряда. */}
            <div className="flex items-center gap-2">
              {attachEnabled ? (
                <>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className={cn('size-9 shrink-0', drafts.length > 0 ? 'text-primary' : 'text-muted-foreground')}
                    onClick={openFilePicker}
                    aria-label={t('supportChat.attach')}
                    title={t('supportChat.attach')}
                  >
                    <Paperclip className="size-4" />
                  </Button>
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept={SUPPORT_IMAGE_ACCEPT}
                    multiple
                    className="hidden"
                    onChange={(e) => {
                      void addFiles(Array.from(e.target.files ?? []))
                      e.target.value = ''
                    }}
                  />
                </>
              ) : null}
              <textarea
                value={text}
                onChange={(e) => setText(e.target.value)}
                rows={2}
                placeholder={t('supportChat.inputPlaceholder')}
                className="min-h-[44px] max-h-32 flex-1 resize-none bg-transparent px-1 py-1 text-sm outline-none"
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    void handleSend()
                  }
                }}
                onPaste={(e) => {
                  // Скриншот из буфера (Win+Shift+S, Cmd+Ctrl+Shift+4) — во вложения, а не текстом.
                  const files = attachEnabled ? filesFromTransfer(e.clipboardData) : []
                  if (files.length === 0) return
                  e.preventDefault()
                  void addFiles(files)
                }}
              />
              <Button
                type="button"
                size="icon"
                className="size-9 shrink-0"
                loading={sendMutation.isPending}
                disabled={!text.trim() && drafts.length === 0}
                onClick={() => void handleSend()}
                aria-label={t('supportChat.send')}
              >
                <Send className="size-4" />
              </Button>
            </div>
          </div>
        </div>
        {viewing ? <SupportImageViewer attachment={viewing} onClose={() => setViewing(null)} /> : null}
      </div>
    </div>,
    document.body,
  )
}
