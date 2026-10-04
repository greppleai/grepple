<script lang="ts">
  import Button from './Button.svelte';
  let count: number = $state(0);
  function increment(): void {
    count += 1;
  }
</script>

{#snippet label(value)}
  <span>{value}</span>
{/snippet}

<section>
  {#if count > 0}
    <p>Count: {count}</p>
  {:else}
    <p>Ready</p>
  {/if}
  {#each [1, 2] as value (value)}
    {@render label(value)}
  {/each}
  <Button onclick={increment} />
</section>

<style>
  section { color: rebeccapurple; }
</style>