<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { api, login } from '#lib/api';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Label } from '#lib/components/ui/label';
	import { Card, CardContent, CardHeader, CardTitle } from '#lib/components/ui/card';
	import logo from '#lib/assets/favicon.svg';

	type TurnstileApi = {
		render: (target: HTMLElement, options: Record<string, unknown>) => string;
		reset: (widgetId?: string) => void;
		remove: (widgetId: string) => void;
	};

	type TurnstileWindow = Window & { turnstile?: TurnstileApi };

	function turnstileApi() {
		return (window as TurnstileWindow).turnstile;
	}

	let turnstilePromise: Promise<void> | null = null;

	function loadTurnstile() {
		if (turnstileApi()) return Promise.resolve();
		if (turnstilePromise) return turnstilePromise;
		turnstilePromise = new Promise<void>((resolve, reject) => {
			const script = document.createElement('script');
			script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';
			script.async = true;
			script.defer = true;
			script.onload = () => resolve();
			script.onerror = () => {
				turnstilePromise = null;
				reject(new Error('Could not load the human verification widget.'));
			};
			document.head.appendChild(script);
		});
		return turnstilePromise;
	}

	let container: HTMLDivElement;
	let widgetId: string | null = null;
	let siteKey = $state('');
	let turnstileToken = $state('');
	let password = $state('');
	let totp = $state('');
	let error = $state('');
	let busy = $state(false);
	let ready = $state(false);
	let destroyed = false;

	onMount(async () => {
		try {
			const config = await api.get<{ site_key: string }>('/auth/config');
			siteKey = config.site_key;
			await loadTurnstile();
			if (destroyed || !container) return;
			widgetId = turnstileApi()!.render(container, {
				sitekey: siteKey,
				action: 'admin_login',
				appearance: 'interaction-only',
				theme: 'auto',
				size: 'flexible',
				execution: 'render',
				retry: 'auto',
				'refresh-expired': 'auto',
				callback: (token: string) => {
					turnstileToken = token;
					error = '';
				},
				'error-callback': () => {
					turnstileToken = '';
					error = 'Human verification failed. Try again.';
				},
				'expired-callback': () => {
					turnstileToken = '';
				},
				'timeout-callback': () => {
					turnstileToken = '';
					error = 'Human verification timed out. Try again.';
				},
				'unsupported-callback': () => {
					turnstileToken = '';
					error = 'This browser is not supported for human verification.';
				}
			});
			ready = true;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Human verification is unavailable.';
		}
	});

	onDestroy(() => {
		destroyed = true;
		if (widgetId && turnstileApi()) turnstileApi()!.remove(widgetId);
		widgetId = null;
	});

	function resetVerification() {
		turnstileToken = '';
		if (widgetId && turnstileApi()) turnstileApi()!.reset(widgetId);
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (!turnstileToken || busy) return;
		busy = true;
		error = '';
		try {
			await login(password, totp, turnstileToken);
			window.location.assign('/admin');
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Sign in failed.';
			resetVerification();
		} finally {
			busy = false;
		}
	}
</script>

<div class="grid min-h-screen place-items-center px-5 py-10">
	<div class="w-full max-w-sm">
		<div class="mb-6 flex items-center gap-3">
			<img src={logo} alt="" class="size-10 rounded-md" />
			<div>
				<p class="text-sm font-semibold">Keydeck</p>
				<p class="text-xs text-muted-foreground">private license + release control</p>
			</div>
		</div>
		<Card class="desk-rise">
			<CardHeader><CardTitle>Sign in</CardTitle></CardHeader>
			<CardContent>
				<form class="space-y-4" onsubmit={submit}>
					<div class="space-y-1.5">
						<Label for="password">Password</Label>
						<Input id="password" type="password" bind:value={password} autocomplete="current-password" required />
					</div>
					<div class="space-y-1.5">
						<Label for="totp">Authenticator code</Label>
						<Input id="totp" inputmode="numeric" bind:value={totp} autocomplete="one-time-code" maxlength={6} required />
					</div>
					<div class="space-y-1.5">
						<Label>Human check</Label>
						<div bind:this={container} class="min-h-[65px]"></div>
						{#if !ready && !error}
							<p class="text-xs text-muted-foreground">Loading verification…</p>
						{/if}
					</div>
					{#if error}<p class="text-sm text-destructive">{error}</p>{/if}
					<Button class="w-full" type="submit" disabled={busy || !turnstileToken || !ready}>
						{busy ? 'Checking…' : 'Open the desk'}
					</Button>
				</form>
			</CardContent>
		</Card>
		<p class="mt-4 text-center text-[11px] text-muted-foreground">IP-restricted. All actions are audited.</p>
	</div>
</div>
