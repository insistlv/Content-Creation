import { spawn, spawnSync } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const webRoot = path.resolve(scriptDir, '..')
const repositoryRoot = path.resolve(webRoot, '..')
const playwrightCLI = path.join(webRoot, 'node_modules', '@playwright', 'test', 'cli.js')
const serverBinary = path.join(repositoryRoot, '.cache', 'e2e', process.platform === 'win32' ? 'workspace-e2e-server.exe' : 'workspace-e2e-server')
const viteCLI = path.join(webRoot, 'node_modules', 'vite', 'bin', 'vite.js')
const args = process.argv.slice(2)

function run(command, commandArgs, options = {}) {
  return spawn(command, commandArgs, { stdio: 'inherit', ...options })
}

function waitForExit(child) {
  return new Promise((resolve, reject) => {
    child.once('error', reject)
    child.once('exit', (code, signal) => resolve(code ?? (signal ? 1 : 0)))
  })
}

async function waitForURL(url, timeoutMs = 120_000) {
  const deadline = Date.now() + timeoutMs
  let lastError
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url)
      if (response.ok) return
      lastError = new Error(`${url} 返回 ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 200))
  }
  throw new Error(`等待 ${url} 超时：${lastError?.message || '服务不可用'}`)
}

async function stop(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return
  child.kill()
  await Promise.race([
    waitForExit(child),
    new Promise(resolve => setTimeout(resolve, 3_000)),
  ])
  if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL')
}

async function main() {
  const externalBaseURL = process.env.PLAYWRIGHT_BASE_URL
  let backend
  let frontend
  let interrupted = false
  const interrupt = () => { interrupted = true }
  process.once('SIGINT', interrupt)
  process.once('SIGTERM', interrupt)

  try {
    if (!externalBaseURL) {
      const build = spawnSync(process.execPath, [path.join(scriptDir, 'build-workspace-e2e-server.mjs')], {
        cwd: webRoot,
        env: process.env,
        stdio: 'inherit',
      })
      if (build.error) throw build.error
      if (build.status !== 0) return build.status ?? 1

      const sharedEnv = {
        ...process.env,
        PORT: '8889',
        VITE_API_PROXY_TARGET: 'http://127.0.0.1:8889',
      }
      backend = run(serverBinary, [], { cwd: repositoryRoot, env: sharedEnv })
      frontend = run(process.execPath, [viteCLI, '--host', '127.0.0.1', '--port', '15173'], { cwd: webRoot, env: sharedEnv })
      await Promise.all([
        waitForURL('http://127.0.0.1:8889/api/workspace/discovery'),
        waitForURL('http://127.0.0.1:15173/'),
      ])
    }

    if (interrupted) return 130
    const test = run(process.execPath, [playwrightCLI, 'test', ...args], {
      cwd: webRoot,
      env: {
        ...process.env,
        PLAYWRIGHT_BASE_URL: externalBaseURL || 'http://127.0.0.1:15173',
      },
    })
    return await waitForExit(test)
  } finally {
    process.removeListener('SIGINT', interrupt)
    process.removeListener('SIGTERM', interrupt)
    await Promise.all([stop(frontend), stop(backend)])
  }
}

process.exitCode = await main()
