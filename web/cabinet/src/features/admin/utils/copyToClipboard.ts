import { copyToClipboard as copyToClipboardBase } from '@/lib/clipboard'
import { hapticError, hapticTap } from '@/lib/haptic'

/**
 * Реализация переехала в `@/lib/clipboard` — она нужна и пользовательской части
 * кабинета, где раньше вызывался голый `navigator.clipboard.writeText`.
 * Здесь — обёртка для админки с вибрацией, как у useCopyToClipboard в кабинете:
 * скопировали — лёгкий толчок, не вышло — ошибка.
 */
export async function copyToClipboard(value: string): Promise<boolean> {
  const ok = await copyToClipboardBase(value)
  if (ok) hapticTap()
  else hapticError()
  return ok
}
