const staticAssetBaseURL = `${import.meta.env.VITE_STATIC_ASSET_BASE_URL || ''}`.replace(/\/+$/, '')

function normalizeAssetPath(path) {
  return `${path || ''}`
    .trim()
    .replace(/^\/+/, '')
    .replace(/^static\/+/i, '')
}

export function staticAsset(path) {
  const normalizedPath = normalizeAssetPath(path)
  if (!normalizedPath) return staticAssetBaseURL
  if (staticAssetBaseURL) return `${staticAssetBaseURL}/${normalizedPath}`
  return `/${['static', normalizedPath].join('/')}`
}

export function staticIcon(name) {
  const normalizedName = `${name || ''}`.trim().replace(/\.png$/i, '')
  return staticAsset(`icons/${normalizedName}.png`)
}
