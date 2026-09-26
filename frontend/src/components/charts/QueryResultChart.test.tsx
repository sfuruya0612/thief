// QueryResultChart が組み立てる option の検証。echarts-for-react は jsdom (canvas 未実装)
// では描画できないため、option を捕まえるスタブに差し替える。軸と凡例の文字色がテーマ
// (dark / light) ごとに THEME_TEXT_COLOR の値になることまで確認する (既存のグラフの
// テストにはテーマごとの色の検証が無い)。
import { render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { QueryResultChart } from './QueryResultChart';
import { THEME_TEXT_COLOR } from './chartTheme';
import { SERIES_COLORS } from '../../lib/timeseries';
import { STORAGE_KEY } from '../../lib/storage';
import { resetTweaksForTest } from '../../hooks/useTweaks';

const captured = vi.hoisted(() => ({ option: {} as Record<string, unknown> }));

vi.mock('echarts-for-react', () => ({
  default: (props: { option: Record<string, unknown> }) => {
    captured.option = props.option;
    return <div data-testid="echarts-stub" />;
  },
}));

const columns = ['name', 'count', 'ratio'];
const rows = [
  ['a', '1', '1.5'],
  ['b', '', '2.5'],
  ['c', '1,234', ''],
];

// renderWithTheme は localStorage の永続化 (cloudlens:v1) からテーマを復元させて描画する。
// useTweaks の共有ストアはモジュールに残るため、未初期化に戻してから描画する。
function renderWithTheme(theme: 'dark' | 'light', type: 'bar' | 'line' = 'bar') {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ tweaks: { theme } }));
  resetTweaksForTest();
  return render(
    <QueryResultChart columns={columns} rows={rows} xIndex={0} yIndexes={[1, 2]} type={type} />,
  );
}

interface SeriesOption {
  name: string;
  type: string;
  data: (number | null)[];
}

describe('QueryResultChart の option', () => {
  afterEach(() => {
    localStorage.clear();
    resetTweaksForTest();
    captured.option = {};
  });

  it('X 列の値を文字列のままカテゴリ軸に置き、Y 列を系列にする', () => {
    renderWithTheme('light');

    expect(captured.option.xAxis).toMatchObject({ type: 'category', data: ['a', 'b', 'c'] });
    expect(captured.option.series as SeriesOption[]).toEqual([
      { name: 'count', type: 'bar', emphasis: { focus: 'series' }, data: [1, null, 1234] },
      { name: 'ratio', type: 'bar', emphasis: { focus: 'series' }, data: [1.5, 2.5, null] },
    ]);
  });

  it('type が line のときは系列を折れ線にする', () => {
    renderWithTheme('light', 'line');

    const series = captured.option.series as SeriesOption[];
    expect(series.map((s) => s.type)).toEqual(['line', 'line']);
  });

  it('系列の色は時系列グラフと同じ固定順のパレットを使う', () => {
    renderWithTheme('light');

    expect(captured.option.color).toEqual(SERIES_COLORS);
  });

  it('THEME_TEXT_COLOR は dark で #a8a8b0、light で #5c5c66 を指す', () => {
    expect(THEME_TEXT_COLOR).toEqual({ dark: '#a8a8b0', light: '#5c5c66' });
  });

  it.each([
    ['dark', '#a8a8b0'],
    ['light', '#5c5c66'],
  ] as const)('テーマ %s では軸と凡例の文字色を %s にする', (theme, expectedColor) => {
    renderWithTheme(theme);

    expect(captured.option.textStyle).toMatchObject({ color: expectedColor });
    expect(captured.option.legend).toMatchObject({ textStyle: { color: expectedColor } });
    expect(captured.option.xAxis).toMatchObject({
      axisLabel: { color: expectedColor },
      axisLine: { lineStyle: { color: expectedColor } },
    });
    expect(captured.option.yAxis).toMatchObject({ axisLabel: { color: expectedColor } });
  });
});
