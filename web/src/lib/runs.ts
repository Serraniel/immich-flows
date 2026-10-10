import { api } from './api';

export type RunStatus = 'running' | 'success' | 'failed';

export type Step = {
	seq: number;
	stepId?: string;
	use: string;
	status: 'success' | 'failed' | 'skipped';
	output?: unknown;
	error?: string;
	startedAt: string;
	durationMs: number;
};

export type Run = {
	id: number;
	flowId: string;
	eventType: string;
	userId: string;
	assetId?: string;
	status: RunStatus;
	error?: string;
	startedAt: string;
	finishedAt?: string;
	steps?: Step[];
};

export type RunFilter = { flow?: string; status?: string; before?: number; limit?: number };

export function runsQuery(f: RunFilter): string {
	const q = new URLSearchParams();
	for (const [k, v] of Object.entries(f)) if (v !== undefined && v !== '') q.set(k, String(v));
	const s = q.toString();
	return s ? `runs?${s}` : 'runs';
}

export const listRuns = (f: RunFilter, fetchFn: typeof fetch = fetch) =>
	api<Run[]>(runsQuery(f), undefined, fetchFn);
export const getRun = (id: number | string, fetchFn: typeof fetch = fetch) =>
	api<Run>(`runs/${id}`, undefined, fetchFn);
export const retryRun = (id: number) =>
	api<{ queued: number }>(`runs/${id}/retry`, { method: 'POST' });

export function duration(run: Run): string {
	if (!run.finishedAt) return '…';
	const ms = new Date(run.finishedAt).getTime() - new Date(run.startedAt).getTime();
	return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
}
