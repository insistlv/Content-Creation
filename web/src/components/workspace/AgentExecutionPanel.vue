<script setup>
import { computed } from 'vue'
import { Eye, ImagePlus, Library, Play, RefreshCw } from 'lucide-vue-next'

const props = defineProps({
  plan: {
    type: Object,
    default: null
  },
  creditEstimate: {
    type: Object,
    default: null
  },
  creditEstimateLoading: {
    type: Boolean,
    default: false
  },
  creditEstimateError: {
    type: String,
    default: ''
  },
  estimateDirty: {
    type: Boolean,
    default: false
  },
  canConfirmGenerate: {
    type: Boolean,
    default: false
  },
  confirmDisabledReason: {
    type: String,
    default: ''
  },
  submitting: {
    type: Boolean,
    default: false
  },
  task: {
    type: Object,
    default: null
  },
  error: {
    type: String,
    default: ''
  },
  failure: {
    type: Object,
    default: null
  },
  stageCopy: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['estimate', 'confirm-generate', 'retry-generate', 'open-result', 'use-result-reference', 'view-works'])

const creditText = computed(() => {
  if (props.creditEstimateLoading) return '正在预估点数'
  if (props.estimateDirty && props.plan?.prompt) return '预估已过期'
  if (props.creditEstimate?.required_credits !== undefined) {
    return `预计 ${Number(props.creditEstimate.required_credits) || 0} 点`
  }
  if (props.creditEstimateError) return '预估失败'
  return '等待方案'
})
const statusText = computed(() => {
  if (!props.plan?.prompt) return '等待方案'
  if (!props.task) return '待确认'
  return props.stageCopy?.shortLabel || props.task.status || '处理中'
})
const confirmButtonText = computed(() => {
  if (props.submitting) return '提交中'
  if (props.creditEstimate?.required_credits !== undefined && !props.estimateDirty) {
    return `确认生成 · 预计 ${Number(props.creditEstimate.required_credits) || 0} 点`
  }
  return '确认生成'
})
const canRetryEstimate = computed(() => Boolean(props.plan?.prompt) && !props.creditEstimateLoading && !props.submitting)
const resultPreview = computed(() => props.task?.preview_url || '')
</script>

<template>
  <section class="agent-execution-panel" aria-label="Agent 执行区">
    <header class="agent-panel-head">
      <span class="agent-panel-kicker">Run</span>
      <h2>执行与结果</h2>
    </header>

    <div class="agent-credit-box" data-testid="agent-credit-estimate">
      <strong>{{ creditText }}</strong>
      <span v-if="creditEstimate?.available_credits !== undefined">当前 {{ creditEstimate.available_credits }} 点</span>
      <span v-if="creditEstimate?.enough === false">还差 {{ creditEstimate.missing_credits ?? 0 }} 点</span>
      <span v-if="estimateDirty && plan?.prompt">方案有变更，正在等待最新点数。</span>
    </div>

    <div class="agent-run-actions">
      <button
        type="button"
        class="secondary-button"
        data-testid="agent-estimate-button"
        :disabled="!canRetryEstimate"
        @click="emit('estimate')"
      >
        <RefreshCw :size="15" />
        {{ creditEstimateError ? '重试预估' : '预估点数' }}
      </button>
      <button
        type="button"
        class="primary-button"
        data-testid="agent-confirm-generate"
        :disabled="!canConfirmGenerate"
        @click="emit('confirm-generate')"
      >
        <Play :size="15" />
        {{ confirmButtonText }}
      </button>
    </div>

    <p v-if="confirmDisabledReason" class="agent-disabled-reason" data-testid="agent-execution-reason">
      {{ confirmDisabledReason }}
    </p>

    <div v-if="failure" class="agent-failure-card" data-testid="agent-generation-failure" role="alert">
      <strong>{{ failure.reasonTitle }}</strong>
      <p>{{ failure.message }}</p>
      <span>{{ failure.suggestion }}</span>
      <button
        type="button"
        class="primary-button"
        data-testid="agent-retry-generate"
        :disabled="submitting || !failure.retryable"
        @click="emit('retry-generate')"
      >
        <RefreshCw :size="15" />
        {{ failure.retryLabel || '重新生成' }}
      </button>
    </div>

    <p v-if="creditEstimateError" class="agent-error" role="alert">{{ creditEstimateError }}</p>
    <p v-if="error" class="agent-error" role="alert">{{ error }}</p>

    <div class="agent-task-status" data-testid="agent-execution-status">
      <strong>{{ statusText }}</strong>
      <span v-if="stageCopy?.description">{{ stageCopy.description }}</span>
      <span v-else>方案确认后才会提交生成任务。</span>
    </div>

    <div v-if="resultPreview" class="agent-result-preview">
      <img :src="resultPreview" alt="Agent 生成结果预览" />
      <div class="agent-result-actions">
        <button type="button" @click="emit('open-result')">
          <Eye :size="15" />
          放大查看
        </button>
        <button type="button" @click="emit('use-result-reference', task)">
          <ImagePlus :size="15" />
          作为参考继续创作
        </button>
        <button type="button" @click="emit('confirm-generate')">
          <RefreshCw :size="15" />
          再生成一次
        </button>
        <button type="button" @click="emit('view-works')">
          <Library :size="15" />
          查看作品库
        </button>
      </div>
    </div>
  </section>
</template>
