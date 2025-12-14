/**
 * Toast Context
 * 
 * Provides toast notification functionality for displaying API errors,
 * success messages, and other notifications across the application.
 * 
 * Requirements: 2.3, 3.3, 8.4
 */

import { createContext, useContext, useState, useCallback, ReactNode } from 'react'

export type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface Toast {
  id: string
  type: ToastType
  message: string
  title?: string
  duration?: number
}

interface ToastContextType {
  toasts: Toast[]
  addToast: (toast: Omit<Toast, 'id'>) => string
  removeToast: (id: string) => void
  clearAllToasts: () => void
  // Convenience methods
  showSuccess: (message: string, title?: string) => string
  showError: (message: string, title?: string) => string
  showWarning: (message: string, title?: string) => string
  showInfo: (message: string, title?: string) => string
}

const ToastContext = createContext<ToastContextType | undefined>(undefined)

// Generate unique ID for toasts
let toastIdCounter = 0
const generateToastId = () => `toast-${++toastIdCounter}-${Date.now()}`

// Default durations by type (in ms)
const DEFAULT_DURATIONS: Record<ToastType, number> = {
  success: 4000,
  error: 6000,
  warning: 5000,
  info: 4000,
}

interface ToastProviderProps {
  children: ReactNode
}

export function ToastProvider({ children }: ToastProviderProps) {
  const [toasts, setToasts] = useState<Toast[]>([])

  const removeToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(toast => toast.id !== id))
  }, [])

  const addToast = useCallback((toast: Omit<Toast, 'id'>): string => {
    const id = generateToastId()
    const duration = toast.duration ?? DEFAULT_DURATIONS[toast.type]
    
    const newToast: Toast = {
      ...toast,
      id,
      duration,
    }

    setToasts(prev => [...prev, newToast])

    // Auto-remove after duration
    if (duration > 0) {
      setTimeout(() => {
        removeToast(id)
      }, duration)
    }

    return id
  }, [removeToast])

  const clearAllToasts = useCallback(() => {
    setToasts([])
  }, [])

  // Convenience methods
  const showSuccess = useCallback((message: string, title?: string) => {
    return addToast({ type: 'success', message, title })
  }, [addToast])

  const showError = useCallback((message: string, title?: string) => {
    return addToast({ type: 'error', message, title: title || 'Erro' })
  }, [addToast])

  const showWarning = useCallback((message: string, title?: string) => {
    return addToast({ type: 'warning', message, title: title || 'Atenção' })
  }, [addToast])

  const showInfo = useCallback((message: string, title?: string) => {
    return addToast({ type: 'info', message, title })
  }, [addToast])

  const value: ToastContextType = {
    toasts,
    addToast,
    removeToast,
    clearAllToasts,
    showSuccess,
    showError,
    showWarning,
    showInfo,
  }

  return (
    <ToastContext.Provider value={value}>
      {children}
      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </ToastContext.Provider>
  )
}

/**
 * Hook to access toast context
 */
export function useToast() {
  const context = useContext(ToastContext)
  if (context === undefined) {
    throw new Error('useToast must be used within a ToastProvider')
  }
  return context
}

// Toast icon components
function SuccessIcon() {
  return (
    <svg className="h-5 w-5 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  )
}

function ErrorIcon() {
  return (
    <svg className="h-5 w-5 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  )
}

function WarningIcon() {
  return (
    <svg className="h-5 w-5 text-yellow-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
    </svg>
  )
}

function InfoIcon() {
  return (
    <svg className="h-5 w-5 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  )
}

// Toast item component
function ToastItem({ toast, onRemove }: { toast: Toast; onRemove: (id: string) => void }) {
  const icons: Record<ToastType, JSX.Element> = {
    success: <SuccessIcon />,
    error: <ErrorIcon />,
    warning: <WarningIcon />,
    info: <InfoIcon />,
  }

  const bgColors: Record<ToastType, string> = {
    success: 'bg-green-50 border-green-200',
    error: 'bg-red-50 border-red-200',
    warning: 'bg-yellow-50 border-yellow-200',
    info: 'bg-blue-50 border-blue-200',
  }

  const titleColors: Record<ToastType, string> = {
    success: 'text-green-800',
    error: 'text-red-800',
    warning: 'text-yellow-800',
    info: 'text-blue-800',
  }

  const messageColors: Record<ToastType, string> = {
    success: 'text-green-700',
    error: 'text-red-700',
    warning: 'text-yellow-700',
    info: 'text-blue-700',
  }

  return (
    <div
      className={`max-w-sm w-full shadow-lg rounded-lg pointer-events-auto border ${bgColors[toast.type]} animate-slide-in`}
      data-testid={`toast-${toast.id}`}
      role="alert"
    >
      <div className="p-4">
        <div className="flex items-start">
          <div className="flex-shrink-0">
            {icons[toast.type]}
          </div>
          <div className="ml-3 w-0 flex-1">
            {toast.title && (
              <p className={`text-sm font-medium ${titleColors[toast.type]}`}>
                {toast.title}
              </p>
            )}
            <p className={`text-sm ${toast.title ? 'mt-1' : ''} ${messageColors[toast.type]}`}>
              {toast.message}
            </p>
          </div>
          <div className="ml-4 flex-shrink-0 flex">
            <button
              className={`rounded-md inline-flex ${titleColors[toast.type]} hover:opacity-75 focus:outline-none focus:ring-2 focus:ring-offset-2`}
              onClick={() => onRemove(toast.id)}
              data-testid={`toast-close-${toast.id}`}
            >
              <span className="sr-only">Fechar</span>
              <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// Toast container component
function ToastContainer({ toasts, onRemove }: { toasts: Toast[]; onRemove: (id: string) => void }) {
  if (toasts.length === 0) return null

  return (
    <div
      className="fixed top-4 right-4 z-50 flex flex-col gap-2 pointer-events-none"
      data-testid="toast-container"
      aria-live="polite"
      aria-atomic="true"
    >
      {toasts.map(toast => (
        <ToastItem key={toast.id} toast={toast} onRemove={onRemove} />
      ))}
    </div>
  )
}

/**
 * Helper function to handle API errors and show toast
 * Can be used outside of React components
 */
export function handleApiError(error: unknown, showError: (message: string, title?: string) => void): void {
  if (error instanceof Error) {
    // Network errors
    if (error.message === 'Failed to fetch' || error.message.includes('NetworkError')) {
      showError('Não foi possível conectar ao servidor. Verifique sua conexão.', 'Erro de Conexão')
      return
    }
    showError(error.message)
  } else if (typeof error === 'string') {
    showError(error)
  } else {
    showError('Ocorreu um erro inesperado. Tente novamente.')
  }
}
