import { createContext, useContext, useState, useEffect, ReactNode, useCallback } from 'react'

interface AuthContextType {
  isAuthenticated: boolean
  isLoading: boolean
  user: string | null
  csrfToken: string | null
  login: (username: string, password: string) => Promise<boolean>
  logout: () => Promise<void>
  getCSRFHeaders: () => Record<string, string>
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [user, setUser] = useState<string | null>(null)
  const [csrfToken, setCSRFToken] = useState<string | null>(null)

  useEffect(() => {
    checkAuthStatus()
  }, [])

  const checkAuthStatus = async () => {
    try {
      const response = await fetch('/api/web/auth/status', {
        credentials: 'include',
      })
      if (response.ok) {
        const data = await response.json()
        setIsAuthenticated(true)
        setUser(data.user?.username || 'user')
        if (data.csrf_token) {
          setCSRFToken(data.csrf_token)
        }
      } else {
        setIsAuthenticated(false)
        setUser(null)
        setCSRFToken(null)
      }
    } catch {
      setIsAuthenticated(false)
      setUser(null)
      setCSRFToken(null)
    } finally {
      setIsLoading(false)
    }
  }

  const login = async (username: string, password: string): Promise<boolean> => {
    try {
      const response = await fetch('/api/auth/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include',
        body: JSON.stringify({ username, password }),
      })

      if (response.ok) {
        const data = await response.json()
        setIsAuthenticated(true)
        setUser(username)
        if (data.csrf_token) {
          setCSRFToken(data.csrf_token)
        }
        return true
      }
      return false
    } catch {
      return false
    }
  }

  const logout = async () => {
    try {
      await fetch('/api/auth/logout', {
        method: 'POST',
        credentials: 'include',
        headers: csrfToken ? { 'X-CSRF-Token': csrfToken } : {},
      })
    } finally {
      setIsAuthenticated(false)
      setUser(null)
      setCSRFToken(null)
    }
  }

  const getCSRFHeaders = useCallback((): Record<string, string> => {
    if (csrfToken) {
      return { 'X-CSRF-Token': csrfToken }
    }
    return {}
  }, [csrfToken])

  return (
    <AuthContext.Provider value={{ isAuthenticated, isLoading, user, csrfToken, login, logout, getCSRFHeaders }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}
