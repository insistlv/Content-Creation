import { spawnSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const repositoryRoot = path.resolve(scriptDir, '..', '..')
const outputDir = path.join(repositoryRoot, '.cache', 'e2e')
const output = path.join(outputDir, process.platform === 'win32' ? 'workspace-e2e-server.exe' : 'workspace-e2e-server')

mkdirSync(outputDir, { recursive: true })
const result = spawnSync('go', ['build', '-o', output, './cmd/workspace-e2e-server'], {
  cwd: repositoryRoot,
  env: {
    ...process.env,
    GOCACHE: process.env.GOCACHE || path.join(repositoryRoot, '.cache', 'go-build'),
  },
  stdio: 'inherit',
})

if (result.error) throw result.error
if (result.status !== 0) process.exit(result.status ?? 1)
