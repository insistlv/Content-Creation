<script setup>
import { computed } from 'vue'
import { ArrowUpRight, Images, Palette, Sparkles, Wrench } from 'lucide-vue-next'
import PillTag from '../PillTag.vue'
import WorkspaceCaseCard from './WorkspaceCaseCard.vue'
import WorkspaceToolCard from './WorkspaceToolCard.vue'

const props = defineProps({
  workspaceTab: {
    type: String,
    default: 'discover'
  },
  discoveryFilter: {
    type: String,
    default: 'all'
  },
  aiToolCards: {
    type: Array,
    default: () => []
  },
  playgroundCards: {
    type: Array,
    default: () => []
  },
  caseTemplates: {
    type: Array,
    default: () => []
  },
  toolMode: {
    type: String,
    default: 'generate'
  },
  task: {
    type: Object,
    default: null
  },
  submitting: {
    type: Boolean,
    default: false
  },
  works: {
    type: Array,
    default: () => []
  },
  worksTotal: {
    type: Number,
    default: 0
  },
  worksPage: {
    type: Number,
    default: 1
  },
  worksPageSize: {
    type: Number,
    default: 18
  },
  worksLoading: {
    type: Boolean,
    default: false
  },
  worksError: {
    type: String,
    default: ''
  },
  discoveryError: {
    type: String,
    default: ''
  },
  resultError: {
    type: String,
    default: ''
  },
  previewAsset: {
    type: Object,
    default: null
  },
  isRemoveBackgroundResult: {
    type: Boolean,
    default: false
  },
  stageCopy: {
    type: Object,
    default: null
  },
  canRetryGeneration: {
    type: Boolean,
    default: false
  },
  canCancelGeneration: {
    type: Boolean,
    default: false
  },
  cancelGenerationLoading: {
    type: Boolean,
    default: false
  },
  regenerateResultLoading: {
    type: Boolean,
    default: false
  },
  regenerateResultError: {
    type: String,
    default: ''
  },
  cancellingTaskIds: {
    type: Array,
    default: () => []
  },
  isCancelledTask: {
    type: Boolean,
    default: false
  },
  activeTasks: {
    type: Array,
    default: () => []
  },
  selectedTaskId: {
    type: [Number, String],
    default: null
  }
})

defineEmits([
  'update:workspaceTab',
  'update:discoveryFilter',
  'select-tool',
  'open-playground-item',
  'select-template',
  'open-preview-zoom',
  'regenerate-result',
  'use-result-as-reference',
  'toggle-result-favorite',
  'share-result',
  'open-works-library',
  'retry-generation',
  'cancel-generation',
  'select-generation-task',
  'retry-generation-task',
  'cancel-generation-task',
  'retry-works',
  'change-works-page',
  'retry-discovery',
  'download-image',
  'select-history-work',
  'use-work-as-reference'
])

const discoveryFilters = [
  { key: 'all', label: '全部' },
  { key: 'image', label: '图片' },
  { key: 'video', label: '视频' },
  { key: 'tool', label: '工具' }
]

const showTools = computed(() => ['all', 'tool'].includes(props.discoveryFilter))
const showPlayground = computed(() => ['all', 'tool'].includes(props.discoveryFilter) && props.playgroundCards.length > 0)
const showCases = computed(() => ['all', 'image'].includes(props.discoveryFilter))
const showVideoEmpty = computed(() => props.discoveryFilter === 'video')
const worksDisplayTotal = computed(() => Number(props.worksTotal || props.works.length || 0))
const worksTotalPages = computed(() => Math.max(1, Math.ceil(worksDisplayTotal.value / Math.max(1, props.worksPageSize))))
const showWorksPagination = computed(() => worksDisplayTotal.value > props.worksPageSize)

const taskStatusLabels = {
  queued: '排队中',
  requesting_provider: '请求模型',
  running: '请求模型',
  persisting_result: '保存结果',
  succeeded: '已完成',
  failed: '失败'
}

function generationTaskLabel(task) {
  if (isUserCancelledTask(task)) return '已取消'
  return taskStatusLabels[task?.stage] || taskStatusLabels[task?.status] || '排队中'
}

function generationTaskSummary(task) {
  if (String(task?.status || '').toLowerCase() === 'failed') {
    return task?.failure_message || '生成失败，请调整提示词后重试'
  }
  return task?.prompt || task?.parameters?.prompt || `任务 ${task?.generation_id || ''}`
}

