<script setup>
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import {
  Activity,
  CheckCircle2,
  Clock3,
  Coins,
  Download,
  Expand,
  ExternalLink,
  History,
  Images,
  ListChecks,
  RotateCcw,
  ShieldCheck,
  X,
  XCircle,
} from "lucide-vue-next";
import showcase from "../../image/image-agent/commerce-showcase.png";
import { displayLabel } from "./commerceDisplayLabels.js";
import { commerceUserMessage } from "./commerceUserMessages.js";

const props = defineProps({
  theme: {
    type: String,
    default: "dark",
    validator: (value) => ["dark", "light"].includes(value),
  },
  mode: { type: String, default: "cases" },
  batches: { type: Array, default: () => [] },
  events: { type: Array, default: () => [] },
  assets: { type: Array, default: () => [] },
  creativeSpec: { type: Object, default: null },
  selectedSections: { type: Array, default: () => [] },
  aspectRatio: { type: String, default: "" },
  qualityTier: { type: String, default: "" },
  layoutTemplate: { type: String, default: "" },
  estimate: { type: Object, default: null },
  currentProject: { type: Object, default: null },
  loading: Boolean,
  error: String,
  definition: { type: Object, default: () => ({}) },
});
const emit = defineEmits(["mode", "cancel-batch", "cancel-item", "retry-item"]);

const dialog = ref("");
const fullscreen = ref(false);
const fullscreenTrigger = ref(null);
const casesTrigger = ref(null);
const historyTrigger = ref(null);
let previousBodyOverflow = "";

const statusLabels = {
  queued: "排队中",
  retrying: "重试中",
  running: "生成中",
  succeeded: "已完成",
  partial_succeeded: "部分完成",
  failed: "失败",
  canceled: "已取消",
  canceling: "取消中",
};
const terminalStatuses = new Set(["succeeded", "failed", "canceled"]);
const qualityLabels = { standard: "标准", high_fidelity: "高清" };
const layoutLabels = {
  clean: "简洁留白",
  dark_gradient: "深色渐变",
  brand_band: "品牌色带",
};
const eventLabels = {
  batch_created: "批次已创建",
  batch_started: "批次开始执行",
  batch_completed: "批次处理完成",
  item_queued: "章节进入队列",
  item_started: "章节开始生成",
  item_succeeded: "生成结果已保存",
  item_failed: "章节生成失败",
  item_retrying: "章节正在重试",
  item_canceled: "章节已取消",
  credits_reserved: "点数已预占",
  credits_settled: "点数已结算",
  credits_released: "未使用点数已释放",
};

const allItems = computed(() =>
  props.batches.flatMap((batch) => batch.items || []),
);
const latestBatch = computed(() => props.batches[0] || null);
const latestResult = computed(() => {
  for (const batch of props.batches) {
    const item = [...(batch.items || [])]
      .reverse()
      .find((candidate) => candidate.status === "succeeded" && candidate.work_id);
    if (item) return item;
  }
  return null;
});
const completedCount = computed(
  () =>
    allItems.value.filter((item) => terminalStatuses.has(item.status)).length,
);
const itemProgressOf = (item) =>
  terminalStatuses.has(item?.status)
    ? 100
    : Math.max(0, Math.min(100, Number(item?.progress_percent) || 0));
const progress = computed(() =>
  allItems.value.length
    ? Math.round(
        allItems.value.reduce((sum, item) => sum + itemProgressOf(item), 0) /
          allItems.value.length,
      )
    : 0,
);
const factCount = computed(
  () => Object.keys(props.creativeSpec?.product_facts || {}).length,
);
const forbiddenCount = computed(
  () => (props.creativeSpec?.forbidden_changes || []).length,
);
const latestEvents = computed(() => props.events.slice(-8).reverse());
const safeError = computed(() =>
  props.error ? commerceUserMessage(props.error) : "",
);

function statusOf(status) {
  return statusLabels[status] || "未知状态";
}
function sectionOf(item) {
  const key = item?.section || `${item?.slot_key || ""}`.split(":").at(-1);
  return displayLabel(props.definition.section_options, key, "未知章节");
}
const snapshotOf = (item) => item?.sku_snapshot || item?.sku || {};
const skuCodeOf = (item) => snapshotOf(item).code || item?.sku_code || "";
const skuPathOf = (item) =>
  item?.specification_path ||
  snapshotOf(item).specification_path ||
  snapshotOf(item).path ||
  snapshotOf(item).spec_path ||
  item?.sku_path ||
  item?.spec_path ||
  "";
