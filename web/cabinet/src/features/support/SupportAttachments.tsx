import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Download, FileImage, ImageOff, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { SupportAttachmentDTO } from '@/lib/api'
import { useAttachmentUrl } from '@/features/support/useAttachmentUrl'

/** Выше окна чата (MODAL_Z = 2500): просмотр открывается поверх него. */
const VIEWER_Z = 2600

/** Соотношение сторон одиночной картинки — по размерам из Telegram, без крайностей. */
function aspectOf(a: SupportAttachmentDTO): number {
  if (!a.width || !a.height) return 4 / 3
  return Math.min(2, Math.max(0.5, a.width / a.height))
}

type TileProps = {
  attachment: SupportAttachmentDTO
  single: boolean
  onOpen: (a: SupportAttachmentDTO) => void
}

function AttachmentTile({ attachment, single, onOpen }: TileProps) {
  const { t } = useTranslation()
  const { url, isError } = useAttachmentUrl(attachment.id)
  // HEIC и TIFF не всякий браузер показывает — тогда карточка файла со скачиванием.
  const [undisplayable, setUndisplayable] = useState(false)
  const shape = single ? 'w-60 max-w-full' : 'aspect-square w-full'
  const style = single ? { aspectRatio: String(aspectOf(attachment)) } : undefined

  if (isError || undisplayable) {
    // Карточка файла: своя высота, а не пропорции картинки. Затемнение, а не светлая
    // подложка — на зелёном пузыре клиента белый текст иначе не читается.
    return (
      <div
        className={cn(
          'flex flex-col items-center justify-center gap-1.5 rounded-xl bg-black/15 px-3 py-4 text-center dark:bg-black/25',
          single ? 'w-60 max-w-full' : 'aspect-square w-full',
        )}
      >
        {isError ? <ImageOff className="size-5 opacity-70" aria-hidden /> : <FileImage className="size-5 opacity-70" aria-hidden />}
        <span className="line-clamp-2 break-all text-[11px] opacity-80">
          {isError ? t('supportChat.imageUnavailable') : attachment.name || t('supportChat.photo')}
        </span>
        {url && !isError ? (
          <a href={url} download={attachment.name || 'image'} className="inline-flex items-center gap-1 text-[11px] font-medium underline underline-offset-2">
            <Download className="size-3" aria-hidden />
            {t('supportChat.download')}
          </a>
        ) : null}
      </div>
    )
  }

  return (
    <button
      type="button"
      className={cn('relative block overflow-hidden rounded-xl bg-black/10 dark:bg-black/25', shape, !url && 'animate-pulse')}
      style={style}
      onClick={() => url && onOpen(attachment)}
      aria-label={t('supportChat.openImage')}
    >
      {url ? (
        <img
          src={url}
          alt=""
          className="size-full object-cover"
          decoding="async"
          draggable={false}
          onError={() => setUndisplayable(true)}
        />
      ) : null}
    </button>
  )
}

/** Картинки в пузыре сообщения: одна — по своим пропорциям, несколько — сеткой в две колонки. */
export function SupportAttachments({
  attachments,
  onOpen,
}: {
  attachments: SupportAttachmentDTO[]
  onOpen: (a: SupportAttachmentDTO) => void
}) {
  if (attachments.length === 0) return null
  const single = attachments.length === 1
  return (
    <div className={cn(single ? 'block' : 'grid w-60 max-w-full grid-cols-2 gap-1')}>
      {attachments.map((a) => (
        <AttachmentTile key={a.id} attachment={a} single={single} onOpen={onOpen} />
      ))}
    </div>
  )
}

/** Полноэкранный просмотр картинки: Esc, крестик или тап по фону закрывают. */
export function SupportImageViewer({
  attachment,
  onClose,
}: {
  attachment: SupportAttachmentDTO
  onClose: () => void
}) {
  const { t } = useTranslation()
  const { url } = useAttachmentUrl(attachment.id)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      // Закрываем только просмотр, а не окно чата под ним.
      e.stopPropagation()
      onClose()
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onClose])

  if (typeof document === 'undefined') return null
  return createPortal(
    <div className="fixed inset-0 flex items-center justify-center p-3 md:p-8" style={{ zIndex: VIEWER_Z }} role="dialog" aria-modal="true">
      <button type="button" className="absolute inset-0 bg-neutral-950/90" aria-label={t('common.close')} onClick={onClose} />
      {url ? (
        <img src={url} alt="" className="relative max-h-full max-w-full rounded-lg object-contain shadow-2xl" />
      ) : null}
      <div className="absolute right-3 top-3 flex gap-2 pt-[env(safe-area-inset-top)]">
        {url ? (
          <Button asChild variant="secondary" size="icon" className="size-10 rounded-full">
            <a href={url} download={attachment.name || 'image'} aria-label={t('supportChat.download')}>
              <Download className="size-4" />
            </a>
          </Button>
        ) : null}
        <Button type="button" variant="secondary" size="icon" className="size-10 rounded-full" onClick={onClose} aria-label={t('common.close')}>
          <X className="size-4" />
        </Button>
      </div>
    </div>,
    document.body,
  )
}
