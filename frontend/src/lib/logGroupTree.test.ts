import { describe, expect, it } from 'vitest';
import { buildLogGroupTree } from './logGroupTree';

describe('buildLogGroupTree', () => {
  it('親 prefix でグルーピングし、葉に ARN を持たせる', () => {
    const tree = buildLogGroupTree([
      { name: '/aws/lambda/api-handler', arn: 'arn:a' },
      { name: '/aws/lambda/batch-worker', arn: 'arn:b' },
      { name: '/aws/ecs/app', arn: 'arn:c' },
    ]);
    expect(tree).toHaveLength(2);
    expect(tree[0].label).toBe('/aws/lambda');
    expect(tree[0].children).toHaveLength(2);
    expect(tree[0].children?.[0]).toEqual({
      key: 'arn:a',
      label: 'api-handler',
      value: 'arn:a',
    });
    expect(tree[1].label).toBe('/aws/ecs');
    expect(tree[1].children?.[0].label).toBe('app');
  });

  it('スラッシュを含まない名前は同名の親を持つ単一葉になる', () => {
    const tree = buildLogGroupTree([{ name: 'alb-access-logs', arn: 'arn:x' }]);
    expect(tree).toHaveLength(1);
    expect(tree[0].label).toBe('alb-access-logs');
    expect(tree[0].children).toHaveLength(1);
    expect(tree[0].children?.[0].value).toBe('arn:x');
  });

  it('親の順序・葉の順序は入力順を保つ', () => {
    const tree = buildLogGroupTree([
      { name: '/b/two', arn: '2' },
      { name: '/a/one', arn: '1' },
      { name: '/b/three', arn: '3' },
    ]);
    expect(tree.map((t) => t.label)).toEqual(['/b', '/a']);
    expect(tree[0].children?.map((c) => c.label)).toEqual(['two', 'three']);
  });

  it('空入力は空配列', () => {
    expect(buildLogGroupTree([])).toEqual([]);
  });
});
