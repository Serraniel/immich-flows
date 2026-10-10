import { listTags } from '#lib/tags.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, depends }) => {
	depends('app:tags');
	return { tags: await listTags(fetch) };
};
