<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  Activity,
  Cpu,
  HardDrive,
  MemoryStick,
  RefreshCw
} from 'lucide-vue-next'

import { api } from '../api/client.js'

const resources = ref(null)
const errorMessage = ref('')
const loading = ref(false)

const cpuCard = computed(() => {
  const cpu = resources.value?.cpu ?? {}
  return {
    usage_percent: cpu.usage_percent ?? 0,
    cores: cpu.cores ?? 0,
    load_average: cpu.load_average ?? []
  }
})

const memoryCard = computed(() => {
  const mem = resources.value?.memory ?? {}
  return {
    usage_percent: mem.usage_percent ?? 0,
    total_bytes: mem.total_bytes ?? 0,
    used_bytes: mem.used_bytes ?? 0,
    available_bytes: mem.available_bytes ?? 0
  }
})

const diskCard = computed(() => {
  const disk = resources.value?.disk ?? {}
  return {
    usage_percent: disk.usage_percent ?? 0,
    total_bytes: disk.total_bytes ?? 0,
    used_bytes: disk.used_bytes ?? 0,
    free_bytes: disk.free_bytes ?? 0
  }
})

const processes = computed(() => resources.value?.processes ?? [])
const generationQueue = computed(() => resources.value?.generation ?? {
  queued: 0,
  running: 0,
  retry_waiting: 0,
  oldest_queue_age_ms: 0,
  concurrency_limit: 4,
  used_slots: 0,
  queue_wait_p95_ms: 0,
  provider_latency_p95_ms: 0,
  provider_429_rate: 0,
  failure_rate: 0
})

const sampledTime = computed(() => {
  const at = resources.value?.sampled_at
  if (!at) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  }).format(new Date(at))
})

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.floor(Math.log(bytes) / Math.log(1024))
  const value = bytes / Math.pow(1024, Math.min(index, units.length - 1))
  const formatted = value >= 10 || Number.isInteger(value) ? String(Math.round(value)) : value.toFixed(1)
  return `${formatted} ${units[Math.min(index, units.length - 1)]}`
}

function formatActiveBreakdown(values) {
  const entries = Object.entries(values ?? {})
  if (entries.length === 0) return '-'
  return entries.map(([name, count]) => `${name}: ${count}`).join(' / ')
}

