<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import {
  Home,
  FolderOpen,
  Type,
  Video,
  Clapperboard,
  Music,
  Sparkles,
  CreditCard,
  User,
  LogIn,
  UserPlus,
  ChevronDown,
  ChevronRight,
  Headphones,
  LogOut,
  ReceiptText,
  Settings,
  ShieldCheck,
  WalletCards,
  ShoppingBag
} from 'lucide-vue-next'
import SoftPanel from './SoftPanel.vue'
import ThemeToggle from './ThemeToggle.vue'

const props = defineProps({
  me: {
    type: Object,
    default: null
  },
  currentRoute: {
    type: String,
    default: ''
  },
  theme: {
    type: String,
    default: 'dark'
  },
  userMenuToggleTestId: {
    type: String,
    default: 'workspace-user-menu-trigger'
  },
  userMenuTestId: {
    type: String,
    default: 'workspace-user-menu'
  },
  logoutButtonTestId: {
    type: String,
    default: 'workspace-user-menu-logout'
  }
})

const emit = defineEmits(['navigate', 'recharge', 'support', 'logout'])

const primaryNavItems = [
  { id: 'workspace', label: '工作台', icon: Home, route: '/workspace', available: true },
  { id: 'works', label: '作品库', icon: Sparkles, route: '/works', available: true },
  { id: 'assets', label: '素材库', icon: FolderOpen, route: '/assets', available: true },
  { id: 'account', label: '账户', icon: User, route: '/account', available: true },
  { id: 'support', label: '联系客服', icon: Headphones, route: '/contact', available: true }
]

const aiGenerationItems = [
  { id: 'text-to-image', label: '图像工坊', icon: Type, route: '/workspace', available: true }
]

const aiToolItems = [
  { id: 'ai-commerce', label: 'AI 电商', icon: ShoppingBag, route: '/workspace/ai-commerce', available: true },
  { id: 'text-to-video', label: '视频生成', icon: Video, route: '/workspace/video', available: true },
  { id: 'novel-video', label: '小说视频', icon: Clapperboard, route: '/workspace/novel-video', available: true },
  { id: 'ai-avatar', label: 'AI数字人', icon: User, route: '/workspace/ai-avatar', available: false },
  { id: 'ai-music', label: 'AI音乐', icon: Music, route: '/workspace/ai-music', available: false }
]

const workspaceSubGroups = [
  { id: 'ai-generation', label: 'AI生成', items: aiGenerationItems },
  { id: 'ai-tools', label: 'AI创作', items: aiToolItems }
]

function isWorkspaceRoute(route) {
  return route === '/workspace' || route.startsWith('/workspace/')
}

const credits = computed(() => props.me?.available_credits ?? 0)
const isLoggedIn = computed(() => Boolean(props.me?.username))
const userName = computed(() => props.me?.display_name || props.me?.username || '用户')
const userAvatarLabel = computed(() => userName.value.trim().slice(0, 1).toUpperCase() || 'U')
const userTier = computed(() => props.me?.tier ?? 'Free')
const userId = computed(() => props.me?.user_id ?? props.me?.id ?? props.me?.username ?? '未登录')
const showUnavailableTip = ref(false)
const userMenuOpen = ref(false)
const userMenuRoot = ref(null)
const userMenuTrigger = ref(null)
const userMenuPanel = ref(null)
const userMenuStyle = ref({})
let unavailableTipTimer = null
let hoverOpenedAt = 0

const desktopMenuWidth = 292
const estimatedMenuHeight = 320
const viewportGap = 12
const mobileBreakpoint = 768

const userMenuThemeClass = computed(() => (
  props.theme === 'light' ? 'workspace-user-menu-light' : 'workspace-user-menu-dark'
))

const userMenuItems = [
  { id: 'credits', label: '查看权益', icon: ShieldCheck, action: 'navigate', route: '/account#credits' },
  { id: 'ledger', label: '充值记录', icon: ReceiptText, action: 'navigate', route: '/account#ledger' },
  { id: 'profile', label: '个人设置', icon: Settings, action: 'navigate', route: '/account#profile' },
  { id: 'support', label: '在线客服', icon: Headphones, action: 'support' },
  { id: 'pricing', label: '点数套餐', icon: WalletCards, action: 'recharge' }
]

