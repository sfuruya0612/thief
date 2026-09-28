// オブジェクトストレージ (S3 / GCS) の Drawer タブ共通実装。バケットをフォルダの階層として
// 1 階層ずつ辿れるオブジェクト一覧 + prefix 検索 + アップロード + ダウンロードリンクを
// まとめて描画する。ストレージ差分 (取得結果・アップロードフック・キー項目・列定義・
// ダウンロード URL) は props で注入する。
//
// 一覧は backend 側で最大 1000 件 (オブジェクトとフォルダの合計) に打ち切られるため、
// prefix 絞り込みはフロントエンドでのフィルタではなく検索ボタン押下でサーバへ再取得を
// 要求する。階層モード (delimiter=/) は今いるフォルダ直下だけを返し、フォルダ行のクリックと
// パンくずで移動する。フラットモード (delimiter なし) は今いるフォルダ以下の全階層を
// 平らに出す (issue 0208 の複数オブジェクト選択で使う)。
//
// Query ボタンはオブジェクトを DuckDB Wasm に取り込んで SQL を実行する (docs/issues/0196)。
// 押せる条件は対象形式・サイズ上限以下・設定の取得完了・ブラウザ対応の 4 つで、押せない行は
// title に理由を出す。押下すると一覧を置き換えて DrawerObjectQuery を表示する。
//
// 複数選択は DataTable のチェックボックス列を selection で制御化して行う (docs/issues/0208)。
// 選べるのは Query 可能なオブジェクト行だけで、選べる行が 1 つも無い一覧 (フォルダだけの
// 階層など) にはチェックボックスが出ない。選択は表示中の行との積で解釈し、件数は
// OBJECT_QUERY_MAX_FILES で打ち切る。
import { useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { UseMutationResult } from '@tanstack/react-query';
import type { ColumnDef } from '../tables/columns';
import { filterText } from '../tables/columns';
import { DataTable } from '../DataTable';
import { Icons } from '../icons/Icons';
import { DrawerObjectPreview } from './DrawerObjectPreview';
import { DrawerObjectQuery } from './DrawerObjectQuery';
import { Loading } from '../Loading';
import { ApiError } from '../../types/common';
import { isPreviewEligible, previewDisabledReason } from '../../lib/objectPreview';
import {
  clampObjectQuerySelection,
  isObjectQuerySupported,
  objectQueryDisabledReason,
  objectQuerySelectionDisabledReason,
  OBJECT_QUERY_MAX_FILES,
  type ObjectQueryConfigState,
  type ObjectQueryTarget,
} from '../../lib/objectQuery';
import { objectQueryEngine } from '../../lib/duckdb';
import { createOpfsIngestor } from '../../lib/opfsIngest';
import { useClientConfig } from '../../api/queries';
import { Button } from '../primitives';

// normalizeSearchPrefix は検索確定時に送る prefix を正規化する。先頭スラッシュのみを
// 取り除く (末尾は加工しない。"logs" でも "logs-2024/..." に前方一致させないためではなく、
// バックエンドの前方一致仕様どおり素通しする)。
function normalizeSearchPrefix(prefix: string): string {
  return prefix.trim().replace(/^\/+/, '');
}

// normalizeUploadPrefix はアップロード先フォルダを確定するための prefix を正規化する。
// 先頭・末尾のスラッシュを取り除き、空でなければ末尾にちょうど 1 つのスラッシュを付ける。
function normalizeUploadPrefix(prefix: string): string {
  const trimmed = prefix.trim().replace(/^\/+/, '').replace(/\/+$/, '');
  return trimmed ? `${trimmed}/` : '';
}

// ObjectUploadMutation は useS3Upload / useGcsUpload の戻り値の形。
export type ObjectUploadMutation = UseMutationResult<
  { status: string; key: string },
  Error,
  { key: string; file: File },
  unknown
>;

// ObjectPreviewQuery は useS3ObjectPreview / useGcsObjectPreview の戻り値のうち、
// DrawerObjectBrowser が参照するフィールドだけを表す (react-query の UseQueryResult は
// duck typing で満たされる)。
export interface ObjectPreviewQuery {
  data: { content: string; contentType: string } | undefined;
  isLoading: boolean;
  error: unknown;
}

// ObjectListQuery は useS3Objects / useGcsObjects の戻り値のうち、DrawerObjectBrowser が
// 参照するフィールドだけを表す。prefixes は階層モード (delimiter=/) のフォルダの完全な
// prefix で、backend が [] を保証するため null を許さない。truncated は backend の
// 1000 件上限 (オブジェクトとフォルダの合計) による打ち切りを示す。
export interface ObjectListQuery<TObject> {
  data: { objects: TObject[]; prefixes: string[]; truncated: boolean } | undefined;
  isLoading: boolean;
  error: unknown;
}

// ObjectBrowserMode は一覧の表示モード。hierarchy は delimiter=/ で今いるフォルダ直下だけを
// 出し、flat は delimiter なしで今いるフォルダ以下の全階層を平らに出す。
export type ObjectBrowserMode = 'hierarchy' | 'flat';

// ObjectFolderRow は API が返したフォルダ (S3 の CommonPrefixes / GCS の ObjectAttrs.Prefix)
// を表す行。id はオブジェクト行の id (S3 はキー、GCS は `${bucket}/${name}#${index}`) と
// 衝突しないよう "folder:" を付ける。prefix は末尾 "/" 付きの完全な prefix で、名前の列が
// S3 (key) と GCS (name) のどちらを読んでもソートできるよう name と key にも同じ値を入れる。
export interface ObjectFolderRow {
  kind: 'folder';
  id: string;
  state: string;
  prefix: string;
  name: string;
  key: string;
}

// ObjectRow はオブジェクト 1 件の行 (TRow に種別を足したもの)。
export type ObjectRow<TRow> = TRow & { kind: 'object' };

// ObjectBrowserRow は一覧に並ぶ行 (フォルダ行とオブジェクト行の和)。
export type ObjectBrowserRow<TRow> = ObjectRow<TRow> | ObjectFolderRow;

export interface DrawerObjectBrowserProps<TObject, TRow extends { id: string }> {
  // useObjects は取得に渡す prefix (今いるフォルダと検索確定済みの入力の連結) と
  // delimiter (階層モードは "/"、フラットモードは "") を受け取るカスタムフック。
  // どちらかが変わるとコンポーネント側の queryKey が変わり、サーバへ再取得される。
  useObjects: (prefix: string, delimiter: string) => ObjectListQuery<TObject>;
  // toTableRow は DataTable が要求する id/state を持つ行へ射影する。
  toTableRow: (obj: TObject) => TRow;
  // baseColumns の先頭は名前の列 (S3 は key、GCS は name) であること。
  // フォルダ行と、階層モードのオブジェクト行の相対名はこの列で描く。
  baseColumns: ColumnDef<TRow>[];
  downloadHref: (row: TRow) => string;
  // useUpload はアップロード先 prefix (内部 state 由来) を受け取るカスタムフック。
  // コンポーネント描画ごとに必ず 1 回呼ばれる。
  useUpload: (uploadPrefix: string | undefined) => ObjectUploadMutation;
  // previewKeyOf / sizeOf は行データからプレビュー可否判定 (拡張子・サイズ) に使う値を取り出す。
  // previewKeyOf はオブジェクトのキーそのものなので、SQL 検索の対象判定にも使う。
  previewKeyOf: (row: TRow) => string;
  sizeOf: (row: TRow) => number;
  // usePreview はプレビュー対象の key (未確定時は undefined) を受け取るカスタムフック。
  // コンポーネント描画ごとに必ず 1 回呼ばれる (react-query の enabled: !!key で遅延取得する)。
  usePreview: (key: string | undefined) => ObjectPreviewQuery;
  // profile は S3 の SSO 期限切れバナー (SSOExpiredBanner) に使う。GCS には SSO が無いため
  // DrawerS3Objects だけが渡す。
  profile?: string;
}

export function DrawerObjectBrowser<TObject, TRow extends { id: string }>({
  useObjects,
  toTableRow,
  baseColumns,
  downloadHref,
  useUpload,
  previewKeyOf,
  sizeOf,
  usePreview,
  profile,
}: DrawerObjectBrowserProps<TObject, TRow>) {
  const { t } = useTranslation('drawerStorage');
  // 今いるフォルダ (空か末尾 "/" 付き)、表示モード、検索欄の入力、検索ボタンで確定した入力。
  // 今いるフォルダとモードは永続化しない (region の変更で作り直す既存の挙動に合わせる)。
  const [currentPrefix, setCurrentPrefix] = useState('');
  const [mode, setMode] = useState<ObjectBrowserMode>('hierarchy');
  const [prefixInput, setPrefixInput] = useState('');
  const [committedInput, setCommittedInput] = useState('');
  const delimiter = mode === 'hierarchy' ? '/' : '';
  const { data, isLoading, error } = useObjects(`${currentPrefix}${committedInput}`, delimiter);
  // アップロード先は今いるフォルダと検索欄の入力の連結にする。今いるフォルダへの
  // アップロードと、検索欄に打った新しいフォルダ名へのアップロードの両方ができる。
  const uploadPrefix = `${currentPrefix}${normalizeUploadPrefix(prefixInput)}`;
  const upload = useUpload(uploadPrefix || undefined);
  // プレビュー編集の保存専用インスタンス。フィルタ入力由来の uploadPrefix を
  // 付けると保存先キーが変わってしまうため、prefix なしで呼び出す。
  const editUpload = useUpload(undefined);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [selected, setSelected] = useState<File | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [previewRow, setPreviewRow] = useState<ObjectRow<TRow> | null>(null);
  // queryFiles は SQL 検索に掛けるオブジェクト (行の Query は 1 件、選択の Query は選択分)。
  const [queryFiles, setQueryFiles] = useState<ObjectQueryTarget[] | null>(null);
  // selectedIds は一覧の複数選択。表示中の行の id との積で解釈する。
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const previewKey = previewRow ? previewKeyOf(previewRow) : undefined;
  const preview = usePreview(previewKey);
  // オブジェクト SQL 検索のサイズ上限。取得中と取得失敗では上限が分からないため Query を
  // 無効化する (既定値で仮に判定すると backend の設定と食い違う)。
  const clientConfig = useClientConfig();
  const configState = useMemo<ObjectQueryConfigState>(
    () =>
      clientConfig.isError
        ? { status: 'error' }
        : clientConfig.data
          ? { status: 'ready', maxBytes: clientConfig.data.objectQueryMaxBytes }
          : { status: 'loading' },
    [clientConfig.isError, clientConfig.data],
  );
  const maxBytes = clientConfig.data?.objectQueryMaxBytes;
  const browserSupported = isObjectQuerySupported();

  // nameKey は名前の列 (baseColumns の先頭) が読むフィールド。S3 は "key"、GCS は "name"。
  const nameKey = baseColumns[0]?.key ?? '';
  const nameOfObject = useCallback(
    (row: TRow): string => {
      const value = (row as Record<string, unknown>)[nameKey];
      return typeof value === 'string' ? value : '';
    },
    [nameKey],
  );
  // relativeName は今いるフォルダからの相対名にする (フォルダ行は末尾 "/" が残る)。
  const relativeName = useCallback(
    (full: string): string =>
      full.startsWith(currentPrefix) ? full.slice(currentPrefix.length) : full,
    [currentPrefix],
  );
  // 名前の列とその列フィルタが使う表示名。フォルダ行と階層モードのオブジェクト行は相対名、
  // フラットモードのオブジェクト行は完全なキー。
  const displayedName = useCallback(
    (row: ObjectBrowserRow<TRow>): string => {
      if (row.kind === 'folder') return relativeName(row.prefix);
      const full = nameOfObject(row);
      return mode === 'hierarchy' ? relativeName(full) : full;
    },
    [mode, nameOfObject, relativeName],
  );

  // 検索の確定とモードの切り替え、フォルダの移動では選択を空にする (表示中の行が変わり、
  // 選択の対象が一覧から消えるため)。
  const runSearch = () => {
    setCommittedInput(normalizeSearchPrefix(prefixInput));
    setSelectedIds(new Set());
  };

  // フォルダ行のクリックとパンくずからの移動。潜った先で前の入力による絞り込みが掛かった
  // ままにならないよう、検索欄と確定済みの入力も空にする。
  const navigateTo = useCallback((prefix: string) => {
    setCurrentPrefix(prefix);
    setPrefixInput('');
    setCommittedInput('');
    setSelectedIds(new Set());
  }, []);

  const changeMode = (next: ObjectBrowserMode) => {
    setMode(next);
    setSelectedIds(new Set());
  };

  const rows = useMemo<ObjectBrowserRow<TRow>[]>(() => {
    const folderRows: ObjectFolderRow[] = (data?.prefixes ?? []).map((prefix) => ({
      kind: 'folder',
      id: `folder:${prefix}`,
      state: '',
      prefix,
      name: prefix,
      key: prefix,
    }));
    const objectRows: ObjectRow<TRow>[] = (data?.objects ?? [])
      .map((obj) => ({ ...toTableRow(obj), kind: 'object' as const }))
      // 階層モードでは今いるフォルダ自身を表すプレースホルダ (キーが currentPrefix と
      // 等しいオブジェクト) を一覧から除く。相対名が空になり、行として意味が無い。
      .filter((row) => mode !== 'hierarchy' || nameOfObject(row) !== currentPrefix);
    // ソートしていない初期表示はフォルダ行を先頭に、オブジェクト行をその後ろに並べる。
    return [...folderRows, ...objectRows];
  }, [data, toTableRow, mode, currentPrefix, nameOfObject]);

  const objectCount = useMemo(() => rows.filter((row) => row.kind === 'object').length, [rows]);

  // isSelectable は行にチェックボックスを描くかどうか。Query 可能なオブジェクト行だけを
  // 選べる (フォルダ行と Query 不可の行は選べない)。設定の取得中と取得失敗、ブラウザ非対応
  // では全行が選べなくなる。
  const isSelectable = useCallback(
    (row: ObjectBrowserRow<TRow>): boolean =>
      row.kind === 'object' &&
      objectQueryDisabledReason(previewKeyOf(row), sizeOf(row), configState, browserSupported) ===
        '',
    [browserSupported, configState, previewKeyOf, sizeOf],
  );

  // 選択は表示中の行との積で解釈する。一覧の変化で消えた id や、選べなくなった行は
  // 選択から外れる。
  const selectedObjectRows = useMemo(
    () =>
      rows.filter(
        (row): row is ObjectRow<TRow> =>
          row.kind === 'object' && isSelectable(row) && selectedIds.has(row.id),
      ),
    [isSelectable, rows, selectedIds],
  );
  const selectedVisibleIds = useMemo(
    () => new Set(selectedObjectRows.map((row) => row.id)),
    [selectedObjectRows],
  );
  const selectedTargets = useMemo(
    () =>
      selectedObjectRows.map((row) => ({
        key: previewKeyOf(row),
        url: downloadHref(row),
        size: sizeOf(row),
      })),
    [downloadHref, previewKeyOf, selectedObjectRows, sizeOf],
  );
  const selectionDisabledReason = objectQuerySelectionDisabledReason(
    selectedTargets,
    configState,
    browserSupported,
  );
  const selectionCountLabel =
    selectedTargets.length >= OBJECT_QUERY_MAX_FILES
      ? t('drawerObjectBrowser.selectionClamped', {
          count: selectedTargets.length,
          max: OBJECT_QUERY_MAX_FILES,
        })
      : t('drawerObjectBrowser.selectedCount', { count: selectedTargets.length });

  // 選択の変更は毎回上限で打ち切る。全選択では DataTable が渡す表示順、1 行のチェックでは
  // 既存の選択の後ろに新しい id が足された順になり、挿入順の先頭 OBJECT_QUERY_MAX_FILES 件が残る。
  const onSelectionChange = useCallback((next: Set<string>) => {
    setSelectedIds(clampObjectQuerySelection(next, OBJECT_QUERY_MAX_FILES));
  }, []);

  // パンくず (ルートと各階層)。クリックするとその階層へ戻る。
  const breadcrumb = useMemo(() => {
    const segments = currentPrefix.split('/').filter(Boolean);
    let accumulated = '';
    return segments.map((name) => {
      accumulated += `${name}/`;
      return { name, prefix: accumulated };
    });
  }, [currentPrefix]);

  // 列は baseColumns を ObjectBrowserRow の列に包んで作る。名前の列 (先頭) はフォルダ行と
  // 相対名の描画を担い、名前以外の列はフォルダ行では何も描かない。
  const columns = useMemo<ColumnDef<ObjectBrowserRow<TRow>>[]>(() => {
    const [nameColumn, ...otherColumns] = baseColumns;
    const wrappedName: ColumnDef<ObjectBrowserRow<TRow>> = {
      ...nameColumn,
      cell: (r) =>
        r.kind === 'folder' ? (
          <button
            type="button"
            className="object-folder-link"
            title={t('drawerObjectBrowser.openFolder', { name: r.prefix })}
            onClick={() => navigateTo(r.prefix)}
          >
            <Icons.folder size={13} />
            <span className="truncate">{relativeName(r.prefix)}</span>
          </button>
        ) : (
          <span className="primary truncate">{displayedName(r)}</span>
        ),
      // 列フィルタは表示している名前で絞る (フォルダ行は相対名、オブジェクト行は階層
      // モードなら相対名、フラットモードなら完全なキー)。
      filterValue: displayedName,
    };
    const wrappedOthers = otherColumns.map<ColumnDef<ObjectBrowserRow<TRow>>>((c) => ({
      ...c,
      cell: (r) => (r.kind === 'folder' ? null : c.cell(r)),
      // フォルダ行は名前以外の列のフィルタに一致しない。今の列はフォルダ行に値を持たないため
      // 元の判定でも一致しないが、filterValue を持つ列が足されても一致しないよう固定する。
      // オブジェクト行は元の列の判定をそのまま使う (元の filterValue があればその値、無ければ
      // row[key] の文字列化)。
      filterValue: (r) => (r.kind === 'folder' ? '' : filterText(c, r)),
    }));
    // Preview / Query / Download の Actions 列はフォルダ行では何も描かない。
    const actionsColumn: ColumnDef<ObjectBrowserRow<TRow>> = {
      key: 'actions',
      header: '',
      width: '24%',
      cell: (r) => {
        if (r.kind === 'folder') return null;
        const key = previewKeyOf(r);
        const size = sizeOf(r);
        const eligible = isPreviewEligible(key, size);
        const reason = previewDisabledReason(key, size);
        const queryReason = objectQueryDisabledReason(key, size, configState, browserSupported);
        return (
          <span style={{ display: 'flex', gap: 6 }}>
            <Button
              size="sm"
              disabled={!eligible}
              title={reason || t('drawerObjectBrowser.openPreview')}
              onClick={() => setPreviewRow(r)}
            >
              Preview
            </Button>
            <Button
              size="sm"
              disabled={queryReason !== ''}
              title={queryReason || t('drawerObjectBrowser.openQuery')}
              onClick={() => setQueryFiles([{ key, url: downloadHref(r), size }])}
            >
              Query
            </Button>
            <a href={downloadHref(r)} download className="btn sm" style={{ padding: '2px 8px' }}>
              Download
            </a>
          </span>
        );
      },
    };
    return [wrappedName, ...wrappedOthers, actionsColumn];
  }, [
    baseColumns,
    browserSupported,
    configState,
    displayedName,
    downloadHref,
    navigateTo,
    previewKeyOf,
    relativeName,
    sizeOf,
    t,
  ]);

  const onUpload = () => {
    if (!selected) return;
    upload.mutate(
      { key: selected.name, file: selected },
      {
        onSuccess: () => {
          setSelected(null);
          if (fileInputRef.current) fileInputRef.current.value = '';
        },
      },
    );
  };

  const pickFile = (file: File | null) => {
    if (upload.isPending) return;
    setSelected(file);
  };

  if (previewRow) {
    const previewKeyForSave = previewKeyOf(previewRow);
    return (
      <DrawerObjectPreview
        fileName={previewKeyForSave}
        content={preview.data?.content}
        isLoading={preview.isLoading}
        error={preview.error}
        onClose={() => setPreviewRow(null)}
        onSave={async (newContent) => {
          const contentType = preview.data?.contentType || 'text/plain';
          const file = new File([newContent], previewKeyForSave.split('/').pop() || 'file', {
            type: contentType,
          });
          await editUpload.mutateAsync({ key: previewKeyForSave, file });
        }}
      />
    );
  }

  if (queryFiles !== null && maxBytes !== undefined) {
    return (
      <DrawerObjectQuery
        files={queryFiles}
        maxBytes={maxBytes}
        profile={profile}
        engine={objectQueryEngine}
        createIngestor={createOpfsIngestor}
        onClose={() => setQueryFiles(null)}
      />
    );
  }

  return (
    <div className="section">
      {/* 件数はオブジェクトのみで、フォルダは含めない */}
      <h3>Objects ({objectCount})</h3>

      <div className="object-breadcrumb">
        <button
          type="button"
          className={currentPrefix === '' ? 'active' : ''}
          onClick={() => navigateTo('')}
        >
          {t('drawerObjectBrowser.breadcrumbRoot')}
        </button>
        {breadcrumb.map((segment) => (
          <span key={segment.prefix} className="object-breadcrumb-segment">
            <span className="sep">/</span>
            <button
              type="button"
              className={segment.prefix === currentPrefix ? 'active' : ''}
              onClick={() => navigateTo(segment.prefix)}
            >
              {segment.name}
            </button>
          </span>
        ))}
      </div>

      <div className="s3-search-row">
        <span className="chip-search s3-prefix-input">
          <input
            placeholder="prefix (folder/subfolder)…"
            value={prefixInput}
            onChange={(e) => setPrefixInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') runSearch();
            }}
          />
          <Button size="sm" onClick={runSearch}>
            {t('drawerObjectBrowser.search')}
          </Button>
        </span>
        {/* トグルは今いるフォルダと検索欄の入力を変えない (選択は表示中の行との積なので空にする) */}
        <div className="seg">
          {(['hierarchy', 'flat'] as const).map((m) => (
            <button key={m} className={mode === m ? 'active' : ''} onClick={() => changeMode(m)}>
              {t(
                m === 'hierarchy'
                  ? 'drawerObjectBrowser.modeHierarchy'
                  : 'drawerObjectBrowser.modeFlat',
              )}
            </button>
          ))}
        </div>
      </div>

      <div className="s3-selection-row">
        <span className="muted">{selectionCountLabel}</span>
        <Button
          size="sm"
          disabled={selectionDisabledReason !== ''}
          title={selectionDisabledReason || t('drawerObjectBrowser.openQuerySelected')}
          onClick={() => setQueryFiles(selectedTargets)}
        >
          {t('drawerObjectBrowser.querySelected', { count: selectedTargets.length })}
        </Button>
      </div>

      {data?.truncated && (
        <div className="s3-truncated-notice">
          {t(
            mode === 'hierarchy'
              ? 'drawerObjectBrowser.truncatedNoticeHierarchy'
              : 'drawerObjectBrowser.truncatedNotice',
          )}
        </div>
      )}

      <div className="s3-upload">
        <label
          className={`s3-upload-dropzone ${dragOver ? 'drag-over' : ''}`}
          onDragOver={(e) => {
            e.preventDefault();
            if (!upload.isPending) setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragOver(false);
            pickFile(e.dataTransfer.files?.[0] ?? null);
          }}
        >
          <input
            ref={fileInputRef}
            type="file"
            className="s3-upload-input"
            onChange={(e) => pickFile(e.target.files?.[0] ?? null)}
            disabled={upload.isPending}
          />
          <span className="s3-upload-text">
            {selected ? selected.name : t('drawerObjectBrowser.selectOrDropFile')}
          </span>
        </label>
        <Button
          size="sm"
          variant="primary"
          onClick={onUpload}
          disabled={!selected || upload.isPending}
        >
          {upload.isPending ? 'Uploading…' : 'Upload'}
        </Button>
        {upload.error && (
          <span style={{ color: 'var(--err)' }}>
            {upload.error instanceof ApiError ? upload.error.message : String(upload.error)}
          </span>
        )}
      </div>

      {isLoading ? (
        <Loading />
      ) : error ? (
        <div style={{ padding: 20, color: 'var(--err)' }}>
          {error instanceof ApiError ? error.message : String(error)}
        </div>
      ) : (
        <DataTable
          rows={rows}
          columns={columns}
          onSelect={() => {}}
          selectedId={null}
          selection={{
            selected: selectedVisibleIds,
            onChange: onSelectionChange,
            isSelectable,
          }}
          rowClassName={(r) =>
            r.kind === 'folder' || isPreviewEligible(previewKeyOf(r), sizeOf(r))
              ? undefined
              : 'preview-ineligible'
          }
        />
      )}
    </div>
  );
}
