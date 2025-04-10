package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Stock define la estructura de datos para cada acción
type Stock struct {
	Ticker     string `json:"ticker" db:"ticker"`
	Company    string `json:"company" db:"company"`
	Brokerage  string `json:"brokerage" db:"brokerage"`
	Action     string `json:"action" db:"action"`
	RatingFrom string `json:"rating_from" db:"rating_from"`
	RatingTo   string `json:"rating_to" db:"rating_to"`
	TargetFrom string `json:"target_from" db:"target_from"`
	TargetTo   string `json:"target_to" db:"target_to"`
	Time       string `json:"time" db:"time"`
}

// APIResponse estructura para parsear la respuesta JSON
type APIResponse struct {
	Items    []Stock `json:"items"`
	NextPage string  `json:"next_page"`
}

var db *sqlx.DB

func initDB() error {
	var err error
	db, err = sqlx.Connect("postgres", os.Getenv("DB_URL"))
	if err != nil {
		return fmt.Errorf("error conectando a la base de datos: %v", err)
	}

	// Configurar conexión
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	return nil
}

func fetchStocks(nextPage string) ([]Stock, string, error) {
	url := "https://8j5baasof2.execute-api.us-west-2.amazonaws.com/production/swechallenge/list"

	if nextPage != "" {
		url += "?next_page=" + nextPage
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creando request: %v", err)
	}

	req.Header.Add("Authorization", "Bearer "+os.Getenv("API_KEY"))
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error haciendo request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("error leyendo respuesta: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("API retornó error: %s, body: %s", resp.Status, string(body))
	}

	var apiResponse APIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, "", fmt.Errorf("error parseando JSON: %v", err)
	}

	return apiResponse.Items, apiResponse.NextPage, nil
}

func saveStocks(stocks []Stock) error {
	if len(stocks) == 0 {
		return nil
	}

	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("error iniciando transacción: %v", err)
	}

	// Usamos NamedExec para inserción por lotes
	query := `
		INSERT INTO stocks (
			ticker, company, brokerage, action, 
			rating_from, rating_to, target_from, target_to, time
		) VALUES (
			:ticker, :company, :brokerage, :action,
			:rating_from, :rating_to, :target_from, :target_to, :time
		) ON CONFLICT (ticker) DO UPDATE SET
			company = EXCLUDED.company,
			brokerage = EXCLUDED.brokerage,
			action = EXCLUDED.action,
			rating_from = EXCLUDED.rating_from,
			rating_to = EXCLUDED.rating_to,
			target_from = EXCLUDED.target_from,
			target_to = EXCLUDED.target_to,
			time = EXCLUDED.time`

	_, err = tx.NamedExec(query, stocks)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error insertando stocks: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error confirmando transacción: %v", err)
	}

	fmt.Printf("Guardados/actualizados %d stocks en la base de datos\n", len(stocks))
	return nil
}

func fetchAllStocks() ([]Stock, error) {
	var allStocks []Stock
	nextPage := ""

	for {
		stocks, newNextPage, err := fetchStocks(nextPage)
		if err != nil {
			return nil, err
		}

		allStocks = append(allStocks, stocks...)

		if newNextPage == "" {
			break
		}

		nextPage = newNextPage
		time.Sleep(500 * time.Millisecond) // Espera para no saturar la API
	}

	return allStocks, nil
}

func main() {
	godotenv.Load() // Cargar variables de entorno desde .env

	// Inicializar base de datos
	if err := initDB(); err != nil {
		fmt.Printf("Error inicializando DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Obtener todos los stocks
	allStocks, err := fetchAllStocks()
	if err != nil {
		fmt.Printf("Error obteniendo stocks: %v\n", err)
		os.Exit(1)
	}

	// Guardar en base de datos
	if err := saveStocks(allStocks); err != nil {
		fmt.Printf("Error guardando stocks: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Proceso completado exitosamente!")
}
