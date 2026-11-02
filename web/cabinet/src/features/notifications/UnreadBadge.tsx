import { cn } from '@/lib/utils'

/**
 * Счётчик непрочитанных: на колокольчике и в заголовке ленты, цвет у обоих
 * один. Есть непрочитанное важное — красный, только обычные — зелёный.
 *
 * Цифра по центру. Браузер ставит текст на целый пиксель, а кружок рисует там,
 * где он стоит, — хоть на 134,97px. Стоит кружок на дробной координате (rem на
 * ПК 18px, заголовок шторки с высотой строки 27,5px) — цифра уезжает от центра
 * до пикселя. will-change выносит счётчик в свой слой: слой целиком ставится
 * на целый пиксель, и кружок с цифрой округляются вместе.
 *
 * Цифры Inter высотой с заглавную (0,727em): при 11px это ровно 8px, и в
 * кружке чётной высоты (18, 20) цифра садится в центр без остатка. Размер
 * шрифта снаружи не передавать: tailwind-merge тогда выкинет leading-none, и
 * высота строки уедет за шрифтом родителя. Обводка — кольцом снаружи, а не
 * рамкой: рамка съела бы внутреннее место.
 */
export function UnreadBadge({ count, important, className }: { count: number | string; important: boolean; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center justify-center rounded-full font-sans text-[11px] font-bold leading-none tabular-nums text-white [will-change:transform]',
        important ? 'bg-red-500' : 'bg-emerald-500',
        className,
      )}
      aria-hidden
    >
      {count}
    </span>
  )
}
