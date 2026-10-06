<script setup lang="ts">
import { ref } from 'vue'
import { request } from '../api'
import type { IdentityPreview } from '../types'
import Modal from './Modal.vue'
const props = defineProps<{ project: string }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const newId = ref(''), preview = ref<IdentityPreview | null>(null), busy = ref(false), error = ref('')
async function verify() {
  busy.value = true; error.value = ''; preview.value = null
  try { preview.value = await request('identity_preview', { project: props.project, new_id: newId.value }) }
  catch(e) { error.value = String(e) } finally { busy.value = false }
}
async function apply() {
  if (!preview.value) return
  busy.value = true; error.value = ''
  try { await request('identity_apply', { preview: preview.value }); emit('saved'); emit('close') }
  catch(e) { error.value = String(e) } finally { busy.value = false }
}
</script>
<template><Modal title="Review unused-project identity" :busy @close="emit('close')"><p class="muted">A project ID identifies its local resources. Changing it does not move database data. Toys offers this repair only when both IDs have no existing containers, volumes, or networks.</p><form @submit.prevent="verify"><label>New unique project ID<input v-model="newId" required pattern="[A-Za-z0-9_-]+" :disabled="busy"></label><button :disabled="busy">Verify & preview</button></form><div v-if="preview" class="preview"><p><code>{{ preview.before }} → {{ preview.after }}</code></p><p>No existing runtime resources were found for these identities.</p><button class="primary" :disabled="busy" @click="apply">Apply identity change</button></div><p v-if="error" role="alert" class="error-text">{{ error }}</p></Modal></template>
