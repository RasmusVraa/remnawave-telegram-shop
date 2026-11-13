/**
 * Изображения, которые клиент прикладывает в чате поддержки. Форматы — как у фото,
 * которые шлют боту в Telegram; окончательно формат проверяет сервер (по содержимому).
 */

const IMAGE_MIMES = new Set([
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/bmp',
  'image/gif',
  'image/heic',
  'image/heif',
  'image/avif',
  'image/tiff',
])

// Windows и часть Android отдают HEIC с пустым type — узнаём по расширению.
const IMAGE_EXTS = /\.(jpe?g|png|webp|bmp|gif|heic|heif|avif|tiff?)$/i

/** Для <input accept>: HEIC отдельно, иначе на Windows его не видно в диалоге. */
export const SUPPORT_IMAGE_ACCEPT = 'image/*,.heic,.heif'

export function isSupportImage(file: File): boolean {
  if (file.type) return IMAGE_MIMES.has(file.type.toLowerCase())
  return IMAGE_EXTS.test(file.name)
}

/** Картинки из буфера обмена или перетаскивания; остальное (тексты, папки) — мимо. */
export function filesFromTransfer(dt: DataTransfer | null): File[] {
  if (!dt) return []
  return Array.from(dt.files ?? []).filter((f) => f.type.startsWith('image/') || IMAGE_EXTS.test(f.name))
}

// Длинная сторона после уменьшения: Telegram всё равно хранит фото не крупнее.
const MAX_SIDE = 2560
// Мелкие файлы не трогаем: выигрыша нет, а скриншот с текстом теряет резкость.
const KEEP_BELOW_BYTES = 300 * 1024

function canvasBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, type, quality))
}

function renamed(name: string, ext: string): string {
  const stem = name.replace(/\.[^./\\]+$/, '') || 'image'
  return `${stem}.${ext}`
}

/**
 * Уменьшает фото перед отправкой: длинная сторона до 2560 px, JPEG (PNG-скриншоты —
 * WebP, чтобы не потерять прозрачность). GIF не трогаем — сломалась бы анимация;
 * HEIC/TIFF, которые браузер не декодирует, уходят как есть — сервер их примет.
 * Если сжатие не дало выигрыша, отправляется исходный файл.
 */
export async function prepareSupportImage(file: File): Promise<File> {
  if (file.size < KEEP_BELOW_BYTES || file.type === 'image/gif' || typeof createImageBitmap !== 'function') {
    return file
  }
  let bitmap: ImageBitmap
  try {
    bitmap = await createImageBitmap(file)
  } catch {
    return file
  }
  try {
    const scale = Math.min(1, MAX_SIDE / Math.max(bitmap.width, bitmap.height))
    const canvas = document.createElement('canvas')
    canvas.width = Math.round(bitmap.width * scale)
    canvas.height = Math.round(bitmap.height * scale)
    const ctx = canvas.getContext('2d')
    if (!ctx) return file
    ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height)

    let blob: Blob | null = null
    if (file.type === 'image/png') {
      blob = await canvasBlob(canvas, 'image/webp', 0.9)
      // Старый Safari вместо WebP молча отдаёт PNG — тогда JPEG.
      if (blob?.type !== 'image/webp') blob = null
    }
    blob ??= await canvasBlob(canvas, 'image/jpeg', 0.88)
    if (!blob || blob.size >= file.size) return file
    const ext = blob.type === 'image/webp' ? 'webp' : 'jpg'
    return new File([blob], renamed(file.name, ext), { type: blob.type, lastModified: file.lastModified })
  } finally {
    bitmap.close()
  }
}
