import ReactMarkdown from 'react-markdown'
import remarkGfm from '@/lib/remarkGfmCompat'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize from 'rehype-sanitize'

import { cn } from '@/lib/utils'

function decodeHtmlEntities(input: string): string {
  if (typeof document === 'undefined') return input
  const el = document.createElement('textarea')
  el.innerHTML = input
  return el.value
}

/**
 * Подготовка описания тарифа из админки / бота.
 * Каждый \n → <br>: 1 Enter — новая строка, 2+ Enter подряд — больше вертикальный зазор.
 * Работает одинаково для HTML Telegram, plain text и inline-markdown (**жирный**).
 */
export function preprocessTelegramMarkup(input: string): string {
  return decodeHtmlEntities(String(input))
    .replace(/\r\n/g, '\n')
    .replace(/<tg-emoji[^>]*>([\s\S]*?)<\/tg-emoji>/gi, '$1')
    .replace(/\t/g, '    ')
    .replace(/\n/g, '<br>')
}

const descriptionClassName =
  '[&_a]:text-primary [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:whitespace-pre-wrap [&_blockquote]:break-words [&_i]:italic [&_li]:ml-4 [&_li]:mb-1 [&_li]:list-disc [&_li]:whitespace-pre-wrap [&_li:last-child]:mb-0 [&_p]:mb-0 [&_p]:whitespace-pre-wrap [&_p]:leading-relaxed'

export function TariffDescription({
  text,
  className,
}: {
  text: string
  className?: string
}) {
  const processed = preprocessTelegramMarkup(text)
  if (!processed.trim()) return null

  return (
    <div className={cn(descriptionClassName, className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeRaw, rehypeSanitize]}>
        {processed}
      </ReactMarkdown>
    </div>
  )
}
