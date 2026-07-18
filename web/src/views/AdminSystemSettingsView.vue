<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Activity,
  Bell,
  CheckCircle2,
  Cloud,
  Database,
  Download,
  Globe2,
  HardDrive,
  Image,
  ListChecks,
  RotateCcw,
  Save,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  Users,
  Wand2,
  X
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const tabs = [
  { id: 'platform', label: '基础设置', icon: Globe2 },
  { id: 'storage', label: '存储与 CDN', icon: Cloud },
  { id: 'generation', label: '生成策略', icon: Wand2 },
  { id: 'notifications', label: '消息通知', icon: Bell },
  { id: 'security', label: '安全权限', icon: ShieldCheck }
]
const timezoneOptions = [
  { value: 'Asia/Shanghai', label: 'Asia/Shanghai' },
  { value: 'UTC', label: 'UTC' },
  { value: 'America/Los_Angeles', label: 'America/Los_Angeles' }
]
const languageOptions = [
  { value: 'zh-CN', label: '简体中文' },
  { value: 'en-US', label: 'English' }
]
const currencyOptions = [
  { value: 'CNY', label: 'CNY' },
  { value: 'USD', label: 'USD' }
]
const aspectRatioOptions = [
  { value: '21:9', label: '21:9' },
  { value: '16:9', label: '16:9' },
  { value: '4:3', label: '4:3' },
  { value: '3:2', label: '3:2' },
  { value: '1:1', label: '1:1' },
  { value: '2:3', label: '2:3' },
  { value: '3:4', label: '3:4' },
  { value: '9:16', label: '9:16' },
  { value: '9:21', label: '9:21' }
]
const reviewPolicyOptions = [
  { value: 'standard', label: '标准审核' },
  { value: 'manual', label: '人工复核' },
  { value: 'auto', label: '自动审核' },
  { value: 'off', label: '关闭' }
]
const storageModeOptions = [
  { value: 'local', label: '本地存储' },
  { value: 'object', label: '对象存储' }
]
const loginPolicyOptions = [
  { value: 'standard', label: '标准' },
  { value: 'strict', label: '严格' },
  { value: 'relaxed', label: '宽松' }
]

const activeTab = ref('platform')
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const errorMessage = ref('')
const updatedAt = ref('')

function blankSettings() {
  return {
    platform: {
      name: '',
      short_name: '',
      logo_url: '',
      timezone: 'Asia/Shanghai',
      language: 'zh-CN',
      currency: 'CNY',
      icp_record_number: '',
      platform_domain: ''
    },
    storage: {
      storage_mode: 'local',
      provider: 'local',
      region: '',
      bucket: '',
      cdn_domain: '',
      cdn_acceleration: false
    },
    generation: {
      upload_limit: 6,
      default_aspect_ratio: '1:1',
      retention_days: 30,
      concurrency_limit: 4,
      review_policy: 'standard',
      negative_prompt_enabled: true,
      advanced_parameters_enabled: true
    },
    notifications: {
      notification_email: '',
      task_complete_notice: true,
      system_alert_notice: true,
      daily_summary_notice: false,
      webhook_url: ''
    },
    security: {
      login_policy: 'standard',
      password_min_length: 8,
      two_factor_enabled: false,
      failed_login_lock_enabled: true,
      admin_permission_management_enabled: true
    }
  }
}

function blankStatus() {
  return {
    runtime_status: '-',
    database_status: '-',
    version: '-',
    started_at: '',
    storage_mode: '-',
    storage_provider: '-',
    storage_bucket: '-',
    storage_used_bytes: 0,
    storage_capacity_bytes: 0,
    cdn_status: '-',
    cdn_traffic_bytes: 0,
    cdn_traffic_limit_bytes: 0,
    today_generations: 0,
    daily_generation_limit: 0,
    queue_status: {
      queued: 0,
      running: 0
    },
    payment: {
      alipay: blankAlipayStatus()
    },
    total_users: 0,
    total_works: 0,
    total_generations: 0
  }
}

function blankAlipayStatus() {
  return {
    configured: false,
    sandbox: false,
    gateway: '',
    notify_url: '',
    return_url_base: '',
    missing: [],
    items: []
  }
}

const form = reactive(blankSettings())
const defaults = ref(blankSettings())
const status = reactive(blankStatus())

