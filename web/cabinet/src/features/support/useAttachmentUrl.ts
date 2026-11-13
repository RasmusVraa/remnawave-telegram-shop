import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'

import { api } from '@/lib/api'

/**
 * object URL картинки из чата поддержки. Вложение не меняется, поэтому Blob
 * кэшируется на всё время жизни вкладки: лента опрашивается каждые несколько
 * секунд, а картинки скачиваются один раз.
 */
export function useAttachmentUrl(id: number) {
  const query = useQuery({
    queryKey: ['support', 'attachment', id],
    queryFn: () => api.supportAttachmentBlob(id),
    staleTime: Infinity,
    gcTime: 30 * 60_000,
    retry: 1,
  })
  const [url, setUrl] = useState<string | null>(null)

  useEffect(() => {
    if (!query.data) return
    const next = URL.createObjectURL(query.data)
    setUrl(next)
    return () => URL.revokeObjectURL(next)
  }, [query.data])

  return { url, isError: query.isError, isLoading: query.isLoading }
}
