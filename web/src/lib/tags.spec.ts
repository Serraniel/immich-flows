import { describe, expect, it } from 'vitest';
import { filterTree, kind, tree, type Tag } from './tags';

const tag = (value: string): Tag => ({ id: value, name: value.split('/').at(-1)!, value });

describe('tags', () => {
	it('builds a sorted tree with missing ancestors', () => {
		const t = tree([tag('b/y'), tag('a'), tag('b/x/1')]);
		expect(t.map((n) => n.value)).toEqual(['a', 'b']);
		expect(t[1].tag).toBeUndefined();
		expect(t[1].children.map((n) => n.name)).toEqual(['x', 'y']);
		expect(t[1].children[0].children[0].value).toBe('b/x/1');
	});

	it('filters keeping ancestors and descendants', () => {
		const t = tree([tag('a/cat'), tag('a/dog'), tag('cats/big'), tag('z')]);
		const f = filterTree(t, 'CAT');
		expect(f.map((n) => n.value)).toEqual(['a', 'cats']);
		expect(f[0].children.map((n) => n.value)).toEqual(['a/cat']);
		expect(f[1].children.map((n) => n.value)).toEqual(['cats/big']);
		expect(filterTree(t, ' ')).toBe(t);
	});

	it('classifies changes', () => {
		const c = (from: string, to: string, exists = false) => ({ from, to, exists, assets: 0 });
		expect(kind(c('a/b', 'a/c'))).toBe('rename');
		expect(kind(c('x', 'y'))).toBe('rename');
		expect(kind(c('x', 'y/x'))).toBe('move');
		expect(kind(c('cat', 'cats', true))).toBe('merge');
	});
});
