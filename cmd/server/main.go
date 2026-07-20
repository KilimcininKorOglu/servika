package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"servika/internal/accounts"
	"servika/internal/antivirus"
	"servika/internal/auth"
	"servika/internal/backups"
	"servika/internal/composer"
	"servika/internal/config"
	"servika/internal/cron"
	"servika/internal/customer"
	"servika/internal/db"
	"servika/internal/dns"
	"servika/internal/domains"
	"servika/internal/files"
	"servika/internal/firewall"
	"servika/internal/git"
	githubpkg "servika/internal/github"
	"servika/internal/httpx"
	"servika/internal/logs"
	"servika/internal/middleware"
	"servika/internal/monitor"
	"servika/internal/nginxset"
	"servika/internal/packages"
	"servika/internal/passwordprotect"
	"servika/internal/performance"
	"servika/internal/php"
	"servika/internal/phpext"
	"servika/internal/phpversion"
	"servika/internal/plans"
	"servika/internal/plugin"
	"servika/internal/pma"
	"servika/internal/provisioner"
	"servika/internal/redis"
	"servika/internal/resource"
	"servika/internal/resourcelimit"
	"servika/internal/sitecopy"
	"servika/internal/sshaccess"
	"servika/internal/stats"
	"servika/internal/subdomain"
	"servika/internal/system"
	"servika/internal/users"
	"servika/internal/waf"
	"servika/internal/wordpress"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

