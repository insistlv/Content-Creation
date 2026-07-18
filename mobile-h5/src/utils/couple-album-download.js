import { api, getStoredAuthToken } from '../api/client.js'

export const albumPosterCanvasID = 'album-poster-canvas'
export const albumPosterCanvasWidth = 1080

const albumPosterPadding = 64
const downloadTimeoutMS = 30000
const posterFooterHeight = 190

function showToast(title) {
  uni.showToast({ title, icon: 'none' })
}

function delay(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms)
  })
}

function normalizeAlbumImageSource(value) {
  const source = `${value || ''}`.trim()
  if (!source) return ''
  if (/^(https?:|wxfile:|cloud:|blob:|data:image\/|file:)/i.test(source)) return source
  if (/^\/(api|static|tmp|usr|store_|wxfile)/i.test(source)) return api.assetURL(source)
  if (source.startsWith('//')) return `https:${source}`
  return api.assetURL(source)
}

export function collectAlbumDownloadPages(pages = [], normalizeImageSource = normalizeAlbumImageSource) {
  const allPages = Array.isArray(pages) ? pages : []
  const items = allPages
    .filter((page) => page?.status === 'succeeded')
    .map((page, index) => {
      const rawURL = page?.download_url || page?.preview_url
      const url = normalizeImageSource(rawURL) || normalizeAlbumImageSource(rawURL)
      return {
        page,
        index,
        url,
        title: `${page?.page_title || `第 ${page?.page_number || index + 1} 页`}`.trim(),
        caption: `${page?.caption || ''}`.trim(),
        pageNumber: page?.page_number || index + 1
      }
    })
    .filter((item) => item.url)
  return {
    items,
    skippedCount: Math.max(0, allPages.length - items.length)
  }
}

function buildDownloadHeaders() {
  const headers = {}
  // #ifdef MP-WEIXIN
  headers['X-Image-Agent-Client'] = 'mp-weixin'
  // #endif
  const token = getStoredAuthToken()
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  return headers
}

function isLocalImagePath(url) {
  return /^(wxfile:|file:|blob:|data:image\/|cloud:)/i.test(`${url || ''}`)
}

function isPermissionError(error) {
  const errMsg = `${error?.errMsg || error?.message || ''}`
  return /auth deny|authorize|permission|scope\.writePhotosAlbum|saveImageToPhotosAlbum/i.test(errMsg)
}

function promptPhotoPermission(error, context = {}) {
  if (!isPermissionError(error) || context.permissionPrompted) return false
  context.permissionPrompted = true
  if (typeof uni.openSetting === 'function') {
    uni.showModal({
      title: '需要相册权限',
      content: '请允许保存到相册，开启后再重新下载。',
      confirmText: '去设置',
      success(result) {
        if (result.confirm) {
          uni.openSetting()
        }
      }
    })
    return true
  }
  showToast('请在系统设置中允许保存到相册')
  return true
}

function fallbackOpenImage(url) {
  // #ifdef H5
  if (typeof window !== 'undefined') {
    window.open(url, '_blank', 'noopener')
    return true
  }
  // #endif
  return false
}

export function openAlbumImageLinksOnH5(items = []) {
  // #ifdef H5
  const links = items.map((item) => item.url).filter(Boolean)
  links.forEach((url, index) => {
    setTimeout(() => {
      fallbackOpenImage(url)
    }, index * 80)
  })
  showToast(`已打开 ${links.length} 张图片链接`)
  return true
  // #endif
  return false
}

function downloadFile(url) {
  if (isLocalImagePath(url)) return Promise.resolve(url)
  return new Promise((resolve, reject) => {
    let finished = false
    let downloadTask = null
    const timer = setTimeout(() => {
      if (finished) return
      finished = true
      if (downloadTask && typeof downloadTask.abort === 'function') {
        downloadTask.abort()
      }
      reject(new Error('download_timeout'))
    }, downloadTimeoutMS)

    function finish(callback) {
      if (finished) return
      finished = true
      clearTimeout(timer)
      callback()
    }

    downloadTask = uni.downloadFile({
      url,
      header: buildDownloadHeaders(),
      timeout: downloadTimeoutMS,
      success(response) {
        finish(() => {
          const status = Number(response.statusCode || 0)
          if (status >= 200 && status < 300 && response.tempFilePath) {
            resolve(response.tempFilePath)
            return
          }
          reject(new Error(`download_failed_${status || 'unknown'}`))
        })
      },
      fail(error) {
        finish(() => reject(error))
      }
    })
  })
}

