package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type requestBody struct {
	Messages []struct {
		Content string `json:"content"`
	} `json:"messages"`
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.BodyDump(func(c echo.Context, reqBody, resBody []byte) {
		fmt.Printf("Request body: %q\n", string(reqBody))
	}))
	e.POST("/v1/chat/completions", func(c echo.Context) error {
		defer c.Request().Body.Close()
		b, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		var req requestBody
		if err := json.Unmarshal(b, &req); err != nil {
			return fmt.Errorf("failed to unmarshal request body: %w", err)
		}

		m := "Hello, World!"
		if n := len(req.Messages); n > 0 {
			m = fmt.Sprintf("You said: %q", req.Messages[n-1].Content)
		}

		return c.JSON(http.StatusOK, map[string]any{
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": m,
					},
				},
			},
		})
	})
	e.Logger.Fatal(e.Start(":1323"))
}
