// ログビューア左ペインのツリー構造。CloudWatch Logs のロググループ名 (スラッシュ区切り) を
// 2 階層のツリー (親: 末尾セグメントを除いた prefix、葉: 末尾セグメント) に組み立てる純関数。

export interface LogTreeNode {
  // 一意キー (親は prefix、葉は選択値 = ロググループ ARN)。
  key: string;
  // 表示名。
  label: string;
  // 葉の選択値。親ノードは undefined。
  value?: string;
  children?: LogTreeNode[];
}

// buildLogGroupTree はロググループ (name + arn) の配列を親でグルーピングしたツリーにする。
// 例: /aws/lambda/api-handler と /aws/lambda/batch-worker は親 "/aws/lambda" の 2 葉になる。
// スラッシュを含まない名前 (alb-access-logs 等) は同名の親を持つ単一葉として扱う。
// 親の並び順・各親配下の葉の並び順は入力の順序を保つ。
export function buildLogGroupTree(groups: { name: string; arn: string }[]): LogTreeNode[] {
  const parents = new Map<string, LogTreeNode>();
  const order: string[] = [];

  for (const g of groups) {
    const slash = g.name.lastIndexOf('/');
    const parentKey = slash > 0 ? g.name.slice(0, slash) : g.name;
    const leafLabel = slash > 0 ? g.name.slice(slash + 1) : g.name;

    let parent = parents.get(parentKey);
    if (!parent) {
      parent = { key: parentKey, label: parentKey, children: [] };
      parents.set(parentKey, parent);
      order.push(parentKey);
    }
    parent.children!.push({ key: g.arn, label: leafLabel, value: g.arn });
  }

  return order.map((k) => parents.get(k) as LogTreeNode);
}
