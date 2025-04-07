package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Get database URL from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	// Connect to database
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	// Read migration file
	migrationFile := "migrations/001_initial_schema.sql"
	content, err := ioutil.ReadFile(migrationFile)
	if err != nil {
		log.Fatalf("Error reading migration file: %v", err)
	}

	// Split into individual statements
	statements := strings.Split(string(content), ";")

	// Execute each statement
	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		err := db.Exec(stmt).Error
		if err != nil {
			log.Fatalf("Error executing statement %d: %v\nStatement: %s", i+1, err, stmt)
		}
		fmt.Printf("Successfully executed statement %d\n", i+1)
	}

	fmt.Println("Migrations applied successfully!")
}
