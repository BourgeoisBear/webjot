title: Example Page
author: Jason Stewart
ldelim: {{
rdelim: }}
@@@@@@@

### Default Variables

| Field      | Value             |
| -----      | -----             |
| title      | {{ .title }}      |
| path       | {{ .PATH }}       |
| fname      | {{ .FNAME }}      |
| modified   | {{ .MODIFIED }}   |
| watchmode  | {{ .WATCHMODE }}  |

*NOTE*: Uses golang's text/template syntax.
