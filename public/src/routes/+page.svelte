<script lang="ts">
	import { onMount } from 'svelte';

	let users: User[] = [];

	interface User {
		username: string;
		totalUsage: number;
		allowedUsage: number;
		expiresAt: number;
		disabled: boolean;
	}

	const formatBytes = (totalBytes: number, space = true) => {
		if (!totalBytes) return `00.00${space ? ' ' : ''}KB`;
		const totalKilos = totalBytes / 1024;
		const totalMegas = totalKilos / 1000;
		const totalGigas = totalMegas / 1000;
		const totalTeras = totalGigas / 1000;
		if (totalKilos < 100)
			return `${totalKilos < 10 ? '0' : ''}${totalKilos.toFixed(2)}${space ? ' ' : ''}KB`;
		if (totalMegas < 100)
			return `${totalMegas < 10 ? '0' : ''}${totalMegas.toFixed(2)}${space ? ' ' : ''}MB`;
		if (totalGigas < 100)
			return `${totalGigas < 10 ? '0' : ''}${totalGigas.toFixed(2)}${space ? ' ' : ''}GB`;
		return `${totalTeras < 10 ? '0' : ''}${totalTeras.toFixed(2)}${space ? ' ' : ''}TB`;
	};

	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

	onMount(async () => {
		try {
			while (true) {
				const res = await fetch('/api/users');
				const data: { string: User } = await res.json();
				users = Object.values(data);
				await sleep(1000);
			}
		} catch (error) {
			console.log(error);
		}
	});

	async function deleteUser(username: string) {
		try {
			if (window.confirm(`Delete ${username}?`)) {
				const res = await fetch('/api/users', {
					method: 'DELETE',
					headers: { 'content-type': 'application/json' },
					body: JSON.stringify({ username })
				});
				console.log(res.status);
			}
		} catch (error) {
			console.log(error);
		}
	}
</script>

<div class="min-h-svh w-full p-4">
	{#each users as user}
		<div class="flex rounded border border-neutral-800 my-4 px-4 py-2">
			<div>{user.username}</div>
			<div class="mx-1">:</div>
			<div>{formatBytes(user.totalUsage)}</div>
			<div class="ml-1">{user.expiresAt}</div>
			<button class="ml-auto" on:click={() => deleteUser(user.username)}>DELETE</button>
		</div>
	{/each}
</div>
