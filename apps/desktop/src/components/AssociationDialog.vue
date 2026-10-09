<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { request } from '../api'
import type { Registry, AssociationSuggestion } from '../types'
import Modal from './Modal.vue'
const props = defineProps<{ registry: Registry; local?: string; account?: string; cloudRef?: string }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const local = ref(props.local ?? ''), account = ref(props.account ?? ''), cloudRef = ref(props.cloudRef ?? '')
const busy = ref(false), error = ref(''), suggestions = ref<AssociationSuggestion[]>([])
const profiles = computed(() => props.registry.accounts ?? [])
const projects = computed(() => profiles.value.find(a => a.id === account.value)?.inventory.projects ?? [])
onMounted(async () => {
 if (!props.local) return
 try { suggestions.value = await request<AssociationSuggestion[]>('association_suggestions', { project: props.local }) }
 catch(e) { error.value = String(e) }
})
function useSuggestion(suggestion: AssociationSuggestion) {
 cloudRef.value = suggestion.cloud_ref
 if (suggestion.account_ids.length === 1) account.value = suggestion.account_ids[0]!
}
async function save() {
 busy.value = true; error.value = ''
 try { await request('associate', { project: local.value, account: account.value, cloud_ref: cloudRef.value }); emit('saved'); emit('close') }
 catch(e) { error.value = e instanceof Error ? e.message : String(e) }
 finally { busy.value = false }
}
</script>
<template><Modal title="Associate local environment" :busy @close="emit('close')"><form @submit.prevent="save">
 <p>Group a local registration under its hosted project. This does not change Supabase links, configuration, credentials, or data.</p>
 <p v-for="suggestion in suggestions" :key="suggestion.cloud_ref">Existing Supabase link: <code>{{ suggestion.cloud_ref }}</code> <button type="button" :disabled="busy" @click="useSuggestion(suggestion)">Use suggestion</button><small v-if="suggestion.account_ids.length > 1">Accessible through several profiles; choose the account used to verify access.</small></p>
 <label>Local environment<select v-model="local" required :disabled="busy || !!props.local"><option disabled value="">Select a registered folder</option><option v-for="p in registry.projects" :key="p.id" :value="p.id">{{ p.name }} · {{ p.path }}</option></select></label>
 <label>Account profile<select v-model="account" required :disabled="busy || !!props.account" @change="cloudRef = ''"><option disabled value="">Select account</option><option v-for="a in profiles" :key="a.id" :value="a.id">{{ a.label }}</option></select></label>
 <label>Hosted project<select v-model="cloudRef" required :disabled="busy || !!props.cloudRef"><option disabled value="">Select hosted project</option><option v-for="p in projects" :key="p.ref" :value="p.ref">{{ p.name }} · {{ p.ref }}</option></select></label>
 <p v-if="error" role="alert" class="error-text">{{ error }}</p><div class="dialog-actions"><button type="button" :disabled="busy" @click="emit('close')">Cancel</button><button class="primary" :disabled="busy || !local || !account || !cloudRef">{{ busy ? 'Verifying access…' : 'Confirm association' }}</button></div>
</form></Modal></template>
