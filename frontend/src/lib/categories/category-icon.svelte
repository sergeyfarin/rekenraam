<script lang="ts">
  import Banknote from '@lucide/svelte/icons/banknote';
  import BriefcaseBusiness from '@lucide/svelte/icons/briefcase-business';
  import Bus from '@lucide/svelte/icons/bus';
  import Car from '@lucide/svelte/icons/car';
  import CircleDollarSign from '@lucide/svelte/icons/circle-dollar-sign';
  import Coffee from '@lucide/svelte/icons/coffee';
  import Ellipsis from '@lucide/svelte/icons/ellipsis';
  import Fuel from '@lucide/svelte/icons/fuel';
  import Gift from '@lucide/svelte/icons/gift';
  import GraduationCap from '@lucide/svelte/icons/graduation-cap';
  import HandHeart from '@lucide/svelte/icons/hand-heart';
  import HeartPulse from '@lucide/svelte/icons/heart-pulse';
  import House from '@lucide/svelte/icons/house';
  import Landmark from '@lucide/svelte/icons/landmark';
  import Plane from '@lucide/svelte/icons/plane';
  import Receipt from '@lucide/svelte/icons/receipt';
  import Shield from '@lucide/svelte/icons/shield';
  import Shirt from '@lucide/svelte/icons/shirt';
  import ShoppingCart from '@lucide/svelte/icons/shopping-cart';
  import Tags from '@lucide/svelte/icons/tags';
  import Users from '@lucide/svelte/icons/users';
  import Utensils from '@lucide/svelte/icons/utensils';
  import WalletCards from '@lucide/svelte/icons/wallet-cards';
  import Wrench from '@lucide/svelte/icons/wrench';
  import type { CategoryIconName } from './category-icons';

  let {
    icon = 'tags',
    className = 'size-4'
  } = $props<{
    icon?: CategoryIconName | string;
    className?: string;
  }>();
</script>

{#if icon === 'banknote'}
  <Banknote class={className} aria-hidden="true" />
{:else if icon === 'briefcase-business'}
  <BriefcaseBusiness class={className} aria-hidden="true" />
{:else if icon === 'bus'}
  <Bus class={className} aria-hidden="true" />
{:else if icon === 'car'}
  <Car class={className} aria-hidden="true" />
{:else if icon === 'circle-dollar-sign'}
  <CircleDollarSign class={className} aria-hidden="true" />
{:else if icon === 'coffee'}
  <Coffee class={className} aria-hidden="true" />
{:else if icon === 'ellipsis'}
  <Ellipsis class={className} aria-hidden="true" />
{:else if icon === 'fuel'}
  <Fuel class={className} aria-hidden="true" />
{:else if icon === 'gift'}
  <Gift class={className} aria-hidden="true" />
{:else if icon === 'graduation-cap'}
  <GraduationCap class={className} aria-hidden="true" />
{:else if icon === 'hand-heart'}
  <HandHeart class={className} aria-hidden="true" />
{:else if icon === 'heart-pulse'}
  <HeartPulse class={className} aria-hidden="true" />
{:else if icon === 'house'}
  <House class={className} aria-hidden="true" />
{:else if icon === 'landmark'}
  <Landmark class={className} aria-hidden="true" />
{:else if icon === 'plane'}
  <Plane class={className} aria-hidden="true" />
{:else if icon === 'receipt'}
  <Receipt class={className} aria-hidden="true" />
{:else if icon === 'shield'}
  <Shield class={className} aria-hidden="true" />
{:else if icon === 'shirt'}
  <Shirt class={className} aria-hidden="true" />
{:else if icon === 'shopping-cart'}
  <ShoppingCart class={className} aria-hidden="true" />
{:else if icon === 'users'}
  <Users class={className} aria-hidden="true" />
{:else if icon === 'utensils'}
  <Utensils class={className} aria-hidden="true" />
{:else if icon === 'wallet-cards'}
  <WalletCards class={className} aria-hidden="true" />
{:else if icon === 'wrench'}
  <Wrench class={className} aria-hidden="true" />
{:else}
  <Tags class={className} aria-hidden="true" />
{/if}
