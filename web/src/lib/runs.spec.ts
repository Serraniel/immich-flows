import { describe, expect, it } from 'vitest';
import { duration, runsQuery, type Run } from './runs';

describe('runs', () => {
	it('builds queries without empty values', () => {
		expect(runsQuery({})).toBe('runs');
		expect(runsQuery({ flow: 'a', status: '', before: 10 })).toBe('runs?flow=a&before=10');
	});

	it('formats durations', () => {
		const base = {
			id: 1,
			flowId: 'f',
			eventType: 'manual',
			userId: 'u',
			status: 'success'
		} as const;
		const run = (s: string, f?: string): Run => ({ ...base, startedAt: s, finishedAt: f });
		expect(duration(run('2026-01-01T00:00:00Z'))).toBe('…');
		expect(duration(run('2026-01-01T00:00:00Z', '2026-01-01T00:00:00.250Z'))).toBe('250 ms');
		expect(duration(run('2026-01-01T00:00:00Z', '2026-01-01T00:00:02.5Z'))).toBe('2.5 s');
	});
});
