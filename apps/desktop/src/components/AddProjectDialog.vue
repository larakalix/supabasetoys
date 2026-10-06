<script setup lang="ts">
import { ref } from 'vue'
import { request, pickFolder } from '../api'
import type { Project } from '../types'
import Modal from './Modal.vue'
const emit = defineEmits<{ close: []; added: [project: Project] }>()
const path = ref(''), name = ref(''), stackId = ref(''), advanced = ref(false), busy = ref(false), error = ref('')
async function browse() { try { const selected = await pickFolder(); if (typeof selected === 'string') path.value = selected } catch(e) { error.value = String(e) } }
async function add() {
  busy.value = true; error.value = ''
  try { const project = await request<Project>('add', { path: path.value, name: name.value || null, stack_id: advanced.value && stackId.value ? stackId.value : null }); emit('added', project); emit('close') }
  catch(e) { error.value = String(e) } finally { busy.value = false }
}
</script>
<template><Modal title="Add a local project" :busy="busy" @close="emit('close')"><form @submit.prevent="add">
  <p class="muted">Choose the folder containing <code>supabase/config.toml</code>. Registration associates this folder with its local Supabase identity.</p>
  <label>Project folder<div class="input-action"><input v-model="path" required placeholder="/path/to/your/project"><button type="button" :disabled="busy" @click="browse">Browse</button></div></label>
  <label>Display name <span class="muted">(optional)</span><input v-model="name" placeholder="Defaults to folder name"></label>
  <label class="check"><input v-model="advanced" type="checkbox"> Associate an existing experimental Docker stack</label>
  <label v-if="advanced">Full stack ID<input v-model="stackId" required><small>Find it with supabase stack list --output-format json. Toys verifies the folder and runtime before registration.</small></label>
  <p v-if="error" role="alert" class="error-text">{{ error }}</p><div class="dialog-actions"><button type="button" :disabled="busy" @click="emit('close')">Cancel</button><button class="primary" :disabled="busy">{{ busy ? 'Adding…' : 'Add project' }}</button></div>
</form></Modal></template>
