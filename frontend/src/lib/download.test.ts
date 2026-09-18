import { describe, expect, it } from 'vitest';
import { crossTableCsv } from './download';

describe('crossTableCsv', () => {
  it('builds Group / Total / period columns in order', () => {
    const table = crossTableCsv(
      ['2026-01-01', '2026-01-02'],
      [
        { group: 'EC2', amounts: [1.5, 2], total: 3.5 },
        { group: 'S3', amounts: [0, 4.25], total: 4.25 },
      ],
    );

    expect(table.columns).toEqual(['Group', 'Total', '2026-01-01', '2026-01-02']);
    expect(table.rows).toEqual([
      ['EC2', '3.50', '1.50', '2.00'],
      ['S3', '4.25', '0.00', '4.25'],
    ]);
  });

  it('returns only Group and Total when there are no categories', () => {
    const table = crossTableCsv([], [{ group: 'Other', amounts: [], total: 10 }]);
    expect(table.columns).toEqual(['Group', 'Total']);
    expect(table.rows).toEqual([['Other', '10.00']]);
  });
});
