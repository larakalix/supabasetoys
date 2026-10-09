import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountSidebar from './components/AccountSidebar.vue'
import AssociationDialog from './components/AssociationDialog.vue'
import CloudProjectPanel from './components/CloudProjectPanel.vue'
import type { AccountProfile, CloudInventory, Registry } from './types'
const mocks = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('./api', () => ({ request: mocks.request }))
const refA = 'aaaaaaaaaaaaaaaaaaaa'
const inventory: CloudInventory = { account_id: 'personal', organizations: [{ id: 'org', slug: 'org', name: 'Organization' }], projects: [{ ref: refA, organization_id: 'org', name: 'Hosted A', region: 'eu-west-1', status: 'ACTIVE_HEALTHY' }], updated_at: '2026-10-05T00:00:00Z', stale: false, access_state: 'ready', message: 'Only accessible resources are shown' }
const personal: AccountProfile = { id: 'personal', label: 'Personal', session_only: false, inventory }
const work: AccountProfile = { id: 'work', label: 'Work', session_only: false, inventory: { ...inventory, account_id: 'work' } }
const registry: Registry = { settings: { supabase_cli: 'supabase', docker_cli: 'docker', docker_endpoint: null }, accounts: [personal, work], projects: [{ id: 'local', name: 'Local A', path: '/local/a', project_id: 'local-a', adapter: { kind: 'standard' } }], associations: [{ local_project_id: 'local', cloud_ref: refA }] }
const wrappers: ReturnType<typeof mount>[] = []
function render(component: Parameters<typeof mount>[0], props: Record<string, unknown>) { const w = mount(component, { props }); wrappers.push(w); return w }
beforeEach(() => { vi.clearAllMocks(); HTMLDialogElement.prototype.showModal = vi.fn(); mocks.request.mockResolvedValue({ ok: true }) })
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()) })
it('groups a shared local environment beneath both accessible profiles', () => {
 const w = render(AccountSidebar, { registry, selectedId: 'local', busy: false, desktop: true, inventory: null, status: null })
 expect(w.findAll('.account-group')).toHaveLength(2)
 expect(w.findAll('.nested-local')).toHaveLength(2)
 expect(w.findAll('.account-group').every(a => a.text().includes('Local A'))).toBe(true)
})
it('keeps local projects accessible after accounts disappear', () => {
 const w = render(AccountSidebar, { registry: { ...registry, accounts: [] }, selectedId: '', busy: false, desktop: true, inventory: null, status: null })
 expect(w.text()).toContain('Local A'); expect(w.text()).toContain('association currently inaccessible')
})
it('masks and clears token input and allows an explicit session-only connection', async () => {
 const w = render(AccountSidebar, { registry, selectedId: '', busy: false, desktop: true, inventory: null, status: null })
 await w.findAll('button').find(b => b.text() === 'Connect account')!.trigger('click')
 await w.find('input[placeholder="Personal or Work"]').setValue('Session')
 const token = w.find('input[type="password"]'); await token.setValue('sbp_private')
 await w.find('input[type="checkbox"]').setValue(true)
 expect(w.text()).not.toContain('sbp_private')
 await w.find('form').trigger('submit'); await flushPromises()
 expect(mocks.request).toHaveBeenCalledWith('account_add', { name: 'Session', token: 'sbp_private', session_only: true })
 expect(w.find('input[type="password"]').exists()).toBe(false); expect(w.emitted('changed')).toHaveLength(1)
})
it('offers session-only explicitly after credential-store failure', async () => {
 mocks.request.mockRejectedValueOnce(new Error('Secure credential storage unavailable; choose session-only'))
 const w = render(AccountSidebar, { registry, selectedId: '', busy: false, desktop: true, inventory: null, status: null })
 await w.findAll('button').find(b => b.text() === 'Connect account')!.trigger('click')
 await w.find('input[type="password"]').setValue('sbp_private')
 await w.find('form').trigger('submit'); await flushPromises()
 expect(w.find('[role="alert"]').text()).toContain('session-only')
 expect((w.find('input[type="password"]').element as HTMLInputElement).value).toBe('')
 expect(w.emitted('changed')).toBeUndefined()
})
it('requires confirmation before disconnect and sends no local lifecycle action', async () => {
 const w = render(AccountSidebar, { registry, selectedId: '', busy: false, desktop: true, inventory: null, status: null })
 await w.find('[aria-label="Disconnect Personal"]').trigger('click')
 expect(mocks.request).not.toHaveBeenCalled()
 await w.findAll('button').find(b => b.text() === 'Disconnect account')!.trigger('click'); await flushPromises()
 expect(mocks.request).toHaveBeenCalledExactlyOnceWith('account_remove', { account: 'personal' })
})
it('confirms exact linked reference suggestions and leaves ambiguous account choice explicit', async () => {
 mocks.request.mockResolvedValueOnce([{ local_project_id: 'local', cloud_ref: refA, account_ids: ['personal', 'work'] }])
 const w = render(AssociationDialog, { registry, local: 'local' }); await flushPromises()
 expect(w.text()).toContain('Existing Supabase link')
 await w.findAll('button').find(b => b.text() === 'Use suggestion')!.trigger('click')
 expect(mocks.request.mock.calls.some(c => c[0] === 'associate')).toBe(false)
 expect((w.findAll('select')[1]!.element as HTMLSelectElement).value).toBe('')
 await w.findAll('select')[1]!.setValue('personal')
 await w.findAll('select')[2]!.setValue(refA)
 await w.find('form').trigger('submit'); await flushPromises()
 expect(mocks.request).toHaveBeenCalledWith('associate', { project: 'local', account: 'personal', cloud_ref: refA })
 expect(w.emitted('saved')).toHaveLength(1)
})
it('labels cloud state stale and exposes dashboard instead of local lifecycle controls', () => {
 const w = render(CloudProjectPanel, { selection: { account: { ...personal, inventory: { ...inventory, stale: true } }, project: inventory.projects[0] }, registry })
 expect(w.text()).toContain('Stale inventory'); expect(w.text()).toContain('eu-west-1')
 expect(w.findAll('button').some(b => ['Start project', 'Stop', 'Restart'].includes(b.text()))).toBe(false)
})
it('ignores an old profile refresh response after account selection changes', async () => {
 let finish!: (result: CloudInventory) => void
 mocks.request.mockImplementation(() => new Promise<CloudInventory>(resolve => { finish = resolve }))
 const w = mount(CloudProjectPanel, { props: { selection: { account: personal, project: inventory.projects[0]! }, registry } }); wrappers.push(w)
 await w.findAll('button').find(b => b.text() === 'Refresh cloud inventory')!.trigger('click')
 await w.setProps({ selection: { account: work, project: inventory.projects[0]! } })
 finish({ ...inventory, projects: [{ ...inventory.projects[0]!, name: 'Old personal result' }] }); await flushPromises()
 expect(w.text()).toContain('Work'); expect(w.text()).not.toContain('Old personal result')
 expect(w.emitted('changed')).toBeUndefined()
})
it('preserves a visible cached project and explains access failure on refresh', async () => {
 mocks.request.mockResolvedValue({ ...inventory, stale: true, access_state: 'permission_denied', message: 'Read permission missing' })
 const w = render(CloudProjectPanel, { selection: { account: personal, project: inventory.projects[0] }, registry })
 await w.findAll('button').find(b => b.text() === 'Refresh cloud inventory')!.trigger('click'); await flushPromises()
 expect(w.text()).toContain('Hosted A'); expect(w.text()).toContain('Read permission missing'); expect(w.text()).toContain('Stale inventory')
})
