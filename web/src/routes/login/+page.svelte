<script lang="ts">
	import { onMount } from 'svelte';
	import { api, login } from '#lib/api';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Label } from '#lib/components/ui/label';
	import { Card, CardContent, CardHeader, CardTitle } from '#lib/components/ui/card';
	import logo from '#lib/assets/favicon.svg';

	type Captcha = { id: string; question: string; expires_in: number };

	let captcha = $state<Captcha | null>(null);
	let password = $state('');
	let totp = $state('');
	let answer = $state('');
	let error = $state('');
	let busy = $state(false);

	async function loadCaptcha() {
		captcha = await api.get<Captcha>('/auth/captcha');
		answer = '';
	}

	onMount(loadCaptcha);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (!captcha || busy) return;
		busy = true;
		error = '';
		try {
			await login(password, totp, captcha.id, answer);
			window.location.assign('/admin');
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Sign in failed.';
			await loadCaptcha();
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
						<Label for="captcha">Human check · {captcha?.question ?? '…'}</Label>
						<Input id="captcha" inputmode="numeric" bind:value={answer} autocomplete="off" required />
					</div>
					{#if error}<p class="text-sm text-destructive">{error}</p>{/if}
					<Button class="w-full" type="submit" disabled={busy}>{busy ? 'Checking…' : 'Open the desk'}</Button>
				</form>
			</CardContent>
		</Card>
		<p class="mt-4 text-center text-[11px] text-muted-foreground">IP-restricted. All actions are audited.</p>
	</div>
</div>
