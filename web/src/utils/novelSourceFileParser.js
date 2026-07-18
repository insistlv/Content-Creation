const SUPPORTED_TEXT_MIME_TYPES = new Set(['text/plain', 'text/markdown', 'text/x-markdown'])

export async function parseNovelSourceFile(file) {
  if (!file) throw new Error('请选择要导入的文档')

  const format = detectNovelSourceFormat(file)
  if (!format) {
    throw new Error('暂不支持该文件格式，请上传 MD、TXT、DOCX 或文本型 PDF')
  }

  if (format === 'doc') {
    throw new Error('暂不支持 .doc，请另存为 .docx 后再导入')
  }

  let text = ''
  if (format === 'markdown' || format === 'txt') {
    text = stripByteOrderMark(await readFileAsText(file))
  } else if (format === 'docx') {
    text = await extractDocxText(file)
  } else if (format === 'pdf') {
    text = await extractPdfText(file)
  }

  if (!text.trim()) {
    throw new Error('文件内容为空，请选择包含小说正文的文档')
  }

  return { text, format }
}

function detectNovelSourceFormat(file) {
  const extension = file.name?.toLowerCase().match(/\.([^.]+)$/)?.[1] ?? ''
  const mimeType = file.type?.toLowerCase() ?? ''

  if (extension === 'doc') return 'doc'
  if (extension === 'md' || extension === 'markdown' || mimeType === 'text/markdown' || mimeType === 'text/x-markdown') return 'markdown'
  if (extension === 'txt' || SUPPORTED_TEXT_MIME_TYPES.has(mimeType)) return 'txt'
  if (extension === 'docx' || mimeType === 'application/vnd.openxmlformats-officedocument.wordprocessingml.document') return 'docx'
  if (extension === 'pdf' || mimeType === 'application/pdf') return 'pdf'
  return ''
}

async function extractDocxText(file) {
  try {
    const mammoth = await import('mammoth')
    const api = mammoth.default ?? mammoth
    const result = await api.extractRawText({ arrayBuffer: await readFileAsArrayBuffer(file) })
    return result.value ?? ''
  } catch {
    throw new Error('DOCX 解析失败，请确认文件未损坏或另存为 .docx 后重试')
  }
}

async function extractPdfText(file) {
  try {
    const pdfjs = await import('pdfjs-dist')
    if (pdfjs.GlobalWorkerOptions && !pdfjs.GlobalWorkerOptions.workerSrc) {
      pdfjs.GlobalWorkerOptions.workerSrc = new URL('pdfjs-dist/build/pdf.worker.mjs', import.meta.url).toString()
    }

    const documentTask = pdfjs.getDocument({ data: new Uint8Array(await readFileAsArrayBuffer(file)) })
    const pdf = await documentTask.promise
    const pages = []

    for (let pageNumber = 1; pageNumber <= pdf.numPages; pageNumber += 1) {
      const page = await pdf.getPage(pageNumber)
      const textContent = await page.getTextContent()
      const pageText = textContent.items
        .map((item) => item.str ?? '')
        .filter(Boolean)
        .join(' ')
        .trim()
      if (pageText) pages.push(pageText)
    }

    const text = pages.join('\n\n')
    if (!text.trim()) {
      throw new Error('PDF 未检测到可复制文字，请上传文本型 PDF 或先转为 TXT/DOCX')
    }
    return text
  } catch (error) {
    if (error.message?.includes('PDF 未检测到可复制文字')) throw error
    throw new Error('PDF 解析失败，请确认文件未加密且包含可复制文字')
  }
}

function stripByteOrderMark(text) {
  return text.replace(/^\uFEFF/, '')
}

function readFileAsText(file) {
  if (typeof file.text === 'function') return file.text()
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(`${reader.result ?? ''}`)
    reader.onerror = () => reject(new Error('文件读取失败，请重新选择文档'))
    reader.readAsText(file)
  })
}

function readFileAsArrayBuffer(file) {
  if (typeof file.arrayBuffer === 'function') return file.arrayBuffer()
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result)
    reader.onerror = () => reject(new Error('文件读取失败，请重新选择文档'))
    reader.readAsArrayBuffer(file)
  })
}
