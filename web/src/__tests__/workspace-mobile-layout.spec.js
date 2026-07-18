import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const stylesPath = resolve(process.cwd(), 'src/styles.css')
const readStyles = () => readFileSync(stylesPath, 'utf8').replace(/\r\n/g, '\n')

const declarationsForSelector = (styles, selector) =>
  [...styles.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
    .filter(([, selectors]) =>
      selectors
        .split(',')
        .map((entry) => entry.trim())
        .includes(selector)
    )
    .map(([, , declarations]) => declarations)

const expectSelectorDeclaration = (styles, selector, declarationPattern) => {
  const declarations = declarationsForSelector(styles, selector).join('\n')

  expect(declarations).toMatch(declarationPattern)
}

describe('workspace mobile layout styles', () => {
  it('does not cap the text-to-image composer to a nested half-screen scroller on mobile', () => {
    const styles = readStyles()

    expect(styles).not.toContain('.workspace-composer-area {\n    max-height: 50vh;')
    expect(styles).toContain('.workspace-composer-area {\n    position: static;\n    max-height: none;\n    overflow: visible;')
  })

  it('does not keep image-generator-only sidebar shell styles', () => {
    const styles = readStyles()

    expect(styles).not.toContain('workspace-image-generator-shell')
    expectSelectorDeclaration(styles, '.workspace-with-sidebar.user-dark-shell', /grid-template-columns:\s*260px\s+minmax\(0,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.workspace-with-sidebar.user-dark-shell .workspace-content', /padding:\s*16px;/)
  })

  it('uses a compact 376px desktop composer and preserves the tablet single-column breakpoint', () => {
    const styles = readStyles()

    expect(styles).toMatch(
      /\.imini-workspace-grid\s*{[^}]*grid-template-columns:\s*376px\s+minmax\(0,\s*1fr\);[^}]*}/s
    )
    expect(styles).toMatch(/\.imini-workspace-grid\s*{[^}]*gap:\s*8px;[^}]*}/s)
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*1024px\)\s*{[\s\S]*?\.imini-workspace-grid\s*{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\);[^}]*}/
    )
  })

  it('stacks the workshop home upload, prompt, controls and create action vertically', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.workshop-home', /overflow-y:\s*auto;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-composer-area', /width:\s*min\(1080px,\s*100%\);/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-composer-area', /margin:\s*0\s+auto;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-composer-card', /grid-template-columns:\s*minmax\(0,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-composer-card', /grid-template-rows:\s*auto;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-reference-block', /grid-column:\s*1;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-reference-block', /grid-row:\s*1;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-reference-block', /min-height:\s*128px;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-reference-block .workspace-reference-attachments', /min-height:\s*128px;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-prompt-card', /grid-column:\s*1;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-prompt-card', /grid-row:\s*2;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-prompt-input', /height:\s*104px;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-prompt-input', /min-height:\s*104px;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-prompt-footer', /margin-top:\s*6px;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-bottom-controls', /grid-column:\s*1;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-bottom-controls', /grid-row:\s*3;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-bottom-controls', /grid-template-columns:\s*minmax\(138px,\s*1fr\)\s+minmax\(118px,\s*0\.86fr\)\s+minmax\(104px,\s*0\.7fr\)\s+minmax\(82px,\s*0\.52fr\);/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-create-button', /grid-column:\s*1;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-create-button', /grid-row:\s*auto;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-create-button', /width:\s*100%;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-composer-card', /border-radius:\s*28px;/)
  })

  it('keeps the workshop home credit cost inside the bottom create button', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.workshop-home .imini-create-button', /position:\s*relative;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-create-button', /overflow:\s*visible;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-create-cost', /position:\s*static;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-create-cost', /transform:\s*none;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-create-cost', /white-space:\s*nowrap;/)
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-home\s+\.imini-create-cost\s*{[^}]*position:\s*static;[^}]*}/
    )
  })

  it('keeps home composer dropdown triggers short and centered', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.workshop-home .imini-home-control-bar .click-select-trigger', /justify-content:\s*center;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-home-control-bar .click-select-trigger', /padding:\s*0\s+12px;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-home-control-bar .click-select-value', /text-align:\s*center;/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-home-control-bar .click-select-value', /font-weight:\s*700;/)
    expectSelectorDeclaration(styles, '.workshop-home .workspace-home-advanced-toggle', /justify-content:\s*center;/)
    expectSelectorDeclaration(styles, '.workshop-home .workspace-home-advanced-toggle', /gap:\s*7px;/)
  })

  it('themes workshop home surfaces with scoped tokens and a dark shell override', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.workshop-home', /--workshop-home-bg:/)
    expectSelectorDeclaration(styles, '.workshop-home', /--workshop-panel-bg:/)
    expectSelectorDeclaration(styles, '.workshop-home', /background:\s*var\(--workshop-home-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-home', /color:\s*var\(--workshop-text\);/)
    expectSelectorDeclaration(styles, '.workspace-with-sidebar.user-dark-shell .workshop-home', /--workshop-home-bg:[\s\S]*#020617/)
    expectSelectorDeclaration(styles, '.workspace-with-sidebar.user-dark-shell .workshop-home', /--workshop-panel-bg:\s*rgba\(15,\s*23,\s*42,\s*0\.88\);/)
    expectSelectorDeclaration(styles, '.workspace-with-sidebar.user-dark-shell .workshop-home', /--workshop-text:\s*#e5eefb;/)
  })

  it('uses workshop home tokens for the dark-sensitive homepage controls and cards', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.workshop-mode-tabs', /background:\s*var\(--workshop-floating-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-mode-tabs button', /color:\s*var\(--workshop-muted-text\);/)
    expectSelectorDeclaration(styles, '.workshop-mode-tabs button.active', /background:\s*var\(--workshop-active-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-composer-card', /background:\s*var\(--workshop-panel-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-prompt-input', /color:\s*var\(--workshop-text\);/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-reference-block .workspace-reference-attachments', /background:\s*var\(--workshop-input-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-home-control-bar .click-select-trigger', /background:\s*var\(--workshop-control-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-quick-prompts button', /background:\s*var\(--workshop-floating-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-feature-item', /color:\s*var\(--workshop-strong-text\);/)
    expectSelectorDeclaration(styles, '.workshop-tool-card', /background:\s*var\(--workshop-card-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-playground-card', /background:\s*var\(--workshop-card-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-workflow-card', /background:\s*var\(--workshop-card-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-home .imini-discovery-card', /background:\s*var\(--workshop-discovery-bg\);/)
  })

  it('keeps workshop feature entries in one compact desktop row while preserving mobile wrapping', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.workshop-feature-grid', /grid-template-columns:\s*repeat\(7,\s*minmax\(0,\s*1fr\)\);/)
    expectSelectorDeclaration(styles, '.workshop-feature-grid', /gap:\s*10px\s+12px;/)
    expectSelectorDeclaration(styles, '.workshop-tool-grid', /grid-template-columns:\s*repeat\(4,\s*minmax\(0,\s*1fr\)\);/)
    expectSelectorDeclaration(styles, '.workshop-workflow-grid', /grid-template-columns:\s*repeat\(3,\s*minmax\(0,\s*1fr\)\);/)
    expectSelectorDeclaration(styles, '.workshop-feature-item', /min-height:\s*104px;/)
    expectSelectorDeclaration(styles, '.workshop-feature-item', /padding:\s*12px\s+8px;/)
    expectSelectorDeclaration(styles, '.workshop-feature-item', /gap:\s*7px;/)
    expectSelectorDeclaration(styles, '.workshop-feature-item', /background:\s*var\(--workshop-card-bg\);/)
    expectSelectorDeclaration(styles, '.workshop-feature-item', /border:\s*1px\s+solid\s*var\(--workshop-card-border\);/)
    expectSelectorDeclaration(styles, '.workshop-feature-item > span', /width:\s*52px;/)
    expectSelectorDeclaration(styles, '.workshop-feature-item > span', /height:\s*52px;/)
    expectSelectorDeclaration(styles, '.workshop-feature-item > span', /border-radius:\s*16px;/)
    expectSelectorDeclaration(styles, '.workshop-feature-item strong', /max-width:\s*100%;/)
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*1024px\)\s*{[\s\S]*?\.workshop-feature-grid\s*{[^}]*grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\);[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*1024px\)\s*{[\s\S]*?\.workshop-tool-grid,\s*\.workshop-playground-grid,\s*\.workshop-workflow-grid\s*{[^}]*grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\);[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-home\s*{[^}]*overflow-x:\s*hidden;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-home\s+\.imini-composer-card\s*{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\);[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-home\s+\.imini-prompt-input\s*{[^}]*min-height:\s*124px;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-feature-grid\s*{[^}]*grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\);[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-feature-item\s*{[^}]*min-height:\s*116px;[^}]*padding:\s*12px\s+10px;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-feature-item\s*>\s*span\s*{[^}]*width:\s*68px;[^}]*height:\s*68px;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-tool-grid,\s*\.workshop-playground-grid,\s*\.workshop-workflow-grid\s*{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\);[^}]*}/
    )
    expect(styles).not.toMatch(/@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-feature-grid\s*{[^}]*grid-template-columns:\s*repeat\(3,\s*minmax\(0,\s*1fr\)\);[^}]*}/)
  })

  it('lets the workshop recommendation masonry fill the discovery width as a four-column desktop group', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.workshop-recommendation-masonry', /column-count:\s*4;/)
    expectSelectorDeclaration(styles, '.workshop-recommendation-masonry', /width:\s*100%;/)
    expectSelectorDeclaration(styles, '.workshop-recommendation-masonry', /max-width:\s*none;/)
    expectSelectorDeclaration(styles, '.workshop-recommendation-masonry', /margin-inline:\s*auto;/)
    expectSelectorDeclaration(styles, '.workshop-recommendation-media img', /min-height:\s*188px;/)
    expectSelectorDeclaration(styles, '.workshop-recommendation-media img', /object-fit:\s*cover;/)
    expect(styles).not.toMatch(/\.workshop-recommendation-masonry\s*{[^}]*max-width:\s*1120px;/s)
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*1024px\)\s*{[\s\S]*?\.workshop-recommendation-masonry\s*{[^}]*column-count:\s*3;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-recommendation-masonry\s*{[^}]*column-count:\s*2;[^}]*}/
    )
  })

  it('keeps home AI tool cards as split horizontal cards with right-side cover media', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.workshop-tool-head', /display:\s*flex;/)
    expectSelectorDeclaration(styles, '.workshop-section-icon', /border-radius:\s*50%;/)
    expectSelectorDeclaration(styles, '.workshop-tool-card', /display:\s*grid;/)
    expectSelectorDeclaration(styles, '.workshop-tool-card', /grid-template-columns:\s*minmax\(0,\s*0\.92fr\)\s+minmax\(118px,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.workshop-tool-card', /min-height:\s*162px;/)
    expectSelectorDeclaration(styles, '.workshop-tool-card-copy', /position:\s*relative;/)
    expectSelectorDeclaration(styles, '.workshop-tool-card-media', /height:\s*100%;/)
    expectSelectorDeclaration(styles, '.workshop-tool-card-media img', /width:\s*100%;/)
    expectSelectorDeclaration(styles, '.workshop-tool-card-media img', /height:\s*100%;/)
    expectSelectorDeclaration(styles, '.workshop-tool-card-media img', /object-fit:\s*cover;/)
    expectSelectorDeclaration(styles, '.workshop-tool-card-media img', /position:\s*static;/)

    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workshop-tool-card\s*{[^}]*grid-template-columns:\s*minmax\(0,\s*0\.96fr\)\s+minmax\(112px,\s*0\.86fr\);[^}]*}/
    )
    expect(styles).not.toMatch(/\.workshop-tool-card\s+img\s*{[^}]*position:\s*absolute;/s)
  })

  it('keeps the default imini workspace surface dark', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.imini-workspace-grid', /background:\s*#111112;/)
    expectSelectorDeclaration(styles, '.imini-composer-card', /background:\s*#171719;/)
    expectSelectorDeclaration(styles, '.imini-discovery-card', /background:\s*#171719;/)
    expectSelectorDeclaration(styles, '.imini-prompt-input', /color:\s*#f8fafc;/)
    expectSelectorDeclaration(styles, '.imini-tool-card', /background:\s*#2a2a2f;/)
    expectSelectorDeclaration(styles, '.imini-result-frame', /background:\s*#18181b;/)
  })

  it('overrides imini workspace surfaces in the light user shell', () => {
    const styles = readStyles()

    expectSelectorDeclaration(
      styles,
      '.workspace-with-sidebar.user-light-shell .imini-workspace-grid',
      /background:\s*#eef5fb;[\s\S]*color:\s*var\(--ink\);/
    )
    expectSelectorDeclaration(
      styles,
      '.workspace-with-sidebar.user-light-shell .imini-composer-card',
      /border-color:\s*var\(--line\);[\s\S]*background:\s*rgba\(255,\s*255,\s*255,\s*0\.92\);/
    )
    expectSelectorDeclaration(
      styles,
      '.workspace-with-sidebar.user-light-shell .imini-discovery-card',
      /border-color:\s*var\(--line\);[\s\S]*background:\s*rgba\(255,\s*255,\s*255,\s*0\.92\);/
    )
    expectSelectorDeclaration(
      styles,
      '.workspace-with-sidebar.user-light-shell .imini-prompt-input',
      /border-color:\s*rgba\(24,\s*44,\s*76,\s*0\.12\);[\s\S]*background:\s*#ffffff;[\s\S]*color:\s*var\(--ink\);/
    )
    expectSelectorDeclaration(
      styles,
      '.workspace-with-sidebar.user-light-shell .imini-tool-card',
      /border-color:\s*rgba\(24,\s*44,\s*76,\s*0\.12\);[\s\S]*background:\s*#ffffff;/
    )
    expectSelectorDeclaration(
      styles,
      '.workspace-with-sidebar.user-light-shell .imini-playground-card',
      /border-color:\s*rgba\(24,\s*44,\s*76,\s*0\.12\);[\s\S]*background:\s*#ffffff;/
    )
    expectSelectorDeclaration(
      styles,
      '.workspace-with-sidebar.user-light-shell .imini-result-frame',
      /border-color:\s*rgba\(24,\s*44,\s*76,\s*0\.12\);[\s\S]*background:\s*#ffffff;/
    )
    expectSelectorDeclaration(
      styles,
      '.workspace-with-sidebar.user-light-shell .imini-history-section',
      /color:\s*var\(--ink\);/
    )
  })

  it('uses the mobile drawer breakpoint without reserving fixed sidebar width', () => {
    const styles = readStyles()

    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workspace-with-sidebar\s*{[^}]*display:\s*block;[^}]*padding:\s*0;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workspace-sidebar-shell\s*{[^}]*position:\s*fixed;[^}]*transform:\s*translateX\(-104%\);[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workspace-with-sidebar\.workspace-sidebar-open \.workspace-sidebar-shell,\n\s*\.site-user-layout\.workspace-sidebar-open \.workspace-sidebar-shell\s*{[^}]*transform:\s*translateX\(0\)\s*!important;[^}]*opacity:\s*1\s*!important;[^}]*pointer-events:\s*auto;[^}]*transition:\s*none;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workspace-with-sidebar:not\(\.workspace-sidebar-open\) \.workspace-sidebar-shell,\n\s*\.site-user-layout:not\(\.workspace-sidebar-open\) \.workspace-sidebar-shell\s*{[^}]*transform:\s*translateX\(-104%\)\s*!important;[^}]*opacity:\s*0\s*!important;[^}]*pointer-events:\s*none;[^}]*transition:\s*none;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.workspace-content\s*{[^}]*width:\s*100%;[^}]*}/
    )
  })

  it('pins desktop workspace and user sidebars while scrolling only the main content', () => {
    const styles = readStyles()

    expect(styles).toMatch(
      /@media\s*\(min-width:\s*769px\)\s*{[\s\S]*?\.workspace-sidebar-shell\s*{[^}]*position:\s*fixed;[^}]*top:\s*16px;[^}]*bottom:\s*16px;[^}]*left:\s*16px;[^}]*width:\s*260px;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(min-width:\s*769px\)\s*{[\s\S]*?\.workspace-content\s*{[^}]*position:\s*fixed;[^}]*top:\s*16px;[^}]*right:\s*16px;[^}]*bottom:\s*16px;[^}]*left:\s*292px;[^}]*overflow-y:\s*auto;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(min-width:\s*769px\)\s*{[\s\S]*?\.site-user-sidebar-shell\s*{[^}]*position:\s*fixed;[^}]*top:\s*18px;[^}]*bottom:\s*18px;[^}]*left:\s*18px;[^}]*width:\s*260px;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(min-width:\s*769px\)\s*{[\s\S]*?\.site-user-main\s*{[^}]*position:\s*fixed;[^}]*top:\s*18px;[^}]*right:\s*18px;[^}]*bottom:\s*18px;[^}]*left:\s*296px;[^}]*overflow-y:\s*auto;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(min-width:\s*769px\)\s*{[\s\S]*?\.workspace-with-sidebar\.user-dark-shell \.workspace-sidebar-shell,\n\s*\.workspace-with-sidebar\.user-light-shell \.workspace-sidebar-shell,\n\s*\.site-shell-user-sidebar\.user-dark-shell \.site-user-sidebar-shell,\n\s*\.site-shell-user-sidebar\.user-light-shell \.site-user-sidebar-shell\s*{[^}]*top:\s*0;[^}]*bottom:\s*0;[^}]*left:\s*0;[^}]*height:\s*100svh;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(min-width:\s*769px\)\s*{[\s\S]*?\.workspace-with-sidebar\.user-dark-shell \.workspace-content,\n\s*\.workspace-with-sidebar\.user-light-shell \.workspace-content,\n\s*\.site-shell-user-sidebar\.user-dark-shell \.site-user-main,\n\s*\.site-shell-user-sidebar\.user-light-shell \.site-user-main\s*{[^}]*top:\s*0;[^}]*right:\s*0;[^}]*bottom:\s*0;[^}]*left:\s*260px;[^}]*overflow-y:\s*auto;[^}]*}/
    )
  })

  it('constrains the create result stage so generated images do not fill or overflow the right panel', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.imini-result-stage', /width:\s*min\(820px,\s*72vw\);/)
    expectSelectorDeclaration(styles, '.imini-result-stage', /max-height:\s*calc\(100vh - 260px\);/)
    expectSelectorDeclaration(styles, '.imini-result-stage', /grid-template-columns:\s*minmax\(0,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.imini-result-frame', /aspect-ratio:\s*16\s*\/\s*10;/)
    expectSelectorDeclaration(styles, '.imini-result-frame', /min-width:\s*0;/)
    expectSelectorDeclaration(styles, '.imini-result-frame .preview-image', /object-fit:\s*contain;/)
  })

  it('gives image tool textareas room for two-line placeholder text', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.imini-prompt-input', /padding:\s*14px\s+16px;/)
    expectSelectorDeclaration(styles, '.imini-prompt-input', /line-height:\s*1\.55;/)
    expectSelectorDeclaration(styles, '.imini-prompt-input', /min-height:\s*160px;/)
    expectSelectorDeclaration(styles, '.user-dark-shell .imini-composer-card .imini-prompt-input', /border:\s*1px\s+solid\s+rgba\(255,\s*255,\s*255,\s*0\.1\);/)
    expectSelectorDeclaration(styles, '.user-dark-shell .imini-composer-card .imini-prompt-input', /background:\s*#151517;/)
    expectSelectorDeclaration(styles, '.imini-tool-option textarea', /padding:\s*14px\s+12px;/)
    expectSelectorDeclaration(styles, '.imini-tool-option textarea', /line-height:\s*1\.5;/)
    expectSelectorDeclaration(styles, '.imini-tool-option textarea', /resize:\s*vertical;/)
    expectSelectorDeclaration(styles, '.imini-tool-option--wide', /grid-column:\s*1\s*\/\s*-1;/)
  })

  it('keeps selected reference thumbnails legible without overflowing mobile controls', () => {
    const styles = readStyles()

    expect(styles).toMatch(
      /\.workspace-reference-stack\s*{[^}]*width:\s*108px;[^}]*height:\s*78px;[^}]*}/s
    )
    expect(styles).toMatch(
      /\.workspace-reference-stack-thumb\s*{[^}]*width:\s*72px;[^}]*height:\s*72px;[^}]*}/s
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*640px\)\s*{[\s\S]*?\.workspace-reference-stack\s*{[^}]*width:\s*92px;[^}]*height:\s*70px;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*640px\)\s*{[\s\S]*?\.workspace-reference-stack-thumb\s*{[^}]*width:\s*64px;[^}]*height:\s*64px;[^}]*}/
    )
    expect(styles).toMatch(
      /\.workspace-reference-stack-thumb img,\s*\.workspace-reference-grid-item img\s*{[^}]*object-fit:\s*cover;[^}]*}/s
    )
  })

  it('keeps discovery cards split into content and media areas instead of overlaying images on copy', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.imini-tool-grid', /grid-template-columns:\s*repeat\(4,\s*minmax\(0,\s*1fr\)\);/)
    expectSelectorDeclaration(styles, '.imini-tool-card', /display:\s*grid;/)
    expectSelectorDeclaration(styles, '.imini-tool-card', /grid-template-columns:\s*minmax\(0,\s*0\.82fr\)\s+minmax\(148px,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.imini-tool-card', /gap:\s*8px;/)
    expectSelectorDeclaration(styles, '.imini-tool-card', /min-height:\s*196px;/)
    expectSelectorDeclaration(styles, '.imini-tool-card-content', /position:\s*relative;/)
    expectSelectorDeclaration(styles, '.imini-tool-card-media', /height:\s*100%;/)
    expectSelectorDeclaration(styles, '.imini-tool-card-media', /min-height:\s*172px;/)
    expectSelectorDeclaration(styles, '.imini-tool-card-media img', /position:\s*static;/)
    expectSelectorDeclaration(styles, '.imini-tool-card-media img', /transform:\s*none;/)
    expectSelectorDeclaration(styles, '.imini-tool-card-media img', /object-fit:\s*cover;/)

    expectSelectorDeclaration(styles, '.imini-playground-grid', /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\);/)
    expectSelectorDeclaration(styles, '.imini-playground-grid', /gap:\s*14px;/)
    expectSelectorDeclaration(styles, '.imini-playground-card', /display:\s*grid;/)
    expectSelectorDeclaration(styles, '.imini-playground-card', /grid-template-columns:\s*minmax\(0,\s*0\.86fr\)\s+minmax\(280px,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.imini-playground-card', /gap:\s*10px;/)
    expectSelectorDeclaration(styles, '.imini-playground-card', /min-height:\s*206px;/)
    expectSelectorDeclaration(styles, '.imini-playground-card', /border-radius:\s*12px;/)
    expectSelectorDeclaration(styles, '.imini-playground-media', /height:\s*100%;/)
    expectSelectorDeclaration(styles, '.imini-playground-media', /min-height:\s*182px;/)
    expectSelectorDeclaration(styles, '.imini-playground-media img', /position:\s*static;/)
    expectSelectorDeclaration(styles, '.imini-playground-media img', /transform:\s*none;/)

    expectSelectorDeclaration(styles, '.imini-case-masonry', /display:\s*grid;/)
    expectSelectorDeclaration(styles, '.imini-case-masonry', /grid-template-columns:\s*repeat\(3,\s*minmax\(0,\s*1fr\)\);/)
    expectSelectorDeclaration(styles, '.imini-template-card', /display:\s*grid;/)
    expectSelectorDeclaration(styles, '.imini-template-card', /grid-template-columns:\s*minmax\(0,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.imini-case-card-media', /aspect-ratio:\s*16\s*\/\s*10;/)
    expectSelectorDeclaration(styles, '.imini-case-card-content', /position:\s*relative;/)

    expect(styles).not.toMatch(/\.imini-playground-visual\s*{/)
    expect(styles).not.toMatch(/\.imini-template-card\s+>\s*img\s*{/)
    expect(styles).not.toMatch(/\.imini-template-card\s+div\s*{[^}]*position:\s*absolute;/s)
    expect(styles).not.toMatch(/\.imini-tool-card\s+img\s*{[^}]*position:\s*absolute;/s)
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*1024px\)\s*{[\s\S]*?\.imini-tool-grid,\s*\.imini-case-masonry\s*{[^}]*grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\);[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.imini-tool-grid,\s*\.imini-playground-grid,\s*\.imini-case-masonry\s*{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\);[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.imini-tool-card,\s*\.imini-playground-card\s*{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\);[^}]*}/
    )
  })

  it('uses a single-column mobile create surface with a bounded result stage and horizontal history rail', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.imini-create-surface', /grid-template-columns:\s*minmax\(0,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.imini-result-section', /width:\s*100%;[\s\S]*min-width:\s*0;/)
    expectSelectorDeclaration(styles, '.imini-history-section', /max-width:\s*100%;[\s\S]*min-width:\s*0;/)
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.imini-create-surface\s*{[^}]*gap:\s*18px;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.imini-result-stage\s*{[^}]*width:\s*100%;[^}]*max-height:\s*none;[^}]*}/
    )
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.imini-history-section\s+\.history-grid\s*{[^}]*grid-auto-flow:\s*column;[^}]*overflow-x:\s*auto;[^}]*}/
    )
  })

  it('keeps the prompt assistant as one chat column with the result bubble inside the message rail', () => {
    const styles = readStyles()

    expectSelectorDeclaration(styles, '.prompt-assistant-layout', /grid-template-columns:\s*minmax\(0,\s*1fr\);/)
    expectSelectorDeclaration(styles, '.prompt-assistant-layout', /overflow-x:\s*hidden;/)
    expectSelectorDeclaration(styles, '.prompt-assistant-chat', /border-right:\s*0;/)
    expectSelectorDeclaration(styles, '.prompt-assistant-messages', /min-width:\s*0;/)
    expectSelectorDeclaration(styles, '.prompt-assistant-result-bubble', /max-width:\s*min\(88%,\s*820px\);/)
    expectSelectorDeclaration(styles, '.prompt-assistant-result-bubble', /overflow-x:\s*hidden;/)
    expect(styles).not.toMatch(/\.prompt-assistant-result-collapse\s*{/)
    expect(styles).toMatch(
      /@media\s*\(max-width:\s*768px\)\s*{[\s\S]*?\.prompt-assistant-result-bubble\s*{[^}]*max-width:\s*100%;[^}]*}/
    )
    expect(styles).not.toMatch(/\.prompt-assistant-result-collapse\s*>\s*summary/)
  })
})
