import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import ImageUploadZone from '../components/ImageUploadZone.vue'

function file(name, type = 'image/png') {
  return new File(['fake'], name, { type })
}

function setInputFiles(input, files) {
  Object.defineProperty(input.element, 'files', {
    value: files,
    configurable: true
  })
}

describe('ImageUploadZone', () => {
  it('shows uploading status inside the upload zone with live status semantics', () => {
    const wrapper = mount(ImageUploadZone, {
      props: {
        uploading: true,
        libraryActionLabel: '从素材库选择',
        libraryActionTestid: 'select-library'
      }
    })

    const zone = wrapper.get('.image-upload-zone')
    const status = wrapper.get('[data-testid="image-upload-status"]')

    expect(zone.classes()).toContain('uploading')
    expect(zone.attributes('aria-busy')).toBe('true')
    expect(zone.attributes('aria-disabled')).toBe('true')
    expect(status.text()).toContain('上传中...')
    expect(status.attributes('role')).toBe('status')
    expect(status.attributes('aria-live')).toBe('polite')
    expect(zone.element.contains(status.element)).toBe(true)
  })

  it('disables file input and library action while uploading', () => {
    const wrapper = mount(ImageUploadZone, {
      props: {
        uploading: true,
        libraryActionLabel: '从素材库选择',
        libraryActionTestid: 'select-library'
      }
    })

    expect(wrapper.get('input[type="file"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="select-library"]').attributes('disabled')).toBeDefined()
  })

  it('blocks upload entry points while uploading', async () => {
    const inputClick = vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mount(ImageUploadZone, {
      props: {
        uploading: true,
        libraryActionLabel: '从素材库选择',
        libraryActionTestid: 'select-library'
      }
    })

    await wrapper.get('.image-upload-zone').trigger('click')
    expect(inputClick).not.toHaveBeenCalled()

    await wrapper.get('.image-upload-zone').trigger('dragover')
    expect(wrapper.get('.image-upload-zone').classes()).not.toContain('dragging')

    await wrapper.get('.image-upload-zone').trigger('drop', {
      dataTransfer: { files: [file('drop.png')] }
    })

    const input = wrapper.get('input[type="file"]')
    setInputFiles(input, [file('select.png')])
    await input.trigger('change')

    expect(wrapper.emitted('upload')).toBeUndefined()
    inputClick.mockRestore()
  })

  it('emits valid JPG and PNG files when not uploading', async () => {
    const wrapper = mount(ImageUploadZone)
    const jpg = file('photo.jpg', 'image/jpeg')
    const png = file('source.png', 'image/png')
    const gif = file('motion.gif', 'image/gif')

    const input = wrapper.get('input[type="file"]')
    setInputFiles(input, [jpg, png, gif])
    await input.trigger('change')

    expect(wrapper.emitted('upload')).toEqual([[jpg], [png]])
  })

  it('accepts WEBP, supports keyboard activation and reports explicit validation errors', async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mount(ImageUploadZone)
    await wrapper.get('.image-upload-zone').trigger('keydown', { key: 'Enter' })
    expect(click).toHaveBeenCalled()
    const input = wrapper.get('input[type="file"]')
    const webp = file('product.webp', 'image/webp')
    setInputFiles(input, [webp]); await input.trigger('change')
    expect(wrapper.emitted('upload')).toEqual([[webp]])
    setInputFiles(input, [file('bad.gif', 'image/gif')]); await input.trigger('change')
    expect(wrapper.get('[data-testid="image-upload-error"]').text()).toContain('JPG、PNG 或 WEBP')
    expect(wrapper.get('.image-upload-zone').attributes('tabindex')).toBe('0')
    click.mockRestore()
  })

  it('supports Space and clears format or size errors after a valid selection', async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mount(ImageUploadZone)
    await wrapper.get('.image-upload-zone').trigger('keydown', { key: ' ' })
    expect(click).toHaveBeenCalled()
    const input = wrapper.get('input[type="file"]')
    setInputFiles(input, [new File([new Uint8Array(20 * 1024 * 1024 + 1)], 'huge.webp', { type: 'image/webp' })])
    await input.trigger('change')
    expect(wrapper.get('[data-testid="image-upload-error"]').text()).toContain('20MB')
    setInputFiles(input, [file('ok.webp', 'image/webp')]); await input.trigger('change')
    expect(wrapper.find('[data-testid="image-upload-error"]').exists()).toBe(false)
    click.mockRestore()
  })
})
