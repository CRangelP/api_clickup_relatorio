/**
 * WebSocket Client Service
 * 
 * Provides a centralized WebSocket connection that maintains connection
 * across tab navigation and handles automatic reconnection.
 * 
 * Requirements: 5.1, 11.3, 11.4
 */

export type WebSocketStatus = 'connecting' | 'connected' | 'disconnected' | 'error'

export interface ProgressUpdate {
  type: string
  job_id?: number
  status: string
  processed_rows?: number
  total_rows?: number
  success_count?: number
  error_count?: number
  message?: string
  progress?: number
  timestamp: string
}

export interface MetadataSyncUpdate {
  type: 'metadata_sync'
  data: {
    status: 'started' | 'completed' | 'error'
    message: string
  }
}

export interface ConnectionMessage {
  type: 'connection'
  data: {
    status: string
  }
  timestamp: string
}

export type WebSocketMessage = ProgressUpdate | MetadataSyncUpdate | ConnectionMessage

export type MessageHandler = (message: WebSocketMessage) => void

interface WebSocketClientOptions {
  reconnectInterval?: number
  maxReconnectAttempts?: number
  pingInterval?: number
}

const DEFAULT_OPTIONS: Required<WebSocketClientOptions> = {
  reconnectInterval: 3000,
  maxReconnectAttempts: 10,
  pingInterval: 30000,
}

class WebSocketClient {
  private ws: WebSocket | null = null
  private status: WebSocketStatus = 'disconnected'
  private reconnectAttempts = 0
  private reconnectTimeout: number | null = null
  private pingInterval: number | null = null
  private messageHandlers: Set<MessageHandler> = new Set()
  private statusHandlers: Set<(status: WebSocketStatus) => void> = new Set()
  private options: Required<WebSocketClientOptions>
  private isIntentionallyClosed = false

  constructor(options: WebSocketClientOptions = {}) {
    this.options = { ...DEFAULT_OPTIONS, ...options }
  }

  /**
   * Get the WebSocket URL based on current location
   */
  private getWebSocketUrl(): string {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${protocol}//${window.location.host}/api/web/ws`
  }

  /**
   * Connect to the WebSocket server
   */
  connect(): void {
    if (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) {
      return
    }

    this.isIntentionallyClosed = false
    this.setStatus('connecting')

    try {
      const url = this.getWebSocketUrl()
      this.ws = new WebSocket(url)

      this.ws.onopen = this.handleOpen.bind(this)
      this.ws.onmessage = this.handleMessage.bind(this)
      this.ws.onclose = this.handleClose.bind(this)
      this.ws.onerror = this.handleError.bind(this)
    } catch (error) {
      console.error('WebSocket connection error:', error)
      this.setStatus('error')
      this.scheduleReconnect()
    }
  }

  /**
   * Disconnect from the WebSocket server
   */
  disconnect(): void {
    this.isIntentionallyClosed = true
    this.clearReconnectTimeout()
    this.clearPingInterval()

    if (this.ws) {
      this.ws.close(1000, 'Client disconnecting')
      this.ws = null
    }

    this.setStatus('disconnected')
  }

  /**
   * Send a message to the server
   */
  send(message: object): boolean {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      console.warn('WebSocket is not connected, cannot send message')
      return false
    }

    try {
      this.ws.send(JSON.stringify(message))
      return true
    } catch (error) {
      console.error('Error sending WebSocket message:', error)
      return false
    }
  }

  /**
   * Subscribe to messages
   */
  onMessage(handler: MessageHandler): () => void {
    this.messageHandlers.add(handler)
    return () => this.messageHandlers.delete(handler)
  }

  /**
   * Subscribe to status changes
   */
  onStatusChange(handler: (status: WebSocketStatus) => void): () => void {
    this.statusHandlers.add(handler)
    // Immediately notify of current status
    handler(this.status)
    return () => this.statusHandlers.delete(handler)
  }

  /**
   * Get current connection status
   */
  getStatus(): WebSocketStatus {
    return this.status
  }

  /**
   * Check if connected
   */
  isConnected(): boolean {
    return this.status === 'connected' && this.ws?.readyState === WebSocket.OPEN
  }

  private handleOpen(): void {
    console.log('WebSocket connected')
    this.reconnectAttempts = 0
    this.setStatus('connected')
    this.startPingInterval()
  }

  private handleMessage(event: MessageEvent): void {
    try {
      const message = JSON.parse(event.data) as WebSocketMessage
      this.notifyMessageHandlers(message)
    } catch (error) {
      console.error('Error parsing WebSocket message:', error)
    }
  }

  private handleClose(event: CloseEvent): void {
    console.log('WebSocket disconnected:', event.code, event.reason)
    this.clearPingInterval()
    this.setStatus('disconnected')

    if (!this.isIntentionallyClosed) {
      this.scheduleReconnect()
    }
  }

  private handleError(event: Event): void {
    console.error('WebSocket error:', event)
    this.setStatus('error')
  }

  private setStatus(status: WebSocketStatus): void {
    if (this.status !== status) {
      this.status = status
      this.notifyStatusHandlers(status)
    }
  }

  private notifyMessageHandlers(message: WebSocketMessage): void {
    this.messageHandlers.forEach(handler => {
      try {
        handler(message)
      } catch (error) {
        console.error('Error in message handler:', error)
      }
    })
  }

  private notifyStatusHandlers(status: WebSocketStatus): void {
    this.statusHandlers.forEach(handler => {
      try {
        handler(status)
      } catch (error) {
        console.error('Error in status handler:', error)
      }
    })
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.options.maxReconnectAttempts) {
      console.warn('Max reconnect attempts reached')
      return
    }

    this.clearReconnectTimeout()
    
    // Exponential backoff with jitter
    const baseDelay = this.options.reconnectInterval
    const exponentialDelay = baseDelay * Math.pow(1.5, this.reconnectAttempts)
    const jitter = Math.random() * 1000
    const delay = Math.min(exponentialDelay + jitter, 30000) // Max 30 seconds

    console.log(`Scheduling reconnect in ${Math.round(delay)}ms (attempt ${this.reconnectAttempts + 1})`)

    this.reconnectTimeout = window.setTimeout(() => {
      this.reconnectAttempts++
      this.connect()
    }, delay)
  }

  private clearReconnectTimeout(): void {
    if (this.reconnectTimeout !== null) {
      clearTimeout(this.reconnectTimeout)
      this.reconnectTimeout = null
    }
  }

  private startPingInterval(): void {
    this.clearPingInterval()
    this.pingInterval = window.setInterval(() => {
      if (this.isConnected()) {
        this.send({ type: 'ping' })
      }
    }, this.options.pingInterval)
  }

  private clearPingInterval(): void {
    if (this.pingInterval !== null) {
      clearInterval(this.pingInterval)
      this.pingInterval = null
    }
  }
}

// Singleton instance
let wsClientInstance: WebSocketClient | null = null

/**
 * Get the singleton WebSocket client instance
 */
export function getWebSocketClient(): WebSocketClient {
  if (!wsClientInstance) {
    wsClientInstance = new WebSocketClient()
  }
  return wsClientInstance
}

/**
 * Reset the WebSocket client (useful for testing)
 */
export function resetWebSocketClient(): void {
  if (wsClientInstance) {
    wsClientInstance.disconnect()
    wsClientInstance = null
  }
}

export default WebSocketClient
