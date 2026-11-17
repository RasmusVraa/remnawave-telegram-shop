import { useCallback, useEffect, useState } from 'react'

const STORAGE_KEY = 'admin.sidebar.collapsed'

/** Меню на ПК — от lg; на телефоне оно шторка и не сворачивается. */
const DESKTOP_QUERY = '(min-width: 1024px)'

function readCollapsed(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  return target.closest('input, textarea, select') !== null
}

/**
 * Свёрнутое боковое меню админки: только иконки. Выбор живёт в localStorage —
 * каждая страница рендерит свой `<AdminLayout>`, и без начального чтения
 * меню разворачивалось бы на каждом переходе.
 *
 * Ctrl+B (⌘B) переключает. По `code`, а не `key`: на русской раскладке
 * `key` — «и». В полях ввода не перехватываем — там Ctrl+B делает жирный.
 */
export function useAdminSidebarCollapsed() {
  const [collapsed, setCollapsed] = useState(readCollapsed)

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, collapsed ? '1' : '0')
    } catch {
      // приватный режим или запрет хранилища — просто не запомним
    }
  }, [collapsed])

  const toggle = useCallback(() => setCollapsed((v) => !v), [])

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.defaultPrevented || e.repeat || e.altKey || e.shiftKey) return
      if (!(e.ctrlKey || e.metaKey) || e.code !== 'KeyB') return
      if (!window.matchMedia(DESKTOP_QUERY).matches) return
      if (isEditableTarget(e.target)) return
      e.preventDefault()
      toggle()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [toggle])

  return { collapsed, toggle }
}
