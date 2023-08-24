package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-faker/faker/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var (
	addr   = flag.String("addr", ":1323", "HTTP server address")
	resLen = flag.Uint("res-len", 20, "Number of words to return per response")
)

type requestBody struct {
	Stream   bool `json:"stream"`
	Messages []struct {
		Content string `json:"content"`
	} `json:"messages"`
}

type chunkReader struct {
	ID           string
	Created      int64
	Chunks       []string
	SentFinished bool
	SentDone     bool
	Delay        time.Duration
}

func newChunkReader(cs []string, d time.Duration) chunkReader {
	return chunkReader{

		Chunks: cs,
		Delay:  d,
	}
}

func (r *chunkReader) done() bool {
	return r.SentFinished && r.SentDone
}

func (r *chunkReader) next() ([]byte, error) {
	// Check if done...
	if r.SentDone {
		return nil, nil
	}

	// Check if done...
	if r.SentFinished {
		b := []byte("data: [DONE]\n\n")
		r.SentDone = true
		return b, nil
	}

	// If out of chunks, send done...
	if len(r.Chunks) == 0 {
		// End with a final message showing stop-reason...
		d := map[string]any{
			"id":      r.ID,
			"object":  "chat.completion.chunk",
			"created": r.Created,
			"model":   "gpt-3.5-turbo",
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]string{},
					"finish_reason": "stop",
				},
			},
		}

		// Convert it to JSON...
		b, err := json.Marshal(d)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal chunk: %w", err)
		}
		r.SentFinished = true
		b = append(
			[]byte("data: "),
			b...,
		)
		b = append(
			b,
			[]byte("\n\n")...,
		)
		return b, nil
	}

	// Otherwise take a chunk and format it...
	c := r.Chunks[0] + " "
	d := map[string]any{
		"id":      r.ID,
		"object":  "chat.completion.chunk",
		"created": r.Created,
		"model":   "gpt-3.5-turbo",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]string{
					"content": c,
				},
				"finish_reason": nil,
			},
		},
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chunk: %w", err)
	}
	r.Chunks = r.Chunks[1:]
	b = append(
		[]byte("data: "),
		b...,
	)
	b = append(
		b,
		[]byte("\n\n")...,
	)
	return b, nil
}

func (r *chunkReader) Read(p []byte) (int, error) {
	// Check if done...
	if r.done() {
		// Then return eof...
		return 0, io.EOF
	}

	// Check if done...
	if r.SentFinished {
		b := []byte("data: [DONE]\n\n")

		// Copy over the data...
		n := copy(p, b)

		// Mark as done...
		r.SentDone = true

		// Return the byte count...
		return n, nil
	}

	// If out of chunks, send done...
	if len(r.Chunks) == 0 {
		// End with a final message showing stop-reason...
		d := map[string]any{
			"id":      r.ID,
			"object":  "chat.completion.chunk",
			"created": r.Created,
			"model":   "gpt-3.5-turbo",
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]string{},
					"finish_reason": "stop",
				},
			},
		}

		// Convert it to JSON...
		b, err := json.Marshal(d)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal chunk: %w", err)
		}

		// Copy over the data...
		b = append(
			[]byte("data: "),
			b...,
		)
		b = append(
			b,
			[]byte("\n\n")...,
		)
		n := copy(p, b)

		// Mark as done...
		r.SentFinished = true

		// Return the byte count...
		return n, nil
	}

	// Format this chunk...
	c := r.Chunks[0] + " "
	d := map[string]any{
		"id":      r.ID,
		"object":  "chat.completion.chunk",
		"created": r.Created,
		"model":   "gpt-3.5-turbo",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]string{
					"content": c,
				},
				"finish_reason": nil,
			},
		},
	}

	// Convert it to JSON...
	b, err := json.Marshal(d)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal chunk: %w", err)
	}

	// Otherwise, read the next chunk...
	b = append(
		[]byte("data: "),
		b...,
	)
	b = append(
		b,
		[]byte("\n\n")...,
	)
	n := copy(p, b)

	// Remove the chunk from the list...
	r.Chunks = r.Chunks[1:]

	// Sleep for the delay...
	time.Sleep(r.Delay)

	// Return the number of bytes read...
	return n, nil
}

func main() {
	// Parse and validate the flags...
	flag.Parse()
	if addr == nil || *addr == "" {
		log.Panic("missing -addr flag")
	}
	if resLen == nil {
		log.Panic("missing -res-len flag")
	}
	if *resLen == 0 {
		log.Panic("-res-len must be greater than 0")
	}

	// Create the app...
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.BodyDump(func(c echo.Context, reqBody, resBody []byte) {
		fmt.Printf("Request body: %q\n", string(reqBody))
	}))

	// Define the routes...
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

		// Get the random response message...
		var ws []string
		for i := 0; i < int(*resLen); i++ {
			w := faker.Word()
			if i > 0 {
				w = " " + w
			}
			ws = append(ws, w)
		}

		// If not streaming, return directly...
		if !req.Stream {
			m := strings.Join(ws, "")
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
		}

		id := faker.UUIDHyphenated()
		ct := time.Now().Unix()
		sd := time.Millisecond * 500

		// Otherwise, create a JSON encoder to write to the response...
		enc := json.NewEncoder(c.Response())

		// Write some headers...
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		c.Response().WriteHeader(http.StatusOK)

		// Loop through the chunks...
		for _, w := range ws {
			d := map[string]any{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": ct,
				"model":   "gpt-3.5-turbo",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]string{
							"content": w,
						},
						"finish_reason": nil,
					},
				},
			}
			if err := enc.Encode(d); err != nil {
				return err
			}
			c.Response().Flush()
			time.Sleep(sd)
		}

		// Write the finished message...
		d := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": ct,
			"model":   "gpt-3.5-turbo",
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]string{},
					"finish_reason": "stop",
				},
			},
		}
		if err := enc.Encode(d); err != nil {
			return err
		}
		c.Response().Flush()
		time.Sleep(sd)

		// Write the done message and return...
		c.Response().Write([]byte("[done]\n"))
		c.Response().Flush()
		return nil
	})

	// Start the app...
	if err := e.Start(*addr); err != nil {
		e.Logger.Fatal(err)
	}
	// if err := e.StartTLS(*addr, "cert.pem", "key.pem"); err != nil {
	// 	e.Logger.Fatal(err)
	// }
}
