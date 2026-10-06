<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { request, copyText } from '../api'
import type { EnvPreview } from '../types'
import Modal from './Modal.vue'
const props = defineProps<{ project: string }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const values = ref<Record<string,string>>({})
const file = ref('.env.local')
const source = ref('ANON_KEY')
const urlTarget = ref('SUPABASE_URL')
const keyTarget = ref('SUPABASE_ANON_KEY')
const revealed = ref(false)
const preview = ref<EnvPreview | null>(null)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const mapping = computed(() => ({ API_URL: urlTarget.value, [source.value]: keyTarget.value }))
const previewMapping = ref<Record<string,string>>({})
async function work(fn: () => Promise<void>) {
  busy.value = true; error.value = ''; notice.value = ''
  try { await fn() } catch(e) { error.value = String(e) } finally { busy.value = false }
}
onMounted(() => work(async () => { values.value = await request('connections', { project: props.project }) }))
async function generate() {
  await work(async () => { previewMapping.value = { ...mapping.value }; preview.value = await request('env_preview', { project: props.project, file: file.value, mapping: mapping.value }) })
}
async function save() {
  if (!preview.value) return
  await work(async () => { await request('env_apply', { preview: preview.value, mapping: previewMapping.value }); notice.value = 'Environment file saved. Restart your app to load its new settings.'; preview.value = null; emit('saved') })
}
async function copy(value: string) { await work(async () => { await copyText(value); notice.value = 'Copied to clipboard.' }) }
function shown(key: string, value: string) {
  let safeUrl = false
  try { const url = new URL(value); safeUrl = key.endsWith('URL') && ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password } catch { /* keys are not URLs */ }
  return revealed.value || safeUrl ? value : '••••••••••••••••'
}
</script>
<template>
  <Modal title="Connections & environment" :busy="busy" @close="emit('close')">
    <p class="muted">Live values for this project. Secret and service-role keys belong only in server code.</p>
    <label class="check"><input v-model="revealed" type="checkbox"> Reveal connection credentials</label>
    <div class="key-list"><div v-for="(value,key) in values" :key="key"><span>{{ key }}</span><code>{{ shown(String(key), value) }}</code><button :disabled="busy" @click="copy(value)">Copy</button></div></div>
    <h3>Export to your app</h3>
    <div class="form-grid"><label>Environment file<input v-model="file" :disabled="busy" placeholder=".env.local"></label><label>URL variable<input v-model="urlTarget" :disabled="busy"></label><label>Key source<select v-model="source" :disabled="busy"><option v-for="key in Object.keys(values).filter(k => k.includes('KEY'))" :key="key">{{ key }}</option></select></label><label>Key variable<input v-model="keyTarget" :disabled="busy"></label></div>
    <p class="muted">Only mapped variables change. Existing files are backed up; unrelated settings are preserved.</p>
    <button :disabled="busy || !Object.keys(values).length" @click="generate">Preview changes</button>
    <div v-if="preview" class="preview"><h3>Write to {{ preview.file }}</h3><p v-for="(value,key) in preview.updates" :key="key"><code>{{ key }}={{ shown(String(key), value) }}</code></p><button class="primary" :disabled="busy" @click="save">Write these changes</button></div>
    <p v-if="error" role="alert" class="error-text">{{ error }}</p><p v-if="notice" role="status" class="success-text">{{ notice }}</p>
  </Modal>
</template>
