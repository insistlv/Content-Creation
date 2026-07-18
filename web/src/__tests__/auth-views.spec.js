import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const authMocks = vi.hoisted(() => ({
  login: vi.fn(),
  adminLogin: vi.fn(),
  getCaptcha: vi.fn(),
  register: vi.fn(),
  sendSMSCode: vi.fn(),
  registerPhone: vi.fn(),
  resetPassword: vi.fn(),
  updateAccountEmail: vi.fn(),
  push: vi.fn()
}))
const routeState = vi.hoisted(() => ({
  query: {}
}))

vi.mock('../api/client.js', () => ({
  api: {
    login: authMocks.login,
    adminLogin: authMocks.adminLogin,
    getCaptcha: authMocks.getCaptcha,
    register: authMocks.register,
    sendSMSCode: authMocks.sendSMSCode,
    registerPhone: authMocks.registerPhone,
    resetPassword: authMocks.resetPassword,
    updateAccountEmail: authMocks.updateAccountEmail
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    push: authMocks.push
  })
}))

import LoginView from '../views/LoginView.vue'
import RegisterView from '../views/RegisterView.vue'
import AuthView from '../views/AuthView.vue'
import AuthForm from '../components/AuthForm.vue'
import AdminLoginView from '../views/AdminLoginView.vue'
import { clearCurrentUser, currentUser } from '../stores/session.js'

