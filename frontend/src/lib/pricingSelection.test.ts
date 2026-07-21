import { describe, expect, it } from 'vitest';
import type { PricingPersistedState } from './storage';
import {
  initialPricingState,
  migratePricingState,
  pricingReducer,
  PRICING_SCHEMA_VERSION,
  PRICING_SERVICES,
} from './pricingSelection';

describe('initialPricingState', () => {
  it('永続化データが無い場合は全サービスを選択した状態で初期化する', () => {
    const state = initialPricingState(undefined);
    expect(state.activeServices).toEqual([...PRICING_SERVICES]);
    expect(state.collapsed).toEqual({});
    expect(state.selection).toEqual({});
    expect(state.pricingSchemaVersion).toBe(PRICING_SCHEMA_VERSION);
  });

  it('永続化データのスキーマ版が最新なら変更せずそのまま使う', () => {
    const persisted: PricingPersistedState = {
      activeServices: ['ec2'],
      collapsed: { rds: true },
      selection: { 'ap-northeast-1': { ec2: { 'sku.1': { checked: true, qty: 3 } } } },
      pricingSchemaVersion: PRICING_SCHEMA_VERSION,
    };
    expect(initialPricingState(persisted)).toBe(persisted);
  });
});

describe('migratePricingState', () => {
  it('スキーマ版が古い場合、その版までに追加された新サービスだけを activeServices に補完する', () => {
    const persisted: PricingPersistedState = {
      activeServices: ['ec2'],
      collapsed: {},
      selection: {},
      pricingSchemaVersion: 0,
    };
    const next = migratePricingState(persisted);
    expect(next.activeServices).toEqual(['ec2', 'compute-sp', 'ec2-instance-sp', 'database-sp']);
    expect(next.pricingSchemaVersion).toBe(PRICING_SCHEMA_VERSION);
  });

  it('pricingSchemaVersion が未設定の永続化データは版 0 として扱う', () => {
    const persisted: PricingPersistedState = {
      activeServices: [],
      collapsed: {},
      selection: {},
    };
    const next = migratePricingState(persisted);
    expect(next.activeServices).toEqual(['compute-sp', 'ec2-instance-sp', 'database-sp']);
  });

  it('ユーザーが手動で OFF にした既存サービスは補完で復活させない', () => {
    const persisted: PricingPersistedState = {
      activeServices: ['rds'],
      collapsed: {},
      selection: {},
      pricingSchemaVersion: 0,
    };
    const next = migratePricingState(persisted);
    expect(next.activeServices).toEqual(['rds', 'compute-sp', 'ec2-instance-sp', 'database-sp']);
    expect(next.activeServices).not.toContain('ec2');
  });
});

describe('pricingReducer / toggleService', () => {
  it('未選択のサービスをアクティブに追加する', () => {
    const state = initialPricingState({
      activeServices: [],
      collapsed: {},
      selection: {},
      pricingSchemaVersion: PRICING_SCHEMA_VERSION,
    });
    const next = pricingReducer(state, { type: 'toggleService', service: 'ec2' });
    expect(next.activeServices).toEqual(['ec2']);
  });

  it('選択済みのサービスを除外する', () => {
    const state = initialPricingState({
      activeServices: ['ec2', 'rds'],
      collapsed: {},
      selection: {},
      pricingSchemaVersion: PRICING_SCHEMA_VERSION,
    });
    const next = pricingReducer(state, { type: 'toggleService', service: 'ec2' });
    expect(next.activeServices).toEqual(['rds']);
  });
});

describe('pricingReducer / toggleCollapsed', () => {
  it('サービスごとに独立して折りたたみをトグルする', () => {
    let state = initialPricingState();
    state = pricingReducer(state, { type: 'toggleCollapsed', service: 'ec2' });
    expect(state.collapsed).toEqual({ ec2: true });
    state = pricingReducer(state, { type: 'toggleCollapsed', service: 'ec2' });
    expect(state.collapsed).toEqual({ ec2: false });
    state = pricingReducer(state, { type: 'toggleCollapsed', service: 'rds' });
    expect(state.collapsed).toEqual({ ec2: false, rds: true });
  });
});

