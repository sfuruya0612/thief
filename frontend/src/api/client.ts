import { ApiError } from '../types/common';

// VITE_API_BASE が未設定ならローカル開発時の backend デフォルトを使う。
const rawApiBase = import.meta.env.VITE_API_BASE;
const BASE_URL = rawApiBase === undefined ? 'http://127.0.0.1:8080' : rawApiBase;

// 非 2xx レスポンスから標準エラー DTO を読み取り ApiError を構築する
async function throwApiError(res: Response): Promise<never> {
  let code: string | undefined;
  let message: string = res.statusText;
  let details: unknown;
  try {
    const body = (await res.json()) as {
      error?: string;
      code?: string;
      message?: string;
      details?: unknown;
    };
    if (typeof body.code === 'string') code = body.code;
    if (typeof body.error === 'string') message = body.error;
    else if (typeof body.message === 'string') message = body.message;
    details = body.details;
  } catch {
    // JSON でなければ statusText を使う
  }
  throw new ApiError(res.status, code, message, details);
}

type QueryParams = Record<string, string | boolean | undefined>;

// パスとクエリパラメータから URL を組み立てる (undefined のパラメータは付与しない)
function buildUrl(path: string, params?: QueryParams): URL {
  const url = new URL(path, BASE_URL);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined) continue;
      url.searchParams.set(k, typeof v === 'boolean' ? String(v) : v);
    }
  }
  return url;
}

// fetch を実行し、ネットワーク到達不能を ApiError(0, 'network_error') に正規化、
// 非 2xx を標準エラー DTO 由来の ApiError に変換して返す
async function doFetch(url: URL, init: RequestInit): Promise<Response> {
  let res: Response;
  try {
    res = await fetch(url.toString(), init);
  } catch (e) {
    // fetch が投げるのは TypeError (ネットワーク到達不能等)
    const message = e instanceof Error ? e.message : 'network error';
    throw new ApiError(0, 'network_error', message);
  }

  if (!res.ok) {
    await throwApiError(res);
  }

  return res;
}

// POST 系共通のレスポンス解釈。202 Accepted / 204 No Content (SSO login 完了等) は
// ボディを持たないため undefined を返す。GET には適用しない (apiGet の挙動を変えない)
async function parsePostResponse<T>(res: Response): Promise<T> {
  if (res.status === 202 || res.status === 204) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

export async function apiGet<T>(path: string, params?: QueryParams): Promise<T> {
  const res = await doFetch(buildUrl(path, params), {
    headers: { Accept: 'application/json' },
  });
  return (await res.json()) as T;
}

// リスト系 GET 用ヘルパー。バックエンドは要素ゼロ時に null を返しうるため空配列に正規化する
export async function apiGetList<T>(path: string, params?: QueryParams): Promise<T[]> {
  return (await apiGet<T[] | null>(path, params)) ?? [];
}

export async function apiPost<T>(path: string, body?: unknown, params?: QueryParams): Promise<T> {
  const res = await doFetch(buildUrl(path, params), {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  return parsePostResponse<T>(res);
}

// DELETE 用ヘルパー (クエリキャンセル等)。204 No Content を想定しボディは解釈しない
export async function apiDelete(path: string, params?: QueryParams): Promise<void> {
  await doFetch(buildUrl(path, params), {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
  });
}

// multipart/form-data 用の POST ヘルパー (S3 アップロード等で使う)
// Content-Type は fetch が自動で boundary 付きで設定するため明示的に指定しない
export async function apiPostForm<T>(
  path: string,
  formData: FormData,
  params?: QueryParams,
): Promise<T> {
  const res = await doFetch(buildUrl(path, params), {
    method: 'POST',
    headers: { Accept: 'application/json' },
    body: formData,
  });
  return parsePostResponse<T>(res);
}

// BASE_URL を外部から参照したい (ダウンロード URL 組み立て等) 場合の getter
export function apiBaseUrl(): string {
  return BASE_URL;
}
