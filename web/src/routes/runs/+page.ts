import { listRuns } from '#lib/runs.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const flow = url.searchParams.get('flow') ?? '';
	const status = url.searchParams.get('status') ?? '';
	return { flow, status, runs: await listRuns({ flow, status }, fetch) };
};
