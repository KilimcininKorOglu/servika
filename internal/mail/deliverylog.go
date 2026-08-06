package mail

import (
	"bufio"
	"context"
	"database/sql"
	"log"
	"os"
	"time"

	"servika/internal/config"
)

// Accumulating the shared Postfix log into per-domain rows.
//
// Postfix writes one log for the whole server, so it cannot be shown to a tenant
// as it stands: it names every other customer's correspondents. Reading it on
// every request would also mean scanning a file that grows all day. The
// accumulator follows the access-log aggregator in internal/stats: keep a cursor,
// read only what is new, and answer requests from a table.

const (
	// deliveryLogInterval is how often the log is drained. A minute is short
	// enough that a customer checking "did it go out?" sees the answer, and long
	// enough that the file is read once rather than per request.
	deliveryLogInterval = time.Minute
	// deliveryLogRetentionDays bounds the table. The log itself is rotated, so
	// keeping rows forever would grow a history nothing else has.
	deliveryLogRetentionDays = 30
	// senderCacheMax bounds the queue-ID map within one drain. Postfix reuses
	// queue IDs, and a malformed run must not turn the map into a leak.
	senderCacheMax = 20000
)

// StartDeliveryLogCollector drains the mail log on a timer.
func StartDeliveryLogCollector(db *sql.DB) {
	go func() {
		// Let the rest of the boot sequence finish first; nothing here is urgent.
		time.Sleep(45 * time.Second)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			if err := CollectDeliveryLog(ctx, db); err != nil {
				log.Printf("mail delivery log: %v", err)
			}
			// An unopened signon token would otherwise sit in the table until the
			// next signon happened to clean it up.
			pruneWebmailTokens(ctx, db)
			cancel()
			time.Sleep(deliveryLogInterval)
		}
	}()
}

// CollectDeliveryLog reads everything appended since the last run and stores the
// deliveries that belong to a hosted domain.
func CollectDeliveryLog(ctx context.Context, db *sql.DB) error {
	path := config.MailLog()
	info, err := os.Stat(path)
	if err != nil {
		// No mail log means mail is not set up on this host. That is not a fault.
		return nil
	}
	size := info.Size()

	var offset, previousSize int64
	_ = db.QueryRowContext(ctx,
		`SELECT offset, size FROM mail_log_cursor WHERE id = 1`).Scan(&offset, &previousSize)
	start := offset
	// A file that shrank was rotated, so the old offset points into the middle of
	// a different file. Starting over is the only correct reading.
	if size < offset || size < previousSize {
		start = 0
	}
	if start == size {
		return pruneDeliveryLog(ctx, db)
	}

	// #nosec G304 -- path is a fixed system path from the configuration, not tenant input.
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if start > 0 {
		if _, err := file.Seek(start, 0); err != nil {
			start = 0
			if _, err := file.Seek(0, 0); err != nil {
				return err
			}
		}
	}

	hosted, err := hostedMailDomains(ctx, db)
	if err != nil {
		return err
	}

	reader := bufio.NewReaderSize(file, 256*1024)
	senders := map[string]string{}
	reference := time.Now()
	consumed := start
	var pending []storedDelivery

	for {
		line, readErr := reader.ReadString('\n')
		// A final line without a newline is still being written; leaving the
		// cursor before it means the complete line is read on the next pass.
		if len(line) > 0 && line[len(line)-1] == '\n' {
			consumed += int64(len(line))
			if queueID, sender, ok := parseSender(line); ok {
				if len(senders) < senderCacheMax {
					senders[queueID] = sender
				}
			} else if record, ok := parseDelivery(line, reference); ok {
				record.Sender = senders[record.QueueID]
				pending = append(pending, matchDomains(record, hosted)...)
			}
		}
		if readErr != nil {
			break
		}
	}

	if err := storeDeliveries(ctx, db, pending); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO mail_log_cursor(id, offset, size) VALUES(1,?,?)
		 ON DUPLICATE KEY UPDATE offset=VALUES(offset), size=VALUES(size)`,
		consumed, size); err != nil {
		return err
	}
	return pruneDeliveryLog(ctx, db)
}

// storedDelivery is a record already resolved to the domain that owns it.
type storedDelivery struct {
	DomainID  int64
	Direction string
	Record    deliveryRecord
}

// matchDomains decides which hosted domains a delivery belongs to.
//
// A message from one hosted domain to another produces two rows, one on each
// side, because both customers have a reason to see it and neither should have
// to read the other's view to find it.
func matchDomains(record deliveryRecord, hosted map[string]int64) []storedDelivery {
	var out []storedDelivery
	if id, ok := hosted[addressDomain(record.Sender)]; ok {
		out = append(out, storedDelivery{DomainID: id, Direction: "out", Record: record})
	}
	if id, ok := hosted[addressDomain(record.Recipient)]; ok {
		out = append(out, storedDelivery{DomainID: id, Direction: "in", Record: record})
	}
	return out
}

// hostedMailDomains maps every active mail domain to its domain row.
func hostedMailDomains(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT domain_name, domain_id FROM mail_domains WHERE status = 'active'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	hosted := map[string]int64{}
	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			return nil, err
		}
		hosted[name] = id
	}
	return hosted, rows.Err()
}

func storeDeliveries(ctx context.Context, db *sql.DB, pending []storedDelivery) error {
	if len(pending) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	statement, err := tx.PrepareContext(ctx,
		`INSERT INTO mail_delivery_log(domain_id, ts, direction, sender, recipient, status, reason, queue_id)
		 VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer func() { _ = statement.Close() }()
	for _, item := range pending {
		if _, err := statement.ExecContext(ctx,
			item.DomainID, item.Record.At, item.Direction,
			item.Record.Sender, item.Record.Recipient,
			item.Record.Status, item.Record.Reason, item.Record.QueueID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func pruneDeliveryLog(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM mail_delivery_log WHERE ts < NOW() - INTERVAL ? DAY`, deliveryLogRetentionDays)
	return err
}
