<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  BarChart3,
  Bell,
  Boxes,
  ChevronDown,
  ChevronRight,
  ClipboardCheck,
  FileClock,
  Folder,
  Gift,
  Image,
  Images,
  KeyRound,
  LayoutTemplate,
  LogOut,
  MessageCircle,
  Menu,
  Package,
  Siren,
  ReceiptText,
  Search,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  Users,
  Video,
  X
} from 'lucide-vue-next'

import { api, getCurrentAdminSession } from '../api/client.js'

const route = useRoute()
const router = useRouter()
const menus = ref([])
const admin = ref(null)
const errorMessage = ref('')
const sidebarOpen = ref(false)
const profileMenuOpen = ref(false)
const profileMenuRef = ref(null)
const searchWrapRef = ref(null)
const adminSearchQuery = ref('')
const adminSearchLoading = ref(false)
const adminSearchError = ref('')
const adminSearchResult = ref(null)
const adminSearchPanelOpen = ref(false)
const expandedMenuGroups = ref(new Set())
const collapsedMenuGroups = ref(new Set())
const passwordModalOpen = ref(false)
const passwordSubmitting = ref(false)
const passwordError = ref('')
const passwordForm = ref({
  current_password: '',
  new_password: '',
  confirm_password: ''
})

const iconMap = {
  '/admin': BarChart3,
  '/admin/users': Users,
  '/admin/packages': Package,
  '/admin/finance-orders': ReceiptText,
  '/admin/system-settings': SlidersHorizontal,
	'/admin/ecommerce-categories': Folder,
  '/admin/system-logs': FileClock,
  '/admin/customer-service': MessageCircle,
  '/admin/prompt-templates': LayoutTemplate,
  '/admin/inspiration-recommendations': Sparkles,
  '/admin/video-style-presets': Video,
  '/admin/couple-album-options': Images,
  '/admin/announcements': Bell,
  '/admin/settings': Settings,
  '/admin/invites': Gift,
  '/admin/generations': Image,
  '/admin/video-generations': Video,
  '/admin/content-reviews': ClipboardCheck,
  '/admin/content-reports': MessageCircle,
  '/admin/algorithm-compliance': ShieldCheck,
  '/admin/incidents': Siren,
  '/admin/permissions': ShieldCheck
}

const menuGroupConfig = [
  {
    key: 'core',
    label: '核心运营',
    paths: ['/admin', '/admin/users', '/admin/generations', '/admin/video-generations']
  },
  {
    key: 'growth',
    label: '交易增长',
    paths: ['/admin/packages', '/admin/finance-orders', '/admin/invites']
  },
  {
    key: 'content',
    label: '内容治理',
    paths: ['/admin/content-reviews', '/admin/content-reports', '/admin/algorithm-compliance', '/admin/incidents']
  },
  {
    key: 'config',
    label: '配置中心',
	paths: ['/admin/customer-service', '/admin/prompt-templates', '/admin/inspiration-recommendations', '/admin/video-style-presets', '/admin/couple-album-options', '/admin/announcements', '/admin/settings', '/admin/system-settings', '/admin/ecommerce-categories']
  },
  {
    key: 'system',
    label: '系统管理',
    paths: ['/admin/system-resources', '/admin/system-logs', '/admin/permissions']
  }
]

const adminInitial = computed(() => {
  const text = admin.value?.display_name || admin.value?.username || 'A'
  return text.slice(0, 1).toUpperCase()
})

const adminDisplayName = computed(() => admin.value?.display_name || admin.value?.username || '后台账号')
const adminAccountName = computed(() => admin.value?.username || 'Admin')
const adminSearchSections = computed(() => adminSearchResult.value?.sections ?? [])
const hasAdminSearchItems = computed(() => adminSearchSections.value.some((section) => (section.items ?? []).length > 0))
const groupedMenus = computed(() => {
  const menusByPath = new Map(menus.value.map((item) => [item.path, item]))
  return menuGroupConfig
    .map((group) => ({
      ...group,
      items: group.paths.map((path) => menusByPath.get(path)).filter(Boolean)
    }))
    .filter((group) => group.items.length > 0)
})

