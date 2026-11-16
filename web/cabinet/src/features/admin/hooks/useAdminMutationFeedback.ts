import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { AdminFeedbackState } from '../components/AdminFeedback'
import { formatAdminApiError } from '../utils/formatAdminApiError'

export function useAdminMutationFeedback() {
  const { t } = useTranslation()
  const [feedback, setFeedback] = useState<AdminFeedbackState | null>(null)

  const clear = useCallback(() => setFeedback(null), [])

  const showError = useCallback(
    (err: unknown) => {
      setFeedback({ type: 'error', message: formatAdminApiError(err, t) })
    },
    [t],
  )

  const showSuccess = useCallback((message: string) => {
    setFeedback({ type: 'success', message })
  }, [])

  const showInfo = useCallback((message: string) => {
    setFeedback({ type: 'info', message })
  }, [])

  const handlers = useCallback(
    (successMessage?: string) => ({
      onSuccess: () => {
        if (successMessage) showSuccess(successMessage)
      },
      onError: showError,
    }),
    [showError, showSuccess],
  )

  return { feedback, clear, showError, showSuccess, showInfo, handlers }
}
