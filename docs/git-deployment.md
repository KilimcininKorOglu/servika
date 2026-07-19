# Git & GitHub Deployment

Servika supports Git-based deployment with GitHub webhook integration for automatic deployments.

## Git Deployment

On the domain detail page, go to the **Git** tab.

### Connect a Repository

| Field          | Description                                         |
|----------------|-----------------------------------------------------|
| Repository URL | HTTPS or SSH Git URL                                |
| Branch         | Branch name (default: `main`)                       |
| Deploy Path    | Subdirectory within the web root (empty = web root) |

Click **Clone** to clone the repository for the first time, or **Connect** if the directory already exists.

### Pull Updates

Click **Pull** to fetch and merge changes from the remote repository. This runs `git pull` as the domain's system user.

### Disconnect

Removes the Git remote configuration but does not delete the files. The directory becomes a regular unmanaged directory.

## GitHub Integration

On the domain detail page, go to the **GitHub** tab.

### Connect GitHub

Authenticate with a GitHub personal access token or OAuth. Once connected:

| Action            | Description                                         |
|-------------------|-----------------------------------------------------|
| **List Repos**    | Browse repositories on the connected GitHub account |
| **List Branches** | View branches for a selected repository             |
| **Use Repo**      | Select a repository and branch for deployment       |

### Webhook Auto-Deploy

When a GitHub repository is connected, Servika can receive push events via webhook:

1. A webhook secret is generated for each domain
2. Add `https://<panel-domain>:8443/api/v1/git-webhook/<secret>` as a GitHub webhook URL
3. On each push to the configured branch, the panel runs `git pull` automatically

The webhook endpoint is unauthenticated (secret-based), so keep the webhook secret confidential.

### Disconnect

Removes the GitHub integration. The repository files remain in place.

## Git Webhook (Generic)

For non-GitHub repositories, use the generic webhook endpoint:

```
POST /api/v1/git-webhook/{secret}
```

Any POST to this URL triggers a `git pull` for the domain associated with that secret. This works with GitLab, Bitbucket, Gitea, or any service that can send a webhook on push events.

## Permissions

All Git operations run as the domain's system user, so file ownership is always correct. The domain user must have its SSH key configured if using SSH-based Git URLs (recommended for private repositories).
