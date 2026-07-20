package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: migrate up|down")
	}
	direction := os.Args[1]
	if direction != "up" && direction != "down" {
		log.Fatalf("direction must be 'up' or 'down', got %q", direction)
	}

	dsn := "host=localhost port=5432 user=livechat password=livechat dbname=livechat sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("db.Ping: %v", err)
	}

	// Ensure migrations tracking table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INT PRIMARY KEY,
		dirty   BOOLEAN NOT NULL DEFAULT FALSE
	)`)
	if err != nil {
		log.Fatalf("create schema_migrations: %v", err)
	}

	migrationsDir := "migrations"
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		log.Fatalf("read migrations dir: %v", err)
	}

	// Collect migration versions
	type mig struct {
		version int
		upFile  string
		downFile string
	}
	migrations := make(map[int]*mig)
	suffix := fmt.Sprintf(".%s.sql", direction)
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}
		var version int
		if _, err := fmt.Sscanf(parts[0], "%d", &version); err != nil {
			continue
		}
		if migrations[version] == nil {
			migrations[version] = &mig{version: version}
		}
		if direction == "up" {
			migrations[version].upFile = name
		} else {
			migrations[version].downFile = name
		}
	}

	// Sort versions
	versions := make([]int, 0, len(migrations))
	for v := range migrations {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	if direction == "down" {
		// Reverse for down migrations
		for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
			versions[i], versions[j] = versions[j], versions[i]
		}
	}

	for _, ver := range versions {
		m := migrations[ver]
		var fileName string
		if direction == "up" {
			fileName = m.upFile
		} else {
			fileName = m.downFile
		}
		if fileName == "" {
			continue
		}

		// Check if already applied (for up) or not applied (for down)
		var applied bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", ver).Scan(&applied)
		if err != nil {
			log.Fatalf("check migration %d: %v", ver, err)
		}

		if direction == "up" && applied {
			log.Printf("skip %d: already applied", ver)
			continue
		}
		if direction == "down" && !applied {
			log.Printf("skip %d: not applied", ver)
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsDir, fileName))
		if err != nil {
			log.Fatalf("read %s: %v", fileName, err)
		}

		log.Printf("running %s", fileName)
		tx, err := db.Begin()
		if err != nil {
			log.Fatalf("begin tx: %v", err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			// Mark dirty
			db.Exec("UPDATE schema_migrations SET dirty=TRUE WHERE version=$1", ver)
			tx.Rollback()
			log.Fatalf("migration %s failed: %v", fileName, err)
		}

		if direction == "up" {
			_, err = tx.Exec("INSERT INTO schema_migrations (version, dirty) VALUES ($1, FALSE) ON CONFLICT (version) DO UPDATE SET dirty=FALSE", ver)
		} else {
			_, err = tx.Exec("DELETE FROM schema_migrations WHERE version=$1", ver)
		}
		if err != nil {
			tx.Rollback()
			log.Fatalf("update schema_migrations: %v", err)
		}

		if err := tx.Commit(); err != nil {
			log.Fatalf("commit: %v", err)
		}
		log.Printf("done %s", fileName)
	}

	log.Println("all migrations complete")
}
