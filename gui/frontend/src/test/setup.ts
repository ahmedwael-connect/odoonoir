import '@testing-library/jest-dom'
import { vi } from 'vitest'

// Mock Wails runtime
vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => () => {}), Emit: vi.fn() },
}))

// Mock bindings
vi.mock('../../bindings/github.com/ahmed/odoonoir/gui/app', () => ({
  Instances: vi.fn(() => Promise.resolve([])),
  Status: vi.fn(() => Promise.resolve({ Instance: { name: 'test', version: '18.0', status: 'stopped', port: 8069, dbName: 'test', dbs: 1, path: '/tmp/test', adopted: false }, Serving: '', Conf: '', Log: '' })),
  Databases: vi.fn(() => Promise.resolve([{ name: 'test', sizeBytes: 1234, owner: 'odoo', initialized: true, primary: true }])),
  Start: vi.fn(() => Promise.resolve()),
  Stop: vi.fn(() => Promise.resolve()),
  Restart: vi.fn(() => Promise.resolve()),
  Create: vi.fn(() => Promise.resolve({ name: 'test', database: 'test', port: 8069 })),
  SystemCheck: vi.fn(() => Promise.resolve([{ check: 'python-compat', severity: 'ok', found: '3.11' }])),
  CheckVersion: vi.fn(() => Promise.resolve([{ check: 'python-compat', severity: 'ok' }])),
  ScaffoldModule: vi.fn(() => Promise.resolve()),
  ListAddonPaths: vi.fn(() => Promise.resolve([{ path: '/a', enabled: true, exists: true, position: 1 }])),
  AddAddonPath: vi.fn((n,p)=> Promise.resolve([{ path: p, enabled: true, exists: true, position: 1 }])),
  RemoveAddonPath: vi.fn(() => Promise.resolve([])),
  ToggleAddonPath: vi.fn(() => Promise.resolve([])),
  MoveAddonPath: vi.fn(() => Promise.resolve([])),
  DetectEnterprise: vi.fn(() => Promise.resolve({ isEnterprise: false, path: '', repo: '', branch: '', addonsPath: [] })),
  LoadEnterprise: vi.fn(() => Promise.resolve({})),
  UnloadEnterprise: vi.fn(() => Promise.resolve({})),
  OpenInVSCode: vi.fn(() => Promise.resolve('code /tmp/test')),
  SudoAptInstall: vi.fn(() => Promise.resolve('ok')),
  GetGitHubToken: vi.fn(() => Promise.resolve('')),
  SetGitHubToken: vi.fn(() => Promise.resolve()),
  ValidateGitHubToken: vi.fn(() => Promise.resolve({ login: 'test' })),
  ClearGitHubToken: vi.fn(() => Promise.resolve()),
  SearchMarketplaceModules: vi.fn(() => Promise.resolve([])),
  GetMarketplaceStats: vi.fn(() => Promise.resolve({ totalModules: 0 })),
  ListGitHubBranches: vi.fn(() => Promise.resolve(['18.0', '16.0'])),
  PublishStandardModule: vi.fn(() => Promise.resolve({ repoUrl: 'https://github.com/test/repo', branch: '18.0' })),
  IndexMarketplaceModule: vi.fn(() => Promise.resolve({})),
  ModuleDepGraph: vi.fn(() => Promise.resolve({ nodes: [], links: [] })),
  BrowseRecords: vi.fn(() => Promise.resolve({ records: [], fields: [], totalCount: 0 })),
  ModelInfo: vi.fn(() => Promise.resolve({ fields: [], access: [], views: [], actions: [] })),
  CronList: vi.fn(() => Promise.resolve([])),
  GetBackupScheduleStatus: vi.fn(() => Promise.resolve({ enabled: false })),
  ListScheduledBackups: vi.fn(() => Promise.resolve([])),
  SetBackupSchedule: vi.fn(() => Promise.resolve()),
  RunBackupNow: vi.fn(() => Promise.resolve()),
  GetDashboardMetrics: vi.fn(() => Promise.resolve({ totalInstances: 0 })),
  ValidateDBConfig: vi.fn(() => Promise.resolve([])),
  ShellURL: vi.fn(() => Promise.resolve('ws://localhost:8073/shell')),
}))

vi.mock('../../../bindings/github.com/ahmed/odoonoir/gui/app', () => ({
  Instances: vi.fn(() => Promise.resolve([])),
  Status: vi.fn(() => Promise.resolve({ Instance: { name: 'test', version: '18.0', status: 'stopped', port: 8069, dbName: 'test', dbs: 1, path: '/tmp/test', adopted: false }, Serving: '', Conf: '', Log: '' })),
  Databases: vi.fn(() => Promise.resolve([{ name: 'test', sizeBytes: 1234, owner: 'odoo', initialized: true, primary: true }])),
}))

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation(query => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})
