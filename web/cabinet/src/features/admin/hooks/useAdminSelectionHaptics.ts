import { useEffect } from 'react'

import { hapticSelection } from '@/lib/haptic'

/** Вкладки, пункты списков и радио: тик, только если выбирают другой. */
const PICK = '[role="tab"], [role="option"], [role="radio"], input[type="radio"]'
/** Тумблеры и чекбоксы: каждое нажатие что-то меняет. */
const TOGGLE = '[role="switch"], [role="checkbox"], input[type="checkbox"]'

function isOff(el: Element): boolean {
  return (
    (el instanceof HTMLButtonElement || el instanceof HTMLInputElement) && el.disabled
  ) || el.getAttribute('aria-disabled') === 'true'
}

/**
 * Тик выбора во всей админке одним слушателем, а не правкой каждой вкладки и тумблера
 * (их десятки в настройках, тарифах, статистике, рассылке).
 *
 * Слушаем click на document в фазе захвата: он срабатывает раньше обработчиков React,
 * поэтому aria-selected ещё старый — по нему понятно, меняется ли выбор.
 * Окна админки рендерятся порталом в body — их клики сюда тоже доходят.
 */
export function useAdminSelectionHaptics(): void {
  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      const target = e.target
      if (!(target instanceof Element)) return
      const pick = target.closest(PICK)
      if (pick) {
        if (isOff(pick)) return
        const state = pick.getAttribute('aria-selected') ?? pick.getAttribute('aria-checked')
        const already = state === 'true' || (pick instanceof HTMLInputElement && pick.checked)
        if (!already) hapticSelection()
        return
      }
      const toggle = target.closest(TOGGLE)
      if (toggle && !isOff(toggle)) hapticSelection()
    }
    document.addEventListener('click', onClick, true)
    return () => document.removeEventListener('click', onClick, true)
  }, [])
}
