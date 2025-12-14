import { useState, useEffect, useCallback } from 'react'
import { useToast } from '../../contexts/ToastContext'
import { useAuth } from '../../contexts/AuthContext'

// Types for hierarchical data
interface ListData {
  id: string
  name: string
}

interface FolderData {
  id: string
  name: string
  lists: ListData[]
}

interface SpaceData {
  id: string
  name: string
  folders: FolderData[]
}

interface WorkspaceData {
  id: string
  name: string
  spaces: SpaceData[]
}

interface CustomFieldData {
  id: string
  name: string
  type: string
  options: Record<string, unknown>
}

interface HierarchyData {
  workspaces: WorkspaceData[]
  custom_fields: CustomFieldData[]
}

interface HierarchyResponse {
  success: boolean
  data: HierarchyData | null
}

export default function BuscarTasks() {
  // Toast notifications
  const { showError, showSuccess } = useToast()
  
  // Auth context for CSRF token
  const { getCSRFHeaders } = useAuth()
  
  // Hierarchy data state
  const [hierarchyData, setHierarchyData] = useState<HierarchyData | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Selection state
  const [selectedWorkspace, setSelectedWorkspace] = useState<string>('')
  const [selectedSpace, setSelectedSpace] = useState<string>('')
  const [selectedFolder, setSelectedFolder] = useState<string>('')
  const [selectedLists, setSelectedLists] = useState<string[]>([])
  const [selectedFields, setSelectedFields] = useState<string[]>([])
  
  // Options state
  const [includeSubtasks, setIncludeSubtasks] = useState(false)
  const [includeClosedTasks, setIncludeClosedTasks] = useState(false)
  
  // Report generation state
  const [isGenerating, setIsGenerating] = useState(false)
  const [generateError, setGenerateError] = useState<string | null>(null)

  // Fetch hierarchy data on mount
  useEffect(() => {
    fetchHierarchy()
  }, [])

  const fetchHierarchy = async () => {
    setIsLoading(true)
    setError(null)
    try {
      const response = await fetch('/api/web/metadata/hierarchy', {
        credentials: 'include',
      })
      
      if (!response.ok) {
        throw new Error('Falha ao carregar dados hierárquicos')
      }
      
      const data: HierarchyResponse = await response.json()
      if (data.success && data.data) {
        setHierarchyData(data.data)
      } else {
        throw new Error('Dados hierárquicos inválidos')
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Erro desconhecido'
      // Check for network errors
      if (err instanceof Error && (err.message === 'Failed to fetch' || err.message.includes('NetworkError'))) {
        showError('Não foi possível conectar ao servidor. Verifique sua conexão.', 'Erro de Conexão')
      }
      setError(errorMessage)
    } finally {
      setIsLoading(false)
    }
  }

  // Get filtered spaces based on selected workspace
  const getFilteredSpaces = useCallback((): SpaceData[] => {
    if (!hierarchyData || !selectedWorkspace) return []
    const workspace = hierarchyData.workspaces.find(w => w.id === selectedWorkspace)
    return workspace?.spaces || []
  }, [hierarchyData, selectedWorkspace])

  // Get filtered folders based on selected space
  const getFilteredFolders = useCallback((): FolderData[] => {
    if (!selectedSpace) return []
    const spaces = getFilteredSpaces()
    const space = spaces.find(s => s.id === selectedSpace)
    return space?.folders || []
  }, [selectedSpace, getFilteredSpaces])

  // Get filtered lists based on selected folder
  const getFilteredLists = useCallback((): ListData[] => {
    if (!selectedFolder) return []
    const folders = getFilteredFolders()
    const folder = folders.find(f => f.id === selectedFolder)
    return folder?.lists || []
  }, [selectedFolder, getFilteredFolders])

  // Handle workspace change - reset dependent selections
  const handleWorkspaceChange = (workspaceId: string) => {
    setSelectedWorkspace(workspaceId)
    setSelectedSpace('')
    setSelectedFolder('')
    setSelectedLists([])
  }

  // Handle space change - reset dependent selections
  const handleSpaceChange = (spaceId: string) => {
    setSelectedSpace(spaceId)
    setSelectedFolder('')
    setSelectedLists([])
  }

  // Handle folder change - reset dependent selections
  const handleFolderChange = (folderId: string) => {
    setSelectedFolder(folderId)
    setSelectedLists([])
  }

  // Handle list multi-select toggle
  const handleListToggle = (listId: string) => {
    setSelectedLists(prev => 
      prev.includes(listId)
        ? prev.filter(id => id !== listId)
        : [...prev, listId]
    )
  }

  // Handle field multi-select toggle
  const handleFieldToggle = (fieldId: string) => {
    setSelectedFields(prev =>
      prev.includes(fieldId)
        ? prev.filter(id => id !== fieldId)
        : [...prev, fieldId]
    )
  }

  // Select all lists in current folder
  const handleSelectAllLists = () => {
    const lists = getFilteredLists()
    setSelectedLists(lists.map(l => l.id))
  }

  // Clear all list selections
  const handleClearLists = () => {
    setSelectedLists([])
  }

  // Generate report
  const handleGenerateReport = async () => {
    if (selectedLists.length === 0) {
      showError('Selecione pelo menos uma lista', 'Validação')
      setGenerateError('Selecione pelo menos uma lista')
      return
    }
    
    if (selectedFields.length === 0) {
      showError('Selecione pelo menos um campo', 'Validação')
      setGenerateError('Selecione pelo menos um campo')
      return
    }

    setIsGenerating(true)
    setGenerateError(null)

    try {
      const response = await fetch('/api/web/reports', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...getCSRFHeaders(),
        },
        credentials: 'include',
        body: JSON.stringify({
          list_ids: selectedLists,
          fields: selectedFields,
          subtasks: includeSubtasks,
          include_closed: includeClosedTasks,
        }),
      })

      if (!response.ok) {
        const errorData = await response.json()
        throw new Error(errorData.error || 'Falha ao gerar relatório')
      }

      // Get filename from Content-Disposition header
      const contentDisposition = response.headers.get('Content-Disposition')
      let filename = 'relatorio.xlsx'
      if (contentDisposition) {
        const match = contentDisposition.match(/filename=(.+)/)
        if (match) {
          filename = match[1]
        }
      }

      // Download the file
      const blob = await response.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      window.URL.revokeObjectURL(url)
      document.body.removeChild(a)
      
      showSuccess(`Relatório "${filename}" gerado com sucesso!`)
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Erro ao gerar relatório'
      // Check for network errors
      if (err instanceof Error && (err.message === 'Failed to fetch' || err.message.includes('NetworkError'))) {
        showError('Não foi possível conectar ao servidor. Verifique sua conexão.', 'Erro de Conexão')
      } else {
        showError(errorMessage, 'Erro ao Gerar Relatório')
      }
      setGenerateError(errorMessage)
    } finally {
      setIsGenerating(false)
    }
  }

  // Check if form is valid for submission
  const isFormValid = selectedLists.length > 0 && selectedFields.length > 0

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
        <span className="ml-3 text-gray-600">Carregando dados...</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4">
        <p className="text-red-700">{error}</p>
        <button
          onClick={fetchHierarchy}
          className="mt-2 text-sm text-red-600 hover:text-red-800 underline"
        >
          Tentar novamente
        </button>
      </div>
    )
  }

  const spaces = getFilteredSpaces()
  const folders = getFilteredFolders()
  const lists = getFilteredLists()

  return (
    <div data-testid="buscar-tasks">
      <h2 className="text-lg font-medium text-gray-900 mb-4">Buscar Tasks</h2>
      <p className="text-gray-600 mb-6">
        Selecione workspace, space, folder e lista para buscar tasks do ClickUp.
      </p>

      {/* Hierarchical Selectors */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        {/* Workspace Selector */}
        <div>
          <label htmlFor="workspace" className="block text-sm font-medium text-gray-700 mb-1">
            Workspace
          </label>
          <select
            id="workspace"
            data-testid="workspace-selector"
            value={selectedWorkspace}
            onChange={(e) => handleWorkspaceChange(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
          >
            <option value="">Selecione um workspace</option>
            {hierarchyData?.workspaces.map((workspace) => (
              <option key={workspace.id} value={workspace.id}>
                {workspace.name}
              </option>
            ))}
          </select>
        </div>

        {/* Space Selector */}
        <div>
          <label htmlFor="space" className="block text-sm font-medium text-gray-700 mb-1">
            Space
          </label>
          <select
            id="space"
            data-testid="space-selector"
            value={selectedSpace}
            onChange={(e) => handleSpaceChange(e.target.value)}
            disabled={!selectedWorkspace}
            className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 disabled:bg-gray-100 disabled:cursor-not-allowed"
          >
            <option value="">Selecione um space</option>
            {spaces.map((space) => (
              <option key={space.id} value={space.id}>
                {space.name}
              </option>
            ))}
          </select>
        </div>

        {/* Folder Selector */}
        <div>
          <label htmlFor="folder" className="block text-sm font-medium text-gray-700 mb-1">
            Folder
          </label>
          <select
            id="folder"
            data-testid="folder-selector"
            value={selectedFolder}
            onChange={(e) => handleFolderChange(e.target.value)}
            disabled={!selectedSpace}
            className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 disabled:bg-gray-100 disabled:cursor-not-allowed"
          >
            <option value="">Selecione um folder</option>
            {folders.map((folder) => (
              <option key={folder.id} value={folder.id}>
                {folder.name}
              </option>
            ))}
          </select>
        </div>

        {/* List Multi-Select */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            Listas ({selectedLists.length} selecionadas)
          </label>
          <div 
            data-testid="list-selector"
            className={`border border-gray-300 rounded-md shadow-sm max-h-40 overflow-y-auto ${!selectedFolder ? 'bg-gray-100' : 'bg-white'}`}
          >
            {selectedFolder && lists.length > 0 ? (
              <>
                <div className="sticky top-0 bg-gray-50 border-b border-gray-200 px-3 py-1 flex justify-between">
                  <button
                    type="button"
                    onClick={handleSelectAllLists}
                    className="text-xs text-blue-600 hover:text-blue-800"
                  >
                    Selecionar todas
                  </button>
                  <button
                    type="button"
                    onClick={handleClearLists}
                    className="text-xs text-gray-600 hover:text-gray-800"
                  >
                    Limpar
                  </button>
                </div>
                {lists.map((list) => (
                  <label
                    key={list.id}
                    className="flex items-center px-3 py-2 hover:bg-gray-50 cursor-pointer"
                  >
                    <input
                      type="checkbox"
                      checked={selectedLists.includes(list.id)}
                      onChange={() => handleListToggle(list.id)}
                      className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                    />
                    <span className="ml-2 text-sm text-gray-700">{list.name}</span>
                  </label>
                ))}
              </>
            ) : (
              <div className="px-3 py-2 text-sm text-gray-500">
                {!selectedFolder ? 'Selecione um folder primeiro' : 'Nenhuma lista disponível'}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Custom Fields Selector */}
      <div className="mb-6">
        <label className="block text-sm font-medium text-gray-700 mb-2">
          Campos Personalizados ({selectedFields.length} selecionados)
        </label>
        <div 
          data-testid="field-selector"
          className="border border-gray-300 rounded-md shadow-sm max-h-48 overflow-y-auto bg-white"
        >
          {hierarchyData?.custom_fields && hierarchyData.custom_fields.length > 0 ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-1 p-2">
              {hierarchyData.custom_fields.map((field) => (
                <label
                  key={field.id}
                  className="flex items-center px-2 py-1 hover:bg-gray-50 cursor-pointer rounded"
                >
                  <input
                    type="checkbox"
                    checked={selectedFields.includes(field.id)}
                    onChange={() => handleFieldToggle(field.id)}
                    className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                  />
                  <span className="ml-2 text-sm text-gray-700">{field.name}</span>
                  <span className="ml-1 text-xs text-gray-400">({field.type})</span>
                </label>
              ))}
            </div>
          ) : (
            <div className="px-3 py-4 text-sm text-gray-500 text-center">
              Nenhum campo personalizado disponível. Sincronize os metadados na aba Configurações.
            </div>
          )}
        </div>
      </div>

      {/* Options */}
      <div className="mb-6 flex flex-wrap gap-6">
        <label className="flex items-center cursor-pointer">
          <input
            type="checkbox"
            data-testid="subtasks-checkbox"
            checked={includeSubtasks}
            onChange={(e) => setIncludeSubtasks(e.target.checked)}
            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
          />
          <span className="ml-2 text-sm text-gray-700">Incluir subtasks</span>
        </label>
        <label className="flex items-center cursor-pointer">
          <input
            type="checkbox"
            data-testid="closed-tasks-checkbox"
            checked={includeClosedTasks}
            onChange={(e) => setIncludeClosedTasks(e.target.checked)}
            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
          />
          <span className="ml-2 text-sm text-gray-700">Incluir tarefas fechadas</span>
        </label>
      </div>

      {/* Error Message */}
      {generateError && (
        <div className="mb-4 bg-red-50 border border-red-200 rounded-lg p-3">
          <p className="text-sm text-red-700">{generateError}</p>
        </div>
      )}

      {/* Generate Button */}
      <div className="flex justify-end">
        <button
          type="button"
          data-testid="generate-report-btn"
          onClick={handleGenerateReport}
          disabled={!isFormValid || isGenerating}
          className="px-6 py-2 bg-blue-600 text-white font-medium rounded-md shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:bg-gray-400 disabled:cursor-not-allowed flex items-center"
        >
          {isGenerating ? (
            <>
              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
              Gerando...
            </>
          ) : (
            'Gerar Relatório Excel'
          )}
        </button>
      </div>
    </div>
  )
}
