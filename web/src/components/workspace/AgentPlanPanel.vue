<script setup>
import { computed } from 'vue'

const props = defineProps({
  plan: {
    type: Object,
    default: null
  },
  candidates: {
    type: Array,
    default: () => []
  },
  selectedCandidateId: {
    type: String,
    default: ''
  },
  safetyNotes: {
    type: Array,
    default: () => []
  },
  planning: {
    type: Boolean,
    default: false
  },
  clarificationPrompt: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['update-plan', 'select-candidate'])

const PROMPT_MAX = 4000

const hasPlan = computed(() => Boolean(props.plan?.prompt))
const promptLength = computed(() => (props.plan?.prompt || '').length)

const selectedCandidate = computed(() => {
  if (!props.candidates.length) return null
  return (
    props.candidates.find(
      (item) => (item.id || item.title) === props.selectedCandidateId
    ) || null
  )
})

const canRestorePlan = computed(() => {
  const candidate = selectedCandidate.value
  if (!candidate) return false
  return (props.plan?.prompt || '') !== (candidate.prompt || '')
})

function updateField(key, value) {
  emit('update-plan', {
    ...(props.plan || {}),
    [key]: value
  })
}

function restorePlan() {
  if (!selectedCandidate.value) return
  emit('select-candidate', selectedCandidate.value.id || selectedCandidate.value.title)
}

function updateReferenceWeight(event) {
  const value = Math.max(0, Math.min(100, Number(event.target.value) || 0))
  updateField('reference_weight', value)
}
</script>

<template>
  <section class="agent-plan-panel" aria-label="Agent 方案区">
    <header class="agent-panel-head">
      <span class="agent-panel-kicker">Plan</span>
      <h2>创作方案</h2>
    </header>

    <div v-if="clarificationPrompt" class="agent-clarification" data-testid="agent-clarification" role="status">
      <strong>需要继续补充</strong>
      <span>{{ clarificationPrompt }}</span>
    </div>

    <div v-if="!hasPlan" class="agent-plan-empty" data-testid="agent-plan-empty">
      <strong>描述任务/加参考</strong>
      <span>Agent 会先整理方案，确认后预估点数，再提交生成。</span>
      <div class="agent-plan-examples" aria-label="示例任务">
        <button type="button" disabled>香薰商品主图</button>
        <button type="button" disabled>旧图局部改色</button>
        <button type="button" disabled>海报 KV 延展</button>
      </div>
    </div>

    <template v-else>
      <div class="agent-plan-summary">
        <span>方案摘要</span>
        <strong data-testid="agent-plan-title">{{ plan.title }}</strong>
        <p>{{ plan.intent || 'image_generation' }} · {{ plan.style_preset || '自定义风格' }}</p>
      </div>

      <div class="agent-candidate-list" v-if="candidates.length">
        <button
          v-for="candidate in candidates"
          :key="candidate.id || candidate.title"
          type="button"
          class="agent-candidate-button"
          :class="{ active: (candidate.id || candidate.title) === selectedCandidateId }"
          :data-testid="`agent-candidate-${candidate.id || candidate.title}`"
          :disabled="planning"
          @click="emit('select-candidate', candidate.id || candidate.title)"
        >
          <strong>{{ candidate.title || '备选方向' }}</strong>
          <span>{{ candidate.prompt }}</span>
        </button>
      </div>

      <div class="agent-plan-editor">
        <label>
          <span class="agent-prompt-label">
            提示词
            <span class="agent-prompt-meta">
              <button
                v-if="canRestorePlan"
                type="button"
                class="agent-prompt-restore"
                data-testid="agent-plan-restore"
                :disabled="planning"
                @click="restorePlan"
              >
                恢复 AI 方案
              </button>
              <span
                class="agent-prompt-counter"
                :class="{ warn: promptLength > PROMPT_MAX * 0.9 }"
                data-testid="agent-plan-prompt-counter"
              >{{ promptLength }}/{{ PROMPT_MAX }}</span>
            </span>
          </span>
          <textarea
            data-testid="agent-plan-prompt"
            :value="plan.prompt"
            rows="7"
            :maxlength="PROMPT_MAX"
            @input="updateField('prompt', $event.target.value)"
          />
        </label>

        <div class="agent-plan-grid">
          <label>
            <span>工具</span>
            <select
              data-testid="agent-plan-tool-mode"
              :value="plan.tool_mode || 'generate'"
              @change="updateField('tool_mode', $event.target.value)"
            >
              <option value="generate">文生图</option>
              <option value="redraw">图生图</option>
              <option value="precision_edit">局部编辑</option>
              <option value="erase">移除物体</option>
              <option value="expand">扩图</option>
              <option value="remove_background">抠图</option>
              <option value="upscale">高清放大</option>
            </select>
          </label>

          <label>
            <span>比例</span>
            <select
              data-testid="agent-plan-aspect-ratio"
              :value="plan.aspect_ratio || '1:1'"
              @change="updateField('aspect_ratio', $event.target.value)"
            >
              <option value="1:1">1:1</option>
              <option value="3:4">3:4</option>
              <option value="4:3">4:3</option>
              <option value="9:16">9:16</option>
              <option value="16:9">16:9</option>
              <option value="21:9">21:9</option>
              <option value="9:21">9:21</option>
            </select>
          </label>

          <label>
            <span>质量</span>
            <select
              data-testid="agent-plan-quality"
              :value="plan.quality || 'medium'"
              @change="updateField('quality', $event.target.value)"
            >
              <option value="low">0.5K</option>
              <option value="medium">1K</option>
              <option value="high">2K</option>
              <option value="ultra">4K</option>
            </select>
          </label>
        </div>

        <details class="agent-advanced-params">
          <summary>高级参数</summary>

          <label>
            <span>标题</span>
            <input
              data-testid="agent-plan-title-input"
              :value="plan.title"
              maxlength="80"
              @input="updateField('title', $event.target.value)"
            />
          </label>

          <label>
            <span>风格</span>
            <input
              data-testid="agent-plan-style-preset"
              :value="plan.style_preset || ''"
              maxlength="40"
              @input="updateField('style_preset', $event.target.value)"
            />
          </label>

          <label>
            <span>负向提示词</span>
            <textarea
              data-testid="agent-plan-negative-prompt"
              :value="plan.negative_prompt || ''"
              rows="2"
              maxlength="1200"
              @input="updateField('negative_prompt', $event.target.value)"
            />
          </label>

          <label class="agent-reference-weight">
            <span>参考强度 {{ plan.reference_weight ?? 75 }}%</span>
            <input
              data-testid="agent-plan-reference-weight"
              type="range"
              min="0"
              max="100"
              :value="plan.reference_weight ?? 75"
              @input="updateReferenceWeight"
            />
          </label>

          <label>
            <span>编辑说明</span>
            <textarea
              data-testid="agent-plan-edit-instruction"
              :value="plan.edit_instruction || ''"
              rows="2"
              maxlength="1200"
              @input="updateField('edit_instruction', $event.target.value)"
            />
          </label>
        </details>
      </div>

      <div v-if="safetyNotes.length" class="agent-safety-notes">
        <span v-for="note in safetyNotes" :key="note">{{ note }}</span>
      </div>
    </template>
  </section>
</template>
