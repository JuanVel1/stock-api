package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var db *sqlx.DB

func main() {

	godotenv.Load(".env")

	// 1. Conectar a CockroachDB
	db = sqlx.MustConnect("postgres", os.Getenv("DB_URL"))
	port := os.Getenv("PORT")

	// 2. Crear API
	r := gin.Default()

	// Configura el middleware CORS para permitir solicitudes desde el frontend
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Configura proxies confiables
	trustedProxies := []string{
		"192.168.101.0/24",
		"192.168.253.0/24",
		"169.254.0.0/16",
		"127.0.0.1",
		"::1",
		"fe80::/10",
	}

	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		log.Fatalf("Error configurando proxies: %v", err)
	}

	// 3. Endpoints
	r.GET("/api/stocks", func(c *gin.Context) {
		var stocks []Stock
		err := db.Select(&stocks, "SELECT * FROM stocks LIMIT 100")
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, stocks)
	})

	r.GET("/api/recommendations", getStockRecommendations)

	// 4. Iniciar servidor

	r.Run(":" + port) // Cambia el puerto según sea necesario
}

type Stock struct {
	Ticker     string  `json:"ticker" db:"ticker"`
	Company    string  `json:"company" db:"company"`
	Brokerage  string  `json:"brokerage" db:"brokerage"`
	Action     string  `json:"action" db:"action"`
	RatingFrom string  `json:"rating_from" db:"rating_from"`
	RatingTo   string  `json:"rating_to" db:"rating_to"`
	TargetFrom float64 `json:"target_from" db:"target_from"`
	TargetTo   float64 `json:"target_to" db:"target_to"`
	Time       string  `json:"time" db:"time"`
}

type StockRecommendation struct {
	Stock
	Score         float64 `json:"score"`
	RatingChange  string  `json:"rating_change"`
	TargetChange  string  `json:"target_change"`
	PercentChange float64 `json:"percent_change"`
}

// Brokerage reputation scores
var brokerageReputation = map[string]float64{
	"The Goldman Sachs Group": 1.2,
	"Morgan Stanley":          1.1,
	"JPMorgan Chase & Co.":    1.15,
	"Citigroup":               1.05,
	"Benchmark":               1.0,
	"Needham & Company LLC":   0.95,
	"Wedbush":                 0.98,
	"Truist Financial":        0.97,
	"Other":                   0.9,
}

// Rating values for scoring
var ratingValues = map[string]float64{
	"Sell":           0,
	"Underweight":    1,
	"Neutral":        2,
	"Market Perform": 2,
	"Buy":            3,
	"Outperform":     3,
	"Strong Buy":     4,
}

func getStockRecommendations(c *gin.Context) {
	// Consultar todos los stocks de la base de datos
	var stocks []Stock
	query := `SELECT 
		ticker, company, brokerage, action, rating_from, rating_to, 
		target_from, target_to, time 
	FROM stocks`
	err := db.Select(&stocks, query)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Procesar los stocks para generar recomendaciones
	recommendations := processRecommendations(stocks)

	// Ordenar por puntaje descendente
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Score > recommendations[j].Score
	})

	// Limitar a las 5 mejores recomendaciones
	if len(recommendations) > 5 {
		recommendations = recommendations[:5]
	}

	c.JSON(200, recommendations)
}

func processRecommendations(stocks []Stock) []StockRecommendation {
	stockMap := make(map[string]StockRecommendation)

	for _, stock := range stocks {
		lastUpdated, err := time.Parse(time.RFC3339, stock.Time)
		if err != nil {
			continue
		}

		currentScore := calculateStockScore(stock, lastUpdated)
		currentRec := StockRecommendation{
			Stock:        stock,
			Score:        currentScore,
			RatingChange: calculateRatingChange(stock.RatingFrom, stock.RatingTo),
			TargetChange: formatTargetChange(stock.TargetTo - stock.TargetFrom),
		}

		if existing, exists := stockMap[stock.Ticker]; !exists || currentScore > existing.Score {
			stockMap[stock.Ticker] = currentRec
		}
	}

	// Convertir a slice y ordenar
	var recommendations []StockRecommendation
	for _, rec := range stockMap {
		recommendations = append(recommendations, rec)
	}

	sort.Slice(recommendations, func(i, j int) bool {
		if recommendations[i].Score == recommendations[j].Score {
			return recommendations[i].TargetTo > recommendations[j].TargetTo
		}
		return recommendations[i].Score > recommendations[j].Score
	})

	return recommendations
}

func calculateStockScore(stock Stock, lastUpdated time.Time) float64 {
	// 1. Puntaje por cambio de rating (más peso)
	ratingScore := (ratingValues[stock.RatingTo] - ratingValues[stock.RatingFrom]) * 20

	// 2. Puntaje por cambio en precio objetivo (porcentaje)
	var targetChangeScore float64
	if stock.TargetFrom > 0 {
		percentChange := ((stock.TargetTo - stock.TargetFrom) / stock.TargetFrom) * 100
		targetChangeScore = percentChange * 0.5
	}

	// 3. Puntaje por reputación del bróker (más diferenciación)
	brokerScore := brokerageReputation[stock.Brokerage] * 8

	// 4. Puntaje por actividad reciente (últimos 7 días)
	recencyScore := 0.0
	if time.Since(lastUpdated).Hours() <= 168 {
		recencyScore = 10 - (time.Since(lastUpdated).Hours() / 16.8)
	}

	// 5. Puntaje por tipo de acción
	actionScore := 0.0
	switch {
	case strings.Contains(stock.Action, "upgraded"):
		actionScore = 8
	case strings.Contains(stock.Action, "initiated"):
		actionScore = 6
	case strings.Contains(stock.Action, "reiterated"):
		actionScore = 5
	}

	// Bonus especial para Strong Buy
	strongBuyBonus := 0.0
	if stock.RatingTo == "Strong Buy" {
		strongBuyBonus = 15
	}

	totalScore := ratingScore + targetChangeScore + brokerScore +
		recencyScore + actionScore + strongBuyBonus

	return math.Max(0, totalScore)
}

func calculateRatingChange(from, to string) string {
	if from == to {
		return "Mantiene " + from
	}
	return fmt.Sprintf("De %s a %s", from, to)
}

func formatTargetChange(change float64) string {
	if change > 0 {
		return fmt.Sprintf("+$%.2f", change)
	} else if change < 0 {
		return fmt.Sprintf("-$%.2f", math.Abs(change))
	}
	return "Sin cambio"
}
