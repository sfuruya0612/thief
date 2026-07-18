// backend 起動待ちの間、画面全体を覆って接続待ちであることを示す。
// 疎通が回復すると useHealthCheck が success になり、呼び出し側で非表示になる。
export function ConnectionWaiting() {
  return (
    <div className="connection-waiting">
      <div className="connection-waiting-spinner" />
      <div className="connection-waiting-title">backend への接続を待っています</div>
      <div className="connection-waiting-hint">backend が起動すると自動で復帰します</div>
    </div>
  );
}
