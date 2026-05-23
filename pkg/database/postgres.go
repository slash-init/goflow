package database

import (
	"fmt"
	"log"

	//we don't call lib/pq directly, but importing it registers the postgres driver for sqlx/database/sql
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/slash-init/goflow/pkg/config"
)

func NewPostgresDB(cfg *config.Config) *sqlx.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName,
	)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatal("Could not connect to PostgreSQL: ", err)
	}

	log.Println("PostgreSQL connected successfully")
	return db
}