describe('pricingReducer / toggleRate', () => {
  it('初回トグルで checked:true, qty:1 のエントリを作成する', () => {
    const state = initialPricingState();
    const next = pricingReducer(state, {
      type: 'toggleRate',
      region: 'ap-northeast-1',
      service: 'ec2',
      rateId: 'sku.1',
    });
    expect(next.selection['ap-northeast-1'].ec2['sku.1']).toEqual({ checked: true, qty: 1 });
  });

  it('再トグルで checked を反転し、qty は保持する', () => {
    let state = initialPricingState();
    state = pricingReducer(state, {
      type: 'toggleRate',
      region: 'ap-northeast-1',
      service: 'ec2',
      rateId: 'sku.1',
    });
    state = pricingReducer(state, {
      type: 'setQty',
      region: 'ap-northeast-1',
      service: 'ec2',
      rateId: 'sku.1',
      qty: 5,
    });
    state = pricingReducer(state, {
      type: 'toggleRate',
      region: 'ap-northeast-1',
      service: 'ec2',
      rateId: 'sku.1',
    });
    expect(state.selection['ap-northeast-1'].ec2['sku.1']).toEqual({ checked: false, qty: 5 });
  });

  it('リージョン/サービスをまたいで独立に管理する', () => {
    let state = initialPricingState();
    state = pricingReducer(state, {
      type: 'toggleRate',
      region: 'ap-northeast-1',
      service: 'ec2',
      rateId: 'sku.1',
    });
    state = pricingReducer(state, {
      type: 'toggleRate',
      region: 'us-east-1',
      service: 'ec2',
      rateId: 'sku.1',
    });
    state = pricingReducer(state, {
      type: 'toggleRate',
      region: 'ap-northeast-1',
      service: 'rds',
      rateId: 'sku.1',
    });
    expect(state.selection['ap-northeast-1'].ec2['sku.1'].checked).toBe(true);
    expect(state.selection['us-east-1'].ec2['sku.1'].checked).toBe(true);
    expect(state.selection['ap-northeast-1'].rds['sku.1'].checked).toBe(true);
  });
});

describe('pricingReducer / setQty', () => {
  it('未チェックのまま qty だけ設定できる', () => {
    const state = initialPricingState();
    const next = pricingReducer(state, {
      type: 'setQty',
      region: 'ap-northeast-1',
      service: 'ec2',
      rateId: 'sku.1',
      qty: 10,
    });
    expect(next.selection['ap-northeast-1'].ec2['sku.1']).toEqual({ checked: false, qty: 10 });
  });
});

describe('pricingReducer / pruneStaleRates', () => {
  function seed(): PricingPersistedState {
    return {
      activeServices: [...PRICING_SERVICES],
      collapsed: {},
      selection: {
        'ap-northeast-1': {
          ec2: {
            'sku.1': { checked: true, qty: 1 },
            'sku.stale': { checked: true, qty: 2 },
          },
          rds: {
            'sku.2': { checked: true, qty: 1 },
          },
        },
      },
      pricingSchemaVersion: PRICING_SCHEMA_VERSION,
    };
  }

  it('現テーブルに存在しない rate_id を除去する', () => {
    const state = seed();
    const next = pricingReducer(state, {
      type: 'pruneStaleRates',
      region: 'ap-northeast-1',
      service: 'ec2',
      validRateIds: ['sku.1'],
    });
    expect(next.selection['ap-northeast-1'].ec2).toEqual({ 'sku.1': { checked: true, qty: 1 } });
    // 他サービス/他リージョンには影響しない
    expect(next.selection['ap-northeast-1'].rds).toEqual({ 'sku.2': { checked: true, qty: 1 } });
  });

  it('除去対象が無ければ同一の state を返す (無駄な再レンダーを避ける)', () => {
    const state = seed();
    const next = pricingReducer(state, {
      type: 'pruneStaleRates',
      region: 'ap-northeast-1',
      service: 'rds',
      validRateIds: ['sku.2'],
    });
    expect(next).toBe(state);
  });

  it('対象リージョン/サービスの選択が無ければ何もしない', () => {
    const state = seed();
    const next = pricingReducer(state, {
      type: 'pruneStaleRates',
      region: 'us-east-1',
      service: 'ec2',
      validRateIds: [],
    });
    expect(next).toBe(state);
  });
});

describe('pricingReducer / clearSelection', () => {
  it('対象リージョンの全サービスの選択を空にする', () => {
    const state: PricingPersistedState = {
      activeServices: [...PRICING_SERVICES],
      collapsed: {},
      selection: {
        'ap-northeast-1': {
          ec2: { 'sku.1': { checked: true, qty: 2 } },
          rds: { 'sku.2': { checked: true, qty: 1 } },
        },
      },
      pricingSchemaVersion: PRICING_SCHEMA_VERSION,
    };
    const next = pricingReducer(state, { type: 'clearSelection', region: 'ap-northeast-1' });
    expect(next.selection['ap-northeast-1']).toEqual({});
  });

  it('他リージョンの選択には影響しない', () => {
    const state: PricingPersistedState = {
      activeServices: [...PRICING_SERVICES],
      collapsed: {},
      selection: {
        'ap-northeast-1': { ec2: { 'sku.1': { checked: true, qty: 2 } } },
        'us-east-1': { ec2: { 'sku.1': { checked: true, qty: 3 } } },
      },
      pricingSchemaVersion: PRICING_SCHEMA_VERSION,
    };
    const next = pricingReducer(state, { type: 'clearSelection', region: 'ap-northeast-1' });
    expect(next.selection['us-east-1']).toEqual({
      ec2: { 'sku.1': { checked: true, qty: 3 } },
    });
  });

  it('対象リージョンの選択が元々無くても空オブジェクトになる', () => {
    const state = initialPricingState();
    const next = pricingReducer(state, { type: 'clearSelection', region: 'ap-northeast-1' });
    expect(next.selection['ap-northeast-1']).toEqual({});
  });
});
