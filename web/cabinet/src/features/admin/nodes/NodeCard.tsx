import { memo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Activity,
  ArrowDown,
  ArrowUp,
  AudioLines,
  Ban,
  CircleAlert,
  CloudUpload,
  Cpu,
  Globe,
  Plug,
  RotateCw,
  Sparkle,
  Users,
  type LucideIcon,
} from 'lucide-react'

import { cn } from '@/lib/utils'
import type { AdminNodeDTO, AdminNodeState } from '@/lib/types/admin'
import { CountryFlag } from '../components/CountryFlag'
import { faviconSrcFromUrl } from '../utils/normalizeUrl'
import { formatBitrate, formatBytes, formatUptime } from './format'

export type NodeLayout = 'row' | 'card'

/**
 * Сплошная подложка карточки. Класс bg-card тут не годится: cn (tailwind-merge) выкидывает его рядом
 * с bg-gradient-to-r, и карточка остаётся прозрачной — сквозь неё видно фон страницы. Градиент состояния
 * ложится поверх этого цвета.
 */
const SOLID_BG = { backgroundColor: 'hsl(var(--card))' } as const

interface StateStyle {
  wrap: string
  icon: LucideIcon
  badge: string
}

/** Цвета состояния — как в панели Remna: зелёная рамка, красная, жёлтая, серая. */
const STATE_STYLE: Record<AdminNodeState, StateStyle> = {
  online: {
    wrap: 'border-emerald-500/45 bg-card bg-gradient-to-r from-emerald-500/10 via-transparent to-transparent',
    icon: Activity,
    badge: 'border-emerald-500/50 bg-emerald-500/10 text-emerald-400',
  },
  offline: {
    wrap: 'border-red-500/45 bg-card bg-gradient-to-r from-red-500/[0.12] via-transparent to-transparent',
    icon: CircleAlert,
    badge: 'border-red-500/50 bg-red-500/10 text-red-400',
  },
  connecting: {
    wrap: 'border-amber-500/45 bg-card bg-gradient-to-r from-amber-500/10 via-transparent to-transparent',
    icon: CloudUpload,
    badge: 'border-amber-500/50 bg-amber-500/10 text-amber-400',
  },
  disabled: {
    wrap: 'border-border bg-card',
    icon: Ban,
    badge: 'border-border bg-muted/40 text-muted-foreground',
  },
}

/** Иконка памяти как в панели и макете: тонкая плашка с двумя точками. В lucide похожей нет. */
function MemoryIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-3 shrink-0"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <rect x="2" y="9" width="20" height="8" rx="2" />
      <path d="M6 13h.01M10 13h.01" />
    </svg>
  )
}

const DASH = <span className="opacity-60">—</span>

function StateBadge({ state }: { state: AdminNodeState }) {
  const { t } = useTranslation()
  const s = STATE_STYLE[state]
  const Icon = s.icon
  return (
    <span
      className={cn('flex size-8 shrink-0 items-center justify-center rounded-[9px] border', s.badge)}
      title={t(`admin.nodes.state.${state}`)}
    >
      <Icon className="size-4" aria-hidden />
    </span>
  )
}

function UsersPill({ count }: { count: number }) {
  const { t } = useTranslation()
  return (
    <span
      title={t('admin.nodes.usersTitle')}
      className={cn(
        'inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[9px] border bg-transparent px-2.5 font-mono text-[13px] font-bold tabular-nums',
        count > 0 ? 'border-emerald-500/60 text-emerald-300' : 'border-foreground/80 text-foreground',
      )}
    >
      <Users className="size-3.5" aria-hidden />
      {count}
    </span>
  )
}

function ProviderBadge({ node }: { node: AdminNodeDTO }) {
  if (!node.provider) return null
  const icon = faviconSrcFromUrl(node.provider.favicon_link)
  return (
    <span className="inline-flex h-6 max-w-full items-center gap-1.5 rounded-[7px] bg-secondary px-2 text-[11px] font-extrabold uppercase tracking-wide">
      {icon && (
        <img
          src={icon}
          alt=""
          loading="lazy"
          referrerPolicy="no-referrer"
          className="size-3.5 shrink-0 rounded-[3px] object-contain"
          onError={(e) => {
            e.currentTarget.style.display = 'none'
          }}
        />
      )}
      <span className="truncate">{node.provider.name}</span>
    </span>
  )
}

