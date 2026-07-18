<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, BarChart3, CheckCircle2, Clock3, Cpu, XCircle } from 'lucide-vue-next'

import { api } from '../api/client.js'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const errorMessage = ref('')
const detail = ref(null)

const model = computed(() => detail.value?.model ?? {})
const usage = computed(() => detail.value?.usage ?? {})
const dailyTrend = computed(() => detail.value?.daily_trend ?? [])
const statusBreakdown = computed(() => detail.value?.status_breakdown ?? [])
const recentGenerations = computed(() => detail.value?.recent_generations ?? [])
const recentCallAttempts = computed(() => detail.value?.recent_call_attempts ?? [])
const showsCallAttempts = computed(() => recentCallAttempts.value.length > 0)
const maxTrendCalls = computed(() => Math.max(1, ...dailyTrend.value.map((item) => Number(item.calls || 0))))

const successRate = computed(() => {
  const total = Number(usage.value.total_calls || 0)
  if (total <= 0) return 0
  return Math.round((Number(usage.value.succeeded_calls || 0) / total) * 100)
})

const kpiCards = computed(() => [
  { label: '总调用', value: formatNumber(usage.value.total_calls), icon: BarChart3 },
  { label: '成功率', value: `${successRate.value}%`, icon: CheckCircle2 },
  { label: '今日调用', value: formatNumber(usage.value.today_calls), icon: Cpu },
  { label: '平均耗时', value: formatLatency(usage.value.average_latency_ms), icon: Clock3 }
])

async function loadDetail() {
  loading.value = true
  errorMessage.value = ''
  try {
    detail.value = await api.getAdminModel(route.params.id)
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

function goBack() {
  router.push('/admin/settings')
}

function formatNumber(value) {
  return Number(value || 0).toLocaleString()
}

function formatLatency(value) {
  const latency = Number(value || 0)
  if (latency <= 0) return '-'
  if (latency >= 1000) return `${(latency / 1000).toFixed(1)}s`
  return `${latency}ms`
}

function trendHeight(point) {
  return `${Math.max(6, Math.round((Number(point.calls || 0) / maxTrendCalls.value) * 100))}%`
}

function statusLabel(status) {
  if (status === 'succeeded') return '成功'
  if (status === 'failed') return '失败'
  if (status === 'running') return '运行中'
  if (status === 'queued') return '排队中'
  return status || '-'
}

function callAttemptClass(item) {
  if (item.status === 'succeeded') return 'is-success'
  if (item.status === 'failed') return 'is-failed'
  return ''
}

function attemptDiagnostics(item) {
  const diagnostics = []
  if (item.error_code) diagnostics.push(item.error_code)
  if (item.http_status) diagnostics.push(`HTTP ${item.http_status}`)
  if (item.error_message) diagnostics.push(item.error_message)
  if (item.provider_request_id) diagnostics.push(item.provider_request_id)
  return diagnostics.join(' · ')
}

onMounted(loadDetail)
</script>

<template>
  <section class="admin-model-detail-page">
    <div class="model-detail-heading">
      <button class="mini-button compact-button" type="button" data-testid="back-to-model-settings" @click="goBack">
        <ArrowLeft :size="16" />
        返回模型列表
      </button>
      <div>
        <p class="panel-kicker">Model Usage</p>
        <h1>{{ model.name || '模型详情' }}</h1>
        <span>{{ model.provider || '-' }} · {{ model.runtime_model || model.name || '-' }}</span>
      </div>
    </div>

    <div v-if="loading" class="page-status">加载中...</div>
    <div v-else-if="errorMessage" class="settings-alert error">
      <XCircle :size="16" />
      <span>{{ errorMessage }}</span>
    </div>

    <template v-else>
      <div class="model-detail-kpi-grid">
        <article v-for="card in kpiCards" :key="card.label" class="model-detail-kpi">
          <component :is="card.icon" :size="18" />
          <span>{{ card.label }}</span>
          <strong>{{ card.value }}</strong>
        </article>
      </div>

      <div class="model-detail-grid">
        <section class="admin-panel model-usage-panel">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Trend</p>
              <h2>调用趋势</h2>
            </div>
            <small>最近 {{ dailyTrend.length }} 天</small>
          </div>
          <div class="model-trend-chart">
            <div
              v-for="point in dailyTrend"
              :key="point.date"
              class="model-trend-column"
              :data-testid="`model-trend-bar-${point.date}`"
            >
              <div class="model-trend-stack">
                <i :style="{ height: trendHeight(point) }"></i>
              </div>
              <span>{{ point.date.slice(5) }}</span>
              <b>{{ point.calls }}</b>
            </div>
          </div>
        </section>

        <section class="admin-panel model-status-panel">
          <div class="panel-title-row">
            <div>
              <p class="panel-kicker">Status</p>
              <h2>状态分布</h2>
            </div>
          </div>
          <div class="model-status-list">
            <div v-for="item in statusBreakdown" :key="item.status" class="model-status-row">
              <span>{{ statusLabel(item.status) }}</span>
              <strong>{{ formatNumber(item.count) }}</strong>
            </div>
            <div v-if="statusBreakdown.length === 0" class="model-detail-empty">暂无调用记录</div>
          </div>
        </section>
      </div>

      <section class="admin-panel model-recent-panel">
        <div class="panel-title-row">
          <div>
            <p class="panel-kicker">Recent Calls</p>
            <h2>最近调用</h2>
          </div>
        </div>
        <div class="model-recent-list">
          <template v-if="showsCallAttempts">
            <article
              v-for="item in recentCallAttempts"
              :key="`attempt-${item.id}`"
              class="model-recent-item model-call-attempt"
              :class="callAttemptClass(item)"
              data-testid="model-call-attempt"
            >
              <div>
                <strong>#{{ item.generation_record_id || '-' }} · 第 {{ item.attempt_index || '-' }} 次</strong>
                <span>{{ attemptDiagnostics(item) || '-' }}</span>
              </div>
              <span class="model-recent-status">{{ statusLabel(item.status) }}</span>
              <small>{{ formatLatency(item.latency_ms) }}</small>
            </article>
          </template>
          <template v-else>
            <article v-for="item in recentGenerations" :key="item.id" class="model-recent-item">
              <div>
                <strong>{{ item.prompt_summary || '-' }}</strong>
                <span>#{{ item.id }} · 用户 {{ item.user_id || '-' }} · {{ item.model }}</span>
              </div>
              <span class="model-recent-status">{{ statusLabel(item.status) }}</span>
              <small>{{ formatLatency(item.latency_ms) }}</small>
            </article>
          </template>
          <div v-if="!showsCallAttempts && recentGenerations.length === 0" class="model-detail-empty">暂无最近调用</div>
        </div>
      </section>
    </template>
  </section>
</template>

<style scoped>
.admin-model-detail-page {
  display: grid;
  gap: 18px;
}

.model-detail-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.model-detail-heading h1 {
  margin: 0;
  color: #101828;
  font-size: 1.72rem;
}

.model-detail-heading span,
.model-detail-heading small,
.model-recent-item span,
.model-recent-item small {
  color: #667085;
}

.model-detail-kpi-grid,
.model-detail-grid {
  display: grid;
  gap: 14px;
}

.model-detail-kpi-grid {
  grid-template-columns: repeat(4, minmax(150px, 1fr));
}

.model-detail-kpi {
  display: grid;
  gap: 8px;
  min-height: 118px;
  padding: 16px;
  border: 1px solid rgba(112, 124, 156, 0.14);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.84);
  box-shadow: 0 16px 34px rgba(82, 92, 126, 0.08);
}

.model-detail-kpi svg {
  color: #2563eb;
}

.model-detail-kpi span {
  color: #667085;
  font-size: 0.82rem;
  font-weight: 900;
}

.model-detail-kpi strong {
  color: #101828;
  font-size: 1.54rem;
}

.model-detail-grid {
  grid-template-columns: minmax(0, 1fr) minmax(260px, 340px);
}

.model-trend-chart {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(42px, 1fr));
  gap: 10px;
  min-height: 230px;
  align-items: end;
}

