import { describe, it, expect, vi, afterEach } from 'vitest'
import { createRun, createSkill, approveFix, rejectFix, generateFix } from '../api'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('createRun', () => {
  it('sends POST and returns uid', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ metadata: { uid: 'abc-123' } }),
    }))

    const uid = await createRun({
      namespace: 'default',
      target: { scope: 'namespace', namespaces: ['default'] },
      modelConfigRef: 'gpt-4',
    })
    expect(uid).toBe('abc-123')

    const mockFetch = vi.mocked(fetch)
    expect(mockFetch).toHaveBeenCalledOnce()
    expect(mockFetch).toHaveBeenCalledWith('/api/runs', expect.objectContaining({ method: 'POST' }))
  })

  it('throws on HTTP error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: async () => 'Internal Server Error',
    }))

    await expect(
      createRun({
        namespace: 'default',
        target: { scope: 'namespace', namespaces: ['default'] },
        modelConfigRef: 'gpt-4',
      })
    ).rejects.toThrow('Internal Server Error')
  })
})

describe('createSkill', () => {
  it('sends POST without error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
    }))

    await expect(
      createSkill({
        name: 'my-skill',
        namespace: 'default',
        dimension: 'health',
        description: 'test skill',
        prompt: 'check health',
        tools: [],
        enabled: true,
      })
    ).resolves.toBeUndefined()

    const mockFetch = vi.mocked(fetch)
    expect(mockFetch).toHaveBeenCalledOnce()
    expect(mockFetch).toHaveBeenCalledWith('/api/skills', expect.objectContaining({ method: 'POST' }))
  })

  it('throws on error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      text: async () => 'Bad Request',
    }))

    await expect(
      createSkill({
        name: 'bad-skill',
        namespace: 'default',
        dimension: 'health',
        description: '',
        prompt: '',
        tools: [],
        enabled: false,
      })
    ).rejects.toThrow('Bad Request')
  })
})

describe('approveFix', () => {
  it('sends PATCH to approve endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
    }))

    await expect(approveFix('fix-42', 'admin')).resolves.toBeUndefined()

    const mockFetch = vi.mocked(fetch)
    expect(mockFetch).toHaveBeenCalledOnce()
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/fixes/fix-42/approve',
      expect.objectContaining({ method: 'PATCH' })
    )
  })
})

describe('rejectFix', () => {
  it('sends PATCH to reject endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
    }))

    await expect(rejectFix('fix-42')).resolves.toBeUndefined()

    const mockFetch = vi.mocked(fetch)
    expect(mockFetch).toHaveBeenCalledOnce()
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/fixes/fix-42/reject',
      expect.objectContaining({ method: 'PATCH' })
    )
  })
})

describe('generateFix', () => {
  it('returns fixID from response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ fixID: 'fix-1' }),
    }))

    const result = await generateFix('finding-99')
    expect(result).toEqual({ fixID: 'fix-1' })

    const mockFetch = vi.mocked(fetch)
    expect(mockFetch).toHaveBeenCalledOnce()
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/findings/finding-99/generate-fix',
      expect.objectContaining({ method: 'POST' })
    )
  })
})
