# PHP Management

Servika supports multiple PHP versions running concurrently, with per-domain version selection and per-version extension management.

## PHP Versions

### Available Versions

| Version | Source           | Notes                                    |
|---------|------------------|------------------------------------------|
| 7.4     | Remi             | Legacy support                           |
| 8.0     | Remi             |                                          |
| 8.1     | Remi             |                                          |
| 8.2     | Remi             |                                          |
| 8.3     | AppStream / Remi | AppStream fallback if Remi not available |
| 8.4     | Remi             |                                          |
| 8.5     | Remi             |                                          |
| 8.6     | Remi             | Latest                                   |

### Installing a PHP Version

Go to **Tools & Settings > PHP Versions** (admin only). Versions available in the DNF repositories are listed with their install status.

Click **Install** next to a version. The panel runs `dnf install` for the PHP packages and registers the FPM service. After installation, the version becomes available for domain assignment.

### Removing a PHP Version

Click **Remove** next to an installed version. The panel runs `dnf remove`. Domains using that version must be switched to another version first.

## PHP Extensions

Go to **Tools & Settings > PHP Extensions** (admin only).

### Extension List

Shows all installed PHP extensions per version, with their status (enabled/disabled).

### Toggling Extensions

Enable or disable extensions per PHP version. Changes take effect after the PHP-FPM service for that version is reloaded.

### Installing PECL Extensions

Click **PECL Install** and provide the extension name. The panel runs `pecl install` for the selected PHP version.

### Removing PECL Extensions

Click **PECL Uninstall** and provide the extension name. The panel runs `pecl uninstall`.

## IonCube Loader

The panel can install and remove the IonCube loader for PHP 7.4 and 8.x.

- **Install IonCube** — Downloads and configures the IonCube loader extension for the selected PHP version
- **Remove IonCube** — Removes the IonCube loader configuration

## Per-Domain PHP Settings

On the domain detail page, go to **PHP Settings**.

### Customizable Settings

| Setting               | Description                   |
|-----------------------|-------------------------------|
| `memory_limit`        | Maximum memory per script     |
| `max_execution_time`  | Maximum script execution time |
| `upload_max_filesize` | Maximum upload file size      |
| `post_max_size`       | Maximum POST data size        |
| `max_input_vars`      | Maximum input variables       |
| `display_errors`      | Show errors in output         |
| `error_reporting`     | Error reporting level         |

Settings are applied to the domain's PHP-FPM pool configuration and take effect on the next FPM reload.

## Composer

On the domain detail page, go to the **Composer** tab.

### View Status

Shows whether `composer.json` exists in the domain's web root and the currently installed packages.

### Run Composer

| Action  | Equivalent                   |
|---------|------------------------------|
| Install | `composer install`           |
| Update  | `composer update`            |
| Require | `composer require <package>` |

Composer runs as the domain's system user with the correct `COMPOSER_HOME` and PHP binary.

## PHP Extensions (per version)

The panel detects Remi PHP packages and maps extension names. Common extensions:

- `php-curl`, `php-gd`, `php-mbstring`, `php-mysqlnd`, `php-xml`
- `php-zip`, `php-json`, `php-intl`, `php-bcmath`
- `php-opcache`, `php-redis`, `php-imagick`
