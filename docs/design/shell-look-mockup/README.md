# Shell look and task rail mockup (2026-11-01)

A static mockup of the owner's 2026-11-01 decisions in the
[Home and navigation design](../home-and-navigation.md):

- olive two-tint header rows, with the current section as a tab joined to
  its pages row and green marking only the current page, badges and
  selected chips
- a rounded *Search Ctrl K* button in place of the header search field
- the [task rail](../home-and-navigation.md#task-rail) on the right, in its own
  tinted panel: *New transaction* plus grouped transaction types on
  Investments, and period, currency, filters and Download CSV on Spending

Open `mockup.html` in a browser. Query parameters:
- `?p=inv` or `?p=spend` picks the page
- `&theme=dark` switches to the dark theme

Resize below about 1180px to see the rail fold into the page header, and to
390px for a phone.

| File | Shows |
|---|---|
| `investments-desktop.png` | Investments at 1440px |
| `spending-desktop.png` | Spending at 1440px, filters in the rail |
| `investments-dark.png` | Dark theme |
| `investments-1024.png` | Rail folded into the page header |
| `investments-phone.png` | 390px phone |

Colours are proposals in the app's olive hue; implementation turns them
into theme tokens and checks contrast with axe. Content and figures are
illustrative only.
