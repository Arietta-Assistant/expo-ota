package db

import (
	"expo-open-ota/config"
	"log"
	"time"

	"gorm.io/driver/postgres"
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
	dsn := config.GetEnv("DATABASE_URL")
	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	// Auto-migrate the schema
	err = db.AutoMigrate(&User{})
	if err != nil {
		log.Fatalf("Error migrating database: %v", err)
	}
}

// UpsertUser creates or updates a user record
func UpsertUser(user User) error {
	return db.Where(User{ID: user.ID}).
		Assign(map[string]interface{}{
			"email":        user.Email,
			"name":         user.Name,
			"last_seen":    user.LastSeen,
			"update_count": gorm.Expr("update_count + 1"),
		}).
		FirstOrCreate(&user).Error
}

// GetUsers returns all users ordered by last seen
func GetUsers() ([]User, error) {
	var users []User
	err := db.Order("last_seen DESC").Find(&users).Error
	return users, err
}

// GetUserByID returns a user by ID
func GetUserByID(id string) (*User, error) {
	var user User
	err := db.First(&user, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
