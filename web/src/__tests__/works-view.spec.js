import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'

const routerPush = vi.hoisted(() => vi.fn())
const routeQuery = vi.hoisted(() => ({}))
const apiMocks = vi.hoisted(() => ({
  listWorks: vi.fn(),
  listCoupleAlbums: vi.fn(),
  reuseWork: vi.fn(),
  updateWork: vi.fn(),
  deleteWork: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    listWorks: apiMocks.listWorks,
    listCoupleAlbums: apiMocks.listCoupleAlbums,
    reuseWork: apiMocks.reuseWork,
    updateWork: apiMocks.updateWork,
    deleteWork: apiMocks.deleteWork
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: routeQuery
  }),
  useRouter: () => ({
    push: routerPush
  })
}))

import WorksView from '../views/WorksView.vue'

const worksPayload = {
  items: [
    {
      work_id: 10,
      prompt: '宣传片主视觉KV',
      aspect_ratio: '16:9',
      category: 'poster_kv',
      visibility: 'private',
      preview_url: '/api/works/10/file',
      download_url: '/api/works/10/download',
      created_at: '2026-04-30T01:04:11Z'
    },
    {
      work_id: 11,
      prompt:
        '生成一张用于电商详情页首屏展示的超长中文提示词图片，需要包含春季促销场景、透明玻璃质感包装、柔和晨光、浅色背景、精致产品阴影、中文标题排版、节日氛围和多层次装饰元素'.repeat(8),
      aspect_ratio: '1:1',
      category: 'image',
      visibility: 'private',
      preview_url: '/api/works/11/file',
      download_url: '/api/works/11/download',
      created_at: '2026-04-29T19:03:13Z'
    }
  ],
  page: 1,
  page_size: 30,
  total: 128,
  summary: {
    total: 128,
    week_new: 16,
    stored_percent: 100,
    private_count: 128,
    category_counts: {
      image: 80,
      poster_kv: 24,
      product_main: 16,
      cover: 8
    }
  }
}

function makeWork(id) {
  return {
    work_id: id,
    prompt: `分页作品 ${id}`,
    aspect_ratio: '1:1',
    category: 'image',
    visibility: 'private',
    preview_url: `/api/works/${id}/file`,
    download_url: `/api/works/${id}/download`,
    created_at: `2026-04-${String(Math.max(1, 30 - (id % 20))).padStart(2, '0')}T01:04:11Z`
  }
}

function imageRect({ left, top, width, height }) {
  return {
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => {}
  }
}

