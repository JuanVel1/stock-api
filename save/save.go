package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"runtime"

	"github.com/joho/godotenv"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"context"
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

func main() {
	// Set up a recovery handler for panics
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Programa recuperado de pánico: %v\n", r)
			// Get stack trace
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			fmt.Printf("Stack trace: %s\n", buf[:n])
			os.Exit(1)
		}
	}()
	
	fmt.Println("Starting stock data fetcher...")
	
	// Cargar variables de entorno desde .env
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("Warning: Error loading .env file: %v\n", err)
	}
	
	// Verificar que las variables de entorno necesarias estén presentes
	apiKey := os.Getenv("DB_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: DB_API_KEY environment variable is missing or empty")
		os.Exit(1)
	}
	
	fmt.Println("Environment variables loaded successfully")
	
	// Inicializar base de datos
	if err := initDB(); err != nil {
		fmt.Printf("Error inicializando DB: %v\n", err)
		fmt.Println("Trying once more with increased timeouts...")
		// Try once more with a longer timeout before giving up
		time.Sleep(5 * time.Second)
		if err := initDB(); err != nil {
			fmt.Printf("Error inicializando DB en segundo intento: %v\n", err)
			os.Exit(1)
		}
	}
	defer func() {
		if db != nil {
			fmt.Println("Closing database connection...")
			db.Close()
		}
	}()

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

func initDB() error {
	var err error
	// Get DB URL from environment or use default
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
		fmt.Println("Using default database URL")
	}
	
	fmt.Printf("Connecting to database: %s\n", dbURL)
	
	// Try to connect to the database with retries
	maxRetries := 5
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Try to connect
		db, err = sqlx.Connect("postgres", dbURL)
		if err == nil {
			// Connection successful
			break
		}
		
		// Save the error and retry
		lastErr = err
		if attempt < maxRetries {
			// Calculate backoff with a jitter to prevent thundering herd
			backoff := time.Duration(math.Pow(2, float64(attempt-1))+float64(time.Now().UnixNano()%1000)/1000) * time.Second
			fmt.Printf("Database connection attempt %d/%d failed: %v\nRetrying in %v...\n", 
				attempt, maxRetries, err, backoff)
			time.Sleep(backoff)
		}
	}
	
	// If we still have an error after all retries, return it
	if err != nil {
		return fmt.Errorf("error conectando a la base de datos después de %d intentos: %v", maxRetries, lastErr)
	}
	
	// Verify connection with ping
	fmt.Println("Database connection established, verifying with ping...")
	err = db.Ping()
	if err != nil {
		return fmt.Errorf("error verificando conexión a la base de datos: %v", err)
	}
	fmt.Println("Database ping successful")

	// Configurar conexión
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	
	fmt.Println("Database connection pool configured")

	// Crear tabla si no existe
	// Drop existing table if it exists with the old schema
	_, err = db.Exec(`DROP TABLE IF EXISTS stocks`)
	if err != nil {
		return fmt.Errorf("error dropping existing table: %v", err)
	}
	
	// Create table with composite primary key
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS stocks (
            ticker TEXT NOT NULL,
            company TEXT,
            brokerage TEXT,
            action TEXT,
            rating_from TEXT,
            rating_to TEXT,
            target_from TEXT,
            target_to TEXT,
            time TEXT NOT NULL,
            PRIMARY KEY (ticker, time)
        )`)
	if err != nil {
		return fmt.Errorf("error creando tabla stocks: %v", err)
	}
	
	// Create index on ticker for quick lookups
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_stocks_ticker ON stocks (ticker)`)
	if err != nil {
		return fmt.Errorf("error creando índice en ticker: %v", err)
	}

	return nil
}

