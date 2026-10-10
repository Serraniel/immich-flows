import { getRun } from '#lib/runs.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => ({
	run: await getRun(params.id, fetch)
});
