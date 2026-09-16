// app.css のうち、jsdom がレイアウトと transform を計算しないために DOM からは検証できない
// 2 つの不変条件を、規則のテキストで固定する (issue 0174 の reopen で崩れていたもの)。
// 1. 下配置の Drawer の閉じ位置が、bottom を持ち上げるドックの高さ (--terminal-dock-h) の分も下がる
// 2. 常駐ターミナルドック (.terminal-dock) が .drawer と .drawer-backdrop より前面にある
// 表示状態そのものの検証は TerminalDock.test.tsx の冒頭コメントのとおり手動確認に委ねる。
import { describe, expect, it } from 'vitest';
// raw import が空文字列にならないよう、vite.config.ts の test.css.include で `.css?raw` を通している。
// Node の fs は使わない (`@types/node` が無く `tsc --noEmit` を通らない)。
import css from './app.css?raw';

// 規則ブロックの抽出。セレクタが行頭から始まり直後に " {" が続くブロックを 1 つだけ探し、
// 宣言部を返す。子孫セレクタや接頭辞を共有する別セレクタ (.drawer と .drawer-backdrop) と
// 誤一致しないよう、セレクタの直後の " {" まで含めて照合する。同じセレクタの規則が複数
// あると後の宣言が前の宣言を上書きするため、1 つだけであることも確認する。
// コメントはブロックの抽出より前に落とす。ブロックの終端を行頭の } で判定するため、
// ブロック内のコメントに行頭の } を含む行があると、そこを終端と誤認して以降の宣言を
// 取りこぼす。宣言の値は書かれたとおりに比較し、`!important` も取り除かない (付けば
// 落ちて、カスケードを変える変更としてレビューに上がる)。
// source を省略すると app.css を読む。合成した CSS を渡して抽出規則そのものも検証する。
function declarationsOf(selector: string, source: string = css): string {
  const withoutComments = source.replace(/\/\*[\s\S]*?\*\//g, '');
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const blocks = [
    ...withoutComments.matchAll(new RegExp(`^${escaped} \\{\\n([\\s\\S]*?)^\\}`, 'gm')),
  ];
  expect(blocks, `rule blocks for ${selector}`).toHaveLength(1);
  return blocks[0][1];
}

function declarationOf(selector: string, property: string, source: string = css): string {
  const match = new RegExp(`^\\s*${property}:\\s*([^;]+);`, 'm').exec(
    declarationsOf(selector, source),
  );
  expect(match, `declaration ${property} in ${selector}`).not.toBeNull();
  return (match as RegExpExecArray)[1].trim();
}

// 抽出規則の検証。app.css の現在の内容では踏まない経路 (コメント内の行頭の }、接頭辞を
// 共有するセレクタ、同じセレクタの重複) を合成した CSS で固定する。
describe('規則ブロックの抽出', () => {
  it('ブロック内のコメントに行頭の } があっても、その後の宣言まで抽出する', () => {
    const source = ['.a {', '  /* 例:', '}', '  */', '  z-index: 12;', '}', ''].join('\n');

    expect(declarationOf('.a', 'z-index', source)).toBe('12');
  });

  it('接頭辞を共有するセレクタや子孫セレクタのブロックを混同しない', () => {
    const source = [
      '.a {',
      '  z-index: 1;',
      '}',
      '.a-b {',
      '  z-index: 2;',
      '}',
      '.a .c {',
      '  z-index: 3;',
      '}',
      '',
    ].join('\n');

    expect(declarationOf('.a', 'z-index', source)).toBe('1');
    expect(declarationOf('.a-b', 'z-index', source)).toBe('2');
  });

  it('同じセレクタの規則ブロックが複数あれば失敗する', () => {
    const source = ['.a {', '  z-index: 1;', '}', '.a {', '  z-index: 2;', '}', ''].join('\n');

    expect(() => declarationsOf('.a', source)).toThrow();
  });
});

describe('app.css の Drawer とターミナルドックの重なり', () => {
  it('下配置の Drawer の閉じ位置は、bottom が参照するドックの高さの分も下げる', () => {
    expect(declarationOf('.drawer.pos-bottom', 'bottom')).toBe(
      'calc(var(--terminal-dock-h, 0px) + 8px)',
    );
    expect(declarationOf('.drawer.pos-bottom', 'transform')).toBe(
      'translateY(calc(100% + var(--terminal-dock-h, 0px) + 16px))',
    );
  });

  it('閉じた Drawer は pointer-events を受け取らない', () => {
    expect(declarationOf('.drawer:not(.open)', 'pointer-events')).toBe('none');
  });

  it('.terminal-dock は position を持ち、.drawer と .drawer-backdrop より大きい z-index を持つ', () => {
    const dock = Number(declarationOf('.terminal-dock', 'z-index'));
    const drawer = Number(declarationOf('.drawer', 'z-index'));
    const backdrop = Number(declarationOf('.drawer-backdrop', 'z-index'));

    expect(declarationOf('.terminal-dock', 'position')).toBe('relative');
    expect(dock).toBeGreaterThan(drawer);
    expect(dock).toBeGreaterThan(backdrop);
  });

  it('.terminal-dock は .app の flex 列で縮められない', () => {
    expect(declarationOf('.terminal-dock', 'flex')).toBe('none');
  });
});
