# Service Plans & Customers

## Service Plans

Service plans define resource limits that apply to all domains assigned to the plan. Go to **Tools & Settings > Service Plans** (admin only).

### Default Plan

The **Starter** plan is created during installation and is selected by default when creating a domain.

### Plan Fields

| Field                  | Description                                |
|------------------------|--------------------------------------------|
| Name                   | Display name (e.g. "Professional")         |
| Disk Space (MB)        | Maximum disk usage per domain              |
| Monthly Bandwidth (GB) | Maximum monthly traffic                    |
| Max Domains            | Domains per customer account               |
| Max Databases          | Databases per domain                       |
| Max FTP Accounts       | FTP accounts per domain                    |
| Max Email Accounts     | Email accounts per domain                  |
| Max Cron Jobs          | Cron jobs per domain                       |
| PHP Memory Limit       | Default PHP `memory_limit`                 |
| Max PHP Workers        | Per-domain PHP-FPM `pm.max_children`       |
| I/O Limit (KB/s)       | Disk I/O bandwidth cap                     |
| I/O Limit (IOPS)       | Disk I/O operations cap                    |
| DB Max Connections     | Per-user MariaDB `MAX_USER_CONNECTIONS`    |
| DB Max Queries/Hour    | Per-user MariaDB `MAX_QUERIES_PER_HOUR`    |
| DB Max Updates/Hour    | Per-user MariaDB `MAX_UPDATES_PER_HOUR`    |
| Slow Query Timeout (s) | Log queries exceeding this threshold       |
| Slow Query Kill (s)    | Terminate queries exceeding this threshold |

### Creating a Plan

Click **Add Plan**, fill in the fields, and save. The plan is immediately available for domain assignment.

### Editing a Plan

Click a plan row, edit the fields, and save. Changes take effect on the next resource limit enforcement cycle (startup healing or manual trigger).

### Deleting a Plan

Plans with assigned domains cannot be deleted — reassign domains first.

### Assigning Plans to Domains

In the domain detail page, select a plan from the dropdown. The change triggers immediate resource limit enforcement.

### Plan Search

Search for domains assigned to a specific plan from the plan detail page.

## Resource Limit Enforcement

Limits are enforced at multiple levels:

| Layer        | Mechanism                                                                    |
|--------------|------------------------------------------------------------------------------|
| Disk         | XFS project quota on `/home/c_<user>/`                                       |
| Disk I/O     | systemd slice `IOReadBandwidthMax` / `IOWriteBandwidthMax` + cgroup `io.max` |
| PHP Workers  | PHP-FPM pool `pm.max_children`                                               |
| MariaDB      | `GRANT` options on the database user                                         |
| Slow Queries | `pt-kill` style query termination                                            |

Limits are reapplied during startup healing without restarting services. Zero-valued limits are cleared from both systemd and cgroup state.

## Customers

Customer accounts allow end-users to log in and manage their own domains without admin access. Go to **Customers** (admin only).

### Creating a Customer

| Field    | Description                             |
|----------|-----------------------------------------|
| Username | Login username                          |
| Password | Login password                          |
| Email    | Contact email                           |
| Plan     | Default plan for the customer's domains |

### Customer Login

Customers log in at the panel URL with their own credentials (separate from admin). They see only their assigned domains.

### Customer Scope

Customer-scoped API routes are protected by `middleware.CustomerScope`, which verifies the domain belongs to the authenticated customer. Customers cannot:

- Create, suspend, or delete domains
- Change service plans
- Access firewall, PHP extensions, packages, or system settings
- View other customers' domains

### Bulk Operations (Admin)

- **Bulk Change Owner** — Reassign multiple domains to a different customer
- **Bulk Change Status** — Suspend or resume multiple domains at once
