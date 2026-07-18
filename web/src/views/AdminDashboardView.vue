<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  Activity,
  Bell,
  CircleDollarSign,
  Image,
  Megaphone,
  Package,
  RefreshCw,
  Ticket,
  Users
} from 'lucide-vue-next'

import { api } from '../api/client.js'

const dashboard = ref(null)
const errorMessage = ref('')
const loading = ref(false)
const bottomTab = ref('announcements')

const kpis = computed(() => {
  const payload = dashboard.value ?? {}
  return payload.kpis ?? {
    users_total: payload.users_total ?? 0,
    works_total: payload.works_total ?? 0,
    generation_total: payload.generation_total ?? 0,
    generation_succeeded: payload.generation_succeeded ?? 0,
    generation_failed: payload.generation_failed ?? 0,
    packages_active: payload.packages_active ?? 0,
    invites_active: payload.invites_active ?? 0,
    revenue_completed: '￥0.00'
  }
})

const kpiCards = computed(() => [
  { label: '用户数', value: formatNumber(kpis.value.users_total), icon: Users, tone: 'blue' },
  { label: '累计生成', value: formatNumber(kpis.value.generation_total), icon: Image, tone: 'green' },
  { label: '成功次数', value: formatNumber(kpis.value.generation_succeeded), icon: Activity, tone: 'mint' },
  { label: '已完成收入', value: kpis.value.revenue_completed ?? '￥0.00', icon: CircleDollarSign, tone: 'red' }
])

const trendPoints = computed(() => {
  const points = dashboard.value?.generation_trend ?? []
  if (points.length === 0) {
    return ''
  }
  const maxValue = Math.max(...points.map((point) => Number(point.total ?? 0)), 1)
  const last = points.length - 1
  return points.map((point, index) => {
    const x = last === 0 ? 0 : (index / last) * 100
    const y = 100 - (Number(point.total ?? 0) / maxValue) * 86 - 7
    return `${x.toFixed(2)},${y.toFixed(2)}`
  }).join(' ')
})

const trendBars = computed(() => dashboard.value?.generation_trend ?? [])
const packages = computed(() => dashboard.value?.packages ?? [])
const models = computed(() => dashboard.value?.models ?? [])
const inviteSummary = computed(() => dashboard.value?.invite_summary ?? {})
const recentGenerations = computed(() => dashboard.value?.recent_generations ?? [])
const announcements = computed(() => dashboard.value?.announcements ?? [])
const operationLogs = computed(() => dashboard.value?.operation_logs ?? [])

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    dashboard.value = await api.getDashboard()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

async function setAnnouncementStatus(id, status) {
  errorMessage.value = ''
  try {
    await api.updateAnnouncementStatus(id, status)
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(Number(value ?? 0))
}

function formatDate(value) {
  if (!value) {
    return '-'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))
}

function statusText(status) {
  const map = {
    active: '启用',
    disabled: '停用',
    submitted: '待处理',
    processing: '处理中',
    completed: '已完成',
    succeeded: '成功',
    failed: '失败',
    queued: '排队中',
    running: '运行中',
    published: '已发布',
    offline: '已下线'
  }
  return map[status] ?? status ?? '-'
}

function levelText(level) {
  const map = {
    info: '普通',
    important: '重要',
    warning: '警示'
  }
  return map[level] ?? level ?? '-'
}

onMounted(load)
</script>

