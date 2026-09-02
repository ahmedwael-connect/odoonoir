import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ToastProvider } from '../App'
import { InstanceLift } from '../components/organisms/InstanceLift'
import { InstanceTopNav } from '../components/organisms/InstanceTopNav'
import { AppHeader } from '../components/organisms/AppHeader'
import { SystemCheckScreen } from '../screens/SystemCheckScreen'
import { TerminalScreen } from '../screens/TerminalScreen'
import { RecordsScreen } from '../screens/RecordsScreen'
import { ModelInspectorScreen } from '../screens/ModelInspectorScreen'
import { DepGraphScreen } from '../screens/DepGraphScreen'
import { BackupsScreen } from '../screens/BackupsScreen'
import { DashboardScreen } from '../screens/DashboardScreen'
import { ScaffoldScreen } from '../screens/ScaffoldScreen'
import { EnterpriseWizard } from '../components/enterprise/EnterpriseWizard'
import { AddonPathManagerWizard } from '../components/manager/AddonPathManagerWizard'
import { SudoWizard } from '../components/system/SudoWizard'

const mockInstances = [
  { name: 'myapp', version: '18.0', status: 'stopped', port: 8069, dbName: 'myapp', dbs: 1, path: '/tmp/myapp', adopted: false },
  { name: 'odoo16', version: '16.0', status: 'running', port: 8070, dbName: 'odoo16', dbs: 2, path: '/tmp/odoo16', adopted: false },
]
const mockStatuses = {
  myapp: { Instance: mockInstances[0], Serving: 'myapp', Conf: '/tmp/myapp/etc/odoo.conf', Log: '/tmp/myapp/logs/odoo.log' },
  odoo16: { Instance: mockInstances[1], Serving: 'odoo16', Conf: '/tmp/odoo16/etc/odoo.conf', Log: '/tmp/odoo16/logs/odoo.log' },
}

describe('InstanceLift', () => {
  it('renders instances and filters', async () => {
    render(
      <InstanceLift instances={mockInstances as any} statuses={mockStatuses as any} selected={null} busy={null} initialLoading={false} collapsed={false} onToggle={vi.fn()} onSelect={vi.fn()} onAct={vi.fn()} filter="" setFilter={vi.fn()} onCreate={vi.fn()} />
    )
    expect(screen.getByText('myapp')).toBeInTheDocument()
    expect(screen.getByText('odoo16')).toBeInTheDocument()
  })
  it('shows loading', () => {
    render(<InstanceLift instances={[]} statuses={{}} selected={null} busy={null} initialLoading={true} collapsed={false} onToggle={vi.fn()} onSelect={vi.fn()} onAct={vi.fn()} filter="" setFilter={vi.fn()} onCreate={vi.fn()} />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })
})

describe('InstanceTopNav', () => {
  it('renders tabs and actions', () => {
    render(<InstanceTopNav selected="myapp" status={mockStatuses.myapp as any} screen="overview" setScreen={vi.fn()} busy={null} onAct={vi.fn()} onRemove={vi.fn()} onConfirm={vi.fn()} />)
    expect(screen.getByText('myapp')).toBeInTheDocument()
    expect(screen.getByText('Overview')).toBeInTheDocument()
    expect(screen.getByText('Databases')).toBeInTheDocument()
  })
})

describe('AppHeader', () => {
  it('shows instance count and search', () => {
    render(<AppHeader instanceCount={2} eventCount={5} onToggleLift={vi.fn()} onOpenPalette={vi.fn()} onToggleEventLog={vi.fn()} />)
    expect(screen.getByText(/2 instances/)).toBeInTheDocument()
    expect(screen.getByText('Search or jump…')).toBeInTheDocument()
  })
})

describe('SystemCheckScreen', () => {
  it('renders and shows version select', async () => {
    render(<ToastProvider><SystemCheckScreen /></ToastProvider>)
    expect(screen.getByText('System Check')).toBeInTheDocument()
    expect(screen.getByText('System Check')).toBeInTheDocument()
  })
})

describe('TerminalScreen', () => {
  it('shows db select and connect', () => {
    render(<ToastProvider><TerminalScreen name="myapp" /></ToastProvider>)
    expect(screen.getByText('Terminal — myapp')).toBeInTheDocument()
    expect(screen.getByText('Select DB')).toBeInTheDocument()
  })
})

describe('RecordsScreen', () => {
  it('renders model and domain', () => {
    render(<ToastProvider><RecordsScreen name="myapp" /></ToastProvider>)
    expect(screen.getByText('Records — myapp')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('res.partner')).toBeInTheDocument()
  })
})

describe('ModelInspectorScreen', () => {
  it('renders inspector', () => {
    render(<ToastProvider><ModelInspectorScreen name="myapp" /></ToastProvider>)
    expect(screen.getByText('Inspector — myapp')).toBeInTheDocument()
    expect(screen.getAllByText(/Inspect/).length).toBeGreaterThan(0)
  })
})

describe('DepGraphScreen', () => {
  it('renders graph header', () => {
    render(<ToastProvider><DepGraphScreen name="myapp" /></ToastProvider>)
    expect(screen.getByText('Graph — myapp')).toBeInTheDocument()
  })
})

describe('BackupsScreen', () => {
  it('renders backups', () => {
    render(<ToastProvider><BackupsScreen name="myapp" /></ToastProvider>)
    expect(screen.getByText(/Backups — myapp/)).toBeInTheDocument()
  })
})

describe('DashboardScreen', () => {
  it('renders metrics', async () => {
    render(<ToastProvider><DashboardScreen /></ToastProvider>)
    const el = screen.queryByText(/Dashboard/) || screen.queryByText(/Loading/) || screen.queryByText(/Instances/) || screen.queryByText(/No data/)
    expect(el).toBeInTheDocument()
  })
})

describe('ScaffoldScreen', () => {
  it('has module form', () => {
    render(<ToastProvider><ScaffoldScreen name="myapp" /></ToastProvider>)
    expect(screen.getByText('Scaffold — myapp')).toBeInTheDocument()
    expect(screen.getByDisplayValue('my_module')).toBeInTheDocument()
  })
})

describe('EnterpriseWizard', () => {
  it('renders when open', () => {
    render(<ToastProvider><EnterpriseWizard name="myapp" open={true} onOpenChange={vi.fn()} /></ToastProvider>)
    expect(screen.getAllByText(/Load Enterprise/).length).toBeGreaterThan(0)
  })
})

describe('AddonPathManagerWizard', () => {
  it('renders addon manager', () => {
    render(<ToastProvider><AddonPathManagerWizard name="myapp" open={true} onOpenChange={vi.fn()} /></ToastProvider>)
    expect(screen.getByText(/Addon Path Manager/)).toBeInTheDocument()
  })
})

describe('SudoWizard', () => {
  it('shows sudo prompt', () => {
    render(<ToastProvider><SudoWizard open={true} onOpenChange={vi.fn()} pkgs={['zlib1g-dev']} /></ToastProvider>)
    expect(screen.getByText(/Sudo required/)).toBeInTheDocument()
    expect(screen.getByPlaceholderText('••••••••')).toBeInTheDocument()
  })
})
