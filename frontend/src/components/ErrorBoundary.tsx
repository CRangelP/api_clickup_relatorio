import { Component, ErrorInfo, ReactNode } from 'react'

interface Props {
  children: ReactNode
  fallback?: ReactNode
  onError?: (error: Error, errorInfo: ErrorInfo) => void
  tabId?: string
  isGlobal?: boolean
}

interface State {
  hasError: boolean
  error: Error | null
  errorInfo: ErrorInfo | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null, errorInfo: null }
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    const context = this.props.isGlobal ? 'global' : `tab ${this.props.tabId || 'unknown'}`
    console.error(`Error in ${context}:`, error, errorInfo)
    this.setState({ errorInfo })
    this.props.onError?.(error, errorInfo)
  }

  handleRetry = () => {
    this.setState({ hasError: false, error: null, errorInfo: null })
  }

  handleReload = () => {
    window.location.reload()
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback
      }

      // Global error boundary - full page error
      if (this.props.isGlobal) {
        return (
          <div className="min-h-screen bg-gray-100 flex items-center justify-center p-4" data-testid="global-error-boundary">
            <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
              <div className="mx-auto flex items-center justify-center h-16 w-16 rounded-full bg-red-100 mb-4">
                <svg className="h-8 w-8 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <h2 className="text-xl font-semibold text-gray-900 mb-2">
                Algo deu errado
              </h2>
              <p className="text-gray-600 mb-4">
                Ocorreu um erro inesperado na aplicação. Por favor, tente recarregar a página.
              </p>
              <div className="bg-red-50 border border-red-200 rounded-md p-3 mb-4 text-left">
                <p className="text-sm text-red-700 font-mono break-all">
                  {this.state.error?.message || 'Erro desconhecido'}
                </p>
              </div>
              <div className="flex flex-col sm:flex-row gap-3 justify-center">
                <button
                  onClick={this.handleRetry}
                  className="px-4 py-2 bg-gray-100 text-gray-700 rounded-md hover:bg-gray-200 transition-colors"
                  data-testid="error-retry-btn"
                >
                  Tentar novamente
                </button>
                <button
                  onClick={this.handleReload}
                  className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors"
                  data-testid="error-reload-btn"
                >
                  Recarregar página
                </button>
              </div>
            </div>
          </div>
        )
      }

      // Tab-level error boundary - inline error
      return (
        <div className="p-4 bg-red-50 border border-red-200 rounded-lg" data-testid="tab-error-boundary">
          <div className="flex items-start">
            <div className="flex-shrink-0">
              <svg className="h-5 w-5 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div className="ml-3 flex-1">
              <h3 className="text-red-800 font-medium">Ocorreu um erro nesta aba</h3>
              <p className="text-red-600 text-sm mt-1">
                {this.state.error?.message || 'Erro desconhecido'}
              </p>
              <p className="text-red-500 text-xs mt-2">
                As outras abas continuam funcionando normalmente.
              </p>
            </div>
          </div>
          <div className="mt-4">
            <button
              onClick={this.handleRetry}
              className="px-4 py-2 bg-red-600 text-white text-sm rounded hover:bg-red-700 transition-colors"
              data-testid="tab-error-retry-btn"
            >
              Tentar novamente
            </button>
          </div>
        </div>
      )
    }

    return this.props.children
  }
}

/**
 * Global Error Boundary wrapper component
 * Wraps the entire application to catch unhandled errors
 * Requirements: 13.4
 */
export function GlobalErrorBoundary({ children }: { children: ReactNode }) {
  return (
    <ErrorBoundary isGlobal={true}>
      {children}
    </ErrorBoundary>
  )
}

export default ErrorBoundary
