/**
 * **Feature: clickup-field-updater, Property 5: Hierarchical filtering correctness**
 * **Validates: Requirements 9.2, 9.3, 9.4**
 *
 * Property 5: Hierarchical filtering correctness
 * *For any* selected parent entity (workspace/space/folder), the system should display
 * only child entities that belong to the selected parent, maintaining referential integrity
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react'
import * as fc from 'fast-check'
import { BrowserRouter } from 'react-router-dom'
import BuscarTasks from './BuscarTasks'
import { ToastProvider } from '../../contexts/ToastContext'
import { AuthProvider } from '../../contexts/AuthContext'

// Suppress console.error for expected errors in tests
beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

// Types matching the component's internal types
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

// Arbitraries for generating test data
const idArb = fc.stringMatching(/^[a-z0-9]{8,16}$/)
// Use alphanumeric names to avoid substring matching issues
const nameArb = fc.stringMatching(/^[A-Za-z][A-Za-z0-9]{2,20}$/)

const listArb: fc.Arbitrary<ListData> = fc.record({
  id: idArb,
  name: nameArb,
})

const folderArb: fc.Arbitrary<FolderData> = fc.record({
  id: idArb,
  name: nameArb,
  lists: fc.array(listArb, { minLength: 0, maxLength: 5 }),
})

const spaceArb: fc.Arbitrary<SpaceData> = fc.record({
  id: idArb,
  name: nameArb,
  folders: fc.array(folderArb, { minLength: 0, maxLength: 5 }),
})

const workspaceArb: fc.Arbitrary<WorkspaceData> = fc.record({
  id: idArb,
  name: nameArb,
  spaces: fc.array(spaceArb, { minLength: 0, maxLength: 5 }),
})

const customFieldArb: fc.Arbitrary<CustomFieldData> = fc.record({
  id: idArb,
  name: nameArb,
  type: fc.constantFrom('text', 'number', 'dropdown', 'date', 'checkbox'),
  options: fc.constant({}),
})

const hierarchyDataArb: fc.Arbitrary<HierarchyData> = fc.record({
  workspaces: fc.array(workspaceArb, { minLength: 1, maxLength: 3 }),
  custom_fields: fc.array(customFieldArb, { minLength: 1, maxLength: 5 }),
})

// Helper to mock fetch with hierarchy data
const mockFetchWithHierarchy = (data: HierarchyData) => {
  global.fetch = vi.fn().mockImplementation((url: string) => {
    // Mock auth status endpoint
    if (url.includes('/api/web/auth/status')) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ user: { username: 'testuser' }, csrf_token: 'test-csrf-token' }),
      })
    }
    // Mock hierarchy data endpoint
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve({ success: true, data }),
    })
  })
}

// Helper to get all option values from a select element
const getSelectOptions = (select: HTMLSelectElement): string[] => {
  return Array.from(select.options)
    .filter(opt => opt.value !== '') // Exclude placeholder option
    .map(opt => opt.value)
}

describe('Property 5: Hierarchical filtering correctness', () => {
  /**
   * Property: When a workspace is selected, only spaces belonging to that workspace
   * should be available in the space selector
   * **Validates: Requirements 9.2**
   */
  it('should filter spaces by selected workspace', async () => {
    await fc.assert(
      fc.asyncProperty(hierarchyDataArb, async (hierarchyData) => {
        cleanup()
        mockFetchWithHierarchy(hierarchyData)

        render(
          <BrowserRouter>
            <AuthProvider>
              <ToastProvider>
                <BuscarTasks />
              </ToastProvider>
            </AuthProvider>
          </BrowserRouter>
        )

        // Wait for data to load
        await waitFor(() => {
          expect(screen.queryByText('Carregando dados...')).not.toBeInTheDocument()
        }, { timeout: 3000 })

        const workspaceSelector = screen.getByTestId('workspace-selector') as HTMLSelectElement
        const spaceSelector = screen.getByTestId('space-selector') as HTMLSelectElement

        // For each workspace in the data
        for (const workspace of hierarchyData.workspaces) {
          // Select the workspace
          fireEvent.change(workspaceSelector, { target: { value: workspace.id } })

          // Get available space options
          const availableSpaceIds = getSelectOptions(spaceSelector)

          // Expected space IDs are only those belonging to this workspace
          const expectedSpaceIds = workspace.spaces.map(s => s.id)

          // Property: Available spaces should exactly match the workspace's spaces
          expect(availableSpaceIds.sort()).toEqual(expectedSpaceIds.sort())

          // Property: No space from other workspaces should be available
          const otherWorkspaceSpaceIds = hierarchyData.workspaces
            .filter(w => w.id !== workspace.id)
            .flatMap(w => w.spaces.map(s => s.id))

          for (const otherSpaceId of otherWorkspaceSpaceIds) {
            expect(availableSpaceIds).not.toContain(otherSpaceId)
          }
        }
      }),
      { numRuns: 100 }
    )
  })

  /**
   * Property: When a space is selected, only folders belonging to that space
   * should be available in the folder selector
   * **Validates: Requirements 9.3**
   */
  it('should filter folders by selected space', async () => {
    await fc.assert(
      fc.asyncProperty(hierarchyDataArb, async (hierarchyData) => {
        cleanup()
        mockFetchWithHierarchy(hierarchyData)

        render(
          <BrowserRouter>
            <AuthProvider>
              <ToastProvider>
                <BuscarTasks />
              </ToastProvider>
            </AuthProvider>
          </BrowserRouter>
        )

        await waitFor(() => {
          expect(screen.queryByText('Carregando dados...')).not.toBeInTheDocument()
        }, { timeout: 3000 })

        const workspaceSelector = screen.getByTestId('workspace-selector') as HTMLSelectElement
        const spaceSelector = screen.getByTestId('space-selector') as HTMLSelectElement
        const folderSelector = screen.getByTestId('folder-selector') as HTMLSelectElement

        // For each workspace and space combination
        for (const workspace of hierarchyData.workspaces) {
          fireEvent.change(workspaceSelector, { target: { value: workspace.id } })

          for (const space of workspace.spaces) {
            fireEvent.change(spaceSelector, { target: { value: space.id } })

            const availableFolderIds = getSelectOptions(folderSelector)
            const expectedFolderIds = space.folders.map(f => f.id)

            // Property: Available folders should exactly match the space's folders
            expect(availableFolderIds.sort()).toEqual(expectedFolderIds.sort())

            // Property: No folder from other spaces should be available
            const otherSpaceFolderIds = workspace.spaces
              .filter(s => s.id !== space.id)
              .flatMap(s => s.folders.map(f => f.id))

            for (const otherFolderId of otherSpaceFolderIds) {
              expect(availableFolderIds).not.toContain(otherFolderId)
            }
          }
        }
      }),
      { numRuns: 100 }
    )
  })

  /**
   * Property: When a folder is selected, only lists belonging to that folder
   * should be available in the list selector
   * **Validates: Requirements 9.4**
   */
  it('should filter lists by selected folder', async () => {
    // Generate data with at least one folder with lists to test
    const hierarchyWithListsArb = hierarchyDataArb.filter(h =>
      h.workspaces.some(w =>
        w.spaces.some(s =>
          s.folders.some(f => f.lists.length > 0)
        )
      )
    )

    await fc.assert(
      fc.asyncProperty(hierarchyWithListsArb, async (hierarchyData) => {
        cleanup()
        mockFetchWithHierarchy(hierarchyData)

        render(
          <BrowserRouter>
            <AuthProvider>
              <ToastProvider>
                <BuscarTasks />
              </ToastProvider>
            </AuthProvider>
          </BrowserRouter>
        )

        await waitFor(() => {
          expect(screen.queryByText('Carregando dados...')).not.toBeInTheDocument()
        }, { timeout: 3000 })

        const workspaceSelector = screen.getByTestId('workspace-selector') as HTMLSelectElement
        const spaceSelector = screen.getByTestId('space-selector') as HTMLSelectElement
        const folderSelector = screen.getByTestId('folder-selector') as HTMLSelectElement

        // Find first workspace with spaces that have folders with lists
        const workspace = hierarchyData.workspaces.find(w =>
          w.spaces.some(s => s.folders.some(f => f.lists.length > 0))
        )
        if (!workspace) return

        fireEvent.change(workspaceSelector, { target: { value: workspace.id } })

        const space = workspace.spaces.find(s => s.folders.some(f => f.lists.length > 0))
        if (!space) return

        fireEvent.change(spaceSelector, { target: { value: space.id } })

        const folder = space.folders.find(f => f.lists.length > 0)
        if (!folder) return

        fireEvent.change(folderSelector, { target: { value: folder.id } })

        // Get list checkboxes from the list selector
        const listSelector = screen.getByTestId('list-selector')
        const listCheckboxes = listSelector.querySelectorAll('input[type="checkbox"]')

        // Property: Number of list checkboxes should match the folder's lists count
        expect(listCheckboxes.length).toBe(folder.lists.length)

        // Property: Each list name should appear in the selector
        for (const list of folder.lists) {
          const listLabel = Array.from(listSelector.querySelectorAll('label')).find(
            label => label.textContent?.includes(list.name)
          )
          expect(listLabel).toBeTruthy()
        }

        // Property: No list from other folders should be available
        const otherFolderLists = space.folders
          .filter(f => f.id !== folder.id)
          .flatMap(f => f.lists)

        for (const otherList of otherFolderLists) {
          const otherListLabel = Array.from(listSelector.querySelectorAll('label')).find(
            label => label.textContent?.includes(otherList.name)
          )
          // Only check if the name is unique (not shared with current folder's lists)
          if (!folder.lists.some(l => l.name === otherList.name)) {
            expect(otherListLabel).toBeFalsy()
          }
        }
      }),
      { numRuns: 100 }
    )
  }, 30000)

  /**
   * Property: Changing a parent selection should reset all child selections
   * This ensures referential integrity is maintained
   * **Validates: Requirements 9.2, 9.3, 9.4**
   */
  it('should reset child selections when parent selection changes', async () => {
    await fc.assert(
      fc.asyncProperty(
        hierarchyDataArb.filter(h => 
          h.workspaces.length >= 2 && 
          h.workspaces.some(w => w.spaces.length >= 1)
        ),
        async (hierarchyData) => {
          cleanup()
          mockFetchWithHierarchy(hierarchyData)

          render(
            <BrowserRouter>
              <AuthProvider>
                <ToastProvider>
                  <BuscarTasks />
                </ToastProvider>
              </AuthProvider>
            </BrowserRouter>
          )

          await waitFor(() => {
            expect(screen.queryByText('Carregando dados...')).not.toBeInTheDocument()
          }, { timeout: 3000 })

          const workspaceSelector = screen.getByTestId('workspace-selector') as HTMLSelectElement
          const spaceSelector = screen.getByTestId('space-selector') as HTMLSelectElement
          const folderSelector = screen.getByTestId('folder-selector') as HTMLSelectElement

          // Find a workspace with spaces
          const workspaceWithSpaces = hierarchyData.workspaces.find(w => w.spaces.length > 0)
          if (!workspaceWithSpaces) return

          // Select the first workspace
          fireEvent.change(workspaceSelector, { target: { value: workspaceWithSpaces.id } })

          // Select a space if available
          if (workspaceWithSpaces.spaces.length > 0) {
            const firstSpace = workspaceWithSpaces.spaces[0]
            fireEvent.change(spaceSelector, { target: { value: firstSpace.id } })

            // Select a folder if available
            if (firstSpace.folders.length > 0) {
              const firstFolder = firstSpace.folders[0]
              fireEvent.change(folderSelector, { target: { value: firstFolder.id } })

              // Now change the workspace
              const otherWorkspace = hierarchyData.workspaces.find(w => w.id !== workspaceWithSpaces.id)
              if (otherWorkspace) {
                fireEvent.change(workspaceSelector, { target: { value: otherWorkspace.id } })

                // Property: Space selector should be reset
                expect(spaceSelector.value).toBe('')

                // Property: Folder selector should be reset
                expect(folderSelector.value).toBe('')
              }
            }
          }
        }
      ),
      { numRuns: 100 }
    )
  })

  /**
   * Property: Child selectors should be disabled when their parent is not selected
   * **Validates: Requirements 9.2, 9.3, 9.4**
   */
  it('should disable child selectors when parent is not selected', async () => {
    // Use data with at least one workspace that has spaces with folders
    const hierarchyWithDepthArb = hierarchyDataArb.filter(h =>
      h.workspaces.some(w =>
        w.spaces.some(s => s.folders.length > 0)
      )
    )

    await fc.assert(
      fc.asyncProperty(hierarchyWithDepthArb, async (hierarchyData) => {
        cleanup()
        mockFetchWithHierarchy(hierarchyData)

        render(
          <BrowserRouter>
            <AuthProvider>
              <ToastProvider>
                <BuscarTasks />
              </ToastProvider>
            </AuthProvider>
          </BrowserRouter>
        )

        await waitFor(() => {
          expect(screen.queryByText('Carregando dados...')).not.toBeInTheDocument()
        }, { timeout: 3000 })

        const workspaceSelector = screen.getByTestId('workspace-selector') as HTMLSelectElement
        const spaceSelector = screen.getByTestId('space-selector') as HTMLSelectElement
        const folderSelector = screen.getByTestId('folder-selector') as HTMLSelectElement

        // Property: Initially, space selector should be disabled (no workspace selected)
        expect(spaceSelector.disabled).toBe(true)

        // Property: Initially, folder selector should be disabled (no space selected)
        expect(folderSelector.disabled).toBe(true)

        // Find a workspace with spaces that have folders
        const workspaceWithDepth = hierarchyData.workspaces.find(w =>
          w.spaces.some(s => s.folders.length > 0)
        )
        if (!workspaceWithDepth) return

        // Select the workspace
        fireEvent.change(workspaceSelector, { target: { value: workspaceWithDepth.id } })

        // Property: Space selector should now be enabled (workspace is selected)
        expect(spaceSelector.disabled).toBe(false)

        // Property: Folder selector should still be disabled (no space selected yet)
        expect(folderSelector.disabled).toBe(true)

        // Select a space with folders
        const spaceWithFolders = workspaceWithDepth.spaces.find(s => s.folders.length > 0)
        if (!spaceWithFolders) return

        fireEvent.change(spaceSelector, { target: { value: spaceWithFolders.id } })

        // Property: Folder selector should now be enabled (space is selected)
        expect(folderSelector.disabled).toBe(false)
      }),
      { numRuns: 100 }
    )
  })
})