function getImageInfo(src) {
  return new Promise((resolve, reject) => {
    uni.getImageInfo({
      src,
      success(info) {
        resolve({
          path: info.path || src,
          width: Number(info.width) || 1,
          height: Number(info.height) || 1
        })
      },
      fail(error) {
        reject(error)
      }
    })
  })
}

async function getDrawableImage(item) {
  const filePath = await downloadFile(item.url)
  const imageInfo = await getImageInfo(filePath)
  return {
    ...item,
    filePath,
    imageInfo
  }
}

function saveImageFile(filePath, platform = {}, context = {}) {
  return new Promise((resolve, reject) => {
    const saver = platform.saveImageToPhotosAlbum || uni.saveImageToPhotosAlbum
    if (typeof saver !== 'function') {
      if (fallbackOpenImage(filePath)) {
        resolve({ fallback: true })
        return
      }
      reject(new Error('saveImageToPhotosAlbum_unavailable'))
      return
    }
    saver.call(uni, {
      filePath,
      success() {
        resolve({})
      },
      fail(error) {
        promptPhotoPermission(error, context)
        reject(error)
      }
    })
  })
}

export async function saveAlbumImagesIndividually(items = [], options = {}) {
  if (items.length === 0) {
    showToast('暂无可下载图片')
    return
  }

  const context = {}
  let successCount = 0
  let failureCount = 0

  for (let index = 0; index < items.length; index += 1) {
    const item = items[index]
    uni.showLoading({ title: `正在保存 ${index + 1}/${items.length}`, mask: true })
    try {
      const filePath = await downloadFile(item.url)
      await saveImageFile(filePath, options.platform, context)
      successCount += 1
    } catch {
      failureCount += 1
    }
  }

  uni.hideLoading()
  const skippedText = options.skippedCount > 0 ? `，跳过 ${options.skippedCount} 页` : ''
  const failedText = failureCount > 0 ? `，失败 ${failureCount} 张` : ''
  showToast(`已保存 ${successCount} 张${failedText}${skippedText}`)
}

function getPosterHeight(pageCount) {
  return 990 + pageCount * 600 + posterFooterHeight
}

export function albumPosterCanvasStyle(height) {
  return `width: ${albumPosterCanvasWidth}px; height: ${height}px;`
}

function drawRoundedRect(ctx, x, y, width, height, radius) {
  if (!ctx.beginPath || !ctx.arcTo) {
    ctx.fillRect(x, y, width, height)
    return
  }
  ctx.beginPath()
  ctx.moveTo(x + radius, y)
  ctx.lineTo(x + width - radius, y)
  ctx.arcTo(x + width, y, x + width, y + radius, radius)
  ctx.lineTo(x + width, y + height - radius)
  ctx.arcTo(x + width, y + height, x + width - radius, y + height, radius)
  ctx.lineTo(x + radius, y + height)
  ctx.arcTo(x, y + height, x, y + height - radius, radius)
  ctx.lineTo(x, y + radius)
  ctx.arcTo(x, y, x + radius, y, radius)
  ctx.closePath()
  ctx.fill()
}

function setText(ctx, color, fontSize, align = 'left') {
  ctx.setFillStyle(color)
  ctx.setFontSize(fontSize)
  if (typeof ctx.setTextAlign === 'function') {
    ctx.setTextAlign(align)
  }
}

function textUnits(char) {
  return /[\u4e00-\u9fa5]/.test(char) ? 1 : 0.58
}

function wrapText(text, maxUnits, maxLines) {
  const source = `${text || ''}`.trim()
  if (!source) return []
  const lines = []
  let line = ''
  let units = 0
  for (const char of source) {
    const charUnits = textUnits(char)
    if (units + charUnits > maxUnits && line) {
      lines.push(line)
      line = char
      units = charUnits
      if (lines.length >= maxLines) break
      continue
    }
    line += char
    units += charUnits
  }
  if (line && lines.length < maxLines) lines.push(line)
  if (lines.length === maxLines && lines[lines.length - 1].length < source.length) {
    lines[lines.length - 1] = `${lines[lines.length - 1].replace(/[。,.，、\s]+$/, '')}...`
  }
  return lines
}

