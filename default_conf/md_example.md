title: Example Page
author: Jason Stewart
ldelim: {{
rdelim: }}
---

### Default Variables

| Field      | Value             |
| -----      | -----             |
| title      | {{ .title }}      |
| path       | {{ .path }}       |
| fname      | {{ .fname }}      |
| modified   | {{ .modified }}   |
| watchmode  | {{ .watchmode }}  |

*NOTE*: Uses golang's text/template syntax.
