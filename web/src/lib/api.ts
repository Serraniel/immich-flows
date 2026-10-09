import { BASE } from './base';

export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string
	) {
		super(message);
	}
}

export function apiUrl(path: string): string {
	return `${BASE}/api/${path.replace(/^\/+/, '')}`;
}

export async function api<T>(
	path: string,
	init?: RequestInit,
	f: typeof fetch = fetch
): Promise<T> {
	const res = await f(apiUrl(path), {
		...init,
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json', ...init?.headers }
	});
	if (!res.ok) {
		throw new ApiError(res.status, (await res.text()) || res.statusText);
	}
	return (res.status === 204 ? undefined : await res.json()) as T;
}
