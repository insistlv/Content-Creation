import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(__dirname, '../views/VideoConversationWorkspaceView.vue'), 'utf8')
const css = readFileSync(resolve(__dirname, '../video-conversation-workspace.css'), 'utf8')

describe('VideoConversationWorkspaceView contract', () => {
  it('keeps the conversation rail, timeline and unified composer in the selected visual order', () => {
    expect(source.indexOf('video-conversation-rail')).toBeLessThan(source.indexOf('video-chat-timeline'))
    expect(source.indexOf('video-chat-timeline')).toBeLessThan(source.indexOf('video-unified-composer'))
    expect(source).not.toContain('video-rail-brand')
    expect(source).toContain('创意对话')
    expect(source).toContain('视频生成')
  })

  it('uses real conversation, assistant and generation APIs', () => {
    expect(source).toContain('api.listVideoConversations')
    expect(source).toContain('api.createVideoConversationMessage')
    expect(source).toContain('api.createVideoGeneration')
    expect(source).toContain('api.getVideoGeneration')
  })

  it('uses the shared workspace sidebar and provides a mobile single-column layout', () => {
    expect(css).not.toContain('workspace-video-compact')
    expect(css).toContain('grid-template-columns:minmax(250px,292px) minmax(0,1fr)')
    expect(css).toContain('@media(max-width:768px)')
  })

  it('renders duration options and defaults from the selected model capability', () => {
    expect(source).toContain('selectedModel.value?.default_duration')
    expect(source).toContain('selectedModel.value?.durations')
    expect(source).toContain('v-for="value in durationOptions"')
    expect(source).toContain('历史任务时长已不再受当前模型支持')
    expect(source).not.toContain('<option value="5">5 秒</option>')
  })

  it('defines semantic light and dark palettes for every major conversation surface', () => {
    expect(source).toContain(':data-theme="theme"')
    expect(css).toContain('.video-chat-workspace[data-theme="light"]')
    expect(css).toContain('.video-asset-modal[data-theme="light"]')
    for (const token of ['--video-bg', '--video-surface', '--video-input-bg', '--video-text', '--video-line', '--video-overlay']) {
      expect(css).toContain(token)
    }
  })

  it('keeps the composer in flow and defines explicit intermediate tablet breakpoints', () => {
    expect(css).toMatch(/\.video-unified-composer \{\s*position: relative;/)
    expect(css).toContain('grid-template-rows: auto minmax(0, 1fr) auto')
    expect(css).toContain('@media (max-width: 1199px)')
    expect(css).toContain('@media (max-width: 1024px)')
    expect(css).toContain('.video-chat-toolbar > .video-mobile-conversations')
  })

  it('keeps large conversation lists inside their own scroll container', () => {
    expect(css).toMatch(/\.video-chat-workspace \{\s*grid-template-rows: minmax\(0, 1fr\);/)
    expect(css).toMatch(/\.video-conversation-rail \{[\s\S]*?height: 100%;[\s\S]*?min-height: 0;[\s\S]*?overflow: hidden;/)
    expect(css).toMatch(/\.video-conversation-list \{[\s\S]*?flex: 1 1 auto;[\s\S]*?min-height: 0;[\s\S]*?overflow-y: auto;/)
    expect(css).toMatch(/\.video-chat-main \{[\s\S]*?height: 100%;[\s\S]*?min-height: 0;[\s\S]*?grid-template-rows: auto minmax\(0, 1fr\) auto;/)
  })

  it('uses a dedicated two-row composer layout without the legacy global footer class', () => {
    expect(source).not.toContain('class="video-composer-footer"')
    expect(source).toContain('class="video-chat-composer-primary"')
    expect(source).toContain('class="video-chat-composer-actions"')
    expect(source).toContain('class="video-chat-composer-params"')
    expect(source.match(/class="video-chat-composer-field"/g)).toHaveLength(4)
    for (const label of ['模型', '画面比例', '分辨率', '时长']) expect(source).toContain(`<span>${label}</span>`)
    expect(css).toMatch(/\.video-chat-composer-params \{[\s\S]*?grid-template-columns:/)
    expect(css).toMatch(/@media \(max-width: 768px\) \{[\s\S]*?\.video-chat-composer-params \{[\s\S]*?repeat\(2, minmax\(0, 1fr\)\)/)
    expect(css).toMatch(/\.video-mode-switch button,[\s\S]*?\.video-cost,[\s\S]*?\.video-send \{[\s\S]*?flex-shrink: 0;[\s\S]*?white-space: nowrap;/)
  })
})