function Address({ value, className }: { value: string; className?: string }) {
  return (
    <span className={cn('flex min-w-0 items-center gap-1.5 font-mono text-xs text-muted-foreground', className)}>
      <Globe className="size-3 shrink-0" aria-hidden />
      <span className="truncate" title={value}>
        {value}
      </span>
    </span>
  )
}

function Bar({ pct, className, tone = 'ok' }: { pct: number; className?: string; tone?: 'ok' | 'warn' | 'bad' }) {
  const color = tone === 'bad' ? 'bg-red-500' : tone === 'warn' ? 'bg-orange-500' : 'bg-emerald-500'
  return (
    <div className={cn('h-1 overflow-hidden rounded-full bg-border/80', className)}>
      <div className={cn('h-full rounded-full', color)} style={{ width: `${Math.min(100, Math.max(0, pct))}%` }} />
    </div>
  )
}

/** Память: выше 70% — оранжевая, выше 90% — красная (пороги как в панели). */
function memoryTone(pct: number): 'ok' | 'warn' | 'bad' {
  if (pct > 90) return 'bad'
  if (pct > 70) return 'warn'
  return 'ok'
}

/**
 * Цвет одного числа load average. Нагрузка осмысленна только относительно числа ядер:
 * 1.5 на одном ядре — перегруз, на восьми — пустяк. Выше 70% ядер — оранжевый, больше ядер — красный,
 * как в панели. Число ядер неизвестно — не красим.
 */
function loadClass(value: number, cpus: number): string {
  if (cpus <= 0) return ''
  const ratio = value / cpus
  if (ratio > 1) return 'text-red-400'
  if (ratio > 0.7) return 'text-orange-400'
  return ''
}

/** Строка нагрузки: память, load average, скорость вниз и вверх. */
function LoadLine({ node, showLoad }: { node: AdminNodeDTO; showLoad: boolean }) {
  const { t } = useTranslation()
  const live = node.state === 'online'
  const mem = node.memory_used_pct
  const load = node.load_avg?.length ? node.load_avg : null
  return (
    <div className="flex items-center gap-2.5 whitespace-nowrap font-mono text-[11.5px] text-muted-foreground">
      <span className="flex items-center gap-1.5" title={t('admin.nodes.memoryTitle')}>
        <MemoryIcon />
        <Bar pct={live && mem != null ? mem : 0} tone={live && mem != null ? memoryTone(mem) : 'ok'} className="w-14" />
        {live && mem != null ? `${Math.round(mem)}%` : DASH}
      </span>
      {showLoad && (
        <span className="flex items-center gap-1" title={t('admin.nodes.loadTitle')}>
          <Cpu className="size-3 shrink-0" aria-hidden />
          {live && load ? (
            <span className="flex gap-[0.5ch]">
              {load.map((v, i) => (
                <span key={i} className={loadClass(v, node.cpus)}>
                  {v.toFixed(2)}
                </span>
              ))}
            </span>
          ) : (
            DASH
          )}
        </span>
      )}
      <span className="flex items-center gap-1" title={t('admin.nodes.rxTitle')}>
        <ArrowDown className="size-3 shrink-0 text-primary" aria-hidden />
        {live && node.rx_bytes_per_sec != null ? formatBitrate(node.rx_bytes_per_sec) : DASH}
      </span>
      <span className="flex items-center gap-1" title={t('admin.nodes.txTitle')}>
        <ArrowUp className="size-3 shrink-0 text-primary" aria-hidden />
        {live && node.tx_bytes_per_sec != null ? formatBitrate(node.tx_bytes_per_sec) : DASH}
      </span>
    </div>
  )
}

function TrafficBlock({ node }: { node: AdminNodeDTO }) {
  const { t } = useTranslation()
  const limit = node.traffic_limit_bytes
  const pct = limit > 0 ? (node.traffic_used_bytes / limit) * 100 : 100
  const tone = limit > 0 ? (pct >= 95 ? 'bad' : pct >= 80 ? 'warn' : 'ok') : 'ok'
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-1.5 whitespace-nowrap font-mono">
        <b className="text-[12.5px] font-bold">{formatBytes(node.traffic_used_bytes)}</b>
        <span className={cn('text-muted-foreground', limit > 0 ? 'text-[11.5px]' : 'text-sm leading-none')}>
          {limit > 0 ? t('admin.nodes.trafficOf', { limit: formatBytes(limit) }) : '∞'}
        </span>
      </div>
      <div className="mt-1.5 flex items-center gap-2">
        <Bar pct={pct} tone={tone} className="h-[5px] flex-1" />
        {node.traffic_reset_day > 0 && (
          <span
            className="flex items-center gap-1 font-mono text-[11.5px] text-muted-foreground"
            title={t('admin.nodes.resetTitle', { day: node.traffic_reset_day })}
          >
            <RotateCw className="size-3" aria-hidden />
            {node.traffic_reset_day}
          </span>
        )}
      </div>
    </div>
  )
}

