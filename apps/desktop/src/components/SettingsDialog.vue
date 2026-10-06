<script setup lang="ts">
import { reactive, ref } from 'vue'
import { request } from '../api'
import type { Settings } from '../types'
import Modal from './Modal.vue'
const props = defineProps<{ settings: Settings }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const fields = reactive({ ...props.settings })
const busy = ref(false)
const error = ref('')
async function save() {
  busy.value = true; error.value = ''
  try { await request('configure', { settings: { ...fields, docker_endpoint: fields.docker_endpoint || null } }); emit('saved'); emit('close') }
  catch(e) { error.value = String(e) } finally { busy.value = false }
}
</script>
<template><Modal title="Local setup" :busy="busy" @close="emit('close')"><form @submit.prevent="save">
  <p class="muted">Toys uses your installed tools. It does not install or upgrade them automatically.</p>
  <label>Supabase executable<input v-model="fields.supabase_cli" required placeholder="supabase"><small>Supported releases: 2.119.0 and 2.118.0. An absolute path works for project-specific installations.</small></label>
  <label>Docker executable<input v-model="fields.docker_cli" required placeholder="docker"></label>
  <label>Local Docker endpoint<input v-model="fields.docker_endpoint" placeholder="Detect from current context"><small>Unix socket or Windows named pipe. The endpoint is pinned on the first lifecycle action.</small></label>
  <p v-if="error" role="alert" class="error-text">{{ error }}</p><div class="dialog-actions"><button type="button" :disabled="busy" @click="emit('close')">Cancel</button><button class="primary" :disabled="busy">{{ busy ? 'Saving…' : 'Save settings' }}</button></div>
</form></Modal></template>
