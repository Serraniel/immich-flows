import { flowSchema, readFlow, template } from '#lib/flows.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params, parent }) => {
	const { user } = await parent();
	if (!user?.isAdmin) return { source: params.source, yaml: '', schema: undefined };
	const [yaml, schema] = await Promise.all([
		params.source ? readFlow(params.source, fetch) : template,
		flowSchema(fetch)
	]);
	return { source: params.source, yaml, schema };
};
