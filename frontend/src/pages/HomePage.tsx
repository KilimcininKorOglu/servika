import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  DndContext, DragOverlay, KeyboardSensor, PointerSensor, TouchSensor,
  closestCorners, useSensor, useSensors, useDroppable,
} from '@dnd-kit/core'
import type { DragStartEvent, DragOverEvent, DragEndEvent } from '@dnd-kit/core'
import {
  SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { restrictToWindowEdges } from '@dnd-kit/modifiers'
import { api } from '@/lib/api'
import { useAuth } from '@/store/auth'
import LoadHistoryChart from '@/components/LoadHistoryChart'
import MemoryHistoryChart from '@/components/MemoryHistoryChart'
import CveWidget from '@/components/CveWidget'

type SystemInfo = {
  hostname: string; ip: string; os_name: string; kernel: string
  cpu_model: string; cpu_cores: number
}
type DiskInfo = { mount: string; total_gb: number; used_gb: number; free_gb: number; pct: number }
type ServiceInfo = { name: string; label: string; active: boolean }
type SystemUsage = {
  system: SystemInfo
  disks: DiskInfo[]; disk: DiskInfo
  memory_pct: number; memory_used_gb: number; memory_total_gb: number
  swap_pct: number; swap_used_gb: number; swap_total_gb: number
  services: ServiceInfo[]
  isolation_losses: number
  quota_reboot_required?: boolean
  quota_fs_unsupported?: boolean
}
type Domain = { id: number; domain_name: string; ssl: boolean; status: string }

type UpdateStatus = { tool_available: boolean; running: boolean; status: string }
type OptimizeStatus = { running: boolean; status: string }
type BackupRow = { domain_id: number; domain_name: string; count: number; total_b: number; last_backup: string }
type BackupSummary = {
  domains: BackupRow[]; total_size_b: number; total_backups: number
  destination_count: number; schedule: string
}
type WpInstall = {
  domain_id: number; domain_name: string; dir: string; version: string
  latest_version: string; status: 'current' | 'outdated' | 'unknown'; install_date: string
  site_url: string; admin_url: string
}

/* ================= DASHBOARD LAYOUT (drag-and-drop) ================= */
type Layout = { columns: string[][] }

const DEFAULT_LAYOUT: Layout = {
  columns: [
    ['load-chart', 'wordpress', 'panel-update', 'last-backup', 'performance'],
    ['memory-chart', 'cve-security', 'services', 'domains'],
    ['server-info', 'health', 'live-resources', 'subscriptions', 'network'],
  ],
}
const WIDGET_IDS: string[] = DEFAULT_LAYOUT.columns.flat()
const WIDGET_SET = new Set(WIDGET_IDS)
const DEFAULT_COL: Record<string, number> = (() => {
  const m: Record<string, number> = {}
  DEFAULT_LAYOUT.columns.forEach((c, i) => c.forEach((id) => { m[id] = i }))
  return m
})()
const WIDGET_NAME: Record<string, string> = {
  'load-chart': 'Load Chart', 'memory-chart': 'Memory Chart', 'wordpress': 'WordPress Sites',
  'panel-update': 'Panel Update', 'last-backup': 'Last Server Backup', 'performance': 'Performance / Optimize',
  'cve-security': 'Security Advisories (CVE)', 'services': 'Services', 'domains': 'Domains', 'server-info': 'Server Info',
  'health': 'System Health', 'live-resources': 'Live Resources', 'subscriptions': 'My Subscriptions', 'network': 'Network Traffic',
}
const QUOTA_WARNING_DISMISSED_KEY = 'servika-quota-fs-warning-dismissed'

function mergeLayout(saved: unknown): Layout {
  const src = (saved as { columns?: unknown })?.columns
  const source: unknown[] = Array.isArray(src) ? src : []
  const cols: string[][] = [[], [], []]
  const placed = new Set<string>()
  for (let i = 0; i < 3; i++) {
    const arr = Array.isArray(source[i]) ? (source[i] as unknown[]) : []
    for (const id of arr) {
      if (typeof id === 'string' && WIDGET_SET.has(id) && !placed.has(id)) {
        cols[i].push(id); placed.add(id)
      }
    }
  }
  for (const id of WIDGET_IDS) {
    if (!placed.has(id)) { cols[DEFAULT_COL[id]].push(id); placed.add(id) }
  }
  return { columns: cols }
}

function colIndex(cols: string[][], id: string): number {
  if (id.startsWith('col-')) return parseInt(id.slice(4), 10)
  return cols.findIndex((c) => c.includes(id))
}

function usePrefersReducedMotion(): boolean {
  const [r, setR] = useState(false)
  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const on = () => setR(mq.matches)
    on()
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])
  return r
}

