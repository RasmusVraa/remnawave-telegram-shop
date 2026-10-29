import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import {
  BROADCAST_LINK_KEYS,
  broadcastLinkLabelKey,
  type BroadcastLinkKey,
} from '../utils/broadcastLinks'
import { EXPANDABLE_CLASS, SPOILER_CLASS, renderTelegramHtml } from '../utils/telegramHtml'
import type { AdminBroadcastMediaKind } from '@/lib/types/admin'

interface BroadcastButtons {
  buy: boolean
  connect: boolean
  promo: boolean
  main_menu: boolean
  share_referral: boolean
  links: BroadcastLinkKey[]
}

interface AdminBroadcastMessagePreviewProps {
  /** Разметка Telegram — ровно та, что уйдёт в Bot API. */
  html: string
  mediaUrl?: string | null
  mediaKind?: AdminBroadcastMediaKind
  /**
   * Текст на месте вложения, когда файл есть, а показать его нечем: черновик
   * после перезагрузки, запись в истории. Файл лежит в Telegram, картинки у
   * кабинета нет.
   */
  mediaPlaceholder?: string
  buttons: BroadcastButtons
}

/**
 * Пузырь сообщения так, как его увидит получатель.
 *
 * Разметку показываем разобранной: превью с «<b>» на экране не отвечало на
 * вопрос, ради которого его открывают. Спойлер закрыт и раскрывается кликом,
 * сворачиваемая цитата подрезана — как в самом Telegram.
 */
export function AdminBroadcastMessagePreview({
  html,
  mediaUrl,
  mediaKind,
  mediaPlaceholder,
  buttons,
}: AdminBroadcastMessagePreviewProps) {
  const { t } = useTranslation()
  const bodyRef = useRef<HTMLDivElement>(null)

  // Порядок повторяет BuildReplyMarkup: купить, мой VPN, разделы кабинета, промокод,
  // пригласить друга, меню.
  const inlineButtons = [
    buttons.buy ? t('admin.broadcast.buttons.buy') : null,
    buttons.connect ? t('admin.broadcast.buttons.connect') : null,
    ...BROADCAST_LINK_KEYS.filter((key) => buttons.links.includes(key)).map((key) =>
      t(broadcastLinkLabelKey(key)),
    ),
    buttons.promo ? t('admin.broadcast.buttons.promo') : null,
    buttons.share_referral ? t('admin.broadcast.buttons.shareReferral') : null,
    buttons.main_menu ? t('admin.broadcast.buttons.mainMenu') : null,
  ].filter(Boolean) as string[]

  /*
   * Спойлер и разворот цитаты вешаем делегированием на контейнер: содержимое
   * приходит строкой и переписывается на каждый ввод, так что слушатели на
   * самих элементах пришлось бы навешивать заново после каждой правки.
   */
  useEffect(() => {
    const node = bodyRef.current
    if (!node) return
    const onClick = (e: MouseEvent) => {
      const target = e.target as HTMLElement
      const spoiler = target.closest(`.${SPOILER_CLASS}`)
      if (spoiler) spoiler.classList.toggle('is-open')
      const quote = target.closest(`blockquote.${EXPANDABLE_CLASS}`)
      if (quote) quote.classList.toggle('is-open')
    }
    node.addEventListener('click', onClick)
    return () => node.removeEventListener('click', onClick)
  }, [])

  const caption = html.trim()
  const hasMedia = Boolean(mediaUrl || mediaKind)

  return (
    // Оба вида Telegram: светлый чат в светлой теме, тёмный в тёмной.
    // Цвета взяты у самого Telegram и намеренно не из наших токенов — это
    // изображение чужого интерфейса, декор-темы к нему отношения не имеют.
    <div className="overflow-hidden rounded-2xl border border-black/[0.06] bg-white text-[#0f0f0f] dark:border-white/[0.07] dark:bg-[#17212b] dark:text-[#f2f5f8]">
      {mediaUrl ? (
        mediaKind === 'video' ? (
          <video src={mediaUrl} className="aspect-video w-full object-cover" controls muted playsInline />
        ) : (
          <img src={mediaUrl} alt="" className="aspect-video w-full object-cover" />
        )
      ) : (
        // Пустое место с подписью честнее, чем молча пропавшее фото.
        mediaKind && (
          <div className="grid aspect-video w-full place-items-center bg-black/[0.04] px-6 text-center text-xs text-black/50 dark:bg-white/[0.04] dark:text-white/50">
            {mediaPlaceholder}
          </div>
        )
      )}

      {caption ? (
        <div
          ref={bodyRef}
          className="cabinet-tg-text cabinet-tg-text--preview px-3 py-2.5 text-[15px] leading-snug"
          dangerouslySetInnerHTML={{ __html: renderTelegramHtml(caption) }}
        />
      ) : (
        <p className="px-3 py-2.5 text-sm text-black/40 dark:text-white/45">
          {hasMedia ? t('admin.broadcast.previewNoText') : t('admin.broadcast.previewEmpty')}
        </p>
      )}

      {inlineButtons.length > 0 && (
        <div className="space-y-1.5 border-t border-black/[0.08] p-2 dark:border-white/10">
          {inlineButtons.map((label) => (
            <div
              key={label}
              className="rounded-lg bg-black/[0.05] px-3 py-2 text-center text-sm font-medium text-[#168acd] dark:bg-[#2b5278]/80 dark:text-[#6ab3f3]"
            >
              {label}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