const skuLabelOf = (item) =>
  skuCodeOf(item) === "DEFAULT"
    ? "默认规格"
    : skuPathOf(item) || skuCodeOf(item) || "未知规格";
function groupsOf(batch) {
  const groups = new Map();
  for (const item of batch?.items || []) {
    const key =
      item.scope === "shared"
        ? "shared"
        : `sku:${item.sku_id || skuCodeOf(item) || skuPathOf(item)}`;
    if (!groups.has(key))
      groups.set(key, {
        key,
        label: item.scope === "shared" ? "公共内容" : skuLabelOf(item),
        code:
          item.scope === "shared" || skuCodeOf(item) === "DEFAULT"
            ? ""
            : skuCodeOf(item),
        path: item.scope === "shared" ? "" : skuPathOf(item),
        items: [],
      });
    groups.get(key).items.push(item);
  }
  return [...groups.values()];
}
function etaOf(batch) {
  if (batch?.eta_seconds > 0) return `${batch.eta_seconds} 秒`;
  return ["succeeded", "partial_succeeded", "failed", "canceled"].includes(
    batch?.status,
  )
    ? "已结束"
    : "计算中";
}
function progressOf(batch) {
  const items = batch?.items || [];
  if (!items.length)
    return ["succeeded", "failed", "canceled"].includes(batch?.status)
      ? 100
      : 0;
  return Math.round(
    items.reduce((sum, item) => sum + itemProgressOf(item), 0) / items.length,
  );
}
function eventOf(event) {
  return eventLabels[event?.event_type] || "任务状态已更新";
}
function eventTime(event) {
  const time = Date.parse(event?.created_at || "");
  return Number.isFinite(time)
    ? new Date(time).toLocaleTimeString("zh-CN", {
        hour: "2-digit",
        minute: "2-digit",
      })
    : "";
}
function estimatedCredits() {
  return (
    props.estimate?.estimated_credits ??
    latestBatch.value?.reserved_credits ??
    latestBatch.value?.held_credits ??
    0
  );
}

function openDialog(name, trigger) {
  dialog.value = name;
  if (name === "cases")
    casesTrigger.value = trigger?.currentTarget || document.activeElement;
  if (name === "history")
    historyTrigger.value = trigger?.currentTarget || document.activeElement;
}
function closeDialog() {
  const trigger =
    dialog.value === "cases" ? casesTrigger.value : historyTrigger.value;
  dialog.value = "";
  nextTick(() => trigger?.focus?.());
}
function openFullscreen(event) {
  fullscreenTrigger.value = event?.currentTarget || document.activeElement;
  fullscreen.value = true;
}
function closeFullscreen() {
  fullscreen.value = false;
  nextTick(() => fullscreenTrigger.value?.focus?.());
}
function onKeydown(event) {
  if (event.key !== "Escape") return;
  if (dialog.value) closeDialog();
  else if (fullscreen.value) closeFullscreen();
}
function syncBodyLock() {
  const locked = Boolean(dialog.value || fullscreen.value);
  if (locked && document.body.style.overflow !== "hidden") {
    previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  } else if (!locked) document.body.style.overflow = previousBodyOverflow;
}

watch([dialog, fullscreen], syncBodyLock);
onMounted(() => window.addEventListener("keydown", onKeydown));
onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKeydown);
  document.body.style.overflow = previousBodyOverflow;
});
</script>

