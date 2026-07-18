import assert from 'node:assert/strict'
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'

export function assertRequiredPageArtifacts({
  appConfig,
  buildRoot,
  pages,
  extensions = ['.js', '.json', '.wxml', '.wxss'],
  exists = existsSync,
  resolvePath = resolve
}) {
  assert(Array.isArray(appConfig.pages), 'mp-weixin app.json must declare pages')

  for (const page of pages) {
    assert(appConfig.pages.includes(page), `mp-weixin app.json must register ${page}`)

    for (const extension of extensions) {
      assert(
        exists(resolvePath(buildRoot, `${page}${extension}`)),
        `mp-weixin required page artifact missing: ${page}${extension}`
      )
    }
  }
}
