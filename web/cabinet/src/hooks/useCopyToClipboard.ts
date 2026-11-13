import { useCallback, useEffect, useRef, useState } from 'react'

import { copyToClipboard } from '@/lib/clipboard'
import { hapticError, hapticTap } from '@/lib/haptic'

export type CopyState = 'idle' | 'done' | 'failed'

const DONE_RESET_MS = 2000
/** Ошибку держим дольше: её нужно успеть прочитать, а не просто заметить галочку. */
const FAILED_RESET_MS = 4000

/**
 * Копирование с индикацией результата.
 *
 * Раньше каждая страница писала это руками — `await navigator.clipboard.writeText`,
 * `setCopied(true)`, `setTimeout`. Помимо дублирования там было две проблемы:
 * необработанное исключение, когда Clipboard API недоступен (частый случай в
 * WebView Telegram), и `setState` после размонтирования, если уйти со страницы
 * в течение двух секунд после нажатия.
 */
export function useCopyToClipboard() {
  const [state, setState] = useState<CopyState>('idle')
  const timerRef = useRef<number | null>(null)

  useEffect(
    () => () => {
      if (timerRef.current !== null) window.clearTimeout(timerRef.current)
    },
    [],
  )

  const copy = useCallback(async (value: string): Promise<boolean> => {
    const ok = await copyToClipboard(value)
    // Скопировали — лёгкий толчок (копируют часто, «успех» тут был бы громким).
    if (ok) hapticTap()
    else hapticError()
    setState(ok ? 'done' : 'failed')
    if (timerRef.current !== null) window.clearTimeout(timerRef.current)
    timerRef.current = window.setTimeout(
      () => setState('idle'),
      ok ? DONE_RESET_MS : FAILED_RESET_MS,
    )
    return ok
  }, [])

  return { state, copy }
}
