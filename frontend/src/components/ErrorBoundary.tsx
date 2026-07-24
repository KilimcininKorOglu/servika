import { Component, type ReactNode } from 'react'

interface Props { children: ReactNode }
interface State { error: Error | null }

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error) {
    // Keep technical details in the console for debugging without showing them to users.
    console.error('ErrorBoundary caught an error:', error)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="min-h-screen flex items-center justify-center bg-slate-50 dark:bg-slate-900 px-4">
          <div className="max-w-md text-center">
            <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100 mb-2">Unexpected Error</h1>
            <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
              The application encountered an unexpected error. Please try refreshing the page.
            </p>
            {/* Raw exception text is shown only in development, never to end users. */}
            {import.meta.env.DEV && (
              <pre className="text-xs text-left text-rose-600 bg-rose-50 dark:bg-rose-950 dark:text-rose-400 rounded-lg p-3 mb-4 overflow-auto max-h-32">
                {this.state.error.message}
              </pre>
            )}
            <button
              onClick={() => window.location.reload()}
              className="inline-flex items-center px-4 py-2 text-sm font-medium text-white bg-brand-600 hover:bg-brand-700 rounded-lg transition-colors"
            >
              Reload Page
            </button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
