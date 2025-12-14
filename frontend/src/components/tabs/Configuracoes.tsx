import { useState, useEffect, lazy, Suspense } from 'react'
import { useWebSocket } from '../../contexts/WebSocketContext'
import { useToast } from '../../contexts/ToastContext'
import { useAuth } from '../../contexts/AuthContext'

// Lazy load PerformanceMonitor since it's not critical
const PerformanceMonitor = lazy(() => import('../PerformanceMonitor'))

// Types
interface ConfigData {
  has_token: boolean
  rate_limit_per_minute: number
}



// Circular progress component
function CircularProgress({ size = 20, strokeWidth = 2 }: { size?: number; strokeWidth?: number }) {
  const radius = (size - strokeWidth) / 2
  const circumference = radius * 2 * Math.PI

  return (
    <svg
      className="animate-spin"
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
    >
      <circle
        className="text-gray-200"
        strokeWidth={strokeWidth}
        stroke="currentColor"
        fill="transparent"
        r={radius}
        cx={size / 2}
        cy={size / 2}
      />
      <circle
        className="text-blue-600"
        strokeWidth={strokeWidth}
        strokeDasharray={circumference}
        strokeDashoffset={circumference * 0.75}
        strokeLinecap="round"
        stroke="currentColor"
        fill="transparent"
        r={radius}
        cx={size / 2}
        cy={size / 2}
      />
    </svg>
  )
}

