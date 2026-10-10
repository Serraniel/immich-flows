import { describe, expect, it } from 'vitest';
import schema from '../../../docs/flow.schema.json';
import { complete, type Schema } from './yamlschema';

// "|" marks the cursor.
function at(doc: string) {
	const pos = doc.indexOf('|');
	return complete(schema as Schema, doc.replace('|', ''), pos);
}
const labels = (doc: string) => at(doc)?.options.map((o) => o.label) ?? null;

describe('yaml schema completion', () => {
	it('completes root keys, skipping present ones', () => {
		expect(labels('id: a\n|')).toEqual(
			expect.arrayContaining(['name', 'trigger', 'steps', 'enabled'])
		);
		expect(labels('id: a\n|')).not.toContain('id');
		expect(at('id: a\ntr|')?.from).toBe(0);
	});

	it('completes enum values', () => {
		expect(labels('trigger:\n  on: |')).toContain('asset.uploaded');
		expect(at('trigger:\n  on: asset.|')?.from).toBe(6);
		expect(labels('enabled: |')).toEqual(['true', 'false']);
	});

	it('completes step keys and use values in sequences', () => {
		expect(labels('steps:\n  - |')).toEqual(expect.arrayContaining(['use', 'with', 'if']));
		expect(labels('steps:\n- use: tag.as|')).toContain('tag.assign');
		const use = at('steps:\n  - id: x\n    use: |')!.options.find((o) => o.label === 'hash');
		expect(use?.info).toBeTruthy();
	});

	it('completes with: keys from the step schema, using siblings in any order', () => {
		expect(labels('steps:\n  - use: hash\n    with:\n      |')).toContain('algorithms');
		expect(labels('steps:\n  - with:\n      |\n    use: hash\n')).toContain('algorithms');
		// Previous items do not leak into the current one.
		expect(
			labels('steps:\n  - use: hash\n    with: {}\n  - use: tag.assign\n    with:\n      |')
		).toContain('tags');
	});

	it('follows cases into nested steps', () => {
		const doc =
			'steps:\n  - use: switch\n    cases:\n      - if: x\n        steps:\n          - use: |';
		expect(labels(doc)).toContain('log');
	});

	it('gives up on unknown paths and flow style', () => {
		expect(labels('nope:\n  |')).toEqual([]);
		expect(at('trigger: {on: |}')).toBeNull();
	});
});
