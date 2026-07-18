import { useEffect, useState } from 'react'
import { api } from '@/lib/api'

type Entry = { name: string; path: string; type: 'folder' | 'file' | 'symlink' }
type ListResp = { path: string; content: Entry[] }

interface Props {
  domainId: number | string
  selected: string
  onSelect: (path: string) => void
  refreshKey?: number // re-fetch when this counter changes (after new folder/delete)
}

export default function DirTree({ domainId, selected, onSelect, refreshKey }: Props) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-2 text-sm overflow-auto min-h-[400px]">
      <TreeNode
        domainId={domainId}
        path="/"
        name="~"
        selected={selected}
        onSelect={onSelect}
        initiallyOpen={true}
        depth={0}
        refreshKey={refreshKey}
      />
    </div>
  )
}

function TreeNode({
  domainId, path, name, selected, onSelect, initiallyOpen, depth, refreshKey
}: {
  domainId: number | string
  path: string
  name: string
  selected: string
  onSelect: (path: string) => void
  initiallyOpen: boolean
  depth: number
  refreshKey?: number
}) {
  const [open, setOpen] = useState(initiallyOpen)
  const [folders, setFolders] = useState<Entry[]>([])
  const [loaded, setLoaded] = useState(false)
  const [loading, setLoading] = useState(false)

  function fetchChildren() {
    setLoading(true)
    api.get<ListResp>(`/domains/${domainId}/files`, { params: { path } })
      .then(r => setFolders(r.data.content.filter(e => e.type === 'folder')))
      .catch(() => setFolders([]))
      .finally(() => { setLoaded(true); setLoading(false) })
  }

  useEffect(() => {
    if (initiallyOpen && !loaded) fetchChildren()
  }, []) // eslint-disable-line

  // Re-fetch if the refresh counter changes and we already have data
  useEffect(() => {
    if (loaded) fetchChildren()
  }, [refreshKey]) // eslint-disable-line

  function handleChevronClick(e: React.MouseEvent) {
    e.stopPropagation()
    if (!open && !loaded) fetchChildren()
    setOpen(!open)
  }

  const isSelected = path === selected || (path === '/' && (selected === '' || selected === '/'))
  const hasChildren = !loaded || folders.length > 0

  return (
    <div>
      <div
        onClick={() => onSelect(path)}
        className={`flex items-center gap-1 px-2 py-1 rounded cursor-pointer transition ${
          isSelected ? 'bg-brand-50 dark:bg-brand-900/20 text-brand-700 dark:text-brand-300' : 'hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300'
        }`}
        style={{ paddingLeft: 8 + depth * 14 }}
        title={path}
      >
        {hasChildren ? (
          <button
            onClick={handleChevronClick}
            className="w-4 h-4 flex items-center justify-center text-slate-400 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300"
          >
            <svg
              className={`w-3 h-3 transition-transform ${open ? 'rotate-90' : ''}`}
              fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </button>
        ) : (
          <span className="w-4" />
        )}
        <svg className="w-4 h-4 text-amber-500 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
          <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
        </svg>
        <span className="truncate text-sm">{name}</span>
      </div>

      {open && (
        <div>
          {loading && folders.length === 0 && (
            <div className="px-3 py-1 text-xs text-slate-400 dark:text-slate-500" style={{ paddingLeft: 24 + depth * 14 }}>
              loading…
            </div>
          )}
          {folders.map(k => (
            <TreeNode
              key={k.path}
              domainId={domainId}
              path={k.path}
              name={k.name}
              selected={selected}
              onSelect={onSelect}
              initiallyOpen={false}
              depth={depth + 1}
              refreshKey={refreshKey}
            />
          ))}
        </div>
      )}
    </div>
  )
}