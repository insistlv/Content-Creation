import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import AspectRatioSelector from '../components/AspectRatioSelector.vue'
import { chooseClickSelect, clickSelectMenu, openClickSelect } from './click-select-test-utils.js'

const componentSource = readFileSync(
  resolve(process.cwd(), 'src/components/AspectRatioSelector.vue'),
  'utf8'
)
const workspaceStylesSource = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8')

describe('AspectRatioSelector', () => {
  it('uses a click dropdown and emits the selected ratio', async () => {
    const wrapper = mount(AspectRatioSelector, {
      props: {
        modelValue: '1:1',
        'onUpdate:modelValue': vi.fn()
      }
    })

    expect(wrapper.find('[data-testid="workspace-size-select"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-size-select"]').classes()).toContain('aspect-ratio-select')
    expect(wrapper.findAll('.aspect-ratio-button')).toHaveLength(0)
    expect(wrapper.text()).toContain('1:1 方图')
    expect(wrapper.text()).toContain('推荐输出尺寸 1024x1024')

    await chooseClickSelect(wrapper, 'workspace-size-select', '9:16')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['9:16'])
  })

  it('keeps ratio options in the expected click dropdown order', async () => {
    const wrapper = mount(AspectRatioSelector, {
      props: {
        modelValue: '1:1'
      }
    })

    await openClickSelect(wrapper, 'workspace-size-select')
    const options = Array.from(clickSelectMenu('workspace-size-select').querySelectorAll('.click-select-option'))

    expect(options.map((option) => option.dataset.testid.replace('workspace-size-select-option-', ''))).toEqual([
      '21:9',
      '16:9',
      '4:3',
      '3:2',
      '1:1',
      '2:3',
      '3:4',
      '9:16',
      '9:21'
    ])
    expect(options.at(0)?.textContent).toBe('21:9 超宽屏 · 横幅 / 影院感 · 推荐输出尺寸 1536x1024')
    expect(options.at(4)?.textContent).toBe('1:1 方图 · 头像 / 商品 · 推荐输出尺寸 1024x1024')
    expect(options.at(7)?.textContent).toBe('9:16 手机竖屏 · 短视频 / 壁纸 · 推荐输出尺寸 1024x1536')
  })

  it('defines dark click-select states for the compact workspace dropdown', () => {
    expect(componentSource).toContain('ClickSelect')
    expect(workspaceStylesSource).toContain('.click-select-menu')
    expect(workspaceStylesSource).toContain('.click-select-option-selected')
    expect(componentSource).toContain('#111827')
    expect(componentSource).toContain('color: #f9fafb')
    expect(workspaceStylesSource).toMatch(
      /\.imini-bottom-controls \.aspect-ratio-select[\s\S]*background:[^;]*#111827/
    )
    expect(workspaceStylesSource).toMatch(/\.imini-bottom-controls \.aspect-ratio-select[\s\S]*color: #f9fafb/)
  })
})
