import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown } from 'lucide-react'

import { COUNTRY_CENTROIDS } from '@/features/landing/countryCentroids'
import { cn } from '@/lib/utils'
import { CountryFlag } from './CountryFlag'

interface CountryOption {
  code: string
  name: string
}

function regionName(code: string, locale: string): string {
  try {
    return new Intl.DisplayNames([locale], { type: 'region' }).of(code) ?? code
  } catch {
    return code
  }
}

/**
 * Выбор страны: флаг и название, поиск по мере набора (по названию и по коду).
 * Значение — двухбуквенный код ISO, как ждёт сервер.
 */
export function CountrySelect({
  value,
  onChange,
  placeholder,
  className,
}: {
  value: string
  onChange: (code: string) => void
  placeholder: string
  className?: string
}) {
  const { i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [cursor, setCursor] = useState(0)
  const listRef = useRef<HTMLUListElement>(null)

  const options = useMemo<CountryOption[]>(() => {
    const locale = i18n.language || 'ru'
    return Object.keys(COUNTRY_CENTROIDS)
      .filter((code) => /^[A-Z]{2}$/.test(code))
      .map((code) => ({ code, name: regionName(code, locale) }))
      .sort((a, b) => a.name.localeCompare(b.name, locale))
  }, [i18n.language])

  const selected = options.find((item) => item.code === value.toUpperCase())
  const needle = query.trim().toLowerCase()
  const filtered = needle
    ? options
        .filter((item) => item.name.toLowerCase().includes(needle) || item.code.toLowerCase().startsWith(needle))
        // Совпадение с начала названия — выше, чем в середине.
        .sort((a, b) => Number(!a.name.toLowerCase().startsWith(needle)) - Number(!b.name.toLowerCase().startsWith(needle)))
    : options

  function pick(item: CountryOption | undefined) {
    if (!item) return
    onChange(item.code)
    setOpen(false)
    setQuery('')
  }

  function scrollTo(index: number) {
    const el = listRef.current?.children[index] as HTMLElement | undefined
    el?.scrollIntoView({ block: 'nearest' })
  }

  return (
    <div className={cn('relative', className)}>
      <div className="relative">
        {!open && selected && (
          <CountryFlag code={selected.code} className="pointer-events-none absolute left-3 top-1/2 h-4 w-6 -translate-y-1/2 rounded-[3px]" />
        )}
        <input
          className={cn(
            'h-10 w-full rounded-lg border border-border/60 bg-background pr-9 text-sm outline-none transition-colors placeholder:text-muted-foreground/60 focus-visible:ring-2 focus-visible:ring-ring',
            !open && selected ? 'pl-11' : 'pl-3',
          )}
          role="combobox"
          aria-expanded={open}
          aria-autocomplete="list"
          autoComplete="off"
          spellCheck={false}
          placeholder={placeholder}
          value={open ? query : (selected?.name ?? value)}
          onFocus={() => {
            setOpen(true)
            setQuery('')
            setCursor(0)
          }}
          onBlur={() => setOpen(false)}
          onChange={(e) => {
            setQuery(e.target.value)
            setCursor(0)
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              const next = Math.min(cursor + 1, filtered.length - 1)
              setCursor(next)
              scrollTo(next)
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              const next = Math.max(cursor - 1, 0)
              setCursor(next)
              scrollTo(next)
            } else if (e.key === 'Enter') {
              e.preventDefault()
              pick(filtered[cursor])
            } else if (e.key === 'Escape') {
              setOpen(false)
              ;(e.target as HTMLInputElement).blur()
            }
          }}
        />
        <ChevronDown className="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      </div>
      {open && (
        <ul
          ref={listRef}
          role="listbox"
          className="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-xl border border-border bg-background p-1 shadow-lg"
        >
          {filtered.length === 0 ? (
            <li className="px-3 py-2 text-sm text-muted-foreground">—</li>
          ) : (
            filtered.map((item, index) => (
              <li
                key={item.code}
                role="option"
                aria-selected={item.code === selected?.code}
                // mousedown, а не click: иначе поле теряет фокус раньше и список закрывается до выбора.
                onMouseDown={(e) => {
                  e.preventDefault()
                  pick(item)
                }}
                onMouseEnter={() => setCursor(index)}
                className={cn(
                  'flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm',
                  index === cursor ? 'bg-muted' : '',
                  item.code === selected?.code && 'font-semibold',
                )}
              >
                <CountryFlag code={item.code} className="h-4 w-6 shrink-0 rounded-[3px]" />
                <span className="truncate">{item.name}</span>
                <span className="ml-auto text-xs text-muted-foreground">{item.code}</span>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  )
}
