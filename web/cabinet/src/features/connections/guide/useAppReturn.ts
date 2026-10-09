import { useCallback, useEffect, useRef } from 'react'

export type AppTrip = 'install' | 'add'

type Pending = { kind: AppTrip; scope: string; at: number; left: boolean }

const KEY = 'cabinet:connections-trip:v1'
/** Ушёл в магазин и вернулся через полчаса — уже не считаем это тем же заходом. */
const MAX_AGE_MS = 30 * 60 * 1000

function read(): Pending | null {
  try {
    const raw = window.sessionStorage.getItem(KEY)
    return raw ? (JSON.parse(raw) as Pending) : null
  } catch {
    return null
  }
}

function write(p: Pending | null) {
  try {
    if (p) window.sessionStorage.setItem(KEY, JSON.stringify(p))
    else window.sessionStorage.removeItem(KEY)
  } catch {
    /* без хранилища возврат после перезагрузки не распознаем — останется ручная кнопка */
  }
}

/**
 * Телефон: понять, что человек сходил в магазин приложений или в само
 * приложение и вернулся, — вместо кнопок «Установил» и «Подписка появилась?».
 *
 * Нажали кнопку → запоминаем, куда ушли. Страница уходит на задний план
 * (visibilitychange) или выгружается совсем (pagehide — iOS Chrome и Telegram
 * открывают приложение через промежуточную /deeplink) → отмечаем «ушёл».
 * Страница снова видна или открылась заново в той же вкладке → onReturn.
 *
 * scope — пара «платформа + приложение»: возврат засчитывается тому шагу,
 * с которого уходили.
 */
export function useAppReturn(scope: string, enabled: boolean, onReturn: (kind: AppTrip) => void) {
  const pending = useRef<Pending | null>(null)
  const callback = useRef(onReturn)
  callback.current = onReturn

  // Страница открылась заново после ухода в приложение (перезагрузка вкладки).
  useEffect(() => {
    if (!enabled || !scope) return
    const p = read()
    if (!p) return
    if (p.scope !== scope || Date.now() - p.at > MAX_AGE_MS) {
      if (Date.now() - p.at > MAX_AGE_MS) write(null)
      return
    }
    if (p.left) {
      write(null)
      callback.current(p.kind)
    } else {
      pending.current = p
    }
  }, [enabled, scope])

  useEffect(() => {
    if (!enabled) return
    function markLeft() {
      const p = pending.current
      if (!p || p.left) return
      p.left = true
      write(p)
    }
    function onVisibility() {
      if (document.visibilityState === 'hidden') {
        markLeft()
        return
      }
      const p = pending.current
      if (p?.left) {
        pending.current = null
        write(null)
        callback.current(p.kind)
      }
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('pagehide', markLeft)
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('pagehide', markLeft)
    }
  }, [enabled])

  const start = useCallback(
    (kind: AppTrip) => {
      if (!enabled || !scope) return
      const p: Pending = { kind, scope, at: Date.now(), left: false }
      pending.current = p
      write(p)
    },
    [enabled, scope],
  )

  /** Ушли ли со страницы после последнего нажатия. */
  const hasLeft = useCallback(() => Boolean(pending.current?.left), [])

  const cancel = useCallback(() => {
    pending.current = null
    write(null)
  }, [])

  return { start, hasLeft, cancel }
}