function isSelectedTask(task) {
  return Number(task?.generation_id) === Number(props.selectedTaskId)
}

function isActiveTask(task) {
  return ['queued', 'running'].includes(String(task?.status || '').toLowerCase())
}

function isCancellingTask(task) {
  if (!task?.generation_id) return false
  return props.cancellingTaskIds.map((id) => Number(id)).includes(Number(task.generation_id))
}

function isUserCancelledTask(task) {
  return task?.error?.code === 'user_cancelled' || task?.error_code === 'user_cancelled'
}
</script>

<template>
  <main class="workspace-preview-area imini-discovery-area">
    <div class="imini-discovery-card">
      <div class="imini-tabs" aria-label="生成视图">
        <button
          type="button"
          data-testid="workspace-tab-discovery"
          :class="{ active: workspaceTab === 'discover' }"
          @click="$emit('update:workspaceTab', 'discover')"
        >
          发现
        </button>
        <button
          type="button"
          data-testid="workspace-tab-create"
          :class="{ active: workspaceTab === 'create' }"
          @click="$emit('update:workspaceTab', 'create')"
        >
          创建
        </button>
      </div>

      <div v-if="workspaceTab === 'discover'" data-testid="workspace-discovery-panel">
        <div class="preview-container workspace-preview-probe" aria-hidden="true" @dblclick="$emit('open-preview-zoom')"></div>

        <div v-if="discoveryError" class="workspace-inline-error" role="alert">
          <p class="error-message">{{ discoveryError }}</p>
          <button type="button" class="secondary-button" @click="$emit('retry-discovery')">重试</button>
        </div>

        <div class="imini-discovery-filters" aria-label="发现筛选">
          <button
            v-for="filter in discoveryFilters"
            :key="filter.key"
            type="button"
            :class="{ active: discoveryFilter === filter.key }"
            :data-testid="`workspace-discovery-filter-${filter.key}`"
            @click="$emit('update:discoveryFilter', filter.key)"
          >
            {{ filter.label }}
          </button>
        </div>

        <section v-if="works.length === 0" class="imini-empty-creation">
          <p>创建您的第一个创作~</p>
        </section>

        <section v-if="showTools" class="imini-section">
          <div class="imini-section-head">
            <div class="imini-section-title">
              <span aria-hidden="true"><Wrench :size="18" /></span>
              <h3>AI 工具</h3>
            </div>
            <span class="imini-section-kicker">发现更多可能</span>
          </div>
          <div class="imini-tool-grid">
            <WorkspaceToolCard
              v-for="tool in aiToolCards"
              :key="tool.mode"
              :tool="tool"
              :active="toolMode === tool.mode"
              :disabled="submitting"
              @select="$emit('select-tool', $event)"
            />
          </div>
        </section>

        <section v-if="showPlayground" class="imini-section imini-playground-section">
          <div class="imini-section-head">
            <div class="imini-section-title">
              <span aria-hidden="true"><Palette :size="18" /></span>
              <h3>创作乐园</h3>
            </div>
            <span class="imini-section-kicker">发现更多可能</span>
          </div>
          <div class="imini-playground-grid">
            <button
              v-for="item in playgroundCards"
              :key="item.id"
              type="button"
              class="imini-playground-card"
              :data-testid="`workspace-playground-${item.id}`"
              @click="$emit('open-playground-item', item)"
            >
              <span class="imini-playground-content">
                <span class="imini-card-copy-head">
                  <span class="imini-playground-icon" aria-hidden="true">
                    <component :is="item.icon" :size="18" />
                  </span>
                  <span class="imini-card-enter" aria-hidden="true">
                    <ArrowUpRight :size="16" />
                  </span>
                </span>
                <strong>{{ item.title }}</strong>
                <span class="imini-playground-description">{{ item.description }}</span>
              </span>
              <span class="imini-playground-media">
                <img :src="item.image" :alt="item.title" />
              </span>
            </button>
          </div>
        </section>

        <section v-if="showCases" class="imini-section">
          <div class="imini-section-head">
            <div class="imini-section-title">
              <span aria-hidden="true"><Images :size="18" /></span>
              <h3>优秀案例</h3>
            </div>
            <span class="imini-section-kicker">发现更多可能</span>
          </div>
          <div v-if="caseTemplates.length > 0" class="imini-case-masonry">
            <WorkspaceCaseCard
              v-for="item in caseTemplates"
              :key="item.id"
              :item="item"
              @select="$emit('select-template', $event)"
            />
          </div>
          <div v-else class="imini-empty-line">暂无优秀案例</div>
        </section>

        <section v-if="showVideoEmpty" class="imini-section">
          <div class="imini-empty-line imini-video-empty">暂无视频案例</div>
        </section>
      </div>

      <div v-else class="imini-create-surface" data-testid="workspace-create-panel">
        <section class="imini-section imini-result-section">
          <div class="imini-result-stage" data-testid="workspace-result-stage">
            <div
              class="preview-container imini-result-frame"
              :class="{ 'workspace-transparent-preview': isRemoveBackgroundResult }"
              data-testid="workspace-result-preview"
            >
              <img
                v-if="!resultError && previewAsset?.preview_url"
                :src="previewAsset.preview_url"
                alt="生成结果"
                class="preview-image"
                title="双击放大查看"
                data-skip-global-image-preview
                @dblclick="$emit('open-preview-zoom')"
              />
              <div
                v-else-if="resultError"
                class="preview-error-state"
                data-testid="workspace-result-error"
                role="status"
              >
                <strong>{{ isCancelledTask ? '已取消生成' : '生成失败' }}</strong>
                <p>{{ isCancelledTask ? '已取消生成，未扣点，可修改提示词后重新生成。' : '没有生成出可用图片，可点击重试或调整提示词后再次创建。' }}</p>
                <button
                  v-if="canRetryGeneration"
                  class="secondary-button"
                  type="button"
                  data-testid="workspace-result-retry-generation"
                  @click="$emit('retry-generation')"
                >
                  重新生成
                </button>
              </div>
              <div v-else class="preview-empty imini-result-empty" data-testid="workspace-result-empty">
                <span class="imini-result-empty-icon" aria-hidden="true">
                  <Sparkles :size="28" />
                </span>
                <p>创建您的第一个创作~</p>
                <p class="preview-hint">提交提示词后，生成结果会显示在这里</p>
              </div>

              <div v-if="task || regenerateResultLoading" class="preview-overlay">
                <PillTag tone="accent">{{ regenerateResultLoading ? '提交中' : '生成中' }}</PillTag>
                <strong>{{ regenerateResultLoading ? '正在提交再次生成...' : stageCopy?.title }}</strong>
                <p>{{ regenerateResultLoading ? '已收到点击，正在创建新的生成任务。' : stageCopy?.description }}</p>
                <button
                  v-if="canCancelGeneration && !regenerateResultLoading"
                  type="button"
                  class="secondary-button workspace-result-cancel-button"
                  data-testid="workspace-result-cancel-generation"
                  :disabled="cancelGenerationLoading"
                  @click.stop="$emit('cancel-generation')"
                >
                  {{ cancelGenerationLoading ? '取消中...' : '取消生成' }}
                </button>
              </div>
            </div>

            <div v-if="!resultError && previewAsset" class="preview-actions">
              <button
                class="secondary-button"
                type="button"
                data-testid="workspace-result-regenerate"
                :disabled="submitting || regenerateResultLoading"
                @click="$emit('regenerate-result')"
              >
                <span v-if="regenerateResultLoading" class="button-spinner" aria-hidden="true"></span>
                {{ regenerateResultLoading ? '提交中...' : '再次生成' }}
              </button>
              <button
                class="secondary-button"
                data-testid="workspace-result-use-reference"
                @click="$emit('use-result-as-reference')"
              >
                作为参考图
              </button>
              <button
                class="secondary-button"
                data-testid="workspace-result-favorite"
                @click="$emit('toggle-result-favorite')"
              >
                {{ previewAsset?.is_favorite ? '取消收藏' : '收藏' }}
              </button>
              <button
                class="secondary-button"
                data-testid="workspace-result-share"
                @click="$emit('share-result')"
              >
                分享
              </button>
              <button
                class="secondary-button"
                data-testid="workspace-result-open-library"
                @click="$emit('open-works-library')"
              >
                去作品库查看
              </button>
              <button class="secondary-button" @click="$emit('download-image')">下载</button>
              <button
                class="secondary-button"
                data-testid="workspace-preview-zoom-button"
                @click="$emit('open-preview-zoom')"
              >
                放大查看
              </button>
            </div>
            <p
              v-if="regenerateResultError"
              class="workspace-result-action-error"
              data-testid="workspace-result-regenerate-error"
              role="alert"
            >
              {{ regenerateResultError }}
            </p>
          </div>
        </section>

        <section v-if="activeTasks.length > 0" class="imini-section imini-generation-task-section" data-testid="workspace-generation-tasks">
          <div class="imini-section-head">
            <div class="imini-section-title">
              <span aria-hidden="true"><Sparkles :size="18" /></span>
              <h3>生成任务 ({{ activeTasks.length }})</h3>
            </div>
            <span class="imini-section-kicker">可切换查看</span>
          </div>
          <div class="imini-generation-task-list">
            <div
              v-for="item in activeTasks"
              :key="item.generation_id"
              class="imini-generation-task"
              :class="{
                active: isSelectedTask(item),
                failed: item.status === 'failed',
                succeeded: item.status === 'succeeded'
              }"
              :data-testid="`workspace-generation-task-${item.generation_id}`"
              @click="$emit('select-generation-task', item)"
            >
              <button type="button" class="imini-generation-task-select" @click="$emit('select-generation-task', item)">
                <span class="imini-generation-task-main">
                  <strong>{{ generationTaskSummary(item) }}</strong>
                  <span>{{ generationTaskLabel(item) }}</span>
                </span>
                <span class="imini-generation-task-meta">
                  {{ new Date(item.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) }}
                </span>
              </button>
              <button
                v-if="isActiveTask(item)"
                type="button"
                class="mini-button"
                :data-testid="`workspace-generation-task-${item.generation_id}-cancel`"
                :disabled="submitting || isCancellingTask(item)"
                @click.stop="$emit('cancel-generation-task', item)"
              >
                {{ isCancellingTask(item) ? '取消中' : '取消' }}
              </button>
              <button
                v-if="item.status === 'failed'"
                type="button"
                class="mini-button"
                :data-testid="`workspace-generation-task-${item.generation_id}-retry`"
                :disabled="submitting"
                @click.stop="$emit('retry-generation-task', item)"
              >
                重新生成
              </button>
            </div>
          </div>
        </section>

        <section class="history-section imini-history-section">
          <h3 class="history-title">生成记录（共 {{ worksDisplayTotal }}）</h3>
          <div v-if="worksError" class="workspace-inline-error" role="alert">
            <p class="error-message">{{ worksError }}</p>
            <button type="button" class="secondary-button" :disabled="worksLoading" @click="$emit('retry-works')">重试</button>
          </div>
          <div v-if="works.length === 0" class="history-empty">
            <p>暂无生成记录</p>
            <p class="history-hint">生成图片后，历史记录会显示在这里</p>
          </div>
          <div v-else class="history-grid">
            <div
              v-for="work in works"
              :key="work.work_id"
              class="history-card"
              @click="$emit('select-history-work', work)"
            >
              <div class="history-card-image">
                <img v-if="work.preview_url" :src="work.preview_url" :alt="work.prompt" />
                <span v-else class="history-card-placeholder">暂无预览</span>
              </div>
              <div class="history-card-content">
                <p class="history-card-prompt">{{ work.prompt }}</p>
                <div class="history-card-meta">
                  <span class="history-card-time">
                    {{ new Date(work.created_at).toLocaleString('zh-CN', {
                      month: 'numeric',
                      day: 'numeric',
                      hour: '2-digit',
                      minute: '2-digit'
                    }) }}
                  </span>
                  <span class="history-card-ratio">{{ work.aspect_ratio }}</span>
                </div>
                <button
                  type="button"
                  class="history-reference-button"
                  data-testid="workspace-history-use-as-reference"
                  :disabled="submitting"
                  @click.stop="$emit('use-work-as-reference', work)"
                >
                  设为参考
                </button>
              </div>
            </div>
          </div>
          <div v-if="showWorksPagination" class="history-pagination" data-testid="workspace-works-pagination">
            <button
              type="button"
              class="secondary-button history-page-button"
              data-testid="workspace-works-prev"
              :disabled="worksLoading || worksPage <= 1"
              @click="$emit('change-works-page', worksPage - 1)"
            >
              上一页
            </button>
            <span class="history-page-status" data-testid="workspace-works-page-status">
              第 {{ worksPage }} / {{ worksTotalPages }} 页
            </span>
            <button
              type="button"
              class="secondary-button history-page-button"
              data-testid="workspace-works-next"
              :disabled="worksLoading || worksPage >= worksTotalPages"
              @click="$emit('change-works-page', worksPage + 1)"
            >
              下一页
            </button>
          </div>
        </section>
      </div>
    </div>
  </main>
</template>
