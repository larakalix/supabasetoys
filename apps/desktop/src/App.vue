<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { request, isDesktop, cancelOperation, saveDiagnosticsPath } from './api'
import { useWorkspace } from './useWorkspace'
import type { Project, RepairPreview, Status } from './types'
import Icon from './components/Icon.vue'
import Modal from './components/Modal.vue'
import ProjectOverview from './components/ProjectOverview.vue'
import LogsPanel from './components/LogsPanel.vue'
import AddProjectDialog from './components/AddProjectDialog.vue'
import SettingsDialog from './components/SettingsDialog.vue'
import ConnectionDialog from './components/ConnectionDialog.vue'
import IdentityDialog from './components/IdentityDialog.vue'
import PortsDialog from './components/PortsDialog.vue'

const { registry, selectedId, selected, status, inventory, busy, refreshing, error, notice, progress, guarded, refresh, select, lifecycle } = useWorkspace()
const tab = ref<'overview' | 'logs' | 'diagnostics'>('overview')
const addDialog = ref(false), settingsDialog = ref(false), connectionsDialog = ref(false), removeDialog = ref(false)
const identityDialog = ref(false)
const portsDialog = ref(false)
const repair = ref<RepairPreview | null>(null)
const report = ref('')
const includeLogs = ref(false)
const desktop = isDesktop()
const unregistered = computed(() => [...new Set(inventory.value?.services.map(s => s.project_id).filter(id => id && !registry.value.projects.some(p => (p.adapter.kind === 'stack' ? p.adapter.stack_id : p.project_id) === id)) ?? [])])
watch(selectedId, () => { tab.value = 'overview'; report.value = ''; repair.value = null; connectionsDialog.value = false; portsDialog.value = false })
async function added(project: Project) { await refresh(); await select(project) }
async function previewRepair() { await guarded(async () => { repair.value = await request('repair_preview', { project: selectedId.value }) }) }
async function applyRepair() {
  const preview = repair.value
  if (!preview) return
  await guarded(async () => { status.value = await request<Status>('repair_apply', { preview, restart: preview.restart_required }); repair.value = null; notice.value = 'Port repair applied. Check your app environment settings before reconnecting.' })
  await refresh()
}
async function openEndpoint(key: string) { await guarded(async () => { await request('open_endpoint', { project: selectedId.value, key }) }) }
async function generateReport() { await guarded(async () => { report.value = (await request<{ report: string }>('diagnostics', { project: selectedId.value, include_logs: includeLogs.value })).report }) }
async function exportReport() {
  await guarded(async () => { const path = await saveDiagnosticsPath(); if (path) { await request('export_diagnostics', { project: selectedId.value, path, include_logs: includeLogs.value }); notice.value = 'Redacted diagnostic report exported.' } })
}
async function remove() {
  await guarded(async () => { await request('remove', { project: selectedId.value }); selectedId.value = ''; status.value = null; removeDialog.value = false; notice.value = 'Registration removed. Containers and database data were preserved.' })
  await refresh()
}
</script>
<template>
  <div class="app-shell">
    <aside class="sidebar">
      <a class="brand" href="#" @click.prevent><span class="brand-icon"><Icon name="box" /></span><span>supabase<span class="brand-suffix">toys</span></span><span class="version">0.1</span></a>
      <div class="sidebar-title"><span>YOUR PROJECTS</span><span>{{ registry.projects.length }}</span></div>
      <nav aria-label="Projects"><button v-for="project in registry.projects" :key="project.id" :class="['project-link', { active: selectedId === project.id }]" :disabled="busy" @click="select(project)"><Icon name="box" /><span>{{ project.name }}<small>{{ project.project_id }}</small></span><span v-if="selectedId === project.id" class="dot" :class="status?.state ?? 'stopped'" /></button></nav>
      <button class="add-project" :disabled="busy || !desktop" @click="addDialog = true"><Icon name="plus" /> Add project</button>
      <div v-if="unregistered.length" class="discovered"><span class="eyebrow">ALSO ON THIS MACHINE</span><p v-for="id in unregistered" :key="id">{{ id }}</p><small>Add its folder to manage it.</small></div>
      <div class="sidebar-bottom"><div class="local-status"><span class="dot" :class="inventory?.available ? 'running' : 'stopped'" />{{ inventory?.available ? 'Docker connected' : desktop ? 'Docker unavailable' : 'Interface preview' }}</div><button class="subtle" :disabled="busy || !desktop" @click="settingsDialog = true"><Icon name="settings" /> Local setup</button><small>Free & open source · MIT</small></div>
    </aside>
    <main>
      <div class="topbar"><span><Icon name="box" /> Local development <span class="slash">/</span> <strong>{{ selected?.name ?? 'Welcome' }}</strong></span><span class="topbar-right"><span class="local-badge">LOCAL ONLY</span><button class="icon-button" :disabled="busy || refreshing || !desktop" aria-label="Refresh project status" @click="refresh"><Icon name="refresh" /></button></span></div>
      <div class="main-content">
        <div v-if="!desktop" class="preview-banner">Interface preview. Launch <code>pnpm desktop</code> to manage your local Supabase projects.</div>
        <div v-if="error" role="alert" class="alert error"><strong>Action needs attention</strong><p>{{ error }}</p><button class="subtle" @click="error = ''">Dismiss</button></div>
        <div v-if="notice" role="status" class="alert success">{{ notice }}</div>
        <div v-if="busy" role="status" class="operation"><span class="spinner" />{{ progress || 'Working…' }}<button :disabled="!progress" @click="cancelOperation">Cancel operation</button></div>
        <template v-if="selected">
          <header class="page-header"><div><p class="eyebrow">PROJECT WORKSPACE</p><h1>{{ selected.name }}</h1><p class="project-path">{{ selected.path }}</p></div><div class="page-actions"><button :disabled="busy || !status || status.state === 'unavailable'" @click="lifecycle('restart')"><Icon name="refresh" /> Restart</button><button v-if="status?.services.some(s => s.state === 'running')" :disabled="busy" @click="lifecycle('stop')"><Icon name="stop" /> Stop</button><button v-else class="primary" :disabled="busy || !status || status.state === 'unavailable'" @click="lifecycle('start')"><Icon name="play" /> Start project</button></div></header>
          <nav class="tabs" aria-label="Project views"><button v-for="item in (['overview','logs','diagnostics'] as const)" :key="item" :aria-current="tab === item ? 'page' : undefined" :class="{ active: tab === item }" @click="tab = item">{{ item }}</button><span>{{ selected.adapter.kind === 'stack' ? 'Experimental stack' : 'Standard CLI' }}</span></nav>
          <div v-if="!status" class="inline-empty">Reading project status…</div>
          <ProjectOverview v-else-if="tab === 'overview'" :status :busy @ports="portsDialog = true" @repair="previewRepair" @connections="connectionsDialog = true" @open="openEndpoint" />
          <LogsPanel v-else-if="tab === 'logs'" :key="selectedId" :project="selectedId" :services="status?.services ?? []" :busy />
          <section v-else class="panel"><div class="section-heading"><div><h2>Diagnostics</h2><p>Inspect setup issues and export a redacted report for a GitHub issue.</p></div><button :disabled="busy" @click="previewRepair">Preview port repair</button></div><button v-if="selected.adapter.kind === 'standard'" :disabled="busy" @click="identityDialog = true">Review unused-project identity</button><label class="check"><input v-model="includeLogs" type="checkbox"> Include recent redacted logs</label><div class="toolbar"><button :disabled="busy" @click="generateReport">Generate report</button><button :disabled="busy" @click="exportReport">Export JSON</button></div><pre v-if="report" class="log-output" tabindex="0" aria-label="Diagnostic report">{{ report }}</pre><p v-else class="inline-empty">Reports include CLI version, service health, port conflicts, and resource readings. Review any included log text before sharing.</p></section>
          <footer class="project-footer"><span>{{ status?.supabase_version ? `Supabase CLI ${status.supabase_version}` : 'Supabase CLI not detected' }}</span><button class="subtle" :disabled="busy" @click="removeDialog = true">Remove registration</button></footer>
        </template>
        <section v-else class="welcome"><span class="welcome-icon"><Icon name="box" /></span><p class="eyebrow">A LITTLE ORDER FOR YOUR LOCAL STACKS</p><h1>More projects.<br>Fewer surprises.</h1><p>Give every Supabase project its own space.<br>See what’s running, catch conflicts, and get back to building.</p><button class="primary" :disabled="!desktop || busy" @click="addDialog = true"><Icon name="plus" /> Add your first project</button><div class="welcome-steps"><div><span>01</span><strong>Add a folder</strong><p>Your existing Supabase setup stays yours.</p></div><div><span>02</span><strong>Check your setup</strong><p>Catch port and identity conflicts before starting.</p></div><div><span>03</span><strong>Start building</strong><p>Open Studio and connect your app.</p></div></div></section>
      </div>
    </main>
    <AddProjectDialog v-if="addDialog" @close="addDialog = false" @added="added" />
    <SettingsDialog v-if="settingsDialog" :settings="registry.settings" @close="settingsDialog = false" @saved="refresh" />
    <ConnectionDialog v-if="connectionsDialog && selected" :project="selectedId" @close="connectionsDialog = false" @saved="refresh" />
    <IdentityDialog v-if="identityDialog && selected" :project="selectedId" @close="identityDialog = false" @saved="refresh" />
    <PortsDialog v-if="portsDialog && status" :status @close="portsDialog = false" @applied="portsDialog = false; notice = 'Project ports updated. Check your app environment settings.'; refresh()" />
    <Modal v-if="repair" title="Review port repair" :busy @close="repair = null"><p class="muted">Only conflicting ports will change. Working ports, project identity, and database data are preserved. A config backup is created before writing.</p><div v-if="repair.changes.length" class="repair-list"><div v-for="change in repair.changes" :key="change.key"><code>{{ change.key }}</code><span>{{ change.before }} → <strong>{{ change.after }}</strong></span></div></div><p v-else class="success-text">No port changes are needed.</p><p v-if="repair.restart_required" class="warning-text">Applying this repair will stop and restart only {{ selected?.name }}.</p><p class="muted">Update your app’s environment URLs after changing ports.</p><p v-if="error" role="alert" class="error-text">{{ error }}</p><div class="dialog-actions"><button :disabled="busy" @click="repair = null">Cancel</button><button class="primary" :disabled="busy || !repair.changes.length" @click="applyRepair">{{ repair.restart_required ? 'Apply & restart project' : 'Apply these changes' }}</button></div></Modal>
    <Modal v-if="removeDialog" title="Remove project registration?" :busy @close="removeDialog = false"><p>This removes {{ selected?.name }} from Toys. Running containers, configuration, and database data remain in place.</p><div class="dialog-actions"><button :disabled="busy" @click="removeDialog = false">Cancel</button><button :disabled="busy" @click="remove">Remove registration</button></div></Modal>
  </div>
</template>
