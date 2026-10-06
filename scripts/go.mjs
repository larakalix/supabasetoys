// Prefer the optional project-local Go installation; never install a toolchain here.
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { spawn } from 'node:child_process'
const local = resolve('.tools/go-toolchain/go/bin', process.platform === 'win32' ? 'go.exe' : 'go')
const executable = existsSync(local) ? local : 'go'
const env = { ...process.env, GOTELEMETRY: 'off' }
if (process.platform === 'darwin') {
 env.MACOSX_DEPLOYMENT_TARGET = '13.0'
 for (const name of ['CGO_CFLAGS', 'CGO_LDFLAGS']) {
  if (!(env[name] ?? '').includes('-mmacosx-version-min')) env[name] = `${env[name] ?? ''} -mmacosx-version-min=13.0`.trim()
 }
 if (!(env.CGO_LDFLAGS ?? '').includes('UniformTypeIdentifiers')) env.CGO_LDFLAGS += ' -framework UniformTypeIdentifiers'
}
if (existsSync(local)) {
 env.GOPATH = resolve('.tools/gopath')
 env.GOCACHE = resolve('.tools/go-cache')
 env.PATH = `${resolve('.tools/go-toolchain/go/bin')}${process.platform === 'win32' ? ';' : ':'}${env.PATH ?? ''}`
}
const child = spawn(executable, process.argv.slice(2), { stdio: 'inherit', env })
child.on('error', error => { console.error(`Cannot run Go. Install Go 1.27.1: ${error.message}`); process.exitCode = 1 })
child.on('exit', (code, signal) => { if (signal) process.kill(process.pid, signal); else process.exitCode = code ?? 1 })
