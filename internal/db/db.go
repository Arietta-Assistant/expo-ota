package db

import (
	"time"

	"gorm.io/gorm"
)

var db *gorm.DB

type User struct {
	ID          string `gorm:"primaryKey"`
	Email       string `gorm:"uniqueIndex"`
	Name        string
	LastSeen    time.Time
	UpdateCount int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func init() {
	// Initialize database connection
	// dsn := config.GetEnv("DATABASE_URL")
	// var err error
	// db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	// if err != nil {
	// 	log.Fatalf("Error connecting to database: %v", err)
	// }

	// // Auto-migrate the schema
	// err = db.AutoMigrate(&User{})
	// if err != nil {
	// 	log.Fatalf("Error migrating database: %v", err)
	// }
}

// UpsertUser creates or updates a user record
func UpsertUser(user User) error {
	return nil
}

// GetUsers returns all users ordered by last seen
func GetUsers() ([]User, error) {
	return []User{}, nil
}

// GetUserByID returns a user by ID
func GetUserByID(id string) (*User, error) {
	return nil, nil
}
