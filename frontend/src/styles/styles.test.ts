// styles/ の CSS のうち、jsdom がレイアウトと transform を計算しないために DOM からは検証できない
// 不変条件を、規則のテキストで固定する。対象は shell.css (ペイン)、features/drawer.css (Drawer)、
// features/terminal.css (ターミナルドック) の 3 ファイルで、読み込み順 (index.css) に結合して検証する。
// 1. 下配置の Drawer の閉じ位置が、bottom を持ち上げるドックの高さ (--terminal-dock-h) の分も下がる
//    (issue 0174 の reopen で崩れていたもの)
// 2. 常駐ターミナルドック (.terminal-dock) が .drawer と .drawer-backdrop より前面にある
//    (issue 0174 の reopen で崩れていたもの)
// 3. 分割していないときのペインのラッパー (.pane.single) がレイアウトに箱を作らない
//    (display: contents。issue 0175)
// 表示状態そのものの検証は TerminalDock.test.tsx の冒頭コメントのとおり手動確認に委ねる。
import { describe, expect, it } from 'vitest';
// raw import が空文字列にならないよう、vite.config.ts の test.css.include で `.css?raw` を通している。
// Node の fs は使わない (`@types/node` が無く `tsc --noEmit` を通らない)。
import shell from './shell.css?raw';
import drawer from './features/drawer.css?raw';
import terminal from './features/terminal.css?raw';

const css = [shell, drawer, terminal].join('\n');

// 規則ブロックの抽出。セレクタが行頭から始まり直後に " {" が続くブロックを 1 つだけ探し、
// 宣言部を返す。子孫セレクタや接頭辞を共有する別セレクタ (.drawer と .drawer-backdrop) と
// 誤一致しないよう、セレクタの直後の " {" まで含めて照合する。同じセレクタの規則が複数
// あると後の宣言が前の宣言を上書きするため、1 つだけであることも確認する。
// コメントはブロックの抽出より前に落とす。ブロックの終端を行頭の } で判定するため、
// ブロック内のコメントに行頭の } を含む行があると、そこを終端と誤認して以降の宣言を
// 取りこぼす。宣言の値は書かれたとおりに比較し、`!important` も取り除かない (付けば
// 落ちて、カスケードを変える変更としてレビューに上がる)。
// source を省略すると上の 3 ファイルを読む。合成した CSS を渡して抽出規則そのものも検証する。
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

// 抽出規則の検証。現在の CSS では踏まない経路 (コメント内の行頭の }、接頭辞を
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

describe('features/drawer.css と features/terminal.css の重なり', () => {
  // issue 0204: ドックの高さ (--terminal-dock-h) の参照は .drawer の --drawer-lift の 1 か所だけにし、
  // bottom と下配置の閉じ位置の両方をそこから計算する (分割前の 100% + dock + 16px と等価)。
  it('下配置の Drawer の閉じ位置は、bottom が参照するドックの高さの分も下げる', () => {
    expect(declarationOf('.drawer', '--drawer-lift')).toBe(
      'calc(var(--terminal-dock-h, 0px) + 8px)',
    );
    expect(declarationOf('.drawer', 'bottom')).toBe('var(--drawer-lift)');
    expect(declarationOf('.drawer.pos-bottom', 'transform')).toBe(
      'translateY(calc(100% + var(--drawer-lift) + 8px))',
    );
    // コメント (等価性の説明) を除いた規則のテキストで数える
    expect(drawer.replace(/\/\*[\s\S]*?\*\//g, '').match(/var\(--terminal-dock-h/g)).toHaveLength(
      1,
    );
  });

  it('分割中の内包 (contained) はドックより上の領域に収まるため、持ち上げは 8px だけになる', () => {
    expect(declarationOf('.drawer.contained', '--drawer-lift')).toBe('8px');
    expect(declarationsOf('.drawer.contained')).not.toMatch(/^\s*bottom:/m);
    expect(declarationsOf('.drawer.contained.pos-bottom')).not.toMatch(/^\s*transform:/m);
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

describe('features/terminal.css のターミナルドックの高さ変更', () => {
  it('.terminal-dock-resizer はドックの上端 6px に重なり、縦方向のカーソルを出す', () => {
    expect(declarationOf('.terminal-dock-resizer', 'position')).toBe('absolute');
    expect(declarationOf('.terminal-dock-resizer', 'top')).toBe('0');
    expect(declarationOf('.terminal-dock-resizer', 'height')).toBe('6px');
    expect(declarationOf('.terminal-dock-resizer', 'z-index')).toBe('1');
    expect(declarationOf('.terminal-dock-resizer', 'cursor')).toBe('ns-resize');
  });

  it('.terminal-panel は min-height を持たない (本文の下限は TypeScript 側のクランプが持つ)', () => {
    expect(declarationsOf('.terminal-panel')).not.toMatch(/^\s*min-height:/m);
    // 他の宣言は変わらない
    expect(declarationOf('.terminal-panel', 'position')).toBe('relative');
    expect(declarationOf('.terminal-panel', 'display')).toBe('flex');
    expect(declarationOf('.terminal-panel', 'flex-direction')).toBe('column');
    expect(declarationOf('.terminal-panel', 'height')).toBe('100%');
  });
});

// issue 0175: 分割していないときのペインのラッパーはレイアウトに影響させない。
// jsdom は display: contents を解釈しないため、規則のテキストで固定する。
describe('shell.css の分割表示', () => {
  it('.pane.single は display: contents で .body のグリッドに影響させない', () => {
    expect(declarationOf('.pane.single', 'display')).toBe('contents');
  });
});
