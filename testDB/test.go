package main

import (
    "fmt" 

    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq"
    "github.com/joho/godotenv"
)

func main() {
    godotenv.Load()
    dbURL := "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
    fmt.Println("Connecting with:", dbURL)

    db, err := sqlx.Connect("postgres", dbURL)
    if err != nil {
        fmt.Println("Connection error:", err)
        return
    }
    defer db.Close()

    err = db.Ping()
    if err != nil {
        fmt.Println("Ping error:", err)
        return
    }
    fmt.Println("Connection successful!")
}