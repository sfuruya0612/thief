import { describe, expect, it } from 'vitest';
import {
  fileExtension,
  isBinaryExtension,
  isPreviewEligible,
  previewDisabledReason,
} from './objectPreview';

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

describe('isBinaryExtension', () => {
  it('既知のバイナリ拡張子は true', () => {
    expect(isBinaryExtension('image.png')).toBe(true);
    expect(isBinaryExtension('doc.PDF')).toBe(true);
    expect(isBinaryExtension('archive.zip')).toBe(true);
  });

  it('テキスト系拡張子・拡張子なしは false', () => {
    expect(isBinaryExtension('app.log')).toBe(false);
    expect(isBinaryExtension('config.yaml')).toBe(false);
    expect(isBinaryExtension('README')).toBe(false);
    expect(isBinaryExtension('data.csv')).toBe(false);
  });
});

describe('isPreviewEligible', () => {
  it('バイナリ拡張子でなく 5 MB 未満なら true', () => {
    expect(isPreviewEligible('data.csv', 100)).toBe(true);
    expect(isPreviewEligible('app.log', 5 * 1024 * 1024 - 1)).toBe(true);
    expect(isPreviewEligible('Dockerfile', 100)).toBe(true);
  });

  it('5 MB ちょうどは false', () => {
    expect(isPreviewEligible('data.json', 5 * 1024 * 1024)).toBe(false);
  });

  it('バイナリ拡張子は false', () => {
    expect(isPreviewEligible('image.png', 100)).toBe(false);
  });
});

describe('previewDisabledReason', () => {
  it('バイナリ拡張子の理由を返す', () => {
    expect(previewDisabledReason('image.png', 100)).toBe('バイナリファイルはプレビューできません');
  });

  it('サイズ超過の理由を返す (バイナリ拡張子ではない)', () => {
    expect(previewDisabledReason('data.csv', 5 * 1024 * 1024)).toBe(
      '5 MB 以上のオブジェクトはプレビューできません',
    );
  });

  it('プレビュー可能なら空文字', () => {
    expect(previewDisabledReason('data.csv', 100)).toBe('');
    expect(previewDisabledReason('app.log', 100)).toBe('');
  });
});
