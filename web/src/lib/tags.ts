import { api, ApiError } from './api';

// Immich's TagResponseDto (subset).
export type Tag = { id: string; name: string; value: string; parentId?: string; color?: string };

export type Move = { from: string; to: string };
export type Bulk = { match: string; replace: string };

export type Change = { from: string; to: string; exists: boolean; assets: number };
export type Plan = { move: Move; changes: Change[] | null; assets: number; settled?: boolean };
export type BulkPlan = { moves: Plan[] | null };

export type TagNode = { name: string; value: string; tag?: Tag; children: TagNode[] };

// listTags reads the caller's tags from Immich itself (same origin, session cookie).
export async function listTags(f: typeof fetch = fetch): Promise<Tag[]> {
	const res = await f('/api/tags', { credentials: 'same-origin' });
	if (!res.ok) throw new ApiError(res.status, (await res.text()) || res.statusText);
	return res.json();
}

export const moveTag = (m: Move, dryRun: boolean) =>
	api<Plan>('tags/move', { method: 'POST', body: JSON.stringify({ ...m, dryRun }) });
export const bulkTags = (b: Bulk, dryRun: boolean) =>
	api<BulkPlan>('tags/bulk', { method: 'POST', body: JSON.stringify({ ...b, dryRun }) });

// tree builds the tag hierarchy from full values, sorted by name.
// Missing ancestors become nodes without a tag.
export function tree(tags: Tag[]): TagNode[] {
	const root: TagNode = { name: '', value: '', children: [] };
	const byValue = new Map<string, TagNode>([['', root]]);
	const node = (value: string): TagNode => {
		let n = byValue.get(value);
		if (n) return n;
		const i = value.lastIndexOf('/');
		n = { name: value.slice(i + 1), value, children: [] };
		byValue.set(value, n);
		node(i < 0 ? '' : value.slice(0, i)).children.push(n);
		return n;
	};
	for (const t of tags) node(t.value).tag = t;
	const sort = (n: TagNode) => {
		n.children.sort((a, b) => a.name.localeCompare(b.name));
		n.children.forEach(sort);
	};
	sort(root);
	return root.children;
}

// filterTree keeps nodes whose value contains q, with their ancestors and descendants.
export function filterTree(nodes: TagNode[], q: string): TagNode[] {
	q = q.trim().toLowerCase();
	if (!q) return nodes;
	return nodes.flatMap((n) => {
		if (n.value.toLowerCase().includes(q)) return [n];
		const children = filterTree(n.children, q);
		return children.length ? [{ ...n, children }] : [];
	});
}

export function kind(c: Change): 'merge' | 'move' | 'rename' {
	if (c.exists) return 'merge';
	const parent = (v: string) => v.slice(0, Math.max(v.lastIndexOf('/'), 0));
	return parent(c.from) === parent(c.to) ? 'rename' : 'move';
}
