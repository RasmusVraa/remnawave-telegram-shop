/** Без 0/O и 1/I: код диктуют голосом и переписывают с картинок. */
const ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'

/** Случайный код для кнопки 🎲 и «Дублировать». */
export function generatePromoCode(length = 8): string {
  const bytes = new Uint8Array(length)
  crypto.getRandomValues(bytes)
  let out = ''
  // 256 делится на 32 без остатка — распределение ровное.
  for (const b of bytes) out += ALPHABET[b % ALPHABET.length]
  return out
}

/** Те же правила, что у бэкенда (validateCreatePromo): 2–64, буквы/цифры/_/-, не с «__». */
export function isValidPromoCode(code: string): boolean {
  const s = code.trim()
  const n = [...s].length
  if (n < 2 || n > 64 || s.startsWith('__')) return false
  return /^[\p{L}\p{Nd}_-]+$/u.test(s)
}
