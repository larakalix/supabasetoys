import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import App from './App.vue'
import ProjectOverview from './components/ProjectOverview.vue'
import PortsDialog from './components/PortsDialog.vue'
import ConnectionDialog from './components/ConnectionDialog.vue'
import type { Project, Registry, Status } from './types'

const mocks = vi.hoisted(() => ({ request: vi.fn(), desktop: vi.fn(() => true), listen: vi.fn(async () => () => {}), save: vi.fn() }))
vi.mock('./api', () => ({ request: mocks.request, isDesktop: mocks.desktop, cancelOperation: vi.fn(), listenProgress: mocks.listen, saveDiagnosticsPath: mocks.save, pickFolder: vi.fn(), copyText: vi.fn() }))
const project: Project = { id: 'A', name: 'Project A', project_id: 'local-A', path: '/tmp/project-a', adapter: { kind: 'standard' } }
const status: Status = { project, state: 'running', services: [{ id: 'container-a', name: 'supabase_db_local-A', project_id: 'local-A', workdir: null, state: 'running', health: 'healthy', ports: [55000] }], diagnostics: [], configured_ports: { 'db.port': 55000, 'db.shadow_port': 55002 }, endpoints: { STUDIO_URL: 'http://localhost:55001' }, resources: [], resources_error: null, supabase_version: '2.119.0' }
const registry: Registry = { projects: [project], settings: { supabase_cli: 'supabase', docker_cli: 'docker', docker_endpoint: null } }
const mounted: ReturnType<typeof mount>[] = []
function render(component: Parameters<typeof mount>[0], options = {}) { const wrapper = mount(component, options); mounted.push(wrapper); return wrapper }
beforeEach(() => {
  vi.clearAllMocks(); mocks.desktop.mockReturnValue(true)
  HTMLDialogElement.prototype.showModal = vi.fn()
  mocks.request.mockImplementation(async action => {
    if (action === 'registry') return registry
    if (action === 'inventory') return { services: [], available: true }
    if (action === 'status' || action === 'lifecycle' || action === 'repair_apply') return status
    if (action === 'ports_preview') return { project: 'A', mode: 'manual', config_hash: 'hash', restart_required: true, changes: [{ key: 'db.port', before: 55000, after: 55100 }] }
    if (action === 'repair_preview') return { project: 'A', config_hash: 'hash', restart_required: true, changes: [{ key: 'db.port', before: 54322, after: 20000 }] }
    if (action === 'connections') return { API_URL: 'http://localhost:55000', ANON_KEY: 'public-test-key', SERVICE_ROLE_KEY: 'secret-test-key' }
    if (action === 'env_preview') return { project: 'A', file: '.env.local', source_hash: 'hash', updates: { SUPABASE_URL: 'http://localhost:55000', SUPABASE_ANON_KEY: 'public-test-key' } }
    return { ok: true }
  })
})
afterEach(() => { mounted.splice(0).forEach(w => w.unmount()) })
describe('desktop workflows', () => {
  it('loads real backend state and targets the selected project for stop', async () => {
    const wrapper = render(App); await flushPromises()
    expect(wrapper.text()).toContain('Project A')
    await wrapper.findAll('button').find(b => b.text() === 'Stop')!.trigger('click'); await flushPromises()
    expect(mocks.request).toHaveBeenCalledWith('lifecycle', { project: 'A', operation: 'stop' })
  })
  it('requires review before applying a repair and makes restart explicit', async () => {
    const wrapper = render(App); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'diagnostics')!.trigger('click')
    await wrapper.findAll('button').find(b => b.text() === 'Preview port repair')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('54322 → 20000')
    expect(mocks.request.mock.calls.some(c => c[0] === 'repair_apply')).toBe(false)
    await wrapper.findAll('button').find(b => b.text() === 'Apply & restart project')!.trigger('click'); await flushPromises()
    expect(mocks.request).toHaveBeenCalledWith('repair_apply', expect.objectContaining({ restart: true }))
  })
  it('shows configured ports while stopped and opens the port editor', async () => {
    const wrapper = render(ProjectOverview, { props: { status: { ...status, state: 'stopped', services: [], endpoints: {} }, busy: false } })
    expect(wrapper.text()).toContain('db.port'); expect(wrapper.text()).toContain('55000')
    await wrapper.findAll('button').find(b => b.text() === 'Change ports')!.trigger('click')
    expect(wrapper.emitted('ports')).toHaveLength(1)
  })
  it('reviews explicit port changes and restarts only after approval', async () => {
    const wrapper = render(PortsDialog, { props: { status } })
    await wrapper.find('input[aria-label="db.port"]').setValue('55100')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(mocks.request).toHaveBeenCalledWith('ports_preview', { project: 'A', ports: { 'db.port': 55100 } })
    expect(wrapper.text()).toContain('55000 → 55100')
    expect(mocks.request.mock.calls.some(c => c[0] === 'repair_apply')).toBe(false)
    await wrapper.findAll('button').find(b => b.text() === 'Apply & restart project')!.trigger('click'); await flushPromises()
    expect(mocks.request).toHaveBeenCalledWith('repair_apply', expect.objectContaining({ restart: true, preview: expect.objectContaining({ mode: 'manual', project: 'A' }) }))
    expect(wrapper.emitted('applied')).toHaveLength(1)
  })
  it('shows port conflicts and never writes a rejected preview', async () => {
    mocks.request.mockRejectedValueOnce(new Error('Port 55100 is occupied'))
    const wrapper = render(PortsDialog, { props: { status } })
    await wrapper.find('input[aria-label="db.port"]').setValue('55100')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('occupied')
    expect(mocks.request.mock.calls.some(c => c[0] === 'repair_apply')).toBe(false)
    expect(wrapper.find('input').exists()).toBe(true)
  })
  it('requires a fresh preview after an apply failure', async () => {
    const original = mocks.request.getMockImplementation()!
    mocks.request.mockImplementation(async (...args) => { if (args[0] === 'repair_apply') throw new Error('configuration changed since preview'); return original(...args) })
    const wrapper = render(PortsDialog, { props: { status } })
    await wrapper.find('input[aria-label="db.port"]').setValue('55100')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'Apply & restart project')!.trigger('click'); await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('configuration changed')
    expect(wrapper.emitted('applied')).toBeUndefined()
    expect(wrapper.find('form').exists()).toBe(true)
  })
  it('shows unavailable resource measurements as unknown', () => {
    const wrapper = render(ProjectOverview, { props: { status, busy: false } })
    expect(wrapper.text()).toContain('No reading available'); expect(wrapper.text()).not.toContain('0.0%')
  })
  it('keeps keys hidden and previews an environment export before writing', async () => {
    const wrapper = render(ConnectionDialog, { props: { project: 'A' } }); await flushPromises()
    expect(wrapper.text()).not.toContain('secret-test-key')
    await wrapper.findAll('button').find(b => b.text() === 'Preview changes')!.trigger('click'); await flushPromises()
    expect(mocks.request.mock.calls.some(c => c[0] === 'env_apply')).toBe(false)
    await wrapper.findAll('button').find(b => b.text() === 'Write these changes')!.trigger('click'); await flushPromises()
    expect(mocks.request).toHaveBeenCalledWith('env_apply', expect.objectContaining({ mapping: { API_URL: 'SUPABASE_URL', ANON_KEY: 'SUPABASE_ANON_KEY' } }))
  })
  it('keeps backend errors visible without reporting success', async () => {
    const original = mocks.request.getMockImplementation()!
    mocks.request.mockImplementation(async (...args) => { if (args[0] === 'lifecycle') throw new Error('Duplicate project identity'); return original(...args) })
    const wrapper = render(App); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'Stop')!.trigger('click'); await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('Duplicate project identity')
    expect(wrapper.text()).not.toContain('Project stopped.')
  })
  it('makes browser preview state explicit and disables native actions', async () => {
    mocks.desktop.mockReturnValue(false)
    const wrapper = render(App); await flushPromises()
    expect(wrapper.text()).toContain('Interface preview')
    expect(wrapper.findAll('button').find(b => b.text().includes('Add your first project'))!.attributes()).toHaveProperty('disabled')
    expect(mocks.request).not.toHaveBeenCalled()
  })
})