function fillWrappedText(ctx, text, x, y, options) {
  setText(ctx, options.color, options.fontSize)
  const lines = wrapText(text, options.maxUnits, options.maxLines)
  lines.forEach((line, index) => {
    ctx.fillText(line, x, y + index * options.lineHeight)
  })
}

function drawAspectFill(ctx, imageInfo, x, y, width, height) {
  const sourceRatio = imageInfo.width / imageInfo.height
  const targetRatio = width / height
  let sx = 0
  let sy = 0
  let sw = imageInfo.width
  let sh = imageInfo.height
  if (sourceRatio > targetRatio) {
    sw = imageInfo.height * targetRatio
    sx = (imageInfo.width - sw) / 2
  } else {
    sh = imageInfo.width / targetRatio
    sy = (imageInfo.height - sh) / 2
  }
  ctx.drawImage(imageInfo.path, sx, sy, sw, sh, x, y, width, height)
}

function drawStats(ctx, stats) {
  const startX = albumPosterPadding
  const y = 822
  const gap = 24
  const width = (albumPosterCanvasWidth - albumPosterPadding * 2 - gap * 2) / 3
  stats.forEach((stat, index) => {
    const x = startX + index * (width + gap)
    ctx.setFillStyle('#ffffff')
    drawRoundedRect(ctx, x, y, width, 116, 24)
    setText(ctx, '#8f1238', 34, 'center')
    ctx.fillText(stat.value, x + width / 2, y + 46)
    setText(ctx, '#667085', 24, 'center')
    ctx.fillText(stat.label, x + width / 2, y + 84)
  })
}

function drawPoster(ctx, payload) {
  const { album, drawablePages, posterHeight, statusText } = payload
  const isChildhoodDreamAlbum = album?.story_template === 'childhood_career_dream'
  const albumBrandName = isChildhoodDreamAlbum ? '白霖共享童年梦想相册' : '白霖共享情侣相册'
  const albumFallbackTitle = isChildhoodDreamAlbum ? '童年梦想相册' : '情侣相册'
  const albumMeta = isChildhoodDreamAlbum ? '六一职业梦想' : album?.location || '旅行相册'
  const albumPosterCopy = isChildhoodDreamAlbum ? '把童年梦想存成一张长图' : '把我们的故事存成一张长图'
  ctx.setFillStyle('#fff8fb')
  ctx.fillRect(0, 0, albumPosterCanvasWidth, posterHeight)
  ctx.setFillStyle('#eef6ff')
  ctx.fillRect(0, Math.floor(posterHeight * 0.52), albumPosterCanvasWidth, Math.ceil(posterHeight * 0.48))

  setText(ctx, '#9f1239', 28)
  ctx.fillText(albumBrandName, albumPosterPadding, 78)
  setText(ctx, '#121b33', 52)
  fillWrappedText(ctx, album?.title || albumFallbackTitle, albumPosterPadding, 146, {
    color: '#121b33',
    fontSize: 52,
    lineHeight: 62,
    maxUnits: 17,
    maxLines: 2
  })
  setText(ctx, '#667085', 26)
  ctx.fillText(`${albumMeta} · ${statusText(album?.status)}`, albumPosterPadding, 292)

  const cover = drawablePages[0]
  ctx.setFillStyle('#f7e9ef')
  drawRoundedRect(ctx, albumPosterPadding, 334, 952, 444, 34)
  if (cover?.imageInfo) {
    drawAspectFill(ctx, cover.imageInfo, albumPosterPadding, 334, 952, 444)
    ctx.setFillStyle('rgba(18, 27, 51, 0.34)')
    ctx.fillRect(albumPosterPadding, 604, 952, 174)
  }
  setText(ctx, '#ffffff', 42)
  ctx.fillText(album?.title || albumFallbackTitle, albumPosterPadding + 34, 704)
  setText(ctx, 'rgba(255,255,255,0.88)', 25)
  ctx.fillText(albumPosterCopy, albumPosterPadding + 34, 746)

  drawStats(ctx, [
    { value: `${drawablePages.length}/8`, label: '成功页面' },
    { value: statusText(album?.status), label: '生成状态' },
    { value: '1080px', label: '长图宽度' }
  ])

  drawablePages.forEach((item, index) => {
    const y = 990 + index * 600
    const imageX = albumPosterPadding
    const imageY = y + 72
    ctx.setFillStyle(index % 2 === 0 ? '#ffffff' : '#fff3f7')
    drawRoundedRect(ctx, albumPosterPadding, y, 952, 536, 30)
    setText(ctx, '#be123c', 27)
    ctx.fillText(`第 ${item.pageNumber} 页`, albumPosterPadding + 34, y + 52)
    ctx.setFillStyle('#f7e9ef')
    drawRoundedRect(ctx, imageX + 34, imageY, 380, 420, 22)
    drawAspectFill(ctx, item.imageInfo, imageX + 34, imageY, 380, 420)
    fillWrappedText(ctx, item.title, imageX + 454, imageY + 24, {
      color: '#121b33',
      fontSize: 36,
      lineHeight: 44,
      maxUnits: 14,
      maxLines: 2
    })
    fillWrappedText(ctx, item.caption || '这一页还没有文案。', imageX + 454, imageY + 132, {
      color: '#536075',
      fontSize: 26,
      lineHeight: 38,
      maxUnits: 19,
      maxLines: 6
    })
    setText(ctx, '#9f1239', 23)
    ctx.fillText(`${index + 1}/${drawablePages.length}`, imageX + 454, imageY + 392)
  })

  const footerY = 990 + drawablePages.length * 600 + 44
  setText(ctx, '#9f1239', 28, 'center')
  ctx.fillText('保存这张长图，继续分享我们的旅行故事', albumPosterCanvasWidth / 2, footerY)
  setText(ctx, '#667085', 22, 'center')
  ctx.fillText('Powered by 白霖共享 AI 图片生成', albumPosterCanvasWidth / 2, footerY + 42)
}