function showUnavailablePrompt() {
  showUnavailableTip.value = true
  if (unavailableTipTimer) {
    clearTimeout(unavailableTipTimer)
  }
  unavailableTipTimer = window.setTimeout(() => {
    showUnavailableTip.value = false
    unavailableTipTimer = null
  }, 1800)
}

function handleMenuItem(item) {
  if (!item.available) {
    showUnavailablePrompt()
    return
  }
  if (item.id === 'support') {
    emit('support')
    return
  }
  emit('navigate', item.route)
}

const workspaceExpanded = ref(isWorkspaceRoute(props.currentRoute))

watch(() => props.currentRoute, (route) => {
  workspaceExpanded.value = isWorkspaceRoute(route)
})

function handlePrimaryItem(item) {
  if (item.id === 'workspace') {
    if (isWorkspaceRoute(props.currentRoute)) {
      workspaceExpanded.value = !workspaceExpanded.value
    } else {
      workspaceExpanded.value = true
    }
    emit('navigate', item.route)
    return
  }
  workspaceExpanded.value = false
  handleMenuItem(item)
}

function isItemActive(item) {
  if (item.route === '/workspace') {
    return props.currentRoute === item.route
  }
  return props.currentRoute === item.route || props.currentRoute.startsWith(`${item.route}/`)
}

function isPrimaryActive(item) {
  if (item.id === 'workspace') {
    return isWorkspaceRoute(props.currentRoute)
  }
  return isItemActive(item)
}

function handleRecharge() {
  emit('recharge')
}

function handleAuthRoute(routePath) {
  emit('navigate', routePath)
}

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max)
}

function formatPx(value) {
  return `${Math.round(value)}px`
}

function updateUserMenuPosition() {
  const trigger = userMenuTrigger.value
  if (!trigger) return

  const rect = trigger.getBoundingClientRect()
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth || 0
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight || 0
  const isMobile = viewportWidth <= mobileBreakpoint
  const menuWidth = isMobile
    ? clamp(rect.width, 240, Math.max(240, viewportWidth - viewportGap * 2))
    : desktopMenuWidth
  const menuHeight = userMenuPanel.value?.offsetHeight || estimatedMenuHeight

  if (isMobile) {
    const left = clamp(rect.left, viewportGap, Math.max(viewportGap, viewportWidth - menuWidth - viewportGap))
    const top = clamp(
      rect.top - menuHeight - 10,
      viewportGap,
      Math.max(viewportGap, viewportHeight - viewportGap - menuHeight)
    )
    userMenuStyle.value = {
      position: 'fixed',
      zIndex: '1000',
      left: formatPx(left),
      top: formatPx(top),
      width: formatPx(menuWidth),
      maxHeight: formatPx(Math.min(420, Math.max(260, viewportHeight - viewportGap * 2))),
      overflowY: 'auto'
    }
    return
  }

  const left = clamp(
    rect.right + viewportGap,
    viewportGap,
    Math.max(viewportGap, viewportWidth - desktopMenuWidth - viewportGap)
  )
  const top = clamp(
    rect.bottom - menuHeight,
    viewportGap,
    Math.max(viewportGap, viewportHeight - viewportGap - menuHeight)
  )
  userMenuStyle.value = {
    position: 'fixed',
    zIndex: '1000',
    left: formatPx(left),
    top: formatPx(top),
    width: formatPx(desktopMenuWidth),
    maxHeight: formatPx(Math.max(260, viewportHeight - viewportGap * 2)),
    overflowY: 'auto'
  }
}

function openUserMenu() {
  if (userMenuOpen.value) {
    updateUserMenuPosition()
    return
  }
  hoverOpenedAt = Date.now()
  userMenuOpen.value = true
}

