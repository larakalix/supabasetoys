import type { OperationProgress } from './types'
interface DesktopBridge {
 Request(payload: Record<string, unknown>): Promise<unknown>
 Cancel(): Promise<void>
 PickFolder(): Promise<string>
 SaveDiagnosticsPath(): Promise<string>
 CopyText(value: string): Promise<void>
}
declare global {
 interface Window {
  go?: { main?: { App?: DesktopBridge } }
  runtime?: { EventsOn(name: string, callback: (payload: OperationProgress) => void): () => void }
 }
}
export const isDesktop = () => Boolean(window.go?.main?.App)
function bridge(): DesktopBridge {
 const app = window.go?.main?.App
 if (!app) throw new Error('Run the desktop app with pnpm desktop to connect to your local projects. This browser view is an interface preview.')
 return app
}
export async function request<T>(action: string, fields: Record<string, unknown> = {}): Promise<T> {
 return await bridge().Request({ action, ...fields }) as T
}
export async function cancelOperation() { return bridge().Cancel() }
export async function pickFolder() { return bridge().PickFolder() }
export async function saveDiagnosticsPath() { return bridge().SaveDiagnosticsPath() }
export async function copyText(value: string) { return bridge().CopyText(value) }
export async function listenProgress(callback: (payload: OperationProgress) => void): Promise<() => void> {
 if (!window.runtime) throw new Error('Desktop events are unavailable')
 return window.runtime.EventsOn('operation-progress', callback)
}
