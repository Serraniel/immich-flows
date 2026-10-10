import { describe, expect, it } from 'vitest';
import schema from '../../../docs/flow.schema.json';
import { field, listText, parseList, stepTypes, triggers, withSchema } from './stepschema';
import type { Schema } from './yamlschema';

const flow = schema as Schema;
const kind = (use: string, key: string) => field(withSchema(flow, use)!.properties![key]);

describe('stepschema', () => {
	it('lists steps and triggers', () => {
		expect(stepTypes(flow).map((c) => c.value)).toEqual(
			expect.arrayContaining(['switch', 'hash', 'tag.assign'])
		);
		expect(triggers(flow).map((c) => c.value)).toContain('asset.uploaded');
		expect(withSchema(flow, 'switch')).toBeUndefined();
	});

	it('classifies fields', () => {
		expect(kind('hash', 'source').kind).toBe('select');
		expect(kind('hash', 'algorithms')).toMatchObject({ kind: 'checks' });
		expect(kind('tag.assign', 'tags')).toEqual({ kind: 'list', allowString: true });
		expect(kind('tag.merge', 'from')).toEqual({ kind: 'list', allowString: false });
		expect(kind('regex', 'caseInsensitive').kind).toBe('boolean');
		expect(kind('regex', 'pattern').kind).toBe('text');
		expect(kind('metadata.set', 'value').kind).toBe('yaml');
	});

	it('parses lists', () => {
		expect(parseList('a\n\n b \n', false)).toEqual(['a', 'b']);
		expect(parseList('a', true)).toBe('a');
		expect(parseList('a', false)).toEqual(['a']);
		expect(parseList('{{ trigger.data.trashed }}', false)).toBe('{{ trigger.data.trashed }}');
		expect(parseList('  ', true)).toBeUndefined();
		expect(listText(['a', 'b'])).toBe('a\nb');
		expect(listText(undefined)).toBe('');
	});
});
