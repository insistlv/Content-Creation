<script setup>
import { computed, ref, watch } from 'vue'
import { ClipboardList, Play } from 'lucide-vue-next'
import AgentTaskInputPanel from './AgentTaskInputPanel.vue'
import AgentExecutionPanel from './AgentExecutionPanel.vue'
import AgentPlanPanel from './AgentPlanPanel.vue'

const props = defineProps({
  step: {
    type: String,
    default: 'describe'
  },
  messages: {
    type: Array,
    default: () => []
  },
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
  },
  referenceItems: {
    type: Array,
    default: () => []
  },
  works: {
    type: Array,
    default: () => []
  },
  referenceUploading: {
    type: Boolean,
    default: false
  },
  requiresAuth: {
    type: Boolean,
    default: false
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

defineEmits([
  'send-message',
  'upload-reference',
  'remove-reference',
  'clear-references',
  'use-work-reference',
  'require-auth',
  'update-plan',
  'select-candidate',
  'estimate',
  'confirm-generate',
  'retry-plan',
  'retry-generate',
  'open-result',
  'use-result-reference',
  'view-works'
])

const steps = [
  { key: 'describe', label: '描述需求' },
  { key: 'plan', label: '确认方案' },
  { key: 'result', label: '生成结果' }
]

const stageTabs = [
  { key: 'plan', label: '创作方案', icon: ClipboardList },
  { key: 'run', label: '执行与结果', icon: Play }
]

const activeStage = ref('plan')

const hasPlan = computed(() => Boolean(props.plan?.prompt))
const hasTask = computed(() => Boolean(props.task))

watch(
  () => props.step,
  (step) => {
    if (step === 'result') {
      activeStage.value = 'run'
    } else if (step === 'plan') {
      activeStage.value = 'plan'
    }
  }
)

function selectStage(key) {
  activeStage.value = key
}
</script>

<template>
  <section class="agent-workspace-panel" :class="`agent-step-${step}`" data-testid="workspace-agent-panel">
    <header class="agent-workbench-head">
      <div>
        <span class="agent-panel-kicker">Agent</span>
        <h2>创作任务代理</h2>
      </div>
      <ol class="agent-step-tabs" aria-label="Agent 流程">
        <li
          v-for="(item, index) in steps"
          :key="item.key"
          :class="{ active: step === item.key, done: steps.findIndex((entry) => entry.key === step) > index }"
          :data-testid="`agent-step-${item.key}`"
        >
          <span>{{ index + 1 }}</span>
          <strong>{{ item.label }}</strong>
        </li>
      </ol>
    </header>

    <div class="agent-workbench-body">
      <AgentTaskInputPanel
        :messages="messages"
        :planning="planning"
        :reference-items="referenceItems"
        :works="works"
        :reference-uploading="referenceUploading"
        :requires-auth="requiresAuth"
        :failure="failure?.phase === 'plan' ? failure : null"
        @send-message="$emit('send-message', $event)"
        @upload-reference="$emit('upload-reference', $event)"
        @remove-reference="$emit('remove-reference', $event)"
        @clear-references="$emit('clear-references')"
        @use-work-reference="$emit('use-work-reference', $event)"
        @require-auth="$emit('require-auth')"
        @retry-plan="$emit('retry-plan')"
      />

      <div class="agent-stage-panel" :class="`agent-stage-${activeStage}`" data-testid="agent-stage-panel">
        <div class="agent-stage-tabs" role="tablist" aria-label="方案与执行">
          <button
            v-for="tab in stageTabs"
            :key="tab.key"
            type="button"
            role="tab"
            class="agent-stage-tab"
            :class="{ active: activeStage === tab.key }"
            :aria-selected="activeStage === tab.key"
            :data-testid="`agent-stage-tab-${tab.key}`"
            @click="selectStage(tab.key)"
          >
            <component :is="tab.icon" :size="15" />
            <span>{{ tab.label }}</span>
            <small v-if="tab.key === 'plan' && hasPlan" class="agent-stage-dot" aria-hidden="true" />
            <small v-if="tab.key === 'run' && hasTask" class="agent-stage-dot" aria-hidden="true" />
          </button>
        </div>

        <div class="agent-stage-body">
          <AgentPlanPanel
            v-show="activeStage === 'plan'"
            :plan="plan"
            :candidates="candidates"
            :selected-candidate-id="selectedCandidateId"
            :safety-notes="safetyNotes"
            :planning="planning"
            :clarification-prompt="clarificationPrompt"
            @update-plan="$emit('update-plan', $event)"
            @select-candidate="$emit('select-candidate', $event)"
          />

          <AgentExecutionPanel
            v-show="activeStage === 'run'"
            :plan="plan"
            :credit-estimate="creditEstimate"
            :credit-estimate-loading="creditEstimateLoading"
            :credit-estimate-error="creditEstimateError"
            :estimate-dirty="estimateDirty"
            :can-confirm-generate="canConfirmGenerate"
            :confirm-disabled-reason="confirmDisabledReason"
            :submitting="submitting"
            :task="task"
            :error="error"
            :failure="failure?.phase === 'generate' ? failure : null"
            :stage-copy="stageCopy"
            @estimate="$emit('estimate')"
            @confirm-generate="$emit('confirm-generate')"
            @retry-generate="$emit('retry-generate')"
            @open-result="$emit('open-result')"
            @use-result-reference="$emit('use-result-reference', $event)"
            @view-works="$emit('view-works')"
          />
        </div>
      </div>
    </div>
  </section>
</template>
