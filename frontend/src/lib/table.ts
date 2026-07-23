const baseContainer = 'rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800'

export const tableContainerClass = `overflow-x-auto ${baseContainer}`

export const tableClass = 'min-w-full divide-y divide-slate-200 dark:divide-slate-700 text-sm'

export const tableHeadCellClass = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400'

export const tableCellClass = 'px-4 py-3 align-middle text-slate-700 dark:text-slate-300'

export const tableActionCellClass = `${tableCellClass} whitespace-nowrap text-right`

export const responsiveTableContainerClass = `responsive-table ${baseContainer} overflow-hidden`

export const responsiveTableClass = 'block w-full text-left lg:table'

export const responsiveTableHeadClass = 'hidden bg-slate-50 text-xs uppercase tracking-wider text-slate-500 dark:bg-slate-900/50 lg:table-header-group'

export const responsiveTableBodyClass = 'block divide-y divide-slate-100 dark:divide-slate-800 lg:table-row-group'

export const responsiveTableRowClass = 'block p-3 transition hover:bg-slate-50 dark:hover:bg-slate-800/60 lg:table-row lg:p-0'

export const responsiveTableCellClass = 'flex items-start justify-between gap-4 py-2 text-sm text-slate-700 dark:text-slate-300 lg:table-cell lg:px-4 lg:py-2.5 lg:align-middle'

export const responsiveTableCodeCellClass = `${responsiveTableCellClass} font-mono text-slate-800 dark:text-slate-200`

export const responsiveTableActionCellClass = `${responsiveTableCellClass} justify-end gap-2 pt-3 lg:text-right`
