import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Button } from './Button'

describe('Button', () => {
  it('renders primary variant', () => {
    render(<Button variant="primary">Click</Button>)
    expect(screen.getByText('Click')).toBeInTheDocument()
  })
  it('shows loading spinner', () => {
    render(<Button loading>Load</Button>)
    expect(document.querySelector('.animate-spin')).toBeTruthy()
  })
  it('disables when loading', () => {
    render(<Button loading>Load</Button>)
    expect(screen.getAllByText('Load')[0].closest('button')?.disabled).toBe(true)
  })
})