function drawCanvas(ctx) {
  return new Promise((resolve) => {
    ctx.draw(false, () => resolve())
  })
}

function canvasToTempFilePath(canvasID, height, platform = {}) {
  return new Promise((resolve, reject) => {
    const exporter = platform.canvasToTempFilePath || uni.canvasToTempFilePath
    if (typeof exporter !== 'function') {
      reject(new Error('canvasToTempFilePath_unavailable'))
      return
    }
    exporter.call(
      uni,
      {
        canvasId: canvasID,
        fileType: 'jpg',
        quality: 0.92,
        width: albumPosterCanvasWidth,
        height,
        destWidth: albumPosterCanvasWidth,
        destHeight: height,
        success(response) {
          resolve(response.tempFilePath)
        },
        fail(error) {
          reject(error)
        }
      },
      platform.canvasOwner
    )
  })
}

export async function saveAlbumPoster(items = [], options = {}) {
  if (items.length === 0) {
    showToast('暂无可下载图片')
    return
  }

  const canvasID = options.canvasID || albumPosterCanvasID
  const context = {}
  const drawablePages = []

  uni.showLoading({ title: '正在生成长图', mask: true })
  for (const item of items) {
    try {
      drawablePages.push(await getDrawableImage(item))
    } catch {
      // Keep generating with the pages that can be downloaded and decoded.
    }
  }

  if (drawablePages.length === 0) {
    uni.hideLoading()
    showToast('长图生成失败')
    return
  }

  const posterHeight = getPosterHeight(drawablePages.length)
  if (typeof options.setCanvasHeight === 'function') {
    options.setCanvasHeight(posterHeight)
  }
  await delay(80)

  try {
    const ctx = uni.createCanvasContext(canvasID, options.platform?.canvasOwner)
    drawPoster(ctx, {
      album: options.album,
      drawablePages,
      posterHeight,
      statusText: options.statusText || ((status) => status || '草稿')
    })
    await drawCanvas(ctx)
    await delay(120)
    const filePath = await canvasToTempFilePath(canvasID, posterHeight, options.platform)
    await saveImageFile(filePath, options.platform, context)
    uni.hideLoading()
    const skippedCount = Math.max(0, (options.totalPageCount || items.length) - drawablePages.length)
    showToast(skippedCount > 0 ? `长图已保存，跳过 ${skippedCount} 页` : '长图已保存')
  } catch (error) {
    uni.hideLoading()
    if (!promptPhotoPermission(error, context)) {
      showToast('长图生成失败')
    }
  }
}
