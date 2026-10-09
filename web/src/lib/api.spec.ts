import { describe, expect, it, vi } from 'vitest';
import { api, apiUrl, ApiError } from './api';

describe('api', () => {
	it('builds URLs under the base path', () => {
		expect(apiUrl('/flows')).toBe('/_flows/api/flows');
		expect(apiUrl('flows/x')).toBe('/_flows/api/flows/x');
	});

	it('returns JSON', async () => {
		const f = vi.fn().mockResolvedValue(new Response('{"a":1}', { status: 200 }));
		await expect(api('x', undefined, f)).resolves.toEqual({ a: 1 });
		expect(f).toHaveBeenCalledWith(
			'/_flows/api/x',
			expect.objectContaining({ credentials: 'same-origin' })
		);
	});

	it('throws ApiError on failure', async () => {
		const f = vi.fn().mockResolvedValue(new Response('nope', { status: 403 }));
		await expect(api('x', undefined, f)).rejects.toEqual(new ApiError(403, 'nope'));
	});
});
