import { api, ApiError } from './api';

export type User = { id: string; email: string; name: string; isAdmin: boolean };

/** Current Immich user, or null if not signed in to Immich. */
export async function currentUser(f: typeof fetch = fetch): Promise<User | null> {
	try {
		return await api<User>('me', undefined, f);
	} catch (e) {
		if (e instanceof ApiError && e.status === 401) return null;
		throw e;
	}
}