<template>
  <section class="admin-dashboard-page">
    <div class="admin-page-heading dashboard-heading">
      <div>
        <p class="eyebrow">Overview</p>
        <h1>运行概览</h1>
      </div>
      <div class="dashboard-heading-actions">
        <button class="secondary-button icon-button-text" type="button" @click="load">
          <RefreshCw :size="16" />
          刷新
        </button>
        <RouterLink class="primary-button icon-button-text" to="/admin/announcements" data-testid="open-announcement-page">
          <Megaphone :size="16" />
          公告通知
        </RouterLink>
      </div>
    </div>

    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    <p v-if="loading && !dashboard" class="page-status">加载中...</p>

    <template v-if="dashboard">
      <div class="dashboard-kpi-grid">
        <article v-for="item in kpiCards" :key="item.label" class="dashboard-kpi-card" :class="`dashboard-kpi-${item.tone}`">
          <span class="dashboard-kpi-icon">
            <component :is="item.icon" :size="18" />
          </span>
          <div>
            <p>{{ item.label }}</p>
            <strong>{{ item.value }}</strong>
          </div>
        </article>
      </div>

      <div class="dashboard-business-grid">
        <article class="admin-panel dashboard-panel">
          <div class="panel-title-row">
            <h2>套餐配置</h2>
            <Package :size="18" />
          </div>
          <div class="dashboard-compact-list">
            <div v-for="pkg in packages" :key="pkg.id" class="dashboard-list-row">
              <div>
                <strong>{{ pkg.name }}</strong>
                <span>{{ pkg.credits }} 点 · {{ pkg.price_label }}</span>
              </div>
              <b :class="{ muted: !pkg.is_active }">{{ pkg.is_active ? pkg.badge || '启用' : '停用' }}</b>
            </div>
          </div>
        </article>

        <article class="admin-panel dashboard-panel">
          <div class="panel-title-row">
            <h2>模型配置</h2>
            <Activity :size="18" />
          </div>
          <div class="dashboard-compact-list">
            <div v-for="model in models" :key="model.name" class="dashboard-list-row">
              <div>
                <strong>{{ model.name }}</strong>
                <span>{{ model.request_timeout_seconds }} 秒超时</span>
              </div>
              <b :class="{ active: model.active }">{{ model.active ? '当前' : '可用' }}</b>
            </div>
          </div>
        </article>
      </div>

      <div class="dashboard-main-grid">
        <article class="admin-panel dashboard-trend-panel">
          <div class="panel-title-row">
            <h2>近 30 天生成趋势</h2>
            <span class="dashboard-panel-meta">总量 / 成功 / 失败</span>
          </div>
          <svg class="dashboard-trend-chart" data-testid="dashboard-trend-chart" viewBox="0 0 100 100" preserveAspectRatio="none" role="img" aria-label="近 30 天生成趋势">
            <defs>
              <linearGradient id="dashboardTrendFill" x1="0" x2="0" y1="0" y2="1">
                <stop offset="0%" stop-color="#2563eb" stop-opacity="0.28" />
                <stop offset="100%" stop-color="#2563eb" stop-opacity="0" />
              </linearGradient>
            </defs>
            <polyline v-if="trendPoints" :points="`0,100 ${trendPoints} 100,100`" fill="url(#dashboardTrendFill)" stroke="none" />
            <polyline v-if="trendPoints" :points="trendPoints" fill="none" stroke="#2563eb" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
          </svg>
          <div class="dashboard-trend-bars">
            <span v-for="point in trendBars.slice(-12)" :key="point.date" :style="{ '--bar': `${Math.max(8, Math.min(100, Number(point.total || 0) * 18))}%` }" :title="`${point.date}: ${point.total}`"></span>
          </div>
        </article>

        <aside class="dashboard-side-stack">
          <article class="admin-panel dashboard-invite-card">
            <div class="panel-title-row">
              <h2>邀请码</h2>
              <Ticket :size="18" />
            </div>
            <strong>{{ formatNumber(inviteSummary.remaining) }}</strong>
            <div class="dashboard-invite-meta">
              <span>启用 {{ formatNumber(inviteSummary.active) }}</span>
              <span>已用 {{ formatNumber(inviteSummary.used) }} / {{ formatNumber(inviteSummary.total) }}</span>
            </div>
          </article>

          <article class="admin-panel dashboard-recent-card">
            <div class="panel-title-row">
              <h2>最近生成</h2>
              <Image :size="18" />
            </div>
            <div class="dashboard-generation-list">
              <div v-for="record in recentGenerations.slice(0, 4)" :key="record.id" class="dashboard-generation-item">
                <img v-if="record.preview_url" :src="record.preview_url" alt="" />
                <div v-else class="dashboard-thumb-placeholder"></div>
                <div>
                  <strong>{{ record.prompt || '未填写提示词' }}</strong>
                  <span>{{ record.model }} · {{ statusText(record.status) }}</span>
                </div>
              </div>
            </div>
          </article>
        </aside>
      </div>

      <article class="admin-panel dashboard-bottom-panel">
        <div class="dashboard-tabs">
          <button type="button" :class="{ active: bottomTab === 'announcements' }" @click="bottomTab = 'announcements'">
            <Bell :size="16" />
            系统公告
          </button>
          <button type="button" :class="{ active: bottomTab === 'logs' }" @click="bottomTab = 'logs'">
            <Activity :size="16" />
            操作日志
          </button>
        </div>

        <div v-if="bottomTab === 'announcements'" class="dashboard-announcement-list">
          <div v-for="announcement in announcements" :key="announcement.id" class="dashboard-announcement-item">
            <div>
              <strong>{{ announcement.title }}</strong>
              <p>{{ announcement.content }}</p>
              <span>{{ levelText(announcement.level) }} · {{ statusText(announcement.status) }} · {{ formatDate(announcement.created_at) }}</span>
            </div>
            <button v-if="announcement.status === 'published'" class="secondary-button compact-button" type="button" @click="setAnnouncementStatus(announcement.id, 'offline')">下线</button>
          </div>
        </div>

        <div v-else class="admin-table-scroll">
          <table class="data-table admin-data-table dashboard-log-table">
            <thead>
              <tr>
                <th>操作</th>
                <th>对象</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="log in operationLogs" :key="log.id">
                <td>{{ log.action }}</td>
                <td>{{ log.target_type }} #{{ log.target_id }}</td>
                <td>{{ formatDate(log.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </article>
    </template>
  </section>
</template>
