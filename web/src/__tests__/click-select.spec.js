import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { nextTick } from 'vue'

import ClickSelect from '../components/ClickSelect.vue'

const options = [
  { value: 'all', label: '全部' },
  { value: 'active', label: '已启用' },
  { value: 'archived', label: '已归档' }
]

const wrappers = []
const stylesSource = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8')

function mountSelect(props = {}, attachTo = document.body) {
  const wrapper = mount(ClickSelect, {
    attachTo,
    props: {
      modelValue: 'all',
      options,
      ariaLabel: '状态',
      dataTestid: 'status-select',
      'onUpdate:modelValue': vi.fn(),
      ...props
    }
  })
  wrappers.push(wrapper)
  return wrapper
}

function menu() {
  return document.body.querySelector('[data-testid="status-select-menu"]')
}

function option(value) {
  return document.body.querySelector(`[data-testid="status-select-option-${value}"]`)
}

function applyClickSelectTheme(element, theme) {
  const variables = theme === 'dark'
    ? {
        '--click-select-menu-bg': 'rgb(13, 18, 27)',
        '--click-select-menu-text': 'rgb(244, 247, 251)',
        '--click-select-option-active-bg': 'rgba(34, 211, 238, 0.14)',
        '--click-select-option-selected-bg': 'rgba(34, 211, 238, 0.18)',
        '--click-select-focus-ring': 'rgba(34, 211, 238, 0.12)'
      }
    : {
        '--click-select-menu-bg': 'rgb(255, 255, 255)',
        '--click-select-menu-text': 'rgb(17, 24, 39)',
        '--click-select-option-active-bg': 'rgba(14, 165, 233, 0.12)',
        '--click-select-option-selected-bg': 'rgba(14, 165, 233, 0.16)',
        '--click-select-focus-ring': 'rgba(14, 165, 233, 0.12)'
      }

  Object.entries(variables).forEach(([name, value]) => {
    element.style.setProperty(name, value)
  })
}

function flushMutationObserver() {
  return new Promise((resolve) => {
    setTimeout(resolve, 0)
  })
}

function selectorBlock(selector) {
  const match = stylesSource.match(new RegExp(`^${selector.replaceAll('.', '\\.')}\\s*\\{([\\s\\S]*?)\\n\\}`, 'm'))
  return match?.[1] || ''
}

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  document.body.innerHTML = ''
})

