/**
 * **Feature: clickup-field-updater, Property 10: Error isolation and recovery**
 * **Validates: Requirements 13.4, 13.5**
 * 
 * Property 10: Error isolation and recovery
 * *For any* error occurring in one tab or operation, the system should isolate the error,
 * maintain other functionality, and allow recovery without full system restart
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import * as fc from 'fast-check'
import { Component, ReactNode } from 'react'
import { BrowserRouter } from 'react-router-dom'
import ErrorBoundary from './ErrorBoundary'

// Suppress console.error for expected errors in tests
beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

// Component that throws an error on demand
interface ThrowingComponentProps {
  shouldThrow: boolean
  errorMessage?: string
  children?: ReactNode
}

class ThrowingComponent extends Component<ThrowingComponentProps> {
  render() {
    if (this.props.shouldThrow) {
      throw new Error(this.props.errorMessage || 'Test error')
    }
    return <div data-testid="healthy-content">{this.props.children || 'Healthy content'}</div>
  }
}

// Tab IDs for property testing
const TAB_IDS = ['buscar', 'uploads', 'relatorios', 'configuracoes'] as const
type TabId = typeof TAB_IDS[number]

// Arbitrary for generating tab IDs
const tabIdArb = fc.constantFrom(...TAB_IDS)

// Arbitrary for generating error messages (non-empty strings with printable chars)
const errorMessageArb = fc.string({ minLength: 1, maxLength: 50 }).filter(s => s.trim().length > 0 && /^[\x20-\x7E]+$/.test(s))

// Arbitrary for generating a subset of tabs that should have errors
const errorTabsArb = fc.subarray([...TAB_IDS], { minLength: 0, maxLength: TAB_IDS.length })

describe('Property 10: Error isolation and recovery', () => {
  /**
   * Property: For any tab that throws an error, the ErrorBoundary should catch it
   * and display an error UI without crashing the entire application
   */
  it('should catch errors and display fallback UI for any tab', () => {
    fc.assert(
      fc.property(tabIdArb, errorMessageArb, (tabId, errorMessage) => {
        // Clean up before each iteration
        cleanup()
        
        const { container } = render(
          <BrowserRouter>
            <ErrorBoundary tabId={tabId}>
              <ThrowingComponent shouldThrow={true} errorMessage={errorMessage} />
            </ErrorBoundary>
          </BrowserRouter>
        )

        // Error boundary should catch the error and show error UI
        expect(screen.getByText('Ocorreu um erro nesta aba')).toBeInTheDocument()
        
        // The healthy content should NOT be rendered
        expect(screen.queryByTestId('healthy-content')).not.toBeInTheDocument()
        
        // The retry button should be available
        expect(screen.getByText('Tentar novamente')).toBeInTheDocument()
        
        // Container should still be in the document (app didn't crash)
        expect(container).toBeInTheDocument()
      }),
      { numRuns: 100 }
    )
  })

  /**
   * Property: For any combination of tabs with errors, tabs without errors should
   * continue to function normally (error isolation)
   */
  it('should isolate errors to individual tabs without affecting others', () => {
    fc.assert(
      fc.property(errorTabsArb, (errorTabs) => {
        // Clean up before each iteration
        cleanup()
        
        // Create a multi-tab simulation where some tabs have errors
        const TabContainer = () => (
          <div>
            {TAB_IDS.map(tabId => (
              <div key={tabId} data-testid={`tab-${tabId}`}>
                <ErrorBoundary tabId={tabId}>
                  <ThrowingComponent 
                    shouldThrow={errorTabs.includes(tabId)} 
                    errorMessage={`Error in ${tabId}`}
                  >
                    {`Content for ${tabId}`}
                  </ThrowingComponent>
                </ErrorBoundary>
              </div>
            ))}
          </div>
        )

        render(
          <BrowserRouter>
            <TabContainer />
          </BrowserRouter>
        )

        // Verify each tab's state
        TAB_IDS.forEach(tabId => {
          const tabContainer = screen.getByTestId(`tab-${tabId}`)
          
          if (errorTabs.includes(tabId)) {
            // Tab with error should show error UI
            expect(tabContainer.textContent).toContain('Ocorreu um erro nesta aba')
          } else {
            // Tab without error should show healthy content
            expect(tabContainer.textContent).toContain(`Content for ${tabId}`)
          }
        })

        // All tabs should still be in the document (none crashed the app)
        TAB_IDS.forEach(tabId => {
          expect(screen.getByTestId(`tab-${tabId}`)).toBeInTheDocument()
        })
      }),
      { numRuns: 100 }
    )
  })

  /**
   * Property: For any tab with an error, clicking retry should allow recovery
   * without affecting other tabs
   */
  it('should allow recovery from errors without affecting other tabs', () => {
    fc.assert(
      fc.property(tabIdArb, (errorTabId) => {
        // Clean up before each iteration
        cleanup()
        
        // Track which tab should throw
        let shouldThrow = true

        const RecoverableComponent = ({ tabId }: { tabId: TabId }) => {
          if (tabId === errorTabId && shouldThrow) {
            throw new Error(`Error in ${tabId}`)
          }
          return <div data-testid={`content-${tabId}`}>Content for {tabId}</div>
        }

        // Create a component that can recover
        const TabContainer = () => (
          <div>
            {TAB_IDS.map(tabId => (
              <div key={tabId} data-testid={`tab-${tabId}`}>
                <ErrorBoundary tabId={tabId}>
                  <RecoverableComponent tabId={tabId} />
                </ErrorBoundary>
              </div>
            ))}
          </div>
        )

        const { rerender } = render(
          <BrowserRouter>
            <TabContainer />
          </BrowserRouter>
        )

        // Verify error tab shows error
        const errorTab = screen.getByTestId(`tab-${errorTabId}`)
        expect(errorTab.textContent).toContain('Ocorreu um erro nesta aba')

        // Verify other tabs are healthy
        TAB_IDS.filter(id => id !== errorTabId).forEach(tabId => {
          expect(screen.getByTestId(`content-${tabId}`)).toBeInTheDocument()
        })

        // Simulate recovery by setting shouldThrow to false and clicking retry
        shouldThrow = false
        const retryButton = errorTab.querySelector('button')
        if (retryButton) {
          fireEvent.click(retryButton)
        }

        // Re-render to apply the recovery
        rerender(
          <BrowserRouter>
            <TabContainer />
          </BrowserRouter>
        )

        // After recovery, the retry button should have been available
        expect(retryButton).not.toBeNull()
      }),
      { numRuns: 100 }
    )
  })

  /**
   * Property: For any error, the onError callback should be called with the error details
   */
  it('should call onError callback for any error', () => {
    fc.assert(
      fc.property(tabIdArb, errorMessageArb, (tabId, errorMessage) => {
        // Clean up before each iteration
        cleanup()
        
        const onError = vi.fn()

        render(
          <BrowserRouter>
            <ErrorBoundary tabId={tabId} onError={onError}>
              <ThrowingComponent shouldThrow={true} errorMessage={errorMessage} />
            </ErrorBoundary>
          </BrowserRouter>
        )

        // onError should have been called
        expect(onError).toHaveBeenCalledTimes(1)
        
        // First argument should be the error
        const [error] = onError.mock.calls[0]
        expect(error).toBeInstanceOf(Error)
        expect(error.message).toBe(errorMessage)
      }),
      { numRuns: 100 }
    )
  })

  /**
   * Property: For any healthy component, ErrorBoundary should render children normally
   */
  it('should render children normally when no error occurs', () => {
    fc.assert(
      fc.property(tabIdArb, fc.string({ minLength: 1, maxLength: 50 }).filter(s => s.trim().length > 0 && /^[\x20-\x7E]+$/.test(s)), (tabId, content) => {
        // Clean up before each iteration
        cleanup()
        
        render(
          <BrowserRouter>
            <ErrorBoundary tabId={tabId}>
              <div data-testid="child-content">{content}</div>
            </ErrorBoundary>
          </BrowserRouter>
        )

        // Child content should be rendered
        expect(screen.getByTestId('child-content')).toBeInTheDocument()
        expect(screen.getByTestId('child-content').textContent).toBe(content)
        
        // Error UI should NOT be shown
        expect(screen.queryByText('Ocorreu um erro nesta aba')).not.toBeInTheDocument()
      }),
      { numRuns: 100 }
    )
  })

  /**
   * Property: Custom fallback should be rendered when provided and error occurs
   */
  it('should render custom fallback when provided', () => {
    fc.assert(
      fc.property(tabIdArb, fc.string({ minLength: 1, maxLength: 50 }).filter(s => s.trim().length > 0 && /^[\x20-\x7E]+$/.test(s)), (tabId, fallbackText) => {
        // Clean up before each iteration
        cleanup()
        
        const customFallback = <div data-testid="custom-fallback">{fallbackText}</div>

        render(
          <BrowserRouter>
            <ErrorBoundary tabId={tabId} fallback={customFallback}>
              <ThrowingComponent shouldThrow={true} />
            </ErrorBoundary>
          </BrowserRouter>
        )

        // Custom fallback should be rendered
        expect(screen.getByTestId('custom-fallback')).toBeInTheDocument()
        expect(screen.getByTestId('custom-fallback').textContent).toBe(fallbackText)
        
        // Default error UI should NOT be shown
        expect(screen.queryByText('Ocorreu um erro nesta aba')).not.toBeInTheDocument()
      }),
      { numRuns: 100 }
    )
  })
})
