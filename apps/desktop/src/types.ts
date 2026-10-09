export type Adapter = { kind: 'standard' } | { kind: 'stack'; stack_id: string }
export interface Project { id: string; name: string; path: string; project_id: string; adapter: Adapter }
export interface Settings { supabase_cli: string; docker_cli: string; docker_endpoint: string | null }
export interface Registry { projects: Project[]; settings: Settings; accounts?: AccountProfile[]; associations?: LocalProjectAssociation[] }
export interface Service { id: string; name: string; project_id: string; workdir: string | null; state: string; health: string | null; ports: number[] }
export interface Inventory { available: boolean; endpoint: string | null; services: Service[]; error: string | null }
export interface Diagnostic { code: string; severity: string; message: string; remedy: string; repairable: boolean }
export interface ResourceReading { container_id: string; cpu_percent: number | null; memory_bytes: number | null }
export interface Status {
  project: Project; state: string; services: Service[]; diagnostics: Diagnostic[]
  configured_ports: Record<string, number>; endpoints: Record<string, string>; resources: ResourceReading[]; resources_error: string | null; supabase_version: string | null
}
export interface RepairPreview { mode?: 'manual'; project: string; config_hash: string; changes: { key: string; before: number; after: number }[]; restart_required: boolean }
export interface EnvPreview { project: string; file: string; source_hash: string; updates: Record<string, string> }
export interface OperationProgress { project: string; phase: string; message: string }
export interface IdentityPreview { project: string; config_hash: string; before: string; after: string }
export interface CloudOrganization { id: string; slug: string; name: string }
export interface CloudProject { ref: string; organization_id: string; name: string; region: string; status: string }
export interface CloudInventory { account_id: string; organizations: CloudOrganization[]; projects: CloudProject[]; updated_at: string; stale: boolean; access_state: string; message: string }
export interface AccountProfile { id: string; label: string; credential_ref?: string; session_only: boolean; inventory: CloudInventory }
export interface LocalProjectAssociation { local_project_id: string; cloud_ref: string }
export interface AssociationSuggestion { local_project_id: string; cloud_ref: string; account_ids: string[] }
export interface CloudSelection { account: AccountProfile; project: CloudProject }