<template>
  <section
    class="result-panel production-console"
    data-testid="commerce-production-console"
    :data-theme="theme"
  >
    <header class="console-toolbar">
      <div>
        <small>实时工作区</small>
        <h2>生产控制台</h2>
      </div>
      <nav aria-label="控制台操作">
        <button
          ref="casesTrigger"
          type="button"
          data-testid="commerce-open-cases"
          @click="openDialog('cases', $event)"
        >
          <Images :size="16" />案例库
        </button>
        <button
          ref="historyTrigger"
          type="button"
          data-testid="commerce-open-history"
          @click="openDialog('history', $event)"
        >
          <History :size="16" />历史记录
        </button>
        <button
          ref="fullscreenTrigger"
          type="button"
          class="fullscreen-action"
          data-testid="commerce-open-fullscreen"
          @click="openFullscreen"
        >
          <Expand :size="16" />全屏
        </button>
      </nav>
    </header>

    <p v-if="safeError" class="console-error" role="alert">{{ safeError }}</p>

    <div
      v-if="!currentProject || !batches.length"
      class="readiness-card"
      data-testid="commerce-console-readiness"
    >
      <div class="readiness-title">
        <ShieldCheck :size="21" />
        <div>
          <h3>生产准备检查</h3>
          <p>左侧完成创作配置后，任务和效果会实时出现在这里。</p>
        </div>
      </div>
      <ol>
        <li :class="{ done: currentProject }">
          <CheckCircle2 :size="15" /><span>{{
            currentProject ? "商品项目已创建" : "创建商品项目"
          }}</span>
        </li>
        <li :class="{ done: assets.length }">
          <CheckCircle2 :size="15" /><span>{{
            assets.length ? `已上传 ${assets.length} 张素材` : "上传商品素材"
          }}</span>
        </li>
        <li :class="{ done: creativeSpec?.status === 'confirmed' }">
          <CheckCircle2 :size="15" /><span>{{
            creativeSpec?.status === "confirmed"
              ? "商品报告已确认"
              : "生成并确认商品报告"
          }}</span>
        </li>
        <li :class="{ done: estimate }">
          <CheckCircle2 :size="15" /><span>{{
            estimate ? "点数估价已完成" : "配置详情页并完成估价"
          }}</span>
        </li>
      </ol>
    </div>

    <div
      class="console-card-grid"
      :class="{ 'has-readiness': !currentProject || !batches.length }"
    >
      <article class="console-card" data-testid="commerce-console-card-queue">
        <header>
          <ListChecks :size="17" />
          <h3>任务队列</h3>
          <span>{{
            latestBatch
              ? statusOf(latestBatch.status)
              : `${completedCount}/${allItems.length}`
          }}</span>
        </header>
        <div v-if="allItems.length" class="compact-list">
          <div v-for="item in allItems.slice(0, 6)" :key="item.id">
            <b>{{ item.scope === "shared" ? "公共内容" : skuLabelOf(item) }}<template v-if="item.scope !== 'shared' && skuCodeOf(item) && skuCodeOf(item) !== 'DEFAULT'"> · {{ skuCodeOf(item) }}</template> · {{ sectionOf(item) }}</b
            ><span :class="`status-${item.status}`">{{
              statusOf(item.status)
            }} · {{ itemProgressOf(item) }}% · {{ item.estimated_credits ?? item.settled_credits ?? 0 }} 点</span>
          </div>
        </div>
        <p v-else class="card-empty">提交生成后显示章节任务。</p>
      </article>

      <article class="console-card" data-testid="commerce-console-card-events">
        <header>
          <Activity :size="17" />
          <h3>实时事件</h3>
          <span>{{ events.length }}</span>
        </header>
        <div v-if="latestEvents.length" class="event-list">
          <p v-for="event in latestEvents" :key="event.id">
            <time>{{ eventTime(event) }}</time
            >{{ eventOf(event) }}
          </p>
        </div>
        <p v-else class="card-empty">暂无执行事件。</p>
      </article>

      <article class="console-card" data-testid="commerce-console-card-inputs">
        <header>
          <ShieldCheck :size="17" />
          <h3>输入与约束</h3>
        </header>
        <dl>
          <div>
            <dt>素材</dt>
            <dd>{{ assets.length }} 张</dd>
          </div>
          <div>
            <dt>商品事实</dt>
            <dd>{{ factCount }} 项</dd>
          </div>
          <div>
            <dt>禁改内容</dt>
            <dd>{{ forbiddenCount }} 项</dd>
          </div>
          <div>
            <dt>报告</dt>
            <dd>
              {{ creativeSpec?.status === "confirmed" ? "已确认" : "待确认" }}
            </dd>
          </div>
        </dl>
        <p class="config-line">
          {{ selectedSections.length }} 个章节 ·
          {{ aspectRatio || "待选择画幅" }} ·
          {{ qualityLabels[qualityTier] || "待选择质量" }} ·
          {{ layoutLabels[layoutTemplate] || "待选择版式" }}
        </p>
      </article>

      <article class="console-card" data-testid="commerce-console-card-costs">
        <header>
          <Coins :size="17" />
          <h3>成本与进度</h3>
          <span>{{ progress }}%</span>
        </header>
        <div class="console-progress">
          <i :style="{ width: `${progress}%` }"></i>
        </div>
        <dl>
          <div>
            <dt>预计</dt>
            <dd>{{ estimatedCredits() }} 点</dd>
          </div>
          <div>
            <dt>已结算</dt>
            <dd>{{ latestBatch?.settled_credits ?? 0 }} 点</dd>
          </div>
          <div>
            <dt>已释放</dt>
            <dd>{{ latestBatch?.released_credits ?? 0 }} 点</dd>
          </div>
          <div>
            <dt>剩余时间</dt>
            <dd>{{ etaOf(latestBatch) }}</dd>
          </div>
        </dl>
      </article>

      <article
        class="console-card latest-result-card"
        data-testid="commerce-console-card-latest"
      >
        <header>
          <Images :size="17" />
          <h3>最新结果</h3>
          <span v-if="latestResult">{{ sectionOf(latestResult) }}</span>
        </header>
        <div v-if="latestResult" class="latest-result-body">
          <img
            :src="`/api/works/${latestResult.work_id}/file`"
            :alt="`${sectionOf(latestResult)}生成结果`"
          />
          <div>
            <b>{{ sectionOf(latestResult) }}</b>
            <p>
              {{
                latestResult.output_snapshot?.output_size || "尺寸以作品为准"
              }}
              · {{ layoutLabels[layoutTemplate] || "详情页版式" }}
            </p>
            <div class="result-actions">
              <a
                :href="`/api/works/${latestResult.work_id}/file`"
                target="_blank"
                ><ExternalLink :size="15" />打开预览</a
              ><a :href="`/api/works/${latestResult.work_id}/download`"
                ><Download :size="15" />下载</a
              >
            </div>
          </div>
        </div>
        <p v-else class="card-empty">首个章节完成后显示最新效果。</p>
      </article>
    </div>

    <Teleport to="body">
      <div
        v-if="fullscreen"
        class="commerce-overlay fullscreen-overlay"
        :data-theme="theme"
        data-testid="commerce-fullscreen-console"
        role="dialog"
        aria-modal="true"
        aria-label="全屏生产控制台"
      >
        <div class="fullscreen-shell">
          <header>
            <div>
              <small>{{ currentProject?.title || "AI 商品详情页" }}</small>
              <h2>全屏生产控制台</h2>
            </div>
            <div class="overlay-actions">
              <button type="button" @click="openDialog('cases', $event)">
                <Images :size="16" />案例库</button
              ><button type="button" @click="openDialog('history', $event)">
                <History :size="16" />历史记录</button
              ><button
                type="button"
                aria-label="关闭全屏控制台"
                @click="closeFullscreen"
              >
                <X :size="19" />
              </button>
            </div>
          </header>
          <div class="fullscreen-grid">
            <section>
              <h3>批次与章节任务</h3>
              <article
                v-for="batch in batches"
                :key="batch.id"
                class="fullscreen-batch"
              >
                <header>
                  <b>批次 #{{ batch.id }}</b
                  ><span>{{ statusOf(batch.status) }}</span>
                </header>
                <div
                  v-for="item in batch.items || []"
                  :key="item.id"
                  class="fullscreen-item"
                >
                  <b>{{ sectionOf(item) }}</b
                  ><span>{{ statusOf(item.status) }}</span>
                </div>
              </article>
              <p v-if="!batches.length" class="card-empty">暂无批次。</p>
            </section>
            <section class="fullscreen-preview">
              <h3>最新生成效果</h3>
              <img
                v-if="latestResult"
                :src="`/api/works/${latestResult.work_id}/file`"
                :alt="`${sectionOf(latestResult)}生成结果`"
              />
              <p v-else class="card-empty">等待首个生成结果。</p>
              <div v-if="latestResult" class="result-actions">
                <a
                  :href="`/api/works/${latestResult.work_id}/file`"
                  target="_blank"
                  >打开预览</a
                ><a :href="`/api/works/${latestResult.work_id}/download`"
                  >下载</a
                >
              </div>
            </section>
            <section>
              <h3>生产诊断</h3>
              <dl>
                <div>
                  <dt>素材</dt>
                  <dd>{{ assets.length }} 张</dd>
                </div>
                <div>
                  <dt>报告</dt>
                  <dd>
                    {{
                      creativeSpec?.status === "confirmed" ? "已确认" : "待确认"
                    }}
                  </dd>
                </div>
                <div>
                  <dt>预计点数</dt>
                  <dd>{{ estimatedCredits() }} 点</dd>
                </div>
                <div>
                  <dt>已结算</dt>
                  <dd>{{ latestBatch?.settled_credits ?? 0 }} 点</dd>
                </div>
                <div>
                  <dt>已释放</dt>
                  <dd>{{ latestBatch?.released_credits ?? 0 }} 点</dd>
                </div>
                <div>
                  <dt>完成进度</dt>
                  <dd>{{ progress }}%</dd>
                </div>
              </dl>
              <h3>最近事件</h3>
              <div class="event-list">
                <p v-for="event in latestEvents" :key="event.id">
                  <time>{{ eventTime(event) }}</time
                  >{{ eventOf(event) }}
                </p>
              </div>
            </section>
          </div>
        </div>
      </div>

      <div
        v-if="dialog"
        class="commerce-overlay dialog-overlay"
        :data-theme="theme"
        :data-testid="
          dialog === 'cases'
            ? 'commerce-cases-dialog'
            : 'commerce-history-dialog'
        "
        role="dialog"
        aria-modal="true"
        :aria-label="dialog === 'cases' ? '案例库' : '历史记录'"
      >
        <div class="commerce-dialog">
          <header>
            <div>
              <small>AI 商品详情页</small>
              <h2>{{ dialog === "cases" ? "案例库" : "历史记录" }}</h2>
            </div>
            <button
              type="button"
              data-testid="commerce-dialog-close"
              :aria-label="`关闭${dialog === 'cases' ? '案例库' : '历史记录'}`"
              @click="closeDialog"
            >
              <X :size="19" />
            </button>
          </header>
          <div v-if="dialog === 'cases'" class="case-library">
            <div class="case-copy">
              <p class="eyebrow">详情页案例</p>
              <h2>高转化商品详情，<br />从一组好素材开始</h2>
              <p>上传真实商品图，AI 先分析，再按章节生成完整详情页。</p>
            </div>
            <figure>
              <img :src="showcase" alt="AI 商品详情页案例" />
              <figcaption>
                <span>生活方式 · 轻盈简约</span
                ><b>{{ definition.sections?.length || "—" }} 个章节</b>
              </figcaption>
            </figure>
            <div class="case-stats">
              <span
                ><b>{{ definition.sections?.length || "—" }}</b
                >章节独立生成</span
              ><span
                ><b>{{ definition.aspect_ratios?.length || "—" }}</b
                >种电商画幅</span
              ><span><b>1</b>次批量提交</span>
            </div>
          </div>
          <div v-else class="history-list">
            <p v-if="safeError" role="alert">{{ safeError }}</p>
            <div v-if="loading" role="status">正在恢复生成记录…</div>
            <div v-else-if="!batches.length" class="empty-state">
              <History :size="30" /><b>还没有生成记录</b
              ><span>完成配置并提交后，结果会出现在这里。</span>
            </div>
            <article
              v-for="batch in batches"
              :key="batch.id"
              class="result-batch"
            >
              <header>
                <div>
                  <small>批次 #{{ batch.id }}</small
                  ><b>{{ statusOf(batch.status) }}</b>
                </div>
                <span
                  >{{ progressOf(batch) }}% · 预计剩余时间
                  {{ etaOf(batch) }}</span
                >
              </header>
              <div class="progress">
                <i :style="{ width: `${progressOf(batch)}%` }"></i>
              </div>
              <p>
                {{ batch.total_items ?? batch.items?.length ?? 0 }} 项 · 已结算
                {{ batch.settled_credits ?? 0 }} 点 · 已释放
                {{ batch.released_credits ?? 0 }} 点
              </p>
              <button
                v-if="['queued', 'running'].includes(batch.status)"
                @click="emit('cancel-batch', batch)"
              >
                <XCircle :size="16" />取消批次
              </button>
              <section
                v-for="group in groupsOf(batch)"
                :key="group.key"
                class="result-group"
              >
                <h3>{{ group.label }}</h3>
                <p v-if="group.path && group.path !== group.label">
                  {{ group.path }}
                </p>
                <code v-if="group.code">{{ group.code }}</code>
                <div class="result-items">
                <div v-for="item in group.items" :key="item.id">
                  <div>
                    <b>{{ sectionOf(item) }}</b
                    ><span>{{ statusOf(item.status) }} · {{ itemProgressOf(item) }}%</span>
                  </div>
                  <p v-if="item.error_message || item.error_code" role="alert">
                    {{
                      commerceUserMessage(item.error_code, "生成失败，请重试")
                    }}
                  </p>
                  <button
                    v-if="
                      ['queued', 'running', 'retrying'].includes(item.status)
                    "
                    @click="emit('cancel-item', item)"
                  >
                    取消</button
                  ><button
                    v-if="item.status === 'failed'"
                    @click="emit('retry-item', item)"
                  >
                    <RotateCcw :size="15" />重试</button
                  ><a
                    v-if="item.work_id"
                    :href="`/api/works/${item.work_id}/file`"
                    target="_blank"
                    ><ExternalLink :size="15" />预览</a
                  ><a
                    v-if="item.work_id"
                    :href="`/api/works/${item.work_id}/download`"
                    ><Download :size="15" />下载</a
                  >
                </div>
                </div>
              </section>
            </article>
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>