describe('ClickSelect', () => {
  it('opens only on click, not hover', async () => {
    const wrapper = mountSelect()

    await wrapper.get('[data-testid="status-select"]').trigger('mouseenter')
    expect(menu()).toBeNull()

    await wrapper.get('[data-testid="status-select"]').trigger('click')
    expect(menu()).not.toBeNull()
    expect(menu()?.getAttribute('role')).toBe('listbox')
    expect(option('active')?.textContent).toContain('已启用')
  })

  it('emits the selected value and closes after selection', async () => {
    const onUpdate = vi.fn()
    mountSelect({ 'onUpdate:modelValue': onUpdate })

    document.body.querySelector('[data-testid="status-select"]').click()
    await nextTick()
    option('active').click()
    await nextTick()

    expect(onUpdate).toHaveBeenCalledWith('active')
    expect(menu()).toBeNull()
  })

  it('can show a fixed trigger label while keeping concrete menu options selectable', async () => {
    const onUpdate = vi.fn()
    const wrapper = mountSelect({
      triggerLabel: '状态',
      'onUpdate:modelValue': onUpdate
    })

    expect(wrapper.get('[data-testid="status-select"]').text()).toContain('状态')
    expect(wrapper.get('[data-testid="status-select"]').text()).not.toContain('全部')

    await wrapper.get('[data-testid="status-select"]').trigger('click')
    expect(option('active')?.textContent).toContain('已启用')
    option('active').click()
    await nextTick()

    expect(onUpdate).toHaveBeenCalledWith('active')
    expect(menu()).toBeNull()
  })

  it('closes on outside click and Escape', async () => {
    const wrapper = mountSelect()

    await wrapper.get('[data-testid="status-select"]').trigger('click')
    expect(menu()).not.toBeNull()
    document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    await nextTick()
    expect(menu()).toBeNull()

    await wrapper.get('[data-testid="status-select"]').trigger('click')
    expect(menu()).not.toBeNull()
    await wrapper.get('[data-testid="status-select"]').trigger('keydown', { key: 'Escape' })
    expect(menu()).toBeNull()
  })

  it('supports keyboard navigation and Enter or Space selection', async () => {
    const onUpdate = vi.fn()
    const wrapper = mountSelect({ 'onUpdate:modelValue': onUpdate })
    const trigger = wrapper.get('[data-testid="status-select"]')

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    expect(menu()).not.toBeNull()
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await trigger.trigger('keydown', { key: 'Enter' })
    expect(onUpdate).toHaveBeenLastCalledWith('active')
    expect(menu()).toBeNull()

    await trigger.trigger('keydown', { key: 'End' })
    await trigger.trigger('keydown', { key: ' ' })
    expect(onUpdate).toHaveBeenLastCalledWith('archived')
    expect(menu()).toBeNull()
  })

  it('does not open or select when disabled', async () => {
    const onUpdate = vi.fn()
    const wrapper = mountSelect({
      disabled: true,
      'onUpdate:modelValue': onUpdate
    })

    await wrapper.get('[data-testid="status-select"]').trigger('click')
    await wrapper.get('[data-testid="status-select"]').trigger('keydown', { key: 'ArrowDown' })

    expect(menu()).toBeNull()
    expect(onUpdate).not.toHaveBeenCalled()
  })

  it('copies click-select theme tokens from the trigger context to the teleported menu', async () => {
    const shell = document.createElement('section')
    shell.className = 'user-dark-shell'
    applyClickSelectTheme(shell, 'dark')
    document.body.appendChild(shell)
    const wrapper = mountSelect({}, shell)

    await wrapper.get('[data-testid="status-select"]').trigger('click')

    expect(menu()?.style.getPropertyValue('--click-select-menu-bg')).toBe('rgb(13, 18, 27)')
    expect(menu()?.style.getPropertyValue('--click-select-menu-text')).toBe('rgb(244, 247, 251)')
    expect(menu()?.style.getPropertyValue('--click-select-option-active-bg')).toBe('rgba(34, 211, 238, 0.14)')
    expect(menu()?.style.getPropertyValue('--click-select-option-selected-bg')).toBe('rgba(34, 211, 238, 0.18)')
    expect(menu()?.style.getPropertyValue('--click-select-focus-ring')).toBe('rgba(34, 211, 238, 0.12)')
  })

  it('refreshes teleported menu tokens when the open theme shell changes class or data-theme', async () => {
    const shell = document.createElement('section')
    shell.className = 'user-dark-shell'
    shell.dataset.theme = 'dark'
    applyClickSelectTheme(shell, 'dark')
    document.body.appendChild(shell)
    const wrapper = mountSelect({}, shell)

    await wrapper.get('[data-testid="status-select"]').trigger('click')
    expect(menu()?.style.getPropertyValue('--click-select-menu-bg')).toBe('rgb(13, 18, 27)')

    shell.className = 'user-light-shell'
    shell.dataset.theme = 'light'
    applyClickSelectTheme(shell, 'light')
    await flushMutationObserver()
    await nextTick()

    expect(menu()?.style.getPropertyValue('--click-select-menu-bg')).toBe('rgb(255, 255, 255)')
    expect(menu()?.style.getPropertyValue('--click-select-menu-text')).toBe('rgb(17, 24, 39)')
    expect(menu()?.style.getPropertyValue('--click-select-option-selected-bg')).toBe('rgba(14, 165, 233, 0.16)')
  })

  it('defines click-select tokens for trigger, menu, item states, focus, and user themes', () => {
    const darkBlock = selectorBlock('.user-dark-shell')
    const lightBlock = selectorBlock('.user-light-shell')

    expect(stylesSource).toMatch(/\.click-select-trigger[\s\S]*background:\s*var\(--click-select-trigger-bg/)
    expect(stylesSource).toMatch(/\.click-select-trigger[\s\S]*border:\s*1px solid var\(--click-select-trigger-border/)
    expect(stylesSource).toMatch(/\.click-select-menu[\s\S]*background:\s*var\(--click-select-menu-bg/)
    expect(stylesSource).toMatch(/\.click-select-menu[\s\S]*color:\s*var\(--click-select-menu-text/)
    expect(stylesSource).toMatch(/\.click-select-option-active[\s\S]*background:\s*var\(--click-select-option-active-bg/)
    expect(stylesSource).toMatch(/\.click-select-option-selected[\s\S]*background:\s*var\(--click-select-option-selected-bg/)
    expect(darkBlock).toContain('--click-select-menu-bg:')
    expect(darkBlock).toContain('--click-select-menu-text:')
    expect(darkBlock).toContain('--click-select-menu-border:')
    expect(darkBlock).toContain('--click-select-option-selected-bg:')
    expect(darkBlock).toContain('--click-select-focus-ring:')
    expect(lightBlock).toContain('--click-select-menu-bg:')
    expect(lightBlock).toContain('--click-select-menu-text:')
    expect(lightBlock).toContain('--click-select-menu-border:')
    expect(lightBlock).toContain('--click-select-option-selected-bg:')
    expect(lightBlock).toContain('--click-select-focus-ring:')
  })
})
