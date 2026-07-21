// i18next の初期化。UI 言語 (ja/en) は Tweaks.lang (useTweaks) から反映される。
// 翻訳リソースは機能領域ごとの名前空間として locales/{lang}/{namespace}.json に置き、
// ここで一括読み込みして i18next に登録する。
import i18n from 'i18next';
import type { InitOptions } from 'i18next';
import { initReactI18next } from 'react-i18next';
import type { Lang } from '../types/common';

const localeModules = import.meta.glob<{ default: Record<string, unknown> }>('./locales/*/*.json', {
  eager: true,
});

// path 例: "./locales/ja/pricing.json" -> lang="ja", ns="pricing"
const LOCALE_PATH_PATTERN = /\.\/locales\/([a-z]+)\/([a-zA-Z0-9_-]+)\.json$/;

function buildResources(): Record<string, Record<string, Record<string, unknown>>> {
  const resources: Record<string, Record<string, Record<string, unknown>>> = {};
  for (const [path, mod] of Object.entries(localeModules)) {
    const match = LOCALE_PATH_PATTERN.exec(path);
    if (!match) continue;
    const [, lang, ns] = match;
    resources[lang] ??= {};
    resources[lang][ns] = mod.default;
  }
  return resources;
}

const initOptions: InitOptions = {
  resources: buildResources(),
  lng: 'ja',
  fallbackLng: 'ja',
  // 全呼び出しが useTranslation('<namespace>') で名前空間を明示指定するため、
  // 既定名前空間 (defaultNS) は使わない。
  interpolation: {
    escapeValue: false, // React が既に XSS エスケープするため不要
  },
  returnNull: false,
  // 全リソースをバンドル時に同梱しており非同期のバックエンド読み込みが無いため、
  // init() を同期完了させて初回レンダーから翻訳を確定させる (Suspense 不要)。
  initAsync: false,
};

void i18n.use(initReactI18next).init(initOptions);

export function setI18nLanguage(lang: Lang): void {
  void i18n.changeLanguage(lang);
}

export default i18n;
