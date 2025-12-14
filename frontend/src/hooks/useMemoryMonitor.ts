import { useState, useEffect, useCallback } from 'react'

export interface MemoryInfo {
  usedJSHeapSize: number
  totalJSHeapSize: number
  jsHeapSizeLimit: number
  usedMB: number
  totalMB: number
  limitMB: number
  usagePercent: number
}

interface PerformanceMemory {
  usedJSHeapSize: number
  totalJSHeapSize: number
  jsHeapSizeLimit: number
}

interface PerformanceWithMemory extends Performance {
  memory?: PerformanceMemory
}

const BYTES_TO_MB = 1024 * 1024

/**
 * Hook to monitor browser memory usage
 * Note: memory API is only available in Chrome/Chromium browsers
 */
export function useMemoryMonitor(intervalMs: number = 5000) {
  const [memoryInfo, setMemoryInfo] = useState<MemoryInfo | null>(null)
  const [isSupported, setIsSupported] = useState(false)

  const updateMemory = useCallback(() => {
    const perf = performance as PerformanceWithMemory
    if (perf.memory) {
      const { usedJSHeapSize, totalJSHeapSize, jsHeapSizeLimit } = perf.memory
      setMemoryInfo({
        usedJSHeapSize,
        totalJSHeapSize,
        jsHeapSizeLimit,
        usedMB: Math.round(usedJSHeapSize / BYTES_TO_MB * 100) / 100,
        totalMB: Math.round(totalJSHeapSize / BYTES_TO_MB * 100) / 100,
        limitMB: Math.round(jsHeapSizeLimit / BYTES_TO_MB * 100) / 100,
        usagePercent: Math.round((usedJSHeapSize / jsHeapSizeLimit) * 100),
      })
    }
  }, [])

  useEffect(() => {
    const perf = performance as PerformanceWithMemory
    const supported = !!perf.memory
    setIsSupported(supported)

    if (!supported) {
      return
    }

    // Initial update
    updateMemory()

    // Set up interval
    const interval = setInterval(updateMemory, intervalMs)

    return () => clearInterval(interval)
  }, [intervalMs, updateMemory])

  return { memoryInfo, isSupported, refresh: updateMemory }
}

/**
 * Fetch backend memory stats
 */
export async function fetchBackendMemoryStats(): Promise<{
  alloc_mb: number
  total_alloc_mb: number
  sys_mb: number
  heap_alloc_mb: number
  heap_inuse_mb: number
  heap_objects: number
  goroutines: number
  gc_runs: number
  gc_pause_total: number
} | null> {
  try {
    const response = await fetch('/debug/memory')
    if (!response.ok) return null
    return await response.json()
  } catch {
    return null
  }
}

/**
 * Fetch database pool stats
 */
export async function fetchDBPoolStats(): Promise<{
  max_open_connections: number
  open_connections: number
  in_use: number
  idle: number
  wait_count: number
  wait_duration_ms: number
  max_idle_closed: number
  max_idle_time_closed: number
  max_lifetime_closed: number
} | null> {
  try {
    const response = await fetch('/debug/db-pool')
    if (!response.ok) return null
    return await response.json()
  } catch {
    return null
  }
}

export default useMemoryMonitor
