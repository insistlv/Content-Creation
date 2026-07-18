import { describe, expect, it, vi } from 'vitest'

import { parseNovelSourceFile } from '../utils/novelSourceFileParser.js'

const mammothMocks = vi.hoisted(() => ({
  extractRawText: vi.fn()
}))

const pdfjsMocks = vi.hoisted(() => ({
  getDocument: vi.fn()
}))

vi.mock('mammoth', () => ({
  default: mammothMocks,
  extractRawText: mammothMocks.extractRawText
}))

vi.mock('pdfjs-dist', () => ({
  getDocument: pdfjsMocks.getDocument,
  GlobalWorkerOptions: {}
}))

function fileOf(content, name, type = '') {
  return new File([content], name, { type })
}

describe('parseNovelSourceFile', () => {
  it('reads markdown files as plain text and keeps markdown markers', async () => {
    const result = await parseNovelSourceFile(fileOf('# 第一章\n\n**灰塔**醒来。', 'story.md', 'text/markdown'))

    expect(result).toEqual({
      format: 'markdown',
      text: '# 第一章\n\n**灰塔**醒来。'
    })
  })

  it('reads txt files as plain text', async () => {
    const result = await parseNovelSourceFile(fileOf('灰塔里有三种守门兽。', 'story.txt', 'text/plain'))

    expect(result).toEqual({
      format: 'txt',
      text: '灰塔里有三种守门兽。'
    })
  })

  it('extracts raw text from docx files through mammoth', async () => {
    mammothMocks.extractRawText.mockResolvedValueOnce({ value: 'DOCX 第一章\n灰塔醒来。' })

    const result = await parseNovelSourceFile(fileOf('fake-docx', 'story.docx', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'))

    expect(mammothMocks.extractRawText).toHaveBeenCalledWith({ arrayBuffer: expect.any(ArrayBuffer) })
    expect(result).toEqual({
      format: 'docx',
      text: 'DOCX 第一章\n灰塔醒来。'
    })
  })

  it('extracts text layer from text pdf pages', async () => {
    pdfjsMocks.getDocument.mockReturnValueOnce({
      promise: Promise.resolve({
        numPages: 2,
        getPage: vi.fn()
          .mockResolvedValueOnce({
            getTextContent: vi.fn().mockResolvedValueOnce({ items: [{ str: '第一章' }, { str: '灰塔' }] })
          })
          .mockResolvedValueOnce({
            getTextContent: vi.fn().mockResolvedValueOnce({ items: [{ str: '第二章' }, { str: '兽群' }] })
          })
      })
    })

    const result = await parseNovelSourceFile(fileOf('fake-pdf', 'story.pdf', 'application/pdf'))

    expect(pdfjsMocks.getDocument).toHaveBeenCalledWith(expect.objectContaining({ data: expect.any(Uint8Array) }))
    expect(result).toEqual({
      format: 'pdf',
      text: '第一章 灰塔\n\n第二章 兽群'
    })
  })

  it('rejects old doc files with a clear unsupported format error', async () => {
    await expect(parseNovelSourceFile(fileOf('fake-doc', 'story.doc', 'application/msword'))).rejects.toThrow('暂不支持 .doc')
  })

  it('rejects scanned pdf files without text layer', async () => {
    pdfjsMocks.getDocument.mockReturnValueOnce({
      promise: Promise.resolve({
        numPages: 1,
        getPage: vi.fn().mockResolvedValueOnce({
          getTextContent: vi.fn().mockResolvedValueOnce({ items: [] })
        })
      })
    })

    await expect(parseNovelSourceFile(fileOf('fake-pdf', 'scan.pdf', 'application/pdf'))).rejects.toThrow('PDF 未检测到可复制文字')
  })

  it('rejects empty parsed content', async () => {
    await expect(parseNovelSourceFile(fileOf('   ', 'empty.txt', 'text/plain'))).rejects.toThrow('文件内容为空')
  })
})
