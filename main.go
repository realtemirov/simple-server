package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

type App struct {
	db  *sql.DB
	rdb *redis.Client
}

type Person struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	app := App{}

	// Connect to the database
	db, err := dbConn()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	app.db = db

	// Connect to the Redis server
	rdb, err := redisConn()
	if err != nil {
		panic(err)
	}
	app.rdb = rdb

	http.HandleFunc("/person", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			app.postMethod(w, r)
		}
	})

	http.HandleFunc("/person/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			app.getMethod(w, r)
		}
	})

	http.HandleFunc("/", app.pingMethod)

	println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}

func dbConn() (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		"localhost", 5432, "postgres", "postgres", "postgres")

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	fmt.Println("Connected to PostgreSQL")

	return db, nil
}

func redisConn() (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // Redis server address
		Password: "",               // No password set
		DB:       0,                // Default database
	})

	// Ping the Redis server to test the connection
	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		return nil, err
	}

	fmt.Println("Connected to Redis:", pong)

	return rdb, nil
}

func (a *App) pingMethod(w http.ResponseWriter, _ *http.Request) {
	log.Println("PING request received")

	response := map[string]string{"ping": "pong"}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Fatal(err)
	}
}

func (a *App) getMethod(w http.ResponseWriter, r *http.Request) {
	log.Println("GET request received")

	parts := strings.Split(r.URL.Path, "/")

	// We expect the ID to be the last part of the URL
	if len(parts) > 1 {
		id := parts[len(parts)-1]
		cmd := a.rdb.Get(context.Background(), id)
		if err := cmd.Err(); err != nil {
			log.Println("failed to get data from redis:", err.Error())
			response := map[string]string{"error": err.Error()}
			
			// Return a 500 Internal Server Error
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode(response); err != nil {
				log.Fatal(err)
			}
			return
		}

		name, err := cmd.Result()
		if err != nil {
			log.Println("failed to get result:", err.Error())
			response := map[string]string{"error": err.Error()}
			
			// Return a 500 Internal Server Error
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode(response); err != nil {
				log.Fatal(err)
			}
			return
		}

		response := map[string]interface{}{"id": id, "name": name}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Fatal(err)
		}
	} else {
		http.Error(w, "ID is missing", http.StatusBadRequest)
	}
}

func (a *App) postMethod(w http.ResponseWriter, r *http.Request) {
	log.Println("POST request received")

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()
	log.Println("Request body:", string(body))

	// Unmarshal JSON into Person struct
	var person Person
	if err := json.Unmarshal(body, &person); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
	}
	log.Println("Person:", person)

	// Get the person from the database
	row := a.db.QueryRow(`SELECT * FROM people WHERE id = $1`, person.ID)

	// Scan the row
	if errScan := row.Scan(&person.ID, &person.Name); errScan != nil {
		log.Println("failed to scan row:", errScan.Error())
		response := map[string]string{"error": errScan.Error()}
		// Return a 500 Internal Server Error
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Fatal(err)
		}
	}

	// Set the person in Redis
	err = a.rdb.Set(
		context.Background(),
		fmt.Sprintf("%d", person.ID),
		person.Name,
		0).Err()
	if err != nil {
		log.Fatalf("Could not set key: %v", err)
	}

	response := map[string]interface{}{"id": person.ID, "name": person.Name}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Fatal(err)
	}
}
