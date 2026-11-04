import { useEffect, useSyncExternalStore, type RefObject } from 'react'
import { useLocation } from 'react-router-dom'

/*
 * Отклик кнопки, пока скачивается код следующей страницы.
 *
 * Страницы кабинета — ленивые чанки (App.tsx), а React Router меняет маршрут
 * внутри startTransition. Пока чанк не скачан, React держит на экране старую
 * страницу и не показывает fallback у Suspense. На медленной сети после нажатия
 * секунды ничего не происходило, и пользователь жал кнопку ещё и ещё.
 *
 * Здесь запоминается элемент, по которому нажали, и, если переход не
 * завершился за PENDING_SHOW_DELAY_MS, на нём включается спиннер:
 *  - Button рендерит его сам (useNavPending ниже);
 *  - у остальных ссылок и кнопок трекер ставит data-nav-pending, а index.css
 *    превращает первую иконку элемента в спиннер (или дугу у [data-pending-ring]).
 * Повторное нажатие на тот же элемент, пока переход идёт, гасится.
 *
 * Правило для новых кнопок — .cursor/docs/frontend/cabinet-loading-states.md.
 */

const PENDING_SHOW_DELAY_MS = 150
/** Нажатие, после которого переход начался не сразу (await запроса), ещё считается его причиной. */
const CLICK_TO_NAV_WINDOW_MS = 15_000
/** Страховка: если маршрут так и не сменился, спиннер не должен крутиться вечно. */
const PENDING_MAX_MS = 30_000

const CLICKABLE = 'a[href], button, [role="button"], [role="menuitem"], [role="tab"], [role="link"]'

type State = { el: HTMLElement | null; shown: boolean }

let state: State = { el: null, shown: false }
const listeners = new Set<() => void>()
let showTimer: ReturnType<typeof setTimeout> | undefined
let maxTimer: ReturnType<typeof setTimeout> | undefined
let markedIcon: Element | null = null
let lastClick: { el: HTMLElement; at: number } | null = null

function emit(next: State) {
  state = next
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

function getSnapshot() {
  return state
}

/** Иконка, которую CSS превратит в спиннер; у Button и у [data-pending-ring] свой рисунок. */
function markIcon(el: HTMLElement): 'icon' | 'text' | 'own' {
  if (el.hasAttribute('data-pending-ui') || el.querySelector('[data-pending-ring], [data-pending-icon]')) return 'own'
  const svg = el.querySelector('svg')
  if (!svg) return 'text'
  svg.setAttribute('data-pending-icon', '')
  markedIcon = svg
  return 'icon'
}

function clearPending() {
  clearTimeout(showTimer)
  clearTimeout(maxTimer)
  if (state.el) state.el.removeAttribute('data-nav-pending')
  if (markedIcon) {
    markedIcon.removeAttribute('data-pending-icon')
    markedIcon = null
  }
  if (state.el || state.shown) emit({ el: null, shown: false })
}

function startPending(el: HTMLElement) {
  clearPending()
  emit({ el, shown: false })
  showTimer = setTimeout(() => {
    if (state.el !== el || !el.isConnected) return
    el.setAttribute('data-nav-pending', markIcon(el))
    emit({ el, shown: true })
  }, PENDING_SHOW_DELAY_MS)
  maxTimer = setTimeout(clearPending, PENDING_MAX_MS)
}

/** Запись истории от React Router: { usr, key, idx }. Чужие pushState (свои ?query и т.п.) не трогаем. */
function isRouterState(s: unknown): boolean {
  return typeof s === 'object' && s !== null && 'key' in s && 'idx' in s
}

function onRouterNavigate() {
  const click = lastClick
  lastClick = null
  if (!click || Date.now() - click.at > CLICK_TO_NAV_WINDOW_MS || !click.el.isConnected) return
  startPending(click.el)
}

let installed = false

function install() {
  if (installed || typeof window === 'undefined') return
  installed = true

  // Capture на document идёт раньше обработчиков React (они висят на корне приложения),
  // поэтому повторное нажатие можно погасить до Link и до onClick.
  document.addEventListener(
    'click',
    (e) => {
      const target = e.target instanceof Element ? e.target.closest<HTMLElement>(CLICKABLE) : null
      if (!target) return
      if (state.el && (state.el === target || state.el.contains(target))) {
        e.preventDefault()
        e.stopPropagation()
        return
      }
      lastClick = { el: target, at: Date.now() }
    },
    true,
  )

  // Кнопки, которые уводят на оплату или вход у провайдера, не гасят спиннер после
  // успеха. Если браузер вернёт страницу «Назад» из bfcache, она оживёт ровно в том
  // виде — с крутящимся спиннером и устаревшими данными. Перезагружаем.
  window.addEventListener('pageshow', (e) => {
    if (e.persisted) window.location.reload()
  })

  // React Router пишет историю синхронно, а сам маршрут меняет позже, в transition, —
  // поэтому начало перехода ловим на pushState/replaceState.
  for (const method of ['pushState', 'replaceState'] as const) {
    const original = window.history[method].bind(window.history)
    window.history[method] = (data: unknown, unused: string, url?: string | URL | null) => {
      original(data, unused, url)
      if (isRouterState(data)) onRouterNavigate()
    }
  }
}

/** Ставится один раз внутри BrowserRouter: снимает спиннер, когда новый маршрут отрисован. */
export function NavPendingTracker() {
  const location = useLocation()
  useEffect(install, [])
  useEffect(() => {
    clearPending()
  }, [location.key])
  return null
}

/** true, пока по этому элементу идёт переход и прошла задержка показа. */
export function useNavPending(ref: RefObject<HTMLElement | null>): boolean {
  const s = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  return s.shown && s.el !== null && s.el === ref.current
}
