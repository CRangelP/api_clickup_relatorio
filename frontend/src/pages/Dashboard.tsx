import { useState, useCallback, useRef, ErrorInfo, lazy, Suspense } from 'react'
import { useAuth } from '../contexts/AuthContext'
import Header from '../components/Header'
import TabNavigation from '../components/TabNavigation'
import ErrorBoundary from '../components/ErrorBoundary'

// Lazy load tab components for better initial load performance
const BuscarTasks = lazy(() => import('../components/tabs/BuscarTasks'))
const Uploads = lazy(() => import('../components/tabs/Uploads'))
const Relatorios = lazy(() => import('../components/tabs/Relatorios'))
const Configuracoes = lazy(() => import('../components/tabs/Configuracoes'))

export type TabId = 'buscar' | 'uploads' | 'relatorios' | 'configuracoes'

const tabs: { id: TabId; label: string }[] = [
  { id: 'buscar', label: 'Buscar Tasks' },
  { id: 'uploads', label: 'Uploads' },
  { id: 'relatorios', label: 'Relatórios' },
  { id: 'configuracoes', label: 'Configurações' },
]

// Loading fallback for lazy-loaded components
function TabLoadingFallback() {
  return (
    <div className="flex items-center justify-center py-12" data-testid="tab-loading">
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
      <span className="ml-3 text-gray-600">Carregando...</span>
    </div>
  )
}

// Track errors per tab for isolation verification
export interface TabErrorState {
  [key: string]: { hasError: boolean; error: Error | null }
}

export default function Dashboard() {
  const [activeTab, setActiveTab] = useState<TabId>('buscar')
  const { user, logout } = useAuth()
  const tabErrorsRef = useRef<TabErrorState>({})

  const handleTabError = useCallback((tabId: TabId) => {
    return (error: Error, _errorInfo: ErrorInfo) => {
      tabErrorsRef.current[tabId] = { hasError: true, error }
    }
  }, [])

  // Get current tab errors for testing purposes
  const getTabErrors = useCallback(() => tabErrorsRef.current, [])

  const renderTabContent = () => {
    const content = (() => {
      switch (activeTab) {
        case 'buscar':
          return (
            <ErrorBoundary tabId="buscar" onError={handleTabError('buscar')}>
              <BuscarTasks />
            </ErrorBoundary>
          )
        case 'uploads':
          return (
            <ErrorBoundary tabId="uploads" onError={handleTabError('uploads')}>
              <Uploads />
            </ErrorBoundary>
          )
        case 'relatorios':
          return (
            <ErrorBoundary tabId="relatorios" onError={handleTabError('relatorios')}>
              <Relatorios />
            </ErrorBoundary>
          )
        case 'configuracoes':
          return (
            <ErrorBoundary tabId="configuracoes" onError={handleTabError('configuracoes')}>
              <Configuracoes />
            </ErrorBoundary>
          )
        default:
          return null
      }
    })()

    return (
      <Suspense fallback={<TabLoadingFallback />}>
        {content}
      </Suspense>
    )
  }

  return (
    <div className="min-h-screen bg-gray-100" data-testid="dashboard">
      <Header user={user} onLogout={logout} />
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <TabNavigation tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab} />
        <div className="mt-6 bg-white rounded-lg shadow-sm p-6" data-testid="tab-content">
          {renderTabContent()}
        </div>
      </div>
      {/* Hidden element for testing - exposes error state */}
      <div data-testid="tab-errors" data-errors={JSON.stringify(getTabErrors())} style={{ display: 'none' }} />
    </div>
  )
}