function Uptime({ node }: { node: AdminNodeDTO }) {
  const { t } = useTranslation()
  if (node.state !== 'online') {
    return (
      <span className="flex items-center gap-1.5 font-mono text-[11.5px] text-muted-foreground">
        <Sparkle className="size-3" aria-hidden />
        {t(`admin.nodes.stateText.${node.state}`)}
      </span>
    )
  }
  return (
    <span
      className="flex items-center gap-1.5 font-mono text-[13px] font-extrabold text-primary"
      title={t('admin.nodes.uptimeTitle')}
    >
      <Sparkle className="size-3.5" aria-hidden />
      {formatUptime(node.xray_uptime_sec)}
    </span>
  )
}

function Tag({ icon: Icon, children, title }: { icon: LucideIcon; children: ReactNode; title?: string }) {
  return (
    <span className="flex items-center gap-1" title={title}>
      <Icon className="size-3 shrink-0" aria-hidden />
      {children}
    </span>
  )
}

/** Плагин — только если он назначен ноде; версии — только у ноды в сети. */
function Versions({ node, className }: { node: AdminNodeDTO; className?: string }) {
  const { t } = useTranslation()
  const live = node.state === 'online'
  const hasVersions = live && (node.xray_version || node.node_version)
  if (!node.plugin_name && !hasVersions) return null
  return (
    <div className={cn('flex flex-wrap gap-x-2.5 gap-y-1 font-mono text-[11px] text-muted-foreground', className)}>
      {node.plugin_name && (
        <Tag icon={Plug} title={t('admin.nodes.pluginTitle')}>
          {node.plugin_name}
        </Tag>
      )}
      {live && node.xray_version && (
        <Tag icon={Sparkle} title={t('admin.nodes.xrayTitle')}>
          {node.xray_version}
        </Tag>
      )}
      {live && node.node_version && (
        <Tag icon={AudioLines} title={t('admin.nodes.nodeTitle')}>
          {node.node_version}
        </Tag>
      )}
    </div>
  )
}

function Title({ node }: { node: AdminNodeDTO }) {
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <StateBadge state={node.state} />
      <UsersPill count={node.users_online} />
      <CountryFlag code={node.country_code} className="h-[18px] w-[25px]" />
      <span className="truncate text-[15px] font-bold" title={node.name}>
        {node.name}
      </span>
    </div>
  )
}

function NodeCardImpl({ node, layout }: { node: AdminNodeDTO; layout: NodeLayout }) {
  const style = STATE_STYLE[node.state]

  if (layout === 'row') {
    return (
      <div
        className={cn(
          'grid grid-cols-[minmax(0,1fr)_158px_164px_160px] items-center gap-4 rounded-[14px] border py-2.5 pl-3 pr-4',
          style.wrap,
        )}
        style={SOLID_BG}
      >
        <div className="min-w-0 space-y-2">
          <Title node={node} />
          <div className="pl-0.5">
            <LoadLine node={node} showLoad />
          </div>
        </div>
        <div className="flex min-w-0 flex-col items-start gap-1.5">
          <ProviderBadge node={node} />
          <Address value={node.address} className="max-w-full" />
        </div>
        <TrafficBlock node={node} />
        <div className="flex flex-col items-end gap-1.5">
          <Uptime node={node} />
          <Versions node={node} className="justify-end" />
        </div>
      </div>
    )
  }

  return (
    <div className={cn('flex flex-col gap-2.5 rounded-[14px] border px-3 py-3', style.wrap)} style={SOLID_BG}>
      <Title node={node} />
      <div className="flex items-center justify-between gap-2.5">
        <ProviderBadge node={node} />
        <Address value={node.address} className="justify-end" />
      </div>
      <TrafficBlock node={node} />
      <div className="flex items-center justify-between">
        <span className="min-h-4" />
        <Uptime node={node} />
      </div>
      <LoadLine node={node} showLoad={false} />
      {(node.plugin_name || node.state === 'online') && (
        <div className="border-t border-border/50 pt-2">
          <Versions node={node} />
        </div>
      )}
    </div>
  )
}

export const NodeCard = memo(NodeCardImpl)
