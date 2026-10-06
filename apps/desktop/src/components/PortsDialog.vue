<script setup lang="ts">
import { computed, ref } from 'vue'
import { request } from '../api'
import type { RepairPreview, Status } from '../types'
import Modal from './Modal.vue'
const { status } = defineProps<{ status: Status }>()
const emit = defineEmits<{ close: []; applied: [] }>()
const values = ref<Record<string, number>>(Object.fromEntries(Object.entries(status.configured_ports)))
const preview = ref<RepairPreview | null>(null)
const busy = ref(false)
const error = ref('')
const changed = computed(() => Object.entries(values.value).some(([key, value]) => value !== status.configured_ports[key]))
async function review() {
  busy.value = true; error.value = ''; preview.value = null
  try {
    const ports = Object.fromEntries(Object.entries(values.value).filter(([key, value]) => value !== status.configured_ports[key]))
    preview.value = await request<RepairPreview>('ports_preview', { project: status.project.id, ports })
  } catch (cause) { error.value = String(cause) }
  finally { busy.value = false }
}
async function apply() {
  if (!preview.value) return
  busy.value = true; error.value = ''
  try {
    await request('repair_apply', { preview: preview.value, restart: preview.value.restart_required })
    emit('applied')
  } catch (cause) { error.value = String(cause); preview.value = null }
  finally { busy.value = false }
}
</script>
<template>
  <Modal title="Change project ports" :busy @close="emit('close')">
    <p class="muted">Choose a separate port for each enabled service. Toys checks other projects and local listeners before writing, and creates a configuration backup.</p>
    <form v-if="!preview" @submit.prevent="review">
      <label v-for="key in Object.keys(values).sort()" :key="key" class="port-setting"><span>{{ key }}</span><input v-model.number="values[key]" :aria-label="key" type="number" min="1" max="65535" step="1" required :disabled="busy"></label>
      <p v-if="error" role="alert" class="error-text">{{ error }}</p>
      <div class="dialog-actions"><button type="button" :disabled="busy" @click="emit('close')">Cancel</button><button type="submit" class="primary" :disabled="busy || !changed">Review changes</button></div>
    </form>
    <template v-else>
      <div class="repair-list"><div v-for="change in preview.changes" :key="change.key"><code>{{ change.key }}</code><span>{{ change.before }} → <strong>{{ change.after }}</strong></span></div></div>
      <p v-if="preview.restart_required" class="warning-text">Applying will stop and restart only {{ status.project.name }}.</p>
      <p class="muted">Update your app’s environment URLs after changing ports. Database data and project identity are preserved.</p>
      <div class="dialog-actions"><button :disabled="busy" @click="preview = null">Back</button><button class="primary" :disabled="busy || !preview.changes.length" @click="apply">{{ preview.restart_required ? 'Apply & restart project' : 'Apply these changes' }}</button></div>
    </template>
  </Modal>
</template>
<style scoped>
.port-setting { display:flex; align-items:center; justify-content:space-between; gap:1rem; margin:0.75rem 0; }
.port-setting input { width:8rem; }
</style>