<style scoped>
.production-console {
  min-height: 100%;
  color: #f5f7f8;
}
.console-toolbar {
  position: sticky;
  top: 0;
  z-index: 3;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 13px 14px;
  border-bottom: 1px solid var(--commerce-border);
  background: #101317ed;
  backdrop-filter: blur(12px);
}
.console-toolbar small,
.fullscreen-shell > header small,
.commerce-dialog > header small {
  color: var(--commerce-muted);
  font-size: 10px;
}
.console-toolbar h2,
.fullscreen-shell h2,
.commerce-dialog h2 {
  margin: 2px 0 0;
  font-size: 16px;
}
.console-toolbar nav {
  display: flex;
  gap: 6px;
  padding: 0;
  border: 0;
  background: none;
  backdrop-filter: none;
}
.console-toolbar nav button {
  min-height: 34px;
  height: 34px;
  padding: 6px 9px;
  border: 1px solid #343d46;
  border-radius: 8px;
  background: #151a20;
  color: #dce2e7;
  font-size: 11px;
}
.console-toolbar nav button:hover {
  border-color: #52616e;
}
.console-toolbar .fullscreen-action {
  color: var(--commerce-accent);
}
.console-error {
  margin: 12px 12px 0;
  padding: 10px;
  border-radius: 8px;
  background: #32151b;
  color: #ff9eaa;
}
.readiness-card {
  margin: 12px;
  padding: 14px;
  border: 1px solid #344028;
  border-radius: 12px;
  background: linear-gradient(135deg, #11180d, #0b0f11);
}
.readiness-title {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
.readiness-title svg {
  color: var(--commerce-accent);
}
.readiness-card h3 {
  margin: 0;
  font-size: 14px;
}
.readiness-card p {
  margin: 3px 0 0;
  color: var(--commerce-muted);
  font-size: 11px;
}
.readiness-card ol {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 7px;
  list-style: none;
  margin: 13px 0 0;
  padding: 0;
}
.readiness-card li {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 8px;
  border-radius: 8px;
  background: #0a0e11;
  color: var(--commerce-muted);
  font-size: 11px;
}
.readiness-card li.done {
  color: #dce4e9;
}
.readiness-card li.done svg {
  color: var(--commerce-accent);
}
.console-card-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  padding: 12px;
}
.console-card {
  min-width: 0;
  border: 1px solid #293039;
  border-radius: 12px;
  background: #0b0e11;
  padding: 12px;
}
.console-card > header {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-bottom: 10px;
}
.console-card > header svg {
  color: var(--commerce-accent);
}
.console-card h3 {
  margin: 0;
  font-size: 12px;
}
.console-card > header > span {
  margin-left: auto;
  color: var(--commerce-muted);
  font-size: 10px;
}
.compact-list > div {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  padding: 7px 0;
  border-top: 1px solid #242b31;
  font-size: 10px;
}
.compact-list > div:first-child {
  border-top: 0;
}
.compact-list span {
  color: var(--commerce-muted);
}
.compact-list .status-succeeded {
  color: var(--commerce-accent);
}
.compact-list .status-running,
.compact-list .status-retrying {
  color: #ffd276;
}
.compact-list .status-failed {
  color: #ff8795;
}
.event-list p {
  display: flex;
  gap: 7px;
  margin: 0;
  padding: 6px 0;
  border-left: 2px solid #36414a;
  color: #c1c8ce;
  font-size: 10px;
}
.event-list time {
  min-width: 34px;
  padding-left: 7px;
  color: var(--commerce-muted);
}
.console-card dl,
.fullscreen-grid dl {
  margin: 0;
}
.console-card dl > div,
.fullscreen-grid dl > div {
  display: flex;
  justify-content: space-between;
  padding: 6px 0;
  border-top: 1px solid #242b31;
  font-size: 10px;
}
.console-card dl > div:first-child,
.fullscreen-grid dl > div:first-child {
  border-top: 0;
}
.console-card dt,
.fullscreen-grid dt {
  color: var(--commerce-muted);
}
.console-card dd,
.fullscreen-grid dd {
  margin: 0;
}
.config-line {
  margin: 8px 0 0;
  color: var(--commerce-muted);
  font-size: 9px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.console-progress {
  height: 5px;
  margin: 5px 0 8px;
  border-radius: 4px;
  background: #252d33;
  overflow: hidden;
}
.console-progress i {
  display: block;
  height: 100%;
  border-radius: 4px;
  background: var(--commerce-accent);
}
.latest-result-card {
  grid-column: 1/-1;
}
.latest-result-body {
  display: grid;
  grid-template-columns: minmax(150px, 1.15fr) minmax(130px, 0.85fr);
  gap: 12px;
  align-items: center;
}
.latest-result-body img {
  width: 100%;
  height: clamp(130px, 24vh, 260px);
  object-fit: contain;
  border-radius: 9px;
  background: #07090c;
}
.latest-result-body b {
  font-size: 13px;
}
.latest-result-body p {
  color: var(--commerce-muted);
  font-size: 10px;
}
.result-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}
.result-actions a {
  min-height: 32px;
  padding: 6px 9px;
  font-size: 10px;
}
.card-empty {
  margin: 8px 0;
  color: var(--commerce-muted);
  font-size: 11px;
}
.commerce-overlay {
  position: fixed;
  inset: 0;
  display: grid;
  background: #030507db;
  backdrop-filter: blur(10px);
}
.fullscreen-overlay {
  z-index: 1000;
  padding: 14px;
}
.fullscreen-shell {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  border: 1px solid #35404a;
  border-radius: 16px;
  background: #0b0e12;
  overflow: hidden;
}
.fullscreen-shell > header,
.commerce-dialog > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid #293039;
}
.overlay-actions {
  display: flex;
  gap: 7px;
}
.overlay-actions button,
.commerce-dialog > header button {
  min-height: 36px;
  border: 1px solid #35404a;
  border-radius: 8px;
  background: #151a20;
  color: #eef2f4;
}
.fullscreen-grid {
  display: grid;
  grid-template-columns: minmax(220px, 0.8fr) minmax(360px, 1.5fr) minmax(
      240px,
      0.9fr
    );
  gap: 12px;
  min-height: 0;
  flex: 1;
  padding: 12px;
}
.fullscreen-grid > section {
  min-width: 0;
  overflow: auto;
  border: 1px solid #293039;
  border-radius: 12px;
  background: #101317;
  padding: 14px;
}
.fullscreen-grid h3 {
  margin: 0 0 11px;
  font-size: 13px;
}
.fullscreen-batch {
  margin-bottom: 10px;
  border: 1px solid #293039;
  border-radius: 9px;
  padding: 9px;
}
.fullscreen-batch > header,
.fullscreen-item {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 10px;
}
.fullscreen-item {
  padding: 7px 0;
  border-top: 1px solid #242b31;
}
.fullscreen-preview {
  display: flex;
  flex-direction: column;
}
.fullscreen-preview img {
  min-height: 0;
  flex: 1;
  width: 100%;
  object-fit: contain;
  border-radius: 10px;
  background: #07090c;
}
.fullscreen-preview .result-actions {
  margin-top: 10px;
}
.dialog-overlay {
  z-index: 1100;
  place-items: center;
  padding: 24px;
}
.commerce-dialog {
  width: min(980px, calc(100vw - 48px));
  max-height: calc(100vh - 48px);
  border: 1px solid #35404a;
  border-radius: 16px;
  background: #101317;
  overflow: auto;
}
.commerce-dialog .case-library,
.commerce-dialog .history-list {
  padding: 24px;
}
.commerce-dialog .case-library {
  display: grid;
  grid-template-columns: 0.8fr 1.2fr;
  gap: 20px;
}
.commerce-dialog .case-library figure {
  margin: 0;
}
.commerce-dialog .case-stats {
  grid-column: 1/-1;
}
.commerce-dialog .case-copy h2 {
  font-size: 30px;
  line-height: 1.05;
  margin: 12px 0;
}
.commerce-dialog .case-copy > p:last-child {
  color: var(--commerce-muted);
}
@media (max-width: 767px) {
  .console-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
  .console-toolbar nav {
    width: 100%;
  }
  .console-toolbar nav button {
    flex: 1;
  }
  .console-toolbar .fullscreen-action {
    display: none;
  }
  .readiness-card ol,
  .console-card-grid {
    grid-template-columns: 1fr;
  }
  .latest-result-card {
    grid-column: auto;
  }
  .latest-result-body {
    grid-template-columns: 1fr;
  }
  .latest-result-body img {
    height: 220px;
  }
  .dialog-overlay {
    padding: 0;
  }
  .commerce-dialog {
    width: 100vw;
    max-height: 100vh;
    height: 100vh;
    border: 0;
    border-radius: 0;
  }
  .commerce-dialog .case-library {
    grid-template-columns: 1fr;
    padding: 18px;
  }
  .commerce-dialog .case-stats {
    grid-column: auto;
  }
  .commerce-dialog .history-list {
    padding: 14px;
  }
}
.commerce-overlay {
  --commerce-accent: #b7ff2a;
  --commerce-border: #293039;
  --commerce-muted: #8d969f;
  color: #f5f7f8;
}
.commerce-overlay button,
.commerce-overlay a {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 36px;
  border: 1px solid #38414a;
  border-radius: 8px;
  background: #151a20;
  color: #edf0f2;
  padding: 7px 10px;
  cursor: pointer;
  text-decoration: none;
}
.commerce-overlay button:focus-visible,
.commerce-overlay a:focus-visible {
  outline: 2px solid var(--commerce-accent);
  outline-offset: 2px;
}

