<script setup lang="ts">
import { computed } from 'vue'
import type { Status } from '../types'
import Icon from './Icon.vue'
const { status, busy } = defineProps<{ status: Status; busy: boolean }>()
const emit = defineEmits<{ ports: []; repair: []; connections: []; open: [key: string] }>()
const measured = computed(() => status.resources.filter(r => r.cpu_percent !== null && r.memory_bytes !== null))
const runningCount = computed(() => status.services.filter(s => s.state === 'running').length)
const memory = computed(() => measured.value.reduce((n,r) => n + (r.memory_bytes ?? 0), 0) / 1024 ** 3)
const cpu = computed(() => measured.value.reduce((n,r) => n + (r.cpu_percent ?? 0), 0))
const partial = computed(() => measured.value.length < runningCount.value)
const readings = computed(() => new Map(status.resources.map(reading => [reading.container_id, reading])))
const endpoints = computed(() => {
  const seen = new Set<string>()
  return Object.entries(status.endpoints).filter(([, url]) => {
    if (seen.has(url)) return false
    seen.add(url)
    return true
  })
})
const endpointLabel = (key: string) => ({ STUDIO_URL: 'Studio', INBUCKET_URL: 'Email inbox', MAILPIT_URL: 'Email inbox', API_URL: 'API' }[key] ?? key.replace(/_URL$/, '').replaceAll('_', ' '))
</script>
<template>
  <div class="metrics">
    <article class="metric"><span>PROJECT STATE</span><strong class="capitalize">{{ status.state }}</strong><small>{{ runningCount }} of {{ status.services.length }} services running</small></article>
    <article class="metric"><span>CPU USAGE <Icon name="activity" /></span><strong>{{ measured.length ? `${cpu.toFixed(1)}%` : '—' }}</strong><small>{{ !measured.length ? 'No reading available' : partial ? 'Partial reading' : 'Across project containers' }}</small></article>
    <article class="metric"><span>MEMORY</span><strong>{{ measured.length ? `${memory.toFixed(2)} GiB` : '—' }}</strong><small>{{ status.resources_error ? 'Docker stats unavailable' : !measured.length ? 'No reading available' : partial ? 'Partial reading' : 'Current measured usage' }}</small></article>
  </div>
  <section v-if="status.diagnostics.length" class="issues">
    <div class="section-heading"><h2>Needs attention <span class="count">{{ status.diagnostics.length }}</span></h2><button v-if="status.diagnostics.some(d => d.repairable)" :disabled="busy" @click="emit('repair')">Review port repair <Icon name="arrow" /></button></div>
    <article v-for="(issue, i) in status.diagnostics" :key="`${issue.code}-${i}`" class="issue" :class="issue.severity">
      <span class="issue-symbol" aria-hidden="true">!</span><div><strong>{{ issue.message }}</strong><p>{{ issue.remedy }}</p></div><span class="badge">{{ issue.repairable ? 'Fix available' : issue.severity }}</span>
    </article>
  </section>
  <section class="panel">
    <div class="section-heading"><div><h2>Project ports</h2><p>Configured host ports. Live bindings appear under Services.</p></div><button :disabled="busy || status.project.adapter.kind !== 'standard' || !Object.keys(status.configured_ports).length" @click="emit('ports')">Change ports</button></div>
    <div v-for="[key, port] in Object.entries(status.configured_ports).sort(([a], [b]) => a.localeCompare(b))" :key="key" class="connection"><span>{{ key }}</span><code>{{ port }}</code></div>
    <p v-if="!Object.keys(status.configured_ports).length" class="inline-empty">No supported configured ports available.</p>
    <p v-if="status.project.adapter.kind === 'stack'" class="muted">Experimental stacks allocate ports automatically.</p>
  </section>
  <section class="panel">
    <div class="section-heading"><div><h2>Connections</h2><p>The endpoints for this project, from runtime status.</p></div><button :disabled="busy || status.state !== 'running'" @click="emit('connections')">Keys & environment <Icon name="arrow" /></button></div>
    <div v-if="!Object.keys(status.endpoints).length" class="inline-empty">Start a healthy project to see its connection details.</div>
    <div v-for="[key,url] in endpoints" :key="key" class="connection"><span>{{ endpointLabel(key) }}</span><code>{{ url }}</code><button :disabled="busy" class="subtle" @click="emit('open', key)">Open <Icon name="arrow" /></button></div>
  </section>
  <section class="panel">
    <div class="section-heading"><h2>Services <span class="count">{{ status.services.length }}</span></h2><span class="muted">Local Docker</span></div>
    <div v-if="!status.services.length" class="inline-empty">No services discovered for this project yet.</div>
    <div v-for="service in status.services" :key="service.id" class="service-row"><span class="dot" :class="service.state === 'running' && service.health !== 'unhealthy' ? 'running' : 'stopped'" /><strong>{{ service.name }}<small class="service-resources">CPU {{ readings.get(service.id)?.cpu_percent != null ? `${readings.get(service.id)!.cpu_percent!.toFixed(1)}%` : 'unavailable' }} · RAM {{ readings.get(service.id)?.memory_bytes != null ? `${(readings.get(service.id)!.memory_bytes! / 1024 ** 2).toFixed(1)} MiB` : 'unavailable' }}</small></strong><code>{{ service.ports.length ? service.ports.join(', ') : 'Internal' }}</code><span class="badge">{{ service.health ?? service.state }}</span></div>
  </section>
</template>
