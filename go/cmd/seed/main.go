// Command seed inserts a known test user into a database so you can log in while
// testing locally. It uses the same byte-compatible crypto the server uses, so
// the created user authenticates exactly like a real one.
//
// Usage:
//
//	go run ./cmd/seed -dsn /abs/path/to/db.sqlite \
//	    -email you@example.test -password secret -salt 0123456789abcdef
//
// -salt must match the ECONUMO_DATA_SALT you run the server with (16 bytes for
// AES-128). The user's per-row password salt is generated for you.
package main

import (
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"time"

	_ "modernc.org/sqlite"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/infra/auth"
)

func main() {
	dsn := flag.String("dsn", "", "sqlite file path (required)")
	email := flag.String("email", "test@econumo.test", "login email")
	password := flag.String("password", "password", "login password")
	dataSalt := flag.String("salt", "0123456789abcdef", "ECONUMO_DATA_SALT (must match the server)")
	name := flag.String("name", "Test User", "display name")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("seed: -dsn is required")
	}

	enc := auth.NewEncodeService(*dataSalt)
	hasher := auth.NewPasswordHasher()

	// Per-user salt: sha1(random 10 bytes) -> 40 hex chars, matching the app.
	b := make([]byte, 10)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		log.Fatal(err)
	}
	sum := sha1.Sum(b)
	userSalt := hex.EncodeToString(sum[:])

	id := vo.NewId().String()
	identifier := enc.Hash(*email) // md5(email + dataSalt) -> CHAR(32) lookup id
	encEmail, err := enc.Encode(*email)
	if err != nil {
		log.Fatal(err)
	}
	pwHash := hasher.Hash(*password, userSalt)

	db, err := sql.Open("sqlite", *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = db.Exec(
		`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		id, identifier, encEmail, *name, "", pwHash, userSalt, now, now,
	)
	if err != nil {
		log.Fatalf("seed: insert user: %v", err)
	}

	fmt.Printf("seeded user:\n  id:       %s\n  email:    %s\n  password: %s\n", id, *email, *password)
}
