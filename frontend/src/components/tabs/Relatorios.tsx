import { useState, useEffect, useCallback } from 'react'
import { useWebSocket } from '../../contexts/WebSocketContext'
import { useToast } from '../../contexts/ToastContext'

// Types
interface OperationHistory {
  id: number
  user_id: string
  operation_type: string
  title: string
  status: string
  details: OperationDetails | null
  created_at: string
}

interface OperationDetails {
  total_rows?: number
  processed_rows?: number
  success_count?: number
  error_count?: number
  errors?: string[]
  progress?: number
  message?: string
}

// Status badge component
function StatusBadge({ status }: { status: string }) {
  const statusConfig: Record<string, { bg: string; text: string; label: string }> = {
    pending: { bg: 'bg-yellow-100', text: 'text-yellow-800', label: 'Pendente' },
    processing: { bg: 'bg-blue-100', text: 'text-blue-800', label: 'Processando' },
    completed: { bg: 'bg-green-100', text: 'text-green-800', label: 'Concluído' },
    failed: { bg: 'bg-red-100', text: 'text-red-800', label: 'Falhou' },
  }

  const config = statusConfig[status] || { bg: 'bg-gray-100', text: 'text-gray-800', label: status }

  return (
    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${config.bg} ${config.text}`}>
      {config.label}
    </span>
  )
}

// Operation type icon
function OperationTypeIcon({ type }: { type: string }) {
  if (type === 'field_update') {
    return (
      <svg className="h-5 w-5 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
      </svg>
    )
  }
  if (type === 'report_generation') {
    return (
      <svg className="h-5 w-5 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
      </svg>
    )
  }
  return (
    <svg className="h-5 w-5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
    </svg>
  )
}

// Format date helper
function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// Format operation type
function formatOperationType(type: string): string {
  const types: Record<string, string> = {
    field_update: 'Atualização de Campos',
    report_generation: 'Geração de Relatório',
  }
  return types[type] || type
}

export default function Relatorios() {
  const [history, setHistory] = useState<OperationHistory[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedOperation, setSelectedOperation] = useState<OperationHistory | null>(null)
  
  // Use centralized WebSocket context
  const { progressUpdates, isConnected } = useWebSocket()
  
  // Toast notifications
  const { showError } = useToast()

  // Fetch operation history
  const fetchHistory = useCallback(async () => {
    try {
      const response = await fetch('/api/web/history', {
        credentials: 'include',
      })
      if (!response.ok) {
        throw new Error('Erro ao carregar histórico')
      }
      const data = await response.json()
      if (data.success && data.data) {
        setHistory(data.data)
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
  }, [showError])

  // Initialize - fetch history on mount
  useEffect(() => {
    fetchHistory()
  }, [fetchHistory])

  // Refresh history when operations complete
  useEffect(() => {
    const completedOrFailed = Object.values(progressUpdates).some(
      update => update.status === 'completed' || update.status === 'failed'
    )
    if (completedOrFailed) {
      fetchHistory()
    }
  }, [progressUpdates, fetchHistory])

  // Get progress for an operation
  const getProgress = (operation: OperationHistory): number => {
    // Check for real-time progress updates first
    const progressUpdate = progressUpdates[operation.id]
    if (progressUpdate?.progress !== undefined) {
      return progressUpdate.progress
    }
    // Fall back to stored details
    if (operation.details?.total_rows && operation.details?.processed_rows) {
      return (operation.details.processed_rows / operation.details.total_rows) * 100
    }
    if (operation.status === 'completed') return 100
    if (operation.status === 'pending') return 0
    return 0
  }

  // Handle operation click
  const handleOperationClick = async (operation: OperationHistory) => {
    try {
      const response = await fetch(`/api/web/history/${operation.id}`, {
        credentials: 'include',
      })
      if (response.ok) {
        const data = await response.json()
        if (data.success && data.data) {
          setSelectedOperation(data.data)
        }
      }
    } catch (err) {
      console.error('Error fetching operation details:', err)
      setSelectedOperation(operation)
    }
  }

  // Close detail modal
  const closeDetailModal = () => {
    setSelectedOperation(null)
  }

  return (
    <div data-testid="relatorios-tab">
      <h2 className="text-lg font-medium text-gray-900 mb-4">Relatórios</h2>
      <div className="flex items-center justify-between mb-6">
        <p className="text-gray-600">
          Acompanhe o histórico e progresso das operações em tempo real.
        </p>
        {/* WebSocket Connection Status */}
        <div className="flex items-center space-x-2" data-testid="ws-status">
          <span className={`h-2 w-2 rounded-full ${isConnected ? 'bg-green-500' : 'bg-red-500'}`}></span>
          <span className="text-xs text-gray-500">
            {isConnected ? 'Conectado' : 'Desconectado'}
          </span>
        </div>
      </div>

      {/* Error Message */}
      {error && (
        <div className="mb-4 bg-red-50 border border-red-200 rounded-lg p-4">
          <p className="text-red-700">{error}</p>
          <button
            onClick={fetchHistory}
            className="mt-2 text-sm text-red-600 hover:text-red-800 underline"
          >
            Tentar novamente
          </button>
        </div>
      )}

      {/* Loading State */}
      {loading && (
        <div className="flex justify-center items-center py-12">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
        </div>
      )}

      {/* Empty State */}
      {!loading && history.length === 0 && (
        <div className="text-center py-12 bg-gray-50 rounded-lg">
          <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
          </svg>
          <h3 className="mt-4 text-sm font-medium text-gray-900">Nenhuma operação encontrada</h3>
          <p className="mt-2 text-sm text-gray-500">
            As operações realizadas aparecerão aqui.
          </p>
        </div>
      )}

      {/* Operation History List */}
      {!loading && history.length > 0 && (
        <div className="bg-white shadow rounded-lg overflow-hidden" data-testid="history-list">
          <ul className="divide-y divide-gray-200">
            {history.map((operation) => {
              const progress = getProgress(operation)
              const isProcessing = operation.status === 'processing'
              const progressUpdate = progressUpdates[operation.id]

              return (
                <li
                  key={operation.id}
                  className="hover:bg-gray-50 cursor-pointer transition-colors"
                  onClick={() => handleOperationClick(operation)}
                  data-testid={`operation-${operation.id}`}
                >
                  <div className="px-4 py-4 sm:px-6">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-3">
                        <OperationTypeIcon type={operation.operation_type} />
                        <div>
                          <p className="text-sm font-medium text-gray-900 truncate">
                            {operation.title}
                          </p>
                          <p className="text-xs text-gray-500">
                            {formatOperationType(operation.operation_type)}
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center space-x-4">
                        <StatusBadge status={progressUpdate?.status || operation.status} />
                        <span className="text-xs text-gray-500">
                          {formatDate(operation.created_at)}
                        </span>
                      </div>
                    </div>

                    {/* Progress Bar */}
                    {(isProcessing || progressUpdate?.status === 'processing') && (
                      <div className="mt-3">
                        <div className="flex justify-between text-xs text-gray-600 mb-1">
                          <span>Progresso</span>
                          <span>{Math.round(progress)}%</span>
                        </div>
                        <div className="w-full bg-gray-200 rounded-full h-2">
                          <div
                            className="bg-blue-600 h-2 rounded-full transition-all duration-300"
                            style={{ width: `${progress}%` }}
                            data-testid={`progress-bar-${operation.id}`}
                          ></div>
                        </div>
                        {progressUpdate && (
                          <div className="flex justify-between text-xs mt-1">
                            <span className="text-green-600">
                              ✓ {progressUpdate.success_count || 0} sucesso
                            </span>
                            <span className="text-red-600">
                              ✗ {progressUpdate.error_count || 0} erros
                            </span>
                          </div>
                        )}
                      </div>
                    )}

                    {/* Completed/Failed Summary */}
                    {(operation.status === 'completed' || operation.status === 'failed') && operation.details && (
                      <div className="mt-2 flex space-x-4 text-xs">
                        {operation.details.success_count !== undefined && (
                          <span className="text-green-600">
                            ✓ {operation.details.success_count} sucesso
                          </span>
                        )}
                        {operation.details.error_count !== undefined && operation.details.error_count > 0 && (
                          <span className="text-red-600">
                            ✗ {operation.details.error_count} erros
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        </div>
      )}

      {/* Refresh Button */}
      {!loading && (
        <div className="mt-4 flex justify-end">
          <button
            onClick={fetchHistory}
            className="inline-flex items-center px-3 py-2 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
          >
            <svg className="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            Atualizar
          </button>
        </div>
      )}

      {/* Operation Detail Modal */}
      {selectedOperation && (
        <div className="fixed inset-0 z-50 overflow-y-auto" data-testid="operation-detail-modal">
          <div className="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
            {/* Background overlay */}
            <div
              className="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity"
              onClick={closeDetailModal}
            ></div>

            {/* Modal panel */}
            <div className="inline-block align-bottom bg-white rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full">
              <div className="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
                <div className="flex items-start justify-between">
                  <div className="flex items-center space-x-3">
                    <OperationTypeIcon type={selectedOperation.operation_type} />
                    <div>
                      <h3 className="text-lg font-medium text-gray-900">
                        {selectedOperation.title}
                      </h3>
                      <p className="text-sm text-gray-500">
                        {formatOperationType(selectedOperation.operation_type)}
                      </p>
                    </div>
                  </div>
                  <button
                    onClick={closeDetailModal}
                    className="text-gray-400 hover:text-gray-500"
                  >
                    <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>

                <div className="mt-4 space-y-4">
                  {/* Status */}
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-gray-500">Status</span>
                    <StatusBadge status={selectedOperation.status} />
                  </div>

                  {/* Timestamp */}
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-gray-500">Data/Hora</span>
                    <span className="text-sm text-gray-900">
                      {formatDate(selectedOperation.created_at)}
                    </span>
                  </div>

                  {/* Details */}
                  {selectedOperation.details && (
                    <>
                      {/* Progress */}
                      {selectedOperation.details.total_rows !== undefined && (
                        <div>
                          <div className="flex items-center justify-between text-sm">
                            <span className="text-gray-500">Progresso</span>
                            <span className="text-gray-900">
                              {selectedOperation.details.processed_rows || 0} / {selectedOperation.details.total_rows}
                            </span>
                          </div>
                          <div className="mt-1 w-full bg-gray-200 rounded-full h-2">
                            <div
                              className="bg-blue-600 h-2 rounded-full"
                              style={{
                                width: `${selectedOperation.details.total_rows > 0
                                  ? ((selectedOperation.details.processed_rows || 0) / selectedOperation.details.total_rows) * 100
                                  : 0}%`
                              }}
                            ></div>
                          </div>
                        </div>
                      )}

                      {/* Success/Error Counts */}
                      <div className="grid grid-cols-2 gap-4">
                        {selectedOperation.details.success_count !== undefined && (
                          <div className="bg-green-50 rounded-lg p-3">
                            <p className="text-xs text-green-600 font-medium">Sucesso</p>
                            <p className="text-2xl font-bold text-green-700">
                              {selectedOperation.details.success_count}
                            </p>
                          </div>
                        )}
                        {selectedOperation.details.error_count !== undefined && (
                          <div className="bg-red-50 rounded-lg p-3">
                            <p className="text-xs text-red-600 font-medium">Erros</p>
                            <p className="text-2xl font-bold text-red-700">
                              {selectedOperation.details.error_count}
                            </p>
                          </div>
                        )}
                      </div>

                      {/* Error Details */}
                      {selectedOperation.details.errors && selectedOperation.details.errors.length > 0 && (
                        <div>
                          <h4 className="text-sm font-medium text-gray-900 mb-2">
                            Detalhes dos Erros
                          </h4>
                          <div className="bg-red-50 rounded-lg p-3 max-h-48 overflow-y-auto">
                            <ul className="space-y-1 text-sm text-red-700">
                              {selectedOperation.details.errors.map((error, idx) => (
                                <li key={idx} className="flex items-start">
                                  <span className="text-red-500 mr-2">•</span>
                                  <span>{error}</span>
                                </li>
                              ))}
                            </ul>
                          </div>
                        </div>
                      )}
                    </>
                  )}
                </div>
              </div>

              <div className="bg-gray-50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
                <button
                  type="button"
                  onClick={closeDetailModal}
                  className="w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 sm:w-auto sm:text-sm"
                >
                  Fechar
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
