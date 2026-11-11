export type Session = { actor: string; csrf: string };

let csrf = '';

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

export function setCsrf(value: string) {
	csrf = value;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
	const headers = new Headers(init.headers);
	if (init.body && !(init.body instanceof FormData)) headers.set('content-type', 'application/json');
	if (csrf && init.method && init.method !== 'GET') headers.set('x-csrf-token', csrf);
	const response = await fetch(`/api${path}`, {
		...init,
		headers,
		credentials: 'same-origin'
	});
	if (!response.ok) {
		let message = `Request failed (${response.status})`;
		try {
			const body = await response.json();
			message = body.error || body.message || message;
		} catch {
			// keep the status message
		}
		throw new ApiError(response.status, message);
	}
	if (response.status === 204) return undefined as T;
	return (await response.json()) as T;
}

export const api = {
	get: <T>(path: string) => request<T>(path),
	post: <T>(path: string, body?: unknown) =>
		request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) }),
	patch: <T>(path: string, body: unknown) =>
		request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
	del: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
	upload: <T>(path: string, form: FormData) =>
		request<T>(path, { method: 'POST', body: form })
};

export async function login(password: string, totp: string, turnstileToken: string) {
	const session = await api.post<Session>('/auth/login', {
		password,
		totp,
		turnstile_token: turnstileToken
	});
	setCsrf(session.csrf);
	return session;
}

export async function me() {
	const session = await api.get<Session>('/auth/me');
	setCsrf(session.csrf);
	return session;
}

export async function logout() {
	await api.post('/auth/logout');
	csrf = '';
}
