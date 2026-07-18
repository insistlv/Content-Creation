import { test } from 'node:test'
import assert from 'node:assert/strict'
import { assertRequiredPageArtifacts } from './mp-build-contract.mjs'

const requiredAlbumPages = [
  'pages/couple-album/create/index',
  'pages/couple-album/detail/index',
  'pages/couple-album/share/index'
]
const requiredArtifacts = ['.js', '.json', '.wxml', '.wxss']

function artifactSetFor(pages) {
  return new Set(pages.flatMap((page) => requiredArtifacts.map((extension) => `${page}${extension}`)))
}

test('accepts registered couple album pages with complete artifacts', () => {
  const existingArtifacts = artifactSetFor(requiredAlbumPages)

  assert.doesNotThrow(() => {
    assertRequiredPageArtifacts({
      appConfig: { pages: requiredAlbumPages },
      buildRoot: 'dist/build/mp-weixin',
      pages: requiredAlbumPages,
      exists: (path) => existingArtifacts.has(path),
      resolvePath: (_, path) => path
    })
  })
})

test('fails when a required couple album page is not registered in app.json', () => {
  const registeredPages = requiredAlbumPages.filter((page) => page !== 'pages/couple-album/create/index')
  const existingArtifacts = artifactSetFor(requiredAlbumPages)

  assert.throws(
    () => {
      assertRequiredPageArtifacts({
        appConfig: { pages: registeredPages },
        buildRoot: 'dist/build/mp-weixin',
        pages: requiredAlbumPages,
        exists: (path) => existingArtifacts.has(path),
        resolvePath: (_, path) => path
      })
    },
    /must register pages\/couple-album\/create\/index/
  )
})

test('fails when a required couple album page artifact is missing', () => {
  const existingArtifacts = artifactSetFor(requiredAlbumPages)
  existingArtifacts.delete('pages/couple-album/create/index.wxml')

  assert.throws(
    () => {
      assertRequiredPageArtifacts({
        appConfig: { pages: requiredAlbumPages },
        buildRoot: 'dist/build/mp-weixin',
        pages: requiredAlbumPages,
        exists: (path) => existingArtifacts.has(path),
        resolvePath: (_, path) => path
      })
    },
    /required page artifact missing: pages\/couple-album\/create\/index\.wxml/
  )
})