.model-trend-column {
  display: grid;
  gap: 7px;
  min-width: 0;
  text-align: center;
}

.model-trend-stack {
  display: flex;
  align-items: end;
  height: 150px;
  padding: 8px;
  border-radius: 14px;
  background: rgba(37, 99, 235, 0.08);
}

.model-trend-stack i {
  display: block;
  width: 100%;
  border-radius: 999px 999px 6px 6px;
  background: linear-gradient(180deg, #2563eb, #7c3aed);
}

.model-trend-column span,
.model-trend-column b {
  font-size: 0.76rem;
}

.model-trend-column span {
  color: #667085;
}

.model-status-list,
.model-recent-list {
  display: grid;
  gap: 10px;
}

.model-status-row,
.model-recent-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px;
  border-radius: 14px;
  background: rgba(248, 250, 253, 0.92);
}

.model-recent-item div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.model-recent-item strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-recent-status {
  flex: 0 0 auto;
  font-weight: 900;
}

.model-call-attempt {
  border: 1px solid transparent;
}

.model-call-attempt.is-success {
  border-color: rgba(22, 163, 74, 0.26);
  background: rgba(240, 253, 244, 0.92);
}

.model-call-attempt.is-success .model-recent-status {
  color: #15803d;
}

.model-call-attempt.is-failed {
  border-color: rgba(220, 38, 38, 0.24);
  background: rgba(254, 242, 242, 0.92);
}

.model-call-attempt.is-failed .model-recent-status {
  color: #b42318;
}

.model-detail-empty {
  padding: 26px;
  color: #8a94a6;
  text-align: center;
}

@media (max-width: 960px) {
  .model-detail-kpi-grid,
  .model-detail-grid {
    grid-template-columns: 1fr;
  }

  .model-detail-heading {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
