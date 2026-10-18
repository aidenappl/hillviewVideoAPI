// Command migrate applies a hand-written SQL file from migrations/ to the
// database in DATABASE_DSN. It prints the target host and database (never the
// credentials) and only executes with -apply.
//
//	go run ./tools/migrate migrations/2026-10-18-mux.sql          # dry run
//	go run ./tools/migrate -apply migrations/2026-10-18-mux.sql   # execute
//
// Each file is executed as one statement batch; keep migrations idempotent or
// check -apply output before re-running.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/go-sql-driver/mysql"
)

func main() {
	apply := flag.Bool("apply", false, "execute the migration (default: dry run)")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("usage: migrate [-apply] <file.sql>")
	}

	dsn, ok := os.LookupEnv("DATABASE_DSN")
	if !ok {
		log.Fatal("DATABASE_DSN is not set")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		log.Fatal("DATABASE_DSN does not parse")
	}
	cfg.MultiStatements = true

	script, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("target: %s/%s\n", cfg.Addr, cfg.DBName)

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping: %v", err)
	}

	if !*apply {
		fmt.Printf("dry run: would execute %s (%d bytes); pass -apply\n", flag.Arg(0), len(script))
		return
	}
	if _, err := db.Exec(string(script)); err != nil {
		log.Fatalf("apply %s: %v", flag.Arg(0), err)
	}
	fmt.Printf("applied %s\n", flag.Arg(0))
}
