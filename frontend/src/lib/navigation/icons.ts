import House from '@lucide/svelte/icons/house';
import Landmark from '@lucide/svelte/icons/landmark';
import List from '@lucide/svelte/icons/list';
import Tags from '@lucide/svelte/icons/tags';
import Download from '@lucide/svelte/icons/download';
import ScanLine from '@lucide/svelte/icons/scan-line';
import ChartNoAxesCombined from '@lucide/svelte/icons/chart-no-axes-combined';
import Shapes from '@lucide/svelte/icons/shapes';
import TrendingUp from '@lucide/svelte/icons/trending-up';
import Coins from '@lucide/svelte/icons/coins';
import ChartLine from '@lucide/svelte/icons/chart-line';
import CalendarDays from '@lucide/svelte/icons/calendar-days';
import Wallet from '@lucide/svelte/icons/wallet';
import Repeat from '@lucide/svelte/icons/repeat';
import CalendarRange from '@lucide/svelte/icons/calendar-range';
import ChartPie from '@lucide/svelte/icons/chart-pie';
import ArrowLeftRight from '@lucide/svelte/icons/arrow-left-right';
import Settings2 from '@lucide/svelte/icons/settings-2';
import Palette from '@lucide/svelte/icons/palette';
import Languages from '@lucide/svelte/icons/languages';
import Database from '@lucide/svelte/icons/database';
import ShieldCheck from '@lucide/svelte/icons/shield-check';

/** Rendering stays separate so the route registry is plain data. */
export const routeIcons = {
  'house': House,
  'landmark': Landmark,
  'list': List,
  'tags': Tags,
  'download': Download,
  'scan-line': ScanLine,
  'chart-no-axes-combined': ChartNoAxesCombined,
  'shapes': Shapes,
  'trending-up': TrendingUp,
  'coins': Coins,
  'chart-line': ChartLine,
  'calendar-days': CalendarDays,
  'wallet': Wallet,
  'repeat': Repeat,
  'calendar-range': CalendarRange,
  'chart-pie': ChartPie,
  'arrow-left-right': ArrowLeftRight,
  'settings-2': Settings2,
  'palette': Palette,
  'languages': Languages,
  'database': Database,
  'shield-check': ShieldCheck,
};

export type RouteIcon = keyof typeof routeIcons;
