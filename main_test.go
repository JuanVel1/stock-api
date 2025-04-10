package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCalculateRatingChange verifica la función de cálculo de cambio de rating
func TestCalculateRatingChange(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		expected string
	}{
		{"Upgrade Neutral to Buy", "Neutral", "Buy", "De Neutral a Buy"},
		{"Downgrade Buy to Neutral", "Buy", "Neutral", "De Buy a Neutral"},
		{"Same Rating", "Buy", "Buy", "Mantiene Buy"},
		{"Empty Input", "", "", "Mantiene "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateRatingChange(tt.from, tt.to)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFormatTargetChange verifica el formateo de cambios en precio objetivo
func TestFormatTargetChange(t *testing.T) {
	tests := []struct {
		name     string
		change   float64
		expected string
	}{
		{"Positive Change", 5.25, "+$5.25"},
		{"Negative Change", -3.50, "-$3.50"},
		{"No Change", 0, "Sin cambio"},
		{"Small Positive", 0.01, "+$0.01"},
		{"Small Negative", -0.01, "-$0.01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTargetChange(tt.change)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCalculateStockScore verifica el cálculo del puntaje de acciones
func TestCalculateStockScore(t *testing.T) {
	now := time.Now()
	recentTime := now.Add(-24 * time.Hour) // Hace 1 día
	oldTime := now.Add(-240 * time.Hour)   // Hace 10 días

	tests := []struct {
		name     string
		stock    Stock
		time     time.Time
		expected float64
	}{
		{
			"Upgrade with Target Increase",
			Stock{
				RatingFrom: "Neutral",
				RatingTo:   "Buy",
				TargetFrom: 100,
				TargetTo:   120,
				Brokerage:  "The Goldman Sachs Group",
				Action:     "upgraded by",
			},
			recentTime,
			(10 * 0.4) + (10 * 0.3) + (6 * 0.15) + (8.57 * 0.1) + (8 * 0.05), // Cálculo manual
		},
		{
			"Reiterated with No Change",
			Stock{
				RatingFrom: "Buy",
				RatingTo:   "Buy",
				TargetFrom: 50,
				TargetTo:   50,
				Brokerage:  "Other Broker",
				Action:     "reiterated by",
			},
			recentTime,
			(0 * 0.4) + (0 * 0.3) + (4.5 * 0.15) + (8.57 * 0.1) + (5 * 0.05), // Cálculo manual
		},
		{
			"Old Downgrade",
			Stock{
				RatingFrom: "Buy",
				RatingTo:   "Neutral",
				TargetFrom: 75,
				TargetTo:   60,
				Brokerage:  "Needham & Company LLC",
				Action:     "target lowered by",
			},
			oldTime,
			0, // No debería ser negativo
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateStockScore(tt.stock, tt.time)
			assert.InDelta(t, tt.expected, result, 0.1) // Permite pequeñas diferencias por floats
		})
	}
}

// TestProcessRecommendations verifica el procesamiento de recomendaciones
func TestProcessRecommendations(t *testing.T) {
    now := time.Now()
    stocks := []Stock{
        // Recomendación "menor" para AAPL
        {
            Ticker:     "AAPL",
            RatingFrom: "Neutral",
            RatingTo:   "Buy",
            TargetFrom: 150,
            TargetTo:   180,
            Brokerage:  "Morgan Stanley",
            Action:     "upgraded by",
            Time:       now.Format(time.RFC3339),
        },
        // Recomendación "mejor" para AAPL
        {
            Ticker:     "AAPL",
            RatingFrom: "Buy",
            RatingTo:   "Strong Buy",
            TargetFrom: 180,
            TargetTo:   200,
            Brokerage:  "The Goldman Sachs Group",
            Action:     "upgraded by",
            Time:       now.Add(-12 * time.Hour).Format(time.RFC3339),
        },
    }

    recommendations := processRecommendations(stocks)
    
    require.Len(t, recommendations, 1)
    require.Equal(t, "De Buy a Strong Buy", recommendations[0].RatingChange)
    require.Equal(t, 200.0, recommendations[0].TargetTo)
    
    // Verificar que el score es mayor que el mínimo esperado
    // Score mínimo esperado: 
    // Rating (Buy->Strong Buy = 1 * 20 = 20)
    // Target ((200-180)/180*100*0.5 ≈ 5.55)
    // Broker (1.2 * 8 = 9.6)
    // Recency (~9)
    // Action (8)
    // Strong Buy bonus (15)
    // Total ≈ 20 + 5.55 + 9.6 + 9 + 8 + 15 ≈ 67.15
    require.Greater(t, recommendations[0].Score, 60.0)
}

// TestRecommendationsEndpoint verifica el endpoint de recomendaciones
func TestRecommendationsEndpoint(t *testing.T) {
	// Configurar router
	router := setupRouter()

	// Crear request
	req, _ := http.NewRequest("GET", "/api/recommendations", nil)
	resp := httptest.NewRecorder()

	// Ejecutar request
	router.ServeHTTP(resp, req)

	// Verificar respuesta
	assert.Equal(t, http.StatusOK, resp.Code)

	var recommendations []StockRecommendation
	err := json.Unmarshal(resp.Body.Bytes(), &recommendations)
	assert.NoError(t, err)

	// Verificar que devuelve un array (puede estar vacío si no hay datos de prueba)
	assert.NotNil(t, recommendations)
}

// TestStocksEndpoint verifica el endpoint de stocks
func TestStocksEndpoint(t *testing.T) {
	// Configurar router
	router := setupRouter()

	// Crear request
	req, _ := http.NewRequest("GET", "/api/stocks", nil)
	resp := httptest.NewRecorder()

	// Ejecutar request
	router.ServeHTTP(resp, req)

	// Verificar respuesta
	assert.Equal(t, http.StatusOK, resp.Code)

	var stocks []Stock
	err := json.Unmarshal(resp.Body.Bytes(), &stocks)
	assert.NoError(t, err)

	// Verificar que devuelve un array
	assert.NotNil(t, stocks)
}

// setupRouter crea un router para testing
func setupRouter() *gin.Engine {
	// Configurar base de datos de prueba (podrías usar una base de datos en memoria para tests)
	db = sqlx.MustConnect("postgres", "postgresql://root@localhost:26257/defaultdb?sslmode=disable")

	// Configurar router
	router := gin.Default()
	router.GET("/api/stocks", func(c *gin.Context) {
		var stocks []Stock
		err := db.Select(&stocks, "SELECT * FROM stocks LIMIT 100")
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, stocks)
	})
	router.GET("/api/recommendations", getStockRecommendations)

	return router
}