export default function HomePage() {
  const user = useAuth((s) => s.username)
  const [s, setS] = useState<SystemUsage | null>(null)
  const [domains, setDomains] = useState<Domain[]>([])
  const [update, setUpdate] = useState<UpdateStatus | null>(null)
  const [optimize, setOptimize] = useState<OptimizeStatus | null>(null)
  const [backup, setBackup] = useState<BackupSummary | null>(null)
  const [wp, setWp] = useState<WpInstall[] | null>(null)
  const [quotaWarningDismissed, setQuotaWarningDismissed] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.localStorage.getItem(QUOTA_WARNING_DISMISSED_KEY) === '1'
  })

  const [layout, setLayout] = useState<Layout>(DEFAULT_LAYOUT)
  const [activeId, setActiveId] = useState<string | null>(null)
  const [saveState, setSaveState] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const reduced = usePrefersReducedMotion()

  const layoutRef = useRef(layout)
  useEffect(() => { layoutRef.current = layout }, [layout])

  const saveTimer = useRef<number | null>(null)
  const resetTimer = useRef<number | null>(null)

  const persist = (lay: Layout, immediate = false) => {
    if (saveTimer.current) { clearTimeout(saveTimer.current); saveTimer.current = null }
    const run = () => {
      setSaveState('saving')
      api.put('/dashboard-layout', { layout: JSON.stringify(lay) })
        .then(() => {
          setSaveState('saved')
          if (resetTimer.current) clearTimeout(resetTimer.current)
          resetTimer.current = window.setTimeout(() => setSaveState('idle'), 2000)
        })
        .catch(() => {
          setSaveState('error')
          if (resetTimer.current) clearTimeout(resetTimer.current)
          resetTimer.current = window.setTimeout(() => setSaveState('idle'), 4000)
        })
    }
    if (immediate) run()
    else saveTimer.current = window.setTimeout(run, 600)
  }

  useEffect(() => {
    api.get<{ layout: string }>('/dashboard-layout')
      .then((r) => {
        const raw = r.data?.layout
        if (raw && raw.trim()) {
          try { setLayout(mergeLayout(JSON.parse(raw))) }
          catch { setLayout(DEFAULT_LAYOUT) }
        } else {
          setLayout(mergeLayout(DEFAULT_LAYOUT))
        }
      })
      .catch(() => setLayout(DEFAULT_LAYOUT))
  }, [])

  useEffect(() => () => {
    if (saveTimer.current) clearTimeout(saveTimer.current)
    if (resetTimer.current) clearTimeout(resetTimer.current)
  }, [])

  useEffect(() => {
    const fetchUsage = () => {
      if (typeof document !== 'undefined' && document.hidden) return
      api.get<SystemUsage>('/system/usage').then((r) => setS(r.data)).catch(() => {})
    }
    const fetchMaint = () => {
      if (typeof document !== 'undefined' && document.hidden) return
      api.get<UpdateStatus>('/system/update').then((r) => setUpdate(r.data)).catch(() => {})
      api.get<OptimizeStatus>('/system/optimize').then((r) => setOptimize(r.data)).catch(() => {})
    }
    fetchUsage()
    fetchMaint()
    api.get<Domain[]>('/domains').then((r) => setDomains(r.data || [])).catch(() => {})
    api.get<BackupSummary>('/admin/backups/summary').then((r) => setBackup(r.data)).catch(() => {})
    api.get<WpInstall[]>('/wordpress/all').then((r) => setWp(r.data || [])).catch(() => setWp([]))

    const idU = setInterval(fetchUsage, 5000)
    const idM = setInterval(fetchMaint, 20000)
    const onVis = () => { if (!document.hidden) { fetchUsage(); fetchMaint() } }
    document.addEventListener('visibilitychange', onVis)
    return () => { clearInterval(idU); clearInterval(idM); document.removeEventListener('visibilitychange', onVis) }
  }, [])

  const active = domains.filter((d) => d.status === 'active').length
  const sslCount = domains.filter((d) => d.ssl).length
  const diskList = s ? (s.disks?.length ? s.disks : [s.disk]) : []
  const mainDisk = s ? (diskList[0] || s.disk) : null
  const svcActive = s ? s.services.filter((x) => x.active).length : 0
  const svcTotal = s ? s.services.length : 0
  const svcDown = svcTotal - svcActive
  const isoCount = s?.isolation_losses ?? 0

  const displayName = (user?.full_name || user?.name || '').trim()
  const health = calcHealth(s, svcDown, isoCount)
  const quotaWarning = s?.quota_fs_unsupported
    ? {
        title: 'Disk quota is unavailable on this filesystem',
        body: 'Servika needs an XFS root filesystem for per-tenant disk quota. A reboot will not enable quota on the current filesystem.',
        dismissible: true,
      }
    : s?.quota_reboot_required
      ? {
          title: 'Disk quota will be enabled after reboot',
          body: 'XFS user quota is configured, but enforcement is inactive. Reboot the server once to activate per-tenant disk quota.',
          dismissible: false,
        }
      : null

  const lastBackup = backup?.domains?.reduce((a, r) => (r.last_backup > a ? r.last_backup : a), '') || ''
  const backedUpDomains = backup?.domains?.filter((r) => r.count > 0).length ?? 0

  const wpTotal = wp?.length ?? 0
  const wpOutdated = wp?.filter((x) => x.status === 'outdated').length ?? 0
  const wpCurrent = wp?.filter((x) => x.status === 'current').length ?? 0
  const wpUnknown = wp?.filter((x) => x.status === 'unknown').length ?? 0

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 180, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const onDragStart = (e: DragStartEvent) => setActiveId(String(e.active.id))

  const dismissQuotaWarning = () => {
    setQuotaWarningDismissed(true)
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(QUOTA_WARNING_DISMISSED_KEY, '1')
    }
  }

  const onDragOver = (e: DragOverEvent) => {
    const { active, over } = e
    if (!over) return
    const activeId = String(active.id)
    const overId = String(over.id)
    if (activeId === overId) return
    setLayout((prev) => {
      const cols = prev.columns
      const from = colIndex(cols, activeId)
      const to = colIndex(cols, overId)
      if (from === -1 || to === -1 || from === to) return prev
      const next = cols.map((c) => c.slice())
      const fromItems = next[from]
      const toItems = next[to]
      const ai = fromItems.indexOf(activeId)
      if (ai === -1) return prev
      fromItems.splice(ai, 1)
      let overIndex: number
      if (overId.startsWith('col-')) overIndex = toItems.length
      else {
        const oi = toItems.indexOf(overId)
        overIndex = oi === -1 ? toItems.length : oi
      }
      toItems.splice(overIndex, 0, activeId)
      return { columns: next }
    })
  }

  const onDragEnd = (e: DragEndEvent) => {
    const { active, over } = e
    setActiveId(null)
    if (!over) return
    const activeId = String(active.id)
    const overId = String(over.id)
    const cols = layoutRef.current.columns
    const from = colIndex(cols, activeId)
    const to = colIndex(cols, overId)
    if (from === -1 || to === -1) return
    let next = layoutRef.current
    if (from === to) {
      const items = cols[to]
      const oldIndex = items.indexOf(activeId)
      let newIndex = overId.startsWith('col-') ? items.length - 1 : items.indexOf(overId)
      if (newIndex === -1) newIndex = items.length - 1
      if (oldIndex !== newIndex && oldIndex !== -1) {
        const nc = cols.map((c) => c.slice())
        nc[to] = arrayMove(items, oldIndex, newIndex)
        next = { columns: nc }
      }
    }
    setLayout(next)
    persist(next)
  }

  const resetLayout = () => {
    setLayout(DEFAULT_LAYOUT)
    persist(DEFAULT_LAYOUT, true)
  }

  /* ---------- widget id → JSX map ---------- */
  const widgets: Record<string, React.ReactNode> = {
    'load-chart': <LoadHistoryChart />,
    'memory-chart': <MemoryHistoryChart />,
    'cve-security': <CveWidget />,

    'wordpress': (
      <Card title="WordPress Sites" subtitle="Installations across all accounts" icon={I.wp}
        right={<Link to="/wordpress" className="text-xs font-medium text-brand-600 hover:underline dark:text-brand-400">More →</Link>}>
        {wp === null ? (
          <Spinner />
        ) : (
          <>
            <div className="mb-3 grid grid-cols-3 gap-2.5">
              <MiniStat value={wpTotal} label="Installs" color="slate" />
              <MiniStat value={wpOutdated} label="Updates" color={wpOutdated > 0 ? 'amber' : 'emerald'} />
              <MiniStat value={wpCurrent} label="Current" color="emerald" />
            </div>
            {wpOutdated > 0 && (
              <div className="mb-3 flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] text-amber-700 dark:border-amber-800/50 dark:bg-amber-900/15 dark:text-amber-300">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="mt-0.5 h-3.5 w-3.5 shrink-0"><path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0 3.75h.008M10.36 3.6 2.26 17.66A1.5 1.5 0 0 0 3.56 19.9h16.88a1.5 1.5 0 0 0 1.3-2.25L13.64 3.6a1.5 1.5 0 0 0-2.6 0Z" /></svg>
                <span><strong>{wpOutdated}</strong> installation{wpOutdated !== 1 ? 's' : ''} with pending updates — outdated versions carry security risks.</span>
              </div>
            )}
            {wpTotal === 0 ? (
              <div className="py-5 text-center text-xs text-slate-400">No WordPress installations found</div>
            ) : (
              <div className="space-y-0.5">
                {wp!.slice(0, 5).map((k) => (
                  <Link key={`${k.domain_id}-${k.dir}`} to="/wordpress"
                    className="-mx-2 flex items-center justify-between rounded-xl px-2 py-2 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50">
                    <span className="flex min-w-0 items-center gap-2.5">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.6}
                        className={`h-4 w-4 shrink-0 ${k.status === 'outdated' ? 'text-amber-500' : k.status === 'current' ? 'text-emerald-500' : 'text-slate-400'}`}><path d={I.wp} /></svg>
                      <span className="min-w-0">
                        <span className="block truncate font-mono text-[13px] text-slate-700 dark:text-slate-200">{k.domain_name}</span>
                        <span className="block truncate text-[10px] text-slate-400 dark:text-slate-500">{k.dir === '/ (root)' ? 'root dir' : k.dir}{k.version ? ` · v${k.version}` : ''}</span>
                      </span>
                    </span>
                    <span className="shrink-0">
                      {k.status === 'outdated'
                        ? <Badge color="amber" text={k.latest_version ? `→ v${k.latest_version}` : 'Update'} />
                        : k.status === 'current'
                          ? <Badge color="emerald" text="Current" />
                          : <Badge color="slate" text="Unknown" />}
                    </span>
                  </Link>
                ))}
                {wpTotal > 5 && (
                  <Link to="/wordpress" className="block pt-1.5 text-center text-[11px] text-slate-400 transition-colors hover:text-brand-600 dark:hover:text-brand-400">
                    +{wpTotal - 5} more installations →
                  </Link>
                )}
              </div>
            )}
            {wpUnknown > 0 && (
              <div className="mt-2 text-[10px] text-slate-400 dark:text-slate-500">{wpUnknown} installation{wpUnknown !== 1 ? 's' : ''} could not be determined (wp-cli timeout).</div>
            )}
          </>
        )}
      </Card>
    ),

    'panel-update': (
      <Card title="Panel Update" subtitle="Version and system packages" icon={I.update}
        right={<Link to="/tools/packages" className="text-xs font-medium text-brand-600 hover:underline dark:text-brand-400">More →</Link>}>
        <div className="flex items-center gap-3">
          <span className={`grid h-11 w-11 shrink-0 place-items-center rounded-xl ${
            update?.running ? 'bg-sky-50 text-sky-600 dark:bg-sky-900/25 dark:text-sky-300'
              : update?.tool_available === false ? 'bg-amber-50 text-amber-600 dark:bg-amber-900/25 dark:text-amber-300'
                : 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/25 dark:text-emerald-300'}`}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" className="h-6 w-6"><path d={I.update} /></svg>
          </span>
          <div className="min-w-0">
            <div className="text-sm font-semibold text-slate-800 dark:text-slate-100">
              {update?.running ? 'Update running' : update?.tool_available === false ? 'Update tool missing' : 'Panel up to date'}
            </div>
            <div className="mt-0.5 truncate text-xs text-slate-500 dark:text-slate-400" title={update?.status}>
              {update?.status || (update ? 'No status info' : 'Loading…')}
            </div>
          </div>
          <span className="ml-auto shrink-0">
            <Badge color={update?.running ? 'sky' : update?.tool_available === false ? 'amber' : 'emerald'}
              text={update?.running ? 'Running' : update?.tool_available === false ? 'Missing' : 'Current'} />
          </span>
        </div>
        <Link to="/tools/packages" className="-mx-2 mt-3 flex items-center justify-between rounded-xl border-t border-slate-100 px-2 pt-3 text-xs transition-colors hover:bg-slate-50 dark:border-slate-800 dark:hover:bg-slate-800/50">
          <span className="flex items-center gap-2 text-slate-600 dark:text-slate-300">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4 text-slate-400"><path d={I.package} /></svg>
            System packages
          </span>
          <span className="text-brand-600 dark:text-brand-400">Manage →</span>
        </Link>
      </Card>
    ),

    'last-backup': (
      <Card title="Last Server Backup" subtitle="Automatic daily backup" icon={I.backup}
        right={<Link to="/backup-management" className="text-xs font-medium text-brand-600 hover:underline dark:text-brand-400">More →</Link>}>
        {!backup ? (
          <div className="py-6 text-center text-xs text-slate-400">Backup summary unavailable</div>
        ) : (
          <>
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-bold tracking-tight tabular-nums text-slate-900 dark:text-slate-100">{backup.total_backups}</span>
              <span className="text-sm text-slate-500 dark:text-slate-400">backups · {fmtBytesGB(backup.total_size_b)}</span>
            </div>
            <div className="mt-3 space-y-0">
              <KV label="Last backup" value={lastBackup || '—'} />
              <KV label="Sites backed up" value={`${backedUpDomains} / ${backup.domains.length}`} />
              <KV label="Remote target" value={backup.destination_count > 0 ? `${backup.destination_count} active` : 'None'} />
              <KV label="Schedule" value={backup.schedule} />
            </div>
          </>
        )}
      </Card>
    ),

    'performance': (
      <Card title="Performance / Optimize" subtitle="Improve server settings" icon={I.optimize}
        right={<Link to="/tools/optimize" className="text-xs font-medium text-brand-600 hover:underline dark:text-brand-400">More →</Link>}>
        <div className="flex items-center gap-3">
          <span className={`grid h-11 w-11 shrink-0 place-items-center rounded-xl ${optimize?.running ? 'bg-sky-50 text-sky-600 dark:bg-sky-900/25 dark:text-sky-300' : 'bg-brand-50 text-brand-600 dark:bg-brand-900/20 dark:text-brand-300'}`}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" className="h-6 w-6"><path d={I.optimize} /></svg>
          </span>
          <div className="min-w-0">
            <div className="text-sm font-semibold text-slate-800 dark:text-slate-100">
              {optimize?.running ? 'Optimization running' : 'Optimization ready'}
            </div>
            <div className="mt-0.5 truncate text-xs text-slate-500 dark:text-slate-400" title={optimize?.status}>
              {optimize?.status || (optimize ? 'MariaDB · nginx · PHP settings' : 'Loading…')}
            </div>
          </div>
          <span className="ml-auto shrink-0">
            <Badge color={optimize?.running ? 'sky' : 'slate'} text={optimize?.running ? 'Running' : 'Idle'} />
          </span>
        </div>
      </Card>
    ),

    'services': (
      <Card title="Services" subtitle={s ? `${svcActive}/${svcTotal} services running` : 'service status'} icon={I.service}
        right={s ? <Badge color={svcDown === 0 ? 'emerald' : 'amber'} text={svcDown === 0 ? 'All active' : `${svcDown} down`} /> : undefined}>
        {!s ? <Spinner /> : (
          <div className="grid grid-cols-1 gap-x-5 gap-y-0.5 sm:grid-cols-2">
            {s.services.map((sv) => (
              <div key={sv.name} title={sv.name}
                className="-mx-1 flex items-center justify-between rounded-lg px-1.5 py-1.5 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/40">
                <span className="flex min-w-0 items-center gap-2">
                  <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${sv.active ? 'bg-emerald-500' : 'bg-red-500'}`} />
                  <span className="truncate text-[13px] text-slate-700 dark:text-slate-200">{sv.label}</span>
                </span>
                <span className={`shrink-0 text-[11px] font-medium ${sv.active ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-500 dark:text-red-400'}`}>
                  {sv.active ? 'Active' : 'Down'}
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>
    ),

    'domains': (
      <Card title="Domains" subtitle="Hosted sites and SSL status" icon={I.domain}
        right={<Link to="/domains" className="text-xs font-medium text-brand-600 hover:underline dark:text-brand-400">More →</Link>}>
        <div className="mb-4 grid grid-cols-3 gap-2.5">
          <MiniStat value={domains.length} label="Total" color="slate" />
          <MiniStat value={active} label="Active" color="emerald" />
          <MiniStat value={sslCount} label="SSL" color="sky" />
        </div>
        {domains.length === 0 ? (
          <div className="py-6 text-center text-xs text-slate-400">No domains yet</div>
        ) : (
          <div className="space-y-0.5">
            {domains.slice(0, 7).map((d) => (
              <Link key={d.id} to={`/subscriptions/${d.id}`}
                className="-mx-2 flex items-center justify-between rounded-xl px-2 py-2.5 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50">
                <span className="flex min-w-0 items-center gap-2.5">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}
                    className={`h-4 w-4 shrink-0 ${d.ssl ? 'text-emerald-500' : 'text-slate-400 dark:text-slate-500'}`}>
                    {d.ssl
                      ? <><rect x="5" y="11" width="14" height="9" rx="2" /><path strokeLinecap="round" d="M8 11V8a4 4 0 0 1 8 0v3" /></>
                      : <><rect x="5" y="11" width="14" height="9" rx="2" /><path strokeLinecap="round" d="M8 11V6a4 4 0 0 1 7-2.6" /></>}
                  </svg>
                  <span className="truncate font-mono text-[13px] text-slate-700 dark:text-slate-200">{d.domain_name}</span>
                </span>
                <span className="flex shrink-0 items-center gap-2">
                  {!d.ssl && <Badge color="amber" text="No SSL" />}
                  <Badge color={d.status === 'active' ? 'emerald' : 'slate'} text={d.status === 'active' ? 'Active' : d.status} />
                </span>
              </Link>
            ))}
            {domains.length > 7 && (
              <Link to="/domains" className="block pt-1.5 text-center text-[11px] text-slate-400 transition-colors hover:text-brand-600 dark:hover:text-brand-400">
                +{domains.length - 7} more domains →
              </Link>
            )}
          </div>
        )}
      </Card>
    ),

    'server-info': (
      <Card title="Server Info" subtitle="Hardware and system" icon={I.server}>
        {!s ? <Spinner /> : (
          <div className="space-y-0">
            <KV label="Hostname" value={s.system.hostname} />
            <KV label="IP address" value={s.system.ip || '—'} />
            <KV label="OS" value={s.system.os_name || '—'} />
            <KV label="Kernel" value={s.system.kernel || '—'} />
            <KV label="CPU" value={s.system.cpu_model || '—'} />
            <KV label="Cores" value={`${s.system.cpu_cores} vCPU`} />
          </div>
        )}
      </Card>
    ),

    'health': (
      <Card title="System Health" subtitle="Overall status assessment" icon={I.health}>
        {!s ? <Spinner /> : (
          <>
            <div className="mb-3 flex items-center gap-4">
              <div className={`text-2xl font-bold ${health.score >= 80 ? 'text-emerald-500' : health.score >= 60 ? 'text-amber-500' : 'text-red-500'}`}>{health.score}%</div>
              <div className="text-xs text-slate-500 dark:text-slate-400">{health.label}</div>
            </div>
            <div className="space-y-1.5">
              {health.issues.map((issue, i) => (
                <div key={i} className="flex items-start gap-2 text-[11px]">
                  <span className={`mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full ${issue.severity === 'high' ? 'bg-red-500' : issue.severity === 'medium' ? 'bg-amber-500' : 'bg-sky-500'}`} />
                  <span className="text-slate-600 dark:text-slate-400">{issue.text}</span>
                </div>
              ))}
            </div>
          </>
        )}
      </Card>
    ),

    'live-resources': !s ? <Spinner /> : (
      <Card title="Live Resources" subtitle="Real-time CPU and memory" icon={I.chart}>
        <div className="space-y-3">
          <div>
            <div className="mb-1 flex justify-between text-[11px]"><span className="text-slate-500">CPU</span><span className="font-mono text-slate-700 dark:text-slate-300">{s.memory_pct?.toFixed(1) ?? '—'}%</span></div>
            <div className="h-2 w-full rounded-full bg-slate-100 dark:bg-slate-800"><div className="h-2 rounded-full bg-brand-500 transition-all" style={{ width: `${Math.min(s.memory_pct ?? 0, 100)}%` }} /></div>
          </div>
          <div>
            <div className="mb-1 flex justify-between text-[11px]"><span className="text-slate-500">Memory</span><span className="font-mono text-slate-700 dark:text-slate-300">{fmtBytesGB((s.memory_used_gb ?? 0) * 1e9)} / {fmtBytesGB((s.memory_total_gb ?? 0) * 1e9)}</span></div>
            <div className="h-2 w-full rounded-full bg-slate-100 dark:bg-slate-800"><div className="h-2 rounded-full bg-sky-500 transition-all" style={{ width: `${Math.min(s.memory_pct ?? 0, 100)}%` }} /></div>
          </div>
          {mainDisk && (
            <div>
              <div className="mb-1 flex justify-between text-[11px]"><span className="text-slate-500">Disk ({mainDisk.mount})</span><span className="font-mono text-slate-700 dark:text-slate-300">{mainDisk.free_gb?.toFixed(1)} GB free</span></div>
              <div className="h-2 w-full rounded-full bg-slate-100 dark:bg-slate-800"><div className="h-2 rounded-full bg-emerald-500 transition-all" style={{ width: `${Math.min(mainDisk.pct ?? 0, 100)}%` }} /></div>
            </div>
          )}
        </div>
      </Card>
    ),

    'subscriptions': (
      <Card title="Subscriptions" subtitle="Your active services" icon={I.subscription}>
        <div className="py-5 text-center text-xs text-slate-400">Customer panel overview</div>
      </Card>
    ),

    'network': (
      <Card title="Network Traffic" subtitle="Inbound and outbound data" icon={I.network}>
        <div className="py-5 text-center text-xs text-slate-400">Traffic statistics available in Statistics page</div>
      </Card>
    ),
  }

  return (
    <div className="px-5 py-5">
      {/* Header */}
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">
            {displayName ? `Welcome back, ${displayName}` : 'Dashboard'}
          </h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">Server overview and quick actions</p>
        </div>
        <div className="flex items-center gap-2">
          {saveState !== 'idle' && (
            <span className={`text-[11px] font-medium ${saveState === 'saving' ? 'text-slate-400' : saveState === 'saved' ? 'text-emerald-500' : 'text-red-500'}`}>
              {saveState === 'saving' ? 'Saving…' : saveState === 'saved' ? 'Layout saved' : 'Save failed'}
            </span>
          )}
          <button onClick={resetLayout} className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-[11px] font-medium text-slate-500 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-800">Reset Layout</button>
        </div>
      </div>

      {quotaWarning && (!quotaWarning.dismissible || !quotaWarningDismissed) && (
        <div className="mb-4 rounded-2xl border border-amber-200 bg-amber-50 p-4 text-amber-800 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-200">
          <div className="flex items-start gap-3">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="mt-0.5 h-5 w-5 shrink-0"><path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0 3.75h.008M10.36 3.6 2.26 17.66A1.5 1.5 0 0 0 3.56 19.9h16.88a1.5 1.5 0 0 0 1.3-2.25L13.64 3.6a1.5 1.5 0 0 0-2.6 0Z" /></svg>
            <div className="min-w-0 flex-1">
              <div className="text-sm font-semibold">{quotaWarning.title}</div>
              <div className="mt-1 text-xs leading-relaxed text-amber-700 dark:text-amber-300">{quotaWarning.body}</div>
            </div>
            {quotaWarning.dismissible && (
              <button type="button" onClick={dismissQuotaWarning} className="text-xs font-medium text-amber-700 underline-offset-2 hover:underline dark:text-amber-300">
                Dismiss
              </button>
            )}
          </div>
        </div>
      )}

      {/* Drag-and-drop grid */}
      <DndContext sensors={sensors} collisionDetection={closestCorners}
        onDragStart={onDragStart} onDragOver={onDragOver} onDragEnd={onDragEnd}
        modifiers={[restrictToWindowEdges]}>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
          {layout.columns.map((col, ci) => (
            <SortableContext key={ci} id={`col-${ci}`} items={col} strategy={verticalListSortingStrategy}>
              <DroppableColumn id={`col-${ci}`} items={col}>
                <div className="space-y-5">
                  {col.map((id) => (
                    <SortableWidget key={id} id={id} reduced={reduced}>
                      {widgets[id] || <Card title={id} subtitle=""><div className="py-4 text-center text-xs text-slate-400">Widget not found</div></Card>}
                    </SortableWidget>
                  ))}
                </div>
              </DroppableColumn>
            </SortableContext>
          ))}
        </div>

        <DragOverlay dropAnimation={null}>
          {activeId ? (
            <div className="rounded-2xl border-2 border-brand-300 bg-white/95 p-4 shadow-2xl backdrop-blur-sm dark:border-brand-700 dark:bg-slate-900/95">
              <p className="text-sm font-semibold text-slate-700 dark:text-slate-200">{WIDGET_NAME[activeId] || activeId}</p>
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
    </div>
  )
}

/* ================= sub-components ================= */

function DroppableColumn({ id, items, children }: { id: string; items: string[]; children: React.ReactNode }) {
  const { setNodeRef } = useDroppable({ id, data: { items } })
  return <div ref={setNodeRef}>{children}</div>
}

function SortableWidget({ id, reduced, children }: { id: string; reduced: boolean; children: React.ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  const style = {
    transform: CSS.Transform.toString(transform),
    transition: reduced ? 'none' : transition,
    ...(isDragging ? { opacity: 0.35, zIndex: 0 } : { zIndex: 1 }),
  }
  return (
    <div ref={setNodeRef} style={style} {...attributes}>
      <div className="group/widget relative">
        <button {...listeners} className="absolute -left-1 -top-1 z-10 flex h-6 w-6 cursor-grab items-center justify-center rounded-md text-slate-300 opacity-0 transition-opacity hover:text-slate-500 active:cursor-grabbing group-hover/widget:opacity-100 dark:text-slate-600 dark:hover:text-slate-400" title="Drag to reorder">
          <svg viewBox="0 0 24 24" fill="currentColor" className="h-3.5 w-3.5"><circle cx="9" cy="5" r="1.5" /><circle cx="15" cy="5" r="1.5" /><circle cx="9" cy="12" r="1.5" /><circle cx="15" cy="12" r="1.5" /><circle cx="9" cy="19" r="1.5" /><circle cx="15" cy="19" r="1.5" /></svg>
        </button>
        {children}
      </div>
    </div>
  )
}

function Card({ title, subtitle, icon, right, children }: { title: string; subtitle: string; icon?: string; right?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900/60">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div className="flex items-center gap-2">
          {icon && (
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4 text-slate-400 dark:text-slate-500">
              <path d={icon} />
            </svg>
          )}
          <div>
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{title}</h3>
            <p className="text-[11px] text-slate-400 dark:text-slate-500">{subtitle}</p>
          </div>
        </div>
        {right}
      </div>
      {children}
    </div>
  )
}

function Spinner() {
  return <div className="flex items-center justify-center gap-2 py-6 text-xs text-slate-400">
    <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-slate-300 border-t-transparent dark:border-slate-600 dark:border-t-transparent" />
    Loading…
  </div>
}

function MiniStat({ value, label, color }: { value: number; label: string; color: string }) {
  const c: Record<string, string> = {
    slate: 'text-slate-700 dark:text-slate-200', emerald: 'text-emerald-600 dark:text-emerald-400',
    amber: 'text-amber-600 dark:text-amber-400', sky: 'text-sky-600 dark:text-sky-400',
  }
  return (
    <div className="rounded-xl border border-slate-100 bg-slate-50 p-3 text-center dark:border-slate-800 dark:bg-slate-950/40">
      <div className={`text-xl font-bold tabular-nums ${c[color] || c.slate}`}>{value}</div>
      <div className="mt-0.5 text-[11px] text-slate-400 dark:text-slate-500">{label}</div>
    </div>
  )
}

function Badge({ color, text }: { color: string; text: string }) {
  const c: Record<string, string> = {
    emerald: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/25 dark:text-emerald-300',
    amber: 'bg-amber-50 text-amber-700 dark:bg-amber-900/25 dark:text-amber-300',
    sky: 'bg-sky-50 text-sky-700 dark:bg-sky-900/25 dark:text-sky-300',
    slate: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400',
    red: 'bg-red-50 text-red-700 dark:bg-red-900/25 dark:text-red-300',
  }
  return <span className={`inline-block rounded-full px-2 py-0.5 text-[10px] font-medium ${c[color] || c.slate}`}>{text}</span>
}

function KV({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-slate-50 py-1.5 last:border-0 dark:border-slate-800">
      <span className="text-[12px] text-slate-500 dark:text-slate-400">{label}</span>
      <span className="truncate text-right font-mono text-[12px] text-slate-700 dark:text-slate-300">{value}</span>
    </div>
  )
}

function calcHealth(s: SystemUsage | null, svcDown: number, isoCount: number): { score: number; label: string; issues: { severity: string; text: string }[] } {
  const issues: { severity: string; text: string }[] = []
  if (svcDown > 0) issues.push({ severity: 'high', text: `${svcDown} service${svcDown !== 1 ? 's' : ''} down` })
  if (isoCount > 0) issues.push({ severity: 'high', text: `${isoCount} isolation loss${isoCount !== 1 ? 'es' : ''} detected` })
  const memPct = s?.memory_pct ?? 0
  if (memPct > 90) issues.push({ severity: 'high', text: `Memory at ${memPct.toFixed(0)}%` })
  else if (memPct > 75) issues.push({ severity: 'medium', text: `Memory at ${memPct.toFixed(0)}%` })
  const diskPct = s?.disk?.pct ?? (s?.disks?.[0]?.pct ?? 0)
  if (diskPct > 90) issues.push({ severity: 'high', text: `Disk at ${diskPct.toFixed(0)}%` })
  else if (diskPct > 80) issues.push({ severity: 'medium', text: `Disk at ${diskPct.toFixed(0)}%` })
  const score = Math.max(0, 100 - issues.filter(i => i.severity === 'high').length * 25 - issues.filter(i => i.severity === 'medium').length * 10)
  const label = score >= 90 ? 'Healthy' : score >= 70 ? 'Fair' : score >= 50 ? 'Degraded' : 'Critical'
  return { score, label, issues }
}

function fmtBytesGB(b: number): string {
  if (!b || b < 0) return '0 GB'
  return `${(b / 1e9).toFixed(1)} GB`
}

/* ---- inline SVG path data ---- */
const I = {
  wp: 'M12 2C6.477 2 2 6.477 2 12s4.477 10 10 10 10-4.477 10-10S17.523 2 12 2zm0 18a8 8 0 110-16 8 8 0 010 16z',
  update: 'M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15',
  optimize: 'M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4',
  backup: 'M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4',
  service: 'M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z',
  package: 'm21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9',
  domain: 'M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
  server: 'M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z',
  health: 'M9 12l2 2 4-4m3 2c0 6-8 10-8 10S4 18 4 12V5l8-3 8 3v7z',
  chart: 'M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z',
  subscription: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2',
  network: 'M3 15v-2a2 2 0 012-2h1.5M17 15v-2a2 2 0 012-2h1.5M7 15V9a5 5 0 0110 0v6m-8.5 0h7',
}