// Confirmation dialog component
function ConfirmationDialog({
  isOpen,
  title,
  message,
  confirmText,
  cancelText,
  onConfirm,
  onCancel,
  requireDoubleConfirm = false,
}: {
  isOpen: boolean
  title: string
  message: string
  confirmText: string
  cancelText: string
  onConfirm: () => void
  onCancel: () => void
  requireDoubleConfirm?: boolean
}) {
  const [confirmStep, setConfirmStep] = useState(0)

  useEffect(() => {
    if (!isOpen) {
      setConfirmStep(0)
    }
  }, [isOpen])

  if (!isOpen) return null

  const handleConfirm = () => {
    if (requireDoubleConfirm && confirmStep === 0) {
      setConfirmStep(1)
    } else {
      onConfirm()
    }
  }

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto" data-testid="confirmation-dialog">
      <div className="flex items-center justify-center min-h-screen px-4 pt-4 pb-20 text-center sm:p-0">
        <div
          className="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity"
          onClick={onCancel}
        />
        <div className="inline-block align-bottom bg-white rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full">
          <div className="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
            <div className="sm:flex sm:items-start">
              <div className="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-red-100 sm:mx-0 sm:h-10 sm:w-10">
                <svg className="h-6 w-6 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div className="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
                <h3 className="text-lg leading-6 font-medium text-gray-900">
                  {title}
                </h3>
                <div className="mt-2">
                  <p className="text-sm text-gray-500">
                    {confirmStep === 1 ? 'Tem certeza absoluta? Esta ação não pode ser desfeita.' : message}
                  </p>
                </div>
              </div>
            </div>
          </div>
          <div className="bg-gray-50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
            <button
              type="button"
              onClick={handleConfirm}
              className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:ml-3 sm:w-auto sm:text-sm"
              data-testid="confirm-btn"
            >
              {confirmStep === 1 ? 'Sim, tenho certeza' : confirmText}
            </button>
            <button
              type="button"
              onClick={onCancel}
              className="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm"
              data-testid="cancel-btn"
            >
              {cancelText}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export default function Configuracoes() {
  // Config state
  const [config, setConfig] = useState<ConfigData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  // Token state
  const [token, setToken] = useState('')
  const [tokenError, setTokenError] = useState<string | null>(null)

  // Rate limit state
  const [rateLimit, setRateLimit] = useState(2000)
  const [rateLimitSaving, setRateLimitSaving] = useState(false)

  // History management state
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [deletingHistory, setDeletingHistory] = useState(false)

  // Use centralized WebSocket context for metadata sync state
  const { metadataSyncState } = useWebSocket()
  
  // Toast notifications
  const { showSuccess, showError } = useToast()
  
  // Auth context for CSRF token
  const { getCSRFHeaders } = useAuth()

  // Fetch config on mount
  useEffect(() => {
    fetchConfig()
  }, [])

  // Handle metadata sync state changes from WebSocket
  useEffect(() => {
    if (metadataSyncState.status === 'completed') {
      showSuccess('Metadados sincronizados com sucesso!')
      setSuccessMessage('Metadados sincronizados com sucesso!')
      setTimeout(() => setSuccessMessage(null), 3000)
    } else if (metadataSyncState.status === 'error') {
      showError(metadataSyncState.message, 'Erro na Sincronização')
      setError(metadataSyncState.message)
    }
  }, [metadataSyncState, showSuccess, showError])

  // Fetch user configuration
  const fetchConfig = async () => {
    try {
      const response = await fetch('/api/web/config', {
        credentials: 'include',
      })
      if (!response.ok) {
        throw new Error('Erro ao carregar configurações')
      }
      const data = await response.json()
      if (data.success && data.data) {
        setConfig(data.data)
        setRateLimit(data.data.rate_limit_per_minute)
      }
      setError(null)
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Erro desconhecido'
      // Check for network errors
      if (err instanceof Error && (err.message === 'Failed to fetch' || err.message.includes('NetworkError'))) {
        showError('Não foi possível conectar ao servidor. Verifique sua conexão.', 'Erro de Conexão')
      }
      setError(errorMessage)
    } finally {
      setLoading(false)
    }
  }

  // Validate token format
  const validateToken = (value: string): string | null => {
    if (!value.trim()) {
      return 'Token é obrigatório'
    }
    if (!value.startsWith('pk_')) {
      return 'Token deve começar com "pk_"'
    }
    if (value.length < 10) {
      return 'Token parece ser muito curto'
    }
    return null
  }

  // Handle token change
  const handleTokenChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setToken(value)
    setTokenError(null)
  }

  // Track if we initiated a sync (to show local loading state)
  const [isSyncing, setIsSyncing] = useState(false)

  // Handle sync metadata
  const handleSyncMetadata = async () => {
    const validationError = validateToken(token)
    if (validationError) {
      setTokenError(validationError)
      return
    }

    setIsSyncing(true)
    setError(null)
    setSuccessMessage(null)

    try {
      const response = await fetch('/api/web/metadata/sync', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          ...getCSRFHeaders(),
        },
        credentials: 'include',
        body: JSON.stringify({ token }),
      })

      if (!response.ok) {
        const errorData = await response.json()
        throw new Error(errorData.error || 'Erro ao sincronizar metadados')
      }

      // Success will be handled by WebSocket
      // Update config to reflect token is now set
      setConfig(prev => prev ? { ...prev, has_token: true } : null)
      setToken('') // Clear token input after successful sync
    } catch (err) {
      setIsSyncing(false)
      const errorMessage = err instanceof Error ? err.message : 'Erro desconhecido'
      showError(errorMessage, 'Erro na Sincronização')
      setError(errorMessage)
    }
  }

  // Reset syncing state when metadata sync completes
  useEffect(() => {
    if (metadataSyncState.status === 'completed' || metadataSyncState.status === 'error' || metadataSyncState.status === 'idle') {
      setIsSyncing(false)
    }
  }, [metadataSyncState.status])

  // Handle rate limit change
  const handleRateLimitChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(e.target.value, 10)
    setRateLimit(value)
  }

  // Save rate limit
  const handleSaveRateLimit = async () => {
    if (rateLimit < 10 || rateLimit > 10000) {
      showError('Rate limit deve estar entre 10 e 10000', 'Validação')
      setError('Rate limit deve estar entre 10 e 10000')
      return
    }

    setRateLimitSaving(true)
    setError(null)
    setSuccessMessage(null)

    try {
      const response = await fetch('/api/web/config', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          ...getCSRFHeaders(),
        },
        credentials: 'include',
        body: JSON.stringify({ rate_limit_per_minute: rateLimit }),
      })

      if (!response.ok) {
        const errorData = await response.json()
        throw new Error(errorData.error || 'Erro ao salvar configuração')
      }

      showSuccess('Rate limit salvo com sucesso!')
      setSuccessMessage('Rate limit salvo com sucesso!')
      setConfig(prev => prev ? { ...prev, rate_limit_per_minute: rateLimit } : null)
      setTimeout(() => setSuccessMessage(null), 3000)
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Erro desconhecido'
      showError(errorMessage, 'Erro ao Salvar')
      setError(errorMessage)
    } finally {
      setRateLimitSaving(false)
    }
  }

  // Handle delete history
  const handleDeleteHistory = async () => {
    setDeletingHistory(true)
    setError(null)
    setSuccessMessage(null)

    try {
      const response = await fetch('/api/web/history', {
        method: 'DELETE',
        headers: { 
          'Content-Type': 'application/json',
          ...getCSRFHeaders(),
        },
        credentials: 'include',
        body: JSON.stringify({ confirm: true }),
      })

      if (!response.ok) {
        const errorData = await response.json()
        throw new Error(errorData.error || 'Erro ao limpar histórico')
      }

      showSuccess('Histórico limpo com sucesso!')
      setSuccessMessage('Histórico limpo com sucesso!')
      setTimeout(() => setSuccessMessage(null), 3000)
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Erro desconhecido'
      showError(errorMessage, 'Erro ao Limpar Histórico')
      setError(errorMessage)
    } finally {
      setDeletingHistory(false)
      setShowDeleteConfirm(false)
    }
  }

  return (
    <div data-testid="configuracoes-tab">
      <h2 className="text-lg font-medium text-gray-900 mb-4">Configurações</h2>
      <p className="text-gray-600 mb-6">
        Configure o token do ClickUp, rate limit e gerencie o histórico.
      </p>

      {/* Loading State */}
      {loading && (
        <div className="flex justify-center items-center py-12">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
        </div>
      )}

      {/* Error Message */}
      {error && (
        <div className="mb-4 bg-red-50 border border-red-200 rounded-lg p-4">
          <p className="text-red-700">{error}</p>
        </div>
      )}

      {/* Success Message */}
      {successMessage && (
        <div className="mb-4 bg-green-50 border border-green-200 rounded-lg p-4">
          <p className="text-green-700">{successMessage}</p>
        </div>
      )}

      {!loading && (
        <div className="space-y-8">
          {/* Token Section */}
          <section className="bg-white shadow rounded-lg p-6" data-testid="token-section">
            <h3 className="text-md font-medium text-gray-900 mb-4">Token do ClickUp</h3>
            
            {config?.has_token && (
              <div className="mb-4 flex items-center text-sm text-green-600">
                <svg className="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                Token configurado
              </div>
            )}

            <div className="space-y-4">
              <div>
                <label htmlFor="token" className="block text-sm font-medium text-gray-700 mb-1">
                  {config?.has_token ? 'Atualizar Token' : 'Token Pessoal'}
                </label>
                <input
                  type="password"
                  id="token"
                  value={token}
                  onChange={handleTokenChange}
                  placeholder="pk_..."
                  className={`w-full max-w-md px-3 py-2 border rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 ${
                    tokenError ? 'border-red-300' : 'border-gray-300'
                  }`}
                  data-testid="token-input"
                />
                {tokenError && (
                  <p className="mt-1 text-sm text-red-600">{tokenError}</p>
                )}
                <p className="mt-1 text-xs text-gray-500">
                  Encontre seu token em: ClickUp → Settings → Apps → API Token
                </p>
              </div>

              <button
                onClick={handleSyncMetadata}
                disabled={isSyncing || metadataSyncState.status === 'syncing' || !token.trim()}
                className="inline-flex items-center px-4 py-2 bg-blue-600 text-white font-medium rounded-md shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:bg-gray-400 disabled:cursor-not-allowed"
                data-testid="sync-btn"
              >
                {(isSyncing || metadataSyncState.status === 'syncing') ? (
                  <>
                    <CircularProgress size={20} strokeWidth={2} />
                    <span className="ml-2">Sincronizando...</span>
                  </>
                ) : (
                  <>
                    <svg className="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                    Atualizar Campos
                  </>
                )}
              </button>

              {(isSyncing || metadataSyncState.status === 'syncing') && (
                <p className="text-sm text-blue-600">{metadataSyncState.message || 'Iniciando sincronização...'}</p>
              )}
            </div>
          </section>

          {/* Rate Limit Section */}
          <section className="bg-white shadow rounded-lg p-6" data-testid="rate-limit-section">
            <h3 className="text-md font-medium text-gray-900 mb-4">Rate Limit</h3>
            <p className="text-sm text-gray-600 mb-4">
              Configure o número máximo de requisições por minuto para a API do ClickUp.
            </p>

            <div className="space-y-4">
              <div>
                <label htmlFor="rate-limit" className="block text-sm font-medium text-gray-700 mb-1">
                  Requisições por minuto: <span className="font-bold text-blue-600">{rateLimit}</span>
                </label>
                <input
                  type="range"
                  id="rate-limit"
                  min="10"
                  max="10000"
                  step="10"
                  value={rateLimit}
                  onChange={handleRateLimitChange}
                  className="w-full max-w-md h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer"
                  data-testid="rate-limit-slider"
                />
                <div className="flex justify-between text-xs text-gray-500 max-w-md">
                  <span>10</span>
                  <span>5000</span>
                  <span>10000</span>
                </div>
              </div>

              <button
                onClick={handleSaveRateLimit}
                disabled={rateLimitSaving || rateLimit === config?.rate_limit_per_minute}
                className="inline-flex items-center px-4 py-2 bg-blue-600 text-white font-medium rounded-md shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:bg-gray-400 disabled:cursor-not-allowed"
                data-testid="save-rate-limit-btn"
              >
                {rateLimitSaving ? (
                  <>
                    <CircularProgress size={20} strokeWidth={2} />
                    <span className="ml-2">Salvando...</span>
                  </>
                ) : (
                  'Salvar Rate Limit'
                )}
              </button>
            </div>
          </section>

          {/* History Management Section */}
          <section className="bg-white shadow rounded-lg p-6" data-testid="history-section">
            <h3 className="text-md font-medium text-gray-900 mb-4">Gerenciamento de Histórico</h3>
            <p className="text-sm text-gray-600 mb-4">
              Limpe todo o histórico de operações. Esta ação não pode ser desfeita.
            </p>

            <button
              onClick={() => setShowDeleteConfirm(true)}
              disabled={deletingHistory}
              className="inline-flex items-center px-4 py-2 bg-red-600 text-white font-medium rounded-md shadow-sm hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 disabled:bg-gray-400 disabled:cursor-not-allowed"
              data-testid="clear-history-btn"
            >
              {deletingHistory ? (
                <>
                  <CircularProgress size={20} strokeWidth={2} />
                  <span className="ml-2">Limpando...</span>
                </>
              ) : (
                <>
                  <svg className="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                  Limpar Histórico
                </>
              )}
            </button>
          </section>
        </div>
      )}

      {/* Delete Confirmation Dialog */}
      <ConfirmationDialog
        isOpen={showDeleteConfirm}
        title="Limpar Histórico"
        message="Você está prestes a deletar todo o histórico de operações. Esta ação não pode ser desfeita."
        confirmText="Limpar Histórico"
        cancelText="Cancelar"
        onConfirm={handleDeleteHistory}
        onCancel={() => setShowDeleteConfirm(false)}
        requireDoubleConfirm={true}
      />

      {/* Performance Monitor - lazy loaded */}
      {!loading && (
        <Suspense fallback={<div className="mt-6 text-sm text-gray-500">Carregando monitor...</div>}>
          <PerformanceMonitor />
        </Suspense>
      )}
    </div>
  )
}
