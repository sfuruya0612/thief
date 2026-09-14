import { describe, expect, it } from 'vitest';
import {
  datadogDashboardDetailFromRaw,
  datadogMetricQueryResultFromRaw,
  datadogDashboardFromRaw,
  datadogLoginStartFromRaw,
  datadogLoginStatusFromRaw,
  datadogOrgFromRaw,
  datadogWidgetFromRaw,
} from './normalizeNonAws';

describe('datadogOrgFromRaw', () => {
  it('logged_in を camelCase に変換する', () => {
    expect(
      datadogOrgFromRaw({ id: 'abc123', name: 'Parent Org', logged_in: true, is_self: true }),
    ).toEqual({
      id: 'abc123',
      name: 'Parent Org',
      loggedIn: true,
      isSelf: true,
    });
  });

  it('未ログインの組織をそのまま通す', () => {
    expect(
      datadogOrgFromRaw({ id: 'sub456', name: 'Sub Org', logged_in: false, is_self: false })
        .loggedIn,
    ).toBe(false);
  });

  it('表示名が空なら id を表示名にする (タブのラベルが空になるのを防ぐ)', () => {
    expect(
      datadogOrgFromRaw({ id: 'sub456', name: '', logged_in: false, is_self: false }).name,
    ).toBe('sub456');
  });

  it('is_self を camelCase に変換する', () => {
    expect(
      datadogOrgFromRaw({ id: 'sub456', name: 'Sub Org', logged_in: false, is_self: false }).isSelf,
    ).toBe(false);
  });
});

describe('datadogLoginStartFromRaw', () => {
  it('state / authorization_url を camelCase に変換する', () => {
    const row = datadogLoginStartFromRaw({
      state: 'abc123',
      authorization_url: 'https://app.datadoghq.com/oauth2/v1/authorize?state=abc123',
    });
    expect(row).toEqual({
      state: 'abc123',
      authorizationUrl: 'https://app.datadoghq.com/oauth2/v1/authorize?state=abc123',
    });
  });
});

describe('datadogLoginStatusFromRaw', () => {
  it('status: pending をそのまま通す', () => {
    const row = datadogLoginStatusFromRaw({ status: 'pending' });
    expect(row).toEqual({ status: 'pending', errorMessage: '' });
  });

  it('status: succeeded をそのまま通す', () => {
    const row = datadogLoginStatusFromRaw({ status: 'succeeded' });
    expect(row).toEqual({ status: 'succeeded', errorMessage: '' });
  });

  it('status: failed で error_message をそのまま通す', () => {
    const row = datadogLoginStatusFromRaw({ status: 'failed', error_message: 'access_denied' });
    expect(row).toEqual({ status: 'failed', errorMessage: 'access_denied' });
  });

  it('error_message が未指定なら空文字にする', () => {
    const row = datadogLoginStatusFromRaw({ status: 'failed' });
    expect(row.errorMessage).toBe('');
  });

  it('backend が返す未知の status 値は failed として扱う (ポーリングを止め続けるため)', () => {
    const row = datadogLoginStatusFromRaw({ status: 'unknown_future_status' });
    expect(row.status).toBe('failed');
  });
});

describe('datadogDashboardFromRaw', () => {
  it('一覧の項目をそのまま通す', () => {
    expect(
      datadogDashboardFromRaw({
        id: 'abc-123',
        title: 'Overview',
        description: 'main',
        url: 'https://app.datadoghq.com/dashboard/abc-123/overview',
      }),
    ).toEqual({
      id: 'abc-123',
      title: 'Overview',
      description: 'main',
      url: 'https://app.datadoghq.com/dashboard/abc-123/overview',
    });
  });

  it('タイトルが空なら id を表示名にする (一覧の行が空になるのを防ぐ)', () => {
    expect(
      datadogDashboardFromRaw({ id: 'abc-123', title: '', description: '', url: '' }).title,
    ).toBe('abc-123');
  });
});

