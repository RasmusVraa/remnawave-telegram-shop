import { cn } from '@/lib/utils'

/**
 * Спиннер загрузки кнопки. Цвет берёт от текста (border-current), размер — h-4 w-4.
 *
 * Внутри Button рисуется сам (проп loading). В обычном <button>/<a> ставится
 * на место ведущей иконки: `{busy ? <Spinner /> : <Trash2 />}` — так подпись и
 * ширина не прыгают. Класс cabinet-btn-spinner прячет следующую за ним иконку
 * (index.css), поэтому спиннер можно и просто поставить перед ней.
 */
export function Spinner({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'cabinet-btn-spinner box-border inline-block h-4 w-4 shrink-0 rounded-full border-2 border-solid border-current border-t-transparent animate-spin',
        className,
      )}
      aria-hidden
    />
  )
}