// checkDBConnection verifica que la conexión a la base de datos esté activa
// y reconecta si es necesario
func checkDBConnection() error {
	// Si la conexión es nil, intentar inicializar
	if db == nil {
		fmt.Println("Database connection is nil, trying to initialize...")
		return initDB()
	}
	
	// Verificar conexión con ping usando un contexto con timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	
	err := db.PingContext(ctx)
	if err != nil {
		fmt.Printf("Database connection lost: %v\n", err)
		fmt.Println("Attempting to reconnect...")
		return initDB()
	}
	
	// Log connection pool stats
	stats := db.Stats()
	fmt.Printf("DB connection pool stats: Open=%d, InUse=%d, Idle=%d\n", 
		stats.OpenConnections, stats.InUse, stats.Idle)
	
	return nil
}
func fetchStocks(nextPage string) ([]Stock, string, error) {
	// Create a function-scoped apiResponse that will be updated by the retry function
	var apiResponse APIResponse
	
	err := retryWithBackoff(3, func() error {
		url := "https://8j5baasof2.execute-api.us-west-2.amazonaws.com/production/swechallenge/list"
		if nextPage != "" {
			url += "?next_page=" + nextPage
		}
		
		fmt.Printf("Fetching stocks from URL: %s\n", url)
		
		// Create the request
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("error creando request: %v", err)
		}
		
		// Get API key and add headers
		apiKey := os.Getenv("DB_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("DB_API_KEY environment variable is missing or empty")
		}
		
		// Use the API key as-is since it already includes "Bearer " prefix in the .env file
		req.Header.Add("Authorization", apiKey)
		req.Header.Add("Content-Type", "application/json")
		fmt.Println("Request headers set: Content-Type=application/json, Authorization=[HIDDEN]")
		
		// Create client and make the request
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error haciendo request: %v", err)
		}
		defer resp.Body.Close()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("error de API: status code %d", resp.StatusCode)
		}
		
		// Read the response body
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error leyendo response: %v", err)
		}
		
		// Print response length for debugging
		fmt.Printf("Received response with length: %d bytes\n", len(responseBody))
		
		// Create a local response object for unmarshaling
		var localResponse APIResponse
		
		// Debug output to see the response body if unmarshaling fails
		err = json.Unmarshal(responseBody, &localResponse)
		if err != nil {
			fmt.Printf("Error unmarshaling JSON: %v\n", err)
			fmt.Printf("Response body: %s\n", string(responseBody))
			return fmt.Errorf("error unmarshaling JSON: %v", err)
		}
		
		// On success, update the function-scoped apiResponse
		apiResponse = localResponse
		
		fmt.Printf("Successfully unmarshaled JSON with %d items\n", len(apiResponse.Items))
		return nil
	})

	if err != nil {
		return nil, "", err
	}

	return apiResponse.Items, apiResponse.NextPage, nil
}

// attemptTransaction intenta ejecutar una transacción con el lote de stocks
func attemptTransaction(ctx context.Context, batch []Stock) error {
	// Begin transaction with default isolation level
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error iniciando transacción: %v", err)
	}
	
	// Ensure transaction is rolled back if it fails
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()
	
	// Usamos NamedExec para inserción por lotes
	query := `
		INSERT INTO stocks (
			ticker, company, brokerage, action, 
			rating_from, rating_to, target_from, target_to, time
		) VALUES (
			:ticker, :company, :brokerage, :action,
			:rating_from, :rating_to, :target_from, :target_to, :time
		) ON CONFLICT (ticker, time) DO UPDATE SET
			company = EXCLUDED.company,
			brokerage = EXCLUDED.brokerage,
			action = EXCLUDED.action,
			rating_from = EXCLUDED.rating_from,
			rating_to = EXCLUDED.rating_to,
			target_from = EXCLUDED.target_from,
			target_to = EXCLUDED.target_to`
	
	// Execute the query with detailed error logging and context timeout
	_, err = tx.NamedExecContext(ctx, query, batch)
	if err != nil {
		return fmt.Errorf("error ejecutando consulta: %v", err)
	}
	
	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("error confirmando transacción: %v", err)
	}
	
	// Mark tx as nil so it doesn't get rolled back in the defer
	tx = nil
	return nil
}

// isRetryableError determina si un error de base de datos se puede reintentar
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	errMsg := err.Error()
	return contains(errMsg, "deadlock") ||
		contains(errMsg, "timeout") ||
		contains(errMsg, "connection") ||
		contains(errMsg, "restart") ||
		contains(errMsg, "try again") ||
		contains(errMsg, "lock") ||
		contains(errMsg, "concurrent") ||
		contains(errMsg, "retry") ||
		contains(errMsg, "conflict")
}

// processBatch procesa un lote de stocks y los guarda en la base de datos
func processBatch(batch []Stock) error {
	if len(batch) == 0 {
		return nil
	}
	
	fmt.Printf("Procesando lote de %d stocks...\n", len(batch))
	
	// Verificar conexión antes de procesar
	if err := checkDBConnection(); err != nil {
		return fmt.Errorf("error verificando conexión a la base de datos: %v", err)
	}
	
	// Retryable transaction logic
	maxRetries := 5 // Increased from 3 to 5 for more resilience
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create context with timeout for the transaction - increase timeout for each retry
		// Use a shorter timeout for early attempts, longer for later attempts
		timeout := time.Duration(10*attempt) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		
		// Log the transaction attempt
		fmt.Printf("Intentando transacción %d/%d con timeout de %v...\n", 
			attempt, maxRetries, timeout)
			
		// Attempt the transaction
		err := attemptTransaction(ctx, batch)
		cancel() // cancel the context regardless of outcome
		
		// If successful, return
		if err == nil {
			fmt.Printf("Lote de %d stocks guardado exitosamente (intento %d/%d)\n", 
				len(batch), attempt, maxRetries)
			return nil
		}
		
		// Log the error
		// Log the error
		fmt.Printf("Error en transacción (intento %d/%d): %v\n", attempt, maxRetries, err)
		lastErr = err
		
		// Check if we should retry
		if attempt < maxRetries && isRetryableError(err) {
			// If it's a connection error, try to reconnect
			if contains(err.Error(), "connection") || 
			   contains(err.Error(), "broken") || 
			   contains(err.Error(), "reset by peer") ||
			   contains(err.Error(), "i/o timeout") {
				fmt.Println("Error de conexión detectado, intentando reconectar...")
				if reconnectErr := initDB(); reconnectErr != nil {
					fmt.Printf("Error al reconectar: %v\n", reconnectErr)
				} else {
					fmt.Println("Reconexión exitosa, continuando con la transacción...")
				}
			}
			// Exponential backoff between retries
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * 500 * time.Millisecond
			fmt.Printf("Reintentando en %v...\n", backoff)
			time.Sleep(backoff)
			continue
		}
		
		// If we're here, it's either the last attempt or a non-retryable error
		break
	}
	
	// If we get here, all attempts failed
	fmt.Println("Muestra de registros en el lote:")
	for i := 0; i < min(3, len(batch)); i++ {
		stock := batch[i]
		fmt.Printf("  - %d: ticker=%s, company=%s, time=%s\n", 
			i, stock.Ticker, stock.Company, stock.Time)
	}
	
	// Return the last error
	return fmt.Errorf("error insertando stocks después de %d intentos: %v", maxRetries, lastErr)
}

