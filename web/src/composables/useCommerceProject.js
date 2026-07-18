import { ref } from 'vue'
import { api } from '../api/client.js'
import { commerceUserMessage } from '../components/ecommerce/commerceUserMessages.js'

const itemsOf = (value) => Array.isArray(value) ? value : (value?.items || [])

export function useCommerceProject() {
  const tabs = ['概览', '商品与 SKU', '素材', 'AI 生产', '批次']
  const tab = ref('概览')
  const recipes = ref([])
  const capabilities = ref(null)
  const projects = ref([])
  const currentProject = ref(null)
  const loading = ref(false)
  const error = ref('')

  async function refresh() {
    loading.value = true
    error.value = ''
    try {
      projects.value = itemsOf(await api.listCommerceProjects())
      if (!currentProject.value && projects.value.length) currentProject.value = projects.value[0]
      return projects.value
    } catch (reason) {
      error.value = commerceUserMessage(reason, '项目加载失败，请稍后重试')
      return []
    } finally { loading.value = false }
  }

  async function create(input) {
    const project = await api.createCommerceProject(input)
    currentProject.value = project
    await refresh()
    return project
  }

  async function remove(project) {
    await api.deleteCommerceProject(project.id)
    if (currentProject.value?.id === project.id) currentProject.value = null
    await refresh()
  }

  function isTabDisabled(name) {
    return name === 'AI 生产' && (!capabilities.value?.enabled || !capabilities.value?.worker_enabled)
  }

  async function initialize() {
    const [capabilityResult, projectResult, recipeResult] = await Promise.allSettled([
      api.getCommerceCapabilities(), refresh(), api.listCommerceRecipes()
    ])
    if (capabilityResult.status === 'fulfilled') capabilities.value = capabilityResult.value
    else error.value = commerceUserMessage(capabilityResult.reason, '能力读取失败，请稍后重试')
    if (projectResult.status === 'rejected' && !error.value) error.value = commerceUserMessage(projectResult.reason, '项目读取失败，请稍后重试')
    if (recipeResult.status === 'fulfilled') recipes.value = itemsOf(recipeResult.value)
    else if (!error.value) error.value = commerceUserMessage(recipeResult.reason, '生成方案读取失败，请稍后重试')
  }

  return { tabs, tab, recipes, capabilities, projects, currentProject, loading, error, refresh, create, remove, initialize, isTabDisabled }
}
