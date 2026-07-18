<script setup>
import { onMounted, ref } from 'vue'

import { api } from '../api/client.js'
import cityImage from '../image/image-agent/city.png'
import interiorImage from '../image/image-agent/interior.png'
import landscapeImage from '../image/image-agent/landscape.png'
import portraitImage from '../image/image-agent/portrait.png'
import productImage from '../image/image-agent/commerce-showcase.png'
import ribbonImage from '../image/image-agent/ribbon.png'

const workflowChips = [
  { icon: '〽', label: '持续生成工作台' },
  { icon: '▣', label: '默认私有作品库' },
  { icon: '▱', label: '结果自动入库' },
  { icon: 'ϟ', label: '人工充值继续生产' }
]

const outputTags = [
  { icon: '◉', label: '商品主图' },
  { icon: '▱', label: '封面图' },
  { icon: '◰', label: '社媒配图' },
  { icon: '□', label: '海报 KV' },
  { icon: '✣', label: '分镜参考' }
]

const visualMetrics = [
  { value: '∞', label: '持续生成' },
  { value: '100%', label: '入库留存' },
  { value: '高效', label: '复用交付' }
]

const visualCards = [
  { className: 'card-landscape', src: landscapeImage, alt: 'AI 山景作品' },
  { className: 'card-portrait', src: portraitImage, alt: 'AI 人像作品' },
  { className: 'card-product', src: productImage, alt: 'AI 商品图作品' },
  { className: 'card-city', src: cityImage, alt: 'AI 城市场景作品' },
  { className: 'card-interior', src: interiorImage, alt: 'AI 室内场景作品' }
]

const coreModules = [
  { icon: 'ϟ', title: '持续生成工作台', description: '高效批量生成，灵感转化不间断' },
  { icon: '▣', title: '智能入库管理', description: '自动入库，作品资产化沉淀' },
  { icon: '▰', title: '多场景模板库', description: '覆盖主图、封面、海报等场景' },
  { icon: '¥', title: '灵活充值机制', description: '人工充值，按需持续生产' },
  { icon: '⌂', title: '安全私有空间', description: '默认私有，数据更安心' }
]

const isLoggedIn = ref(false)

onMounted(async () => {
  try {
    const me = await api.getMe()
    isLoggedIn.value = Boolean(me?.username)
  } catch {
    isLoggedIn.value = false
  }
})
</script>

<template>
  <div class="agent-home">
    <section class="agent-home-hero" aria-labelledby="agent-home-title">
      <div class="agent-home-copy">
        <p class="agent-home-kicker">
          <span aria-hidden="true"></span>
          CREATOR PORTAL
        </p>

        <h1 id="agent-home-title">
          <span>为内容创作者</span>
          <span>打造的持续生成平台</span>
        </h1>

        <p class="agent-home-lead">
          把提示词、模板、作品沉淀与人工充值打通成一条高效工作流，
          让封面图、商品主图、海报 KV 和社媒配图都能稳定产出。
        </p>

        <div class="agent-home-actions" aria-label="主要操作">
          <RouterLink class="agent-home-primary" :to="isLoggedIn ? '/workspace' : '/register'">
            {{ isLoggedIn ? '进入工作台' : '注册并进入工作台' }}
            <span aria-hidden="true">→</span>
          </RouterLink>
          <RouterLink
            class="agent-home-couple"
            data-testid="home-couple-album-entry"
            :to="isLoggedIn ? '/workspace/couple-album' : '/register'"
          >
            生成情侣相册
          </RouterLink>
          <RouterLink class="agent-home-secondary" to="/pricing">查看套餐与充值</RouterLink>
        </div>

        <div class="agent-home-workflow" aria-label="工作流能力">
          <span v-for="item in workflowChips" :key="item.label">
            <b aria-hidden="true">{{ item.icon }}</b>
            {{ item.label }}
          </span>
        </div>

        <div class="agent-home-tags" aria-label="适用输出类型">
          <span v-for="item in outputTags" :key="item.label">
            <b aria-hidden="true">{{ item.icon }}</b>
            {{ item.label }}
          </span>
        </div>
      </div>

      <aside
        class="agent-home-visual"
        :style="{ '--agent-ribbon-image': `url(${ribbonImage})` }"
        aria-label="白霖共享 平台演示"
      >
        <div class="agent-home-visual-copy">
          <span class="agent-home-visual-badge">白霖共享</span>
          <h2>
            <span>生成、入库、</span><span>复用、交付</span>
            <span class="agent-home-spark" aria-hidden="true">✦</span>
          </h2>
          <p>不是试玩页，而是给创作者长期使用的轻门户生产前台。</p>

          <dl class="agent-home-metrics" aria-label="平台指标">
            <div v-for="item in visualMetrics" :key="item.label">
              <dt>{{ item.value }}</dt>
              <dd>{{ item.label }}</dd>
            </div>
          </dl>

          <RouterLink class="agent-home-demo" to="/workspace">
            <span aria-hidden="true">▶</span>
            观看平台演示
          </RouterLink>
        </div>

        <div class="agent-home-visual-stage" aria-hidden="true">
          <figure
            v-for="item in visualCards"
            :key="item.className"
            :class="['agent-home-visual-card', item.className]"
          >
            <img :src="item.src" :alt="item.alt">
          </figure>
          <span class="agent-home-floating-badge badge-ai">AI</span>
          <span class="agent-home-floating-badge badge-image">▰</span>
        </div>
      </aside>
    </section>

    <section class="agent-home-core" aria-labelledby="agent-home-core-title">
      <p class="agent-home-section-kicker">CORE MODULES <span aria-hidden="true">✦</span></p>
      <h2 id="agent-home-core-title">核心能力，覆盖创作全流程</h2>

      <div class="agent-home-core-grid">
        <article v-for="item in coreModules" :key="item.title" class="agent-home-module">
          <span class="agent-home-module-icon" aria-hidden="true">{{ item.icon }}</span>
          <div>
            <h3>{{ item.title }}</h3>
            <p>{{ item.description }}</p>
          </div>
        </article>
      </div>
    </section>

    <footer class="agent-home-footer">
      <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">
        蜀ICP备2026023334号
      </a>
    </footer>
  </div>
</template>
