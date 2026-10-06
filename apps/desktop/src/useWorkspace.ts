import { computed, onMounted, onUnmounted, ref } from 'vue'
import { request, isDesktop, listenProgress } from './api'
import type { Inventory, Project, Registry, Status } from './types'

export function useWorkspace() {
  const registry = ref<Registry>({ projects: [], settings: { supabase_cli: 'supabase', docker_cli: 'docker', docker_endpoint: null } })
  const selectedId = ref('')
  const status = ref<Status | null>(null)
  const inventory = ref<Inventory | null>(null)
  const busy = ref(false)
  const refreshing = ref(false)
  const error = ref('')
  const notice = ref('')
  const progress = ref('')
  const selected = computed(() => registry.value.projects.find(p => p.id === selectedId.value) ?? null)
  let timer: ReturnType<typeof setTimeout> | undefined
  let unlisten: (() => void) | undefined
  let disposed = false

  async function guarded<T>(work: () => Promise<T>): Promise<T | undefined> {
    if (busy.value) return
    busy.value = true; error.value = ''; notice.value = ''
    try { return await work() }
    catch (e) { error.value = String(e instanceof Error ? e.message : e); return undefined }
    finally { busy.value = false; progress.value = '' }
  }
  async function refresh() {
    if (!isDesktop() || refreshing.value || busy.value || disposed) return
    refreshing.value = true
    const id = selectedId.value
    try {
      const nextRegistry = await request<Registry>('registry')
      if (disposed) return
      registry.value = nextRegistry
      inventory.value = await request<Inventory>('inventory')
      if (id) {
        const next = await request<Status>('status', { project: id, resources: true })
        if (!disposed && selectedId.value === id) status.value = next
      }
    } catch (e) { error.value = String(e instanceof Error ? e.message : e) }
    finally { refreshing.value = false }
  }
  async function select(project: Project) {
    selectedId.value = project.id; status.value = null; error.value = ''; notice.value = ''
    if (refreshing.value) return // next scheduled refresh loads the newly selected project
    await refresh()
  }
  async function initialize() {
    await refresh()
    if (!selectedId.value && registry.value.projects[0]) await select(registry.value.projects[0])
  }
  async function lifecycle(operation: 'start' | 'stop' | 'restart') {
    const id = selectedId.value
    if (!id) return
    await guarded(async () => {
      status.value = await request<Status>('lifecycle', { project: id, operation })
      notice.value = `Project ${status.value.state}.`
    })
    await refresh()
  }
  async function poll() {
    if (disposed) return
    if (!document.hidden) await refresh()
    if (!disposed) timer = setTimeout(poll, 5000)
  }
  onMounted(async () => {
    if (isDesktop()) unlisten = await listenProgress(event => { if (event.project === selectedId.value) progress.value = event.message })
    await initialize()
    if (!disposed) timer = setTimeout(poll, 5000)
  })
  onUnmounted(() => { disposed = true; clearTimeout(timer); unlisten?.() })
  return { registry, selectedId, selected, status, inventory, busy, refreshing, error, notice, progress, guarded, refresh, select, lifecycle }
}
