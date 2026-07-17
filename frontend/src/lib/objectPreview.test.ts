import { describe, expect, it } from 'vitest';
import { fileExtension, isPreviewEligible, previewDisabledReason } from './objectPreview';

describe('fileExtension', () => {
  it('末尾の拡張子を小文字で取り出す', () => {
    expect(fileExtension('path/to/file.CSV')).toBe('.csv');
    expect(fileExtension('notes.txt')).toBe('.txt');
  });

  it('多重拡張子は最後の拡張子だけを見る', () => {
    expect(fileExtension('archive.json.gz')).toBe('.gz');
  });

  it('拡張子が無い場合は空文字', () => {
    expect(fileExtension('README')).toBe('');
  });

  it('ディレクトリ名に "." を含んでも拡張子扱いしない', () => {
    expect(fileExtension('my.folder/readme')).toBe('');
  });

  it('ディレクトリ名の "." より後ろにファイル名の拡張子がある場合は正しく取り出す', () => {
    expect(fileExtension('my.folder/data.json')).toBe('.json');
  });
});

describe('isPreviewEligible', () => {
  it('csv / txt / json かつ 5 MB 未満なら true', () => {
    expect(isPreviewEligible('data.csv', 100)).toBe(true);
    expect(isPreviewEligible('data.txt', 5 * 1024 * 1024 - 1)).toBe(true);
  });

  it('5 MB ちょうどは false', () => {
    expect(isPreviewEligible('data.json', 5 * 1024 * 1024)).toBe(false);
  });

  it('対象外拡張子は false', () => {
    expect(isPreviewEligible('image.png', 100)).toBe(false);
  });
});

describe('previewDisabledReason', () => {
  it('対象外拡張子の理由を返す', () => {
    expect(previewDisabledReason('image.png', 100)).toBe('csv / txt / json のみプレビューできます');
  });

  it('サイズ超過の理由を返す (拡張子は対象内)', () => {
    expect(previewDisabledReason('data.csv', 5 * 1024 * 1024)).toBe(
      '5 MB 以上のオブジェクトはプレビューできません',
    );
  });

  it('プレビュー可能なら空文字', () => {
    expect(previewDisabledReason('data.csv', 100)).toBe('');
  });
});
