import { api } from './api';

export type Connection = { connected: boolean; createdAt?: string };

export const getConnection = (f: typeof fetch = fetch) =>
	api<Connection>('connection', undefined, f);
export const connect = () => api<Connection>('connection', { method: 'POST' });
export const disconnect = () => api<void>('connection', { method: 'DELETE' });
