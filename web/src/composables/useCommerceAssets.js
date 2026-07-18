import { ref } from 'vue'
import { api } from '../api/client.js'
import { commerceUserMessage } from '../components/ecommerce/commerceUserMessages.js'

export function useCommerceAssets() {
  const assets = ref([])
  const loading = ref(false)
  const error = ref('')
  let generation = 0
  async function refresh(projectId) {
    if (!projectId) return []
    const current = ++generation
    loading.value = true
    try {
      const response = await api.listCommerceAssets(projectId)
      const items = Array.isArray(response) ? response : (response?.items || [])
      if (current === generation) {
        assets.value = items
        error.value = ''
      }
      return items
    } catch (reason) {
      if (current === generation) error.value = commerceUserMessage(reason, '素材加载失败，请稍后重试')
      return []
    } finally {
      if (current === generation) loading.value = false
    }
  }
  async function upload(projectId, file, binding = {}) {
    const normalized = typeof binding === 'string' ? { role: binding } : binding
    const policy = await api.createCommerceAssetUploadPolicy(projectId, { filename: file.name, mime_type: file.type, size: file.size })
    await api.uploadCommerceAssetBinary(policy, file)
    await api.completeCommerceAssetUpload(projectId, {
      object_key: policy.object_key,
      upload_token: policy.upload_token,
      role: normalized.role,
      lifecycle: normalized.lifecycle || 'project',
      ...(normalized.sku_id ? { sku_id: normalized.sku_id } : {}),
      ...(Number.isInteger(normalized.sort_order) ? { sort_order: normalized.sort_order } : {})
    })
    return refresh(projectId)
  }
  async function remove(projectId, assetId) { await api.deleteCommerceAsset(projectId, assetId); return refresh(projectId) }
  return { assets, loading, error, refresh, upload, remove }
}
