import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ToastProvider } from '../App'
import { CommandPalette } from '../CommandPalette'
import { DeployScreen } from '../screens/DeployScreen'

describe('CommandPalette', () => {
  const items = [
    { id: 'inst:myapp', label: 'myapp', category: 'instance' as const, hint: 'v18.0 port 8069', icon: '■', action: vi.fn() },
    { id: 'screen:overview', label: 'Overview', category: 'screen' as const, icon: '📊', action: vi.fn() },
    { id: 'act:start', label: 'Start myapp', category: 'action' as const, icon: '▶', action: vi.fn() },
  ]

  it('renders when open', () => {
    render(<CommandPalette open={true} onClose={vi.fn()} items={items} />)
    expect(screen.getByPlaceholderText(/Search instances/)).toBeInTheDocument()
  })

  it('does not render when closed', () => {
    const { container } = render(<CommandPalette open={false} onClose={vi.fn()} items={items} />)
    expect(container.innerHTML).toBe('')
  })

  it('filters items by search', () => {
    render(<CommandPalette open={true} onClose={vi.fn()} items={items} />)
    const input = screen.getByPlaceholderText(/Search instances/)
    fireEvent.change(input, { target: { value: 'overview' } })
    expect(screen.getByText('Overview')).toBeInTheDocument()
  })

  it('calls action and closes on Enter', () => {
    const action = vi.fn()
    const onClose = vi.fn()
    const singleItem = [{ id: 'test', label: 'TestItem', category: 'action' as const, icon: '⚡', action }]
    render(<CommandPalette open={true} onClose={onClose} items={singleItem} />)
    const input = screen.getByPlaceholderText(/Search instances/)
    fireEvent.change(input, { target: { value: 'test' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(action).toHaveBeenCalled()
    expect(onClose).toHaveBeenCalled()
  })

  it('closes on Escape', () => {
    const onClose = vi.fn()
    render(<CommandPalette open={true} onClose={onClose} items={items} />)
    const input = screen.getByPlaceholderText(/Search instances/)
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(onClose).toHaveBeenCalled()
  })

  it('navigates with arrow keys without crash', () => {
    render(<CommandPalette open={true} onClose={vi.fn()} items={items} />)
    const input = screen.getByPlaceholderText(/Search instances/)
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.keyDown(input, { key: 'ArrowUp' })
  })

  it('shows empty state when no matches', () => {
    render(<CommandPalette open={true} onClose={vi.fn()} items={items} />)
    const input = screen.getByPlaceholderText(/Search instances/)
    fireEvent.change(input, { target: { value: 'zzznonexistent' } })
    expect(screen.getByText('No results')).toBeInTheDocument()
  })

  it('groups items by category', () => {
    render(<CommandPalette open={true} onClose={vi.fn()} items={items} />)
    expect(screen.getByText('Instances')).toBeInTheDocument()
    expect(screen.getByText('Screens')).toBeInTheDocument()
    expect(screen.getByText('Actions')).toBeInTheDocument()
  })
})

describe('DeployScreen', () => {
  it('renders deploy heading', () => {
    render(<ToastProvider><DeployScreen name="myapp" /></ToastProvider>)
    expect(screen.getByText(/Deploy — myapp/)).toBeInTheDocument()
  })

  it('shows checklist', () => {
    render(<ToastProvider><DeployScreen name="myapp" /></ToastProvider>)
    expect(screen.getByText(/Production deployment checklist/)).toBeInTheDocument()
  })

  it('has interactive checkboxes', () => {
    render(<ToastProvider><DeployScreen name="myapp" /></ToastProvider>)
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes.length).toBeGreaterThan(0)
    fireEvent.click(checkboxes[0])
  })
})
