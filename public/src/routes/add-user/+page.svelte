<script lang="ts">
	import { goto } from '$app/navigation';

	let username: HTMLInputElement;
	let password: HTMLInputElement;
	let e = '';
	async function addUser() {
		try {
			const res = await fetch('/api/users', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ username: username.value.trim(), password: password.value.trim() })
			});
			console.log(res.status);
			if (res.status === 201) {
				goto('/');
			} else {
				e = res.status.toString();
			}
		} catch (error) {
			console.log(error);
		}
	}
</script>

<div class="min-h-svh w-full p-4">
	<input
		class="border rounded border-neutral-800 px-4 py-2 bg-neutral-950"
		bind:this={username}
		type="text"
		placeholder="username"
	/>
	<input
		class="border rounded border-neutral-800 px-4 py-2 bg-neutral-950"
		bind:this={password}
		type="text"
		placeholder="password"
	/>
	<div class="flex items-center">
		<button on:click={addUser} class="mr-4 rounded border border-neutral-800 text-lg px-4 py-2"
			>CREATE</button
		>
		<a href="/" class="rounded border border-neutral-800 text-lg px-4 py-2">BACK</a>
	</div>
	<div class="text-red-500">
		{e}
	</div>
</div>