const version = "0.3.0-f2"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	d, err := db.Open(cfg.DBDsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer func() { _ = d.Close() }()

	// migrations
	runMigrations(d)
	provisioner.Init(d)
	middleware.Init(d)
	if err := dns.SeedTemplateIfEmpty(context.Background(), d); err != nil {
		log.Printf("DNS template seed warn: %v", err)
	}
	if err := dns.HealZoneIncludes(context.Background(), d); err != nil {
		log.Printf("DNS zone include heal warn: %v", err)
	}

	ipv4 := detectIPv4()
	log.Printf("server ipv4: %s", ipv4)

	if err := domains.SeedIfEmpty(context.Background(), d, ipv4); err != nil {
		log.Printf("seed warn: %v", err)
	}
	if err := plans.SeedIfEmpty(context.Background(), d); err != nil {
		log.Printf("plans seed warn: %v", err)
	}
	if err := plans.SeedSync(context.Background(), d); err != nil {
		log.Printf("plans seed sync warn: %v", err)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		resourcelimit.HealTenantFPM(ctx, d)
	}()
	go resourcelimit.SlowQueryWatchdog(context.Background(), d)
	// HealQuotaOnStartup: reassert XFS user quota (disk + inode, CloudLinux parity) for every
	// tenant on boot. When the root XFS has noquota (single reboot pending after GRUB update),
	// all tenants are silently skipped — never a hard error. Runs in a background goroutine so
	// panel boot is not blocked.
	go resourcelimit.HealQuotaOnStartup(context.Background(), d)

	customerH := &customer.Handlers{DB: d, Secret: cfg.JWTSecret}
	authH := &auth.Handlers{DB: d, Secret: cfg.JWTSecret, LifetimeSec: cfg.JWTLifetime}
	usersH := &users.Handlers{DB: d}
	domainsH := &domains.Handlers{DB: d, IPv4: ipv4}
	filesH := &files.Handlers{DB: d}
	cronH := &cron.Handlers{DB: d}
	logsH := &logs.Handlers{DB: d}
	plansH := &plans.Handlers{DB: d}
	dnsH := &dns.Handlers{DB: d}
	accountsH := &accounts.Handlers{DB: d}
	backupsH := &backups.Handlers{DB: d}
	backups.StartScheduler(d)
	gitH := &git.Handlers{DB: d}
	githubH := &githubpkg.Handlers{DB: d, WebhookBase: "https://" + ipv4 + ":8443"}
	pmaH := &pma.Handlers{DB: d}
	phpH := &php.Handlers{DB: d}
	resourceH := &resource.Handlers{DB: d}
	monitorH := &monitor.Handlers{DB: d}
	nginxsetH := &nginxset.Handlers{DB: d}
	sshH := &sshaccess.Handlers{DB: d, IPv4: ipv4}
	statH := &stats.Handlers{DB: d}
	perfH := &performance.Handlers{DB: d}
	compH := &composer.Handlers{DB: d}
	protectionH := &passwordprotect.Handlers{DB: d}
	avH := &antivirus.Handlers{DB: d}
	copyH := &sitecopy.Handlers{DB: d}
	wpH := &wordpress.Handlers{DB: d}
	fwH := &firewall.Handlers{DB: d}
	wafH := &waf.Handlers{DB: d}
	redisH := &redis.Handlers{DB: d}
	subH := &subdomain.Handlers{DB: d, IPv4: ipv4}
	sshaccess.EnsureInfra()
	phpExtH := &phpext.Handlers{DB: d}
	packagesH := &packages.Handlers{DB: d}
	phpVersionH := &phpversion.Handlers{DB: d}
	// PERF: move PHP availability discovery (dnf) to a background sweeper so request-path
	// callers like /php/versions never block on a slow or locked dnf.
	phpversion.StartAvailabilitySweeper()
	pluginH := &plugin.Handlers{DB: d}
	go pluginH.HealthLoop(context.Background())

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(300 * time.Second))
	r.Use(chimw.Compress(5, "application/json", "text/html", "text/css", "text/javascript", "application/javascript"))
	r.Use(middleware.CORS)
	r.Use(middleware.MaintenanceMode)
	r.Use(middleware.BodyLimit)

	r.Post("/api/v1/git-webhook/{secret}", gitH.Webhook)
	r.Post("/api/v1/internal/pma-redeem", pmaH.Redeem)
	r.Get("/api/v1/plugin-bundle/{name}/app.js", pluginH.Bundle)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "up",
			"version": version,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", authH.Login)
		r.Post("/customer/login", customerH.Login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(cfg.JWTSecret))
			r.Get("/me", usersH.Me)
			r.With(middleware.AdminOnly).Put("/me", authH.UpdateProfile)
			r.With(middleware.AdminOnly).Post("/me/password", authH.ChangePassword)
			r.With(middleware.AdminOnly).Get("/me/2fa/setup", authH.TwoFASetup)
			r.With(middleware.AdminOnly).Post("/me/2fa/enable", authH.TwoFAEnable)
			r.With(middleware.AdminOnly).Post("/me/2fa/disable", authH.TwoFADisable)
			r.With(middleware.AdminOnly).Get("/domains", domainsH.List)
			r.With(middleware.AdminOnly).Get("/dns-template", dnsH.GetTemplate)
			r.With(middleware.AdminOnly).Put("/dns-template", dnsH.PutTemplate)
			r.With(middleware.CustomerScope).Get("/domains/{id}", domainsH.Get)
			r.With(middleware.AdminOnly).Get("/system/usage", system.Handler)
			r.With(middleware.AdminOnly).Get("/system/services", system.ServiceStatuses)
			r.With(middleware.AdminOnly).Post("/system/service-action", system.ServiceAction)
			r.With(middleware.AdminOnly).Get("/system/update", system.UpdateStatus)
			r.With(middleware.AdminOnly).Post("/system/update/start", system.StartUpdate)
			r.With(middleware.AdminOnly).Get("/system/update/log", system.UpdateLog)
			r.With(middleware.AdminOnly).Get("/system/optimize", system.OptimizeStatus)
			r.With(middleware.AdminOnly).Post("/system/optimize/start", system.OptimizeStart)
			r.With(middleware.AdminOnly).Get("/system/optimize/log", system.OptimizeLog)
			pluginH.Routes(r)
			r.With(middleware.AdminOnly).Get("/system/processes", monitor.Processes)
			r.With(middleware.AdminOnly).Get("/system/load-history", monitorH.LoadHistory)
			r.With(middleware.AdminOnly).Get("/admin/system/logs", monitorH.ServerLog)
			r.With(middleware.CustomerScope).Get("/domains/{id}/health", monitorH.Health)

			// Write + customer-scope routes — authorised per-route with AdminOnly/CustomerScope
			r.Group(func(r chi.Router) {
				r.With(middleware.AdminOnly).Post("/domains", domainsH.Create)
				r.With(middleware.AdminOnly).Post("/domains/{id}/suspend", domainsH.Suspend)
				r.With(middleware.AdminOnly).Post("/domains/{id}/resume", domainsH.Resume)
				r.With(middleware.CustomerScope).Delete("/domains/{id}", domainsH.Delete)
				r.With(middleware.AdminOnly).Post("/domains/bulk/owner", domainsH.BulkOwner)
				r.With(middleware.AdminOnly).Post("/domains/bulk/status", domainsH.BulkStatus)
				r.With(middleware.CustomerScope).Put("/domains/{id}/php", domainsH.SetPHP)
				r.With(middleware.CustomerScope).Get("/domains/{id}/ssh", sshH.Show)
				r.With(middleware.AdminOnly).Put("/domains/{id}/ssh", sshH.Configure)
				r.With(middleware.AdminOnly).Put("/domains/{id}/ssh/key", sshH.SaveKey)
				r.With(middleware.CustomerScope).Get("/domains/{id}/statistics", statH.Show)
				r.With(middleware.CustomerScope).Get("/domains/{id}/performance", perfH.Show)
				r.With(middleware.CustomerScope).Get("/domains/{id}/composer", compH.Status)
				r.With(middleware.CustomerScope).Post("/domains/{id}/composer", compH.Run)
				r.With(middleware.CustomerScope).Get("/domains/{id}/redis", redisH.Status)
				r.With(middleware.CustomerScope).Post("/domains/{id}/redis", redisH.Open)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/redis", redisH.Close)
				r.With(middleware.CustomerScope).Get("/domains/{id}/protection", protectionH.List)
				r.With(middleware.CustomerScope).Post("/domains/{id}/protection", protectionH.Add)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/protection/{kid}", protectionH.Delete)
				r.With(middleware.CustomerScope).Get("/domains/{id}/antivirus", avH.Status)
				r.With(middleware.CustomerScope).Post("/domains/{id}/antivirus/scan", avH.Scan)
				r.With(middleware.CustomerScope).Get("/domains/{id}/antivirus/scan/{sid}", avH.ScanStatus)
				r.With(middleware.CustomerScope).Post("/domains/{id}/antivirus/quarantine", avH.Quarantine)
				r.With(middleware.CustomerScope).Post("/domains/{id}/antivirus/update-signature", avH.UpdateSignature)
				r.With(middleware.CustomerScope).Get("/domains/{id}/copy", copyH.List)
				r.With(middleware.CustomerScope).Post("/domains/{id}/copy", copyH.Create)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/copy/{name}", copyH.Delete)
				r.With(middleware.CustomerScope).Get("/domains/{id}/wordpress", wpH.List)
				r.With(middleware.CustomerScope).Post("/domains/{id}/wordpress", wpH.Install)
				r.With(middleware.CustomerScope).Post("/domains/{id}/wordpress/update", wpH.Update)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/wordpress", wpH.Delete)
				// WordPress Toolkit — plugin/theme/user management + repair + tools
				r.With(middleware.CustomerScope).Get("/domains/{id}/wordpress/status", wpH.Status)
				r.With(middleware.CustomerScope).Get("/domains/{id}/wordpress/plugins", wpH.Plugins)
				r.With(middleware.CustomerScope).Post("/domains/{id}/wordpress/plugin", wpH.PluginAction)
				r.With(middleware.CustomerScope).Get("/domains/{id}/wordpress/themes", wpH.Themes)
				r.With(middleware.CustomerScope).Post("/domains/{id}/wordpress/theme", wpH.ThemeAction)
				r.With(middleware.CustomerScope).Get("/domains/{id}/wordpress/users", wpH.Users)
				r.With(middleware.CustomerScope).Post("/domains/{id}/wordpress/user-password", wpH.UserPassword)
				r.With(middleware.CustomerScope).Post("/domains/{id}/wordpress/repair", wpH.Repair)
				r.With(middleware.CustomerScope).Post("/domains/{id}/wordpress/tool", wpH.ToolAction)
				r.With(middleware.AdminOnly).Get("/wordpress/all", wpH.ListAll)
				r.With(middleware.AdminOnly).Get("/firewall", fwH.List)
				r.With(middleware.AdminOnly).Post("/firewall", fwH.Add)
				r.With(middleware.AdminOnly).Post("/firewall/template", fwH.Template)
				r.With(middleware.AdminOnly).Delete("/firewall/{id}", fwH.Delete)
				r.With(middleware.AdminOnly).Post("/firewall/{id}/status", fwH.Status)
				r.With(middleware.CustomerScope).Get("/domains/{id}/subdomain", subH.List)
				r.With(middleware.CustomerScope).Post("/domains/{id}/subdomain", subH.Create)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/subdomain/{sid}", subH.Delete)
				r.With(middleware.CustomerScope).Get("/domains/{id}/subdomain/{sid}/ssl", subH.SSLStatus)
				r.With(middleware.CustomerScope).Post("/domains/{id}/subdomain/{sid}/ssl", subH.SSLIssue)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/subdomain/{sid}/ssl", subH.SSLRemove)
				r.With(middleware.CustomerScope).Get("/domains/{id}/web-backend", domainsH.GetWebBackend)
				r.With(middleware.CustomerScope).Put("/domains/{id}/web-backend", domainsH.SetWebBackend)
				r.With(middleware.CustomerScope).Put("/domains/{id}/ftp/password", domainsH.SetFTPPassword)
				r.With(middleware.CustomerScope).Get("/domains/{id}/ftp/password-show", domainsH.ShowFTPPassword)
				r.With(middleware.CustomerScope).Get("/domains/{id}/databases", domainsH.ListDatabases)
				r.With(middleware.CustomerScope).Post("/domains/{id}/databases", domainsH.CreateDatabase)
				r.With(middleware.AdminOnly).Delete("/databases/{dbid}", domainsH.DeleteDatabase)
				r.With(middleware.AdminOnly).Put("/databases/{dbid}/password", domainsH.SetDatabasePassword)
				r.With(middleware.CustomerScope).Get("/domains/{id}/files", filesH.List)
				r.With(middleware.CustomerScope).Get("/domains/{id}/files/read", filesH.Read)
				r.With(middleware.CustomerScope).Get("/domains/{id}/files/download", filesH.Download)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/mkdir", filesH.Mkdir)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/upload", filesH.Upload)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/files", filesH.Delete)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/write", filesH.Write)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/rename", filesH.Rename)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/chmod", filesH.Chmod)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/extract", filesH.Extract)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/copy", filesH.Copy)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/move", filesH.Move)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/archive", filesH.Archive)
				r.With(middleware.CustomerScope).Post("/domains/{id}/files/new-file", filesH.NewFile)
				r.With(middleware.CustomerScope).Get("/domains/{id}/files/size", filesH.CalculateSize)
				r.With(middleware.CustomerScope).Get("/domains/{id}/files/search", filesH.Search)
				r.With(middleware.CustomerScope).Get("/domains/{id}/ssl", domainsH.SSLStatus)
				r.With(middleware.CustomerScope).Post("/domains/{id}/ssl/issue", domainsH.SSLIssue)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/ssl", domainsH.SSLDisable)
				r.With(middleware.CustomerScope).Get("/domains/{id}/cron", cronH.List)
				r.With(middleware.CustomerScope).Post("/domains/{id}/cron", cronH.Create)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/cron/{idx}", cronH.Delete)
				r.With(middleware.CustomerScope).Get("/domains/{id}/logs", logsH.List)
				r.With(middleware.CustomerScope).Get("/domains/{id}/logs/read", logsH.Read)
				r.With(middleware.CustomerScope).Get("/domains/{id}/logs/live", logsH.Tail)
				r.With(middleware.CustomerScope).Post("/domains/{id}/calculate-disk", domainsH.CalculateDisk)
				r.With(middleware.AdminOnly).Get("/plans", plansH.List)
				r.With(middleware.AdminOnly).Get("/plans/{id}", plansH.Get)
				r.With(middleware.AdminOnly).Post("/plans", plansH.Create)
				r.With(middleware.AdminOnly).Put("/plans/{id}", plansH.Update)
				r.With(middleware.AdminOnly).Delete("/plans/{id}", plansH.Delete)
				r.With(middleware.AdminOnly).Get("/plans/{id}/domains", plansH.SearchDomains)
				r.With(middleware.AdminOnly).Put("/domains/{id}/plan", domainsH.SetPlan)
				r.With(middleware.CustomerScope).Get("/domains/{id}/dns", dnsH.List)
				r.With(middleware.CustomerScope).Post("/domains/{id}/dns", dnsH.Create)
				r.With(middleware.CustomerScope).Put("/domains/{id}/dns/{rid}", dnsH.Update)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/dns/{rid}", dnsH.Delete)
				r.With(middleware.CustomerScope).Post("/domains/{id}/dns/template", dnsH.ApplyTemplate)
				r.With(middleware.CustomerScope).Post("/domains/{id}/dns/bulk-delete", dnsH.BulkDelete)
				r.With(middleware.CustomerScope).Post("/domains/{id}/dns/bulk-status", dnsH.BulkStatus)
				r.With(middleware.CustomerScope).Get("/domains/{id}/dns/soa", dnsH.GetSOA)
				r.With(middleware.CustomerScope).Put("/domains/{id}/dns/soa", dnsH.PutSOA)
				r.With(middleware.CustomerScope).Get("/domains/{id}/dns/dnssec", dnsH.GetDNSSEC)
				r.With(middleware.CustomerScope).Post("/domains/{id}/dns/dnssec", dnsH.PostDNSSEC)
				r.With(middleware.AdminOnly).Get("/customers", accountsH.ListCustomers)
				r.With(middleware.AdminOnly).Post("/customers", accountsH.CreateCustomer)
				r.With(middleware.AdminOnly).Put("/customers/{id}", accountsH.UpdateCustomer)
				r.With(middleware.AdminOnly).Delete("/customers/{id}", accountsH.DeleteCustomer)
				r.With(middleware.CustomerScope).Get("/domains/{id}/backups", backupsH.List)
				r.With(middleware.CustomerScope).Post("/domains/{id}/backups", backupsH.Create)
				r.With(middleware.CustomerScope).Get("/domains/{id}/backups/{bid}/download", backupsH.Download)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/backups/{bid}", backupsH.Delete)
				r.With(middleware.CustomerScope).Post("/domains/{id}/backups/{bid}/restore", backupsH.Restore)
				r.With(middleware.CustomerScope).Get("/domains/{id}/backup-schedule", backupsH.GetSchedule)
				r.With(middleware.CustomerScope).Put("/domains/{id}/backup-schedule", backupsH.SetSchedule)
				r.With(middleware.AdminOnly).Post("/admin/backups/tick", backupsH.TickNow)
				r.With(middleware.AdminOnly).Post("/admin/traffic/tick", func(w http.ResponseWriter, _ *http.Request) {
					processed := stats.AggregateAll(d)
					httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "processed_domains": processed})
				})
				r.With(middleware.AdminOnly).Get("/admin/backups/summary", backupsH.Summary)
				r.With(middleware.CustomerScope).Get("/domains/{id}/backup-destination", backupsH.GetDestination)
				r.With(middleware.CustomerScope).Put("/domains/{id}/backup-destination", backupsH.PutDestination)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/backup-destination", backupsH.DeleteDestination)
				r.With(middleware.CustomerScope).Post("/domains/{id}/backup-destination/test", backupsH.TestDestination)
				r.With(middleware.CustomerScope).Get("/domains/{id}/git", gitH.Get)
				r.With(middleware.CustomerScope).Post("/domains/{id}/git", gitH.Connect)
				r.With(middleware.CustomerScope).Post("/domains/{id}/git/clone", gitH.Clone)
				r.With(middleware.CustomerScope).Post("/domains/{id}/git/pull", gitH.Pull)
				r.With(middleware.CustomerScope).Get("/domains/{id}/github", githubH.Get)
				r.With(middleware.CustomerScope).Post("/domains/{id}/github/connect", githubH.Connect)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/github", githubH.Disconnect)
				r.With(middleware.CustomerScope).Get("/domains/{id}/github/repos", githubH.ListRepos)
				r.With(middleware.CustomerScope).Get("/domains/{id}/github/branches", githubH.ListBranches)
				r.With(middleware.CustomerScope).Post("/domains/{id}/github/use", githubH.Use)
				r.Post("/databases/{dbId}/pma-token", pmaH.RequestToken)
				r.Get("/php/versions", phpH.Versions)
				r.With(middleware.CustomerScope).Get("/domains/{id}/php-settings", phpH.GetSettings)
				r.With(middleware.CustomerScope).Put("/domains/{id}/php-settings", phpH.PutSettings)
				r.With(middleware.CustomerScope).Get("/domains/{id}/php/debug-log", phpH.GetDebugLog)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/php/debug-log", phpH.ClearDebugLog)
				r.With(middleware.CustomerScope).Get("/domains/{id}/resources", resourceH.Show)
				r.With(middleware.CustomerScope).Get("/domains/{id}/waf", wafH.Show)
				r.With(middleware.CustomerScope).Put("/domains/{id}/waf", wafH.Save)
				r.With(middleware.CustomerScope).Get("/domains/{id}/nginx-settings", nginxsetH.Show)
				r.With(middleware.CustomerScope).Put("/domains/{id}/nginx-settings", nginxsetH.Save)
				r.With(middleware.AdminOnly).Get("/php-extensions", phpExtH.List)
				r.With(middleware.AdminOnly).Put("/php-extensions/toggle", phpExtH.Toggle)
				r.With(middleware.AdminOnly).Post("/php-extensions/pecl-install", phpExtH.PECLInstall)
				r.With(middleware.AdminOnly).Post("/php-extensions/pecl-uninstall", phpExtH.PECLRemove)
				r.With(middleware.AdminOnly).Post("/php-extensions/ioncube-install", phpExtH.IonCubeInstall)
				r.With(middleware.AdminOnly).Post("/php-extensions/ioncube-remove", phpExtH.IonCubeRemove)
				r.With(middleware.AdminOnly).Get("/packages", packagesH.Search)
				r.With(middleware.AdminOnly).Get("/packages/installed", packagesH.Installed)
				r.With(middleware.AdminOnly).Get("/packages/info", packagesH.Info)
				r.With(middleware.AdminOnly).Get("/packages/status", packagesH.Status)
				r.With(middleware.AdminOnly).Post("/packages/install", packagesH.Install)
				r.With(middleware.AdminOnly).Post("/packages/remove", packagesH.Remove)
				r.With(middleware.AdminOnly).Post("/packages/update", packagesH.Update)
				r.With(middleware.AdminOnly).Get("/php-versions", phpVersionH.List)
				r.With(middleware.AdminOnly).Post("/php-versions/install", phpVersionH.Install)
				r.With(middleware.AdminOnly).Post("/php-versions/remove", phpVersionH.Remove)
				r.With(middleware.CustomerScope).Delete("/domains/{id}/git", gitH.Delete)
			})
		})
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Minute,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	monitor.StartLoadSampler(d, 60*time.Second) // dashboard load-history sampler
	stats.StartTrafficAggregator(d, 5*time.Minute)
	if err := firewall.Reapply(d); err != nil {
		log.Printf("firewall reapply warn: %v", err)
	}

	go func() {
		log.Printf("servika %s listening on %s (env=%s)", version, cfg.ListenAddr, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Printf("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func runMigrations(d *sql.DB) {
	dir := "/opt/servika/src/migrations"
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("migration directory could not be read: %v", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			continue
		}
		log.Printf("migration: %s", e.Name())
		// Strip comment lines first
		var cleaned []string
		for _, line := range strings.Split(string(body), "\n") {
			t := strings.TrimSpace(line)
			if t == "" || strings.HasPrefix(t, "--") {
				continue
			}
			cleaned = append(cleaned, line)
		}
		sqlBody := strings.Join(cleaned, "\n")
		for _, stmt := range strings.Split(sqlBody, ";") {
			s := strings.TrimSpace(stmt)
			if s == "" {
				continue
			}
			if _, err := d.Exec(s); err != nil {
				log.Printf("  - error (%s): %v", e.Name(), err)
			}
		}
	}
}

func detectIPv4() string {
	if v := strings.TrimSpace(os.Getenv("SERVIKA_PUBLIC_IPV4")); v != "" {
		return v
	}
	// Return the first non-loopback IPv4 address as a simple fallback.
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip := ipnet.IP.To4(); ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}
