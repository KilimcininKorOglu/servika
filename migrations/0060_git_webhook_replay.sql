-- 0060 - GitHub webhook replay protection. delivery_id is GitHub's
-- X-GitHub-Delivery UUID, which is globally unique per delivery.
CREATE TABLE IF NOT EXISTS git_webhook_deliveries (
  delivery_id VARCHAR(128) NOT NULL PRIMARY KEY,
  git_repo_id BIGINT UNSIGNED NOT NULL,
  event       VARCHAR(64) NOT NULL DEFAULT '',
  received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY ix_git_webhook_received (received_at),
  KEY ix_git_webhook_repo (git_repo_id),
  CONSTRAINT fk_git_webhook_repo
    FOREIGN KEY (git_repo_id) REFERENCES git_repos(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
