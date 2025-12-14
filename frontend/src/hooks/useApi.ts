import { useAuth } from '../contexts/AuthContext'
import { useCallback } from 'react'

interface FetchOptions extends RequestInit {
  skipCSRF?: boolean
}

/**
 * Custom hook for making API calls with CSRF token support
 */
export function useApi() {
  const { getCSRFHeaders, csrfToken } = useAuth()

  const fetchWithCSRF = useCallback(async (url: string, options: FetchOptions = {}): Promise<Response> => {
    const { skipCSRF = false, headers = {}, ...rest } = options

    // Build headers with CSRF token for state-changing requests
    const method = (options.method || 'GET').toUpperCase()
    const needsCSRF = !skipCSRF && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)

    const finalHeaders: Record<string, string> = {
      ...(headers as Record<string, string>),
    }

    if (needsCSRF) {
      const csrfHeaders = getCSRFHeaders()
      Object.assign(finalHeaders, csrfHeaders)
    }

    return fetch(url, {
      ...rest,
      headers: finalHeaders,
      credentials: 'include',
    })
  }, [getCSRFHeaders])

  return {
    fetchWithCSRF,
    csrfToken,
    getCSRFHeaders,
  }
}

export default useApi
