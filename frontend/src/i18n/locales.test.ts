// ja / en のロケールリソースのキー集合が一致することを検査する。
// i18n は fallbackLng: 'ja' で初期化されているため、en にキーが無くても
// 日本語の文言が表示され、欠落に気付きにくい (TerminalDock のリサイズハンドルの
// title が英語 UI で日本語のままになった不具合と同じ形)。片方のロケールにだけ
// キーを追加したときにここで検出する。ロケールの読み込みは index.ts と同じ
// import.meta.glob の形にする。
import { describe, expect, it } from 'vitest';

const localeModules = import.meta.glob<{ default: Record<string, unknown> }>('./locales/*/*.json', {
  eager: true,
});

// path 例: "./locales/ja/pricing.json" -> lang="ja", ns="pricing"
const LOCALE_PATH_PATTERN = /\.\/locales\/([a-z]+)\/([a-zA-Z0-9_-]+)\.json$/;

type NamespaceKeyPaths = Record<string, string[]>;

// ネストしたオブジェクトのキーを "terminal.resizeDock" の形に平坦化する。
// 値が配列の場合は i18next のキーを持たないため葉として扱う。
function flattenKeyPaths(value: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(value).flatMap(([key, child]) => {
    const path = prefix === '' ? key : `${prefix}.${key}`;
    if (child !== null && typeof child === 'object' && !Array.isArray(child)) {
      return flattenKeyPaths(child as Record<string, unknown>, path);
    }
    return [path];
  });
}

// { ja: { pricing: ['title', ...] }, en: { ... } } の形に集める。
function collectKeyPathsByLang(): Record<string, NamespaceKeyPaths> {
  const result: Record<string, NamespaceKeyPaths> = {};
  for (const [path, mod] of Object.entries(localeModules)) {
    const match = LOCALE_PATH_PATTERN.exec(path);
    if (!match) continue;
    const [, lang, ns] = match;
    result[lang] ??= {};
    result[lang][ns] = flattenKeyPaths(mod.default);
  }
  return result;
}

const keyPathsByLang = collectKeyPathsByLang();

describe('locales', () => {
  it('ja と en の両方のロケールが読み込まれている', () => {
    expect(keyPathsByLang.ja, 'locales/ja/*.json').toBeDefined();
    expect(keyPathsByLang.en, 'locales/en/*.json').toBeDefined();
  });

  it('ja と en のネームスペース集合が一致する', () => {
    const jaNamespaces = Object.keys(keyPathsByLang.ja ?? {});
    const enNamespaces = Object.keys(keyPathsByLang.en ?? {});
    const missing: string[] = [];
    for (const ns of jaNamespaces) {
      if (!enNamespaces.includes(ns)) missing.push(`locales/en/${ns}.json does not exist`);
    }
    for (const ns of enNamespaces) {
      if (!jaNamespaces.includes(ns)) missing.push(`locales/ja/${ns}.json does not exist`);
    }

    expect(missing).toEqual([]);
  });

  it('ja と en の全ネームスペースでキー集合が双方向に一致する', () => {
    const namespaces = [
      ...new Set([
        ...Object.keys(keyPathsByLang.ja ?? {}),
        ...Object.keys(keyPathsByLang.en ?? {}),
      ]),
    ].sort();
    const missing: string[] = [];
    for (const ns of namespaces) {
      const jaKeys = new Set(keyPathsByLang.ja?.[ns] ?? []);
      const enKeys = new Set(keyPathsByLang.en?.[ns] ?? []);
      for (const key of [...jaKeys].sort()) {
        if (!enKeys.has(key)) missing.push(`locales/en/${ns}.json is missing "${key}"`);
      }
      for (const key of [...enKeys].sort()) {
        if (!jaKeys.has(key)) missing.push(`locales/ja/${ns}.json is missing "${key}"`);
      }
    }

    expect(missing).toEqual([]);
  });
});