const currentMenuGroupKey = computed(() => {
  const currentPath = route.path
  return menuGroupConfig.find((group) => group.paths.some((path) => isMenuPathActive(path, currentPath)))?.key ?? ''
})

async function loadAdmin() {
  try {
    const payload = await getCurrentAdminSession(true)
    admin.value = payload.admin
    menus.value = payload.menus ?? []
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function logout() {
  closeProfileMenu()
  await api.adminLogout()
  router.push('/admin/login')
}

function menuIcon(path) {
  return iconMap[path] ?? Boxes
}

function isMenuPathActive(menuPath, currentPath) {
  if (menuPath === '/admin') {
    return currentPath === '/admin'
  }
  return currentPath === menuPath || currentPath.startsWith(`${menuPath}/`)
}

function isMenuGroupExpanded(group) {
  if (expandedMenuGroups.value.has(group.key)) {
    return true
  }
  if (collapsedMenuGroups.value.has(group.key)) {
    return false
  }
  return currentMenuGroupKey.value === group.key
}

function toggleMenuGroup(groupKey) {
  const nextExpanded = new Set(expandedMenuGroups.value)
  const nextCollapsed = new Set(collapsedMenuGroups.value)
  const group = groupedMenus.value.find((item) => item.key === groupKey)

  if (!group) {
    return
  }

  if (isMenuGroupExpanded(group)) {
    nextExpanded.delete(groupKey)
    nextCollapsed.add(groupKey)
  } else {
    nextCollapsed.delete(groupKey)
    nextExpanded.add(groupKey)
  }

  expandedMenuGroups.value = nextExpanded
  collapsedMenuGroups.value = nextCollapsed
}

function closeSidebar() {
  sidebarOpen.value = false
}

function toggleProfileMenu() {
  profileMenuOpen.value = !profileMenuOpen.value
}

function closeProfileMenu() {
  profileMenuOpen.value = false
}

function closeAdminSearchPanel() {
  adminSearchPanelOpen.value = false
}

async function submitAdminSearch() {
  const query = adminSearchQuery.value.trim()
  adminSearchError.value = ''
  if (!query) {
    adminSearchResult.value = null
    adminSearchPanelOpen.value = false
    return
  }

  adminSearchLoading.value = true
  adminSearchPanelOpen.value = true
  try {
    adminSearchResult.value = await api.searchAdmin({ q: query })
  } catch (error) {
    adminSearchResult.value = { query, sections: [] }
    adminSearchError.value = error.message || '搜索失败'
  } finally {
    adminSearchLoading.value = false
  }
}

function navigateAdminSearchResult(item) {
  if (!item?.to) {
    return
  }
  closeAdminSearchPanel()
  router.push(item.to)
}

function handleDocumentClick(event) {
  if (profileMenuOpen.value && !profileMenuRef.value?.contains(event.target)) {
    closeProfileMenu()
  }
  if (adminSearchPanelOpen.value && !searchWrapRef.value?.contains(event.target)) {
    closeAdminSearchPanel()
  }
}

function handleDocumentKeydown(event) {
  if (event.key !== 'Escape') {
    return
  }
  closeProfileMenu()
  closeAdminSearchPanel()
  if (passwordModalOpen.value && !passwordSubmitting.value) {
    closePasswordModal()
  }
}

function resetPasswordForm() {
  passwordForm.value = {
    current_password: '',
    new_password: '',
    confirm_password: ''
  }
  passwordError.value = ''
}

function openPasswordModal() {
  closeProfileMenu()
  resetPasswordForm()
  passwordModalOpen.value = true
}

function closePasswordModal() {
  if (passwordSubmitting.value) {
    return
  }
  passwordModalOpen.value = false
  resetPasswordForm()
}

async function submitPasswordChange() {
  passwordError.value = ''
  if (passwordForm.value.new_password.trim().length < 8) {
    passwordError.value = '新密码至少 8 位'
    return
  }
  if (passwordForm.value.new_password !== passwordForm.value.confirm_password) {
    passwordError.value = '两次输入的新密码不一致'
    return
  }

  passwordSubmitting.value = true
  try {
    await api.changeAdminPassword({
      current_password: passwordForm.value.current_password,
      new_password: passwordForm.value.new_password
    })
    passwordModalOpen.value = false
    router.push('/admin/login')
  } catch (error) {
    passwordError.value = error.message || '密码修改失败'
  } finally {
    passwordSubmitting.value = false
  }
}

onMounted(() => {
  loadAdmin()
  document.addEventListener('click', handleDocumentClick)
  document.addEventListener('keydown', handleDocumentKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleDocumentClick)
  document.removeEventListener('keydown', handleDocumentKeydown)
})
</script>

<template>
  <div class="admin-shell" :class="{ 'sidebar-open': sidebarOpen }">
    <aside class="admin-sidebar">
      <div class="admin-brand">
        <div class="admin-brand-mark">IA</div>
        <div>
          <p class="brand-kicker">白霖共享</p>
          <h2>运营后台</h2>
        </div>
      </div>
      <nav class="admin-nav">
        <section v-for="group in groupedMenus" :key="group.key" class="admin-nav-group">
          <button
            class="admin-nav-group-toggle"
            type="button"
            :aria-expanded="isMenuGroupExpanded(group)"
            :aria-controls="`admin-nav-group-panel-${group.key}`"
            :data-testid="`admin-nav-group-${group.key}`"
            @click="toggleMenuGroup(group.key)"
          >
            <Folder :size="14" />
            <span>{{ group.label }}</span>
            <ChevronDown class="admin-nav-group-chevron" :size="14" />
          </button>
          <div v-if="isMenuGroupExpanded(group)" :id="`admin-nav-group-panel-${group.key}`" class="admin-nav-group-items">
            <RouterLink
              v-for="item in group.items"
              :key="item.path"
              :to="item.path"
              :data-testid="`admin-nav-link-${item.path}`"
              @click="closeSidebar"
            >
              <component :is="menuIcon(item.path)" :size="18" />
              <span>{{ item.label }}</span>
              <ChevronRight class="admin-nav-chevron" :size="15" />
            </RouterLink>
          </div>
        </section>
      </nav>
      <div class="admin-sidebar-footer">
        <div class="admin-avatar">{{ adminInitial }}</div>
        <div>
          <p>{{ adminDisplayName }}</p>
          <span>管理员</span>
        </div>
        <button class="mini-button icon-only" type="button" aria-label="退出后台" @click="logout">
          <LogOut :size="16" />
        </button>
      </div>
    </aside>
    <main class="admin-main">
      <header class="admin-topbar">
        <button class="mini-button admin-menu-toggle" type="button" aria-label="打开菜单" @click="sidebarOpen = !sidebarOpen">
          <component :is="sidebarOpen ? X : Menu" :size="18" />
        </button>
        <div ref="searchWrapRef" class="admin-global-search-wrap">
          <form class="admin-top-search" data-testid="admin-global-search-form" role="search" @submit.prevent="submitAdminSearch">
            <Search :size="17" />
            <input
              v-model="adminSearchQuery"
              data-testid="admin-global-search-input"
              type="search"
              placeholder="搜索用户、记录或配置"
              autocomplete="off"
              aria-label="后台全局搜索"
            />
            <button class="admin-global-search-button" data-testid="admin-global-search-submit" type="submit" :disabled="adminSearchLoading || !adminSearchQuery.trim()" aria-label="搜索后台内容">
              <Search :size="15" />
              <span>{{ adminSearchLoading ? '搜索中' : '搜索' }}</span>
            </button>
          </form>
          <div v-if="adminSearchPanelOpen" class="admin-global-search-panel" data-testid="admin-global-search-panel">
            <p v-if="adminSearchError" class="admin-global-search-error">{{ adminSearchError }}</p>
            <template v-else-if="hasAdminSearchItems">
              <section v-for="section in adminSearchSections" :key="section.key" class="admin-global-search-section">
                <template v-if="(section.items ?? []).length > 0">
                  <p>{{ section.label }}</p>
                  <button
                    v-for="item in section.items"
                    :key="`${section.key}-${item.id}`"
                    class="admin-global-search-result"
                    :data-testid="`admin-global-search-result-${item.id}`"
                    type="button"
                    @click="navigateAdminSearchResult(item)"
                  >
                    <span>
                      <strong>{{ item.title }}</strong>
                      <small>{{ item.subtitle }}</small>
                    </span>
                    <ChevronRight :size="15" />
                  </button>
                </template>
              </section>
            </template>
            <p v-else class="admin-global-search-empty">没有找到匹配结果</p>
          </div>
        </div>
        <div class="admin-top-actions">
          <RouterLink class="mini-button admin-quick-link" to="/workspace" aria-label="打开创作台">
            <Sparkles :size="16" />
            <span>创作台</span>
          </RouterLink>
          <button class="mini-button icon-only admin-notify-button" type="button" aria-label="通知">
            <Bell :size="17" />
            <span aria-hidden="true"></span>
          </button>
          <div ref="profileMenuRef" class="admin-profile-menu-wrap">
            <button
              class="admin-top-profile"
              data-testid="admin-profile-button"
              type="button"
              aria-haspopup="menu"
              :aria-expanded="profileMenuOpen"
              @click="toggleProfileMenu"
            >
              <span class="admin-top-profile-avatar">{{ adminInitial }}</span>
              <strong>{{ adminDisplayName }}</strong>
              <ChevronDown class="admin-profile-chevron" :size="15" />
            </button>
            <div v-if="profileMenuOpen" class="admin-profile-menu" data-testid="admin-profile-menu" role="menu">
              <div class="admin-profile-menu-head">
                <div class="admin-avatar">{{ adminInitial }}</div>
                <div>
                  <strong>{{ adminDisplayName }}</strong>
                  <span>{{ adminAccountName }}</span>
                </div>
              </div>
              <button class="admin-profile-menu-item" data-testid="admin-menu-change-password" type="button" role="menuitem" @click="openPasswordModal">
                <KeyRound :size="16" />
                <span>修改密码</span>
              </button>
              <button class="admin-profile-menu-item danger" data-testid="admin-menu-logout" type="button" role="menuitem" @click="logout">
                <LogOut :size="16" />
                <span>退出登录</span>
              </button>
            </div>
          </div>
        </div>
      </header>
      <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
      <div class="admin-content-surface">
        <RouterView />
      </div>
    </main>
    <div v-if="passwordModalOpen" class="dashboard-modal-backdrop admin-password-modal-backdrop" data-testid="admin-password-modal" @click.self="closePasswordModal">
      <form class="dashboard-modal admin-panel admin-password-modal" data-testid="admin-password-form" @submit.prevent="submitPasswordChange">
        <div class="panel-title-row">
          <h2>修改密码</h2>
          <button class="mini-button icon-only" type="button" aria-label="关闭修改密码弹层" :disabled="passwordSubmitting" @click="closePasswordModal">
            <X :size="16" />
          </button>
        </div>
        <label>
          <span class="meta-label">当前密码</span>
          <input v-model="passwordForm.current_password" class="text-input" data-testid="admin-password-current" type="password" autocomplete="current-password" required />
        </label>
        <label>
          <span class="meta-label">新密码</span>
          <input v-model="passwordForm.new_password" class="text-input" data-testid="admin-password-new" type="password" autocomplete="new-password" required />
        </label>
        <label>
          <span class="meta-label">确认新密码</span>
          <input v-model="passwordForm.confirm_password" class="text-input" data-testid="admin-password-confirm" type="password" autocomplete="new-password" required />
        </label>
        <p v-if="passwordError" class="status-error">{{ passwordError }}</p>
        <div class="admin-password-actions">
          <button class="mini-button" type="button" :disabled="passwordSubmitting" @click="closePasswordModal">取消</button>
          <button class="primary-button" type="submit" :disabled="passwordSubmitting">
            {{ passwordSubmitting ? '提交中' : '保存并重新登录' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>
