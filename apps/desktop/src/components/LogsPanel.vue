<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { request } from '../api'
import type { Service } from '../types'
const props = defineProps<{ project: string; services: Service[]; busy: boolean }>()
const service = ref('')
const logs = ref('')
const following = ref(false)
const loading = ref(false)
const error = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
let since: string | null = null
async function load(append = false) {
  if (loading.value || props.busy || disposed) return
  loading.value = true; error.value = ''
  const next = Math.floor(Date.now() / 1000).toString()
  try {
    const result = await request<{ logs: string }>('logs', { project: props.project, service: service.value || null, since: append ? since : null })
    if (!disposed) logs.value = (append ? logs.value + '\n' + result.logs : result.logs).slice(-100000)
    since = next
  } catch (e) { error.value = String(e) }
  finally { loading.value = false }
}
async function poll() {
  if (disposed) return
  if (following.value && !document.hidden) await load(true)
  if (!disposed) timer = setTimeout(poll, 2000)
}
watch(service, () => { since = null; void load() })
onMounted(() => { void load(); timer = setTimeout(poll, 2000) })
onUnmounted(() => { disposed = true; clearTimeout(timer) })
</script>
<template>
  <section class="panel"><div class="section-heading"><div><h2>Project logs</h2><p>Credentials are redacted. Recent history is limited to 200 lines per service.</p></div></div>
    <div class="toolbar"><label>Service <select v-model="service"><option value="">All services</option><option v-for="item in services" :key="item.id" :value="item.id">{{ item.name }}</option></select></label><label class="check"><input v-model="following" type="checkbox"> Follow new logs</label><button :disabled="loading || busy" @click="load()">{{ loading ? 'Loading…' : 'Refresh' }}</button></div>
    <p v-if="error" role="alert" class="error-text">{{ error }}</p><pre class="log-output" tabindex="0" aria-label="Service logs">{{ logs || 'No log lines available.' }}</pre>
  </section>
</template>
