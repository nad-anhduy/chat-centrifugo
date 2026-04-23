package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"be-chat-centrifugo/config"
	"github.com/gocql/gocql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.LoadConfig("config/config.yaml")
	if err != nil {
		log.Printf("Could not load config file, falling back to ENV: %v", err)
	}

	// 1. Initialize Postgres
	fmt.Println(">>> Starting Postgres initialization...")
	db, err := connectPostgres(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}

	sqlFiles, err := listMigrationFiles("migrations", "*.up.sql")
	if err != nil {
		log.Fatalf("Failed to list Postgres migrations: %v", err)
	}
	if len(sqlFiles) == 0 {
		log.Fatalf("No Postgres migration files found in migrations/")
	}
	for _, f := range sqlFiles {
		fmt.Printf(">>> [Postgres] Applying %s...\n", f)
		if err := runSQLMigration(db, f); err != nil {
			log.Fatalf("Failed to run Postgres migrations: %v", err)
		}
	}
	fmt.Println(">>> Postgres initialization complete.")

	// 2. Initialize ScyllaDB
	fmt.Println(">>> Starting ScyllaDB initialization...")
	cqlFiles, err := listMigrationFiles("migrations", "*.up.cql")
	if err != nil {
		log.Fatalf("Failed to list Scylla migrations: %v", err)
	}
	if len(cqlFiles) == 0 {
		log.Fatalf("No Scylla migration files found in migrations/")
	}

	// First connect without keyspace to ensure we can create it
	session, err := connectScylla(cfg.ScyllaHosts, "")
	if err != nil {
		log.Fatalf("Failed to connect to ScyllaDB: %v", err)
	}

	// Pass 1: create keyspace statements across all files.
	for _, f := range cqlFiles {
		fmt.Printf(">>> [Scylla] (keyspace) Applying %s...\n", f)
		if err := runCQLMigration(session, f, true); err != nil {
			log.Fatalf("Failed to run ScyllaDB migrations (keyspace phase): %v", err)
		}
	}
	session.Close()

	// Re-connect with the specific keyspace to ensure tables are created in the right place
	session, err = connectScylla(cfg.ScyllaHosts, cfg.ScyllaKeyspace)
	if err != nil {
		log.Fatalf("Failed to connect to ScyllaDB with keyspace %s: %v", cfg.ScyllaKeyspace, err)
	}
	defer session.Close()

	// Pass 2: run table/alter statements (skip CREATE KEYSPACE + USE).
	for _, f := range cqlFiles {
		fmt.Printf(">>> [Scylla] (tables) Applying %s...\n", f)
		if err := runCQLMigration(session, f, false); err != nil {
			log.Fatalf("Failed to run ScyllaDB migrations (table phase): %v", err)
		}
	}
	fmt.Println(">>> ScyllaDB initialization complete.")
}

func listMigrationFiles(dir string, pattern string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func connectPostgres(dsn string) (*gorm.DB, error) {
	var db *gorm.DB
	var err error
	for i := 1; i <= 15; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			return db, nil
		}
		log.Printf("Failed to connect to Postgres (attempt %d/15): %v. Retrying in 5s...", i, err)
		time.Sleep(5 * time.Second)
	}
	return nil, err
}

func connectScylla(hosts string, keyspace string) (*gocql.Session, error) {
	cluster := gocql.NewCluster(strings.Split(hosts, ",")...)
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second
	cluster.Keyspace = keyspace

	var session *gocql.Session
	var err error
	for i := 1; i <= 15; i++ {
		session, err = cluster.CreateSession()
		if err == nil {
			return session, nil
		}
		log.Printf("Failed to connect to ScyllaDB with keyspace %q (attempt %d/15): %v. Retrying in 5s...", keyspace, i, err)
		time.Sleep(5 * time.Second)
	}
	return nil, err
}

func runSQLMigration(db *gorm.DB, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read SQL file %s: %w", filePath, err)
	}

	// Simple split by semicolon. Note: This might fail if there are semicolons in strings or comments.
	// But for simple migrations it's usually fine.
	queries := strings.Split(string(content), ";")
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if err := db.Exec(q).Error; err != nil {
			return fmt.Errorf("failed to execute query: %s\nError: %w", q, err)
		}
	}
	return nil
}

func runCQLMigration(session *gocql.Session, filePath string, onlyKeyspace bool) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read CQL file %s: %w", filePath, err)
	}

	queries := strings.Split(string(content), ";")
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}

		upperQ := strings.ToUpper(q)
		isKeyspaceStmt := strings.HasPrefix(upperQ, "CREATE KEYSPACE")
		isUseStmt := strings.HasPrefix(upperQ, "USE ")

		if onlyKeyspace {
			if !isKeyspaceStmt {
				continue
			}
		} else {
			if isKeyspaceStmt || isUseStmt {
				continue
			}
		}

		if err := session.Query(q).Exec(); err != nil {
			return fmt.Errorf("failed to execute CQL: %s\nError: %w", q, err)
		}
	}
	return nil
}
