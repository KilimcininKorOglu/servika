package apps

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/config"
)

// reUnitFile matches a unit this package owns, so healing never touches a unit
// somebody else put in the same directory.
var reUnitFile = regexp.MustCompile(`^servika-app-([0-9]+)\.service$`)

// HealOnStartup brings the host back in line with the database.
//
// Three things drift. A unit can be missing after a restore that brought the
// database back but not /etc. An enabled application can be down after a reboot
// that raced its dependencies. And a unit can outlive its row when a delete only
// half completed, which matters more than the others because that unit still
// holds a port the allocator believes is free.
func HealOnStartup(db *sql.DB) {
	ctx := context.Background()
	list, err := ListAll(ctx, db)
	if err != nil {
		log.Printf("apps: startup heal could not read the applications: %v", err)
		return
	}

	known := make(map[int64]bool, len(list))
	for _, app := range list {
		known[app.ID] = true
		healOne(ctx, db, app)
	}
	removeOrphanUnits(known)
}

// healOne repairs one application's presence on the host.
func healOne(ctx context.Context, db *sql.DB, app App) {
	var systemUser string
	if err := db.QueryRowContext(ctx,
		`SELECT system_user FROM domains WHERE id=?`, app.DomainID).Scan(&systemUser); err != nil {
		log.Printf("apps: heal application %d: read its domain: %v", app.ID, err)
		return
	}
	if !ValidSystemUser(systemUser) {
		log.Printf("apps: heal application %d: %q is not a tenant login", app.ID, systemUser)
		return
	}
	appDir, err := SafeAppDir(systemUser, app.AppRoot)
	if err != nil {
		log.Printf("apps: heal application %d: %v", app.ID, err)
		return
	}
	argv, err := ParseStartCommand(app.Start)
	if err != nil {
		log.Printf("apps: heal application %d: %v", app.ID, err)
		return
	}
	execStart, err := ResolveExec(app.Runtime, app.Version, appDir, argv)
	if err != nil {
		// The interpreter this application was created against is gone. Saying
		// so is the whole repair: rewriting the unit against a different one
		// would run the application on a runtime nobody chose.
		log.Printf("apps: application %d cannot start: %v", app.ID, err)
		return
	}

	want := RenderUnit(app, systemUser, appDir, execStart)
	// #nosec G304 -- a fixed path this package owns, named after a row id.
	have, readErr := os.ReadFile(UnitPath(app.ID))
	if readErr != nil || string(have) != want {
		values, err := ReadEnv(ctx, db, app.ID)
		if err != nil {
			log.Printf("apps: heal application %d: %v", app.ID, err)
			return
		}
		if err := WriteEnvFile(app, values); err != nil {
			log.Printf("apps: heal application %d: %v", app.ID, err)
			return
		}
		if err := EnsureLogFile(app.ID); err != nil {
			log.Printf("apps: heal application %d: %v", app.ID, err)
			return
		}
		if err := InstallUnit(app.ID, want); err != nil {
			log.Printf("apps: heal application %d: %v", app.ID, err)
			return
		}
		log.Printf("apps: rewrote the unit of application %d", app.ID)
	}

	status := UnitStatus(app.ID)
	switch {
	case app.Enabled && status.ActiveState != "active" && status.ActiveState != "activating":
		if err := Enable(app.ID); err != nil {
			log.Printf("apps: start application %d: %v", app.ID, err)
		}
	case !app.Enabled && status.ActiveState == "active":
		// The row says stopped. Restart=always means a process that came back
		// on its own would otherwise stay up against the panel's own record.
		if err := Disable(app.ID); err != nil {
			log.Printf("apps: stop application %d: %v", app.ID, err)
		}
	}
}

// removeOrphanUnits tears down units whose application row is gone, freeing the
// port they still hold.
func removeOrphanUnits(known map[int64]bool) {
	entries, err := os.ReadDir(unitDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		match := reUnitFile.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		id, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || known[id] {
			continue
		}
		log.Printf("apps: removing unit %s, which has no application row", entry.Name())
		Teardown(id)
	}
	removeOrphanFiles(known, config.AppEnvDir(), ".env")
	removeOrphanFiles(known, config.AppLogDir(), ".log")
}

// removeOrphanFiles drops the per-application files left behind by a row that no
// longer exists.
func removeOrphanFiles(known map[int64]bool, dir, suffix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimSuffix(name, suffix), 10, 64)
		if err != nil || known[id] {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}
