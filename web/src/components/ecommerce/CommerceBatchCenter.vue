<script setup>
import { onMounted, ref, watch } from 'vue'
import { CircleDot, Download, RotateCcw, XCircle } from 'lucide-vue-next'
import { api } from '../../api/client.js'
import { useCommerceBatches } from '../../composables/useCommerceBatches.js'
const props = defineProps({ projectId: { type: [Number, String], required: true } })
const { batches, error, start } = useCommerceBatches()
const pendingRetries = ref(new Set())
const labels = { queued: '排队中', retrying: '重试中', running: '运行中', succeeded: '已完成', partial_succeeded: '部分完成', failed: '失败', canceled: '已取消', canceling: '取消中' }
const statusOf = (status) => labels[status] || '未知状态'
const activeItem = (status) => ['queued', 'retrying', 'running'].includes(status)
async function retry(item) {
  if (pendingRetries.value.has(item.id)) return
  pendingRetries.value.add(item.id)
  const key = `commerce-retry-${item.id}-${Date.now()}-${Math.random().toString(36).slice(2)}`
  try { await api.retryCommerceItem(item.id, key) } finally { pendingRetries.value.delete(item.id) }
}
watch(() => props.projectId, (id) => start(id))
onMounted(() => start(props.projectId))
</script>
<template><section class="commerce-card batch-center"><h2>批次恢复</h2><p v-if="error" role="alert">{{ error }}</p><p v-if="!batches.length">暂无服务端批次。</p><article v-for="batch in batches" :key="batch.id" class="batch-row"><header><span data-testid="commerce-batch-status" :aria-label="`批次状态：${statusOf(batch.status)}`"><CircleDot :size="16" aria-hidden="true"/>{{ statusOf(batch.status) }}</span><b>#{{ batch.id }}</b></header><p>项目 {{ batch.total_items ?? batch.items?.length ?? 0 }} · 预占 {{ batch.held_credits ?? batch.reserved_credits ?? 0 }} 点 · 已结算 {{ batch.settled_credits ?? 0 }} 点 · 已释放 {{ batch.released_credits ?? 0 }} 点</p><p>预计剩余时间：{{ batch.eta_seconds > 0 ? `${batch.eta_seconds} 秒` : ['succeeded','partial_succeeded','failed','canceled'].includes(batch.status) ? '已结束' : '计算中' }}</p><button v-if="['queued','running'].includes(batch.status)" type="button" @click="api.cancelCommerceBatch(batch.id)"><XCircle :size="15"/>取消批次</button><div class="commerce-results"><div v-for="item in batch.items || []" :key="item.id" class="commerce-result-row"><span>{{ statusOf(item.status) }}</span><button v-if="activeItem(item.status)" type="button" :data-testid="`commerce-item-cancel-${item.id}`" @click="api.cancelCommerceItem(item.id)">取消</button><button v-if="item.status === 'failed'" type="button" :disabled="pendingRetries.has(item.id)" :data-testid="`commerce-item-retry-${item.id}`" @click="retry(item)"><RotateCcw :size="15"/>重试</button><a v-if="item.work_id" :href="`/api/works/${item.work_id}/download`" :data-testid="`commerce-item-download-${item.id}`"><Download :size="15"/>下载</a></div></div></article></section></template>
