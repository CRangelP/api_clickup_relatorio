import { useState, useEffect } from 'react'
import { useMemoryMonitor, fetchBackendMemoryStats, fetchDBPoolStats } from '../hooks/useMemoryMonitor'

interface BackendStats {
  alloc_mb: number
  heap_alloc_mb: number
  goroutines: number
  gc_runs: number
}

interface DBPoolStats {
  max_open_connections: number
  open_connections: number
  in_use: number
  idle: number
}

export default function PerformanceMonitor() {
  const { memoryInfo, isSupported } = useMemoryMonitor(10000)
  const [backendStats, setBackendStats] = useState<BackendStats | null>(null)
  const [dbPoolStats, setDbPoolStats] = useState<DBPoolStats | null>(null)
  const [isExpanded, setIsExpanded] = useState(false)

  useEffect(() => {
    if (!isExpanded) return

    const fetchStats = async () => {
      const [backend, dbPool] = await Promise.all([
        fetchBackendMemoryStats(),
        fetchDBPoolStats(),
      ])
      if (backend) setBackendStats(backend)
      if (dbPool) setDbPoolStats(dbPool)
    }

    fetchStats()
    const interval = setInterval(fetchStats, 10000)
    return () => clearInterval(interval)
  }, [isExpanded])

  return (
    <div className="mt-6 border-t pt-4">
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center text-sm text-gray-600 hover:text-gray-800"
      >
        <span className={`transform transition-transform ${isExpanded ? 'rotate-90' : ''}`}>
          ▶
        </span>
        <span className="ml-2">Monitoramento de Performance</span>
      </button>

      {isExpanded && (
        <div className="mt-4 grid grid-cols-1 md:grid-cols-3 gap-4">
          {/* Frontend Memory */}
          <div className="bg-gray-50 rounded-lg p-4">
            <h4 className="text-sm font-medium text-gray-700 mb-2">Frontend (Browser)</h4>
            {isSupported && memoryInfo ? (
              <div className="space-y-1 text-xs text-gray-600">
                <div className="flex justify-between">
                  <span>Heap Usado:</span>
                  <span className="font-mono">{memoryInfo.usedMB} MB</span>
                </div>
                <div className="flex justify-between">
                  <span>Heap Total:</span>
                  <span className="font-mono">{memoryInfo.totalMB} MB</span>
                </div>
                <div className="flex justify-between">
                  <span>Uso:</span>
                  <span className={`font-mono ${memoryInfo.usagePercent > 80 ? 'text-red-600' : ''}`}>
                    {memoryInfo.usagePercent}%
                  </span>
                </div>
                <div className="mt-2 h-2 bg-gray-200 rounded-full overflow-hidden">
                  <div
                    className={`h-full ${memoryInfo.usagePercent > 80 ? 'bg-red-500' : 'bg-blue-500'}`}
                    style={{ width: `${memoryInfo.usagePercent}%` }}
                  />
                </div>
              </div>
            ) : (
              <p className="text-xs text-gray-500">
                {isSupported ? 'Carregando...' : 'API de memória não disponível neste navegador'}
              </p>
            )}
          </div>

          {/* Backend Memory */}
          <div className="bg-gray-50 rounded-lg p-4">
            <h4 className="text-sm font-medium text-gray-700 mb-2">Backend (Go)</h4>
            {backendStats ? (
              <div className="space-y-1 text-xs text-gray-600">
                <div className="flex justify-between">
                  <span>Heap Alocado:</span>
                  <span className="font-mono">{backendStats.heap_alloc_mb} MB</span>
                </div>
                <div className="flex justify-between">
                  <span>Memória Total:</span>
                  <span className="font-mono">{backendStats.alloc_mb} MB</span>
                </div>
                <div className="flex justify-between">
                  <span>Goroutines:</span>
                  <span className="font-mono">{backendStats.goroutines}</span>
                </div>
                <div className="flex justify-between">
                  <span>GC Runs:</span>
                  <span className="font-mono">{backendStats.gc_runs}</span>
                </div>
              </div>
            ) : (
              <p className="text-xs text-gray-500">Carregando...</p>
            )}
          </div>

          {/* Database Pool */}
          <div className="bg-gray-50 rounded-lg p-4">
            <h4 className="text-sm font-medium text-gray-700 mb-2">Pool de Conexões DB</h4>
            {dbPoolStats ? (
              <div className="space-y-1 text-xs text-gray-600">
                <div className="flex justify-between">
                  <span>Conexões Abertas:</span>
                  <span className="font-mono">{dbPoolStats.open_connections}/{dbPoolStats.max_open_connections}</span>
                </div>
                <div className="flex justify-between">
                  <span>Em Uso:</span>
                  <span className="font-mono">{dbPoolStats.in_use}</span>
                </div>
                <div className="flex justify-between">
                  <span>Ociosas:</span>
                  <span className="font-mono">{dbPoolStats.idle}</span>
                </div>
                <div className="mt-2 h-2 bg-gray-200 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-green-500"
                    style={{ width: `${(dbPoolStats.open_connections / dbPoolStats.max_open_connections) * 100}%` }}
                  />
                </div>
              </div>
            ) : (
              <p className="text-xs text-gray-500">Carregando...</p>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