function closeUserMenu() {
  hoverOpenedAt = 0
  userMenuOpen.value = false
}

function toggleUserMenu() {
  if (userMenuOpen.value && Date.now() - hoverOpenedAt < 300) {
    hoverOpenedAt = 0
    updateUserMenuPosition()
    return
  }
  hoverOpenedAt = 0
  userMenuOpen.value = !userMenuOpen.value
}

function handleUserMenuItem(item) {
  closeUserMenu()
  if (item.action === 'navigate') {
    emit('navigate', item.route)
    return
  }
  if (item.action === 'support') {
    emit('support')
    return
  }
  if (item.action === 'recharge') {
    emit('recharge')
  }
}

function handleUserLogout() {
  closeUserMenu()
  emit('logout')
}

function handleUserMenuKeydown(event) {
  if (event.key === 'Escape') {
    closeUserMenu()
  }
}

function handleDocumentKeydown(event) {
  if (event.key === 'Escape') {
    closeUserMenu()
  }
}

function handleOutsidePointerDown(event) {
  if (!userMenuOpen.value) return
  if (userMenuRoot.value?.contains(event.target)) return
  if (userMenuPanel.value?.contains(event.target)) return
  closeUserMenu()
}

watch(userMenuOpen, async (open) => {
  if (open) {
    await nextTick()
    updateUserMenuPosition()
    document.addEventListener('pointerdown', handleOutsidePointerDown)
    document.addEventListener('keydown', handleDocumentKeydown)
    window.addEventListener('resize', updateUserMenuPosition)
    window.addEventListener('scroll', updateUserMenuPosition, true)
    return
  }
  document.removeEventListener('pointerdown', handleOutsidePointerDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('resize', updateUserMenuPosition)
  window.removeEventListener('scroll', updateUserMenuPosition, true)
})

onBeforeUnmount(() => {
  if (unavailableTipTimer) {
    clearTimeout(unavailableTipTimer)
  }
  document.removeEventListener('pointerdown', handleOutsidePointerDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('resize', updateUserMenuPosition)
  window.removeEventListener('scroll', updateUserMenuPosition, true)
})
</script>

<template>
  <template v-if="showUnavailableTip">
    <div class="workspace-soft-toast" role="status" data-testid="workspace-unavailable-tip">
      功能暂未开放，敬请期待!
    </div>
  </template>

  <SoftPanel class="workspace-sidebar" tone="default">
    <div class="sidebar-content">
      <div class="sidebar-header">
        <div class="sidebar-brand-kicker">创作者 AI 图片平台</div>
        <h1 class="sidebar-title">
          <Sparkles :size="24" class="title-icon" />
          白霖共享
        </h1>
      </div>

      <nav class="sidebar-nav">
        <div class="nav-section">
          <template v-for="item in primaryNavItems" :key="item.id">
            <button
              class="nav-item"
              :class="{ active: isPrimaryActive(item), 'nav-item-parent': item.id === 'workspace' }"
              :data-testid="`workspace-nav-${item.id}`"
              :aria-expanded="item.id === 'workspace' ? String(workspaceExpanded) : undefined"
              @click="handlePrimaryItem(item)"
            >
              <component :is="item.icon" :size="20" />
              <span>{{ item.label }}</span>
              <ChevronDown
                v-if="item.id === 'workspace'"
                :size="16"
                class="nav-parent-chevron"
                :class="{ expanded: workspaceExpanded }"
                aria-hidden="true"
              />
            </button>

            <Transition v-if="item.id === 'workspace'" name="nav-drawer">
              <div
                v-if="workspaceExpanded"
                class="nav-subdrawer"
                data-testid="workspace-nav-subdrawer"
              >
                <div
                  v-for="group in workspaceSubGroups"
                  :key="group.id"
                  class="nav-subgroup"
                >
                  <div class="section-title">{{ group.label }}</div>
                  <button
                    v-for="sub in group.items"
                    :key="sub.id"
                    class="nav-item nav-subitem"
                    :class="{ active: isItemActive(sub) }"
                    :data-testid="`workspace-nav-${sub.id}`"
                    @click="handleMenuItem(sub)"
                  >
                    <component :is="sub.icon" :size="18" />
                    <span>{{ sub.label }}</span>
                  </button>
                </div>
              </div>
            </Transition>
          </template>
        </div>
      </nav>

      <div class="sidebar-footer">
        <div class="sidebar-theme-row">
          <span>主题</span>
          <ThemeToggle class="workspace-theme-toggle" />
        </div>

        <div class="credits-section">
          <div class="credits-info">
            <span class="credits-label">剩余点数</span>
            <span class="credits-value">{{ credits.toLocaleString() }}</span>
          </div>
          <button class="recharge-button" @click="handleRecharge">
            <CreditCard :size="16" />
            <span>充值</span>
          </button>
        </div>

        <div v-if="!isLoggedIn" class="sidebar-auth-actions">
          <button
            class="sidebar-auth-button"
            data-testid="site-login-link"
            type="button"
            @click="handleAuthRoute('/login')"
          >
            <LogIn :size="16" />
            <span>登录</span>
          </button>
          <button
            class="sidebar-auth-button sidebar-auth-button-primary"
            data-testid="site-register-link"
            type="button"
            @click="handleAuthRoute('/register')"
          >
            <UserPlus :size="16" />
            <span>注册</span>
          </button>
        </div>

        <div v-else ref="userMenuRoot" class="user-menu-wrap" @mouseenter="openUserMenu">
          <button
            ref="userMenuTrigger"
            class="user-section user-menu-trigger"
            :data-testid="userMenuToggleTestId"
            type="button"
            :aria-label="`打开 ${userName} 的账户菜单`"
            :aria-expanded="String(userMenuOpen)"
            aria-controls="workspace-user-menu"
            @mouseenter="openUserMenu"
            @click="toggleUserMenu"
          >
            <div class="user-avatar" aria-hidden="true">
              <span>{{ userAvatarLabel }}</span>
            </div>
            <div class="user-info">
              <div class="user-name">{{ userName }}</div>
              <div class="user-tier">{{ userTier }} 用户</div>
            </div>
            <ChevronRight :size="16" class="user-menu-chevron" aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  </SoftPanel>

  <Teleport to="body">
    <div
      v-if="userMenuOpen"
      id="workspace-user-menu"
      ref="userMenuPanel"
      :class="['workspace-user-menu', userMenuThemeClass]"
      :style="userMenuStyle"
      :data-testid="userMenuTestId"
      role="menu"
      tabindex="-1"
      @keydown="handleUserMenuKeydown"
    >
      <div class="workspace-user-menu-head">
        <div class="workspace-user-menu-avatar" aria-hidden="true">
          {{ userAvatarLabel }}
        </div>
        <div>
          <strong>{{ userName }}</strong>
          <span>{{ isLoggedIn ? userId : '登录后查看账户权益' }}</span>
        </div>
      </div>

      <div class="workspace-user-menu-meta">
        <span>{{ userTier }}</span>
        <strong>{{ credits.toLocaleString() }} 点</strong>
      </div>

      <div class="workspace-user-menu-list">
        <button
          v-for="item in userMenuItems"
          :key="item.id"
          class="workspace-user-menu-item"
          :data-testid="`workspace-user-menu-${item.id}`"
          type="button"
          role="menuitem"
          @click="handleUserMenuItem(item)"
        >
          <component :is="item.icon" :size="16" />
          <span>{{ item.label }}</span>
          <ChevronRight :size="14" aria-hidden="true" />
        </button>
      </div>

      <button
        v-if="isLoggedIn"
        class="workspace-user-menu-item workspace-user-menu-logout"
        :data-testid="logoutButtonTestId"
        type="button"
        role="menuitem"
        @click="handleUserLogout"
      >
        <LogOut :size="16" />
        <span>退出登录</span>
        <ChevronRight :size="14" aria-hidden="true" />
      </button>
    </div>
  </Teleport>
</template>
