import { describe, it, expect } from 'vitest'
import { symptomsToSkills, SYMPTOM_PRESETS, type SymptomPreset } from '../symptoms'

describe('symptomsToSkills', () => {
  it('returns the correct skills for a single symptom', () => {
    const result = symptomsToSkills(['cpu-high'])
    expect(result).toBeDefined()
    expect(result).toEqual(expect.arrayContaining(['pod-health-analyst', 'pod-cost-analyst']))
    expect(result).toHaveLength(2)
  })

  it('returns deduplicated merged skills for multiple symptoms', () => {
    // cpu-high: [pod-health-analyst, pod-cost-analyst]
    // pod-restart: [pod-health-analyst, reliability-analyst]
    // combined (deduplicated): [pod-health-analyst, pod-cost-analyst, reliability-analyst]
    const result = symptomsToSkills(['cpu-high', 'pod-restart'])
    expect(result).toBeDefined()
    expect(result).toEqual(
      expect.arrayContaining(['pod-health-analyst', 'pod-cost-analyst', 'reliability-analyst'])
    )
    expect(result).toHaveLength(3)
  })

  it('returns undefined for full-check', () => {
    const result = symptomsToSkills(['full-check'])
    expect(result).toBeUndefined()
  })

  it('returns undefined when full-check is mixed with other symptoms', () => {
    const result = symptomsToSkills(['cpu-high', 'full-check'])
    expect(result).toBeUndefined()
  })

  it('returns undefined for empty array', () => {
    const result = symptomsToSkills([])
    expect(result).toBeUndefined()
  })

  it('returns undefined for unknown symptom ids', () => {
    const result = symptomsToSkills(['unknown-symptom'])
    expect(result).toBeUndefined()
  })
})

describe('SYMPTOM_PRESETS', () => {
  it('has at least one preset', () => {
    expect(SYMPTOM_PRESETS.length).toBeGreaterThan(0)
  })

  it('all presets have required fields: id, label_zh, label_en, skills', () => {
    for (const preset of SYMPTOM_PRESETS) {
      expect(preset).toHaveProperty('id')
      expect(preset).toHaveProperty('label_zh')
      expect(preset).toHaveProperty('label_en')
      expect(preset).toHaveProperty('skills')

      expect(typeof preset.id).toBe('string')
      expect(preset.id.length).toBeGreaterThan(0)

      expect(typeof preset.label_zh).toBe('string')
      expect(preset.label_zh.length).toBeGreaterThan(0)

      expect(typeof preset.label_en).toBe('string')
      expect(preset.label_en.length).toBeGreaterThan(0)

      expect(Array.isArray(preset.skills)).toBe(true)
    }
  })

  it('all preset ids are unique', () => {
    const ids = SYMPTOM_PRESETS.map((p: SymptomPreset) => p.id)
    const uniqueIds = new Set(ids)
    expect(uniqueIds.size).toBe(ids.length)
  })

  it('contains a full-check preset with an empty skills array', () => {
    const fullCheck = SYMPTOM_PRESETS.find((p: SymptomPreset) => p.id === 'full-check')
    expect(fullCheck).toBeDefined()
    expect(fullCheck!.skills).toHaveLength(0)
  })
})
