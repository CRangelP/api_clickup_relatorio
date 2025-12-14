/**
 * WebSocket Context
 * 
 * Provides WebSocket connection state and progress updates to all components.
 * Maintains connection across tab navigation.
 * 
 * Requirements: 5.1, 5.2, 8.2, 11.2, 11.3, 11.4
 */

import { createContext, useContext, useEffect, useState, useCallback, useRef, ReactNode } from 'react'
import { 
  getWebSocketClient, 
  WebSocketStatus, 
  WebSocketMessage, 
  ProgressUpdate,
  MetadataSyncUpdate 
} from '../services/websocket'

export interface ProgressState {
  [jobId: number]: ProgressUpdate
}

export interface MetadataSyncState {
  status: 'idle' | 'syncing' | 'completed' | 'error'
  message: string
}

interface WebSocketContextType {
  // Connection state
  status: WebSocketStatus
  isConnected: boolean
  
  // Progress updates by job ID
  progressUpdates: ProgressState
  
  // Metadata sync state
  metadataSyncState: MetadataSyncState
  
  // Methods
  connect: () => void
  disconnect: () => void
  clearProgress: (jobId: number) => void
  clearAllProgress: () => void
  
  // Subscribe to specific job progress
  subscribeToJob: (jobId: number, callback: (progress: ProgressUpdate) => void) => () => void
}

const WebSocketContext = createContext<WebSocketContextType | undefined>(undefined)

interface WebSocketProviderProps {
  children: ReactNode
  autoConnect?: boolean
}

export function WebSocketProvider({ children, autoConnect = true }: WebSocketProviderProps) {
  const [status, setStatus] = useState<WebSocketStatus>('disconnected')
  const [progressUpdates, setProgressUpdates] = useState<ProgressState>({})
  const [metadataSyncState, setMetadataSyncState] = useState<MetadataSyncState>({
    status: 'idle',
    message: '',
  })
  
  // Job-specific subscribers
  const jobSubscribers = useRef<Map<number, Set<(progress: ProgressUpdate) => void>>>(new Map())
  
  const wsClient = getWebSocketClient()

  // Handle incoming messages
  const handleMessage = useCallback((message: WebSocketMessage) => {
    if (message.type === 'progress') {
      const progressMessage = message as ProgressUpdate
      if (progressMessage.job_id) {
        setProgressUpdates(prev => ({
          ...prev,
          [progressMessage.job_id!]: progressMessage,
        }))
        
        // Notify job-specific subscribers
        const subscribers = jobSubscribers.current.get(progressMessage.job_id)
        if (subscribers) {
          subscribers.forEach(callback => {
            try {
              callback(progressMessage)
            } catch (error) {
              console.error('Error in job subscriber callback:', error)
            }
          })
        }
      }
    } else if (message.type === 'metadata_sync') {
      const syncMessage = message as MetadataSyncUpdate
      const { status: syncStatus, message: syncMsg } = syncMessage.data
      
      if (syncStatus === 'started') {
        setMetadataSyncState({ status: 'syncing', message: syncMsg })
      } else if (syncStatus === 'completed') {
        setMetadataSyncState({ status: 'completed', message: syncMsg })
        // Reset to idle after 3 seconds
        setTimeout(() => {
          setMetadataSyncState({ status: 'idle', message: '' })
        }, 3000)
      } else if (syncStatus === 'error') {
        setMetadataSyncState({ status: 'error', message: syncMsg })
      }
    }
  }, [])

  // Handle status changes
  const handleStatusChange = useCallback((newStatus: WebSocketStatus) => {
    setStatus(newStatus)
  }, [])

  // Connect to WebSocket
  const connect = useCallback(() => {
    wsClient.connect()
  }, [wsClient])

  // Disconnect from WebSocket
  const disconnect = useCallback(() => {
    wsClient.disconnect()
  }, [wsClient])

  // Clear progress for a specific job
  const clearProgress = useCallback((jobId: number) => {
    setProgressUpdates(prev => {
      const updated = { ...prev }
      delete updated[jobId]
      return updated
    })
  }, [])

  // Clear all progress updates
  const clearAllProgress = useCallback(() => {
    setProgressUpdates({})
  }, [])

  // Subscribe to specific job progress
  const subscribeToJob = useCallback((jobId: number, callback: (progress: ProgressUpdate) => void) => {
    if (!jobSubscribers.current.has(jobId)) {
      jobSubscribers.current.set(jobId, new Set())
    }
    jobSubscribers.current.get(jobId)!.add(callback)
    
    // Return unsubscribe function
    return () => {
      const subscribers = jobSubscribers.current.get(jobId)
      if (subscribers) {
        subscribers.delete(callback)
        if (subscribers.size === 0) {
          jobSubscribers.current.delete(jobId)
        }
      }
    }
  }, [])

  // Set up WebSocket connection and listeners
  useEffect(() => {
    const unsubscribeMessage = wsClient.onMessage(handleMessage)
    const unsubscribeStatus = wsClient.onStatusChange(handleStatusChange)

    if (autoConnect) {
      connect()
    }

    return () => {
      unsubscribeMessage()
      unsubscribeStatus()
    }
  }, [wsClient, handleMessage, handleStatusChange, autoConnect, connect])

  const value: WebSocketContextType = {
    status,
    isConnected: status === 'connected',
    progressUpdates,
    metadataSyncState,
    connect,
    disconnect,
    clearProgress,
    clearAllProgress,
    subscribeToJob,
  }

  return (
    <WebSocketContext.Provider value={value}>
      {children}
    </WebSocketContext.Provider>
  )
}

/**
 * Hook to access WebSocket context
 */
export function useWebSocket() {
  const context = useContext(WebSocketContext)
  if (context === undefined) {
    throw new Error('useWebSocket must be used within a WebSocketProvider')
  }
  return context
}

/**
 * Hook to get progress for a specific job
 */
export function useJobProgress(jobId: number | null | undefined): ProgressUpdate | null {
  const { progressUpdates } = useWebSocket()
  
  if (jobId === null || jobId === undefined) {
    return null
  }
  
  return progressUpdates[jobId] || null
}

/**
 * Hook to subscribe to a specific job's progress with callback
 */
export function useJobProgressSubscription(
  jobId: number | null | undefined,
  callback: (progress: ProgressUpdate) => void
): void {
  const { subscribeToJob } = useWebSocket()
  
  useEffect(() => {
    if (jobId === null || jobId === undefined) {
      return
    }
    
    return subscribeToJob(jobId, callback)
  }, [jobId, callback, subscribeToJob])
}
