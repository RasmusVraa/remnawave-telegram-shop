const DEFAULT_PREVIEW_LEN = 6

export function truncatePreview(text: string, maxLen = DEFAULT_PREVIEW_LEN): string {
  if (text.length <= maxLen) return text
  return `${text.slice(0, maxLen)}…`
}
