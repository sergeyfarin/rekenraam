<script lang="ts">
  import { page } from '$app/state';
  import DividendReview from '#lib/imports/dividend-review.svelte';
  const batchID = $derived(Number(page.url.searchParams.get('batch_id')) || undefined);
</script>
<DividendReview {batchID} />