describe('WorksView', () => {
  let intersectionCallback

  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    apiMocks.listCoupleAlbums.mockResolvedValue({ albums: [] })
    routerPush.mockReset()
    for (const key of Object.keys(routeQuery)) {
      delete routeQuery[key]
    }
    window.open = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    intersectionCallback = undefined
    window.IntersectionObserver = vi.fn((callback) => {
      intersectionCallback = callback
      return {
        observe: vi.fn(),
        disconnect: vi.fn()
      }
    })
  })

  it('renders the screenshot-style private library with real work data', async () => {
    apiMocks.listWorks.mockResolvedValueOnce(worksPayload)

    const wrapper = mount(WorksView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('LIBRARY')
    expect(wrapper.text()).toContain('你的私有作品库')
    expect(wrapper.text()).toContain('总作品')
    expect(wrapper.text()).toContain('本周新增')
    expect(wrapper.text()).toContain('已入库')
    expect(wrapper.text()).toContain('默认私有')
    expect(wrapper.find('[data-testid="works-filter-bar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="works-card-10"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="works-card-10"] img').attributes('src')).toBe('/api/works/10/file')
    expect(wrapper.text()).toContain('宣传片主视觉KV')
    expect(wrapper.text()).toContain('海报KV')
    expect(wrapper.text()).toContain('16:9')
    expect(wrapper.get('[data-testid="works-download-10"]').attributes('href')).toBe('/api/works/10/download')
    expect(wrapper.find('[data-testid="works-card-11"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('生成一张用于电商详情页首屏展示的超长中文提示词图片')
  })

  it('truncates long prompts on cards and preview media while preserving the full title', async () => {
    apiMocks.listWorks.mockResolvedValueOnce(worksPayload)
    const longPrompt = worksPayload.items[1].prompt

    const wrapper = mount(WorksView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    const card = wrapper.get('[data-testid="works-card-11"]')
    const heading = card.get('h2')
    const image = card.get('img')

    expect(heading.text().length).toBeLessThan(longPrompt.length)
    expect(heading.text()).toMatch(/...$/)
    expect(heading.attributes('title')).toBe(longPrompt)
    expect(image.attributes('alt')).toBe(heading.text())
    expect(image.attributes('title')).toBe(longPrompt)

    await wrapper.get('[data-testid="works-view-11"]').trigger('click')
    await wrapper.vm.$nextTick()

    const modalTitle = wrapper.get('[data-testid="works-preview-title-text"]')
    const modalImage = wrapper.get('[data-testid="works-preview-zoom-image"]')
    expect(modalTitle.text().length).toBeLessThan(longPrompt.length)
    expect(modalTitle.text()).toMatch(/...$/)
    expect(modalTitle.attributes('title')).toBe(longPrompt)
    expect(modalImage.attributes('alt')).toBe(modalTitle.text())
    expect(modalImage.attributes('title')).toBe(longPrompt)
  })

  it('loads works with search, category, time range and sort parameters', async () => {
    apiMocks.listWorks.mockResolvedValue(worksPayload)

    const wrapper = mount(WorksView)
    await flushPromises()

    await wrapper.get('[data-testid="works-search-input"]').setValue('小猫')
    await wrapper.get('[data-testid="works-category-poster_kv"]').trigger('click')
    await wrapper.get('[data-testid="works-time-range"]').setValue('week')
    await wrapper.get('[data-testid="works-sort"]').setValue('oldest')
    await wrapper.get('[data-testid="works-search-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.listWorks).toHaveBeenLastCalledWith({
      q: '小猫',
      category: 'poster_kv',
      time_range: 'week',
      sort: 'oldest',
      exclude_album_pages: true,
      page: 1,
      page_size: 30
    })
  })

  it('loads album cards with works and opens the album detail from the all library', async () => {
    apiMocks.listWorks.mockResolvedValueOnce({
      ...worksPayload,
      items: [
        worksPayload.items[0],
        {
          work_id: 101,
          prompt: '相册第一页普通作品',
          aspect_ratio: '3:4',
          category: 'image',
          visibility: 'private',
          preview_url: '/api/works/101/file',
          download_url: '/api/works/101/download',
          created_at: '2026-05-01T01:00:00Z'
        }
      ],
      total: 1,
      summary: {
        ...worksPayload.summary,
        total: 1,
        category_counts: {
          image: 0,
          poster_kv: 1,
          product_main: 0,
          cover: 0
        }
      }
    })
    apiMocks.listCoupleAlbums.mockResolvedValueOnce({
      albums: [
        {
          id: 88,
          title: '大理旅行相册',
          location: '大理',
          status: 'generating',
          cover_page_id: 202,
          created_at: '2026-05-02T01:00:00Z',
          updated_at: '2026-05-02T02:00:00Z',
          pages: [
            {
              id: 201,
              page_title: '序章',
              caption: '洱海边的风',
              status: 'succeeded',
              work_id: 101,
              preview_url: '/api/works/101/file'
            },
            {
              id: 202,
              page_title: '封面',
              caption: '大理日落',
              status: 'succeeded',
              work_id: 102,
              preview_url: '/api/works/102/file'
            },
            {
              id: 203,
              page_title: '尾声',
              caption: '仍在生成',
              status: 'generating',
              work_id: 0,
              preview_url: ''
            }
          ]
        }
      ]
    })

    const wrapper = mount(WorksView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listWorks).toHaveBeenCalledWith(expect.objectContaining({
      exclude_album_pages: true,
      page: 1,
      page_size: 30
    }))
    expect(apiMocks.listCoupleAlbums).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="works-card-101"]').exists()).toBe(false)

    const albumCard = wrapper.get('[data-testid="works-album-card-88"]')
    expect(albumCard.get('img').attributes('src')).toBe('/api/works/102/file')
    expect(albumCard.text()).toContain('大理旅行相册')
    expect(albumCard.text()).toContain('相册')
    expect(albumCard.text()).toContain('大理')
    expect(albumCard.text()).toContain('2/3 页 · 生成中')
    expect(wrapper.text()).toContain('共 2 个')

    await wrapper.get('[data-testid="works-album-view-88"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith('/workspace/couple-album/88')
  })

  it('hides album cards outside the all category and favorite filter', async () => {
    apiMocks.listWorks.mockResolvedValue(worksPayload)
    apiMocks.listCoupleAlbums.mockResolvedValue({
      albums: [
        {
          id: 99,
          title: '海边相册',
          location: '厦门',
          status: 'succeeded',
          pages: [
            {
              id: 301,
              page_title: '封面',
              caption: '海风',
              status: 'succeeded',
              work_id: 301,
              preview_url: '/api/works/301/file'
            }
          ]
        }
      ]
    })

    const wrapper = mount(WorksView)
    await flushPromises()
    expect(wrapper.find('[data-testid="works-album-card-99"]').exists()).toBe(true)

    await wrapper.get('[data-testid="works-category-image"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="works-album-card-99"]').exists()).toBe(false)

    await wrapper.get('[data-testid="works-category-all"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="works-favorite-filter"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="works-album-card-99"]').exists()).toBe(false)
  })

  it('uses the route category query when opening the video library', async () => {
    routeQuery.category = 'video'
    apiMocks.listWorks.mockResolvedValueOnce(worksPayload)

    mount(WorksView)
    await flushPromises()

    expect(apiMocks.listWorks).toHaveBeenCalledWith({
      q: '',
      category: 'video',
      time_range: 'all',
      sort: 'recent',
      exclude_album_pages: true,
      page: 1,
      page_size: 30
    })
  })

  it('loads the next page when the scroll sentinel becomes visible', async () => {
    const firstPage = {
      ...worksPayload,
      items: Array.from({ length: 30 }, (_, index) => makeWork(index + 1)),
      page: 1,
      page_size: 30,
      total: 35
    }
    const secondPage = {
      ...worksPayload,
      items: Array.from({ length: 5 }, (_, index) => makeWork(index + 31)),
      page: 2,
      page_size: 30,
      total: 35
    }
    apiMocks.listWorks
      .mockResolvedValueOnce(firstPage)
      .mockResolvedValueOnce(secondPage)

    const wrapper = mount(WorksView)
    await flushPromises()

    expect(apiMocks.listWorks).toHaveBeenNthCalledWith(1, {
      q: '',
      category: 'all',
      time_range: 'all',
      sort: 'recent',
      exclude_album_pages: true,
      page: 1,
      page_size: 30
    })
    expect(wrapper.findAll('[data-testid^="works-card-"]')).toHaveLength(30)

    intersectionCallback?.([{ isIntersecting: true }])
    await flushPromises()

    expect(apiMocks.listWorks).toHaveBeenNthCalledWith(2, {
      q: '',
      category: 'all',
      time_range: 'all',
      sort: 'recent',
      exclude_album_pages: true,
      page: 2,
      page_size: 30
    })
    expect(wrapper.findAll('[data-testid^="works-card-"]')).toHaveLength(35)
    expect(wrapper.text()).toContain('已显示全部作品')
  })

  it('keeps view, reuse, delete and collection actions wired to real behavior', async () => {
    apiMocks.listWorks.mockResolvedValue(worksPayload)
    apiMocks.reuseWork.mockResolvedValueOnce({
      prompt: '宣传片主视觉KV',
      aspect_ratio: '16:9',
      tool_mode: 'generate',
      style_preset: '海报',
      reference_work_id: 10
    })
    apiMocks.updateWork.mockResolvedValueOnce({ ...worksPayload.items[0], is_favorite: true })
    apiMocks.deleteWork.mockResolvedValueOnce({ ok: true })

    const wrapper = mount(WorksView)
    await flushPromises()

    await wrapper.get('[data-testid="works-view-10"]').trigger('click')
    expect(window.open).not.toHaveBeenCalled()
    const modal = wrapper.get('[data-testid="works-preview-modal"]')
    expect(modal.attributes('role')).toBe('dialog')
    expect(modal.attributes('aria-modal')).toBe('true')
    expect(modal.get('[data-testid="works-preview-zoom-image"]').attributes('src')).toBe('/api/works/10/file')
    expect(modal.get('[data-testid="works-preview-download"]').attributes('href')).toBe('/api/works/10/download')

    await modal.get('[data-testid="works-preview-reuse"]').trigger('click')
    await flushPromises()
    expect(apiMocks.reuseWork).toHaveBeenCalledWith(10)
    expect(window.sessionStorage.getItem('image_agent_workspace_prefill:v1')).toBe(JSON.stringify({
      prompt: '宣传片主视觉KV',
      aspect_ratio: '16:9',
      tool_mode: 'generate',
      style_preset: '海报',
      reference_work_id: 10
    }))
    expect(routerPush).toHaveBeenCalledWith('/workspace')

    await modal.get('[data-testid="works-preview-collect"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateWork).toHaveBeenCalledWith(10, { is_favorite: true })
    expect(wrapper.text()).toContain('已收藏。')

    await modal.get('[data-testid="works-preview-delete"]').trigger('click')
    await flushPromises()
    expect(apiMocks.deleteWork).toHaveBeenCalledWith(10)
    expect(wrapper.text()).toContain('作品已删除。')
    expect(wrapper.find('[data-testid="works-preview-modal"]').exists()).toBe(false)
  })

  it('filters favorite works and toggles favorites from cards', async () => {
    apiMocks.listWorks.mockResolvedValue(worksPayload)
    apiMocks.updateWork.mockResolvedValueOnce({ ...worksPayload.items[0], is_favorite: true })

    const wrapper = mount(WorksView)
    await flushPromises()

    await wrapper.get('[data-testid="works-favorite-filter"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listWorks).toHaveBeenLastCalledWith(expect.objectContaining({
      favorite: true,
      page: 1
    }))

    await wrapper.get('[data-testid="works-favorite-10"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateWork).toHaveBeenCalledWith(10, { is_favorite: true })
    expect(wrapper.text()).toContain('已收藏。')
  })

  it('confirms public visibility before sharing private works and opens a capped share route', async () => {
    apiMocks.listWorks.mockResolvedValueOnce({
      ...worksPayload,
      items: Array.from({ length: 18 }, (_, index) => ({
        ...makeWork(index + 1),
        visibility: index === 0 ? 'public' : 'private'
      })),
      total: 18
    })
    apiMocks.updateWork.mockResolvedValue({ visibility: 'public' })

    const wrapper = mount(WorksView)
    await flushPromises()

    await wrapper.get('[data-testid="works-share-selected"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('分享前需要将私有作品转为公开，是否继续？')
    expect(apiMocks.updateWork).toHaveBeenCalledTimes(15)
    expect(apiMocks.updateWork).toHaveBeenNthCalledWith(1, 2, { visibility: 'public' })
    expect(routerPush).toHaveBeenCalledWith('/works/share?ids=1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16')
  })

  it('groups works with the same batch id into one card and previews the batch items', async () => {
    apiMocks.listWorks.mockResolvedValueOnce({
      ...worksPayload,
      items: [
        {
          work_id: 30,
          generation_record_id: 130,
          batch_id: 'batch-web-20260515',
          batch_index: 1,
          batch_total: 2,
          prompt: '同一次生成的电商主图',
          aspect_ratio: '1:1',
          category: 'image',
          visibility: 'private',
          preview_url: '/api/works/30/file',
          download_url: '/api/works/30/download',
          created_at: '2026-05-15T02:01:00Z'
        },
        {
          work_id: 29,
          generation_record_id: 129,
          batch_id: 'batch-web-20260515',
          batch_index: 0,
          batch_total: 2,
          prompt: '同一次生成的电商主图',
          aspect_ratio: '1:1',
          category: 'image',
          visibility: 'private',
          preview_url: '/api/works/29/file',
          download_url: '/api/works/29/download',
          created_at: '2026-05-15T02:00:00Z'
        }
      ],
      total: 2
    })

    const wrapper = mount(WorksView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.findAll('[data-testid^="works-card-"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('2张')

    await wrapper.get('[data-testid="works-view-29"]').trigger('click')
    await wrapper.vm.$nextTick()

    const modal = wrapper.get('[data-testid="works-preview-modal"]')
    expect(modal.text()).toContain('1 / 2')
    expect(modal.get('[data-testid="works-preview-zoom-image"]').attributes('src')).toBe('/api/works/29/file')
    expect(modal.get('[data-testid="works-preview-download"]').attributes('href')).toBe('/api/works/29/download')

    await modal.get('[data-testid="works-preview-next"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="works-preview-zoom-image"]').attributes('src')).toBe('/api/works/30/file')
    expect(wrapper.get('[data-testid="works-preview-download"]').attributes('href')).toBe('/api/works/30/download')

    await wrapper.get('[data-testid="works-preview-prev"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="works-preview-zoom-image"]').attributes('src')).toBe('/api/works/29/file')
  })

  it('zooms image previews around the mouse position with translate and scale', async () => {
    apiMocks.listWorks.mockResolvedValue(worksPayload)

    const wrapper = mount(WorksView)
    await flushPromises()

    await wrapper.get('[data-testid="works-view-10"]').trigger('click')
    const surface = wrapper.get('[data-testid="works-preview-zoom-surface"]')
    const image = wrapper.get('[data-testid="works-preview-zoom-image"]')
    image.element.getBoundingClientRect = vi.fn(() => imageRect({
      left: 100,
      top: 50,
      width: 400,
      height: 300
    }))

    surface.element.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    expect(image.element.style.transform).toBe('translate(-40px, -15px) scale(1.2)')
    expect(image.element.style.transformOrigin).toBe('0px 0px')
  })

  it('keeps the pointed works preview pixel stable across consecutive wheel zooms', async () => {
    apiMocks.listWorks.mockResolvedValue(worksPayload)

    const wrapper = mount(WorksView)
    await flushPromises()

    await wrapper.get('[data-testid="works-view-10"]').trigger('click')
    const surface = wrapper.get('[data-testid="works-preview-zoom-surface"]')
    const image = wrapper.get('[data-testid="works-preview-zoom-image"]')
    image.element.getBoundingClientRect = vi.fn()
      .mockReturnValueOnce(imageRect({ left: 100, top: 50, width: 400, height: 300 }))
      .mockReturnValueOnce(imageRect({ left: 60, top: 35, width: 480, height: 360 }))

    surface.element.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    surface.element.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    expect(image.element.style.transform).toBe('translate(-80px, -30px) scale(1.4)')
  })

  it('resets works preview translation at 1x and closes with Escape or the close button', async () => {
    apiMocks.listWorks.mockResolvedValue(worksPayload)

    const wrapper = mount(WorksView)
    await flushPromises()

    await wrapper.get('[data-testid="works-view-10"]').trigger('click')
    const surface = wrapper.get('[data-testid="works-preview-zoom-surface"]')
    const image = wrapper.get('[data-testid="works-preview-zoom-image"]')
    image.element.getBoundingClientRect = vi.fn()
      .mockReturnValueOnce(imageRect({ left: 100, top: 50, width: 400, height: 300 }))
      .mockReturnValueOnce(imageRect({ left: 60, top: 35, width: 480, height: 360 }))

    surface.element.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    surface.element.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: 100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    expect(image.element.style.transform).toBe('translate(0px, 0px) scale(1)')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="works-preview-modal"]').exists()).toBe(false)

    await wrapper.get('[data-testid="works-view-10"]').trigger('click')
    await wrapper.get('[data-testid="works-preview-close"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="works-preview-modal"]').exists()).toBe(false)
  })

  it('shows video works in the preview modal without image zoom controls', async () => {
    apiMocks.listWorks.mockResolvedValue({
      ...worksPayload,
      items: [
        {
          work_id: 20,
          prompt: '产品动画短片',
          aspect_ratio: '16:9',
          category: 'video',
          mime_type: 'video/mp4',
          visibility: 'private',
          preview_url: '/api/works/20/file',
          download_url: '/api/works/20/download',
          created_at: '2026-04-30T01:04:11Z'
        }
      ]
    })

    const wrapper = mount(WorksView)
    await flushPromises()

    await wrapper.get('[data-testid="works-view-20"]').trigger('click')

    const video = wrapper.get('[data-testid="works-preview-video"]')
    expect(video.attributes('src')).toBe('/api/works/20/file')
    expect(video.attributes()).toHaveProperty('controls')
    expect(wrapper.find('[data-testid="works-preview-zoom-image"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="works-preview-zoom-surface"]').exists()).toBe(false)
  })

  it('shows an empty state without static screenshot examples', async () => {
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [],
      total: 0,
      summary: {
        total: 0,
        week_new: 0,
        stored_percent: 0,
        private_count: 0,
        category_counts: {}
      }
    })

    const wrapper = mount(WorksView)
    await flushPromises()

    expect(wrapper.text()).toContain('还没有作品')
    expect(wrapper.find('[data-testid^="works-card-"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('宣传片主视觉KV')
    expect(wrapper.text()).not.toContain('春日促销海报KV')
  })

  it('keeps works library cards contained when prompts are very long', () => {
    const css = readFileSync('src/styles.css', 'utf8')

    expect(css).toMatch(/\.works-library-card\s*{[^}]*overflow:\s*hidden;[^}]*min-width:\s*0;/s)
    expect(css).toMatch(/\.works-library-card\s*{[^}]*align-content:\s*start;/s)
    expect(css).toMatch(
      /\.works-card-frame,\s*\.works-card-body,\s*\.works-card-actions\s*{[^}]*min-width:\s*0;[^}]*width:\s*100%;[^}]*max-width:\s*100%;[^}]*}/s
    )
    expect(css).toMatch(/\.works-card-body\s*>\s*div\s*{[^}]*min-width:\s*0;[^}]*}/s)
  })
})