.production-console,
.commerce-overlay {
  color-scheme: dark;
  --commerce-accent: #b7ff2a;
  --commerce-accent-fill: #b7ff2a;
  --commerce-border: #293039;
  --commerce-border-strong: #38414a;
  --commerce-muted: #8d969f;
  --commerce-text: #f5f7f8;
  --commerce-text-secondary: #c1c8ce;
  --commerce-surface: #101317;
  --commerce-surface-raised: #151a20;
  --commerce-surface-subtle: #0b0e11;
  --commerce-media-bg: #07090c;
  --commerce-accent-soft: #18210e;
  --commerce-readiness-bg: linear-gradient(135deg, #11180d, #0b0f11);
  --commerce-progress-bg: #252d33;
  --commerce-overlay-bg: rgba(3, 5, 7, .86);
  --commerce-danger-bg: #32151b;
  --commerce-danger-text: #ff9eaa;
  --commerce-warning-text: #ffd276;
  --commerce-shadow: 0 22px 65px rgba(0, 0, 0, .44);
}
.production-console[data-theme="light"],
.commerce-overlay[data-theme="light"] {
  color-scheme: light;
  --commerce-accent: #4d7c0f;
  --commerce-accent-fill: #84cc16;
  --commerce-border: #d6dde7;
  --commerce-border-strong: #bdc8d5;
  --commerce-muted: #667085;
  --commerce-text: #17202c;
  --commerce-text-secondary: #475467;
  --commerce-surface: #ffffff;
  --commerce-surface-raised: #f5f7fa;
  --commerce-surface-subtle: #f8fafc;
  --commerce-media-bg: #eef2f6;
  --commerce-accent-soft: #eff8dc;
  --commerce-readiness-bg: linear-gradient(135deg, #f7fbea, #ffffff);
  --commerce-progress-bg: #e4e9ef;
  --commerce-overlay-bg: rgba(15, 23, 42, .42);
  --commerce-danger-bg: #fff0f1;
  --commerce-danger-text: #b42336;
  --commerce-warning-text: #92610a;
  --commerce-shadow: 0 22px 55px rgba(15, 23, 42, .16);
}
.production-console { color: var(--commerce-text); }
.console-toolbar { background: color-mix(in srgb, var(--commerce-surface) 94%, transparent); }
.console-toolbar nav button,
.overlay-actions button,
.commerce-dialog > header button,
.commerce-overlay button,
.commerce-overlay a { background: var(--commerce-surface-raised); color: var(--commerce-text); border-color: var(--commerce-border-strong); }
.console-toolbar nav button:hover { border-color: var(--commerce-accent); }
.console-error { background: var(--commerce-danger-bg); color: var(--commerce-danger-text); }
.readiness-card { background: var(--commerce-readiness-bg); border-color: var(--commerce-border); }
.readiness-card li,
.console-card,
.fullscreen-grid > section,
.commerce-dialog { background: var(--commerce-surface); border-color: var(--commerce-border); }
.readiness-card li,
.result-batch { background: var(--commerce-surface-subtle); }
.readiness-card li.done,
.event-list p { color: var(--commerce-text-secondary); }
.console-card,
.compact-list > div,
.event-list p,
.console-card dl > div,
.fullscreen-grid dl > div,
.fullscreen-item,
.commerce-dialog > header { border-color: var(--commerce-border); }
.console-progress { background: var(--commerce-progress-bg); }
.latest-result-body img,
.fullscreen-preview img { background: var(--commerce-media-bg); }
.compact-list .status-running,
.compact-list .status-retrying { color: var(--commerce-warning-text); }
.compact-list .status-failed { color: var(--commerce-danger-text); }
.commerce-overlay { background: var(--commerce-overlay-bg); color: var(--commerce-text); }
.fullscreen-shell,
.commerce-dialog { background: var(--commerce-surface); border-color: var(--commerce-border-strong); box-shadow: var(--commerce-shadow); }
.fullscreen-grid > section { background: var(--commerce-surface-subtle); }
.commerce-dialog .case-library figure { background: var(--commerce-media-bg); border-color: var(--commerce-border); }
.commerce-dialog .case-stats span { background: var(--commerce-surface-subtle); }
.commerce-dialog .case-stats b { color: var(--commerce-text); }
</style>
