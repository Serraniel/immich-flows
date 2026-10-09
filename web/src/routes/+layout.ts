import { currentUser } from '#lib/user.ts';
import type { LayoutLoad } from './$types';

// SPA: the Go binary serves static files with 200.html as fallback.
export const ssr = false;
export const prerender = false;

export const load: LayoutLoad = async ({ fetch }) => ({ user: await currentUser(fetch) });
