import { describe, expect, it, vi } from 'vitest';
import { currentUser } from './user';

describe('currentUser', () => {
	it('returns null when not signed in', async () => {
		const f = vi.fn().mockResolvedValue(new Response('unauthenticated', { status: 401 }));
		await expect(currentUser(f)).resolves.toBeNull();
	});

	it('rethrows other errors', async () => {
		const f = vi.fn().mockResolvedValue(new Response('down', { status: 502 }));
		await expect(currentUser(f)).rejects.toThrow('down');
	});
});
