import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe
  }
}))

import {
  applyAvailableCredits,
  clearCurrentUser,
  currentUser,
  setCurrentUser
} from '../stores/session.js'
import * as sessionStore from '../stores/session.js'

describe('session store', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearCurrentUser()
  })

  it('loads and shares the current user payload', async () => {
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 7,
      username: 'creator',
      available_credits: 12
    })

    const payload = await sessionStore.loadCurrentUser()

    expect(payload?.username).toBe('creator')
    expect(currentUser.value?.available_credits).toBe(12)
  })

  it('applies new available credits without replacing profile fields', () => {
    setCurrentUser({
      user_id: 7,
      username: 'creator',
      display_name: '创作者',
      available_credits: 12
    })

    applyAvailableCredits(9)

    expect(currentUser.value).toMatchObject({
      user_id: 7,
      username: 'creator',
      display_name: '创作者',
      available_credits: 9
    })
  })

  it('confirms a valid cookie session and writes it into shared state', async () => {
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 8,
      username: 'cookie-user',
      available_credits: 18
    })

    expect(sessionStore.ensureUserSession).toEqual(expect.any(Function))

    const result = await sessionStore.ensureUserSession()

    expect(result).toBe(true)
    expect(currentUser.value).toMatchObject({
      user_id: 8,
      username: 'cookie-user',
      available_credits: 18
    })
  })

  it('clears shared state when session confirmation returns 401', async () => {
    setCurrentUser({
      user_id: 7,
      username: 'creator'
    })
    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('unauthorized'), { status: 401 }))

    expect(sessionStore.ensureUserSession).toEqual(expect.any(Function))

    const result = await sessionStore.ensureUserSession()

    expect(result).toBe(false)
    expect(currentUser.value).toBeNull()
  })

  it('keeps an existing user when session confirmation fails temporarily', async () => {
    setCurrentUser({
      user_id: 7,
      username: 'creator'
    })
    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('服务暂不可用'), { status: 500 }))

    expect(sessionStore.ensureUserSession).toEqual(expect.any(Function))

    const result = await sessionStore.ensureUserSession()

    expect(result).toBe(true)
    expect(currentUser.value?.username).toBe('creator')
  })
})
