import { listFlows } from '#lib/flows.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends, parent }) => {
	depends('app:flows');
	const { user } = await parent();
	return { entries: user?.isAdmin ? await listFlows(fetch) : [] };
};