// cleanupResources realiza una limpieza de recursos y fuerza la recolección de basura
func cleanupResources() {
	// Close and reopen idle connections to prevent stale connections
	if db != nil {
		fmt.Println("Cleaning up database connections...")
		
		// Force the database to clear idle connections by temporarily setting max idle to 0
		// then back to our standard value
		db.SetMaxIdleConns(0)
		db.SetMaxIdleConns(25)
		
		// Log current connection stats
		stats := db.Stats()
		fmt.Printf("Connection pool after cleanup: Open=%d, InUse=%d, Idle=%d\n", 
			stats.OpenConnections, stats.InUse, stats.Idle)
	}
	
	// Run garbage collection to free memory
	runtime.GC()
	
	// Force a short sleep to allow system to stabilize
	time.Sleep(500 * time.Millisecond)
}

func saveStocks(stocks []Stock) error {
	if len(stocks) == 0 {
		return nil
	}
	
	fmt.Printf("Guardando %d stocks en la base de datos\n", len(stocks))
	
	// Verificar conexión a la base de datos antes de comenzar
	if err := checkDBConnection(); err != nil {
		return fmt.Errorf("error verificando conexión inicial: %v", err)
	}
	
	// Definir tamaño de lote - reducimos para evitar problemas de memoria
	const batchSize = 25 // Reduced from 50 to 25 for smaller batches
	var successCount int
	var failedCount int
	
	// Procesar en lotes
	for i := 0; i < len(stocks); i += batchSize {
		end := i + batchSize
		if end > len(stocks) {
			end = len(stocks)
		}
		
		batch := stocks[i:end]
		fmt.Printf("Procesando lote %d/%d (%d registros)\n", 
			(i/batchSize)+1, (len(stocks)+batchSize-1)/batchSize, len(batch))
		
		// Procesar lote con manejo de errores
		err := processBatch(batch)
		if err != nil {
			fmt.Printf("Error procesando lote %d: %v\n", (i/batchSize)+1, err)
			failedCount += len(batch)
		} else {
			successCount += len(batch)
		}
		
		// Pequeña pausa entre lotes para evitar sobrecargar la BD
		if end < len(stocks) {
			time.Sleep(100 * time.Millisecond)
			
			// Verificar conexión periódicamente entre lotes
			if (i/batchSize)%5 == 0 {
				// Clean up resources periodically
				cleanupResources()
				
				if err := checkDBConnection(); err != nil {
					fmt.Printf("Warning: Error checking connection between batches: %v\n", err)
				}
			}
		}
	}
	
	fmt.Printf("Proceso completado: %d stocks guardados exitosamente, %d fallidos\n", 
		successCount, failedCount)
	
	if failedCount > 0 {
		return fmt.Errorf("hubo errores al guardar %d stocks", failedCount)
	}
	
	fmt.Println("Database transaction completed successfully")
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

func retryWithBackoff(attempts int, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		// Espera exponencial: 1s, 2s, 4s, 8s, etc.
		waitTime := time.Duration(math.Pow(2, float64(i))) * time.Second
		fmt.Printf("Intento %d falló, reintentando en %v...\n", i+1, waitTime)
		time.Sleep(waitTime)
	}
	return err
}

// Helper functions
func contains(s string, substr string) bool {
	if s == "" {
		return false
	}
	return strings.Contains(s, substr)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ParsePriceString convierte un string de precio (ej. "$135.00") a float64
func ParsePriceString(price string) (float64, error) {
	// Si el precio está vacío o es nulo, retorna 0
	if price == "" || price == "null" || price == "N/A" {
		return 0, nil
	}
	
	// Eliminar el símbolo de dólar, espacios, y comas
	price = strings.TrimSpace(price)
	price = strings.ReplaceAll(price, "$", "")
	price = strings.ReplaceAll(price, ",", "")
	
	// Convertir a float64
	return strconv.ParseFloat(price, 64)
}

// FormatPriceFloat formatea un float64 como un string de precio (ej. "$135.00")
func FormatPriceFloat(price float64) string {
	if price == 0 {
		return ""
	}
	return fmt.Sprintf("$%.2f", price)
}