const formattedUpdatedAt = computed(() => {
  if (!updatedAt.value) {
    return '尚未保存'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(new Date(updatedAt.value))
})

const formattedStartedAt = computed(() => {
  if (!status.started_at) {
    return '-'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(new Date(status.started_at))
})

const queueStatus = computed(() => ({
  queued: Number(status.queue_status?.queued ?? 0),
  running: Number(status.queue_status?.running ?? 0)
}))

const alipayStatus = computed(() => ({
  ...blankAlipayStatus(),
  ...(status.payment?.alipay ?? {}),
  items: Array.isArray(status.payment?.alipay?.items) ? status.payment.alipay.items : [],
  missing: Array.isArray(status.payment?.alipay?.missing) ? status.payment.alipay.missing : []
}))

const alipayModeText = computed(() => (alipayStatus.value.sandbox ? '沙箱联调' : '正式环境'))

const statusCards = computed(() => [
  { label: '运行状态', value: status.runtime_status === 'running' ? '运行中' : status.runtime_status, icon: Activity },
  { label: '数据库', value: status.database_status === 'connected' ? '已连接' : status.database_status, icon: Database },
  { label: '用户数', value: formatNumber(status.total_users), icon: Users },
  { label: '作品数', value: formatNumber(status.total_works), icon: Image },
  { label: '生成总量', value: formatNumber(status.total_generations), icon: Wand2 }
])

const usageCards = computed(() => [
  {
    label: '存储用量',
    value: formatByteLimit(status.storage_used_bytes, status.storage_capacity_bytes),
    configured: Number(status.storage_capacity_bytes) > 0,
    percent: usagePercent(status.storage_used_bytes, status.storage_capacity_bytes),
    icon: HardDrive
  },
  {
    label: 'CDN 流量',
    value: formatByteLimit(status.cdn_traffic_bytes, status.cdn_traffic_limit_bytes),
    configured: Number(status.cdn_traffic_limit_bytes) > 0,
    percent: usagePercent(status.cdn_traffic_bytes, status.cdn_traffic_limit_bytes),
    icon: Cloud
  },
  {
    label: '今日生成',
    value: formatLimit(status.today_generations, status.daily_generation_limit),
    configured: Number(status.daily_generation_limit) > 0,
    percent: usagePercent(status.today_generations, status.daily_generation_limit),
    icon: ListChecks
  }
])

function clone(value) {
  return JSON.parse(JSON.stringify(value ?? blankSettings()))
}

function assignSettings(target, source = {}) {
  Object.keys(blankSettings()).forEach((section) => {
    Object.assign(target[section], source[section] ?? {})
  })
}

function assignStatus(source = {}) {
  Object.assign(status, blankStatus(), source)
  status.queue_status = {
    queued: Number(source.queue_status?.queued ?? 0),
    running: Number(source.queue_status?.running ?? 0)
  }
  status.payment = {
    alipay: {
      ...blankAlipayStatus(),
      ...(source.payment?.alipay ?? {}),
      items: Array.isArray(source.payment?.alipay?.items) ? source.payment.alipay.items : [],
      missing: Array.isArray(source.payment?.alipay?.missing) ? source.payment.alipay.missing : []
    }
  }
}

function setActiveSection(id) {
  activeTab.value = id
  const element = globalThis.document?.getElementById(`system-section-${id}`)
  element?.scrollIntoView?.({ block: 'start', behavior: 'smooth' })
}

function formatNumber(value) {
  return Number(value ?? 0).toLocaleString('zh-CN')
}

function formatBytes(value) {
  const bytes = Number(value ?? 0)
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = bytes
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  const formatted = size >= 10 || Number.isInteger(size) ? String(Math.round(size)) : size.toFixed(1)
  return `${formatted} ${units[unit]}`
}

function formatByteLimit(used, limit) {
  const limitNumber = Number(limit ?? 0)
  if (limitNumber <= 0) {
    return `${formatBytes(used)} / 未配置`
  }
  return `${formatBytes(used)} / ${formatBytes(limitNumber)}`
}

function formatLimit(used, limit) {
  const limitNumber = Number(limit ?? 0)
  if (limitNumber <= 0) {
    return `${formatNumber(used)} / 未配置`
  }
  return `${formatNumber(used)} / ${formatNumber(limitNumber)}`
}

function usagePercent(used, limit) {
  const usedNumber = Number(used ?? 0)
  const limitNumber = Number(limit ?? 0)
  if (limitNumber <= 0) {
    return 0
  }
  return Math.max(0, Math.min(100, Math.round((usedNumber / limitNumber) * 100)))
}

function queueSummary() {
  return `${formatNumber(queueStatus.value.queued)} 等待 / ${formatNumber(queueStatus.value.running)} 运行`
}

function configStateText(configured) {
  return configured ? '已配置' : '缺失'
}

async function loadSettings() {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await api.getSystemSettings()
    assignSettings(form, data.settings)
    defaults.value = clone(data.defaults)
    assignStatus(data.status)
    updatedAt.value = data.updated_at ?? ''
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

function payload() {
  return clone(form)
}

async function saveSettings() {
  saving.value = true
  message.value = ''
  errorMessage.value = ''
  try {
    const data = await api.updateSystemSettings(payload())
    if (data.settings) {
      assignSettings(form, data.settings)
    }
    message.value = '系统设置已保存'
    updatedAt.value = new Date().toISOString()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

function restoreDefaults() {
  assignSettings(form, defaults.value)
  message.value = '已恢复默认值，保存后生效'
  errorMessage.value = ''
}

function exportSettings() {
  globalThis.open?.(api.systemSettingsExportURL(), 'system-settings-export')
}

function placeholderAction() {
  message.value = '暂未接入'
  errorMessage.value = ''
}

onMounted(loadSettings)
</script>

<template>
  <section class="system-settings-page">
    <div class="admin-page-heading system-settings-heading">
      <div>
        <p class="eyebrow">管理用户、套餐、模型与生成业务 / 系统设置</p>
        <h1>系统设置</h1>
        <span>配置平台运行参数、功能策略与权限管理</span>
      </div>
      <div class="system-heading-actions">
        <button class="mini-button compact-button" type="button" data-testid="export-system-settings" @click="exportSettings">
          <Download :size="16" />
          导出配置
        </button>
      </div>
    </div>

    <div v-if="message" class="settings-alert success">
      <CheckCircle2 :size="16" />
      <span>{{ message }}</span>
    </div>
    <div v-if="errorMessage" class="settings-alert error">
      <X :size="16" />
      <span>{{ errorMessage }}</span>
    </div>

    <div v-if="loading" class="admin-panel page-status">加载中...</div>

    <div v-else class="system-settings-workspace">
        <div class="system-tabs" role="tablist" aria-label="系统设置分组">
          <button
            v-for="tab in tabs"
            :key="tab.id"
            class="system-tab"
            :class="{ active: activeTab === tab.id }"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.id"
            :data-testid="`tab-${tab.id}`"
            @click="setActiveSection(tab.id)"
          >
            <component :is="tab.icon" :size="16" />
            <span>{{ tab.label }}</span>
          </button>
        </div>

        <div class="system-settings-layout">
        <section id="system-section-platform" class="admin-panel system-section-card" :class="{ active: activeTab === 'platform' }">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Platform</p>
              <h2>平台信息</h2>
            </div>
            <Globe2 :size="20" />
          </div>
          <div class="system-current-strip">
            <strong>{{ form.platform.name || '-' }}</strong>
            <span>{{ form.platform.platform_domain || '未配置域名' }}</span>
          </div>
          <div class="system-form-grid">
            <label>
              <span>平台名称</span>
              <input v-model="form.platform.name" class="text-input" data-testid="platform-name" />
            </label>
            <label>
              <span>简称</span>
              <input v-model="form.platform.short_name" class="text-input" />
            </label>
            <label>
              <span>Logo URL</span>
              <input v-model="form.platform.logo_url" class="text-input" />
            </label>
            <label>
              <span>平台域名</span>
              <input v-model="form.platform.platform_domain" class="text-input" />
            </label>
            <label>
              <span>时区</span>
              <ClickSelect v-model="form.platform.timezone" :options="timezoneOptions" class="select-input" aria-label="时区" />
            </label>
            <label>
              <span>语言</span>
              <ClickSelect v-model="form.platform.language" :options="languageOptions" class="select-input" aria-label="语言" />
            </label>
            <label>
              <span>币种</span>
              <ClickSelect v-model="form.platform.currency" :options="currencyOptions" class="select-input" aria-label="币种" />
            </label>
            <label>
              <span>ICP备案号</span>
              <input v-model="form.platform.icp_record_number" class="text-input" />
            </label>
          </div>
        </section>

        <section id="system-section-generation" class="admin-panel system-section-card" :class="{ active: activeTab === 'generation' }">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Generation</p>
              <h2>生成策略</h2>
            </div>
            <Wand2 :size="20" />
          </div>
          <div class="system-form-grid">
            <label>
              <span>上传数量上限</span>
              <input v-model.number="form.generation.upload_limit" class="text-input" type="number" min="1" max="20" />
            </label>
            <label>
              <span>默认画幅</span>
              <ClickSelect v-model="form.generation.default_aspect_ratio" :options="aspectRatioOptions" class="select-input" aria-label="默认画幅" />
            </label>
            <label>
              <span>图片保留天数</span>
              <input v-model.number="form.generation.retention_days" class="text-input" type="number" min="1" />
            </label>
            <label>
              <span>并发限制</span>
              <input v-model.number="form.generation.concurrency_limit" class="text-input" type="number" min="1" max="100" />
            </label>
            <label>
              <span>审核策略</span>
              <ClickSelect v-model="form.generation.review_policy" :options="reviewPolicyOptions" class="select-input" aria-label="审核策略" />
            </label>
            <label class="system-switch-row">
              <input v-model="form.generation.negative_prompt_enabled" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>允许负面提示词</strong><small>创作台高级输入项</small></span>
            </label>
            <label class="system-switch-row system-wide-field">
              <input v-model="form.generation.advanced_parameters_enabled" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>开放高级参数</strong><small>风格强度、参考权重等入口</small></span>
            </label>
          </div>
        </section>

        <section class="admin-panel system-status-panel">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Status</p>
              <h2>系统状态</h2>
            </div>
            <Database :size="20" />
          </div>
          <div class="system-status-grid">
            <article v-for="card in statusCards" :key="card.label">
              <component :is="card.icon" :size="17" />
              <span>{{ card.label }}</span>
              <strong>{{ card.value }}</strong>
            </article>
          </div>
          <div class="system-usage-stack">
            <article v-for="card in usageCards" :key="card.label" class="system-usage-card">
              <div>
                <span><component :is="card.icon" :size="15" />{{ card.label }}</span>
                <strong>{{ card.value }}</strong>
              </div>
              <div class="system-progress" :class="{ muted: !card.configured }">
                <span :style="{ width: `${card.percent}%` }"></span>
              </div>
            </article>
            <article class="system-usage-card">
              <div>
                <span><ListChecks :size="15" />队列</span>
                <strong>{{ queueSummary() }}</strong>
              </div>
              <div class="system-queue-bars">
                <span></span>
                <span></span>
              </div>
            </article>
          </div>
          <div class="system-payment-card" data-testid="alipay-config-status">
            <div class="system-payment-card-head">
              <div>
                <span>支付宝支付</span>
                <strong :class="{ missing: !alipayStatus.configured }">{{ configStateText(alipayStatus.configured) }}</strong>
              </div>
              <small>{{ alipayModeText }}</small>
            </div>
            <div class="system-config-items">
              <div v-for="item in alipayStatus.items" :key="item.key">
                <span>{{ item.key }}</span>
                <strong :class="{ missing: !item.configured }">{{ configStateText(item.configured) }}</strong>
              </div>
            </div>
            <dl class="system-payment-urls">
              <div>
                <dt>notify_url</dt>
                <dd>{{ alipayStatus.notify_url || '-' }}</dd>
              </div>
              <div>
                <dt>return_url</dt>
                <dd>{{ alipayStatus.return_url_base || '-' }}</dd>
              </div>
            </dl>
          </div>
          <dl class="system-status-list">
            <div><dt>版本</dt><dd>{{ status.version }}</dd></div>
            <div><dt>启动</dt><dd>{{ formattedStartedAt }}</dd></div>
            <div><dt>存储</dt><dd>{{ status.storage_provider }} / {{ status.storage_bucket || '-' }}</dd></div>
            <div><dt>CDN</dt><dd>{{ status.cdn_status === 'enabled' ? '已启用' : '未启用' }}</dd></div>
            <div><dt>最近保存</dt><dd>{{ formattedUpdatedAt }}</dd></div>
          </dl>
        </section>

        <section id="system-section-storage" class="admin-panel system-section-card" :class="{ active: activeTab === 'storage' }">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Storage</p>
              <h2>存储与 CDN</h2>
            </div>
            <Cloud :size="20" />
          </div>
          <div class="system-form-grid">
            <label>
              <span>存储方式</span>
              <ClickSelect v-model="form.storage.storage_mode" :options="storageModeOptions" class="select-input" aria-label="存储方式" />
            </label>
            <label>
              <span>Provider</span>
              <input v-model="form.storage.provider" class="text-input" data-testid="storage-provider" />
            </label>
            <label>
              <span>区域</span>
              <input v-model="form.storage.region" class="text-input" />
            </label>
            <label>
              <span>Bucket</span>
              <input v-model="form.storage.bucket" class="text-input" />
            </label>
            <label class="system-wide-field">
              <span>CDN 域名</span>
              <input v-model="form.storage.cdn_domain" class="text-input" />
            </label>
            <label class="system-switch-row system-wide-field">
              <input v-model="form.storage.cdn_acceleration" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>开启 CDN 加速</strong><small>云厂商侧仍需单独配置</small></span>
            </label>
          </div>
        </section>

        <section id="system-section-notifications" class="admin-panel system-section-card" :class="{ active: activeTab === 'notifications' }">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Notice</p>
              <h2>消息通知</h2>
            </div>
            <Bell :size="20" />
          </div>
          <div class="system-form-grid">
            <label>
              <span>通知邮箱</span>
              <input v-model="form.notifications.notification_email" class="text-input" type="email" />
            </label>
            <label>
              <span>Webhook URL</span>
              <input v-model="form.notifications.webhook_url" class="text-input" />
            </label>
            <label class="system-switch-row">
              <input v-model="form.notifications.task_complete_notice" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>任务完成通知</strong><small>生成完成后发送</small></span>
            </label>
            <label class="system-switch-row">
              <input v-model="form.notifications.system_alert_notice" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>系统告警</strong><small>异常和失败率升高</small></span>
            </label>
            <label class="system-switch-row system-wide-field">
              <input v-model="form.notifications.daily_summary_notice" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>每日汇总</strong><small>平台运行数据摘要</small></span>
            </label>
          </div>
        </section>

        <section class="admin-panel system-actions-panel">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Actions</p>
              <h2>快捷操作</h2>
            </div>
            <SlidersHorizontal :size="20" />
          </div>
          <button class="mini-button compact-button" type="button" data-testid="clean-temp-files" @click="placeholderAction">
            <Trash2 :size="16" />
            清理临时文件
          </button>
          <button class="mini-button compact-button" type="button" data-testid="rebuild-thumbnails" @click="placeholderAction">
            <Image :size="16" />
            重建缩略图
          </button>
          <button class="mini-button compact-button" type="button" data-testid="quick-export-system-settings" @click="exportSettings">
            <Download :size="16" />
            导出配置
          </button>
        </section>

        <section id="system-section-security" class="admin-panel system-section-card system-section-card-wide" :class="{ active: activeTab === 'security' }">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Security</p>
              <h2>安全权限</h2>
            </div>
            <ShieldCheck :size="20" />
          </div>
          <div class="system-form-grid">
            <label>
              <span>登录安全策略</span>
              <ClickSelect v-model="form.security.login_policy" :options="loginPolicyOptions" class="select-input" aria-label="登录安全策略" />
            </label>
            <label>
              <span>密码最小长度</span>
              <input v-model.number="form.security.password_min_length" class="text-input" type="number" min="8" max="64" />
            </label>
            <label class="system-switch-row">
              <input v-model="form.security.two_factor_enabled" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>启用 2FA</strong><small>管理员二次验证</small></span>
            </label>
            <label class="system-switch-row">
              <input v-model="form.security.failed_login_lock_enabled" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>登录失败锁定</strong><small>连续失败后保护账户</small></span>
            </label>
            <label class="system-switch-row system-wide-field">
              <input v-model="form.security.admin_permission_management_enabled" type="checkbox" />
              <span class="system-switch" aria-hidden="true"></span>
              <span><strong>管理员权限管理</strong><small>后台角色与权限维护入口</small></span>
            </label>
          </div>
        </section>
      </div>
    </div>

    <div class="system-savebar" data-testid="system-settings-savebar">
      <div>
        <strong>最近保存：{{ formattedUpdatedAt }}</strong>
        <span>安全权限与运行参数会立即影响后台行为。</span>
      </div>
      <div class="system-savebar-actions">
        <button class="secondary-button compact-button" type="button" data-testid="restore-system-defaults" :disabled="saving || loading" @click="restoreDefaults">
          <RotateCcw :size="16" />
          恢复默认
        </button>
        <button class="primary-button" type="button" data-testid="save-system-settings" :disabled="saving || loading" @click="saveSettings">
          <Save :size="16" />
          {{ saving ? '保存中...' : '保存设置' }}
        </button>
      </div>
    </div>
  </section>
</template>