function statusText(status) {
  const map = {
    running: '运行中',
    sleeping: '休眠',
    waiting: '等待中',
    zombie: '僵尸',
    stopped: '已停止',
    idle: '空闲',
    unknown: '未知'
  }
  return map[status] ?? status ?? '-'
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    resources.value = await api.getSystemResources()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="admin-system-resources-page">
    <div class="admin-page-heading dashboard-heading">
      <div>
        <p class="eyebrow">System Resources</p>
        <h1>资源监控</h1>
      </div>
      <div class="dashboard-heading-actions">
        <span class="dashboard-panel-meta" style="margin-right:12px">最近采样 {{ sampledTime }}</span>
        <button class="secondary-button icon-button-text" type="button" data-testid="system-resources-refresh" @click="load">
          <RefreshCw :size="16" />
          刷新
        </button>
      </div>
    </div>

    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    <p v-if="loading && !resources" class="page-status">加载中...</p>

    <template v-if="resources">
      <div class="dashboard-kpi-grid">
        <article class="dashboard-kpi-card dashboard-kpi-blue">
          <span class="dashboard-kpi-icon">
            <Cpu :size="18" />
          </span>
          <div>
            <p>CPU 使用率</p>
            <strong>{{ cpuCard.usage_percent }}%</strong>
          </div>
        </article>
        <article class="dashboard-kpi-card dashboard-kpi-green">
          <span class="dashboard-kpi-icon">
            <MemoryStick :size="18" />
          </span>
          <div>
            <p>内存使用</p>
            <strong>{{ formatBytes(memoryCard.used_bytes) }} / {{ formatBytes(memoryCard.total_bytes) }}</strong>
          </div>
        </article>
        <article class="dashboard-kpi-card dashboard-kpi-mint">
          <span class="dashboard-kpi-icon">
            <HardDrive :size="18" />
          </span>
          <div>
            <p>磁盘使用</p>
            <strong>{{ formatBytes(diskCard.used_bytes) }} / {{ formatBytes(diskCard.total_bytes) }}</strong>
          </div>
        </article>
        <article class="dashboard-kpi-card dashboard-kpi-red">
          <span class="dashboard-kpi-icon">
            <Activity :size="18" />
          </span>
          <div>
            <p>运行进程</p>
            <strong>{{ processes.length }}</strong>
          </div>
        </article>
        <article class="dashboard-kpi-card dashboard-kpi-blue" data-testid="generation-queue-kpi">
          <span class="dashboard-kpi-icon"><Activity :size="18" /></span>
          <div>
            <p>图片生成队列</p>
            <strong>{{ generationQueue.queued }} 排队 · {{ generationQueue.running }} 执行</strong>
          </div>
        </article>
      </div>

      <article class="admin-panel dashboard-bottom-panel" data-testid="generation-queue-panel">
        <div class="panel-title-row">
          <h2>图片生成容量与稳定性</h2>
          <span class="dashboard-panel-meta">槽位 {{ generationQueue.used_slots }} / {{ generationQueue.concurrency_limit }}</span>
        </div>
        <div class="dashboard-compact-list">
          <div class="dashboard-list-row"><div><strong>排队 / 重试等待</strong></div><b>{{ generationQueue.queued }} / {{ generationQueue.retry_waiting }}</b></div>
          <div class="dashboard-list-row"><div><strong>最老排队时长</strong></div><b>{{ Math.round(generationQueue.oldest_queue_age_ms / 1000) }} 秒</b></div>
          <div class="dashboard-list-row"><div><strong>队列等待 P95</strong></div><b>{{ generationQueue.queue_wait_p95_ms }} ms</b></div>
          <div class="dashboard-list-row"><div><strong>供应商耗时 P95</strong></div><b>{{ generationQueue.provider_latency_p95_ms }} ms</b></div>
          <div class="dashboard-list-row"><div><strong>429 / 失败率</strong></div><b>{{ generationQueue.provider_429_rate }}% / {{ generationQueue.failure_rate }}%</b></div>
          <div class="dashboard-list-row"><div><strong>租约过期次数</strong></div><b>{{ generationQueue.lease_expired_count ?? 0 }}</b></div>
          <div class="dashboard-list-row"><div><strong>活跃业务入口</strong></div><b>{{ formatActiveBreakdown(generationQueue.active_by_entry_point) }}</b></div>
          <div class="dashboard-list-row"><div><strong>活跃供应商 / 渠道</strong></div><b>{{ formatActiveBreakdown(generationQueue.active_by_provider) }} / {{ formatActiveBreakdown(generationQueue.active_by_channel) }}</b></div>
          <div class="dashboard-list-row"><div><strong>Swap 使用</strong></div><b>{{ formatBytes(resources?.memory?.swap_used_bytes ?? 0) }} / {{ formatBytes(resources?.memory?.swap_total_bytes ?? 0) }}</b></div>
        </div>
      </article>

      <div class="dashboard-business-grid">
        <article class="admin-panel dashboard-panel" style="grid-column: span 2">
          <div class="panel-title-row">
            <h2>CPU 详情</h2>
            <span class="dashboard-panel-meta">{{ cpuCard.cores }} 核心</span>
          </div>
          <div class="dashboard-compact-list">
            <div class="dashboard-list-row">
              <div><strong>使用率</strong></div>
              <b>{{ cpuCard.usage_percent }}%</b>
            </div>
            <div class="dashboard-list-row">
              <div><strong>负载均值 (1/5/15 min)</strong></div>
              <b>{{ cpuCard.load_average.join(' / ') }}</b>
            </div>
          </div>
        </article>

        <article class="admin-panel dashboard-panel">
          <div class="panel-title-row">
            <h2>内存详情</h2>
            <span class="dashboard-panel-meta">{{ memoryCard.usage_percent }}%</span>
          </div>
          <div class="dashboard-compact-list">
            <div class="dashboard-list-row">
              <div><strong>总计</strong></div>
              <b>{{ formatBytes(memoryCard.total_bytes) }}</b>
            </div>
            <div class="dashboard-list-row">
              <div><strong>已用</strong></div>
              <b>{{ formatBytes(memoryCard.used_bytes) }}</b>
            </div>
            <div class="dashboard-list-row">
              <div><strong>可用</strong></div>
              <b>{{ formatBytes(memoryCard.available_bytes) }}</b>
            </div>
          </div>
        </article>

        <article class="admin-panel dashboard-panel">
          <div class="panel-title-row">
            <h2>磁盘详情</h2>
            <span class="dashboard-panel-meta">{{ diskCard.usage_percent }}%</span>
          </div>
          <div class="dashboard-compact-list">
            <div class="dashboard-list-row">
              <div><strong>挂载点</strong></div>
              <b>{{ resources?.disk?.path ?? '/' }}</b>
            </div>
            <div class="dashboard-list-row">
              <div><strong>总计</strong></div>
              <b>{{ formatBytes(diskCard.total_bytes) }}</b>
            </div>
            <div class="dashboard-list-row">
              <div><strong>已用</strong></div>
              <b>{{ formatBytes(diskCard.used_bytes) }}</b>
            </div>
            <div class="dashboard-list-row">
              <div><strong>可用</strong></div>
              <b>{{ formatBytes(diskCard.free_bytes) }}</b>
            </div>
          </div>
        </article>
      </div>

      <article class="admin-panel dashboard-bottom-panel">
        <div class="panel-title-row">
          <h2>运行中进程</h2>
          <span class="dashboard-panel-meta">按资源占用排序，最多 {{ processes.length }} 个</span>
        </div>
        <div class="admin-table-wrap">
          <table class="admin-table">
            <thead>
              <tr>
                <th>PID</th>
                <th>进程名</th>
                <th>CPU%</th>
                <th>内存%</th>
                <th>RSS</th>
                <th>状态</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="proc in processes" :key="proc.pid">
                <td><code>{{ proc.pid }}</code></td>
                <td><strong>{{ proc.name }}</strong></td>
                <td>{{ proc.cpu_percent }}%</td>
                <td>{{ proc.memory_percent }}%</td>
                <td>{{ formatBytes(proc.rss_bytes) }}</td>
                <td>{{ statusText(proc.status) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </article>
    </template>
  </section>
</template>