describe('auth views', () => {
  const mountOptions = {
    global: {
      stubs: {
        RouterLink: {
          props: ['to'],
          computed: {
            href() {
              if (typeof this.to === 'string') return this.to
              const query = new URLSearchParams(this.to?.query || {}).toString()
              return `${this.to?.path || ''}${query ? `?${query}` : ''}`
            }
          },
          template: '<a :href="href"><slot /></a>'
        }
      }
    }
  }

  beforeEach(() => {
    routeState.query = {}
    authMocks.getCaptcha.mockResolvedValue({
      captcha_id: 'cap-user',
      image_base64: 'png-user',
      expires_in: 300
    })
  })

  afterEach(() => {
    clearCurrentUser()
    vi.clearAllMocks()
    vi.useRealTimers()
  })

  it('renders the screenshot-style login page and keeps login workflow intact', async () => {
    authMocks.login.mockResolvedValueOnce({ phone: '13800138000' })

    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })

    expect(wrapper.text()).toContain('欢迎回来')
    expect(wrapper.text()).toContain('登录并进入工作台')
    expect(wrapper.text()).toContain('还没有账号？')
    expect(wrapper.text()).toContain('立即注册')
    expect(wrapper.find('.auth-visual').exists()).toBe(false)
    expect(wrapper.find('.auth-visual-image').exists()).toBe(false)
    expect(wrapper.findAll('.auth-feature-item')).toHaveLength(0)
    expect(wrapper.find('#login-user').exists()).toBe(true)
    expect(wrapper.find('#login-captcha').exists()).toBe(true)
    expect(wrapper.find('#register-user').exists()).toBe(false)
    await flushPromises()
    expect(authMocks.getCaptcha).toHaveBeenCalledWith('user_login')
    expect(wrapper.get('[data-testid="auth-captcha-image"]').attributes('src')).toBe('data:image/png;base64,png-user')

    await wrapper.get('#login-user').setValue('creator')
    await wrapper.get('#login-password').setValue('secret')
    await wrapper.get('#login-captcha').setValue('A2B3C')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.login).toHaveBeenCalledWith('creator', 'secret', {
      captcha_id: 'cap-user',
      captcha_code: 'A2B3C'
    }, { rememberLogin: false })
    expect(authMocks.push).toHaveBeenCalledWith('/workspace')
  })

  it('emits authenticated without navigating when the shared auth form logs in', async () => {
    authMocks.login.mockResolvedValueOnce({
      user_id: 9,
      username: 'creator',
      phone: '13800138000',
      available_credits: 12
    })
    const wrapper = mount(AuthForm, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    await flushPromises()

    await wrapper.get('#login-user').setValue('creator')
    await wrapper.get('#login-password').setValue('secret')
    await wrapper.get('#login-captcha').setValue('A2B3C')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.login).toHaveBeenCalledWith('creator', 'secret', {
      captcha_id: 'cap-user',
      captcha_code: 'A2B3C'
    }, { rememberLogin: false })
    expect(currentUser.value).toMatchObject({
      user_id: 9,
      username: 'creator',
      available_credits: 12
    })
    expect(wrapper.emitted('authenticated')?.[0]?.[0]).toMatchObject({ phone: '13800138000' })
    expect(authMocks.push).not.toHaveBeenCalled()
  })

  it('redirects legacy web accounts without phone binding to workspace after login', async () => {
    authMocks.login.mockResolvedValueOnce({ phone: null })

    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    await flushPromises()

    await wrapper.get('#login-user').setValue('legacy')
    await wrapper.get('#login-password').setValue('secret')
    await wrapper.get('#login-captcha').setValue('A2B3C')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.text()).not.toContain('当前账号还未绑定手机号')
    expect(authMocks.push).toHaveBeenCalledWith('/workspace')
  })

  it('refreshes and clears the web captcha after login errors', async () => {
    authMocks.getCaptcha
      .mockResolvedValueOnce({
        captcha_id: 'cap-user',
        image_base64: 'png-user',
        expires_in: 300
      })
      .mockResolvedValueOnce({
        captcha_id: 'cap-user-refresh',
        image_base64: 'png-user-refresh',
        expires_in: 300
      })
    authMocks.login.mockRejectedValueOnce(new Error('验证码错误或已过期'))

    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    await flushPromises()

    await wrapper.get('#login-user').setValue('creator')
    await wrapper.get('#login-password').setValue('secret')
    await wrapper.get('#login-captcha').setValue('WRONG')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.text()).toContain('验证码错误或已过期')
    expect(authMocks.getCaptcha).toHaveBeenCalledTimes(2)
    expect(wrapper.get('#login-captcha').element.value).toBe('')
    expect(wrapper.get('[data-testid="auth-captcha-image"]').attributes('src')).toBe('data:image/png;base64,png-user-refresh')
    expect(authMocks.push).not.toHaveBeenCalled()
  })

  it('registers web users with phone SMS verification in the shared auth page', async () => {
    authMocks.sendSMSCode.mockResolvedValueOnce({})
    authMocks.registerPhone.mockResolvedValueOnce({
      user_id: 10,
      username: 'creator',
      phone: '13800138000',
      available_credits: 20
    })

    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'register'
      }
    })

    expect(wrapper.text()).toContain('立即创建账号')
    expect(wrapper.text()).toContain('注册并开始创作')
    expect(wrapper.text()).toContain('已有账号？')
    expect(wrapper.text()).toContain('立即登录')
    expect(wrapper.text()).toContain('手机号')
    expect(wrapper.text()).toContain('短信验证码')
    expect(wrapper.text()).toContain('确认密码')
    expect(wrapper.text()).toContain('邀请码（可选）')
    expect(wrapper.text()).toContain('用户协议')
    expect(wrapper.text()).toContain('隐私政策')
    expect(wrapper.text()).toContain('算法公示')
    expect(wrapper.find('.auth-visual').exists()).toBe(false)
    expect(wrapper.find('.auth-visual-image').exists()).toBe(false)
    expect(wrapper.findAll('.auth-feature-item')).toHaveLength(0)
    expect(wrapper.get('[data-testid="auth-terms-link"]').attributes('href')).toBe('/terms')
    expect(wrapper.get('[data-testid="auth-privacy-link"]').attributes('href')).toBe('/privacy')
    expect(wrapper.get('[data-testid="auth-algorithm-link"]').attributes('href')).toBe('/algorithm-disclosure')
    expect(wrapper.find('#register-user').exists()).toBe(true)
    expect(wrapper.find('#register-phone').exists()).toBe(true)
    expect(wrapper.find('#register-code').exists()).toBe(true)
    expect(wrapper.find('#register-email').exists()).toBe(false)
    expect(wrapper.find('#register-confirm-password').exists()).toBe(true)
    expect(wrapper.find('#register-invite-code').exists()).toBe(true)
    expect(wrapper.find('#register-display-name').exists()).toBe(false)
    expect(wrapper.find('#login-user').exists()).toBe(false)

    await wrapper.get('#register-phone').setValue('13800138000')
    await wrapper.get('[data-testid="auth-send-register-code"]').trigger('click')
    await flushPromises()

    expect(authMocks.sendSMSCode).toHaveBeenCalledWith({
      phone: '13800138000',
      purpose: 'register'
    })

    await wrapper.get('#register-user').setValue('creator')
    await wrapper.get('#register-code').setValue('123456')
    await wrapper.get('#register-password').setValue('longsecret')
    await wrapper.get('#register-confirm-password').setValue('longsecret')
    await wrapper.get('#register-invite-code').setValue('INVITE-2026')
    await wrapper.get('[data-testid="auth-accept-terms"]').setValue(true)
    expect(wrapper.get('button.primary-button').attributes('disabled')).toBeUndefined()

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.registerPhone).toHaveBeenCalledWith({
      phone: '13800138000',
      verification_code: '123456',
      username: 'creator',
      password: 'longsecret',
      invite_code: 'INVITE-2026'
    })
    expect(authMocks.register).not.toHaveBeenCalled()
    expect(authMocks.updateAccountEmail).not.toHaveBeenCalled()
    expect(currentUser.value).toMatchObject({
      user_id: 10,
      username: 'creator',
      available_credits: 20
    })
    expect(authMocks.push).toHaveBeenCalledWith('/workspace')
  })

  it('locks duplicate phone registration and offers login or password reset recovery', async () => {
    const phoneExistsError = Object.assign(new Error('手机号已注册'), {
      code: 'phone_exists',
      status: 409
    })
    authMocks.sendSMSCode.mockRejectedValueOnce(phoneExistsError)

    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'register'
      }
    })

    await wrapper.get('#register-user').setValue('creator')
    await wrapper.get('#register-phone').setValue('13800138000')
    await wrapper.get('#register-code').setValue('123456')
    await wrapper.get('#register-password').setValue('longsecret')
    await wrapper.get('#register-confirm-password').setValue('longsecret')
    await wrapper.get('[data-testid="auth-accept-terms"]').setValue(true)
    expect(wrapper.get('button.primary-button').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="auth-send-register-code"]').trigger('click')
    await flushPromises()

    const phoneField = wrapper.get('[data-testid="auth-register-phone-field"]')
    expect(phoneField.text()).toContain('手机号已注册')
    expect(phoneField.text()).toContain('立即登录')
    expect(phoneField.text()).toContain('找回密码')
    expect(wrapper.get('[data-testid="auth-register-login-link"]').attributes('href')).toBe('/login')
    expect(wrapper.get('[data-testid="auth-register-reset-link"]').attributes('href')).toBe('/login?reset=1&phone=13800138000')
    expect(wrapper.get('#register-code').element.value).toBe('')
    expect(wrapper.get('[data-testid="auth-send-register-code"]').attributes()).toHaveProperty('disabled')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')
    const globalError = wrapper.find('.status-error')
    expect(globalError.exists() ? globalError.text() : '').not.toContain('手机号已注册')

    await wrapper.get('#register-phone').setValue('13900139000')
    expect(phoneField.text()).not.toContain('手机号已注册')
    expect(wrapper.get('[data-testid="auth-send-register-code"]').attributes('disabled')).toBeUndefined()
  })

  it('keeps phone-exists errors visible when final phone registration fails', async () => {
    const phoneExistsError = Object.assign(new Error('手机号已注册'), {
      code: 'phone_exists',
      status: 409
    })
    authMocks.registerPhone.mockRejectedValueOnce(phoneExistsError)

    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'register'
      }
    })

    await wrapper.get('#register-user').setValue('creator')
    await wrapper.get('#register-phone').setValue('13800138000')
    await wrapper.get('#register-code').setValue('123456')
    await wrapper.get('#register-password').setValue('longsecret')
    await wrapper.get('#register-confirm-password').setValue('longsecret')
    await wrapper.get('[data-testid="auth-accept-terms"]').setValue(true)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.registerPhone).toHaveBeenCalled()
    expect(wrapper.get('[data-testid="auth-register-phone-field"]').text()).toContain('手机号已注册')
    const globalError = wrapper.find('.status-error')
    expect(globalError.exists() ? globalError.text() : '').not.toContain('手机号已注册')
    expect(authMocks.push).not.toHaveBeenCalled()
  })

  it('uses a single centered auth card without the previous left-side visual', () => {
    const loginWrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    const registerWrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'register'
      }
    })

    expect(loginWrapper.get('.auth-agent-page').classes()).toContain('auth-agent-page-login')
    expect(loginWrapper.get('.auth-agent-page').classes()).not.toContain('auth-agent-page-register')
    expect(loginWrapper.get('.auth-card').classes()).toContain('auth-card-login')
    expect(loginWrapper.find('.auth-visual').exists()).toBe(false)
    expect(loginWrapper.find('.auth-visual-image').exists()).toBe(false)
    expect(loginWrapper.findAll('.auth-feature-item')).toHaveLength(0)
    expect(registerWrapper.get('.auth-agent-page').classes()).toContain('auth-agent-page-register')
    expect(registerWrapper.get('.auth-card').classes()).toContain('auth-card-register')
    expect(registerWrapper.get('.auth-card').classes()).not.toContain('auth-card-login')
    expect(registerWrapper.find('.auth-visual').exists()).toBe(false)
    expect(registerWrapper.find('.auth-visual-image').exists()).toBe(false)
    expect(registerWrapper.findAll('.auth-feature-item')).toHaveLength(0)
  })

  it('passes remember-login when submitting platform login', async () => {
    authMocks.login.mockResolvedValueOnce({ phone: '13800138000' })
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    await flushPromises()

    await wrapper.get('#login-user').setValue('creator')
    await wrapper.get('#login-password').setValue('secret')
    await wrapper.get('#login-captcha').setValue('A2B3C')
    await wrapper.get('[data-testid="auth-remember-login"]').setValue(true)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.login).toHaveBeenCalledWith('creator', 'secret', {
      captcha_id: 'cap-user',
      captcha_code: 'A2B3C'
    }, { rememberLogin: true })
  })

  it('redirects to a safe relative target after login', async () => {
    routeState.query = { redirect: '/workspace/video?seed=1' }
    authMocks.login.mockResolvedValueOnce({ phone: '13800138000' })
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    await flushPromises()

    await wrapper.get('#login-user').setValue('creator')
    await wrapper.get('#login-password').setValue('secret')
    await wrapper.get('#login-captcha').setValue('A2B3C')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.push).toHaveBeenCalledWith('/workspace/video?seed=1')
  })

  it('redirects legacy web accounts without phone binding to a safe relative target after login', async () => {
    routeState.query = { redirect: '/workspace/video?seed=1' }
    authMocks.login.mockResolvedValueOnce({ phone: null })
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    await flushPromises()

    await wrapper.get('#login-user').setValue('legacy')
    await wrapper.get('#login-password').setValue('secret')
    await wrapper.get('#login-captcha').setValue('A2B3C')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.text()).not.toContain('当前账号还未绑定手机号')
    expect(authMocks.push).toHaveBeenCalledWith('/workspace/video?seed=1')
  })

  it('falls back to workspace when login redirect is unsafe', async () => {
    routeState.query = { redirect: 'https://evil.example/workspace' }
    authMocks.login.mockResolvedValueOnce({ phone: '13800138000' })
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    await flushPromises()

    await wrapper.get('#login-user').setValue('creator')
    await wrapper.get('#login-password').setValue('secret')
    await wrapper.get('#login-captcha').setValue('A2B3C')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.push).toHaveBeenCalledWith('/workspace')
  })

  it('supports remember-login, password visibility and switching into reset-password form', async () => {
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })

    await wrapper.get('#login-password').setValue('secret')
    expect(wrapper.get('#login-password').attributes('type')).toBe('password')

    await wrapper.get('[data-testid="auth-toggle-password"]').trigger('click')
    expect(wrapper.get('#login-password').attributes('type')).toBe('text')

    await wrapper.get('[data-testid="auth-remember-login"]').setValue(true)
    expect(wrapper.get('[data-testid="auth-remember-login"]').element.checked).toBe(true)

    await wrapper.get('[data-testid="auth-forgot-password"]').trigger('click')
    expect(wrapper.text()).toContain('找回密码')
    expect(wrapper.text()).not.toContain('请联系管理员处理')
    expect(wrapper.find('#login-user').exists()).toBe(false)
    expect(wrapper.find('#reset-phone').exists()).toBe(true)
    expect(wrapper.find('#reset-code').exists()).toBe(true)
    expect(wrapper.find('#reset-password').exists()).toBe(true)
    expect(wrapper.find('#reset-confirm-password').exists()).toBe(true)

    await wrapper.get('[data-testid="auth-back-login"]').trigger('click')
    expect(wrapper.find('#login-user').exists()).toBe(true)
    expect(wrapper.find('#reset-phone').exists()).toBe(false)
    expect(authMocks.login).not.toHaveBeenCalled()
    expect(authMocks.register).not.toHaveBeenCalled()
    expect(authMocks.resetPassword).not.toHaveBeenCalled()
    expect(authMocks.updateAccountEmail).not.toHaveBeenCalled()
  })

  it('opens reset-password mode from login query and pre-fills the phone without sending SMS', async () => {
    routeState.query = { reset: '1', phone: '13800138000' }

    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('找回密码')
    expect(wrapper.find('#login-user').exists()).toBe(false)
    expect(wrapper.get('#reset-phone').element.value).toBe('13800138000')
    expect(authMocks.sendSMSCode).not.toHaveBeenCalled()
  })

  it('sends reset-password SMS codes with reset purpose', async () => {
    authMocks.sendSMSCode.mockResolvedValueOnce({})
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })

    await wrapper.get('[data-testid="auth-forgot-password"]').trigger('click')
    await wrapper.get('#reset-phone').setValue('13800138000')
    await wrapper.get('[data-testid="auth-send-reset-code"]').trigger('click')
    await flushPromises()

    expect(authMocks.sendSMSCode).toHaveBeenCalledWith({
      phone: '13800138000',
      purpose: 'reset_password'
    })
    expect(wrapper.text()).toContain('验证码已发送，请注意查收')
  })

  it('shows register SMS rate-limit cooldown with a 60 second fallback', async () => {
    vi.useFakeTimers()
    const rateLimitError = Object.assign(new Error('请求过于频繁，请稍后再试'), {
      code: 'too_many_requests',
      status: 429
    })
    authMocks.sendSMSCode.mockRejectedValueOnce(rateLimitError)
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'register'
      }
    })

    await wrapper.get('#register-phone').setValue('13800138000')
    await wrapper.get('[data-testid="auth-send-register-code"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="auth-register-code-field"]').text()).toContain('请求过于频繁，请稍后再试')
    expect(wrapper.get('[data-testid="auth-send-register-code"]').text()).toBe('60s')
    expect(wrapper.get('[data-testid="auth-send-register-code"]').attributes()).toHaveProperty('disabled')

    vi.advanceTimersByTime(1000)
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="auth-send-register-code"]').text()).toBe('59s')
  })

  it('shows missing reset phone errors next to the phone field only', async () => {
    const phoneNotFoundError = Object.assign(new Error('手机号未注册'), {
      code: 'phone_not_found',
      status: 404
    })
    authMocks.sendSMSCode.mockRejectedValueOnce(phoneNotFoundError)
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })

    await wrapper.get('[data-testid="auth-forgot-password"]').trigger('click')
    await wrapper.get('#reset-phone').setValue('13800138000')
    await wrapper.get('[data-testid="auth-send-reset-code"]').trigger('click')
    await flushPromises()

    const phoneField = wrapper.get('[data-testid="auth-reset-phone-field"]')
    expect(phoneField.text()).toContain('手机号未注册')
    const globalError = wrapper.find('.status-error')
    expect(globalError.exists() ? globalError.text() : '').not.toContain('手机号未注册')
  })

  it('submits reset password and returns to login with the phone prefilled', async () => {
    authMocks.resetPassword.mockResolvedValueOnce({})
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })

    await wrapper.get('[data-testid="auth-forgot-password"]').trigger('click')
    await wrapper.get('#reset-phone').setValue('13800138000')
    await wrapper.get('#reset-code').setValue('123456')
    await wrapper.get('#reset-password').setValue('NewPass123')
    await wrapper.get('#reset-confirm-password').setValue('NewPass123')
    expect(wrapper.get('button.primary-button').attributes('disabled')).toBeUndefined()

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.resetPassword).toHaveBeenCalledWith({
      phone: '13800138000',
      verification_code: '123456',
      new_password: 'NewPass123'
    })
    expect(wrapper.find('#reset-phone').exists()).toBe(false)
    expect(wrapper.get('#login-user').element.value).toBe('13800138000')
    expect(wrapper.text()).toContain('密码已重置，请使用新密码登录')
  })

  it('blocks invalid reset-password details before calling APIs', async () => {
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'login'
      }
    })

    await wrapper.get('[data-testid="auth-forgot-password"]').trigger('click')
    await wrapper.get('#reset-phone').setValue('12800138000')
    await wrapper.get('#reset-code').setValue('123456')
    await wrapper.get('#reset-password').setValue('NewPass123')
    await wrapper.get('#reset-confirm-password').setValue('NewPass123')
    expect(wrapper.text()).toContain('请输入有效手机号')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('#reset-phone').setValue('13800138000')
    await wrapper.get('#reset-code').setValue('12345')
    expect(wrapper.text()).toContain('请输入 6 位短信验证码')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('#reset-code').setValue('123456')
    await wrapper.get('#reset-password').setValue('short')
    await wrapper.get('#reset-confirm-password').setValue('short')
    expect(wrapper.text()).toContain('还差 3 位')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('#reset-password').setValue('NewPass123')
    await wrapper.get('#reset-confirm-password').setValue('OtherPass123')
    expect(wrapper.text()).toContain('两次输入的密码不一致')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.sendSMSCode).not.toHaveBeenCalled()
    expect(authMocks.resetPassword).not.toHaveBeenCalled()
    expect(authMocks.login).not.toHaveBeenCalled()
  })

  it('explains why short registration passwords cannot be submitted', async () => {
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'register'
      }
    })

    await wrapper.get('#register-user').setValue('xiaobai')
    await wrapper.get('#register-phone').setValue('13800138000')
    await wrapper.get('#register-code').setValue('123456')
    await wrapper.get('#register-password').setValue('123456')
    await wrapper.get('#register-confirm-password').setValue('123456')
    await wrapper.get('[data-testid="auth-accept-terms"]').setValue(true)

    expect(wrapper.text()).toContain('还差 2 位')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.register).not.toHaveBeenCalled()
    expect(authMocks.registerPhone).not.toHaveBeenCalled()
    expect(authMocks.updateAccountEmail).not.toHaveBeenCalled()
    expect(authMocks.push).not.toHaveBeenCalled()
  })

  it('blocks invalid registration details before calling registration APIs', async () => {
    const wrapper = mount(AuthView, {
      ...mountOptions,
      props: {
        mode: 'register'
      }
    })

    await wrapper.get('#register-user').setValue('creator')
    await wrapper.get('#register-phone').setValue('13800138000')
    await wrapper.get('#register-code').setValue('123456')
    await wrapper.get('#register-password').setValue('longsecret')
    await wrapper.get('#register-confirm-password').setValue('different')
    await wrapper.get('[data-testid="auth-accept-terms"]').setValue(true)
    expect(wrapper.text()).toContain('两次输入的密码不一致')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('#register-confirm-password').setValue('longsecret')
    await wrapper.get('#register-phone').setValue('12800138000')
    expect(wrapper.text()).toContain('请输入有效手机号')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('#register-phone').setValue('13800138000')
    await wrapper.get('#register-code').setValue('')
    expect(wrapper.text()).toContain('请输入 6 位短信验证码')
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('#register-code').setValue('123456')
    await wrapper.get('[data-testid="auth-accept-terms"]').setValue(false)
    expect(wrapper.get('button.primary-button').attributes()).toHaveProperty('disabled')

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.register).not.toHaveBeenCalled()
    expect(authMocks.registerPhone).not.toHaveBeenCalled()
    expect(authMocks.updateAccountEmail).not.toHaveBeenCalled()
    expect(authMocks.push).not.toHaveBeenCalled()
  })

  it('keeps the route-specific wrappers pointed at the shared auth page', () => {
    const loginWrapper = mount(LoginView, mountOptions)
    const registerWrapper = mount(RegisterView, mountOptions)

    expect(loginWrapper.text()).toContain('欢迎回来')
    expect(registerWrapper.text()).toContain('立即创建账号')
  })

  it('renders the admin control-room login layout with visual, copy and controls', async () => {
    authMocks.getCaptcha.mockResolvedValueOnce({
      captcha_id: 'cap-admin',
      image_base64: 'png-admin',
      expires_in: 300
    })
    const wrapper = mount(AdminLoginView)
    await flushPromises()

    expect(wrapper.text()).toContain('白霖共享 Admin')
    expect(wrapper.text()).toContain('后台控制中心')
    expect(wrapper.text()).toContain('数据可视化')
    expect(wrapper.text()).toContain('权限安全')
    expect(wrapper.text()).toContain('系统监控')
    expect(wrapper.text()).toContain('管理员登录')
    expect(wrapper.text()).toContain('记住登录状态')
    expect(wrapper.get('.admin-login-hero-image').attributes('src')).toContain('admin-login-hero.png')
    expect(wrapper.find('#adminUser').exists()).toBe(true)
    expect(wrapper.find('#adminPass').exists()).toBe(true)
    expect(wrapper.find('#adminCaptcha').exists()).toBe(true)
    expect(authMocks.getCaptcha).toHaveBeenCalledWith('admin_login')
    expect(wrapper.get('[data-testid="admin-captcha-image"]').attributes('src')).toBe('data:image/png;base64,png-admin')

    await wrapper.get('#adminPass').setValue('secret')
    expect(wrapper.get('#adminPass').attributes('type')).toBe('password')

    await wrapper.get('[data-testid="admin-toggle-password"]').trigger('click')
    expect(wrapper.get('#adminPass').attributes('type')).toBe('text')

    await wrapper.get('[data-testid="admin-remember-login"]').setValue(true)
    expect(wrapper.get('[data-testid="admin-remember-login"]').element.checked).toBe(true)

    await wrapper.get('[data-testid="admin-forgot-password"]').trigger('click')
    expect(wrapper.text()).toContain('请联系系统管理员重置密码')
    expect(authMocks.adminLogin).not.toHaveBeenCalled()
  })

  it('keeps admin login submission pointed at the existing API and admin route', async () => {
    authMocks.adminLogin.mockResolvedValueOnce({})
    authMocks.getCaptcha.mockResolvedValueOnce({
      captcha_id: 'cap-admin',
      image_base64: 'png-admin',
      expires_in: 300
    })
    const wrapper = mount(AdminLoginView)
    await flushPromises()

    await wrapper.get('#adminUser').setValue('  root  ')
    await wrapper.get('#adminPass').setValue('secret')
    await wrapper.get('#adminCaptcha').setValue('D4E5F')
    await wrapper.get('[data-testid="admin-remember-login"]').setValue(true)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(authMocks.adminLogin).toHaveBeenCalledWith('root', 'secret', {
      captcha_id: 'cap-admin',
      captcha_code: 'D4E5F'
    }, { rememberLogin: true })
    expect(authMocks.push).toHaveBeenCalledWith('/admin')
  })

  it('shows admin login API errors without navigating', async () => {
    authMocks.adminLogin.mockRejectedValueOnce(new Error('账号或密码错误'))
    authMocks.getCaptcha
      .mockResolvedValueOnce({
        captcha_id: 'cap-admin',
        image_base64: 'png-admin',
        expires_in: 300
      })
      .mockResolvedValueOnce({
        captcha_id: 'cap-admin-refresh',
        image_base64: 'png-admin-refresh',
        expires_in: 300
      })
    const wrapper = mount(AdminLoginView)
    await flushPromises()

    await wrapper.get('#adminUser').setValue('root')
    await wrapper.get('#adminPass').setValue('wrong')
    await wrapper.get('#adminCaptcha').setValue('WRONG')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.text()).toContain('账号或密码错误')
    expect(authMocks.getCaptcha).toHaveBeenCalledTimes(2)
    expect(wrapper.get('#adminCaptcha').element.value).toBe('')
    expect(wrapper.get('[data-testid="admin-captcha-image"]').attributes('src')).toBe('data:image/png;base64,png-admin-refresh')
    expect(authMocks.push).not.toHaveBeenCalled()
  })
})
