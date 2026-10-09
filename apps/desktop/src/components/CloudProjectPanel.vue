<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { request } from '../api'
import type { CloudSelection, CloudInventory, Registry, Project } from '../types'
import AssociationDialog from './AssociationDialog.vue'
const props = defineProps<{ selection: CloudSelection; registry: Registry }>()
const emit = defineEmits<{ changed: []; select: [project: Project] }>()
const inventory = ref<CloudInventory>(props.selection.account.inventory)
const busy = ref(false), error = ref(''), associate = ref(false)
let generation = 0
watch(() => props.selection, () => { generation++; inventory.value = props.selection.account.inventory; busy.value = false; error.value = ''; associate.value = false })
onUnmounted(() => { generation++ })
const cloud = computed(() => inventory.value.projects.find(p => p.ref === props.selection.project.ref))
const locals = computed(() => props.registry.projects.filter(p => props.registry.associations?.some(a => a.local_project_id === p.id && a.cloud_ref === props.selection.project.ref)))
async function refresh() {
 if (busy.value) return
 const current = ++generation, account = props.selection.account.id
 busy.value = true; error.value = ''
 try { const next = await request<CloudInventory>('cloud_list', { account, refresh: true }); if (current !== generation) return; inventory.value = next; emit('changed') }
 catch(e) { if (current === generation) error.value = String(e) }
 finally { if (current === generation) busy.value = false }
}
async function open() {
 try { await request('open_dashboard', { account: props.selection.account.id, cloud_ref: props.selection.project.ref }) }
 catch(e) { error.value = String(e) }
}
defineExpose({ refresh, busy })
</script>
<template><section>
 <header class="page-header"><div><p class="eyebrow">CLOUD PROJECT · READ ONLY</p><h1>{{ cloud?.name ?? selection.project.name }}</h1><p>{{ selection.account.label }} · {{ selection.project.ref }}</p></div><div class="page-actions"><button :disabled="busy" @click="refresh">{{ busy ? 'Refreshing…' : 'Refresh cloud inventory' }}</button><button :disabled="busy || !cloud" @click="open">Open Dashboard</button></div></header>
 <p v-if="error" role="alert" class="error-text">{{ error }}</p>
 <div class="panel"><p v-if="inventory.stale" class="warning-text">Stale inventory — these readings may have changed.</p><p>{{ inventory.message }}</p><p v-if="!cloud">This project is no longer visible through this profile. Local associations are retained.</p><dl v-else><dt>Region</dt><dd>{{ cloud.region || 'Unavailable' }}</dd><dt>Reported status</dt><dd>{{ cloud.status || 'Unavailable' }}</dd><dt>Last updated</dt><dd>{{ inventory.updated_at ? new Date(inventory.updated_at).toLocaleString() : 'Never' }}</dd></dl></div>
 <section class="panel"><div class="section-heading"><div><h2>Linked local environments</h2><p>Local start, stop, ports, logs, and resources are available when you select an environment.</p></div><button :disabled="busy || !cloud || !registry.projects.length" @click="associate = true">Associate local environment</button></div><button v-for="p in locals" :key="p.id" class="project-link" @click="emit('select', p)"><span>{{ p.name }}<small>Local · {{ p.path }}</small></span></button><p v-if="!locals.length" class="muted">No local environments associated yet. Add a local folder, then associate it here.</p></section>
 <AssociationDialog v-if="associate" :registry :account="selection.account.id" :cloud-ref="selection.project.ref" @close="associate = false" @saved="emit('changed')" />
</section></template>
