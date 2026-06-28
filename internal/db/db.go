package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func InitDB(dbPath string) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create db directory: %v", err)
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-8000&_foreign_keys=on")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	migrateSchema()

	log.Println("Database initialized successfully.")
}

func migrateSchema() {
	var userVersion int
	DB.QueryRow("PRAGMA user_version").Scan(&userVersion)

	log.Printf("Current schema version: %d", userVersion)

	if userVersion < 1 {
		migrateToV1()
		userVersion = 1
	}

	// 后续迁移由此递增：
	// if userVersion < 2 {
	//     migrateToV2()
	//     userVersion = 2
	// }

	log.Printf("Schema migration complete. Version: %d", userVersion)
}

func migrateToV1() {
	log.Println("Running schema migration to v1...")

	// Drop old tables (data will be lost — intentional per v1 migration)
	DB.Exec("DROP TABLE IF EXISTS submissions")
	DB.Exec("DROP TABLE IF EXISTS forms")

	createTables := `
	CREATE TABLE forms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		query_password TEXT,
		notify_email TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE submissions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		form_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		email TEXT,
		phone TEXT,
		content TEXT NOT NULL,
		source_url TEXT,
		resend_status TEXT,
		client_ip TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (form_id) REFERENCES forms(id)
	);
	`
	_, err := DB.Exec(createTables)
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	DB.Exec("PRAGMA user_version = 1")
	log.Println("Schema migration to v1 complete.")
}
