import * as React from 'react'
import { Slot, Slottable } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'
import { useNavPending } from '@/lib/navPending'
import { Spinner } from '@/components/ui/spinner'

const buttonVariants = cva(
  // cabinet-btn / cabinet-btn-primary — стабильные хуки для декор-тем:
  // цепляться за utility-классы Tailwind (bg-primary и т.п.) слишком хрупко.
  'cabinet-btn inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium transition-all duration-200 active:scale-[0.98] active:duration-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default:
          'cabinet-btn-primary bg-primary text-primary-foreground shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] hover:brightness-110 hover:shadow-[0_10px_26px_-10px_hsl(var(--cabinet-accent)/0.6)]',
        destructive:
          // Свой отклик, а не только brightness: на тёмной теме осветление красного почти незаметно.
          'bg-destructive text-destructive-foreground shadow-[0_4px_6px_-1px_rgb(0_0_0_/_0.1),0_2px_4px_-2px_rgb(0_0_0_/_0.1)] hover:bg-[color-mix(in_srgb,hsl(var(--destructive))_86%,white)] hover:shadow-[0_10px_26px_-10px_hsl(var(--destructive)/0.75)] active:bg-[color-mix(in_srgb,hsl(var(--destructive))_88%,black)]',
        outline:
          'border border-border bg-transparent hover:border-[hsl(var(--cabinet-accent)/0.5)] hover:bg-secondary hover:text-secondary-foreground hover:shadow-[0_8px_22px_-10px_hsl(var(--cabinet-accent)/0.5)] dark:bg-card/40 dark:hover:bg-secondary/80',
        secondary:
          'bg-secondary text-secondary-foreground hover:bg-secondary/80 hover:shadow-[0_8px_22px_-10px_hsl(var(--cabinet-accent)/0.4)]',
        ghost:
          'hover:bg-secondary hover:text-secondary-foreground',
        link:
          'text-primary underline-offset-4 hover:underline active:scale-100',
      },
      size: {
        default: 'h-10 px-4 py-2',
        sm: 'h-8 rounded-md px-3 text-xs',
        lg: 'h-11 rounded-lg px-8',
        icon: 'h-9 w-9',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
  /**
   * Запрос в работе: спиннер вместо иконки, повторное нажатие невозможно.
   * Обязателен для кнопок, которые ждут сервер (обычно `loading={mutation.isPending}`).
   * Если onClick возвращает Promise, кнопка включает это состояние сама до его завершения.
   * Переход на другую страницу отслеживается сам (lib/navPending.ts).
   */
  loading?: boolean
  /**
   * Спиннер вне потока, с этими классами позиции (`absolute …`). Для кнопок-карточек
   * с колонкой внутри (период тарифа): обычная обёртка-строка со спиннером
   * перекладывает их содержимое в ряд.
   */
  overlaySpinnerClassName?: string
  /**
   * Спиннер встаёт прямо в раскладку кнопки на место первой иконки, без обёртки-строки.
   * Для кнопок-колонок «значок над подписью» (вход через соцсети).
   */
  spinnerInFlow?: boolean
}

function isPromiseLike(v: unknown): v is PromiseLike<unknown> {
  return typeof v === 'object' && v !== null && typeof (v as PromiseLike<unknown>).then === 'function'
}

/**
 * Спиннер встаёт первым, а ведущая иконка прячется (`.cabinet-btn-spinner + svg` в index.css):
 * подпись и ширина кнопки при загрузке не меняются.
 */
const spinner = <Spinner />

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, loading = false, overlaySpinnerClassName, spinnerInFlow = false, children, disabled, onClick, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button'
    const innerRef = React.useRef<HTMLButtonElement | null>(null)
    const setRefs = React.useCallback(
      (node: HTMLButtonElement | null) => {
        innerRef.current = node
        if (typeof ref === 'function') ref(node)
        else if (ref) ref.current = node
      },
      [ref],
    )
    const navPending = useNavPending(innerRef)
    const [promisePending, setPromisePending] = React.useState(false)
    const busy = loading || navPending || promisePending

    const handleClick = onClick
      ? (e: React.MouseEvent<HTMLButtonElement>) => {
          // Тип onClick обещает void, но стрелка `() => mutateAsync()` вернёт Promise — его и ждём.
          const result: unknown = onClick(e)
          if (!isPromiseLike(result)) return
          setPromisePending(true)
          Promise.resolve(result).finally(() => setPromisePending(false))
        }
      : undefined

    return (
      <Comp
        className={cn(
          buttonVariants({ variant, size, className }),
          /* При загрузке кнопка остаётся disabled, но не «серой»: виден спиннер как на активной CTA */
          busy && '!opacity-100',
        )}
        ref={setRefs}
        disabled={disabled || busy}
        aria-busy={busy || undefined}
        // Спиннер рисует сама кнопка — трекер переходов не трогает её иконку.
        data-pending-ui=""
        onClick={handleClick}
        {...props}
      >
        {/* У Slot дети идут списком, а не фрагментом: иначе он не найдёт Slottable. */}
        {asChild && busy ? spinner : null}
        {asChild ? (
          <Slottable>{children}</Slottable>
        ) : busy && overlaySpinnerClassName ? (
          // Спиннер после детей: правило `.cabinet-btn-spinner + svg` не должно прятать их иконки.
          <>
            {children}
            <Spinner className={overlaySpinnerClassName} />
          </>
        ) : busy && spinnerInFlow ? (
          // contents: спиннер и дети — прямые участники флекса кнопки; иконку прячет `.cabinet-btn-spinner + svg`.
          <span className="contents">
            {spinner}
            {children}
          </span>
        ) : busy ? (
          <span className="inline-flex w-full min-w-0 items-center justify-center gap-2">
            {spinner}
            {children}
          </span>
        ) : (
          children
        )}
      </Comp>
    )
  },
)
Button.displayName = 'Button'

export { Button, buttonVariants }
