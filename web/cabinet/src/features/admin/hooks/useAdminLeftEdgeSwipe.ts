import { useEffect, useRef } from 'react'

import { hapticTap } from '@/lib/haptic'
import { useAdminShell } from '../layout/AdminShellContext'
import { getAdminMobileNavWidthPx } from '../layout/adminMobileNav'

const MOBILE_MQ = '(max-width: 1023px)'
/** Не с самого края — иначе срабатывает системный жест «назад». */
const EDGE_START_MIN_PX = 20
const EDGE_START_MAX_PX = 72
const CANCEL_VERTICAL_PX = 14
/**
 * Порог, после которого касание считается свайпом, а не тапом.
 *
 * Без него хватало одного пикселя дрожания пальца: панель прыгала под пальцем,
 * WebKit при синтезе клика делал новый hit-test и не находил под точкой ссылку —
 * тап по пункту меню закрывал шторку вместо перехода. Чем чаще устройство
 * опрашивает тачскрин (ProMotion), тем вероятнее такое дрожание.
 */
const DRAG_START_MIN_PX = 10
/** Собственные тач-цели шторки: бургер и сама панель. Свайп их не перехватывает. */
const NAV_OWN_TARGETS = '[data-admin-nav-panel],[data-admin-nav-toggle]'
/**
 * Элементы со своим горизонтальным жестом (например, ручка перетаскивания
 * тарифа): касание, начатое на них, не считается стартом свайпа за меню.
 * Проверяется в touchstart, поэтому не зависит от React-состояния «тянем».
 */
const NO_EDGE_SWIPE_TARGETS = '[data-admin-no-edge-swipe]'

/**
 * Свайп вправо из левой зоны — плавно выдвигает мобильное админ-меню.
 */
export function useAdminLeftEdgeSwipe(enabled: boolean) {
  const { setMobileNavDrag, commitMobileNavDrag, mobileNavOffsetPx } = useAdminShell()
  const mobileNavOffsetRef = useRef(mobileNavOffsetPx)
  mobileNavOffsetRef.current = mobileNavOffsetPx

  const trackingRef = useRef({
    active: false,
    dragging: false,
    startX: 0,
    startY: 0,
    preventBack: false,
    lastOffset: 0,
    armed: false,
  })

  useEffect(() => {
    if (!enabled) return

    const mobileMq = window.matchMedia(MOBILE_MQ)

    function reset() {
      trackingRef.current = {
        active: false,
        dragging: false,
        startX: 0,
        startY: 0,
        preventBack: false,
        lastOffset: 0,
        armed: false,
      }
    }

    /*
     * Смотрим на смещение панели, а не на флаг «меню открыто»: флаг живёт своей
     * жизнью во время анимаций, а видимая позиция панели — единственное, что
     * честно отвечает на вопрос «палец сейчас над шторкой или над страницей».
     */
    function isPanelVisible() {
      return mobileNavOffsetRef.current > 0
    }

    function onTouchStart(e: TouchEvent) {
      if (!mobileMq.matches || isPanelVisible()) return
      const touch = e.touches[0]
      if (!touch) return
      // Зона старта свайпа накрывает и бургер, и иконки пунктов меню: касание
      // по ним обязано остаться обычным тапом.
      const target = e.target
      if (target instanceof Element && target.closest(NAV_OWN_TARGETS)) return
      if (target instanceof Element && target.closest(NO_EDGE_SWIPE_TARGETS)) return
      const x = touch.clientX
      if (x < EDGE_START_MIN_PX || x > EDGE_START_MAX_PX) return
      trackingRef.current = {
        active: true,
        dragging: false,
        startX: x,
        startY: touch.clientY,
        preventBack: false,
        lastOffset: 0,
        armed: false,
      }
    }

    function onTouchMove(e: TouchEvent) {
      const state = trackingRef.current
      if (!state.active) return
      const touch = e.touches[0]
      if (!touch) return

      const dx = touch.clientX - state.startX
      const dy = Math.abs(touch.clientY - state.startY)

      if (dy > CANCEL_VERTICAL_PX && dy > dx) {
        reset()
        return
      }

      if (!state.dragging) {
        if (dx < DRAG_START_MIN_PX) return
        state.dragging = true
      }

      if (!state.preventBack && dx > 12 && dx > dy * 0.9) {
        state.preventBack = true
      }
      if (state.preventBack) {
        e.preventDefault()
      }

      state.lastOffset = dx
      // Толчок в момент, когда меню уже откроется при отпускании (порог — как в commitMobileNavDrag).
      const armed = dx >= getAdminMobileNavWidthPx() * 0.35
      if (armed && !state.armed) hapticTap()
      state.armed = armed
      setMobileNavDrag(dx)
    }

    function onTouchEnd() {
      const state = trackingRef.current
      // Тап без жеста ничего не коммитит: раньше он приходил сюда с lastOffset = 0
      // и закрывал шторку, хотя пользователь её не тянул.
      if (state.active && state.dragging) {
        commitMobileNavDrag(state.lastOffset)
      }
      reset()
    }

    document.addEventListener('touchstart', onTouchStart, { passive: true })
    document.addEventListener('touchmove', onTouchMove, { passive: false })
    document.addEventListener('touchend', onTouchEnd, { passive: true })
    document.addEventListener('touchcancel', onTouchEnd, { passive: true })

    return () => {
      document.removeEventListener('touchstart', onTouchStart)
      document.removeEventListener('touchmove', onTouchMove)
      document.removeEventListener('touchend', onTouchEnd)
      document.removeEventListener('touchcancel', onTouchEnd)
      reset()
    }
  }, [enabled, setMobileNavDrag, commitMobileNavDrag])
}
