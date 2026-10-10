import { describe, expect, it } from 'vitest';
import { InvalidFlow, offset } from './flows';

describe('flows', () => {
	it('maps line and column to offsets', () => {
		const t = 'ab\ncde\n';
		expect(offset(t, 1, 1)).toBe(0);
		expect(offset(t, 2, 2)).toBe(4);
		expect(offset(t, 2, 99)).toBe(6);
		expect(offset(t, 9, 1)).toBe(t.length);
		expect(offset(t, 1, 0)).toBe(0);
	});

	it('formats problems', () => {
		expect(new InvalidFlow([{ line: 3, col: 5, message: 'bad' }]).message).toBe('3:5: bad');
	});
});
