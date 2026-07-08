import { ApiError } from '../types/common';

const BASE_URL = import.meta.env.VITE_API_BASE ?? 'http://127.0.0.1:8080';

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

export async function apiGet<T>(
  path: string,
  params?: Record<string, string | boolean | undefined>,
): Promise<T> {
  const url = new URL(path, BASE_URL);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined) continue;
      url.searchParams.set(k, typeof v === 'boolean' ? String(v) : v);
    }
  }

  let res: Response;
  try {
    res = await fetch(url.toString(), {
      headers: { Accept: 'application/json' },
    });
  } catch (e) {
    // fetch が投げるのは TypeError (ネットワーク到達不能等)
    const message = e instanceof Error ? e.message : 'network error';
    throw new ApiError(0, 'network_error', message);
  }

  if (!res.ok) {
    await throwApiError(res);
  }

  return (await res.json()) as T;
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const url = new URL(path, BASE_URL);

  let res: Response;
  try {
    res = await fetch(url.toString(), {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) {
    const message = e instanceof Error ? e.message : 'network error';
    throw new ApiError(0, 'network_error', message);
  }

  if (!res.ok) {
    await throwApiError(res);
  }

  // 202 Accepted (SSO login 起動等) はボディを持たない
  if (res.status === 202 || res.status === 204) {
    return undefined as T;
  }

  return (await res.json()) as T;
}

// multipart/form-data 用の POST ヘルパー (S3 アップロード等で使う)
// Content-Type は fetch が自動で boundary 付きで設定するため明示的に指定しない
export async function apiPostForm<T>(
  path: string,
  formData: FormData,
  params?: Record<string, string | boolean | undefined>,
): Promise<T> {
  const url = new URL(path, BASE_URL);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined) continue;
      url.searchParams.set(k, typeof v === 'boolean' ? String(v) : v);
    }
  }

  let res: Response;
  try {
    res = await fetch(url.toString(), {
      method: 'POST',
      headers: { Accept: 'application/json' },
      body: formData,
    });
  } catch (e) {
    const message = e instanceof Error ? e.message : 'network error';
    throw new ApiError(0, 'network_error', message);
  }

  if (!res.ok) {
    await throwApiError(res);
  }

  if (res.status === 202 || res.status === 204) {
    return undefined as T;
  }

  return (await res.json()) as T;
}

// BASE_URL を外部から参照したい (ダウンロード URL 組み立て等) 場合の getter
export function apiBaseUrl(): string {
  return BASE_URL;
}
