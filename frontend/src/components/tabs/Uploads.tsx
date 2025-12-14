import { useState, useCallback, useRef, useEffect } from 'react'
import { useJobProgress } from '../../contexts/WebSocketContext'
import { useToast } from '../../contexts/ToastContext'
import { useAuth } from '../../contexts/AuthContext'

// Processing State Component - uses WebSocket for real-time updates
function ProcessingState({ job, onReset }: { job: JobData; onReset: () => void }) {
  // Get real-time progress from WebSocket
  const wsProgress = useJobProgress(job.id)
  
  // Use WebSocket progress if available, otherwise fall back to job data
  const status = wsProgress?.status || job.status
  const processedRows = wsProgress?.processed_rows ?? job.processed_rows
  const totalRows = wsProgress?.total_rows ?? job.total_rows
  const successCount = wsProgress?.success_count ?? job.success_count
  const errorCount = wsProgress?.error_count ?? job.error_count
  const progress = wsProgress?.progress ?? (totalRows > 0 ? (processedRows / totalRows) * 100 : 0)
  
  const isCompleted = status === 'completed'
  const isFailed = status === 'failed'
  const isFinished = isCompleted || isFailed

  return (
    <div className={`border rounded-lg p-6 text-center ${
      isCompleted ? 'bg-green-50 border-green-200' : 
      isFailed ? 'bg-red-50 border-red-200' : 
      'bg-blue-50 border-blue-200'
    }`}>
      {!isFinished && (
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto mb-4"></div>
      )}
      {isCompleted && (
        <svg className="h-12 w-12 text-green-500 mx-auto mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      )}
      {isFailed && (
        <svg className="h-12 w-12 text-red-500 mx-auto mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      )}
      <h3 className={`text-lg font-medium mb-2 ${
        isCompleted ? 'text-green-900' : 
        isFailed ? 'text-red-900' : 
        'text-blue-900'
      }`}>
        {isCompleted ? 'Atualização Concluída' : 
         isFailed ? 'Atualização Falhou' : 
         'Processando Atualização'}
      </h3>
      <p className={`mb-4 ${
        isCompleted ? 'text-green-700' : 
        isFailed ? 'text-red-700' : 
        'text-blue-700'
      }`}>
        Job #{job.id} - {status}
      </p>
      <div className="max-w-md mx-auto">
        <div className={`flex justify-between text-sm mb-1 ${
          isCompleted ? 'text-green-600' : 
          isFailed ? 'text-red-600' : 
          'text-blue-600'
        }`}>
          <span>Progresso</span>
          <span>{processedRows} / {totalRows}</span>
        </div>
        <div className={`w-full rounded-full h-2 ${
          isCompleted ? 'bg-green-200' : 
          isFailed ? 'bg-red-200' : 
          'bg-blue-200'
        }`}>
          <div 
            className={`h-2 rounded-full transition-all duration-300 ${
              isCompleted ? 'bg-green-600' : 
              isFailed ? 'bg-red-600' : 
              'bg-blue-600'
            }`}
            style={{ width: `${progress}%` }}
            data-testid="job-progress"
          ></div>
        </div>
        <div className="flex justify-between text-sm mt-2">
          <span className="text-green-600">✓ {successCount} sucesso</span>
          <span className="text-red-600">✗ {errorCount} erros</span>
        </div>
      </div>
      {!isFinished && (
        <p className="mt-4 text-sm text-blue-600">
          Acompanhe o progresso na aba "Relatórios"
        </p>
      )}
      <button
        onClick={onReset}
        className={`mt-4 px-4 py-2 text-white rounded-md ${
          isCompleted ? 'bg-green-600 hover:bg-green-700' : 
          isFailed ? 'bg-red-600 hover:bg-red-700' : 
          'bg-blue-600 hover:bg-blue-700'
        }`}
      >
        Novo Upload
      </button>
    </div>
  )
}

// Types
interface FileUploadData {
  filename: string
  size: number
  content_type: string
  columns: string[]
  preview: string[][]
  temp_path: string
  total_rows: number
}

interface CustomFieldData {
  id: string
  name: string
  type: string
  options: Record<string, unknown>
}

interface ColumnMapping {
  column: string
  field_id: string
  field_name: string
  field_type: string
  is_required: boolean
  is_task_id: boolean
}

interface JobData {
  id: number
  status: string
  total_rows: number
  processed_rows: number
  success_count: number
  error_count: number
}

// Upload states
type UploadState = 'idle' | 'uploading' | 'uploaded' | 'mapping' | 'submitting' | 'processing'

export default function Uploads() {
  // Toast notifications
  const { showError, showSuccess } = useToast()
  
  // Auth context for CSRF token
  const { getCSRFHeaders } = useAuth()
  
  // File upload state
  const [uploadState, setUploadState] = useState<UploadState>('idle')
  const [uploadProgress, setUploadProgress] = useState(0)
  const [uploadError, setUploadError] = useState<string | null>(null)
  const [fileData, setFileData] = useState<FileUploadData | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Mapping state
  const [customFields, setCustomFields] = useState<CustomFieldData[]>([])
  const [mappings, setMappings] = useState<ColumnMapping[]>([])
  const [mappingErrors, setMappingErrors] = useState<string[]>([])
  const [taskIdColumn, setTaskIdColumn] = useState<string>('')

  // Job state
  const [currentJob, setCurrentJob] = useState<JobData | null>(null)
  const [jobTitle, setJobTitle] = useState('')

  // Fetch custom fields on mount
  useEffect(() => {
    fetchCustomFields()
  }, [])

  const fetchCustomFields = async () => {
    try {
      const response = await fetch('/api/web/metadata/hierarchy', {
        credentials: 'include',
      })
      if (response.ok) {
        const data = await response.json()
        if (data.success && data.data?.custom_fields) {
          setCustomFields(data.data.custom_fields)
        }
      }
    } catch (err) {
      console.error('Error fetching custom fields:', err)
    }
  }

  // File validation
  const validateFile = (file: File): string | null => {
    const validTypes = ['text/csv', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet']
    const validExtensions = ['.csv', '.xlsx']
    const maxSize = 10 * 1024 * 1024 // 10MB

    const extension = file.name.toLowerCase().slice(file.name.lastIndexOf('.'))
    if (!validExtensions.includes(extension) && !validTypes.includes(file.type)) {
      return 'Formato inválido. Apenas arquivos CSV e XLSX são aceitos.'
    }
    if (file.size > maxSize) {
      return 'Arquivo muito grande. O limite máximo é 10MB.'
    }
    return null
  }

  // Handle file upload
  const uploadFile = async (file: File) => {
    const validationError = validateFile(file)
    if (validationError) {
      showError(validationError, 'Arquivo Inválido')
      setUploadError(validationError)
      return
    }

    setUploadState('uploading')
    setUploadError(null)
    setUploadProgress(0)

    const formData = new FormData()
    formData.append('file', file)

    try {
      const xhr = new XMLHttpRequest()
      
      xhr.upload.addEventListener('progress', (e) => {
        if (e.lengthComputable) {
          const progress = Math.round((e.loaded / e.total) * 100)
          setUploadProgress(progress)
        }
      })

      const response = await new Promise<Response>((resolve, reject) => {
        xhr.onload = () => {
          resolve(new Response(xhr.response, {
            status: xhr.status,
            statusText: xhr.statusText,
          }))
        }
        xhr.onerror = () => reject(new Error('Network error'))
        xhr.open('POST', '/api/web/upload')
        xhr.withCredentials = true
        // Add CSRF token header
        const csrfHeaders = getCSRFHeaders()
        if (csrfHeaders['X-CSRF-Token']) {
          xhr.setRequestHeader('X-CSRF-Token', csrfHeaders['X-CSRF-Token'])
        }
        xhr.send(formData)
      })

      if (!response.ok) {
        const errorData = await response.json()
        throw new Error(errorData.error || 'Erro ao fazer upload')
      }

      const data = await response.json()
      if (data.success && data.data) {
        setFileData(data.data)
        initializeMappings(data.data.columns)
        setUploadState('uploaded')
        setJobTitle(file.name.replace(/\.[^/.]+$/, ''))
        showSuccess(`Arquivo "${file.name}" carregado com sucesso!`)
      } else {
        throw new Error('Resposta inválida do servidor')
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Erro desconhecido'
      // Check for network errors
      if (err instanceof Error && (err.message === 'Failed to fetch' || err.message.includes('Network error'))) {
        showError('Não foi possível conectar ao servidor. Verifique sua conexão.', 'Erro de Conexão')
      } else {
        showError(errorMessage, 'Erro no Upload')
      }
      setUploadError(errorMessage)
      setUploadState('idle')
    }
  }

  // Initialize mappings from columns
  const initializeMappings = (columns: string[]) => {
    const initialMappings: ColumnMapping[] = columns.map(col => ({
      column: col,
      field_id: '',
      field_name: '',
      field_type: '',
      is_required: false,
      is_task_id: col.toLowerCase().includes('id') && col.toLowerCase().includes('task'),
    }))
    setMappings(initialMappings)
    
    // Auto-detect task ID column
    const taskIdCol = columns.find(col => 
      col.toLowerCase().includes('id') && col.toLowerCase().includes('task')
    )
    if (taskIdCol) {
      setTaskIdColumn(taskIdCol)
    }
  }

  // Drag and drop handlers
  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(true)
  }, [])

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
  }, [])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)

    const files = e.dataTransfer.files
    if (files.length > 0) {
      uploadFile(files[0])
    }
  }, [])

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (files && files.length > 0) {
      uploadFile(files[0])
    }
  }

  const handleBrowseClick = () => {
    fileInputRef.current?.click()
  }

  // Handle mapping change
  const handleMappingChange = (columnIndex: number, fieldId: string) => {
    const field = customFields.find(f => f.id === fieldId)
    setMappings(prev => {
      const updated = [...prev]
      updated[columnIndex] = {
        ...updated[columnIndex],
        field_id: fieldId,
        field_name: field?.name || '',
        field_type: field?.type || '',
        is_task_id: false,
      }
      return updated
    })
    validateMappings()
  }

  // Handle task ID column change
  const handleTaskIdChange = (column: string) => {
    setTaskIdColumn(column)
    setMappings(prev => prev.map(m => ({
      ...m,
      is_task_id: m.column === column,
    })))
    validateMappings()
  }

  // Validate mappings
  const validateMappings = useCallback(() => {
    const errors: string[] = []
    
    // Check for task ID column
    if (!taskIdColumn) {
      errors.push('Selecione a coluna que contém o ID da task')
    }

    // Check for duplicate field mappings
    const fieldIds = mappings
      .filter(m => m.field_id && !m.is_task_id)
      .map(m => m.field_id)
    const duplicates = fieldIds.filter((id, index) => fieldIds.indexOf(id) !== index)
    if (duplicates.length > 0) {
      const dupNames = [...new Set(duplicates)].map(id => 
        customFields.find(f => f.id === id)?.name || id
      )
      errors.push(`Campos duplicados: ${dupNames.join(', ')}`)
    }

    setMappingErrors(errors)
    return errors.length === 0
  }, [taskIdColumn, mappings, customFields])

  useEffect(() => {
    if (uploadState === 'uploaded' || uploadState === 'mapping') {
      validateMappings()
    }
  }, [taskIdColumn, mappings, uploadState, validateMappings])

  // Submit mapping and create job
  const handleSubmit = async () => {
    if (!validateMappings() || !fileData) return

    setUploadState('submitting')
    setUploadError(null)

    try {
      // First save the mapping
      const mappingPayload = {
        file_path: fileData.temp_path,
        mappings: mappings.map(m => ({
          ...m,
          is_task_id: m.column === taskIdColumn,
        })),
        title: jobTitle || fileData.filename,
      }

      const mappingResponse = await fetch('/api/web/mapping', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          ...getCSRFHeaders(),
        },
        credentials: 'include',
        body: JSON.stringify(mappingPayload),
      })

      if (!mappingResponse.ok) {
        const errorData = await mappingResponse.json()
        throw new Error(errorData.error || 'Erro ao salvar mapeamento')
      }

      const mappingData = await mappingResponse.json()
      if (!mappingData.success) {
        const validationErrors = mappingData.validation?.errors || ['Erro de validação']
        throw new Error(validationErrors.join(', '))
      }

      // Create job
      const jobResponse = await fetch('/api/web/jobs', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          ...getCSRFHeaders(),
        },
        credentials: 'include',
        body: JSON.stringify({
          mapping_id: mappingData.data.id,
          title: jobTitle || fileData.filename,
        }),
      })

      if (!jobResponse.ok) {
        const errorData = await jobResponse.json()
        throw new Error(errorData.error || 'Erro ao criar job')
      }

      const jobData = await jobResponse.json()
      if (jobData.success && jobData.data) {
        setCurrentJob(jobData.data)
        setUploadState('processing')
        showSuccess('Job de atualização criado com sucesso!')
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Erro desconhecido'
      showError(errorMessage, 'Erro ao Criar Job')
      setUploadError(errorMessage)
      setUploadState('uploaded')
    }
  }

  // Reset to initial state
  const handleReset = async () => {
    if (fileData?.temp_path) {
      try {
        await fetch('/api/web/upload/cleanup', {
          method: 'POST',
          headers: { 
            'Content-Type': 'application/json',
            ...getCSRFHeaders(),
          },
          credentials: 'include',
          body: JSON.stringify({ temp_path: fileData.temp_path }),
        })
      } catch (err) {
        console.error('Error cleaning up temp file:', err)
      }
    }
    setUploadState('idle')
    setFileData(null)
    setMappings([])
    setTaskIdColumn('')
    setUploadError(null)
    setCurrentJob(null)
    setJobTitle('')
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  // Check if form is valid for submission
  const isFormValid = taskIdColumn !== '' && 
    mappingErrors.length === 0 && 
    mappings.some(m => m.field_id || m.is_task_id)

  // Format file size
  const formatFileSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  return (
    <div data-testid="uploads-tab">
      <h2 className="text-lg font-medium text-gray-900 mb-4">Uploads</h2>
      <p className="text-gray-600 mb-6">
        Faça upload de arquivos CSV ou XLSX para atualizar campos personalizados.
      </p>

      {/* Error Message */}
      {uploadError && (
        <div className="mb-4 bg-red-50 border border-red-200 rounded-lg p-4">
          <p className="text-red-700">{uploadError}</p>
        </div>
      )}

      {/* Upload Area - Show when idle or uploading */}
      {(uploadState === 'idle' || uploadState === 'uploading') && (
        <div
          data-testid="upload-dropzone"
          onDragEnter={handleDragEnter}
          onDragLeave={handleDragLeave}
          onDragOver={handleDragOver}
          onDrop={handleDrop}
          onClick={handleBrowseClick}
          className={`
            border-2 border-dashed rounded-lg p-8 text-center cursor-pointer transition-colors
            ${isDragging 
              ? 'border-blue-500 bg-blue-50' 
              : 'border-gray-300 hover:border-gray-400 hover:bg-gray-50'
            }
            ${uploadState === 'uploading' ? 'pointer-events-none opacity-75' : ''}
          `}
        >
          <input
            ref={fileInputRef}
            type="file"
            accept=".csv,.xlsx"
            onChange={handleFileSelect}
            className="hidden"
            data-testid="file-input"
          />
          
          {uploadState === 'uploading' ? (
            <div className="space-y-4">
              <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto"></div>
              <p className="text-gray-600">Enviando arquivo... {uploadProgress}%</p>
              <div className="w-full bg-gray-200 rounded-full h-2 max-w-xs mx-auto">
                <div 
                  className="bg-blue-600 h-2 rounded-full transition-all duration-300"
                  style={{ width: `${uploadProgress}%` }}
                  data-testid="upload-progress"
                ></div>
              </div>
            </div>
          ) : (
            <>
              <svg className="mx-auto h-12 w-12 text-gray-400" stroke="currentColor" fill="none" viewBox="0 0 48 48">
                <path d="M28 8H12a4 4 0 00-4 4v20m32-12v8m0 0v8a4 4 0 01-4 4H12a4 4 0 01-4-4v-4m32-4l-3.172-3.172a4 4 0 00-5.656 0L28 28M8 32l9.172-9.172a4 4 0 015.656 0L28 28m0 0l4 4m4-24h8m-4-4v8m-12 4h.02" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
              <p className="mt-4 text-gray-600">
                <span className="text-blue-600 font-medium">Clique para selecionar</span> ou arraste um arquivo
              </p>
              <p className="mt-2 text-sm text-gray-500">CSV ou XLSX (máx. 10MB)</p>
            </>
          )}
        </div>
      )}

      {/* File Preview and Mapping - Show when uploaded */}
      {(uploadState === 'uploaded' || uploadState === 'mapping' || uploadState === 'submitting') && fileData && (
        <div className="space-y-6">
          {/* File Info */}
          <div className="bg-gray-50 rounded-lg p-4 flex items-center justify-between">
            <div className="flex items-center space-x-3">
              <svg className="h-8 w-8 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <div>
                <p className="font-medium text-gray-900">{fileData.filename}</p>
                <p className="text-sm text-gray-500">
                  {formatFileSize(fileData.size)} • {fileData.total_rows} linhas • {fileData.columns.length} colunas
                </p>
              </div>
            </div>
            <button
              onClick={handleReset}
              className="text-gray-500 hover:text-gray-700"
              title="Remover arquivo"
            >
              <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          {/* Preview Table */}
          <div>
            <h3 className="text-sm font-medium text-gray-700 mb-2">Preview (primeiras 5 linhas)</h3>
            <div className="overflow-x-auto border border-gray-200 rounded-lg" data-testid="preview-table">
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    {fileData.columns.map((col, idx) => (
                      <th key={idx} className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        {col}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                  {fileData.preview.map((row, rowIdx) => (
                    <tr key={rowIdx}>
                      {row.map((cell, cellIdx) => (
                        <td key={cellIdx} className="px-4 py-2 text-sm text-gray-900 whitespace-nowrap">
                          {cell || <span className="text-gray-400 italic">vazio</span>}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Column Mapping */}
          <div>
            <h3 className="text-sm font-medium text-gray-700 mb-2">Mapeamento de Colunas</h3>
            
            {/* Task ID Column Selection */}
            <div className="mb-4 p-4 bg-yellow-50 border border-yellow-200 rounded-lg">
              <label className="block text-sm font-medium text-yellow-800 mb-2">
                Coluna de ID da Task (obrigatório)
              </label>
              <select
                value={taskIdColumn}
                onChange={(e) => handleTaskIdChange(e.target.value)}
                className="w-full max-w-xs px-3 py-2 border border-yellow-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-yellow-500 focus:border-yellow-500"
                data-testid="task-id-select"
              >
                <option value="">Selecione a coluna com ID da task</option>
                {fileData.columns.map((col, idx) => (
                  <option key={idx} value={col}>{col}</option>
                ))}
              </select>
            </div>

            {/* Mapping Errors */}
            {mappingErrors.length > 0 && (
              <div className="mb-4 bg-red-50 border border-red-200 rounded-lg p-3">
                <ul className="list-disc list-inside text-sm text-red-700">
                  {mappingErrors.map((error, idx) => (
                    <li key={idx}>{error}</li>
                  ))}
                </ul>
              </div>
            )}

            {/* Column to Field Mapping */}
            <div className="space-y-3" data-testid="mapping-interface">
              {mappings.map((mapping, idx) => (
                <div key={idx} className="flex items-center space-x-4 p-3 bg-gray-50 rounded-lg">
                  <div className="flex-1">
                    <span className="text-sm font-medium text-gray-700">{mapping.column}</span>
                    {mapping.column === taskIdColumn && (
                      <span className="ml-2 text-xs bg-yellow-100 text-yellow-800 px-2 py-0.5 rounded">ID Task</span>
                    )}
                  </div>
                  <svg className="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
                  </svg>
                  <div className="flex-1">
                    <select
                      value={mapping.field_id}
                      onChange={(e) => handleMappingChange(idx, e.target.value)}
                      disabled={mapping.column === taskIdColumn}
                      className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 disabled:bg-gray-100 disabled:cursor-not-allowed text-sm"
                      data-testid={`mapping-select-${idx}`}
                    >
                      <option value="">Não mapear</option>
                      {customFields.map((field) => (
                        <option key={field.id} value={field.id}>
                          {field.name} ({field.type})
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Job Title and Submit */}
          <div className="border-t border-gray-200 pt-6">
            <div className="mb-4">
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Título do Job
              </label>
              <input
                type="text"
                value={jobTitle}
                onChange={(e) => setJobTitle(e.target.value)}
                placeholder="Nome para identificar esta atualização"
                className="w-full max-w-md px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                data-testid="job-title-input"
              />
            </div>
            <div className="flex justify-end space-x-3">
              <button
                onClick={handleReset}
                className="px-4 py-2 border border-gray-300 rounded-md text-gray-700 hover:bg-gray-50"
              >
                Cancelar
              </button>
              <button
                onClick={handleSubmit}
                disabled={!isFormValid || uploadState === 'submitting'}
                className="px-6 py-2 bg-blue-600 text-white font-medium rounded-md shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:bg-gray-400 disabled:cursor-not-allowed flex items-center"
                data-testid="submit-btn"
              >
                {uploadState === 'submitting' ? (
                  <>
                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                    Enviando...
                  </>
                ) : (
                  'Iniciar Atualização'
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Processing State */}
      {uploadState === 'processing' && currentJob && (
        <ProcessingState 
          job={currentJob} 
          onReset={handleReset} 
        />
      )}
    </div>
  )
}
