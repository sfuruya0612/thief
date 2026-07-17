import { describe, expect, it } from 'vitest';
import { presetToRange } from './logTimeRange';

describe('presetToRange', () => {
  const now = new Date('2026-07-18T12:00:00.000Z');

  it.each([
    ['15m', '2026-07-18T11:45:00.000Z'],
    ['1h', '2026-07-18T11:00:00.000Z'],
    ['6h', '2026-07-18T06:00:00.000Z'],
    ['24h', '2026-07-17T12:00:00.000Z'],
    ['7d', '2026-07-11T12:00:00.000Z'],
  ] as const)('%s は now から遡った start と now を end に変換する', (preset, wantStart) => {
    const range = presetToRange(preset, now);
    expect(range).toEqual({ start: wantStart, end: '2026-07-18T12:00:00.000Z' });
  });
});
