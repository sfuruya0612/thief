// ECharts のオプションに渡すテーマ依存の色。ECharts は CSS 変数を解釈しないため、
// styles/tokens.css の --text-2 相当を直値で持つ。軸と凡例の文字色は複数のチャートで共通なので、
// それぞれのファイルに重複定義すると値の変更漏れが起きる。ここに 1 つだけ置く。

// THEME_TEXT_COLOR は軸・凡例の文字色。
export const THEME_TEXT_COLOR: Record<'dark' | 'light', string> = {
  dark: '#a8a8b0',
  light: '#5c5c66',
};
