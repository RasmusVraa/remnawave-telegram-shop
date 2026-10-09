import { isIosWebKit } from '@/lib/utils'

/**
 * Автозум iOS на фокусе поля ввода.
 *
 * WebKit принудительно приближает страницу, когда фокус получает input,
 * textarea или select с font-size меньше 16px, и обратно её не отдаляет: после
 * блюра страница остаётся увеличенной и уезжает за правый край экрана. Blink на
 * Android так не делает, поэтому ловится это только на iPhone — и в Safari, и в
 * Chrome (он там тот же WebKit), и в Telegram Mini App.
 *
 * Поля кабинета набраны text-sm (14px) намеренно, и поднимать их до 16px ради
 * одного браузера мы не стали: это заметно меняет плотность всех форм. Вместо
 * этого на время ввода фиксируем масштаб через maximum-scale — приближать
 * страницу браузеру становится некуда, и он оставляет её как есть.
 *
 * После блюра ограничение снимается: постоянный maximum-scale отобрал бы
 * пинч-зум у всего кабинета, а это единственный способ разглядеть мелкое для
 * тех, кому он нужен.
 */

/**
 * Зумит только то, куда вводят текст с клавиатуры. Ползунки, чекбоксы и кнопки
 * тоже получают фокус, но масштаб не трогают — блокировать его на них значит
 * без причины отбирать пинч-зум.
 */
const ZOOMING_FIELDS = [
  'textarea',
  'select',
  'input:not([type=range]):not([type=checkbox]):not([type=radio])',
  '[contenteditable=""]',
  '[contenteditable="true"]',
].join(', ')

function isZoomingField(el: Element | null): boolean {
  return Boolean(el && el.matches(ZOOMING_FIELDS))
}

export function preventIosInputZoom(): void {
  if (!isIosWebKit()) return

  const meta = document.querySelector<HTMLMetaElement>('meta[name="viewport"]')
  if (!meta) return

  const base = meta.getAttribute('content')
  // Если масштаб ограничен прямо в index.html, вмешиваться нечем и незачем.
  if (!base || base.includes('maximum-scale')) return
  const locked = `${base}, maximum-scale=1.0`

  document.addEventListener('focusin', (e) => {
    if (isZoomingField(e.target as Element | null)) meta.setAttribute('content', locked)
  })

  document.addEventListener('focusout', () => {
    /*
     * Переход между двумя полями — это focusout первого до focusin второго.
     * Снять ограничение прямо здесь значит отпустить масштаб ровно в тот
     * момент, когда фокус переезжает на соседнее поле, — и зум всё-таки
     * случится. Поэтому решение откладываем на кадр и смотрим, где фокус
     * оказался на самом деле.
     */
    requestAnimationFrame(() => {
      if (isZoomingField(document.activeElement)) return
      meta.setAttribute('content', base)
    })
  })
}
