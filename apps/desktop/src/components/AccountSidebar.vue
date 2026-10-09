<script setup lang="ts">
import { computed, ref } from 'vue'
import { request } from '../api'
import type { Registry, AccountProfile, CloudSelection, Project, Inventory, Status } from '../types'
import Modal from './Modal.vue'
const props = defineProps<{ registry: Registry; selectedId: string; busy: boolean; desktop: boolean; inventory: Inventory | null; status: Status | null }>()
const emit = defineEmits<{ select: [project: Project]; cloud: [selection: CloudSelection]; changed: [] }>()
const profiles = computed(() => props.registry.accounts ?? [])
const associations = computed(() => props.registry.associations ?? [])
const visibleRefs = computed(() => new Set(profiles.value.flatMap(a => a.inventory.projects.map(p => p.ref))))
const ungrouped = computed(() => props.registry.projects.filter(p => !associations.value.some(a => a.local_project_id === p.id && visibleRefs.value.has(a.cloud_ref))))
const connect = ref(false), pending = ref(false), label = ref(''), token = ref(''), sessionOnly = ref(false), error = ref('')
const disconnect = ref<AccountProfile | null>(null)
function locals(ref: string) { return props.registry.projects.filter(p => associations.value.some(a => a.local_project_id === p.id && a.cloud_ref === ref)) }
function localDescription(project: Project) {
 if (props.status?.project.id === project.id) return `Local · ${props.status.state}${props.status.configured_ports['api.port'] ? ` · API :${props.status.configured_ports['api.port']}` : ''}`
 if (!props.inventory?.available) return 'Local · Status unavailable'
 const id = project.adapter.kind === 'stack' ? project.adapter.stack_id : project.project_id
 const services = props.inventory.services.filter(s => s.project_id === id)
 const running = services.some(s => s.state === 'running')
 const api = services.find(s => s.name.includes('kong'))?.ports[0]
 return `Local · ${running ? services.some(s => s.health === 'unhealthy') ? 'Unhealthy' : 'Running' : 'Stopped'}${api ? ` · API :${api}` : ''}`
}
async function refreshAccount(account: AccountProfile) {
 pending.value = true; error.value = ''
 try { await request('cloud_list', { account: account.id, refresh: true }); emit('changed') }
 catch(e) { error.value = e instanceof Error ? e.message : String(e) }
 finally { pending.value = false }
}
async function tokensPage() { try { await request('open_token_settings') } catch(e) { error.value = String(e) } }
async function add() {
 pending.value = true; error.value = ''
 try { await request('account_add', { name: label.value, token: token.value, session_only: sessionOnly.value }); connect.value = false; label.value = ''; emit('changed') }
 catch (e) { error.value = e instanceof Error ? e.message : String(e) }
 finally { token.value = ''; pending.value = false }
}
async function remove() {
 if (!disconnect.value) return
 pending.value = true; error.value = ''
 try { await request('account_remove', { account: disconnect.value.id }); disconnect.value = null; emit('changed') }
 catch(e) { error.value = e instanceof Error ? e.message : String(e) }
 finally { pending.value = false }
}
</script>
<template>
 <nav aria-label="Accounts and projects" class="account-tree">
  <details v-for="account in profiles" :key="account.id" class="account-group" open>
   <summary class="account-heading"><strong>{{ account.label }}</strong><button class="subtle" :disabled="pending || busy" :aria-label="`Disconnect ${account.label}`" @click.stop.prevent="disconnect = account; error = ''">×</button></summary>
   <button class="subtle" :disabled="pending || busy" :aria-label="`Refresh ${account.label}`" @click="refreshAccount(account)">Refresh inventory</button>
   <small>{{ account.session_only ? 'Session only' : 'Connected profile' }} · {{ account.inventory.stale ? 'Stale inventory' : 'Cached inventory' }}</small>
   <p v-if="account.inventory.access_state !== 'ready'" class="warning-text">{{ account.inventory.message }}</p>
   <div v-for="org in account.inventory.organizations" :key="org.id" class="organization-group"><strong>{{ org.name }}</strong>
    <div v-for="cloud in account.inventory.projects.filter(p => p.organization_id === org.id)" :key="cloud.ref" class="cloud-group">
     <button class="project-link" :disabled="busy" @click="emit('cloud', { account, project: cloud })"><span>{{ cloud.name }}<small>Cloud · {{ cloud.status }}</small></span></button>
     <button v-for="local in locals(cloud.ref)" :key="local.id" :class="['project-link', 'nested-local', { active: selectedId === local.id }]" :disabled="busy" @click="emit('select', local)"><span>{{ local.name }}<small>{{ localDescription(local) }}</small></span></button>
    </div>
   </div>
   <p v-if="!account.inventory.projects.length" class="muted">No accessible projects.</p>
   <small>Updated {{ account.inventory.updated_at ? new Date(account.inventory.updated_at).toLocaleString() : 'Never' }}</small>
  </details>
  <div class="sidebar-title"><span>{{ profiles.length ? 'UNGROUPED LOCAL PROJECTS' : 'YOUR PROJECTS' }}</span><span>{{ ungrouped.length }}</span></div>
  <button v-for="project in ungrouped" :key="project.id" :class="['project-link', { active: selectedId === project.id }]" :disabled="busy" @click="emit('select', project)"><span>{{ project.name }}<small>{{ localDescription(project) }}</small><small v-if="associations.some(a => a.local_project_id === project.id)">Cloud association currently inaccessible</small></span></button>
 </nav>
 <p v-if="error && !connect && !disconnect" role="alert" class="error-text">{{ error }}</p>
 <button class="add-project" :disabled="busy || !desktop || pending" @click="connect = true; error = ''">Connect account</button>
 <Modal v-if="connect" title="Connect Supabase account" :busy="pending" @close="connect = false; token = ''"><form @submit.prevent="add">
  <p>Connect a named profile to browse the organizations and projects accessible to its token. Labels do not verify account ownership.</p>
  <p class="muted">Create a scoped token with <strong>Organizations: Read</strong> and <strong>Organization Projects: Read</strong>. Restrict it to the resources you intend to show.</p>
  <button type="button" class="subtle" @click="tokensPage">Create a scoped token in Supabase</button>
  <label>Account label<input v-model="label" required maxlength="120" placeholder="Personal or Work" :disabled="pending"></label>
  <label>Personal access token<input v-model="token" type="password" required autocomplete="off" spellcheck="false" :disabled="pending"></label>
  <label class="check"><input v-model="sessionOnly" type="checkbox" :disabled="pending"> Connect for this app session only</label>
  <p class="muted">Saved tokens use your OS credential store. Session-only connections disappear when Toys closes.</p>
  <p v-if="error" role="alert" class="error-text">{{ error }}</p>
  <div class="dialog-actions"><button type="button" :disabled="pending" @click="connect = false; token = ''">Cancel</button><button class="primary" :disabled="pending">{{ pending ? 'Validating access…' : 'Connect account' }}</button></div>
 </form></Modal>
 <Modal v-if="disconnect" title="Disconnect account?" :busy="pending" @close="disconnect = null"><p>Remove {{ disconnect.label }} and its cached inventory from Toys. Local environments and associations remain available.</p><p>To revoke the token itself, visit <button class="subtle" @click="tokensPage">Supabase account token settings</button>.</p><p v-if="error" role="alert" class="error-text">{{ error }}</p><div class="dialog-actions"><button :disabled="pending" @click="disconnect = null">Cancel</button><button :disabled="pending" @click="remove">Disconnect account</button></div></Modal>
</template>
<style scoped>
.account-group { padding: 10px 0; border-bottom: 1px solid var(--border, #ddd); }
.account-group small { display: block; font-size: 11px; opacity: .75; }
.account-heading { display: flex; align-items: center; justify-content: space-between; cursor: pointer; }
.account-heading strong::before { content: '▾ '; color: var(--muted); }
.account-group:not([open]) .account-heading strong::before { content: '▸ '; }
.account-group .project-link { padding-top: 6px; padding-bottom: 6px; }
.account-group > .subtle { min-height: 28px; padding: 4px 10px; font-size: 11px; }
.organization-group { margin: 12px 0 8px 8px; font-size: 12px; }
.nested-local { margin-left: 12px; width: calc(100% - 12px); }
.account-tree { overflow: auto; min-height: 0; flex-shrink: 1; }
</style>
