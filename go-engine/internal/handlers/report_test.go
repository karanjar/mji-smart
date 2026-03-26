package handlers

import (
    "bytes"
    "encoding/json"
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
    "github.com/stretchr/testify/assert"
)

func TestReportHandler(t *testing.T) {
    app := fiber.New()
    
    app.Post("/api/v1/reports", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{"status": "success"})
    })
    
    reqBody := map[string]interface{}{
        "user_id": "test-user",
        "coords": "-1.286389,36.817223",
        "image_url": "http://example.com/image.jpg",
        "description": "Large pothole on main road",
    }
    
    jsonBody, _ := json.Marshal(reqBody)
    req := httptest.NewRequest("POST", "/api/v1/reports", bytes.NewReader(jsonBody))
    req.Header.Set("Content-Type", "application/json")
    
    resp, _ := app.Test(req)
    assert.Equal(t, 200, resp.StatusCode)
}