describe('datadogWidgetFromRaw', () => {
  it('timeseries ウィジェットをクエリ付きで変換する', () => {
    expect(
      datadogWidgetFromRaw({
        id: 1,
        kind: 'timeseries',
        type: 'timeseries',
        title: 'CPU',
        queries: ['avg:system.cpu.user{*}'],
      }),
    ).toEqual({
      id: 1,
      kind: 'timeseries',
      type: 'timeseries',
      title: 'CPU',
      queries: ['avg:system.cpu.user{*}'],
    });
  });

  it('未対応ウィジェットは種別名を残す', () => {
    const row = datadogWidgetFromRaw({
      id: 2,
      kind: 'unsupported',
      type: 'toplist',
      title: 'Top',
      queries: null,
    });
    expect(row.kind).toBe('unsupported');
    expect(row.type).toBe('toplist');
    expect(row.queries).toEqual([]);
  });

  it('backend が返す未知の kind は unsupported として扱う (描き方が決まらないため)', () => {
    expect(
      datadogWidgetFromRaw({
        id: 3,
        kind: 'future_kind',
        type: 'future_widget',
        title: '',
        queries: null,
      }).kind,
    ).toBe('unsupported');
  });

  it('種別名が空なら unknown にする', () => {
    expect(
      datadogWidgetFromRaw({ id: 4, kind: 'unsupported', type: '', title: '', queries: null }).type,
    ).toBe('unknown');
  });
});

describe('datadogDashboardDetailFromRaw', () => {
  it('ウィジェットを Row へ変換する', () => {
    const row = datadogDashboardDetailFromRaw({
      id: 'abc-123',
      title: 'Overview',
      description: '',
      url: 'https://app.datadoghq.com/dashboard/abc-123/overview',
      widgets: [
        { id: 1, kind: 'timeseries', type: 'timeseries', title: 'CPU', queries: ['avg:x{*}'] },
        { id: 2, kind: 'unsupported', type: 'note', title: '', queries: [] },
      ],
    });
    expect(row.widgets.map((w) => w.kind)).toEqual(['timeseries', 'unsupported']);
  });

  it('widgets が null なら空配列にする', () => {
    const row = datadogDashboardDetailFromRaw({
      id: 'abc-123',
      title: 'Overview',
      description: '',
      url: '',
      widgets: null,
    });
    expect(row.widgets).toEqual([]);
  });
});

describe('datadogMetricQueryResultFromRaw', () => {
  it('系列と点を Row へ変換する', () => {
    const row = datadogMetricQueryResultFromRaw({
      query: 'avg:system.cpu.user{*}',
      series: [
        {
          name: 'avg:system.cpu.user{host:web-1}',
          scope: 'host:web-1',
          unit: '%',
          points: [
            { t: 1_700_000_000_000, v: 12.5 },
            { t: 1_700_000_060_000, v: 13.5 },
          ],
        },
      ],
    });
    expect(row.query).toBe('avg:system.cpu.user{*}');
    expect(row.series).toHaveLength(1);
    expect(row.series[0].unit).toBe('%');
    expect(row.series[0].points).toEqual([
      { t: 1_700_000_000_000, v: 12.5 },
      { t: 1_700_000_060_000, v: 13.5 },
    ]);
  });

  it('欠測は null のまま残し 0 に潰さない', () => {
    // 0 に潰すと「値が 0 だった」と読めてしまう。
    const row = datadogMetricQueryResultFromRaw({
      query: 'q',
      series: [{ name: 'q', scope: '', unit: '', points: [{ t: 1, v: null }] }],
    });
    expect(row.series[0].points).toEqual([{ t: 1, v: null }]);
  });

  it('series が null なら空配列にする', () => {
    const row = datadogMetricQueryResultFromRaw({ query: 'q', series: null });
    expect(row.series).toEqual([]);
  });

  it('points が null なら空配列にする', () => {
    const row = datadogMetricQueryResultFromRaw({
      query: 'q',
      series: [{ name: 'q', scope: '', unit: '', points: null }],
    });
    expect(row.series[0].points).toEqual([]);
  });
});
