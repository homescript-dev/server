package geolocation

import (
	"encoding/json"
	"fmt"
	"homescript-server/internal/logger"
	"io"
	"net/http"
	"time"
)

// Location represents geographic coordinates
type Location struct {
	Latitude  float64
	Longitude float64
	City      string
	Country   string
}

// GetLocationByIP tries to determine location from public IP address
// Uses free ip-api.com service (no API key required, 45 requests/minute limit)
func GetLocationByIP() (*Location, error) {
	logger.Debug("Attempting to determine location from IP address...")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Use ip-api.com free service
	resp, err := client.Get("http://ip-api.com/json/?fields=status,message,country,city,lat,lon")
	if err != nil {
		return nil, fmt.Errorf("failed to get location from IP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("location API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Status  string  `json:"status"`
		Message string  `json:"message"`
		Country string  `json:"country"`
		City    string  `json:"city"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("location API error: %s", result.Message)
	}

	location := &Location{
		Latitude:  result.Lat,
		Longitude: result.Lon,
		City:      result.City,
		Country:   result.Country,
	}

	logger.Info("Detected location from IP: %s, %s (%.4f, %.4f)",
		location.City, location.Country, location.Latitude, location.Longitude)

	return location, nil
}
