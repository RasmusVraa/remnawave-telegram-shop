import { Component, Fragment, type ErrorInfo, type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle } from 'lucide-react'

import { Button } from '@/components/ui/button'

/**
 * Граница ошибок вокруг маршрутов.
 *
 * Без неё любая ошибка рендера обрушивала всё дерево в белый экран. В браузере
 * человек хотя бы перезагрузит вкладку, а в мини-аппе Telegram он видит пустоту,
 * не может открыть консоль и просто закрывает приложение.
 *
 * Границу сбрасываем при смене маршрута: если сломалась одна страница, переход
 * на другую должен вернуть работоспособный интерфейс без перезагрузки.
 *
 * «Повторить» раньше только снимал флаг ошибки, и дерево тут же падало снова:
 * - чанк страницы не загрузился (чаще всего после деплоя: старая вкладка просит
 *   файл с прежним хешем, а его уже нет). React.lazy запоминает отклонённый
 *   import навсегда, так что помогает только перезагрузка — её и делаем;
 * - ошибка из данных: запрос в кэше React Query остаётся в состоянии error,
 *   и страница рендерится с ним же. Сбрасываем упавшие запросы и
 *   перемонтируем поддерево, чтобы заодно обнулить локальный state.
 */

/** Ошибка загрузки lazy-чанка: формулировки Chrome, Safari, Firefox и прелоадера Vite. */
function isChunkLoadError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error)
  return /dynamically imported module|Importing a module script failed|Unable to preload CSS/i.test(message)
}

const CHUNK_RELOAD_KEY = 'cabinet:chunk-reload-at'

/**
 * Один автоматический reload на битый чанк: после деплоя пользователь не должен
 * видеть экран ошибки вовсе. Метка времени не даёт уйти в цикл, если чанк
 * недоступен по другой причине (сеть, CDN) — тогда показываем экран как обычно.
 */
function tryAutoReload(): boolean {
  try {
    const last = Number(sessionStorage.getItem(CHUNK_RELOAD_KEY) || 0)
    if (Date.now() - last < 30_000) return false
    sessionStorage.setItem(CHUNK_RELOAD_KEY, String(Date.now()))
  } catch {
    return false
  }
  window.location.reload()
  return true
}

interface Props {
  children: ReactNode
  /** Смена значения сбрасывает пойманную ошибку. Сюда передаём pathname. */
  resetKey: string
  fallback: (retry: () => void) => ReactNode
  /** Вызывается перед повторным рендером: сбросить то, из-за чего упали. */
  onRetry: () => void
}

interface State {
  error: unknown
  /** Ключ поддерева: меняем при «Повторить», чтобы перемонтировать страницу целиком. */
  attempt: number
}

class ErrorBoundaryInner extends Component<Props, State> {
  state: State = { error: null, attempt: 0 }

  static getDerivedStateFromError(error: unknown): Partial<State> {
    return { error: error ?? new Error('unknown render error') }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Единственный след, по которому потом можно понять, что случилось у пользователя.
    console.error('cabinet: ошибка рендера маршрута', error, info.componentStack)
    if (isChunkLoadError(error)) tryAutoReload()
  }

  componentDidUpdate(prev: Props) {
    if (this.state.error && prev.resetKey !== this.props.resetKey) {
      this.setState({ error: null })
    }
  }

  private retry = () => {
    if (isChunkLoadError(this.state.error)) {
      window.location.reload()
      return
    }
    this.props.onRetry()
    this.setState((s) => ({ error: null, attempt: s.attempt + 1 }))
  }

  render() {
    if (this.state.error) return this.props.fallback(this.retry)
    return <Fragment key={this.state.attempt}>{this.props.children}</Fragment>
  }
}

export function RouteErrorBoundary({ children }: { children: ReactNode }) {
  const { pathname } = useLocation()
  const queryClient = useQueryClient()

  return (
    <ErrorBoundaryInner
      resetKey={pathname}
      onRetry={() => void queryClient.resetQueries({ predicate: (q) => q.state.status === 'error' })}
      fallback={(retry) => <ErrorFallback onRetry={retry} />}
    >
      {children}
    </ErrorBoundaryInner>
  )
}

function ErrorFallback({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation()

  return (
    <div className="flex min-h-dvh items-center justify-center px-4">
      <div className="w-full max-w-sm rounded-[var(--radius)] border border-border bg-card px-6 py-8 text-center">
        <div className="mx-auto flex size-12 items-center justify-center rounded-xl bg-destructive/10">
          <AlertTriangle className="size-6 text-destructive" aria-hidden />
        </div>
        <p className="mt-4 text-lg font-semibold">{t('errors.boundaryTitle')}</p>
        <p className="mt-1 text-sm text-muted-foreground">{t('errors.boundaryHint')}</p>
        <div className="mt-5 flex flex-col gap-2">
          <Button onClick={onRetry}>{t('errors.retry')}</Button>
          <Button variant="outline" onClick={() => window.location.reload()}>
            {t('errors.boundaryReload')}
          </Button>
        </div>
      </div>
    </div>
  )
}
