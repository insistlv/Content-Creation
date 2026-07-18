import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import * as apiClient from '../api/client.js'

const { api, ensureAdminSession } = apiClient

describe('api client', () => {
  beforeEach(() => {
    document.cookie = 'csrf_token=test-csrf-token; path=/'
    apiClient.clearRecentNetworkErrors?.()
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    Object.defineProperty(window.navigator, 'onLine', {
      configurable: true,
      value: true
    })
    document.cookie = 'csrf_token=; Max-Age=0; path=/'
  })

  it('normalizes rejected fetch errors into actionable network ApiErrors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    await expect(api.getPackages()).rejects.toMatchObject({
      code: 'network_unreachable',
      status: 0,
      retryable: true,
      method: 'GET',
      path: '/api/packages',
      online: true,
      message: '网络连接不稳定，暂时无法连接服务器，请稍后重试'
    })
  })

  it('uses an offline-specific message for rejected fetch errors', async () => {
    Object.defineProperty(window.navigator, 'onLine', {
      configurable: true,
      value: false
    })
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    await expect(api.getPackages()).rejects.toMatchObject({
      code: 'network_unreachable',
      status: 0,
      retryable: true,
      online: false,
      message: '当前网络已断开，请检查网络后重试'
    })
  })

  it('normalizes CSRF token network failures before mutating requests', async () => {
    document.cookie = 'csrf_token=; Max-Age=0; path=/'
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    await expect(api.logout()).rejects.toMatchObject({
      code: 'network_unreachable',
      status: 0,
      retryable: true,
      method: 'GET',
      path: '/api/auth/csrf-token',
      message: '网络连接不稳定，暂时无法连接服务器，请稍后重试'
    })
  })

  it('normalizes OSS upload network failures without leaking browser messages', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({
          upload_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com',
          object_key: 'assets/reference-assets/1/ref.png',
          upload_token: 'signed-token',
          form_data: {
            key: 'assets/reference-assets/1/ref.png',
            policy: 'policy'
          }
        })
      })
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.uploadReferenceAsset(new File(['fake'], 'ref.png', { type: 'image/png' }))).rejects.toMatchObject({
      code: 'network_unreachable',
      status: 0,
      retryable: true,
      method: 'POST',
      path: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com',
      message: '网络连接不稳定，暂时无法连接服务器，请稍后重试'
    })
  })

  it('normalizes export network failures without leaking browser messages', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    await expect(api.exportNovelVideoProject(12)).rejects.toMatchObject({
      code: 'network_unreachable',
      status: 0,
      retryable: true,
      method: 'GET',
      path: '/api/novel-video-projects/12/export',
      message: '网络连接不稳定，暂时无法连接服务器，请稍后重试'
    })
  })

  it('keeps backend business error messages unchanged', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: vi.fn().mockResolvedValue({
        error: {
          code: 'credit_not_enough',
          message: '余额不足，请先充值'
        }
      })
    }))

    await expect(api.getPackages()).rejects.toMatchObject({
      code: 'credit_not_enough',
      status: 409,
      message: '余额不足，请先充值'
    })
  })

  it('keeps backend SMS rate-limit errors and reads Retry-After seconds', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 429,
      headers: new Headers({ 'Retry-After': '37' }),
      json: vi.fn().mockResolvedValue({
        error: {
          code: 'sms_rate_limited',
          message: '短信发送过于频繁，请稍后再试'
        }
      })
    }))

    await expect(api.sendSMSCode({
      phone: '13800138000',
      purpose: 'register'
    })).rejects.toMatchObject({
      code: 'sms_rate_limited',
      status: 429,
      message: '短信发送过于频繁，请稍后再试',
      retry_after_seconds: 37
    })
  })

  it('normalizes plain-text 429 responses into readable rate-limit errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 429,
      headers: new Headers(),
      json: vi.fn().mockRejectedValue(new SyntaxError('Unexpected token T'))
    }))

    await expect(api.sendSMSCode({
      phone: '13800138000',
      purpose: 'register'
    })).rejects.toMatchObject({
      code: 'too_many_requests',
      status: 429,
      message: '请求过于频繁，请稍后再试'
    })
  })

  it('keeps recent network diagnostics without request bodies', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    await expect(api.getPackages()).rejects.toMatchObject({ code: 'network_unreachable' })

    expect(apiClient.getRecentNetworkErrors()).toEqual([
      expect.objectContaining({
        code: 'network_unreachable',
        method: 'GET',
        path: '/api/packages',
        online: true,
        timestamp: expect.any(String)
      })
    ])
    expect(JSON.stringify(apiClient.getRecentNetworkErrors())).not.toContain('body')
  })

  it('explains how to recover from local cross-origin blocks', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 403,
      json: vi.fn().mockResolvedValue({
        error: {
          code: 'cross_origin_blocked',
          message: 'Origin mismatch'
        }
      })
    }))

    await expect(api.login('creator', 'secret')).rejects.toMatchObject({
      code: 'cross_origin_blocked',
      status: 403,
      message: expect.stringContaining('http://localhost:8888')
    })
  })

  it('loads auth captcha and sends captcha fields during web login', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.getCaptcha('user_login')
    await api.login('creator', 'secret', {
      captcha_id: 'cap-user',
      captcha_code: 'A2B3C'
    }, { rememberLogin: true })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/auth/captcha?purpose=user_login', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/auth/login', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({
        username: 'creator',
        password: 'secret',
        captcha_id: 'cap-user',
        captcha_code: 'A2B3C',
        remember_login: true
      })
    }))
  })

  it('sends video soundtrack generate and upload requests', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['music'], 'song.mp3', { type: 'audio/mpeg' })
    await api.generateVideoSoundtrack(42, { variation: 'replace' })
    await api.uploadVideoSoundtrack(42, file)
    await api.listVideoSoundtracks(42)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/videos/42/soundtracks/generate', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'Content-Type': 'application/json',
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({ variation: 'replace' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/videos/42/soundtracks/upload', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.not.objectContaining({
        'Content-Type': 'application/json'
      }),
      body: expect.any(FormData)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/videos/42/soundtracks', expect.objectContaining({
      method: 'GET',
      credentials: 'include'
    }))
  })

  it('sends reset-password requests with CSRF protection', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.resetPassword({
      phone: '13800138000',
      verification_code: '123456',
      new_password: 'NewPass123'
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/auth/reset-password', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({
        phone: '13800138000',
        verification_code: '123456',
        new_password: 'NewPass123'
      })
    }))
  })

  it('posts image Agent planning requests with CSRF protection', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({
        reply: '已生成方案',
        plan: {
          title: '商品主图',
          prompt: '商品主图提示词',
          tool_mode: 'generate',
          aspect_ratio: '1:1'
        },
        candidates: []
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.planImageAgent({
      message: '帮我做商品主图',
      reference_asset_ids: [42]
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/agent/image-plan', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({
        message: '帮我做商品主图',
        reference_asset_ids: [42]
      })
    }))
  })

  it('sends captcha fields during admin login', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ admin: { id: 1 } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.adminLogin('root', 'secret', {
      captcha_id: 'cap-admin',
      captcha_code: 'D4E5F'
    }, { rememberLogin: true })

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/login', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({
        username: 'root',
        password: 'secret',
        captcha_id: 'cap-admin',
        captcha_code: 'D4E5F',
        remember_login: true
      })
    }))
  })

  it('uploads reference assets through backend policy, OSS form post, and completion callback', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({
          upload_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com',
          object_key: 'assets/reference-assets/1/2026/05/ref.png',
          upload_token: 'signed-token',
          form_data: {
            key: 'assets/reference-assets/1/2026/05/ref.png',
            policy: 'policy',
            OSSAccessKeyId: 'access-key',
            Signature: 'signature',
            'Content-Type': 'image/png',
            success_action_status: '201'
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        text: vi.fn().mockResolvedValue('')
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: vi.fn().mockResolvedValue({ id: 1, preview_url: 'https://image/ref.png' })
      })
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['fake'], 'ref.png', { type: 'image/png' })
    const result = await api.uploadReferenceAsset(file)

    expect(result).toEqual({ id: 1, preview_url: 'https://image/ref.png' })
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/reference-assets/upload-policy', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'Content-Type': 'application/json',
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({
        filename: 'ref.png',
        mime_type: 'image/png',
        size: 4
      })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, 'https://example-assets.oss-cn-shenzhen.aliyuncs.com', expect.objectContaining({
      method: 'POST',
      body: expect.any(FormData)
    }))
    const ossForm = fetchMock.mock.calls[1][1].body
    expect(ossForm.get('key')).toBe('assets/reference-assets/1/2026/05/ref.png')
    expect(ossForm.get('Content-Type')).toBe('image/png')
    expect(ossForm.get('file')).toBe(file)
    expect(fetchMock.mock.calls[1][1].credentials).toBeUndefined()
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/reference-assets/complete-upload', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({
        object_key: 'assets/reference-assets/1/2026/05/ref.png',
        upload_token: 'signed-token'
      })
    }))
  })

  it('updates reference asset display names with PATCH and CSRF protection', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({
        id: 42,
        display_name: '商品参考图'
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    const result = await api.updateReferenceAsset(42, { display_name: '商品参考图' })

    expect(result).toEqual({
      id: 42,
      display_name: '商品参考图'
    })
    expect(fetchMock).toHaveBeenCalledWith('/api/reference-assets/42', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      headers: expect.objectContaining({
        'Content-Type': 'application/json',
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({
        display_name: '商品参考图'
      })
    }))
  })

  it('plans moments marketing campaigns with CSRF protection', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({
        moments_text: '今天推荐这家店',
        image_cards: []
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.planMomentsMarketing({
      input_mode: 'text',
      output_type: 'poster_overlay',
      image_count: 3,
      product_name: '巷口咖啡'
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/marketing/moments/plan', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'Content-Type': 'application/json',
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({
        input_mode: 'text',
        output_type: 'poster_overlay',
        image_count: 3,
        product_name: '巷口咖啡'
      })
    }))
  })

  it('plans article image campaigns with CSRF protection', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({
        article_summary: '文章配图方案',
        image_cards: []
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.planArticleImages({
      title: '活动增长方法论',
      body: '第一段介绍活动背景。第二段说明三步流程。',
      image_count: 3,
      include_cover: true
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/marketing/article-images/plan', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'Content-Type': 'application/json',
        'X-CSRF-Token': 'test-csrf-token'
      }),
      body: JSON.stringify({
        title: '活动增长方法论',
        body: '第一段介绍活动背景。第二段说明三步流程。',
        image_count: 3,
        include_cover: true
      })
    }))
  })

  it('loads credit transactions with pagination and kind query params', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.getCreditTransactions({ page: 2, page_size: 10, kind: 'consume' })

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/account/credit-transactions?page=2&page_size=10&kind=consume',
      expect.objectContaining({
        credentials: 'include'
      })
    )
  })

  it('fetches a CSRF token before the first mutating request when the cookie is missing', async () => {
    document.cookie = 'csrf_token=; Max-Age=0; path=/'
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ csrf_token: 'fresh-csrf-token' })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ ok: true })
      })
    vi.stubGlobal('fetch', fetchMock)

    await api.logout()

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/auth/csrf-token', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/auth/logout', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'X-CSRF-Token': 'fresh-csrf-token'
      }),
      body: JSON.stringify({})
    }))
  })

  it('uses configured absolute backend base URLs when present', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'http://182.92.93.11/')
    vi.resetModules()
    const { api: configuredApi } = await import('../api/client.js')
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ packages: [] })
    })
    vi.stubGlobal('fetch', fetchMock)

    await configuredApi.getPackages()

    expect(fetchMock).toHaveBeenCalledWith('http://182.92.93.11/api/packages', expect.objectContaining({
      credentials: 'include'
    }))
    expect(configuredApi.generationExportURL({ status: 'failed' })).toBe('http://182.92.93.11/api/admin/generations/export?status=failed')
  })

  it('passes explicit pagination and filter params for admin generations', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0, page: 2, page_size: 20 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listGenerations({
      q: '海报',
      model: 'gpt-image-2',
      user_keyword: 'creator_alpha',
      status: 'succeeded',
      date_from: '2026-04-25',
      date_to: '2026-05-01',
      page: 2,
      page_size: 20
    })
    await api.getGeneration(42)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/generations?q=%E6%B5%B7%E6%8A%A5&model=gpt-image-2&user_keyword=creator_alpha&status=succeeded&date_from=2026-04-25&date_to=2026-05-01&page=2&page_size=20', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/generations/42', expect.objectContaining({
      credentials: 'include'
    }))
    expect(api.generationExportURL({ status: 'failed', q: '超时', user_keyword: 'creator_alpha' })).toBe('/api/admin/generations/export?status=failed&q=%E8%B6%85%E6%97%B6&user_keyword=creator_alpha')
  })

  it('passes explicit pagination and filter params for admin video generations', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0, page: 2, page_size: 20 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listVideoGenerations({
      q: 'launch',
      source: 'workspace',
      provider: 'Wuyin',
      runtime_model: 'grok-imagine-video-1.5-preview',
      status: 'succeeded',
      date_from: '2026-04-25',
      date_to: '2026-05-01',
      page: 2,
      page_size: 20
    })
    await api.getAdminVideoGeneration(42)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/video-generations?q=launch&source=workspace&provider=Wuyin&runtime_model=grok-imagine-video-1.5-preview&status=succeeded&date_from=2026-04-25&date_to=2026-05-01&page=2&page_size=20', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/video-generations/42', expect.objectContaining({
      credentials: 'include'
    }))
    expect(api.videoGenerationExportURL({ status: 'failed', q: 'timeout', provider: 'Wuyin' })).toBe('/api/admin/video-generations/export?status=failed&q=timeout&provider=Wuyin')
  })

  it('passes user video generation history filters', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0, page: 2, page_size: 10 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listUserVideoGenerations({
      q: 'launch',
      status: 'succeeded',
      model: 'sora-2-pro',
      enhancement: '高清',
      page: 2,
      page_size: 10
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/videos/generations?q=launch&status=succeeded&model=sora-2-pro&enhancement=%E9%AB%98%E6%B8%85&page=2&page_size=10', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('loads public video model capabilities', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [] })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listVideoModels()

    expect(fetchMock).toHaveBeenCalledWith('/api/videos/models', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('sends novel video render preflight requests before queueing', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ renderable: 1, blocked: 0 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.renderNovelVideoPreflight(12)

    expect(fetchMock).toHaveBeenCalledWith('/api/novel-video-projects/12/render-preflight', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({})
    }))
  })

  it('calls the expanded novel video asset render compose and export APIs', async () => {
    const blob = new Blob(['zip'])
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ items: [] })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ status: 'queued', jobs: [] })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ items: [] })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ project: { total_credits: 3 } })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ status: 'succeeded' })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        blob: vi.fn().mockResolvedValue(blob)
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ items: [] })
      })
    vi.stubGlobal('fetch', fetchMock)

    await api.generateNovelVideoAssets(12, { kinds: ['character'] })
    await api.queueNovelVideoRender(12)
    await api.generateNovelVideoGrids(12, { grid_size: 4 })
    await api.getNovelVideoCostEstimate(12)
    await api.composeNovelVideoProject(12)
    await expect(api.exportNovelVideoProjectPackage(12, 'jianying')).resolves.toBe(blob)
    await api.listNovelVideoCompositions(12)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/novel-video-projects/12/assets/generate', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ kinds: ['character'] })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/novel-video-projects/12/render', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/novel-video-projects/12/grids/generate', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ grid_size: 4 })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/novel-video-projects/12/cost-estimate', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/novel-video-projects/12/compose', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(6, '/api/novel-video-projects/12/export?format=jianying', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(7, '/api/novel-video-projects/12/compositions', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('calls the novel video asset dedupe API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ removed: 1, items: [] })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.dedupeNovelVideoAssets(12)

    expect(fetchMock).toHaveBeenCalledWith('/api/novel-video-projects/12/assets/dedupe', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({})
    }))
  })

  it('deletes a novel video asset through the asset API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ deleted_id: 71, items: [], jobs: [] })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.deleteNovelVideoAsset(12, 71)

    expect(fetchMock).toHaveBeenCalledWith('/api/novel-video-projects/12/assets/71', expect.objectContaining({
      method: 'DELETE',
      credentials: 'include',
      body: JSON.stringify({})
    }))
  })

  it('calls the short film image planning actor lock and shot image APIs', async () => {
    const blob = new Blob(['images'])
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ id: 12 })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ id: 21 })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ item: { id: 31 } })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ queued: 4, items: [] })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ items: [] })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ id: 41, selected: true })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        blob: vi.fn().mockResolvedValue(blob)
      })
    vi.stubGlobal('fetch', fetchMock)

    await api.generateNovelVideoImagePlan(12, { shot_count: 20 })
    await api.updateNovelVideoActor(12, 21, { lock_level: 'strict', review_status: 'approved' })
    await api.generateNovelVideoActorLockSheet(12, 21)
    await api.generateNovelVideoShotImages(12, { shot_ids: [41], candidates_per_shot: 4, mode: 'text_to_image' })
    await api.listNovelVideoShotImages(12, { shot_id: 41, review_status: 'needs_review' })
    await api.updateNovelVideoShotImage(12, 51, { selected: true })
    await expect(api.exportNovelVideoProjectPackage(12, 'image_package')).resolves.toBe(blob)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/novel-video-projects/12/image-plan', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ shot_count: 20 })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/novel-video-projects/12/actors/21', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({ lock_level: 'strict', review_status: 'approved' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/novel-video-projects/12/actors/21/generate-lock-sheet', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/novel-video-projects/12/images/generate', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ shot_ids: [41], candidates_per_shot: 4, mode: 'text_to_image' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/novel-video-projects/12/images?shot_id=41&review_status=needs_review', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(6, '/api/novel-video-projects/12/images/51', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({ selected: true })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(7, '/api/novel-video-projects/12/export?format=image_package', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('passes global admin search keywords', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({
        query: 'creator',
        sections: []
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.searchAdmin({ q: 'creator' })).resolves.toEqual({
      query: 'creator',
      sections: []
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/search?q=creator', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('passes channel call attempt filters to the model center diagnostics API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0, page: 2, page_size: 20 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listModelCenterChannelCallAttempts(31, {
      model_id: 11,
      page: 2,
      page_size: 20,
      status: 'failed',
      date_from: '2026-05-01',
      date_to: '2026-05-18'
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/model-center/channels/31/call-attempts?model_id=11&page=2&page_size=20&status=failed&date_from=2026-05-01&date_to=2026-05-18', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('loads a single admin model usage detail', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ model: { id: 3 } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.getAdminModel(3)

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/models/3', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('deletes an admin model through the admin API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.deleteAdminModel(12, { force: true })

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/models/12?force=true', expect.objectContaining({
      method: 'DELETE',
      credentials: 'include',
      body: JSON.stringify({})
    }))
  })

  it('passes finance order filters, detail, export and status updates to admin APIs', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listFinanceOrders({
      type: 'package',
      payment_status: 'paid',
      date_from: '2026-05-01',
      date_to: '2026-05-31',
      q: 'FO-2026',
      page: 2,
      page_size: 20
    })
    await api.getFinanceOrder(42)
    await api.syncFinanceOrderPayment(42)
    await api.updateFinanceRefund(7, { status: 'completed' })
    await api.updateFinanceInvoice(9, { status: 'issued' })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/finance-orders?type=package&payment_status=paid&date_from=2026-05-01&date_to=2026-05-31&q=FO-2026&page=2&page_size=20', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/finance-orders/42', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/finance-orders/42/sync-payment', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/admin/finance-refunds/7', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({ status: 'completed' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/admin/finance-invoices/9', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({ status: 'issued' })
    }))
    expect(api.financeOrdersExportURL({ payment_status: 'paid', q: 'FO-2026' })).toBe('/api/admin/finance-orders/export?payment_status=paid&q=FO-2026')
  })

  it('passes Alipay checkout order, pay and query calls to payment APIs', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ order_number: 'FO-ALIPAY-001' })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.createAlipayOrder({ package_id: 3 })
    await api.getAlipayOrder('FO-ALIPAY-001')
    await api.payAlipayOrder('FO-ALIPAY-001')
    await api.queryAlipayOrder('FO-ALIPAY-001')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/payments/alipay/orders', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ package_id: 3 })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/payments/alipay/orders/FO-ALIPAY-001', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/payments/alipay/orders/FO-ALIPAY-001/pay', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/payments/alipay/orders/FO-ALIPAY-001/query', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({})
    }))
  })

  it('passes system settings read, update and export calls to admin APIs', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    const payload = {
      platform: { name: '白霖共享' },
      generation: { upload_limit: 6 }
    }
    await api.getSystemSettings()
    await api.updateSystemSettings(payload)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/system-settings', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/system-settings', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify(payload)
    }))
    expect(api.systemSettingsExportURL()).toBe('/api/admin/system-settings/export')
  })

  it('passes system log filters to the admin system logs API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 30 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listSystemLogs({
      category: 'user_operation',
      page: 2,
      page_size: 30,
      level: 'error',
      method: 'POST',
      status: 503,
      keyword: 'alipay',
      date_from: '2026-05-13',
      date_to: '2026-05-13'
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/system-logs?category=user_operation&page=2&page_size=30&level=error&method=POST&status=503&keyword=alipay&date_from=2026-05-13&date_to=2026-05-13', expect.objectContaining({
      credentials: 'include'
    }))
    expect(api.systemLogsExportURL({
      category: 'system_operation',
      keyword: 'model.update',
      date_from: '2026-05-13'
    })).toBe('/api/admin/system-logs/export?category=system_operation&keyword=model.update&date_from=2026-05-13')
  })

  it('loads system resources through the admin read API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ sampled_at: '2026-05-21T01:02:03Z' })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.getSystemResources()

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/system-resources', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('refreshes a cached admin session once before denying a newly added permission', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ permissions: ['dashboard.read'] })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ permissions: ['dashboard.read', 'finance_orders.read'] })
      })
    vi.stubGlobal('fetch', fetchMock)

    await api.adminLogin('admin', 'secret')
    const result = await ensureAdminSession('finance_orders.read')

    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/me', expect.objectContaining({
      credentials: 'include'
    }))
    expect(result).toEqual({ authenticated: true, authorized: true })
  })

  it('changes the current admin password and clears cached admin session data', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({
          admin: { id: 1, username: 'admin', display_name: '管理员' },
          permissions: ['dashboard.read']
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ ok: true })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({
          admin: { id: 1, username: 'admin', display_name: '管理员' },
          permissions: []
        })
      })
    vi.stubGlobal('fetch', fetchMock)

    await api.adminLogin('admin', 'OldPass123')
    await api.changeAdminPassword({
      current_password: 'OldPass123',
      new_password: 'NewPass456'
    })
    const result = await ensureAdminSession()

    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/password', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({
        current_password: 'OldPass123',
        new_password: 'NewPass456'
      })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/me', expect.objectContaining({
      credentials: 'include'
    }))
    expect(result).toEqual({ authenticated: true, authorized: true })
  })

  it('passes invite filters, batch generation and redemption filters to admin APIs', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listInvites({
      status: 'partial',
      q: 'OPS',
      page: 2,
      page_size: 10
    })
    await api.batchCreateInvites({
      prefix: 'OPS',
      quantity: 5,
      expires_at: '2026-06-01T23:59:59+08:00',
      total_quota: 3
    })
    await api.listInviteRedemptions({
      start_date: '2026-05-01',
      end_date: '2026-05-31',
      result: 'converted',
      page: 1,
      page_size: 10
    })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/invites?status=partial&q=OPS&page=2&page_size=10', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/invites/batch', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({
        prefix: 'OPS',
        quantity: 5,
        expires_at: '2026-06-01T23:59:59+08:00',
        total_quota: 3
      })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/invite-redemptions?start_date=2026-05-01&end_date=2026-05-31&result=converted&page=1&page_size=10', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('sends announcement management and popup calls to the API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listAnnouncements({
      status: 'published',
      level: 'important',
      client: 'web',
      keyword: '版本',
      page: 2,
      page_size: 12
    })
    await api.createAnnouncement({
      title: '版本更新',
      content: '新模型已上线',
      level: 'important',
      status: 'draft',
      target_clients: ['web'],
      popup_enabled: true,
      priority: 20
    })
    await api.updateAnnouncement(9, {
      title: '版本更新公告',
      content: '新模型已上线',
      level: 'warning',
      target_clients: ['web', 'mp-weixin'],
      popup_enabled: false,
      priority: 10
    })
    await api.updateAnnouncementStatus(9, 'offline')
    await api.listPopupAnnouncements('web')
    await api.dismissAnnouncement(9, 'web')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/announcements?status=published&level=important&client=web&keyword=%E7%89%88%E6%9C%AC&page=2&page_size=12', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/announcements', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({
        title: '版本更新',
        content: '新模型已上线',
        level: 'important',
        status: 'draft',
        target_clients: ['web'],
        popup_enabled: true,
        priority: 20
      })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/announcements/9', expect.objectContaining({
      method: 'PUT',
      credentials: 'include',
      body: JSON.stringify({
        title: '版本更新公告',
        content: '新模型已上线',
        level: 'warning',
        target_clients: ['web', 'mp-weixin'],
        popup_enabled: false,
        priority: 10
      })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/admin/announcements/9/status', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({ status: 'offline' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/announcements/popup?client=web', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(6, '/api/announcements/9/dismiss', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ client: 'web' })
    }))
  })

  it('passes filters for admin users and paginates admin credit transactions', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [], total: 0 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listAdminUsers({
      page: 2,
      page_size: 10,
      q: 'creator',
      role: 'standard_user',
      status: 'online',
      sort_by: 'available_credits',
      sort_dir: 'desc'
    })
    await api.listAdminCreditTransactions({ page: 1, page_size: 8 })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/users?page=2&page_size=10&q=creator&role=standard_user&status=online&sort_by=available_credits&sort_dir=desc', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/credit-transactions?page=1&page_size=8', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('pings account presence through the lightweight heartbeat endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true, online_window_seconds: 300 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.pingPresence()

    expect(fetchMock).toHaveBeenCalledWith('/api/account/presence', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('sends manual credit adjustments to the admin API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ user_id: 7, available_credits: 28 })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.adjustAdminCredits(7, {
      type: 'deduct',
      amount: 8,
      note: '人工扣减'
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/users/7/credit-adjustments', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({
        type: 'deduct',
        amount: 8,
        note: '人工扣减'
      })
    }))
  })

  it('sends admin WeChat binding update and unbind requests', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ user_id: 7, wechat_bound: true, wechat_open_id: 'wx-openid' })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.updateAdminUserWechatBinding(7, {
      openid: 'wx-openid',
      note: '客服核验后修正'
    })
    await api.deleteAdminUserWechatBinding(7, { note: '用户要求解绑' })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/users/7/wechat-binding', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({
        openid: 'wx-openid',
        note: '客服核验后修正'
      })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/users/7/wechat-binding', expect.objectContaining({
      method: 'DELETE',
      credentials: 'include',
      body: JSON.stringify({ note: '用户要求解绑' })
    }))
  })

  it('sends account and admin phone unbind requests with JSON bodies', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ phone: null })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.unbindAccountPhone({ current_password: 'test-password' })
    await api.deleteAdminUserPhoneBinding(7, { note: '后台解绑手机号' })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/account/phone', expect.objectContaining({
      method: 'DELETE',
      credentials: 'include',
      body: JSON.stringify({ current_password: 'test-password' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/users/7/phone-binding', expect.objectContaining({
      method: 'DELETE',
      credentials: 'include',
      body: JSON.stringify({ note: '后台解绑手机号' })
    }))
  })

  it('sends admin user password reset, delete and batch delete requests', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.resetAdminUserPassword(7, { password: 'NewPass456' })
    await api.deleteAdminUser(7)
    await api.batchDeleteAdminUsers([7, 9])

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/users/7/reset-password', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ password: 'NewPass456' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/users/7', expect.objectContaining({
      method: 'DELETE',
      credentials: 'include',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/users/batch-delete', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ user_ids: [7, 9] })
    }))
  })

  it('sends extended package create, update and delete requests to the admin API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    const packagePayload = {
      name: '商业加速包',
      price_cents: 12800,
      credits: 88,
      valid_days: 45,
      audience: '商业创作者',
      tags: ['商用', '加急'],
      description: '面向商业海报和产品主图',
      is_active: false
    }
    await api.createAdminPackage(packagePayload)
    await api.updateAdminPackage(12, {
      name: '商业加速包 Pro',
      price_cents: 16800,
      credits: 120,
      valid_days: 60,
      audience: '工作室',
      tags: ['团队', '商用'],
      description: '升级后的商业套餐',
      is_active: true
    })
    await api.deleteAdminPackage(12)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/admin/packages', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify(packagePayload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/packages/12', expect.objectContaining({
      method: 'PUT',
      credentials: 'include',
      body: JSON.stringify({
        name: '商业加速包 Pro',
        price_cents: 16800,
        credits: 120,
        valid_days: 60,
        audience: '工作室',
        tags: ['团队', '商用'],
        description: '升级后的商业套餐',
        is_active: true
      })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/packages/12', expect.objectContaining({
      method: 'DELETE',
      credentials: 'include',
      body: JSON.stringify({})
    }))
  })

  it('reads public customer service config and updates the admin config', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    const payload = {
      title: '联系客服',
      wechat: { account: 'bailin_ai' },
      qq: { account: '123456789' },
      faqs: [{ question: '充值未到账怎么办？', answer: '联系客服处理' }]
    }

    await api.getCustomerService()
    await api.getAdminCustomerService()
    await api.updateAdminCustomerService(payload)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/customer-service', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/customer-service', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/customer-service', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify(payload)
    }))
  })

  it('uploads customer service qr code images with form data', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: vi.fn().mockResolvedValue({ url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/contact.png' })
    })
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['fake'], 'wechat.png', { type: 'image/png' })
    await api.uploadCustomerServiceQRCode(file)

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/customer-service/qrcode', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: expect.any(FormData)
    }))
    const [, options] = fetchMock.mock.calls[0]
    expect(options.headers?.['Content-Type']).toBeUndefined()
  })

  it('sends account profile, email and preference updates to the account API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.updateProfile({ display_name: '视觉主理人' })
    await api.updateAccountEmail({ email: 'creator@example.com' })
    await api.updateAccountPreferences({
      login_notification_enabled: false,
      risk_notification_enabled: true
    })
    await api.bindAccountPhone({
      phone: '13800138000',
      verification_code: '123456'
    })

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/account/profile', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({ display_name: '视觉主理人' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/account/email', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({ email: 'creator@example.com' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/account/preferences', expect.objectContaining({
      method: 'PATCH',
      credentials: 'include',
      body: JSON.stringify({
        login_notification_enabled: false,
        risk_notification_enabled: true
      })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/account/phone', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({
        phone: '13800138000',
        verification_code: '123456'
      })
    }))
  })

  it('calls couple album option admin and public APIs', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ locations: [], story_templates: [], styles: [] })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ items: [], total: 0 })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: vi.fn().mockResolvedValue({ id: 12 })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ id: 12 })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ ok: true })
      })
    vi.stubGlobal('fetch', fetchMock)

    await api.getCoupleAlbumOptions()
    await api.listAdminCoupleAlbumOptions({ type: 'location', active: 'true', q: '大理' })
    await api.createAdminCoupleAlbumOption({ type: 'location', value: '杭州', label: '杭州西湖' })
    await api.updateAdminCoupleAlbumOption(12, { type: 'location', value: '杭州', label: '杭州西湖旅拍' })
    await api.deleteAdminCoupleAlbumOption(12)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/couple-album/options', expect.objectContaining({ credentials: 'include' }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/couple-album-options?type=location&active=true&q=%E5%A4%A7%E7%90%86', expect.objectContaining({ credentials: 'include' }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/couple-album-options', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ type: 'location', value: '杭州', label: '杭州西湖' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/admin/couple-album-options/12', expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ type: 'location', value: '杭州', label: '杭州西湖旅拍' })
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/admin/couple-album-options/12', expect.objectContaining({
      method: 'DELETE',
      body: JSON.stringify({})
    }))
  })

  it('calls couple album user and public share APIs', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ required_credits: 24, available_credits: 40, enough: true })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: vi.fn().mockResolvedValue({ album: { id: 12 } })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 202,
        json: vi.fn().mockResolvedValue({ album: { id: 12, status: 'generating' } })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ album: { id: 12 } })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ albums: [] })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 202,
        json: vi.fn().mockResolvedValue({ album: { id: 12, status: 'generating' } })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ share_token: 'share-token' })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({ album: { title: '公开相册' } })
      })
    vi.stubGlobal('fetch', fetchMock)

    const payload = {
      title: '我们的旅行',
      location: '大理',
      story_template: 'city_walk',
      style: 'film',
      male_reference_asset_id: 1,
      female_reference_asset_id: 2
    }

    await api.estimateCoupleAlbum(payload)
    await api.createCoupleAlbum(payload)
    await api.generateCoupleAlbum(12)
    await api.getCoupleAlbum(12)
    await api.listCoupleAlbums()
    await api.retryCoupleAlbumPage(12, 99)
    await api.shareCoupleAlbum(12)
    await api.getPublicCoupleAlbum('share token')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/couple-albums/estimate', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'X-CSRF-Token': 'test-csrf-token' }),
      body: JSON.stringify(payload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/couple-albums', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(payload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/couple-albums/12/generate', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/couple-albums/12', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/couple-albums', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(6, '/api/couple-albums/12/pages/99/retry', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(7, '/api/couple-albums/12/share', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(8, '/api/public/couple-albums/share%20token', expect.objectContaining({
      credentials: 'include'
    }))
  })

  it('preserves credit shortfall fields on credits_insufficient errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: vi.fn().mockResolvedValue({
        error: {
          code: 'credits_insufficient',
          message: '点数不足，请先充值',
          required_credits: 24,
          available_credits: 8,
          missing_credits: 16,
          recommended_package: {
            id: 3,
            name: '高频包',
            credits: 140
          }
        }
      })
    }))

    await expect(api.generateCoupleAlbum(12)).rejects.toMatchObject({
      code: 'credits_insufficient',
      status: 409,
      required_credits: 24,
      available_credits: 8,
      missing_credits: 16,
      recommended_package: {
        id: 3,
        name: '高频包'
      }
    })
  })

  it('preserves virtual try-on body validation errors on ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: vi.fn().mockResolvedValue({
        error: {
          code: 'invalid_body_profile',
          message: '身型参数填写有误，请按提示修改',
          validation_errors: [
            {
              field: 'height_cm',
              label: '身高',
              value: 30,
              min: 80,
              max: 230,
              unit: 'cm',
              required: true
            }
          ]
        }
      })
    }))

    await expect(api.estimateVirtualTryOn({
      body_profile: { height_cm: 30, weight_kg: 58 },
      garment: { garment_reference_asset_id: 42, category: 'shirt' },
      scene: { category: 'work_business', sub_scene: 'office' },
      generation: { quality: 'high', aspect_ratio: '3:4' }
    })).rejects.toMatchObject({
      code: 'invalid_body_profile',
      status: 400,
      validation_errors: [
        {
          field: 'height_cm',
          label: '身高',
          value: 30,
          min: 80,
          max: 230,
          unit: 'cm',
          required: true
        }
      ]
    })
  })

  it('keeps workspace image, video, prompt optimization, and works endpoints aligned with the public contract', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    const imagePayload = {
      prompt: '水彩风格城市天际线',
      aspect_ratio: '21:9',
      model_id: 7,
      num: 4,
      quality: 'high',
      tool_mode: 'expand'
    }
    const videoPayload = {
      prompt: '产品旋转展示',
      aspect_ratio: '16:9',
      duration: '10',
      model: 'sora-2',
      hd: false,
      reference_asset_ids: []
    }
    const optimizePayload = {
      prompt: '一只猫',
      action: 'start',
      history: []
    }

    await api.getWorkspaceDiscovery()
    await api.estimateImageGeneration(imagePayload)
    await api.createImageGeneration(imagePayload)
    await api.getImageGeneration(88)
    await api.optimizePrompt(optimizePayload)
    await api.estimateVideoGeneration(videoPayload)
    await api.createVideoGeneration(videoPayload)
    await api.getVideoGeneration(99)
    await api.listWorks({ q: '猫', category: 'video', time_range: 'week', sort: 'oldest', page: 2, page_size: 30 })
    await api.getPublicWorks({ ids: '1,2,3' })
    await api.reuseWork(12)
    await api.deleteWork(12)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/workspace/discovery', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/images/generations/estimate', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'X-CSRF-Token': 'test-csrf-token' }),
      body: JSON.stringify(imagePayload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/images/generations/async', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(imagePayload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/images/generations/88', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/prompts/optimize', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(optimizePayload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(6, '/api/videos/generations/estimate', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(videoPayload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(7, '/api/videos/generations/async', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(videoPayload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(8, '/api/videos/generations/99', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(9, '/api/works?q=%E7%8C%AB&category=video&time_range=week&sort=oldest&page=2&page_size=30', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(10, '/api/public/works?ids=1%2C2%2C3', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(11, '/api/works/12/reuse', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(12, '/api/works/12', expect.objectContaining({
      method: 'DELETE',
      body: JSON.stringify({})
    }))
  })

  it('calls virtual try-on estimate and async generation endpoints', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)
    const payload = {
      body_profile: { height_cm: 170, weight_kg: 58 },
      garment: { garment_reference_asset_id: 42, category: 'shirt' },
      scene: { category: 'work_business', sub_scene: 'office' },
      generation: { quality: 'high', aspect_ratio: '3:4' }
    }

    await api.estimateVirtualTryOn(payload)
    await api.createVirtualTryOn(payload)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/virtual-try-on/generations/estimate', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'X-CSRF-Token': 'test-csrf-token' }),
      body: JSON.stringify(payload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/virtual-try-on/generations/async', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'X-CSRF-Token': 'test-csrf-token' }),
      body: JSON.stringify(payload)
    }))
  })

  it('passes an abort signal through regular image credit estimates', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()
    const imagePayload = {
      prompt: 'abortable estimate',
      aspect_ratio: '1:1',
      tool_mode: 'generate'
    }

    await api.estimateImageGeneration(imagePayload, { signal: controller.signal })

    expect(fetchMock).toHaveBeenCalledWith('/api/images/generations/estimate', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(imagePayload),
      signal: controller.signal
    }))
  })

  it('lists reference assets with an optional kind query', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [] })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.listReferenceAssets({ kind: 'video' })

    expect(fetchMock).toHaveBeenCalledWith('/api/reference-assets?kind=video', expect.objectContaining({
      credentials: 'include',
      headers: expect.objectContaining({
        'Content-Type': 'application/json'
      })
    }))
  })

  it('passes an abort signal through video credit estimates', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()
    const videoPayload = {
      prompt: 'abortable video estimate',
      aspect_ratio: '16:9',
      duration: '10',
      model: 'sora-2'
    }

    await api.estimateVideoGeneration(videoPayload, { signal: controller.signal })

    expect(fetchMock).toHaveBeenCalledWith('/api/videos/generations/estimate', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(videoPayload),
      signal: controller.signal
    }))
  })

  it('calls the image generation cancel endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ status: 'failed' })
    })
    vi.stubGlobal('fetch', fetchMock)

    await api.cancelImageGeneration(88)

    expect(fetchMock).toHaveBeenCalledWith('/api/images/generations/88/cancel', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'X-CSRF-Token': 'test-csrf-token' }),
      body: JSON.stringify({})
    }))
  })

  it('keeps inspiration recommendation endpoints aligned with the public and admin contracts', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ ok: true })
    })
    vi.stubGlobal('fetch', fetchMock)

    const payload = {
      title: 'Cyberpunk City',
      slug: 'cyberpunk-city',
      prompt: 'cyberpunk city',
      preview_url: 'https://oss.example.com/cyber.png',
      heat_tags: ['weekly-hot'],
      params: { seed: 918 },
      is_active: true
    }

    await api.useInspirationRecommendation(12)
    await api.listAdminInspirationRecommendations({ q: 'city', active: 'true', page: 2, page_size: 20 })
    await api.createAdminInspirationRecommendation(payload)
    await api.updateAdminInspirationRecommendation(12, payload)
    await api.deleteAdminInspirationRecommendation(12)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/workspace/inspiration-recommendations/12/use', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({})
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/admin/inspiration-recommendations?q=city&active=true&page=2&page_size=20', expect.objectContaining({
      credentials: 'include'
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/inspiration-recommendations', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(payload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/admin/inspiration-recommendations/12', expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify(payload)
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/admin/inspiration-recommendations/12', expect.objectContaining({
      method: 'DELETE',
      body: JSON.stringify({})
    }))
  })
})
